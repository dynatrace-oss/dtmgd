package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	clusterSchemas   = "GET /api/cluster/v2/settings/schemas"
	clusterObjects   = "GET /api/cluster/v2/settings/objects"
	clusterEffective = "GET /api/cluster/v2/settings/effectiveValues"
	clusterPost      = "POST /api/cluster/v2/settings/objects"
	clusterObjPrefix = "GET /api/cluster/v2/settings/objects/"
	clusterDelPrefix = "DELETE /api/cluster/v2/settings/objects/"
	clusterWindowDef = "GET /api/cluster/v2/settings/schemas/" + schemaUpdateWindows
)

// withFakeCluster writes a config whose current context is cluster context
// "c" at level (none when ""), pointing at api, beside environment context
// "a" and "b" pointing at env.
func withFakeCluster(t *testing.T, level string, api, env *fakeAPI) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("apiVersion: dtmgd.io/v1\nkind: Config\ncurrent-context: c\ncontexts:\n")
	fmt.Fprintf(&b, "  - name: c\n    context:\n      host: %s\n      token-ref: tok\n", api.URL)
	if level != "" {
		fmt.Fprintf(&b, "      safety-level: %s\n", level)
	}
	for _, name := range []string{"a", "b"} {
		fmt.Fprintf(&b, "  - name: %s\n    context:\n      host: %s\n      env-id: env-%s\n      token-ref: tok\n      safety-level: readwrite-all\n", name, env.URL, name)
	}
	b.WriteString("tokens:\n  - name: tok\n    token: dt0c01.TEST\n")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// clusterSettingsFake is a cluster at level serving routes, and its config.
func clusterSettingsFake(t *testing.T, level string, routes map[string]route) (*fakeAPI, *fakeAPI, string) {
	t.Helper()
	api, env := newFakeAPI(t, routes), newFakeAPI(t, nil)
	return api, env, withFakeCluster(t, level, api, env)
}

const clusterWindows = `{"items":[{"objectId":"uw-c1","schemaId":"builtin:deployment.management.update-windows","scope":"cluster","value":{"enabled":true,"name":"Cluster window","recurrence":"DAILY","dailyRecurrence":{"every":1,"updateTime":{"startTime":"01:00","timeZone":"GMT+00:00","duration":30}}}}],"totalCount":1}`

func TestClusterSettingsSchemas(t *testing.T) {
	api, _, cfg := clusterSettingsFake(t, "", map[string]route{clusterSchemas: {File: "settings_schemas.json"}})
	r := runCLI(t, cfg, "get", "settings-schemas")
	if r.Code != 0 || dataRows(r.Stdout) != 5 || api.Hits(clusterSchemas) != 1 {
		t.Errorf("%s", r)
	}
}

func TestClusterGetSettings(t *testing.T) {
	api, _, cfg := clusterSettingsFake(t, "", map[string]route{clusterObjects: {Body: clusterWindows}})
	r := runCLI(t, cfg, "get", "settings", "--schema", schemaUpdateWindows, "--scope", "cluster")
	qs := api.Queries(clusterObjects)
	if r.Code != 0 || len(qs) != 1 || qs[0].Get("scopes") != "cluster" || qs[0].Get("fields") != settingsObjectFields || !strings.Contains(r.Stdout, "daily 01:00 GMT+00:00, 30 min") {
		t.Errorf("%s", r)
	}
}

func TestClusterEffectiveValues(t *testing.T) {
	api, _, cfg := clusterSettingsFake(t, "", map[string]route{clusterEffective: {Body: `{"items":[],"totalCount":0}`}})
	if r := runCLI(t, cfg, "get", "settings", "--effective", "--scope", "cluster", "-o", "json"); r.Code != 0 || len(api.Queries(clusterEffective)) != 1 || api.Queries(clusterEffective)[0].Get("scope") != "cluster" {
		t.Errorf("%s", r)
	}
}

func TestClusterDescribeSettings(t *testing.T) {
	api, _, cfg := clusterSettingsFake(t, "", map[string]route{clusterObjPrefix + "uw-c1": {Body: `{"objectId":"uw-c1","schemaId":"builtin:deployment.management.update-windows","scope":"cluster","value":{"name":"Cluster window"}}`}})
	if r := runCLI(t, cfg, "describe", "settings", "uw-c1"); r.Code != 0 || api.Hits(clusterObjPrefix+"uw-c1") != 1 || !strings.Contains(r.Stdout, "Cluster window") {
		t.Errorf("%s", r)
	}
}

