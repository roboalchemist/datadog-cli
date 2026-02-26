package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// --- NewClient / URL construction ---

func TestNewClient_DefaultSite(t *testing.T) {
	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a"})
	if c.baseURL != "https://api.datadoghq.com/" {
		t.Errorf("expected default base URL, got %q", c.baseURL)
	}
}

func TestNewClient_EUSite(t *testing.T) {
	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a", Site: "datadoghq.eu"})
	if c.baseURL != "https://api.datadoghq.eu/" {
		t.Errorf("expected EU base URL, got %q", c.baseURL)
	}
}

func TestNewClient_GovSite(t *testing.T) {
	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a", Site: "ddog-gov.com"})
	if c.baseURL != "https://ddog-gov.com/" {
		t.Errorf("expected gov base URL (no api. prefix), got %q", c.baseURL)
	}
}

func TestNewClient_US3Site(t *testing.T) {
	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a", Site: "us3.datadoghq.com"})
	if c.baseURL != "https://api.us3.datadoghq.com/" {
		t.Errorf("expected US3 base URL, got %q", c.baseURL)
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a"})
	if c.httpClient.Timeout.Seconds() != 30 {
		t.Errorf("expected 30s default timeout, got %v", c.httpClient.Timeout)
	}
}

func TestNewClient_CustomTimeout(t *testing.T) {
	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a", Timeout: 10})
	if c.httpClient.Timeout.Seconds() != 10 {
		t.Errorf("expected 10s timeout, got %v", c.httpClient.Timeout)
	}
}

func TestNewClient_StoresKeys(t *testing.T) {
	c := NewClient(ClientConfig{APIKey: "my-api-key", AppKey: "my-app-key"})
	if c.apiKey != "my-api-key" {
		t.Errorf("apiKey not stored, got %q", c.apiKey)
	}
	if c.appKey != "my-app-key" {
		t.Errorf("appKey not stored, got %q", c.appKey)
	}
}

// --- GET / POST happy path ---

func newTestClient(t *testing.T, srv *httptest.Server) *DatadogClient {
	t.Helper()
	c := NewClient(ClientConfig{APIKey: "test-api-key", AppKey: "test-app-key"})
	c.baseURL = srv.URL + "/"
	c.retryMax = 0 // disable retries for basic tests
	return c
}

func TestGet_ReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	body, err := c.Get("/api/v1/validate", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestGet_WithQueryParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	params := url.Values{}
	params.Set("from", "1234")
	params.Set("to", "5678")
	_, err := c.Get("/api/v1/query", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "from=1234") {
		t.Errorf("expected from=1234 in query, got %q", gotQuery)
	}
}

func TestPost_SendsBody(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	payload := map[string]string{"query": "service:foo"}
	_, err := c.Post("/api/v2/logs/events/search", payload, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := gotBody["query"]; !ok || v != "service:foo" {
		t.Errorf("expected body to contain query=service:foo, got %v", gotBody)
	}
}

// --- Headers ---

func TestGet_SetsAuthHeaders(t *testing.T) {
	var gotAPIKey, gotAppKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("DD-API-KEY")
		gotAppKey = r.Header.Get("DD-APPLICATION-KEY")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Get("/api/v1/validate", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAPIKey != "test-api-key" {
		t.Errorf("DD-API-KEY header = %q, want test-api-key", gotAPIKey)
	}
	if gotAppKey != "test-app-key" {
		t.Errorf("DD-APPLICATION-KEY header = %q, want test-app-key", gotAppKey)
	}
}

func TestPost_SetsContentTypeJSON(t *testing.T) {
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Post("/api/v1/check_run", map[string]string{"k": "v"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
}

// --- Error mapping ---

func TestGet_401_ReturnsAuthenticationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":["Invalid API key"]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Get("/api/v1/validate", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*AuthenticationError); !ok {
		t.Errorf("expected *AuthenticationError, got %T: %v", err, err)
	}
}

func TestGet_403_ReturnsAuthenticationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["Forbidden"]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Get("/api/v1/monitor", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*AuthenticationError); !ok {
		t.Errorf("expected *AuthenticationError, got %T: %v", err, err)
	}
}

func TestGet_404_ReturnsNotFoundError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["not found"]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Get("/api/v1/monitor/999", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*NotFoundError); !ok {
		t.Errorf("expected *NotFoundError, got %T: %v", err, err)
	}
}

func TestGet_400_ReturnsBadRequestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":["invalid query"]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Get("/api/v1/query", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*BadRequestError); !ok {
		t.Errorf("expected *BadRequestError, got %T: %v", err, err)
	}
}

