package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	objPrefix        = "GET /e/env1/api/v2/settings/objects/"
	putPrefix        = "PUT /e/env1/api/v2/settings/objects/"
	updateWindowsDef = "GET /e/env1/api/v2/settings/schemas/" + schemaUpdateWindows
)

// alertingObject is the GET body of an alerting profile object, whose
// updateToken is "server-<id>".
func alertingObject(id, name string) route {
	return route{Body: fmt.Sprintf(`{"objectId":%q,"schemaId":"builtin:alerting.profile","scope":"environment","updateToken":"server-%s","value":{"name":%q}}`, id, id, name)}
}

// applyFake serves routes from one writable context and returns the fake
// and its config.
func applyFake(t *testing.T, level string, routes map[string]route) (*fakeAPI, string) {
	t.Helper()
	api := newFakeAPI(t, routes)
	return api, withFakeEnvLevels(t, map[string]string{"a": level}, fakeEnv{"a", api})
}

// applyJSON runs apply -f file -o json and returns the result and report.
func applyJSON(t *testing.T, cfg, file string, extra ...string) (cliResult, writeReport) {
	t.Helper()
	r := runCLI(t, cfg, append([]string{"apply", "-f", file, "-o", "json"}, extra...)...)
	var rep writeReport
	if r.Stdout != "" {
		if err := json.Unmarshal([]byte(r.Stdout), &rep); err != nil {
			t.Fatalf("%v\n%s", err, r)
		}
	}
	return r, rep
}

// actions lists a report's actions in order.
func actions(rep writeReport) string {
	var out []string
	for _, r := range rep.Results {
		out = append(out, r.Action)
	}
	return strings.Join(out, ",")
}

// putBody is the JSON body of the n-th PUT to id.
func putBody(t *testing.T, api *fakeAPI, id string) map[string]interface{} {
	t.Helper()
	bodies := api.Bodies(putPrefix + id)
	if len(bodies) == 0 {
		t.Fatalf("no PUT to %s", id)
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(bodies[0]), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestApplyCreatesANewObject(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", map[string]route{
		updateWindowsDef:   {Body: `{"multiObject":true}`},
		updateWindowsRoute: {File: "settings_update_windows.json"},
		postObjects:        {Body: `[{"code":200,"objectId":"new-1"}]`},
	})
	r, rep := applyJSON(t, cfg, "testdata/apply/window_new.yaml")
	if r.Code != 0 || actions(rep) != "created" || rep.Results[0].ObjectID != "new-1" || api.Hits(postObjects) != 1 {
		t.Errorf("%s", r)
	}
}

func TestApplyUpdatesByObjectID(t *testing.T) {
	routes := map[string]route{
		objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default"),
		objPrefix + "obj-alerting-2": alertingObject("obj-alerting-2", "Payments"),
		putPrefix + "obj-alerting-1": {Body: `{"code":200,"objectId":"obj-alerting-1"}`},
		putPrefix + "obj-alerting-2": {Body: `{"code":200,"objectId":"obj-alerting-2"}`},
	}
	api, cfg := applyFake(t, "readwrite-all", routes)
	r, rep := applyJSON(t, cfg, "testdata/apply/export_list.yaml")
	if r.Code != 0 || actions(rep) != "updated,updated" {
		t.Fatalf("%s", r)
	}
	if b := putBody(t, api, "obj-alerting-1"); b["updateToken"] != "tok-1" || b["value"].(map[string]interface{})["name"] != "Default renamed" {
		t.Errorf("PUT body %v, want the file's token and value", b)
	}

	noToken := filepath.Join(t.TempDir(), "o.yaml")
	_ = os.WriteFile(noToken, []byte("objectId: obj-alerting-1\nschemaId: builtin:alerting.profile\nscope: environment\nvalue: {name: X}\n"), 0o600)
	api, cfg = applyFake(t, "readwrite-all", routes)
	applyJSON(t, cfg, noToken)
	if b := putBody(t, api, "obj-alerting-1"); b["updateToken"] != "server-obj-alerting-1" {
		t.Errorf("PUT body %v, want the GET's token", b)
	}
}

