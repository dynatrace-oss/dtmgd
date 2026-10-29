package client

import (
	"errors"
	"strings"
	"testing"
)

func TestDiagnose(t *testing.T) {
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
			hint := Diagnose(tt.err)
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

// TestDiagnoseIgnoresPortNumbers pins that a status hint comes from an API
// error's status, not from any 40x in the text: an ephemeral port such as
// 40431 or 4030 made a refused connection "Resource not found" and a
// timeout "Permission denied".
func TestDiagnoseIgnoresPortNumbers(t *testing.T) {
	hint := Diagnose(errors.New("dial tcp 127.0.0.1:40431: connect: connection refused"))
	if !strings.Contains(hint, "Connection refused") || strings.Contains(hint, "Resource not found") {
		t.Errorf("connection refused on port 40431: hint %q", hint)
	}
	hint = Diagnose(errors.New("dial tcp 127.0.0.1:4030: i/o timeout"))
	if strings.Contains(hint, "Permission denied") || !strings.Contains(hint, "timed out") {
		t.Errorf("timeout on port 4030: hint %q", hint)
	}
}

// TestDiagnosePartialFailure pins that a multi-environment failure gets a
// status hint only when every failed environment shares the status: with
// 403 and 401 the first status-looking text in the message used to win.
func TestDiagnosePartialFailure(t *testing.T) {
	partial := func(codes ...int) error {
		p := &PartialFailureError{Total: len(codes)}
		for i, c := range codes {
			p.Failed = append(p.Failed, EnvResult{Name: string(rune('a' + i)), Error: APIError(c, "")})
		}
		return p
	}
	if hint := Diagnose(partial(403, 403)); !strings.Contains(hint, "Permission denied") {
		t.Errorf("all 403: hint %q, want the permission hint", hint)
	}
	if hint := Diagnose(partial(403, 401)); hint != "" {
		t.Errorf("403 and 401: hint %q, want none", hint)
	}
}
