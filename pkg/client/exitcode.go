package client

import (
	"errors"
	"fmt"
	"strings"
)

// Exit codes, shared with dtctl so scripts can branch on the cause of a
// failure the same way for both tools.
const (
	ExitSuccess    = 0
	ExitError      = 1
	ExitUsage      = 2
	ExitAuth       = 3 // HTTP 401
	ExitNotFound   = 4 // HTTP 404
	ExitPermission = 5 // HTTP 403
)

// ExitCodeForStatus maps an HTTP status to an exit code.
func ExitCodeForStatus(status int) int {
	switch status {
	case 401:
		return ExitAuth
	case 403:
		return ExitPermission
	case 404:
		return ExitNotFound
	}
	return ExitError
}

// PartialFailureError reports that some environments of a multi-environment
// call failed. The others' results are still valid and are still printed.
type PartialFailureError struct {
	Failed []EnvResult // the failed environments, in config order
	Total  int         // how many environments were queried
}

func (e *PartialFailureError) Error() string {
	parts := make([]string, len(e.Failed))
	for i, r := range e.Failed {
		parts[i] = fmt.Sprintf("%s: %v", r.Name, r.Error)
	}
	return fmt.Sprintf("%d of %d environments failed: %s", len(e.Failed), e.Total, strings.Join(parts, "; "))
}

// Code is the exit code for the whole call: when every environment failed
// with the same kind of API error, that error's code (all 403 → 5);
// otherwise ExitError, since no single cause describes the outcome.
func (e *PartialFailureError) Code() int {
	if len(e.Failed) != e.Total || e.Total == 0 {
		return ExitError
	}
	code := -1
	for _, r := range e.Failed {
		var apiErr *ErrAPI
		if !errors.As(r.Error, &apiErr) {
			return ExitError
		}
		c := ExitCodeForStatus(apiErr.StatusCode)
		if code != -1 && c != code {
			return ExitError
		}
		code = c
	}
	return code
}
