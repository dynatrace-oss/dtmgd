package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

// TestAgentErrorEnvelope pins the error envelope agents parse: a classified
// code, the HTTP status, and the hint as a suggestion rather than glued into
// the message.
func TestAgentErrorEnvelope(t *testing.T) {
	api := newFakeAPI(t, map[string]route{problemsPath: {Status: 403, File: "error_403.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-A")
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("stdout is not one envelope: %v\n%s", err, r.Stdout)
	}
	if resp.OK || resp.Error == nil {
		t.Fatalf("want ok:false with an error, got %+v", resp)
	}
	if resp.Error.Code != "permission_denied" || resp.Error.StatusCode != 403 || len(resp.Error.Suggestions) != 1 {
		t.Errorf("error = %+v; want permission_denied, 403, one suggestion", resp.Error)
	}
	if strings.Contains(resp.Error.Message, "💡") {
		t.Errorf("hint glued into the message: %q", resp.Error.Message)
	}
	if r.Code != 5 {
		t.Errorf("code = %d, want 5", r.Code)
	}
}

// TestAgentNoticeNamesAgent pins the auto-detection notice wording.
func TestAgentNoticeNamesAgent(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems")
	if !strings.Contains(r.Stderr, "Agent mode is on (detected claude") {
		t.Errorf("stderr = %q", r.Stderr)
	}
}

// TestUserAgentHasAgentSuffix pins that requests made for an agent say so,
// so cluster admins can tell agent traffic apart in their access logs.
func TestUserAgentHasAgentSuffix(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems")
	h := api.Headers(problemsPath)
	if len(h) == 0 || !strings.Contains(h[0].Get("User-Agent"), "AI-Agent:") {
		t.Errorf("User-Agent = %q", h)
	}
}