// TestApplyCreatesOnlyAfter404 pins that only a 404 turns an objectId into a
// create; any other error fails the object and creates nothing.
func TestApplyCreatesOnlyAfter404(t *testing.T) {
	file := filepath.Join(t.TempDir(), "o.yaml")
	_ = os.WriteFile(file, []byte("objectId: other-env-id\nschemaId: builtin:test.multi\nscope: environment\nvalue: {name: X}\n"), 0o600)
	routes := createRoutes()
	routes[objectsPath+" schemaIds=builtin:test.multi"] = route{Body: `{"items":[],"totalCount":0}`}
	api, cfg := applyFake(t, "readwrite-all", routes)
	if r, rep := applyJSON(t, cfg, file); r.Code != 0 || actions(rep) != "created" || api.Hits(postObjects) != 1 {
		t.Errorf("404: %s", r)
	}
	routes[objPrefix+"other-env-id"] = route{Status: 403, File: "error_403.json"}
	api, cfg = applyFake(t, "readwrite-all", routes)
	if r, _ := applyJSON(t, cfg, file); r.Code == 0 || api.Hits(postObjects) != 0 {
		t.Errorf("403: %s (%d POSTs)", r, api.Hits(postObjects))
	}
}

func TestApplyMatchesSingletonByScope(t *testing.T) {
	file := filepath.Join(t.TempDir(), "o.yaml")
	_ = os.WriteFile(file, []byte("schemaId: builtin:deployment.oneagent.updates\nscope: HOST_GROUP-0000000000000001\nvalue: {updateMode: AUTOMATIC}\n"), 0o600)
	api, cfg := applyFake(t, "readwrite-all", map[string]route{
		singletonDef:           {Body: `{"multiObject":false}`},
		oneAgentRoute:          {Body: `{"items":[{"objectId":"oa-group","scope":"HOST_GROUP-0000000000000001","updateToken":"t-g","value":{"updateMode":"MANUAL"}}],"totalCount":1}`},
		putPrefix + "oa-group": {Body: `{"code":200,"objectId":"oa-group"}`},
	})
	if r, rep := applyJSON(t, cfg, file); r.Code != 0 || actions(rep) != "updated" || api.Hits(postObjects) != 0 || putBody(t, api, "oa-group")["updateToken"] != "t-g" {
		t.Errorf("%s", r)
	}
}

func TestApplyMatchesExternalID(t *testing.T) {
	file := filepath.Join(t.TempDir(), "o.yaml")
	_ = os.WriteFile(file, []byte("externalId: ext-7\nschemaId: builtin:test.multi\nscope: environment\nvalue: {name: X2}\n"), 0o600)
	routes := createRoutes()
	routes[objectsPath+" schemaIds=builtin:test.multi"] = route{Body: `{"items":[{"objectId":"obj-7","externalId":"ext-7","updateToken":"t-7","value":{"name":"X"}}],"totalCount":1}`}
	routes[putPrefix+"obj-7"] = route{Body: `{"code":200,"objectId":"obj-7"}`}
	api, cfg := applyFake(t, "readwrite-all", routes)
	if r, rep := applyJSON(t, cfg, file); r.Code != 0 || actions(rep) != "updated" || api.Hits(postObjects) != 0 {
		t.Errorf("%s", r)
	}
}

