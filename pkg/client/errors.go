package client

import "fmt"

// ErrAPI represents an HTTP error response from the Dynatrace Managed API.
type ErrAPI struct {
	StatusCode int
	Body       string
}

func (e *ErrAPI) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Body)
}

// APIError returns a new ErrAPI.
func APIError(code int, body string) error {
	return &ErrAPI{StatusCode: code, Body: body}
}
