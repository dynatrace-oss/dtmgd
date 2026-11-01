package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// tempFile writes content to a temp file and returns its path.
func tempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// keyedRoutes is a fake where builtin:test.keyed is a multi-object schema
// with key property "key", holding object obj-k with key SENSOR_X.
func keyedRoutes() map[string]route {
	routes := createRoutes()
	routes["GET /e/env1/api/v2/settings/schemas/builtin:test.keyed"] = route{Body: `{"multiObject":true,"keyProperty":"key"}`}
	routes[objectsPath+" schemaIds=builtin:test.keyed"] = route{Body: `{"items":[{"objectId":"obj-k","value":{"key":"SENSOR_X","enabled":false}}],"totalCount":1}`}
	return routes
}

// TestCreateRefusesAKeyPropertyMatch pins that create settings looks before
// a POST the server would turn into an overwrite: a key-property match is
// refused, at every level and in a dry run.
func TestCreateRefusesAKeyPropertyMatch(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-mine", keyedRoutes())
	args := []string{"create", "settings", "-f", tempFile(t, "key: SENSOR_X\nenabled: true\n"), "--schema", "builtin:test.keyed", "--scope", "environment"}
	if r := runCLI(t, cfg, args...); r.Code != 1 || !strings.Contains(r.Stderr, "apply") || api.Hits(postObjects) != 0 {
		t.Errorf("%s", r)
	}
	if r := runCLI(t, cfg, append(args, "--dry-run")...); r.Code != 1 {
		t.Errorf("dry run: %s", r)
	}
}

// TestWriteRefusesDuplicateContexts pins that a context named twice, or two
// contexts for the same environment, never receive the same writes twice.
func TestWriteRefusesDuplicateContexts(t *testing.T) {
	api := newFakeAPI(t, createRoutes())
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-all", "b": "readwrite-all"}, fakeEnv{"a", api}, fakeEnv{"b", api})
	for _, env := range []string{"a;a", "a; a", "ALL_ENVIRONMENTS"} {
		if r := runCLI(t, cfg, createArgs(t, "--env", env)...); r.Code != 2 {
			t.Errorf("--env %q: %s", env, r)
		}
	}
	if api.Hits(postObjects) != 0 {
		t.Errorf("%d POSTs, want none", api.Hits(postObjects))
	}
}

// TestApplyRefusesAMovedObject pins that an objectId only updates the object
// the file says it is: same schema, same scope ("tenant" and "environment"
// alike).
func TestApplyRefusesAMovedObject(t *testing.T) {
	routes := map[string]route{
		objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default"),
		putPrefix + "obj-alerting-1": {Body: `{"code":200}`},
	}
	api, cfg := applyFake(t, "readwrite-all", routes)
	for _, body := range []string{
		"objectId: obj-alerting-1\nschemaId: builtin:other\nscope: environment\nvalue: {name: X}\n",
		"objectId: obj-alerting-1\nschemaId: builtin:alerting.profile\nscope: HOST_GROUP-0000000000000001\nvalue: {name: X}\n",
	} {
		if r := runCLI(t, cfg, "apply", "-f", tempFile(t, body)); r.Code != 2 || !strings.Contains(r.Stderr, "objectId") {
			t.Errorf("%q: %s", body, r)
		}
	}
	if api.Hits(putPrefix+"obj-alerting-1") != 0 {
		t.Error("a moved object was written")
	}
	tenant := "objectId: obj-alerting-1\nschemaId: builtin:alerting.profile\nscope: tenant\nvalue: {name: X}\n"
	if r := runCLI(t, cfg, "apply", "-f", tempFile(t, tenant)); r.Code != 0 {
		t.Errorf("scope tenant for environment: %s", r)
	}
}

