package client

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// realBody is the shape a Dynatrace Managed cluster actually returns for a
// rejected request. constraintViolations describes the API's internal
// validation routing — parameter names and whether each was matched in the
// path or the query string — and used to be forwarded verbatim to stderr and
// into the agent-mode JSON envelope.
const realBody = `{"error":{"code":400,"message":"Constraints violated.","constraintViolations":[` +
	`{"path":"problemId","message":"Invalid format.","parameterLocation":"PATH","location":null},` +
	`{"path":"entityId","message":"Invalid format.","parameterLocation":"QUERY","location":null}]}}`

func TestErrAPIDoesNotForwardWholeBody(t *testing.T) {
	err := APIError(400, realBody)

	got := err.Error()

	// The actionable sentence survives.
	if !strings.Contains(got, "Constraints violated.") {
		t.Errorf("error dropped the message users need: %q", got)
	}
	// The internal fields do not.
	for _, leak := range []string{"parameterLocation", "constraintViolations", "PATH", "QUERY", "problemId"} {
		if strings.Contains(got, leak) {
			t.Errorf("error still leaks %q: %q", leak, got)
		}
	}
}

func TestErrAPIUnknownBodyShape(t *testing.T) {
	// A body that is not the Dynatrace envelope could be anything — an HTML
	// error page from a proxy, a Java stack trace, a plain string. Nothing is
	// guessed at: the status code stands alone.
	for _, body := range []string{
		"Unauthorized",
		"<html><body>502 Bad Gateway - internal-lb-07.corp</body></html>",
		`{"unexpected":"shape"}`,
		"",
	} {
		got := APIError(403, body).Error()
		if got != "API error 403" {
			t.Errorf("body %q produced %q, want the bare status", body, got)
		}
	}
}

func TestErrAPICapsMessageLength(t *testing.T) {
	long := strings.Repeat("A", maxAPIMessageLen*3)
	err := APIError(500, `{"error":{"message":"`+long+`"}}`)

	got := err.Error()

	if len(got) > maxAPIMessageLen+64 {
		t.Errorf("message was not capped: %d chars", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncation was not signalled: %q", got)
	}
}

// TestErrAPIStillDiagnosable is the regression this change could plausibly
// have caused: Diagnose matches on status-code substrings, so stripping
// the body must not take the hints with it.
func TestErrAPIStillDiagnosable(t *testing.T) {
	cases := map[int]string{
		401: "Authentication failed",
		403: "Permission denied",
		404: "Resource not found",
		429: "Rate limited",
	}
	for code, wantHint := range cases {
		hint := Diagnose(APIError(code, "Unauthorized"))
		if !strings.Contains(hint, wantHint) {
			t.Errorf("status %d lost its diagnosis: got %q, want it to contain %q", code, hint, wantHint)
		}
	}
}

// TestErrAPINoCrossStatusFalseMatch is a side benefit worth pinning: with the
// body forwarded, a 400 whose text merely mentioned "404" was diagnosed as a
// missing resource. Dropping the body removes that confusion.
func TestErrAPINoCrossStatusFalseMatch(t *testing.T) {
	err := APIError(400, `{"error":{"message":"see RFC 404 for details"}}`)

	if hint := Diagnose(err); strings.Contains(hint, "Resource not found") {
		t.Errorf("a 400 was diagnosed from text in its own message: %q", hint)
	}
}

// TestErrAPICapsMessageByCharacters pins that the cap counts characters:
// cutting a message of three-byte characters at byte 200 split one in half.
func TestErrAPICapsMessageByCharacters(t *testing.T) {
	long := strings.Repeat("€", maxAPIMessageLen*2)
	got := APIError(400, `{"error":{"message":"`+long+`"}}`).Error()
	if !utf8.ValidString(got) {
		t.Fatalf("message is not valid UTF-8: %q", got)
	}
	if n := strings.Count(got, "€"); n != maxAPIMessageLen {
		t.Errorf("kept %d characters, want %d", n, maxAPIMessageLen)
	}
}
