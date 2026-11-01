package cmd

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

const (
	objectsPath = "GET /e/env1/api/v2/settings/objects"
	// objectPath is the route of settings_object.json, whose ID needs
	// escaping; route keys hold the decoded path.
	objectPath = objectsPath + "/vu9/x+y="
)

// settingsFake is a fake API answering settings object requests from
// routes, and the config of one context pointing at it.
func settingsFake(t *testing.T, routes map[string]route) (*fakeAPI, string) {
	t.Helper()
	api := newFakeAPI(t, routes)
	return api, withFakeEnvs(t, fakeEnv{"a", api})
}

func TestGetSettingsNeedsSchemaOrScope(t *testing.T) {
	api, cfg := settingsFake(t, nil)
	r := runCLI(t, cfg, "get", "settings")
	if r.Code != 2 || !strings.Contains(r.Stderr, "--schema or --scope") || api.totalHits() != 0 {
		t.Errorf("want exit 2 before any request: %s", r)
	}
}

// TestGetSettingsQuery pins the list query: schema IDs comma-joined from
// repeated and comma-separated --schema, the scope, the fields that make an
// exported object apply back, and the page size.
func TestGetSettingsQuery(t *testing.T) {
	api, cfg := settingsFake(t, map[string]route{objectsPath: {File: "settings_objects_alerting.json"}})
	runCLI(t, cfg, "get", "settings", "--schema", "a", "--schema", "b,c", "--scope", "HOST-1", "-o", "json")
	runCLI(t, cfg, "get", "settings", "--schema", "a", "--limit", "3", "-o", "json")
	qs := api.Queries(objectsPath)
	if len(qs) != 2 {
		t.Fatalf("%d queries, want 2", len(qs))
	}
	q := qs[0]
	if q.Get("schemaIds") != "a,b,c" || q.Get("scopes") != "HOST-1" || q.Get("fields") != settingsObjectFields || q.Get("pageSize") != "500" {
		t.Errorf("query = %v", q)
	}
	if qs[1].Get("pageSize") != "3" || qs[1].Has("scopes") {
		t.Errorf("--limit 3 query = %v", qs[1])
	}
}

func TestGetSettingsPagingSendsOnlyNextPageKey(t *testing.T) {
	routes := listPages(objectsPath, "items", 3, 2, func(i int) string {
		return fmt.Sprintf(`{"objectId":"obj-%d","schemaId":"builtin:alerting.profile","scope":"environment","summary":"p%d"}`, i, i)
	})
	api, cfg := settingsFake(t, routes)
	if r := runCLI(t, cfg, "get", "settings", "--schema", "builtin:alerting.profile", "-o", "json"); listLen(t, r.Stdout, "items") != 3 {
		t.Fatalf("want 3 items: %s", r)
	}
	qs := api.Queries(objectsPath)
	if len(qs) != 2 {
		t.Fatalf("%d queries, want 2", len(qs))
	}
	var keys []string
	for k := range qs[1] {
		keys = append(keys, k)
	}
	if !slices.Equal(keys, []string{"nextPageKey"}) {
		t.Errorf("page 2 sent %v, want nextPageKey alone", qs[1])
	}
}

func TestGetSettingsGenericTable(t *testing.T) {
	_, cfg := settingsFake(t, map[string]route{objectsPath: {File: "settings_objects_alerting.json"}})
	r := runCLI(t, cfg, "get", "settings", "--schema", "builtin:alerting.profile")
	if strings.Contains(r.Stdout, "SCHEMA") {
		t.Errorf("a single schema needs no SCHEMA column:\n%s", r.Stdout)
	}
	assertGolden(t, "get_settings_generic_table", r.String())
}

// TestGetSettingsMixedSchemas pins that a listing spanning several schemas
// gets the generic table with a SCHEMA column and keeps every item.
func TestGetSettingsMixedSchemas(t *testing.T) {
	_, cfg := settingsFake(t, map[string]route{objectsPath: {File: "settings_objects_mixed.json"}})
	r := runCLI(t, cfg, "get", "settings", "--scope", "HOST-0000000000000001")
	if !strings.HasPrefix(strings.TrimSpace(r.Stdout), "SCHEMA") || dataRows(r.Stdout) != 3 {
		t.Errorf("want a SCHEMA column and 3 rows:\n%s", r)
	}
	assertGolden(t, "get_settings_mixed_table", r.String())
}

