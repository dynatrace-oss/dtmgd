package cmd

import (
	"strings"
	"testing"
)

// problemsCfg is a one-environment config whose problems are OPEN, and
// whose security problems answer 403.
func problemsCfg(t *testing.T) string {
	t.Helper()
	api := newFakeAPI(t, map[string]route{
		problemsPath:                          {File: "problems_small.json"},
		"GET /e/env1/api/v2/securityProblems": {Status: 403, File: "error_403.json"},
	})
	return withFakeEnvs(t, fakeEnv{"a", api})
}

// TestForceColorColorsTables pins architecture guideline 5 for a table:
// bold headers and a red OPEN status (FORCE_COLOR stands in for a terminal).
func TestForceColorColorsTables(t *testing.T) {
	r := runCLIWithEnv(t, map[string]string{"FORCE_COLOR": "1"}, problemsCfg(t), "get", "problems")
	if r.Code != 0 || !strings.Contains(r.Stdout, "\x1b[1mPROBLEM-ID\x1b[0m") || !strings.Contains(r.Stdout, "\x1b[31mOPEN\x1b[0m") {
		t.Errorf("table not colored: %q", r.Stdout)
	}
}

func TestForceColorColorsTheErrorGlyph(t *testing.T) {
	r := runCLIWithEnv(t, map[string]string{"FORCE_COLOR": "1"}, problemsCfg(t), "get", "security-problems")
	if r.Code != 5 || !strings.Contains(r.Stderr, "\x1b[1;31m✗\x1b[0m ") {
		t.Errorf("error glyph not colored: %q", r.Stderr)
	}
}

// TestNoColorWhereItWouldCorruptData pins that data formats, the agent
// envelope and NO_COLOR never get escape codes, even with FORCE_COLOR.
func TestNoColorWhereItWouldCorruptData(t *testing.T) {
	for name, c := range map[string]struct {
		env  map[string]string
		args []string
	}{
		"csv":      {map[string]string{"FORCE_COLOR": "1"}, []string{"-o", "csv"}},
		"json":     {map[string]string{"FORCE_COLOR": "1"}, []string{"-o", "json"}},
		"agent":    {map[string]string{"FORCE_COLOR": "1", "CLAUDECODE": "1"}, nil},
		"no-color": {map[string]string{"FORCE_COLOR": "1", "NO_COLOR": "1"}, nil},
		"plain":    {map[string]string{"FORCE_COLOR": "1"}, []string{"--plain"}},
		"pipe":     {nil, nil},
	} {
		r := runCLIWithEnv(t, c.env, problemsCfg(t), append([]string{"get", "problems"}, c.args...)...)
		if strings.Contains(r.Stdout+r.Stderr, "\x1b") {
			t.Errorf("%s: escape codes in output:\n%q\n%q", name, r.Stdout, r.Stderr)
		}
	}
}

// TestStderrColorNeedsATerminal pins Review Focus 3: with stdout on a
// terminal but stderr redirected to a file, stderr gets no escape codes,
// unless FORCE_COLOR asks for them.
func TestStderrColorNeedsATerminal(t *testing.T) {
	t.Setenv("FORCE_COLOR", "")
	if stderrWantsColor(false) {
		t.Error("a redirected stderr wants color")
	}
	if !stderrWantsColor(true) {
		t.Error("a terminal stderr does not want color")
	}
	t.Setenv("FORCE_COLOR", "1")
	if !stderrWantsColor(false) {
		t.Error("FORCE_COLOR does not color a redirected stderr")
	}
}