func TestGet_500_ReturnsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":["internal server error"]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	c.retryMax = 0 // no retries for speed
	_, err := c.Get("/api/v1/validate", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*ServerError); !ok {
		t.Errorf("expected *ServerError, got %T: %v", err, err)
	}
}

// --- Retry logic ---

func TestGet_429_RetriesAndSucceeds(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			// First call: rate limited with Retry-After: 0 to avoid sleep
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"errors":["rate limit exceeded"]}`))
			return
		}
		// Second call: success
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a"})
	c.baseURL = srv.URL + "/"
	c.retryMax = 3

	body, err := c.Get("/api/v1/validate", nil)
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if string(body) != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", body)
	}
	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 calls (1 retry), got %d", callCount)
	}
}

func TestGet_429_ExhaustsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errors":["rate limit exceeded"]}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a"})
	c.baseURL = srv.URL + "/"
	c.retryMax = 2

	_, err := c.Get("/api/v1/validate", nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if _, ok := err.(*RateLimitError); !ok {
		t.Errorf("expected *RateLimitError, got %T: %v", err, err)
	}
}

func TestGet_500_RetriesAndSucceeds(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a"})
	c.baseURL = srv.URL + "/"
	c.retryMax = 3

	_, err := c.Get("/api/v1/validate", nil)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 calls, got %d", callCount)
	}
}

// --- Non-retryable errors are NOT retried ---

func TestGet_401_DoesNotRetry(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a"})
	c.baseURL = srv.URL + "/"
	c.retryMax = 3

	_, err := c.Get("/api/v1/validate", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 call (no retries for 401), got %d", callCount)
	}
}

// --- extractErrorMessage ---

func TestExtractErrorMessage_ErrorsArray(t *testing.T) {
	body := []byte(`{"errors":["first error","second error"]}`)
	msg := extractErrorMessage(body)
	if !strings.Contains(msg, "first error") {
		t.Errorf("expected first error in message, got %q", msg)
	}
}

func TestExtractErrorMessage_ErrorField(t *testing.T) {
	body := []byte(`{"error":"something went wrong"}`)
	msg := extractErrorMessage(body)
	if msg != "something went wrong" {
		t.Errorf("expected 'something went wrong', got %q", msg)
	}
}

func TestExtractErrorMessage_MessageField(t *testing.T) {
	body := []byte(`{"message":"not found"}`)
	msg := extractErrorMessage(body)
	if msg != "not found" {
		t.Errorf("expected 'not found', got %q", msg)
	}
}

func TestExtractErrorMessage_EmptyBody(t *testing.T) {
	msg := extractErrorMessage(nil)
	if msg != "" {
		t.Errorf("expected empty string for nil body, got %q", msg)
	}
}

func TestExtractErrorMessage_InvalidJSON(t *testing.T) {
	msg := extractErrorMessage([]byte(`not json`))
	if msg != "" {
		t.Errorf("expected empty string for invalid JSON, got %q", msg)
	}
}

// --- Verbose/debug output ---

func TestClient_VerboseFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	// Just verify verbose client doesn't panic or error
	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a", Verbose: true})
	c.baseURL = srv.URL + "/"
	c.retryMax = 0
	_, err := c.Get("/api/v1/validate", nil)
	if err != nil {
		t.Errorf("verbose client should not error on 200, got: %v", err)
	}
}

func TestClient_DebugFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"debug":"response"}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a", Debug: true})
	c.baseURL = srv.URL + "/"
	c.retryMax = 0
	_, err := c.Post("/api/v1/check_run", map[string]string{"k": "v"}, nil)
	if err != nil {
		t.Errorf("debug client should not error on 200, got: %v", err)
	}
}

// --- RateLimitError with Retry-After header ---

func TestGet_429_WithRetryAfterHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errors":["rate limited"]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	c.retryMax = 0 // don't actually retry, just check error value
	_, err := c.Get("/api/v1/validate", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	rle, ok := err.(*RateLimitError)
	if !ok {
		t.Fatalf("expected *RateLimitError, got %T", err)
	}
	if rle.RetryAfter != 5 {
		t.Errorf("expected RetryAfter=5, got %d", rle.RetryAfter)
	}
}

// --- Unknown status code ---

func TestGet_UnknownStatus_ReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(418) // I'm a teapot
		_, _ = w.Write([]byte(`{"message":"teapot"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Get("/api/v1/validate", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*APIError); !ok {
		t.Errorf("expected *APIError, got %T: %v", err, err)
	}
}

// --- Network error path ---