func TestGetSettingsJSONIsAPIShape(t *testing.T) {
	_, cfg := settingsFake(t, map[string]route{objectsPath: {File: "settings_objects_alerting.json"}})
	r := runCLI(t, cfg, "get", "settings", "--schema", "builtin:alerting.profile", "-o", "json")
	var body struct {
		Items []settingsObject `json:"items"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &body); err != nil || len(body.Items) != 2 {
		t.Fatalf("%v\n%s", err, r)
	}
	got := body.Items[0]
	if got.ObjectID != "obj-alerting-1" || got.SchemaID != "builtin:alerting.profile" || got.Scope != "environment" || got.Value["name"] != "Default" || got.UpdateToken != "tok-1" {
		t.Errorf("items[0] = %+v", got)
	}
}

func TestGetSettingsByObjectID(t *testing.T) {
	api, cfg := settingsFake(t, map[string]route{objectPath: {File: "settings_object.json"}})
	r := runCLI(t, cfg, "get", "settings", "vu9/x+y=")
	if r.Code != 0 || dataRows(r.Stdout) != 1 {
		t.Fatalf("want one row: %s", r)
	}
	if p := api.EscapedPaths(objectPath); len(p) != 1 || !strings.HasSuffix(p[0], "/vu9%2Fx+y=") {
		t.Errorf("sent %v, want the ID escaped", p)
	}
	if r := runCLI(t, cfg, "get", "settings", "vu9", "--schema", "a"); r.Code != 2 {
		t.Errorf("an ID with --schema: %s", r)
	}
}

func TestGetSettingsUnknownSchema(t *testing.T) {
	_, cfg := settingsFake(t, map[string]route{objectsPath: {
		Status: 404, Body: `{"error":{"code":404,"message":"Configuration schema x does not exist."}}`,
	}})
	r := runCLI(t, cfg, "get", "settings", "--schema", "x")
	if r.Code != 4 || !strings.Contains(r.Stderr, "get settings-schemas --search") {
		t.Errorf("want exit 4 with a hint: %s", r)
	}
}

func TestGetSettingsAgentEnvelope(t *testing.T) {
	_, cfg := settingsFake(t, map[string]route{objectsPath: {File: "settings_objects_alerting.json"}})
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "get", "settings", "--schema", "builtin:alerting.profile")
	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Items []interface{} `json:"items"`
		} `json:"result"`
		Context struct {
			Resource string `json:"resource"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil {
		t.Fatalf("%v\n%s", err, r)
	}
	if !env.OK || len(env.Result.Items) != 2 || env.Context.Resource != "settings" {
		t.Errorf("envelope = %+v", env)
	}
}

func TestDescribeSettingsHuman(t *testing.T) {
	_, cfg := settingsFake(t, map[string]route{
		objectPath:                   {File: "settings_object.json"},
		objectsPath + "/obj-plain-1": {Body: `{"objectId":"obj-plain-1","schemaId":"builtin:alerting.profile","scope":"environment","value":{"name":"x"}}`},
	})
	r := runCLI(t, cfg, "describe", "settings", "vu9/x+y=")
	assertGolden(t, "describe_settings_human", r.String())
	if plain := runCLI(t, cfg, "describe", "settings", "obj-plain-1"); plain.Code != 0 || strings.Contains(plain.Stdout, "External ID:") {
		t.Errorf("an object without externalId: %s", plain)
	}
}

// TestGetSettingsNotFoundHintNamesScope pins that a 404 on a listing also
// points at the scopes a schema allows: the API answers 404 for a scope it
// cannot use, too.
func TestGetSettingsNotFoundHintNamesScope(t *testing.T) {
	_, cfg := settingsFake(t, map[string]route{objectsPath: {
		Status: 404, Body: `{"error":{"code":404,"message":"Scope 'host-1' not found"}}`,
	}})
	r := runCLI(t, cfg, "get", "settings", "--schema", "x", "--scope", "host-1")
	if r.Code != 4 || !strings.Contains(r.Stderr, "describe settings-schema") {
		t.Errorf("%s", r)
	}
}

// TestGetSettingsHelpExplainsEffective pins that get settings' help says
// what --effective returns.
func TestGetSettingsHelpExplainsEffective(t *testing.T) {
	r := runCLI(t, "", "get", "settings", "--help")
	if !strings.Contains(r.Stdout, "get settings --effective --scope") || !strings.Contains(r.Stdout, "origin") {
		t.Errorf("%s", r.Stdout)
	}
}

// TestSettingsSchemaAliases pins the singular and plural forms the other
// get and describe commands accept.
func TestSettingsSchemaAliases(t *testing.T) {
	_, cfg := schemasFake(t)
	for _, args := range [][]string{
		{"get", "settings-schema"},
		{"describe", "settings-schemas", "builtin:deployment.oneagent.updates"},
	} {
		if r := runCLI(t, cfg, args...); r.Code != 0 {
			t.Errorf("%v: %s", args, r)
		}
	}
}
