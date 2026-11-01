package cmd

import (
	"strings"
	"testing"
)

const effectivePath = "GET /e/env1/api/v2/settings/effectiveValues"

// getEffective runs get settings --effective against a fake serving file as
// the effective values (and the update windows for name lookups).
func getEffective(t *testing.T, file string, extra ...string) (cliResult, *fakeAPI) {
	t.Helper()
	routes := updatesRoutes()
	routes[effectivePath] = route{File: file}
	api, cfg := settingsFake(t, routes)
	args := append([]string{"get", "settings", "--effective", "--scope", "HOST-0000000000000001"}, extra...)
	return runCLI(t, cfg, args...), api
}

func TestGetSettingsEffectiveNeedsScope(t *testing.T) {
	api, cfg := settingsFake(t, nil)
	if r := runCLI(t, cfg, "get", "settings", "--effective"); r.Code != 2 || !strings.Contains(r.Stderr, "--scope") {
		t.Errorf("--effective without --scope: %s", r)
	}
	if r := runCLI(t, cfg, "get", "settings", "obj-1", "--effective", "--scope", "environment"); r.Code != 2 {
		t.Errorf("--effective with an object ID: %s", r)
	}
	if api.totalHits() != 0 {
		t.Errorf("%d requests, want none", api.totalHits())
	}
}

// TestGetSettingsEffectiveQuery pins the effective-values query: the scope
// is the singular "scope", unlike the object list's "scopes".
func TestGetSettingsEffectiveQuery(t *testing.T) {
	_, api := getEffective(t, "settings_effective_oneagent.json", "--schema", schemaOneAgentUpdates, "-o", "json")
	qs := api.Queries(effectivePath)
	if len(qs) != 1 {
		t.Fatalf("%d queries, want 1", len(qs))
	}
	q := qs[0]
	if q.Get("scope") != "HOST-0000000000000001" || q.Get("schemaIds") != schemaOneAgentUpdates || q.Get("fields") != settingsEffectiveFields || q.Get("pageSize") != "500" || q.Has("scopes") {
		t.Errorf("query = %v", q)
	}
}

func TestGetSettingsEffectiveOneAgentTable(t *testing.T) {
	r, _ := getEffective(t, "settings_effective_oneagent.json", "--schema", schemaOneAgentUpdates)
	header := strings.Fields(strings.SplitN(r.Stdout, "\n", 2)[0])
	if len(header) == 0 || header[0] != "ORIGIN" || strings.Contains(r.Stdout, "OBJECT-ID") || !strings.Contains(r.Stdout, "Weekly maintenance") {
		t.Errorf("want ORIGIN first, no OBJECT-ID, the window's name:\n%s", r)
	}
	assertGolden(t, "get_settings_effective_oneagent_table", r.String())
}

// TestEffectiveEmptyOriginShowsDefault pins that a value with no origin (the
// schema default applies) reads "default".
func TestEffectiveEmptyOriginShowsDefault(t *testing.T) {
	r, _ := getEffective(t, "settings_effective_default.json", "--schema", schemaOneAgentUpdates)
	if fields := strings.Fields(rowOf(t, r.Stdout, "AUTOMATIC")); fields[0] != "default" {
		t.Errorf("row %q, want ORIGIN default\n%s", fields, r)
	}
}

func TestGetSettingsEffectiveMixed(t *testing.T) {
	r, _ := getEffective(t, "settings_effective_mixed.json")
	header := strings.Fields(strings.SplitN(r.Stdout, "\n", 2)[0])
	if strings.Join(header, " ") != "SCHEMA ORIGIN SUMMARY" || dataRows(r.Stdout) != 3 {
		t.Errorf("want SCHEMA ORIGIN SUMMARY and 3 rows:\n%s", r)
	}
	assertGolden(t, "get_settings_effective_mixed_table", r.String())
}
