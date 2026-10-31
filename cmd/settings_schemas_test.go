package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	schemasPath        = "GET /e/env1/api/v2/settings/schemas"
	oneAgentSchemaPath = schemasPath + "/builtin:deployment.oneagent.updates"
)

// schemasFake is a fake API serving the schema list and the OneAgent updates
// schema, and the config of one context pointing at it.
func schemasFake(t *testing.T) (*fakeAPI, string) {
	t.Helper()
	api := newFakeAPI(t, map[string]route{
		schemasPath:        {File: "settings_schemas.json"},
		oneAgentSchemaPath: {File: "settings_schema_oneagent_updates.json"},
	})
	return api, withFakeEnvs(t, fakeEnv{"a", api})
}

// dataRows counts a table's rows below its header.
func dataRows(stdout string) int {
	return strings.Count(strings.TrimSpace(stdout), "\n")
}

func TestGetSettingsSchemasTable(t *testing.T) {
	_, cfg := schemasFake(t)
	assertGolden(t, "get_settings_schemas_table", runCLI(t, cfg, "get", "settings-schemas").String())
}

// TestGetSettingsSchemasSearch pins --search: a case-insensitive match on the
// schema ID or display name, with the total recounted.
func TestGetSettingsSchemasSearch(t *testing.T) {
	_, cfg := schemasFake(t)
	r := runCLI(t, cfg, "get", "settings-schemas", "--search", "ONEAGENT", "-o", "json")
	var body struct {
		Items      []map[string]interface{} `json:"items"`
		TotalCount int                      `json:"totalCount"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &body); err != nil {
		t.Fatalf("%v\n%s", err, r)
	}
	if len(body.Items) != 1 || body.Items[0]["schemaId"] != "builtin:deployment.oneagent.updates" || body.TotalCount != 1 {
		t.Errorf("items %v, totalCount %d", body.Items, body.TotalCount)
	}
	if byName := runCLI(t, cfg, "get", "settings-schemas", "--search", "update windows"); dataRows(byName.Stdout) != 1 {
		t.Errorf("display-name search: %s", byName)
	}
}

func TestGetSettingsSchemasLimit(t *testing.T) {
	_, cfg := schemasFake(t)
	r := runCLI(t, cfg, "get", "settings-schemas", "--limit", "2")
	if r.Code != 0 || dataRows(r.Stdout) != 2 {
		t.Fatalf("want 2 rows: %s", r)
	}
	if !strings.Contains(r.Stderr, "Showing 2 of 5 settings-schemas") {
		t.Errorf("stderr %q", r.Stderr)
	}
}

func TestGetSettingsSchemaByID(t *testing.T) {
	api, cfg := schemasFake(t)
	r := runCLI(t, cfg, "get", "settings-schemas", "builtin:deployment.oneagent.updates")
	if r.Code != 0 || dataRows(r.Stdout) != 1 || !strings.Contains(r.Stdout, "1.0.4") {
		t.Fatalf("want one row with version 1.0.4: %s", r)
	}
	if n := api.Hits(oneAgentSchemaPath); n != 1 {
		t.Errorf("%d requests for the schema, want 1", n)
	}
	if r := runCLI(t, cfg, "get", "settings-schemas", "x", "--search", "y"); r.Code != 2 {
		t.Errorf("--search with an ID: %s", r)
	}
}

func TestDescribeSettingsSchemaHuman(t *testing.T) {
	_, cfg := schemasFake(t)
	r := runCLI(t, cfg, "describe", "settings-schema", "builtin:deployment.oneagent.updates")
	for _, want := range []string{"AUTOMATIC, AUTOMATIC_DURING_MW, MANUAL", "HOST, HOST_GROUP, environment", "one per scope", "set of MaintenanceWindow"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("stdout lacks %q", want)
		}
	}
	assertGolden(t, "describe_settings_schema_human", r.String())
}

func TestDescribeSettingsSchemaNotFound(t *testing.T) {
	api := newFakeAPI(t, map[string]route{schemasPath + "/builtin:nope": {
		Status: 404, Body: `{"error":{"code":404,"message":"Configuration schema builtin:nope does not exist."}}`,
	}})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "describe", "settings-schema", "builtin:nope")
	if r.Code != 4 || !strings.Contains(r.Stderr, "dtmgd get settings-schemas --search") {
		t.Errorf("want exit 4 with a hint: %s", r)
	}
}
