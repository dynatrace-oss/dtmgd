package cmd

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	clusterSchemas      = "GET /api/cluster/v2/settings/schemas"
	clusterObjects      = "GET /api/cluster/v2/settings/objects"
	clusterEffective    = "GET /api/cluster/v2/settings/effectiveValues"
	clusterPost         = "POST /api/cluster/v2/settings/objects"
	clusterObjPrefix    = "GET /api/cluster/v2/settings/objects/"
	clusterDelPrefix    = "DELETE /api/cluster/v2/settings/objects/"
	clusterWindowDef    = "GET /api/cluster/v2/settings/schemas/" + schemaUpdateWindows
	clusterSchemaPrefix = "/api/cluster/v2/settings/schemas"
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
	maps.Copy(routes, schemaCatalog(clusterSchemaPrefix, map[string]string{"builtin:test.multi": `{"multiObject":true}`}))
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

func TestClusterDeleteRefusesAReferencedObject(t *testing.T) {
	routes := map[string]route{
		clusterObjPrefix + "obj-x": {Body: `{"objectId":"obj-x","schemaId":"builtin:test.multi","scope":"cluster","updateToken":"t-x","value":{}}`},
		clusterDelPrefix + "obj-x": {Body: ""},
		clusterObjects:             {Body: `{"items":[{"objectId":"user-1","schemaId":"builtin:test.user","scope":"cluster","summary":"Uses it","value":{"ref":"obj-x"}}],"totalCount":1}`},
	}
	maps.Copy(routes, schemaCatalog(clusterSchemaPrefix, map[string]string{
		"builtin:test.multi": `{"multiObject":true}`,
		"builtin:test.user":  `{"properties":{"ref":{"type":"setting","referencedType":"builtin:test.multi"}}}`,
	}))
	api, _, cfg := clusterSettingsFake(t, "readwrite-all", routes)
	r := runCLI(t, cfg, "delete", "settings", "obj-x", "--yes")
	if r.Code != 1 || !strings.Contains(r.Stderr, "user-1") || api.Hits(clusterObjects) != 1 || api.Hits(clusterDelPrefix+"obj-x") != 0 {
		t.Errorf("%s", r)
	}
}

const noSchemaScopeOnly = `{"error":{"code":404,"message":"No schema with schemaId builtin:accounting.ddu.limit"}}`

// clusterFallbackRoutes is a cluster that cannot list by scope alone: three
// schemas (listed b, a, c), where a and b hold objects and c is a schema
// whose scope the cluster does not serve.
func clusterFallbackRoutes(path string) map[string]route {
	routes := schemaCatalog(clusterSchemaPrefix, map[string]string{"builtin:b": "{}", "builtin:a": "{}", "builtin:c": "{}"})
	routes["GET "+path] = route{Status: 404, Body: noSchemaScopeOnly}
	routes["GET "+path+" schemaIds=builtin:a"] = route{Body: `{"items":[{"objectId":"a1"},{"objectId":"a2"}],"totalCount":2}`}
	routes["GET "+path+" schemaIds=builtin:b"] = route{Body: `{"items":[{"objectId":"b1"}],"totalCount":1}`}
	routes["GET "+path+" schemaIds=builtin:c"] = route{Status: 404, Body: `{"error":{"code":404,"message":"No schema with topic identifier 'x'"}}`}
	return routes
}

