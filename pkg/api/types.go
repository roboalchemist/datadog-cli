package api

// ClientConfig holds configuration for the Datadog API client.
type ClientConfig struct {
	APIKey  string
	AppKey  string
	Site    string
	Debug   bool
	Timeout int // seconds
}

// APIError represents a Datadog API error response.
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

// PaginationMeta holds pagination metadata from API responses.
type PaginationMeta struct {
	Page      int `json:"page"`
	PageCount int `json:"page_count"`
	PageSize  int `json:"page_size"`
	TotalCount int `json:"total_count"`
}
