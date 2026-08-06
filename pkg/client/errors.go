package client

import (
	"encoding/json"
	"fmt"
)

// ErrAPI represents an HTTP error response from the Dynatrace Managed API.
type ErrAPI struct {
	StatusCode int
	// Body is the complete response body. It is kept for callers that need
	// it — the -vv response dump prints the raw body from resty directly —
	// but Error deliberately does not return all of it. See apiErrorMessage.
	Body string
}

// maxAPIMessageLen caps the API-supplied text that reaches a user-facing
// error. Dynatrace validation messages are a sentence; anything much longer
// is a stack trace or a dump, which is what the cap is there to stop.
const maxAPIMessageLen = 200

func (e *ErrAPI) Error() string {
	if msg := apiErrorMessage(e.Body); msg != "" {
		return fmt.Sprintf("API error %d: %s", e.StatusCode, msg)
	}
	return fmt.Sprintf("API error %d", e.StatusCode)
}

// apiErrorMessage extracts the human-readable message from a Dynatrace API
// error envelope, and nothing else.
//
// Forwarding the whole body used to put every field the cluster returned into
// stderr and into the agent-mode JSON envelope, where agent frameworks persist
// it. Observed content included constraint-violation paths and parameter
// locations, which describe the API's internal validation routing; on other
// Managed versions the same unconditional path could carry exception chains,
// internal service names or cluster hostnames.
//
// Only error.message is surfaced, because that is the part written for a
// human to read. A body in any other shape yields "", so the caller reports
// the status code alone rather than guessing what is safe to print.
func apiErrorMessage(body string) string {
	if body == "" {
		return ""
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return ""
	}
	msg := envelope.Error.Message
	if len(msg) > maxAPIMessageLen {
		msg = msg[:maxAPIMessageLen] + "…"
	}
	return msg
}

// APIError returns a new ErrAPI.
func APIError(code int, body string) error {
	return &ErrAPI{StatusCode: code, Body: body}
}