func TestGet_NetworkError_RetryAndFail(t *testing.T) {
	// Start and immediately close a server so all requests get connection refused
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close immediately — all connection attempts will fail

	c := NewClient(ClientConfig{APIKey: "k", AppKey: "a"})
	c.baseURL = srv.URL + "/"
	c.retryMax = 1 // only 1 retry to keep test fast

	_, err := c.Get("/api/v1/validate", nil)
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if _, ok := err.(*NetworkError); !ok {
		t.Errorf("expected *NetworkError, got %T: %v", err, err)
	}
}

// --- URL path construction ---

func TestGet_PathWithLeadingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _ = c.Get("/api/v1/validate", nil)
	if gotPath != "/api/v1/validate" {
		t.Errorf("expected path /api/v1/validate, got %q", gotPath)
	}
}

// --- Error type .Error() methods ---

func TestErrorTypes_ErrorMethods(t *testing.T) {
	// APIError
	e1 := &APIError{StatusCode: 422, Message: "unprocessable"}
	if e1.Error() != "unprocessable" {
		t.Errorf("APIError.Error() = %q", e1.Error())
	}
	e1empty := &APIError{}
	if e1empty.Error() != "unknown API error" {
		t.Errorf("APIError empty = %q", e1empty.Error())
	}
	e1errors := &APIError{Errors: []string{"first"}}
	if e1errors.Error() != "first" {
		t.Errorf("APIError with errors = %q", e1errors.Error())
	}

	// AuthenticationError
	e2 := &AuthenticationError{Message: "bad key"}
	if e2.Error() != "bad key" {
		t.Errorf("AuthenticationError.Error() = %q", e2.Error())
	}
	e2empty := &AuthenticationError{}
	if e2empty.Error() != "authentication failed" {
		t.Errorf("AuthenticationError empty = %q", e2empty.Error())
	}

	// RateLimitError
	e3 := &RateLimitError{Message: "slow down"}
	if e3.Error() != "slow down" {
		t.Errorf("RateLimitError.Error() = %q", e3.Error())
	}
	e3empty := &RateLimitError{}
	if e3empty.Error() != "rate limit exceeded" {
		t.Errorf("RateLimitError empty = %q", e3empty.Error())
	}

	// NotFoundError
	e4 := &NotFoundError{Message: "gone"}
	if e4.Error() != "gone" {
		t.Errorf("NotFoundError.Error() = %q", e4.Error())
	}
	e4empty := &NotFoundError{}
	if e4empty.Error() != "resource not found" {
		t.Errorf("NotFoundError empty = %q", e4empty.Error())
	}

	// BadRequestError
	e5 := &BadRequestError{Message: "invalid"}
	if e5.Error() != "invalid" {
		t.Errorf("BadRequestError.Error() = %q", e5.Error())
	}
	e5empty := &BadRequestError{}
	if e5empty.Error() != "bad request" {
		t.Errorf("BadRequestError empty = %q", e5empty.Error())
	}

	// ServerError
	e6 := &ServerError{Message: "crash"}
	if e6.Error() != "crash" {
		t.Errorf("ServerError.Error() = %q", e6.Error())
	}
	e6empty := &ServerError{}
	if e6empty.Error() != "server error" {
		t.Errorf("ServerError empty = %q", e6empty.Error())
	}

	// NetworkError
	cause := &AuthenticationError{Message: "test"}
	e7 := &NetworkError{Cause: cause}
	if e7.Error() == "" {
		t.Errorf("NetworkError.Error() returned empty")
	}
	if e7.Unwrap() != cause {
		t.Errorf("NetworkError.Unwrap() returned wrong error")
	}
}

// --- mapStatusError edge cases ---

func TestMapStatusError_400_WithEmptyBody(t *testing.T) {
	c := &DatadogClient{}
	err := c.mapStatusError(400, []byte{}, http.Header{})
	badReq, ok := err.(*BadRequestError)
	if !ok {
		t.Fatalf("expected *BadRequestError, got %T", err)
	}
	if badReq.Message == "" {
		t.Error("expected non-empty message for 400 with empty body")
	}
}

func TestMapStatusError_404_WithCustomMessage(t *testing.T) {
	c := &DatadogClient{}
	err := c.mapStatusError(404, []byte(`{"errors":["monitor 42 not found"]}`), http.Header{})
	nfe, ok := err.(*NotFoundError)
	if !ok {
		t.Fatalf("expected *NotFoundError, got %T", err)
	}
	if !strings.Contains(nfe.Message, "monitor 42 not found") {
		t.Errorf("expected message to contain 'monitor 42 not found', got %q", nfe.Message)
	}
}