func wantObjectIDs(t *testing.T, r cliResult, want ...string) {
	t.Helper()
	var got struct {
		Items      []struct{ ObjectID string }
		TotalCount int
	}
	if err := json.Unmarshal([]byte(r.Stdout), &got); err != nil || r.Code != 0 {
		t.Fatalf("%v\n%s", err, r)
	}
	var ids []string
	for _, it := range got.Items {
		ids = append(ids, it.ObjectID)
	}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestClusterScopeFallbackMergesSchemas(t *testing.T) {
	_, _, cfg := clusterSettingsFake(t, "", clusterFallbackRoutes("/api/cluster/v2/settings/objects"))
	r := runCLI(t, cfg, "get", "settings", "--scope", "cluster", "-o", "json")
	wantObjectIDs(t, r, "a1", "a2", "b1")
	if !strings.Contains(r.Stdout, `"totalCount": 3`) {
		t.Errorf("totalCount not 3:\n%s", r.Stdout)
	}
}

func TestClusterScopeFallbackForEffectiveValues(t *testing.T) {
	api, _, cfg := clusterSettingsFake(t, "", clusterFallbackRoutes("/api/cluster/v2/settings/effectiveValues"))
	r := runCLI(t, cfg, "get", "settings", "--effective", "--scope", "cluster", "-o", "json")
	wantObjectIDs(t, r, "a1", "a2", "b1")
	var ids []string
	queries := api.Queries(clusterEffective)
	if len(queries) < 2 {
		t.Fatalf("want the scope-only request and one per schema, got %d requests", len(queries))
	}
	for _, q := range queries[1:] {
		if q.Get("scope") != "cluster" {
			t.Errorf("scope = %q", q.Get("scope"))
		}
		ids = append(ids, q.Get("schemaIds"))
	}
	sort.Strings(ids)
	if strings.Join(ids, ",") != "builtin:a,builtin:b,builtin:c" {
		t.Errorf("per-schema schemaIds = %v", ids)
	}
}

func TestClusterScopeFallbackTruncationIsNotSilent(t *testing.T) {
	_, _, cfg := clusterSettingsFake(t, "", clusterFallbackRoutes("/api/cluster/v2/settings/objects"))
	r := runCLI(t, cfg, "get", "settings", "--scope", "cluster", "--limit", "1")
	if r.Code != 0 || strings.Contains(r.Stdout, "a2") || strings.Contains(r.Stdout, "b1") {
		t.Errorf("%s", r)
	}
	wantStderr(t, r, "Use --limit or narrower filters")
}

func TestEnvironmentScopeListingHasNoFallback(t *testing.T) {
	env := newFakeAPI(t, map[string]route{"GET /e/env-a/api/v2/settings/objects": {Status: 404, Body: noSchemaScopeOnly}})
	api := newFakeAPI(t, nil)
	cfg := withFakeCluster(t, "", api, env)
	r := runCLI(t, cfg, "--context", "a", "get", "settings", "--scope", "environment")
	if r.Code != 4 || env.Hits("GET /e/env-a/api/v2/settings/schemas") != 0 {
		t.Errorf("%s", r)
	}
}

func TestClusterSettings404WithMessageHasNoHostHint(t *testing.T) {
	_, _, cfg := clusterSettingsFake(t, "", map[string]route{clusterObjPrefix + "bad": {Status: 404, Body: `{"error":{"code":404,"message":"Settings objectId 'bad' not found"}}`}})
	r := runCLI(t, cfg, "describe", "settings", "bad")
	wantStderr(t, r, hintObjectNotFound)
	if r.Code != 4 || strings.Contains(r.Stderr, "port 9999") {
		t.Errorf("%s", r)
	}
}

func TestClusterScopeFallbackFailsOnANon404SchemaError(t *testing.T) {
	routes := clusterFallbackRoutes("/api/cluster/v2/settings/objects")
	routes["GET /api/cluster/v2/settings/objects schemaIds=builtin:b"] = route{Status: 400, Body: `{"error":{"code":400,"message":"Constraints violated"}}`}
	_, _, cfg := clusterSettingsFake(t, "", routes)
	r := runCLI(t, cfg, "get", "settings", "--scope", "cluster", "-o", "json")
	if r.Code == 0 || strings.Contains(r.Stdout, "a1") {
		t.Errorf("%s", r)
	}
}

// TestClusterScopeFallbackKeepsAPartialSchema pins that a schema cut short
// by the page cap makes the merged list partial: no totalCount, a note.
func TestClusterScopeFallbackKeepsAPartialSchema(t *testing.T) {
	routes := clusterFallbackRoutes("/api/cluster/v2/settings/objects")
	routes["GET /api/cluster/v2/settings/objects schemaIds=builtin:a"] = route{Body: `{"items":[{"objectId":"a1"},{"objectId":"a2"}],"nextPageKey":"more"}`}
	_, _, cfg := clusterSettingsFake(t, "", routes)
	r := runCLI(t, cfg, "get", "settings", "--scope", "cluster", "--max-pages", "1", "-o", "json")
	wantObjectIDs(t, r, "a1", "a2", "b1")
	if strings.Contains(r.Stdout, "totalCount") {
		t.Errorf("totalCount present:\n%s", r.Stdout)
	}
	r = runCLI(t, cfg, "get", "settings", "--scope", "cluster", "--max-pages", "1")
	wantStderr(t, r, "Use --limit or narrower filters")
}

// TestClusterScopeFallbackSkipsSchemasNotQueryableAtTheScope pins a live
// finding: a schema whose allowed scope is another one answers 400 "Not
// allowed to query scope", which skips it like a 404 does.
func TestClusterScopeFallbackSkipsSchemasNotQueryableAtTheScope(t *testing.T) {
	notAllowed := route{Status: 400, Body: `{"error":{"code":400,"message":"Not allowed to query scope tenant for config 'builtin:b'"}}`}
	for name, tc := range map[string]struct {
		path string
		args []string
	}{
		"stored":    {"/api/cluster/v2/settings/objects", []string{"get", "settings", "--scope", "cluster", "-o", "json"}},
		"effective": {"/api/cluster/v2/settings/effectiveValues", []string{"get", "settings", "--effective", "--scope", "cluster", "-o", "json"}},
	} {
		t.Run(name, func(t *testing.T) {
			routes := clusterFallbackRoutes(tc.path)
			routes["GET "+tc.path+" schemaIds=builtin:b"] = notAllowed
			_, _, cfg := clusterSettingsFake(t, "", routes)
			wantObjectIDs(t, runCLI(t, cfg, tc.args...), "a1", "a2")
		})
	}
}

// TestClusterScopeFallbackErrorsWhenNoSchemaAnswers pins that a scope no
// schema can answer at (a typo, an environment scope on a cluster) is an
// error with a hint, not "No settings found.".
func TestClusterScopeFallbackErrorsWhenNoSchemaAnswers(t *testing.T) {
	path := "/api/cluster/v2/settings/objects"
	routes := clusterFallbackRoutes(path)
	for _, id := range []string{"a", "b", "c"} {
		routes["GET "+path+" schemaIds=builtin:"+id] = route{Status: 404, Body: `{"error":{"code":404,"message":"No schema with topic identifier 'x'"}}`}
	}
	_, _, cfg := clusterSettingsFake(t, "", routes)
	r := runCLI(t, cfg, "get", "settings", "--scope", "clustr")
	if r.Code != 4 || strings.Contains(r.Stdout, "No settings found") {
		t.Errorf("%s", r)
	}
	wantStderr(t, r, "No settings schema of this cluster can be read at scope clustr")
}

func TestClusterScopeFallbackEmptyAnswerIsNotAnError(t *testing.T) {
	path := "/api/cluster/v2/settings/objects"
	routes := clusterFallbackRoutes(path)
	routes["GET "+path+" schemaIds=builtin:a"] = route{Body: `{"items":[],"totalCount":0}`}
	routes["GET "+path+" schemaIds=builtin:b"] = route{Status: 404, Body: `{"error":{"code":404,"message":"No schema with topic identifier 'x'"}}`}
	_, _, cfg := clusterSettingsFake(t, "", routes)
	r := runCLI(t, cfg, "get", "settings", "--scope", "cluster")
	if r.Code != 0 {
		t.Errorf("%s", r)
	}
	wantStderr(t, r, "No settings found.")
}

func TestClusterScopeFallbackKnowsTheTotalWhenLimited(t *testing.T) {
	path := "/api/cluster/v2/settings/objects"
	routes := clusterFallbackRoutes(path)
	routes["GET "+path+" schemaIds=builtin:a"] = route{Body: `{"items":[{"objectId":"a1"}],"totalCount":1}`}
	_, _, cfg := clusterSettingsFake(t, "", routes)
	r := runCLI(t, cfg, "get", "settings", "--scope", "cluster", "--limit", "1", "-o", "json")
	wantObjectIDs(t, r, "a1")
	if !strings.Contains(r.Stdout, `"totalCount": 2`) {
		t.Errorf("totalCount not 2:\n%s", r.Stdout)
	}
}
