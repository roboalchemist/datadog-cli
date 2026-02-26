package api

import "fmt"

// ClientConfig holds configuration for the Datadog API client.
type ClientConfig struct {
	APIKey  string
	AppKey  string
	Site    string
	Verbose bool
	Debug   bool
	Timeout int // seconds
}

// APIError represents a generic Datadog API error response for status codes
// not covered by a more specific error type.
type APIError struct {
	StatusCode int
	Message    string
	Errors     []string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if len(e.Errors) > 0 {
		return e.Errors[0]
	}
	return "unknown API error"
}

// AuthenticationError is returned for 401 and 403 responses.
type AuthenticationError struct {
	Message string
}

func (e *AuthenticationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "authentication failed"
}

// RateLimitError is returned for 429 responses.
type RateLimitError struct {
	Message    string
	RetryAfter int // seconds from Retry-After header, 0 if not present
}

func (e *RateLimitError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "rate limit exceeded"
}

// NotFoundError is returned for 404 responses.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "resource not found"
}

// BadRequestError is returned for 400 responses.
type BadRequestError struct {
	Message string
}

func (e *BadRequestError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "bad request"
}

// ServerError is returned for 5xx responses after retries are exhausted.
type ServerError struct {
	Message string
}

func (e *ServerError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "server error"
}

// NetworkError wraps a network-level error (connection refused, timeout, etc.)
// with a user-friendly message.
type NetworkError struct {
	Cause error
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error: %v", e.Cause)
}

func (e *NetworkError) Unwrap() error {
	return e.Cause
}

// PaginationMeta holds pagination metadata from API responses.
type PaginationMeta struct {
	Page       int `json:"page"`
	PageCount  int `json:"page_count"`
	PageSize   int `json:"page_size"`
	TotalCount int `json:"total_count"`
}

// ---- Traces / Spans API types ----

// SpanEvent is a single span event returned by the spans search API (v2).
type SpanEvent struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Attributes SpanAttributes `json:"attributes"`
}

// SpanAttributes holds the fields of a span event.
type SpanAttributes struct {
	// Top-level span fields
	Service      string                 `json:"service"`
	ResourceName string                 `json:"resource_name"`
	Name         string                 `json:"name"`
	TraceID      string                 `json:"trace_id"`
	SpanID       string                 `json:"span_id"`
	Duration     int64                  `json:"duration"`
	Error        int                    `json:"error"`
	Timestamp    string                 `json:"timestamp"`
	// Nested metadata
	Meta    map[string]interface{} `json:"meta"`
	Metrics map[string]interface{} `json:"metrics"`
	Tags    []string               `json:"tags"`
}

// SpanSearchResponse is the response envelope for POST /api/v2/spans/events/search.
type SpanSearchResponse struct {
	Data []SpanEvent    `json:"data"`
	Meta SpanSearchMeta `json:"meta"`
}

// SpanSearchMeta holds pagination metadata for span search responses.
type SpanSearchMeta struct {
	Page SpanSearchPage `json:"page"`
}

// SpanSearchPage contains cursor-based pagination info for span searches.
type SpanSearchPage struct {
	After string `json:"after"`
}

// SpanAggregateResponse is the response envelope for POST /api/v2/spans/analytics/aggregate.
type SpanAggregateResponse struct {
	Data []SpanAggregateBucket `json:"data"`
}

// SpanAggregateBucket is one result bucket from a span aggregation.
type SpanAggregateBucket struct {
	Type       string               `json:"type"`
	Attributes SpanBucketAttributes `json:"attributes"`
}

// SpanBucketAttributes holds the by and compute fields of an aggregate bucket.
type SpanBucketAttributes struct {
	By      map[string]interface{} `json:"by"`
	Compute map[string]interface{} `json:"compute"`
}
