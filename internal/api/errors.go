package api

import "fmt"

// APIError is a structured error returned by the HTTP API.
type APIError struct {
	Status    int
	Code      string
	ErrorCode int
	Message   string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Status > 0 {
		return fmt.Sprintf("api error: %d", e.Status)
	}
	return "api error"
}
