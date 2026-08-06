package client

import (
	"errors"
	"fmt"
	"strings"
)

// DiagnoseError returns a user-friendly hint for common API errors.
func DiagnoseError(err error) string {
	if err == nil {
		return ""
	}

	// An *ErrAPI carries the status code as a number, so it is answered from
	// the code alone and the text is never consulted. Part of that text comes
	// from the server: a 400 whose message reads "see RFC 404 for details"
	// was previously diagnosed as a missing resource. An unrecognised status
	// yields no hint, which is correct — falling through to substring
	// matching is exactly the bug.
	var apiErr *ErrAPI
	if errors.As(err, &apiErr) {
		return diagnoseStatus(apiErr.StatusCode)
	}

	msg := err.Error()

	switch {
	// The status-substring cases remain for errors that are not *ErrAPI —
	// notably ones rebuilt from text, which the tests and older call paths
	// still produce.
	case strings.Contains(msg, "401"):
		return diagnoseStatus(401)
	case strings.Contains(msg, "403"):
		return diagnoseStatus(403)
	case strings.Contains(msg, "404"):
		return diagnoseStatus(404)
	case strings.Contains(msg, "429"):
		return diagnoseStatus(429)
	case strings.Contains(msg, "connection refused"):
		return "Connection refused. Check that the host URL is correct and the cluster is reachable.\n  Run: dtmgd get environments"
	case strings.Contains(msg, "no such host"):
		return "DNS resolution failed. Check the host URL in your context.\n  Run: dtmgd ctx"
	case strings.Contains(msg, "certificate"):
		return "TLS certificate error. The cluster may use a self-signed certificate.\n  If behind a proxy, configure: https-proxy in your context."
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
		return "Request timed out. The cluster may be slow or unreachable. Check network connectivity."
	case strings.Contains(msg, "token") && strings.Contains(msg, "not found"):
		return "Token reference not found. Store it with:\n  dtmgd config set-credentials <token-ref> --token <api-token>"
	case strings.Contains(msg, "no current context"):
		return "No context configured. Set one up with:\n  dtmgd config set-context <name> --host <url> --env-id <id> --token-ref <ref>"
	}
	return ""
}

// diagnoseStatus maps an HTTP status code to its hint, so the structured and
// text-matched paths above cannot drift apart.
func diagnoseStatus(code int) string {
	switch code {
	case 401:
		return "Authentication failed. Check your API token and ensure it hasn't expired.\n  Run: dtmgd config set-credentials <token-ref>"
	case 403:
		return "Permission denied. Your API token may be missing required scopes.\n  Required scopes: DataExport, ReadConfig, ReadLogContent, ReadEvents, ReadProblems, ReadSecurityProblems, ReadSLO"
	case 404:
		return "Resource not found. Check the ID and ensure you're using the correct environment."
	case 429:
		return "Rate limited by the Dynatrace API. Wait a moment and retry, or reduce request frequency."
	}
	return ""
}

// WrapWithDiagnosis wraps an error with a diagnostic hint if available.
func WrapWithDiagnosis(err error) error {
	if err == nil {
		return nil
	}
	hint := DiagnoseError(err)
	if hint == "" {
		return err
	}
	return fmt.Errorf("%w\n\n  💡 %s", err, hint)
}
