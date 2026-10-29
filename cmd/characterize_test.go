package cmd

import (
	"strings"
	"testing"
)

// Characterization tests pin what the CLI does today, bugs included, so the
// refactors that follow can prove they changed nothing they did not mean to.
// When a later change fixes one of the pinned bugs, the test is renamed after
// the new behaviour and its golden is regenerated.

const (
	problemsPath = "GET /e/env1/api/v2/problems"
	entitiesPath = "GET /e/env1/api/v2/entities"
	logsPath     = "GET /e/env1/api/v2/logs/search"
)

func pagedProblems() map[string]route {
	return map[string]route{
		problemsPath:                        {File: "problems_page1.json"},
		problemsPath + " nextPageKey=PAGE2": {File: "problems_page2.json"},
	}
}

func TestCharGetProblemsTable(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems")
	assertGolden(t, "char_get_problems_table", r.String())
	if got := api.Hits(problemsPath); got != 2 {
		t.Errorf("page requests = %d, want 2", got)
	}
}

// -o json prints the API's own response, merged across pages (it used to
// print dtmgd's typed subset of it).
func TestGetProblemsJSONIsAPIShaped(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-o", "json")
	assertGolden(t, "get_problems_json", r.String())
}

// The agent envelope carries the API-shaped result and a context with verb
// and total.
func TestGetProblemsAgentEnvelope(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-A")
	assertGolden(t, "get_problems_agent", r.String())
}

// An empty result with -o json prints the object with an empty list (it used
// to print nothing, which broke | jq).
func TestGetProblemsEmptyJSONPrintsEmptyList(t *testing.T) {
	api := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_empty.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-o", "json")
	assertGolden(t, "get_problems_empty_json", r.String())
	if !strings.Contains(r.Stdout, `"problems": []`) || r.Code != 0 {
		t.Errorf("stdout=%q code=%d", r.Stdout, r.Code)
	}
}

// describe in table mode prints the object as YAML (it used to print Go's
// map[...] syntax).
func TestDescribeEntityTableIsYAML(t *testing.T) {
	api := newFakeAPI(t, map[string]route{entitiesPath + "/HOST-1": {File: "entity.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "describe", "entity", "HOST-1")
	assertGolden(t, "describe_entity_table_yaml", r.String())
	if strings.Contains(r.Stdout, "map[") || !strings.Contains(r.Stdout, "entityId: HOST-1") {
		t.Errorf("want YAML, got:\n%s", r.Stdout)
	}
}

func TestCharMultiEnvJSON(t *testing.T) {
	a := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	b := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "problems", "--env", "a;b", "-o", "json")
	assertGolden(t, "char_multi_env_json", r.String())
}

// Every environment failing with 403 exits 5 (it used to exit 0).
func TestMultiEnvAllFailExitsNonZero(t *testing.T) {
	deny := map[string]route{problemsPath: {Status: 403, File: "error_403.json"}}
	a, b := newFakeAPI(t, deny), newFakeAPI(t, deny)
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "problems", "--env", "a;b", "-o", "json")
	assertGolden(t, "multi_env_all_fail", r.String())
	if r.Code != 5 {
		t.Errorf("code = %d, want 5", r.Code)
	}
}

// query logs prints only the records on stdout; the "Found N" header is on
// stderr (stdout used to start with a blank line left over from it).
func TestQueryLogsTable(t *testing.T) {
	api := newFakeAPI(t, map[string]route{logsPath: {File: "logs.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "query", "logs", "--query", "error", "--from", "now-1h")
	assertGolden(t, "query_logs_table", r.String())
	if strings.HasPrefix(r.Stdout, "\n") || !strings.Contains(r.Stderr, "Found 2 log records") {
		t.Errorf("stdout %q, stderr %q", r.Stdout, r.Stderr)
	}
}

// A mistyped resource is a usage error (exit 2) with a suggestion; it used to
// dump help to stdout and exit 1.
func TestUnknownResourceIsUsageError(t *testing.T) {
	api := newFakeAPI(t, nil)
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "prblems")
	assertGolden(t, "unknown_resource_usage_error", r.String())
	if r.Code != 2 {
		t.Errorf("code = %d, want 2", r.Code)
	}
}
