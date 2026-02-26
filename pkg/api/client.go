package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSite     = "datadoghq.com"
	defaultTimeout  = 30
	defaultRetryMax = 3
	baseDelay       = time.Second
)

// retryableStatusCodes are HTTP status codes that should trigger a retry.
var retryableStatusCodes = map[int]bool{
	429: true,
	500: true,
	502: true,
	503: true,
	504: true,
}

// DatadogClient is the HTTP client for Datadog API requests.
// It handles authentication, retries with exponential backoff, rate limiting,
// and converts HTTP errors into typed error values.
type DatadogClient struct {
	baseURL    string
	apiKey     string
	appKey     string
	httpClient *http.Client
	verbose    bool
	debug      bool
	retryMax   int
}

// Client is a type alias for DatadogClient for backward compatibility.
type Client = DatadogClient

// NewClient creates a new DatadogClient from the given ClientConfig.
// The base URL is constructed as https://api.{site}/ using the site in the config.
// Special case: ddog-gov.com is used as-is (not prefixed with "api.").
// Override: if DD_API_URL env var is set, it is used as the base URL directly
// (useful for integration tests pointing at a local mock server).
func NewClient(cfg ClientConfig) *DatadogClient {
	site := cfg.Site
	if site == "" {
		site = defaultSite
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	var baseURL string
	// DD_API_URL overrides the constructed URL entirely (e.g. for integration tests)
	if override := os.Getenv("DD_API_URL"); override != "" {
		baseURL = override
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}
	} else if strings.Contains(site, "ddog-gov.com") {
		baseURL = fmt.Sprintf("https://%s/", site)
	} else {
		baseURL = fmt.Sprintf("https://api.%s/", site)
	}

	return &DatadogClient{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		appKey:  cfg.AppKey,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		verbose:  cfg.Verbose,
		debug:    cfg.Debug,
		retryMax: defaultRetryMax,
	}
}

// Get performs a GET request to the given path with optional query parameters.
// Returns the raw response body bytes or a typed error.
func (c *DatadogClient) Get(path string, params url.Values) ([]byte, error) {
	return c.doRequest(http.MethodGet, path, nil, params)
}

// Post performs a POST request to the given path with a JSON body and optional
// query parameters. Returns the raw response body bytes or a typed error.
func (c *DatadogClient) Post(path string, body interface{}, params url.Values) ([]byte, error) {
	return c.doRequest(http.MethodPost, path, body, params)
}