func TestApplyReadsEveryYAMLDocument(t *testing.T) {
	routes := createRoutes()
	routes[objectsPath+" schemaIds=builtin:test.multi"] = route{Body: `{"items":[],"totalCount":0}`}
	api, cfg := applyFake(t, "readwrite-all", routes)
	docs := "schemaId: builtin:test.multi\nscope: environment\nvalue: {name: one}\n---\nschemaId: builtin:test.multi\nscope: environment\nvalue: {name: two}\n"
	if r, rep := applyJSON(t, cfg, tempFile(t, docs)); r.Code != 0 || len(rep.Results) != 2 || api.Hits(postObjects) != 2 {
		t.Errorf("%s", r)
	}
	if r := runCLI(t, cfg, "create", "settings", "-f", tempFile(t, "name: one\n---\nname: two\n"), "--schema", "builtin:test.multi", "--scope", "environment"); r.Code != 2 {
		t.Errorf("create with two documents: %s", r)
	}
}

// TestApplyUnwrapsAnAgentEnvelope pins that an object exported in agent mode
// (the envelope) applies back.
func TestApplyUnwrapsAnAgentEnvelope(t *testing.T) {
	_, cfg := applyFake(t, "readwrite-all", map[string]route{
		objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default"),
		putPrefix + "obj-alerting-1": {Body: `{"code":200}`},
	})
	env := `{"ok":true,"result":{"objectId":"obj-alerting-1","schemaId":"builtin:alerting.profile","scope":"environment","value":{"name":"New"}},"context":{"verb":"describe"}}`
	if r, rep := applyJSON(t, cfg, tempFile(t, env)); r.Code != 0 || actions(rep) != "updated" {
		t.Errorf("%s", r)
	}
}

// TestTransportFailureWarnsItMayBeSaved pins that a write lost in transport
// says the cluster may have saved it, so nobody blindly repeats it.
func TestTransportFailureWarnsItMayBeSaved(t *testing.T) {
	routes := createRoutes()
	routes[postObjects] = route{Drop: true}
	_, cfg := applyFake(t, "readwrite-all", routes)
	if r := runCLI(t, cfg, createArgs(t)...); r.Code == 0 || !strings.Contains(r.Stderr, "may have saved") {
		t.Errorf("%s", r)
	}
}

// TestWriteFailureNamesTheObject pins the error of one failed object: the
// server's message and the HTTP class, not an environments count.
func TestWriteFailureNamesTheObject(t *testing.T) {
	routes := createRoutes()
	routes[postObjects] = route{Status: 400, Body: `[{"code":400,"error":{"code":400,"message":"must not be blank"}}]`}
	_, cfg := applyFake(t, "readwrite-all", routes)
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, createArgs(t)...)
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &e); err != nil {
		t.Fatalf("%v\n%s", err, r)
	}
	if e.Error.Code != "bad_request" || !strings.Contains(e.Error.Message, "must not be blank") || strings.Contains(e.Error.Message, "environments") {
		t.Errorf("error = %+v", e.Error)
	}
}

// TestApplyPrefersExternalID pins the server's order: an externalId match
// wins over a key-property match.
func TestApplyPrefersExternalID(t *testing.T) {
	routes := keyedRoutes()
	routes[objectsPath+" schemaIds=builtin:test.keyed"] = route{Body: `{"items":[{"objectId":"obj-k","value":{"key":"SENSOR_X"}},{"objectId":"obj-e","externalId":"ext-1","value":{"key":"OTHER"}}],"totalCount":2}`}
	routes[putPrefix+"obj-e"] = route{Body: `{"code":200}`}
	api, cfg := applyFake(t, "readwrite-all", routes)
	file := tempFile(t, "externalId: ext-1\nschemaId: builtin:test.keyed\nscope: environment\nvalue: {key: SENSOR_X}\n")
	if r := runCLI(t, cfg, "apply", "-f", file); r.Code != 0 || api.Hits(putPrefix+"obj-e") != 1 || api.Hits(putPrefix+"obj-k") != 0 {
		t.Errorf("%s", r)
	}
}

