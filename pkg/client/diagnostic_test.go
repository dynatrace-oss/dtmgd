package client

import (
	"errors"
	"strings"
	"testing"
)

func TestDiagnoseError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"nil", nil, ""},
		{"401", errors.New("API error 401: Unauthorized"), "token"},
		{"403", errors.New("API error 403: Forbidden"), "scopes"},
		{"404", errors.New("API error 404: Not Found"), "not found"},
		{"429", errors.New("API error 429: Too Many Requests"), "Rate limited"},
		{"connection refused", errors.New("connection refused"), "host URL"},
		{"no such host", errors.New("dial tcp: lookup bad.host: no such host"), "DNS"},
		{"certificate", errors.New("x509: certificate signed by unknown authority"), "certificate"},
		{"timeout", errors.New("context deadline exceeded (Client.Timeout)"), "timed out"},
		{"token not found", errors.New("token \"prod\" not found"), "set-credentials"},
		{"no context", errors.New("no current context set"), "set-context"},
		{"unknown", errors.New("something weird"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := DiagnoseError(tt.err)
			if tt.contains == "" {
				if hint != "" {
					t.Errorf("expected empty hint, got: %s", hint)
				}
				return
			}
			if !strings.Contains(strings.ToLower(hint), strings.ToLower(tt.contains)) {
				t.Errorf("expected hint containing %q, got: %s", tt.contains, hint)
			}
		})
	}
}

func TestWrapWithDiagnosis(t *testing.T) {
	err := WrapWithDiagnosis(errors.New("API error 401: Unauthorized"))
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "401") {
		t.Error("original error should be preserved")
	}
	if !strings.Contains(msg, "💡") {
		t.Error("hint should be included")
	}
}

func TestWrapWithDiagnosisNil(t *testing.T) {
	if WrapWithDiagnosis(nil) != nil {
		t.Error("nil error should return nil")
	}
}

func TestWrapWithDiagnosisNoHint(t *testing.T) {
	original := errors.New("something unknown")
	wrapped := WrapWithDiagnosis(original)
	if wrapped != original {
		t.Error("error without hint should be returned unchanged")
	}
}
