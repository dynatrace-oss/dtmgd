package cmd

import (
	"strings"
	"testing"
)

func TestGetProblemsCSV(t *testing.T) {
	api := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-o", "csv")
	if r.Code != 0 {
		t.Fatalf("exit %d\nstderr: %s", r.Code, r.Stderr)
	}
	// Every column, wide ones included; one line per problem.
	assertGolden(t, "get_problems_csv", r.Stdout)
}

func TestGetProblemsTSV(t *testing.T) {
	api := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-o", "tsv", "--columns", "PROBLEM-ID,STATUS")
	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	if r.Code != 0 || len(lines) != 3 || lines[0] != "PROBLEM-ID\tSTATUS" {
		t.Errorf("exit %d, stdout:\n%s", r.Code, r.Stdout)
	}
}

func TestEmptyCSVIsAHeaderLine(t *testing.T) {
	api := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_empty.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-o", "csv")
	want := "PROBLEM-ID,DISPLAY-ID,TITLE,STATUS,SEVERITY,IMPACT,START-TIME,ROOT-CAUSE\n"
	if r.Code != 0 || r.Stdout != want {
		t.Errorf("exit %d, stdout %q, want %q", r.Code, r.Stdout, want)
	}
}

func TestMultiEnvironmentCSVLeadsWithAContextColumn(t *testing.T) {
	a := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	b := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "problems", "--env", "a;b", "-o", "csv", "--columns", "PROBLEM-ID")
	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	if r.Code != 0 || len(lines) != 5 || lines[0] != "CONTEXT,PROBLEM-ID" ||
		!strings.HasPrefix(lines[1], "a,") || !strings.HasPrefix(lines[3], "b,") {
		t.Errorf("exit %d, stdout:\n%s\nstderr: %s", r.Code, r.Stdout, r.Stderr)
	}
	if strings.Contains(r.Stdout, "==") {
		t.Errorf("csv has a table section header:\n%s", r.Stdout)
	}
}

func TestQueryLogsCSV(t *testing.T) {
	api := newFakeAPI(t, map[string]route{logsPath: {File: "logs.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "query", "logs", "--query", "error", "--from", "now-1h", "-o", "csv")
	lines := strings.Split(strings.TrimRight(r.Stdout, "\n"), "\n")
	if r.Code != 0 || len(lines) != 3 || lines[0] != "TIMESTAMP,STATUS,CONTENT" {
		t.Errorf("exit %d, stdout:\n%s\nstderr: %s", r.Code, r.Stdout, r.Stderr)
	}
}

func TestCSVIsRefusedForSummaryCommands(t *testing.T) {
	for _, args := range [][]string{
		{"describe", "problem", "p1", "-o", "csv"},
		{"query", "metrics", "--metric", "builtin:host.cpu.usage", "--from", "now-1h", "-o", "tsv"},
	} {
		api := newFakeAPI(t, nil)
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), args...)
		if r.Code != 2 || api.totalHits() != 0 || !strings.Contains(r.Stderr, "-o json") {
			t.Errorf("%v: exit %d, %d requests, stderr:\n%s", args, r.Code, api.totalHits(), r.Stderr)
		}
	}
}

func TestAgentModeKeepsTheEnvelopeForCSV(t *testing.T) {
	// As with json and yaml: the envelope wins; --no-agent gives csv.
	api := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-o", "csv")
	if r.Code != 0 || !strings.HasPrefix(r.Stdout, "{") {
		t.Errorf("exit %d, stdout:\n%s", r.Code, r.Stdout)
	}
}
