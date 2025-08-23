package models

// APIError is the uniform error envelope returned by REST endpoints. The
// optional Code lets clients react programmatically (e.g. rate_limited)
// instead of matching on human-readable messages.
type APIError struct {
	Error string `json:"error" example:"something went wrong"`
	Code  string `json:"code,omitempty" example:"rate_limited"`
}
