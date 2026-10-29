package cmd

import (
	"errors"
	"strings"
	"testing"
)

const securityProblemsPath = "GET /e/env1/api/v2/securityProblems"

// TestEnumFlagsAreCaseInsensitive pins that enum flags accept any case and
// send the API's upper-case value.
func TestEnumFlagsAreCaseInsensitive(t *testing.T) {
	api := newFakeAPI(t, map[string]route{
		problemsPath:         {Body: `{"totalCount":0,"problems":[]}`},
		securityProblemsPath: {Body: `{"totalCount":0,"securityProblems":[]}`},
	})
	cfg := withFakeEnvs(t, fakeEnv{"a", api})

	if r := runCLI(t, cfg, "get", "problems", "--status", "open", "--impact", "services", "-o", "json"); r.Code != 0 {
		t.Fatalf("get problems: %s", r)
	}
	if got := api.Queries(problemsPath)[0].Get("problemSelector"); got != `status("OPEN"),impactLevel("SERVICES")` {
		t.Errorf("problemSelector = %q", got)
	}

	if r := runCLI(t, cfg, "get", "security-problems", "--risk", "critical", "--status", "resolved", "-o", "json"); r.Code != 0 {
		t.Fatalf("get security-problems: %s", r)
	}
	if got := api.Queries(securityProblemsPath)[0].Get("securityProblemSelector"); got != `status("RESOLVED"),riskLevel("CRITICAL")` {
		t.Errorf("securityProblemSelector = %q", got)
	}
}

// TestInvalidEnumIsUsageError pins that a value the API does not accept is a
// usage error naming the allowed values, and that nothing is sent.
func TestInvalidEnumIsUsageError(t *testing.T) {
	api := newFakeAPI(t, map[string]route{
		problemsPath:         {Body: `{"totalCount":0,"problems":[]}`},
		securityProblemsPath: {Body: `{"totalCount":0,"securityProblems":[]}`},
	})
	cfg := withFakeEnvs(t, fakeEnv{"a", api})

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"get", "problems", "--impact", "SERVICE"}, `invalid --impact "SERVICE" (want one of: APPLICATION, ENVIRONMENT, INFRASTRUCTURE, SERVICES)`},
		{[]string{"get", "problems", "--status", "RESOLVED"}, `invalid --status "RESOLVED" (want one of: OPEN, CLOSED)`},
		{[]string{"get", "security-problems", "--status", "MUTED"}, `invalid --status "MUTED" (want one of: OPEN, RESOLVED)`},
		{[]string{"get", "security-problems", "--risk", "SEVERE"}, `invalid --risk "SEVERE" (want one of: CRITICAL, HIGH, MEDIUM, LOW, NONE)`},
	}
	for _, tc := range cases {
		r := runCLI(t, cfg, tc.args...)
		if r.Code != 2 || !strings.Contains(r.Stderr, tc.want) {
			t.Errorf("%v: want exit 2 and %q, got %s", tc.args, tc.want, r)
		}
	}
	if n := api.Hits(problemsPath) + api.Hits(securityProblemsPath); n != 0 {
		t.Errorf("an invalid flag still reached the API %d times", n)
	}
}

// TestNegativeLimitIsUsageError pins that --limit -1 is rejected rather than
// bypassing the agent-mode default.
func TestNegativeLimitIsUsageError(t *testing.T) {
	api := newFakeAPI(t, map[string]route{problemsPath: {Body: `{"totalCount":0,"problems":[]}`}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "--limit", "-1", "-A")
	if r.Code != 2 || !strings.Contains(r.Stdout+r.Stderr, "--limit must be 0 (no limit) or positive") {
		t.Errorf("want exit 2 and the --limit message, got %s", r)
	}
	if n := api.Hits(problemsPath); n != 0 {
		t.Errorf("a negative --limit still reached the API %d times", n)
	}
}

// TestEnumFlag pins enumFlag itself: empty passes through, case is ignored,
// and the canonical spelling comes back upper-cased or as allowed spells it.
func TestEnumFlag(t *testing.T) {
	cases := []struct {
		value   string
		allowed []string
		upper   bool
		want    string
	}{
		{"", []string{"OPEN"}, true, ""},
		{"open", []string{"OPEN", "CLOSED"}, true, "OPEN"},
		{"Closed", []string{"OPEN", "CLOSED"}, true, "CLOSED"},
		{"NAME", []string{"name", "-name"}, false, "name"},
		{"ALL", []string{"true", "false", "all"}, false, "all"},
	}
	for _, tc := range cases {
		got, err := enumFlag("x", tc.value, tc.allowed, tc.upper)
		if err != nil || got != tc.want {
			t.Errorf("enumFlag(%q) = %q, %v; want %q", tc.value, got, err, tc.want)
		}
	}
	_, err := enumFlag("sort", "date", []string{"name", "-name"}, false)
	var ue *UsageError
	if err == nil || !errors.As(err, &ue) || err.Error() != `invalid --sort "date" (want one of: name, -name)` {
		t.Errorf("enumFlag miss = %v", err)
	}
}

// TestListCommandsRejectStrayArguments pins that list and query commands,
// which take no positional arguments, reject one with a usage error instead
// of dropping it. The common case is a bool flag's value written with a
// space: "--show-global false" left --show-global true and sent it.
func TestListCommandsRejectStrayArguments(t *testing.T) {
	api := newFakeAPI(t, nil)
	cfg := withFakeEnvs(t, fakeEnv{"a", api})
	for _, args := range [][]string{
		{"get", "slos", "--show-global", "false"},
		{"get", "slos", "--demo", "false"},
		{"get", "problems", "extra"},
		{"get", "events", "--from", "now-1h", "extra"},
		{"get", "entities", "--selector", "type(HOST)", "extra"},
		{"get", "entity-types", "extra"},
		{"get", "metrics", "extra"},
		{"get", "security-problems", "extra"},
		{"get", "environments", "extra"},
		{"query", "logs", "--query", "e", "--from", "now-1h", "extra"},
		{"query", "log-counts", "--entity", "type(SERVICE)", "extra"},
		{"query", "metrics", "--metric", "m", "--from", "now-1h", "extra"},
	} {
		r := runCLI(t, cfg, args...)
		if r.Code != 2 || !strings.Contains(r.Stderr, "takes no arguments") {
			t.Errorf("%v: want exit 2 and \"takes no arguments\", got %s", args, r)
		}
	}
	if len(api.hits) != 0 {
		t.Errorf("stray arguments still reached the API: %v", api.hits)
	}
}
