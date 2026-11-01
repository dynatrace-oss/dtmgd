//go:build integration

package integration

import (
	"slices"
	"testing"
)

// The update schemas phase 1 shows as readable tables.
const (
	updateWindowsSchema = "builtin:deployment.management.update-windows"
	oneAgentSchema      = "builtin:deployment.oneagent.updates"
	activeGateSchema    = "builtin:deployment.activegate.updates"
)

func TestSettingsSchemasIncludeTheUpdateSchemas(t *testing.T) {
	body := decode[map[string]any](t, "get", "settings-schemas", "--search", "deployment", "--context", env(t), "-o", "json")
	var ids []string
	for _, item := range items(t, body, "items") {
		m, _ := item.(map[string]any)
		id, _ := m["schemaId"].(string)
		ids = append(ids, id)
	}
	for _, want := range []string{updateWindowsSchema, oneAgentSchema, activeGateSchema} {
		if !slices.Contains(ids, want) {
			t.Errorf("schema %s is not listed (%d deployment schemas listed)", want, len(ids))
		}
	}
}

func TestOneAgentUpdatesSchemaDescribes(t *testing.T) {
	body := decode[map[string]any](t, "describe", "settings-schema", oneAgentSchema, "--context", env(t), "-o", "json")
	props, _ := body["properties"].(map[string]any)
	if _, ok := props["updateMode"]; !ok {
		t.Error("the schema has no updateMode property")
	}
	// Scope names are part of the schema, not secrets: report them.
	scopes, _ := body["allowedScopes"].([]any)
	if !slices.Contains(scopes, any("HOST")) {
		t.Errorf("allowedScopes = %v, want HOST among them", scopes)
	}
}

func TestEffectiveOneAgentUpdatesAtEnvironment(t *testing.T) {
	body := decode[map[string]any](t, "get", "settings", "--effective", "--scope", "environment", "--schema", oneAgentSchema, "--context", env(t), "-o", "json")
	list := items(t, body, "items")
	if len(list) != 1 {
		t.Fatalf("%d effective values, want 1", len(list))
	}
	m, _ := list[0].(map[string]any)
	value, _ := m["value"].(map[string]any)
	if mode, _ := value["updateMode"].(string); mode == "" {
		t.Error("the effective value has no updateMode")
	}
}

func TestUpdateWindowsDecode(t *testing.T) {
	body := decode[map[string]any](t, "get", "settings", "--schema", updateWindowsSchema, "--scope", "environment", "--context", env(t), "-o", "json")
	for i, item := range items(t, body, "items") {
		m, _ := item.(map[string]any)
		value, _ := m["value"].(map[string]any)
		// Recurrence values are enum names, not secrets: report them.
		if r, _ := value["recurrence"].(string); !slices.Contains([]string{"ONCE", "DAILY", "WEEKLY", "MONTHLY"}, r) {
			t.Errorf("window %d has recurrence %q", i, r)
		}
	}
}

// TestSettingsTablesRender pins that the readable tables decode real
// objects: each exits 0. Output is not printed; it holds names and IDs.
func TestSettingsTablesRender(t *testing.T) {
	ctx := env(t)
	for _, args := range [][]string{
		{"get", "settings", "--schema", updateWindowsSchema, "--scope", "environment"},
		{"get", "settings", "--schema", oneAgentSchema},
		{"get", "settings", "--schema", activeGateSchema},
		{"get", "settings", "--effective", "--scope", "environment", "--schema", oneAgentSchema},
	} {
		r := run(t, append(args, "--context", ctx)...)
		skipWithoutScope(t, r, args)
		if r.Code != 0 {
			t.Errorf("%v: %s", args[2:], brief(r))
		}
	}
}
