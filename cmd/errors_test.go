package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

// TestExitCodes pins the exit-code contract: 3 for 401, 4 for 404, 5 for 403,
// and a non-zero code when any environment of a multi-environment call fails.
func TestExitCodes(t *testing.T) {
	single := []struct {
		fixture string
		status  int
		want    int
	}{
		{"error_401.json", 401, 3},
		{"error_404.json", 404, 4},
		{"error_403.json", 403, 5},
	}
	for _, tt := range single {
		api := newFakeAPI(t, map[string]route{problemsPath: {Status: tt.status, File: tt.fixture}})
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems")
		if r.Code != tt.want {
			t.Errorf("HTTP %d: code = %d, want %d (stderr %q)", tt.status, r.Code, tt.want, r.Stderr)
		}
	}

	deny := map[string]route{problemsPath: {Status: 403, File: "error_403.json"}}
	a, b := newFakeAPI(t, deny), newFakeAPI(t, deny)
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "problems", "--env", "a;b", "-o", "json")
	if r.Code != 5 || !strings.Contains(r.Stderr, "a: API error 403") || !strings.Contains(r.Stderr, "b: API error 403") {
		t.Errorf("both 403: code = %d, stderr = %q; want 5 naming a and b", r.Code, r.Stderr)
	}

	ok := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	denied := newFakeAPI(t, deny)
	r = runCLI(t, withFakeEnvs(t, fakeEnv{"a", ok}, fakeEnv{"b", denied}), "get", "problems", "--env", "a;b", "-o", "json")
	if r.Code != 1 {
		t.Errorf("one ok, one 403: code = %d, want 1", r.Code)
	}
}

func TestClosest(t *testing.T) {
	cands := []string{"problems", "entities", "events", "slos"}
	if got, ok := closest("prblems", cands); !ok || got != "problems" {
		t.Errorf("closest(prblems) = %q, %v", got, ok)
	}
	if got, ok := closest("EVENTS", cands); !ok || got != "events" {
		t.Errorf("closest(EVENTS) = %q, %v", got, ok)
	}
	if _, ok := closest("frobnicate", cands); ok {
		t.Error("closest(frobnicate) matched; want no match beyond distance 2")
	}
}

// TestDidYouMeanResource pins that a mistyped resource is a usage error that
// names the likely intended command, instead of dumping help to stdout.
func TestDidYouMeanResource(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}), "get", "prblems")
	if r.Code != 2 || r.Stdout != "" {
		t.Errorf("code = %d, stdout = %q; want 2 and empty", r.Code, r.Stdout)
	}
	if !strings.Contains(r.Stderr, `unknown resource type "prblems"`) || !strings.Contains(r.Stderr, "dtmgd get problems") {
		t.Errorf("stderr = %q", r.Stderr)
	}
}

func TestDidYouMeanFlag(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}), "get", "problems", "--statsu", "OPEN")
	if r.Code != 2 || !strings.Contains(r.Stderr, "did you mean --status?") {
		t.Errorf("code = %d, stderr = %q", r.Code, r.Stderr)
	}
}

// TestParseFailureEnvelope pins agent-mode reporting when the command line
// cannot be parsed: one envelope with a usage code, exit 2, and --no-agent
// still honoured although flag parsing stopped before reaching it.
func TestParseFailureEnvelope(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)})
	for _, tt := range []struct {
		args []string
		code string
	}{
		{[]string{"get", "problems", "--bogus", "-A"}, "validation_error"},
		{[]string{"get", "prblems", "-A"}, "unknown_command"},
	} {
		r := runCLI(t, cfg, tt.args...)
		var resp agentmode.Response
		if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
			t.Errorf("%v: stdout is not one envelope: %v\n%s", tt.args, err, r.Stdout)
			continue
		}
		if resp.Error == nil || resp.Error.Code != tt.code || r.Code != 2 {
			t.Errorf("%v: error = %+v, code = %d; want %s and 2", tt.args, resp.Error, r.Code, tt.code)
		}
	}

	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "get", "problems", "--bogus", "--no-agent")
	if r.Stdout != "" || !strings.HasPrefix(r.Stderr, "Error: ") || r.Code != 2 {
		t.Errorf("--no-agent: stdout = %q, stderr = %q, code = %d", r.Stdout, r.Stderr, r.Code)
	}
}
