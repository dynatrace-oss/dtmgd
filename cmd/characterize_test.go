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

func TestCharGetProblemsJSON(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-o", "json")
	assertGolden(t, "char_get_problems_json", r.String())
}

func TestCharGetProblemsAgent(t *testing.T) {
	api := newFakeAPI(t, pagedProblems())
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-A")
	assertGolden(t, "char_get_problems_agent", r.String())
}

// Today: an empty result with -o json writes nothing to stdout.
func TestCharGetProblemsEmptyJSON(t *testing.T) {
	api := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_empty.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "problems", "-o", "json")
	assertGolden(t, "char_get_problems_empty_json", r.String())
	if r.Stdout != "" || r.Code != 0 {
		t.Errorf("today's behaviour changed: stdout=%q code=%d", r.Stdout, r.Code)
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

// Today: every environment failing still exits 0.
func TestCharMultiEnvAllFail(t *testing.T) {
	deny := map[string]route{problemsPath: {Status: 403, File: "error_403.json"}}
	a, b := newFakeAPI(t, deny), newFakeAPI(t, deny)
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "problems", "--env", "a;b", "-o", "json")
	assertGolden(t, "char_multi_env_all_fail", r.String())
	if r.Code != 0 {
		t.Errorf("today's behaviour changed: code=%d", r.Code)
	}
}

func TestCharQueryLogsTable(t *testing.T) {
	api := newFakeAPI(t, map[string]route{logsPath: {File: "logs.json"}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "query", "logs", "--query", "error", "--from", "now-1h")
	assertGolden(t, "char_query_logs_table", r.String())
}

// Today: a mistyped resource prints help to stdout and exits 1.
func TestCharUnknownResource(t *testing.T) {
	api := newFakeAPI(t, nil)
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "prblems")
	assertGolden(t, "char_unknown_resource", r.String())
	if r.Code != 1 {
		t.Errorf("today's behaviour changed: code=%d", r.Code)
	}
}