func TestApplyUnchangedSendsNothing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "o.yaml")
	_ = os.WriteFile(file, []byte("objectId: obj-alerting-1\nschemaId: builtin:alerting.profile\nscope: environment\nvalue: {name: Default}\n"), 0o600)
	api, cfg := applyFake(t, "readwrite-all", map[string]route{objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default")})
	if r, rep := applyJSON(t, cfg, file); r.Code != 0 || actions(rep) != "unchanged" || api.Hits(putPrefix+"obj-alerting-1") != 0 {
		t.Errorf("%s", r)
	}
}

// TestApplyTakesAListResponse pins that get settings -o yaml output applies
// as it is: every item, extra top-level keys ignored.
func TestApplyTakesAListResponse(t *testing.T) {
	_, cfg := applyFake(t, "readwrite-all", map[string]route{
		objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default"),
		objPrefix + "obj-alerting-2": alertingObject("obj-alerting-2", "Payments"),
		putPrefix + "obj-alerting-1": {Body: `{"code":200}`},
		putPrefix + "obj-alerting-2": {Body: `{"code":200}`},
	})
	if r, rep := applyJSON(t, cfg, "testdata/apply/export_list.yaml"); r.Code != 0 || len(rep.Results) != 2 {
		t.Errorf("%s", r)
	}
}

// TestApplyConflictFailsOneObject pins that a 409 fails its object only:
// the others run, every result is reported, and the exit code is non-zero.
func TestApplyConflictFailsOneObject(t *testing.T) {
	routes := createRoutes()
	routes[objectsPath+" schemaIds=builtin:test.multi"] = route{Body: `{"items":[],"totalCount":0}`}
	routes[objPrefix+"obj-a"] = alertingObject("obj-a", "A")
	routes[objPrefix+"obj-b"] = alertingObject("obj-b", "B")
	routes[putPrefix+"obj-a"] = route{Body: `{"code":200}`}
	routes[putPrefix+"obj-b"] = route{Status: 409, Body: `{"error":{"code":409,"message":"Outdated version."}}`}
	_, cfg := applyFake(t, "readwrite-all", routes)
	r := runCLI(t, cfg, "apply", "-f", "testdata/apply/mixed.json", "-o", "json")
	var rep writeReport
	_ = json.Unmarshal([]byte(r.Stdout), &rep)
	if r.Code != 1 || actions(rep) != "updated,failed,created" || !strings.Contains(r.Stderr, "re-read") {
		t.Errorf("%s", r)
	}
}

func TestApplyReadwriteMineBlocksAnUpdate(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-mine", map[string]route{
		objPrefix + "obj-alerting-1": alertingObject("obj-alerting-1", "Default"),
		objPrefix + "obj-alerting-2": alertingObject("obj-alerting-2", "Payments"),
	})
	if r, _ := applyJSON(t, cfg, "testdata/apply/export_list.yaml"); r.Code != 6 || api.Hits(putPrefix+"obj-alerting-1") != 0 {
		t.Errorf("%s", r)
	}
}

// TestApplyReportsAnOverwriteAsUpdated pins that a create the server turned
// into an overwrite (the answer names an object that already existed) is
// reported as an update.
func TestApplyReportsAnOverwriteAsUpdated(t *testing.T) {
	file := filepath.Join(t.TempDir(), "o.yaml")
	_ = os.WriteFile(file, []byte("schemaId: builtin:test.multi\nscope: environment\nvalue: {name: Keyed}\n"), 0o600)
	routes := createRoutes()
	routes[objectsPath+" schemaIds=builtin:test.multi"] = route{Body: `{"items":[{"objectId":"obj-9","value":{"name":"Other"}}],"totalCount":1}`}
	routes[postObjects] = route{Body: `[{"code":200,"objectId":"obj-9"}]`}
	_, cfg := applyFake(t, "readwrite-all", routes)
	if r, rep := applyJSON(t, cfg, file); r.Code != 0 || actions(rep) != "updated" {
		t.Errorf("%s", r)
	}
}

func TestApplyStdin(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", map[string]route{
		updateWindowsDef:   {Body: `{"multiObject":true}`},
		updateWindowsRoute: {File: "settings_update_windows.json"},
		postObjects:        {Body: `[{"code":200,"objectId":"new-1"}]`},
	})
	data, err := os.ReadFile("testdata/apply/window_new.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = wr.Write(data)
	_ = wr.Close()
	orig := os.Stdin
	os.Stdin = rd
	t.Cleanup(func() { os.Stdin = orig })
	if r, rep := applyJSON(t, cfg, "-"); r.Code != 0 || actions(rep) != "created" || api.Hits(postObjects) != 1 {
		t.Errorf("%s", r)
	}
}

func TestApplyBadFile(t *testing.T) {
	_, cfg := applyFake(t, "readwrite-all", nil)
	dir := t.TempDir()
	noSchema := filepath.Join(dir, "a.yaml")
	_ = os.WriteFile(noSchema, []byte("scope: environment\nvalue: {name: X}\n"), 0o600)
	if r := runCLI(t, cfg, "apply", "-f", noSchema); r.Code != 2 || !strings.Contains(r.Stderr, "0") {
		t.Errorf("no schemaId: %s", r)
	}
	broken := filepath.Join(dir, "b.yaml")
	_ = os.WriteFile(broken, []byte("{not: [valid"), 0o600)
	if r := runCLI(t, cfg, "apply", "-f", broken); r.Code != 2 {
		t.Errorf("broken YAML: %s", r)
	}
}
