package api

// ErrorResponse is a generic JSON error wrapper.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}