// TestClusterSettings403NamesTheScope pins the hint for a cluster token made
// with ServiceProviderAPI only, as tokens for the cluster commands were.
func TestClusterSettings403NamesTheScope(t *testing.T) {
	_, _, cfg := clusterSettingsFake(t, "", map[string]route{clusterObjects: {Status: 403, File: "error_403.json"}})
	r := runCLI(t, cfg, "get", "settings", "--scope", "cluster")
	if r.Code != 5 || !strings.Contains(r.Stderr, "settings.read") || !strings.Contains(r.Stderr, "cluster token") {
		t.Errorf("%s", r)
	}
}

// TestClusterSettingsRefuseFanOut pins that a cluster context is never part
// of a multi-environment run, for reads and writes.
func TestClusterSettingsRefuseFanOut(t *testing.T) {
	api, env, cfg := clusterSettingsFake(t, "readwrite-all", createRoutes())
	for _, args := range [][]string{
		{"get", "settings", "--scope", "cluster", "--env", "a;c"},
		{"apply", "-f", tempFile(t, "schemaId: builtin:test.multi\nscope: cluster\nvalue: {name: X}\n"), "--env", "a;c"},
	} {
		if r := runCLI(t, cfg, args...); r.Code != 2 {
			t.Errorf("%v: %s", args[:2], r)
		}
	}
	if api.totalHits()+env.totalHits() != 0 {
		t.Errorf("%d requests, want none", api.totalHits()+env.totalHits())
	}
}

func TestClusterApplyCreatesAWindow(t *testing.T) {
	api, _, cfg := clusterSettingsFake(t, "readwrite-all", map[string]route{
		clusterWindowDef: {Body: `{"multiObject":true}`},
		clusterObjects:   {Body: `{"items":[],"totalCount":0}`},
		clusterPost:      {Body: `[{"code":200,"objectId":"uw-new"}]`},
	})
	file := tempFile(t, "schemaId: "+schemaUpdateWindows+"\nscope: cluster\nvalue: {enabled: false, name: dtmgd-test, recurrence: DAILY}\n")
	r, rep := applyJSON(t, cfg, file)
	if r.Code != 0 || actions(rep) != "created" || api.Hits(clusterPost) != 1 {
		t.Errorf("%s", r)
	}
}

func TestClusterWritesKeepTheGuards(t *testing.T) {
	file := tempFile(t, "schemaId: "+schemaUpdateWindows+"\nscope: cluster\nvalue: {enabled: false, name: dtmgd-test, recurrence: DAILY}\n")
	routes := map[string]route{
		clusterWindowDef:           {Body: `{"multiObject":true}`},
		clusterObjects:             {Body: `{"items":[],"totalCount":0}`},
		clusterPost:                {Body: `[{"code":200,"objectId":"uw-new"}]`},
		clusterVersionPath:         {Body: nodeList("1.287.0.20240101-000000")},
		clusterObjPrefix + "obj-x": {Body: `{"objectId":"obj-x","schemaId":"builtin:test.multi","scope":"cluster","updateToken":"t-x","value":{}}`},
		clusterDelPrefix + "obj-x": {Body: ""},
	}
	api, _, cfg := clusterSettingsFake(t, "", routes)
	if r := runCLI(t, cfg, "apply", "-f", file); r.Code != 6 || api.totalHits() != 0 {
		t.Errorf("readonly: %s", r)
	}
	api, _, cfg = clusterSettingsFake(t, "readwrite-all", routes)
	if r := runCLI(t, cfg, "apply", "-f", file, "--validate-only"); r.Code != 2 || api.Hits(clusterPost) != 0 {
		t.Errorf("1.287: %s", r)
	}
	if r := runCLI(t, cfg, "delete", "settings", "obj-x", "--yes"); r.Code != 0 || api.Hits(clusterDelPrefix+"obj-x") != 1 {
		t.Errorf("delete: %s", r)
	}
}
