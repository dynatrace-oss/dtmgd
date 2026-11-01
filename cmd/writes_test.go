package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	schemaPathS  = "GET /e/env1/api/v2/settings/schemas/builtin:test.multi"
	singletonDef = "GET /e/env1/api/v2/settings/schemas/" + schemaOneAgentUpdates
	postObjects  = "POST /e/env1/api/v2/settings/objects"
)

// valueFile writes a bare settings value to a temp file and returns its path.
func valueFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "value.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// createRoutes is a fake environment where builtin:test.multi is a
// multi-object schema and a POST creates object new-1.
func createRoutes() map[string]route {
	return map[string]route{
		schemaPathS:        {Body: `{"schemaId":"builtin:test.multi","multiObject":true,"maxObjects":100}`},
		postObjects:        {Body: `[{"code":200,"objectId":"new-1"}]`},
		clusterVersionPath: {Body: nodeList("1.300.0.20250101-000000")},
	}
}

// createArgs is a create settings command line for builtin:test.multi.
func createArgs(t *testing.T, extra ...string) []string {
	t.Helper()
	return append([]string{"create", "settings", "-f", valueFile(t, "name: dtmgd-test\n"), "--schema", "builtin:test.multi", "--scope", "environment"}, extra...)
}

// envelope is the agent envelope as write tests read it.
type envelope struct {
	OK     bool        `json:"ok"`
	Result writeReport `json:"result"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func decodeEnvelope(t *testing.T, stdout string) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal([]byte(stdout), &e); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	return e
}

func decodeReport(t *testing.T, stdout string) writeReport {
	t.Helper()
	var r writeReport
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	return r
}

func TestReadonlyBlocksWritesBeforeAnyRequest(t *testing.T) {
	api := newFakeAPI(t, createRoutes())
	cfg := withFakeEnvs(t, fakeEnv{"a", api})
	r := runCLI(t, cfg, createArgs(t)...)
	if r.Code != 6 || !strings.Contains(r.Stderr, `"a"`) || !strings.Contains(r.Stderr, "readonly") || !strings.Contains(r.Stderr, "set-context a --safety-level readwrite-all") {
		t.Errorf("want exit 6 naming the context, its level and the fix: %s", r)
	}
	if api.totalHits() != 0 {
		t.Errorf("%d requests, want none", api.totalHits())
	}
	agent := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, createArgs(t)...)
	if e := decodeEnvelope(t, agent.Stdout); agent.Code != 6 || e.Error == nil || e.Error.Code != "safety_blocked" {
		t.Errorf("agent: %s", agent)
	}
}

// TestUnknownSafetyLevelRefusesWrites pins that a typo in safety-level
// refuses writes, naming the valid levels, and leaves reads alone.
func TestUnknownSafetyLevelRefusesWrites(t *testing.T) {
	routes := createRoutes()
	routes[objectsPath] = route{File: "settings_objects_alerting.json"}
	api := newFakeAPI(t, routes)
	cfg := withFakeEnvLevels(t, map[string]string{"a": "read-write"}, fakeEnv{"a", api})
	r := runCLI(t, cfg, createArgs(t)...)
	if r.Code != 2 || !strings.Contains(r.Stderr, "dangerously-unrestricted") || api.totalHits() != 0 {
		t.Errorf("%s (%d requests)", r, api.totalHits())
	}
	if read := runCLI(t, cfg, "get", "settings", "--schema", "builtin:alerting.profile"); read.Code != 0 {
		t.Errorf("a read: %s", read)
	}
}

func TestReadwriteMineMayCreate(t *testing.T) {
	api := newFakeAPI(t, createRoutes())
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-mine"}, fakeEnv{"a", api})
	r := runCLI(t, cfg, append(createArgs(t), "-o", "json")...)
	rep := decodeReport(t, r.Stdout)
	if r.Code != 0 || api.Hits(postObjects) != 1 || len(rep.Results) != 1 || rep.Results[0].Action != "created" || rep.Results[0].ObjectID != "new-1" {
		t.Errorf("%s", r)
	}
}

func TestCreateRefusesAnExistingSingleton(t *testing.T) {
	routes := createRoutes()
	routes[singletonDef] = route{Body: `{"schemaId":"builtin:deployment.oneagent.updates","multiObject":false}`}
	routes[oneAgentRoute] = route{File: "settings_oneagent_updates.json"}
	api := newFakeAPI(t, routes)
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-all"}, fakeEnv{"a", api})
	r := runCLI(t, cfg, "create", "settings", "-f", valueFile(t, "updateMode: MANUAL\n"), "--schema", schemaOneAgentUpdates, "--scope", "environment")
	if r.Code != 1 || !strings.Contains(r.Stderr, "apply") || api.Hits(postObjects) != 0 {
		t.Errorf("%s", r)
	}
}

func TestDryRunSendsOnlyGets(t *testing.T) {
	api := newFakeAPI(t, createRoutes())
	cfg := withFakeEnvs(t, fakeEnv{"a", api})
	r := runCLI(t, cfg, append(createArgs(t, "--dry-run"), "-o", "json")...)
	rep := decodeReport(t, r.Stdout)
	if r.Code != 0 || !rep.DryRun || len(rep.Results) != 1 || rep.Results[0].Action != "create" || rep.Results[0].Value["name"] != "dtmgd-test" {
		t.Errorf("%s", r)
	}
	if api.Hits(postObjects) != 0 {
		t.Error("a dry run sent a POST")
	}
}

func TestValidateOnlyGate(t *testing.T) {
	levels := map[string]string{"a": "readwrite-all"}
	old := createRoutes()
	old[clusterVersionPath] = route{Body: nodeList("1.287.0.20240101-000000")}
	api := newFakeAPI(t, old)
	if r := runCLI(t, withFakeEnvLevels(t, levels, fakeEnv{"a", api}), createArgs(t, "--validate-only")...); r.Code != 2 || !strings.Contains(r.Stderr, "1.288") || api.Hits(postObjects) != 0 {
		t.Errorf("1.287: %s", r)
	}

	unreadable := createRoutes()
	delete(unreadable, clusterVersionPath)
	unreadable["GET /e/env1/api/v1/config/clusterversion"] = route{Status: 403, File: "error_403.json"}
	api = newFakeAPI(t, unreadable)
	if r := runCLI(t, withFakeEnvLevels(t, levels, fakeEnv{"a", api}), createArgs(t, "--validate-only")...); r.Code != 2 || !strings.Contains(r.Stderr, "DataExport") || !strings.Contains(r.Stderr, "--dry-run") {
		t.Errorf("unreadable version: %s", r)
	}

	api = newFakeAPI(t, createRoutes())
	r := runCLI(t, withFakeEnvLevels(t, levels, fakeEnv{"a", api}), append(createArgs(t, "--validate-only"), "-o", "json")...)
	qs := api.Queries(postObjects)
	if rep := decodeReport(t, r.Stdout); r.Code != 0 || len(qs) != 1 || qs[0].Get("validateOnly") != "true" || rep.Results[0].Action != "validated" || !rep.ValidateOnly {
		t.Errorf("1.300: %s", r)
	}
}

// TestValidateOnlyIsAllowedAtReadonly pins that a validation, which saves
// nothing, counts as a read for the write guard.
func TestValidateOnlyIsAllowedAtReadonly(t *testing.T) {
	api := newFakeAPI(t, createRoutes())
	if r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), createArgs(t, "--validate-only")...); r.Code != 0 {
		t.Errorf("%s", r)
	}
}

func TestMultiEnvChecksEveryContextFirst(t *testing.T) {
	a, b := newFakeAPI(t, createRoutes()), newFakeAPI(t, createRoutes())
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-all"}, fakeEnv{"a", a}, fakeEnv{"b", b})
	if r := runCLI(t, cfg, createArgs(t, "--env", "a;b")...); r.Code != 6 || a.Hits(postObjects)+b.Hits(postObjects) != 0 {
		t.Errorf("one readonly context: %s", r)
	}
	cfg = withFakeEnvLevels(t, map[string]string{"a": "readwrite-all", "b": "readwrite-all"}, fakeEnv{"a", a}, fakeEnv{"b", b})
	r := runCLI(t, cfg, append(createArgs(t, "--env", "a;b"), "-o", "json")...)
	rep := decodeReport(t, r.Stdout)
	if r.Code != 0 || a.Hits(postObjects) != 1 || b.Hits(postObjects) != 1 || len(rep.Results) != 2 || rep.Results[0].Context != "a" || rep.Results[1].Context != "b" {
		t.Errorf("both writable: %s", r)
	}
}

func TestWriteRefusesClusterContext(t *testing.T) {
	api := newFakeAPI(t, createRoutes())
	cfg := withFakeContexts(t, "c", nil, []fakeEnv{{"c", api}})
	if r := runCLI(t, cfg, createArgs(t)...); r.Code != 2 || api.totalHits() != 0 {
		t.Errorf("%s", r)
	}
}

func TestWriteFailureReport(t *testing.T) {
	routes := createRoutes()
	routes[postObjects] = route{Status: 400, Body: `[{"code":400,"error":{"code":400,"message":"Validation failed for 1 Validators.","constraintViolations":[{"path":"name","message":"must not be blank","parameterLocation":"PAYLOAD_BODY"}]}}]`}
	api := newFakeAPI(t, routes)
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-all"}, fakeEnv{"a", api})
	r := runCLI(t, cfg, append(createArgs(t), "-o", "json")...)
	rep := decodeReport(t, r.Stdout)
	if r.Code != 1 || rep.Results[0].Action != "failed" || rep.Results[0].Error == nil || len(rep.Results[0].Error.ConstraintViolations) != 1 || rep.Results[0].Error.ConstraintViolations[0].Path != "name" {
		t.Errorf("%s", r)
	}
	agent := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, createArgs(t)...)
	if e := decodeEnvelope(t, agent.Stdout); e.OK || len(e.Result.Results) != 1 {
		t.Errorf("agent: %s", agent)
	}
}

// TestVerboseDumpsNoRequestBody pins that -vv shows no write body, which can
// hold secret values.
func TestVerboseDumpsNoRequestBody(t *testing.T) {
	api := newFakeAPI(t, createRoutes())
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-all"}, fakeEnv{"a", api})
	args := []string{"create", "settings", "-f", valueFile(t, "password: s3cret-value\n"), "--schema", "builtin:test.multi", "--scope", "environment", "-vv"}
	if r := runCLI(t, cfg, args...); r.Code != 0 || strings.Contains(r.Stderr, "s3cret-value") {
		t.Errorf("%s", r)
	}
}

func TestCatalogMarksWriteCommands(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}), "commands", "-o", "json")
	var cat struct {
		Commands []struct {
			Path     string `json:"path"`
			Mutating bool   `json:"mutating"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &cat); err != nil {
		t.Fatalf("%v\n%s", err, r)
	}
	marks := map[string]bool{}
	for _, c := range cat.Commands {
		marks[c.Path] = c.Mutating
	}
	if !marks["create settings"] || marks["get settings"] {
		t.Errorf("marks = %v", marks)
	}
}
