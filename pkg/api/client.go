package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultSite    = "datadoghq.com"
	defaultTimeout = 30
)

// Client is the Datadog API HTTP client.
type Client struct {
	config     ClientConfig
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Datadog API client with the given config.
func NewClient(cfg ClientConfig) *Client {
	if cfg.Site == "" {
		cfg.Site = defaultSite
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		baseURL: fmt.Sprintf("https://api.%s", cfg.Site),
	}
}

// Get performs a GET request to the given path and decodes the response into dest.
func (c *Client) Get(path string, params map[string]string, dest interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("DD-API-KEY", c.config.APIKey)
	req.Header.Set("DD-APPLICATION-KEY", c.config.AppKey)
	req.Header.Set("Content-Type", "application/json")

	if len(params) > 0 {
		q := req.URL.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Errors []string `json:"errors"`
		}
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && len(apiErr.Errors) > 0 {
			return &APIError{StatusCode: resp.StatusCode, Errors: apiErr.Errors}
		}
		return &APIError{StatusCode: resp.StatusCode, Message: string(body)}
	}

	if dest != nil {
		if err := json.Unmarshal(body, dest); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// Post performs a POST request to the given path with the given body.
func (c *Client) Post(path string, body io.Reader, dest interface{}) error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("DD-API-KEY", c.config.APIKey)
	req.Header.Set("DD-APPLICATION-KEY", c.config.AppKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Errors []string `json:"errors"`
		}
		if jsonErr := json.Unmarshal(respBody, &apiErr); jsonErr == nil && len(apiErr.Errors) > 0 {
			return &APIError{StatusCode: resp.StatusCode, Errors: apiErr.Errors}
		}
		return &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	if dest != nil {
		if err := json.Unmarshal(respBody, dest); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}