// TestApplyRefusesDuplicateTargets pins that two objects of one file never
// write the same object.
func TestApplyRefusesDuplicateTargets(t *testing.T) {
	_, cfg := applyFake(t, "readwrite-all", map[string]route{
		singletonDef:                 {Body: `{"multiObject":false}`},
		oneAgentRoute:                {Body: `{"items":[],"totalCount":0}`},
		objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default"),
	})
	for _, body := range []string{
		"- {schemaId: " + schemaOneAgentUpdates + ", scope: environment, value: {updateMode: MANUAL}}\n- {schemaId: " + schemaOneAgentUpdates + ", scope: environment, value: {updateMode: AUTOMATIC}}\n",
		"- {objectId: obj-alerting-1, schemaId: builtin:alerting.profile, scope: environment, value: {name: A}}\n- {objectId: obj-alerting-1, schemaId: builtin:alerting.profile, scope: environment, value: {name: B}}\n",
	} {
		if r := runCLI(t, cfg, "apply", "-f", tempFile(t, body)); r.Code != 2 {
			t.Errorf("%q: %s", body, r)
		}
	}
}

// TestEditChecksTheLevelBeforeTheEditor pins that a context that cannot
// update never opens the editor, and that an edit that is not YAML is kept.
func TestEditChecksTheLevelBeforeTheEditor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake editor is a shell script")
	}
	marker := filepath.Join(t.TempDir(), "opened")
	script := filepath.Join(t.TempDir(), "editor.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\nprintf '{not: [valid' > \"$1\"\n"), 0o700)
	t.Setenv("EDITOR", script)
	t.Setenv("VISUAL", "")
	_, mine := applyFake(t, "readwrite-mine", map[string]route{objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default")})
	if r := runInteractive(t, mine, "", "edit", "setting", "obj-alerting-1"); r.Code != 6 {
		t.Errorf("readwrite-mine: %s", r)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the editor opened for a context that cannot update")
	}
	_, all := applyFake(t, "readwrite-all", map[string]route{objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default")})
	r := runInteractive(t, all, "", "edit", "setting", "obj-alerting-1")
	if r.Code == 0 || !strings.Contains(r.Stderr, "kept in") {
		t.Errorf("invalid YAML: %s", r)
	}
}

func TestSetContextWarnsWhenRepointingAWritableContext(t *testing.T) {
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-all"}, fakeEnv{"a", newFakeAPI(t, nil)})
	r := runCLI(t, cfg, "config", "set-context", "a", "--host", "https://other.example.com")
	if r.Code != 0 || !strings.Contains(r.Stderr, "readwrite-all") {
		t.Errorf("%s", r)
	}
}

// TestValidateOnlyRefusesDeletes pins that a validation can never carry a
// delete, which the API cannot validate and would carry out.
func TestValidateOnlyRefusesDeletes(t *testing.T) {
	s := writeSpec{ValidateOnly: true}
	p := contextPlan{writes: []plannedWrite{{Op: config.OpDelete, Action: actionDelete}}, version: "1.300.0"}
	if err := s.checkPlan("a", p, config.SafetyReadWriteAll); err == nil {
		t.Error("a validate-only delete passed the checks")
	}
}

func TestDryRunTableSaysNothingChanged(t *testing.T) {
	_, cfg := applyFake(t, "", createRoutes())
	if r := runCLI(t, cfg, createArgs(t, "--dry-run")...); r.Code != 0 || !strings.Contains(r.Stderr, "nothing was changed") {
		t.Errorf("%s", r)
	}
}

func TestApplyReadsTheAppStdin(t *testing.T) {
	routes := createRoutes()
	routes[objectsPath+" schemaIds=builtin:test.multi"] = route{Body: `{"items":[],"totalCount":0}`}
	api, cfg := applyFake(t, "readwrite-all", routes)
	r := runInteractive(t, cfg, "schemaId: builtin:test.multi\nscope: environment\nvalue: {name: X}\n", "apply", "-f", "-")
	if r.Code != 0 || api.Hits(postObjects) != 1 {
		t.Errorf("%s", r)
	}
}

func TestApplyCreatesNothingAfter400(t *testing.T) {
	routes := createRoutes()
	routes[objPrefix+"bad-id"] = route{Status: 400, Body: `{"error":{"code":400,"message":"Your supplied key is not valid"}}`}
	api, cfg := applyFake(t, "readwrite-all", routes)
	file := tempFile(t, "objectId: bad-id\nschemaId: builtin:test.multi\nscope: environment\nvalue: {name: X}\n")
	if r := runCLI(t, cfg, "apply", "-f", file); r.Code == 0 || api.Hits(postObjects) != 0 {
		t.Errorf("%s", r)
	}
}