// doRequest executes an HTTP request with retry logic and error handling.
// It serializes body to JSON if non-nil, attaches auth headers, and handles
// retryable status codes (429, 500, 502, 503, 504) with exponential backoff.
// For 429 responses the Retry-After header is respected when present.
func (c *DatadogClient) doRequest(method, path string, body interface{}, params url.Values) ([]byte, error) {
	// Build full URL
	fullURL := c.baseURL + strings.TrimPrefix(path, "/")
	if len(params) > 0 {
		fullURL = fullURL + "?" + params.Encode()
	}

	// Serialize body to JSON if provided
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
	}

	// Verbose: log request URL
	if c.verbose || c.debug {
		fmt.Fprintf(os.Stderr, "→ %s %s\n", method, fullURL)
	}

	// Debug: log request body
	if c.debug && len(bodyBytes) > 0 {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, bodyBytes, "", "  "); err == nil {
			fmt.Fprintf(os.Stderr, "Request body: %s\n", pretty.String())
		} else {
			fmt.Fprintf(os.Stderr, "Request body: %s\n", string(bodyBytes))
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.retryMax; attempt++ {
		// Apply backoff delay for retry attempts (not the first attempt)
		if attempt > 0 {
			// Default exponential backoff: 1s, 2s, 4s
			delay := baseDelay * (1 << uint(attempt-1))

			// For rate-limit errors, use Retry-After if it was provided
			if rle, ok := lastErr.(*RateLimitError); ok && rle.RetryAfter > 0 {
				delay = time.Duration(rle.RetryAfter) * time.Second
			}

			time.Sleep(delay)
		}

		respBytes, statusCode, headers, err := c.executeRequest(method, fullURL, bodyBytes)
		if err != nil {
			// Network-level error — retry
			lastErr = &NetworkError{Cause: err}
			continue
		}

		// Debug: log response
		if c.debug {
			fmt.Fprintf(os.Stderr, "← Response (%d): %s\n", statusCode, string(respBytes))
		}

		// Success
		if statusCode >= 200 && statusCode < 300 {
			return respBytes, nil
		}

		// Map status code to a typed error
		typedErr := c.mapStatusError(statusCode, respBytes, headers)

		// Non-retryable errors are returned immediately
		if !retryableStatusCodes[statusCode] {
			return nil, typedErr
		}

		// Retryable — save the error and loop
		lastErr = typedErr
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("request failed after %d retries", c.retryMax)
}

// executeRequest performs a single HTTP request and returns the response body,
// status code, response headers, and any network-level error.
func (c *DatadogClient) executeRequest(method, fullURL string, bodyBytes []byte) ([]byte, int, http.Header, error) {
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("DD-API-KEY", c.apiKey)
	req.Header.Set("DD-APPLICATION-KEY", c.appKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, resp.Header, fmt.Errorf("reading response body: %w", err)
	}

	return respBytes, resp.StatusCode, resp.Header, nil
}

// mapStatusError converts an HTTP status code into the appropriate typed error.
func (c *DatadogClient) mapStatusError(statusCode int, body []byte, headers http.Header) error {
	msg := extractErrorMessage(body)

	switch {
	case statusCode == 400:
		if msg == "" {
			msg = "Bad request"
		}
		return &BadRequestError{Message: msg}

	case statusCode == 401:
		return &AuthenticationError{
			Message: "Authentication failed: Unauthorized (401).\n\n" +
				"Your credentials may be invalid, expired, or lack access to this endpoint.\n" +
				"Ensure your DD_API_KEY and DD_APP_KEY are correct and have the required permissions.\n" +
				"See: https://docs.datadoghq.com/account_management/api-app-keys/",
		}

	case statusCode == 403:
		return &AuthenticationError{
			Message: "Authorization failed: Access denied.\n\n" +
				"Your API key may not have permission to access this resource.\n" +
				"Ensure your DD_APPLICATION_KEY has the required permissions.\n" +
				"See: https://docs.datadoghq.com/account_management/api-app-keys/",
		}

	case statusCode == 404:
		if msg == "" {
			msg = "Resource not found"
		}
		return &NotFoundError{Message: msg}

	case statusCode == 429:
		retryAfter := 0
		if ra := headers.Get("Retry-After"); ra != "" {
			if v, err := strconv.Atoi(ra); err == nil {
				retryAfter = v
			}
		}
		rateMsg := "Rate limit exceeded."
		if retryAfter > 0 {
			rateMsg += fmt.Sprintf(" Retry after %d seconds.", retryAfter)
		}
		return &RateLimitError{Message: rateMsg, RetryAfter: retryAfter}

	case statusCode >= 500 && statusCode < 600:
		return &ServerError{
			Message: fmt.Sprintf(
				"Server error (%d). The Datadog API is experiencing issues.\nPlease try again later.",
				statusCode,
			),
		}

	default:
		if msg == "" {
			msg = fmt.Sprintf("Request failed with status %d", statusCode)
		}
		return &APIError{StatusCode: statusCode, Message: msg}
	}
}

// extractErrorMessage attempts to parse a Datadog API error response body and
// return a user-friendly message. Returns empty string if no message is found.
func extractErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	// Datadog API returns errors in three common shapes:
	//   {"errors": ["message1", "message2"]}
	//   {"error": "message"}
	//   {"message": "message"}
	if errs, ok := data["errors"]; ok {
		switch v := errs.(type) {
		case []interface{}:
			parts := make([]string, 0, len(v))
			for _, e := range v {
				parts = append(parts, fmt.Sprintf("%v", e))
			}
			return strings.Join(parts, "\n")
		default:
			return fmt.Sprintf("%v", v)
		}
	}

	if err, ok := data["error"]; ok {
		return fmt.Sprintf("%v", err)
	}

	if msg, ok := data["message"]; ok {
		return fmt.Sprintf("%v", msg)
	}

	return ""
}
