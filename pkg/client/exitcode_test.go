package client

import (
	"errors"
	"testing"
)

func TestExitCodeForStatus(t *testing.T) {
	for status, want := range map[int]int{401: 3, 403: 5, 404: 4, 400: 1, 429: 1} {
		if got := ExitCodeForStatus(status); got != want {
			t.Errorf("ExitCodeForStatus(%d) = %d, want %d", status, got, want)
		}
	}
}

// TestPartialFailureCode pins how a multi-environment failure maps to one
// exit code: the shared code when every environment failed the same way,
// otherwise the general error code.
func TestPartialFailureCode(t *testing.T) {
	denied := EnvResult{Name: "a", Error: APIError(403, "")}
	tests := []struct {
		name string
		err  *PartialFailureError
		want int
	}{
		{"all 403", &PartialFailureError{Failed: []EnvResult{denied, {Name: "b", Error: APIError(403, "")}}, Total: 2}, ExitPermission},
		{"403 and 401", &PartialFailureError{Failed: []EnvResult{denied, {Name: "b", Error: APIError(401, "")}}, Total: 2}, ExitError},
		{"one ok, one 403", &PartialFailureError{Failed: []EnvResult{denied}, Total: 2}, ExitError},
		{"all transport errors", &PartialFailureError{Failed: []EnvResult{{Name: "a", Error: errors.New("refused")}}, Total: 1}, ExitError},
	}
	for _, tt := range tests {
		if got := tt.err.Code(); got != tt.want {
			t.Errorf("%s: Code() = %d, want %d", tt.name, got, tt.want)
		}
	}
}
