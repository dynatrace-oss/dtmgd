package client

import (
	"errors"
	"strings"
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

// TestUnwrapSingleReportsFailedEnvironments pins that a multi-environment
// result with failures returns them as a PartialFailureError alongside the
// merged data, instead of dropping the error (which made such calls exit 0).
func TestUnwrapSingleReportsFailedEnvironments(t *testing.T) {
	results := []EnvResult{{Name: "a", Data: "ok"}, {Name: "b", Error: APIError(403, "")}}
	data, err := UnwrapSingle(results)
	var pf *PartialFailureError
	if !errors.As(err, &pf) || pf.Total != 2 || len(pf.Failed) != 1 {
		t.Fatalf("want a PartialFailureError for 1 of 2, got %v", err)
	}
	if !strings.Contains(err.Error(), "1 of 2 environments failed: b: API error 403") {
		t.Errorf("message = %q", err.Error())
	}
	if m := data.(map[string]interface{}); m["a"] != "ok" {
		t.Errorf("merged data lost the ok environment: %v", m)
	}
}
