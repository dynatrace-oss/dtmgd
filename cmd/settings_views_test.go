package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	updateWindowsRoute = objectsPath + " schemaIds=" + schemaUpdateWindows
	oneAgentRoute      = objectsPath + " schemaIds=" + schemaOneAgentUpdates
	activeGateRoute    = objectsPath + " schemaIds=" + schemaActiveGateUpdates
)

// updatesRoutes serves the update windows and the OneAgent and ActiveGate
// update settings by the schema the query asks for.
func updatesRoutes() map[string]route {
	return map[string]route{
		updateWindowsRoute: {File: "settings_update_windows.json"},
		oneAgentRoute:      {File: "settings_oneagent_updates.json"},
		activeGateRoute:    {File: "settings_activegate_updates.json"},
	}
}

func TestUpdateWindowSchedule(t *testing.T) {
	cases := map[string]string{
		`{"recurrence":"WEEKLY","weeklyRecurrence":{"every":1,"selectedWeekDays":{"monday":true},"updateTime":{"startTime":"03:00","timeZone":"GMT+00:00","duration":60}}}`:               "weekly Mon 03:00 GMT+00:00, 60 min",
		`{"recurrence":"WEEKLY","weeklyRecurrence":{"every":2,"selectedWeekDays":{"friday":true,"monday":true},"updateTime":{"startTime":"03:00","timeZone":"GMT+01:00","duration":90}}}`: "every 2 weeks Mon,Fri 03:00 GMT+01:00, 90 min",
		`{"recurrence":"DAILY","dailyRecurrence":{"every":1,"updateTime":{"startTime":"03:00","timeZone":"GMT+00:00","duration":60}}}`:                                                    "daily 03:00 GMT+00:00, 60 min",
		`{"recurrence":"DAILY","dailyRecurrence":{"every":3,"updateTime":{"startTime":"03:00","timeZone":"GMT+00:00","duration":60}}}`:                                                    "every 3 days 03:00 GMT+00:00, 60 min",
		`{"recurrence":"MONTHLY","monthlyRecurrence":{"every":1,"selectedMonthDay":15,"updateTime":{"startTime":"03:00","timeZone":"GMT+00:00","duration":60}}}`:                          "monthly on day 15 03:00 GMT+00:00, 60 min",
		`{"recurrence":"MONTHLY","monthlyRecurrence":{"every":2,"selectedMonthDay":15,"updateTime":{"startTime":"03:00","timeZone":"GMT+00:00","duration":60}}}`:                          "every 2 months on day 15 03:00 GMT+00:00, 60 min",
		`{"recurrence":"ONCE","onceRecurrence":{"recurrenceRange":{"start":"2026-11-01T03:00:00Z","end":"2026-11-01T04:00:00Z"}}}`:                                                        "once 2026-11-01T03:00:00Z – 2026-11-01T04:00:00Z",
		`{"recurrence":"WEEKLY"}`: "WEEKLY",
	}
	for in, want := range cases {
		var v map[string]interface{}
		if err := json.Unmarshal([]byte(in), &v); err != nil {
			t.Fatal(err)
		}
		if got := updateWindowSchedule(v); got != want {
			t.Errorf("updateWindowSchedule(%s) = %q, want %q", in, got, want)
		}
	}
}

// getUpdates runs get settings for schema against updatesRoutes in table
// form and returns the result and the fake API.
func getUpdates(t *testing.T, schema string, extra ...string) (cliResult, *fakeAPI) {
	t.Helper()
	api, cfg := settingsFake(t, updatesRoutes())
	return runCLI(t, cfg, append([]string{"get", "settings", "--schema", schema}, extra...)...), api
}

// rowOf returns the line of a table holding key.
func rowOf(t *testing.T, stdout, key string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, key) {
			return line
		}
	}
	t.Fatalf("no row holds %q:\n%s", key, stdout)
	return ""
}

func TestGetSettingsUpdateWindowsTable(t *testing.T) {
	r, _ := getUpdates(t, schemaUpdateWindows)
	if !strings.HasPrefix(strings.TrimSpace(r.Stdout), "NAME") || !strings.Contains(rowOf(t, r.Stdout, "uw-weekly"), " no ") {
		t.Errorf("want NAME first and the weekly window disabled:\n%s", r)
	}
	assertGolden(t, "get_settings_update_windows_table", r.String())
}

func TestGetSettingsOneAgentUpdatesTable(t *testing.T) {
	r, _ := getUpdates(t, schemaOneAgentUpdates)
	if row := rowOf(t, r.Stdout, "oa-env"); !strings.Contains(row, "Weekly maintenance, uw-gone") {
		t.Errorf("environment row: %q", row)
	}
	if row := rowOf(t, r.Stdout, "oa-group"); !strings.Contains(row, "MANUAL") || !strings.Contains(row, "—") {
		t.Errorf("host group row: %q", row)
	}
	if row := rowOf(t, r.Stdout, "oa-host"); !strings.Contains(row, "1.341") || !strings.Contains(row, "latest") {
		t.Errorf("host row: %q", row)
	}
	assertGolden(t, "get_settings_oneagent_updates_table", r.String())
}

// TestUpdatesTableShowsUnknownWindowID pins that a reference to a window
// that no longer exists (the server lets such a window be deleted) shows the
// window's object ID.
func TestUpdatesTableShowsUnknownWindowID(t *testing.T) {
	r, _ := getUpdates(t, schemaOneAgentUpdates)
	if r.Code != 0 || !strings.Contains(rowOf(t, r.Stdout, "oa-env"), "uw-gone") {
		t.Errorf("%s", r)
	}
}

func TestGetSettingsActiveGateUpdatesTable(t *testing.T) {
	r, _ := getUpdates(t, schemaActiveGateUpdates)
	if row := rowOf(t, r.Stdout, "ag-env"); !strings.Contains(row, "Daily window") {
		t.Errorf("environment row: %q", row)
	}
	assertGolden(t, "get_settings_activegate_updates_table", r.String())
}

// TestOldValueShapesStillRender pins that a value in a shape the table does
// not know (ActiveGate updates before schema 1.0.4) renders with empty
// cells instead of failing.
func TestOldValueShapesStillRender(t *testing.T) {
	r, _ := getUpdates(t, schemaActiveGateUpdates)
	fields := strings.Fields(rowOf(t, r.Stdout, "ag-old"))
	if r.Code != 0 || strings.Count(strings.Join(fields, " "), "—") != 3 {
		t.Errorf("want UPDATE-MODE, TARGET-VERSION and WINDOWS empty: %q\n%s", fields, r)
	}
}

// lookups counts the update-window name lookups get settings sent.
func lookups(api *fakeAPI) int {
	return len(api.Queries(objectsPath)) - 1
}

func TestWindowLookupOnlyForTables(t *testing.T) {
	for _, args := range [][]string{{"-o", "json"}, {"-o", "yaml"}} {
		if _, api := getUpdates(t, schemaOneAgentUpdates, args...); lookups(api) != 0 {
			t.Errorf("%v sent %d name lookups", args, lookups(api))
		}
	}
	api, cfg := settingsFake(t, updatesRoutes())
	runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "get", "settings", "--schema", schemaOneAgentUpdates)
	if lookups(api) != 0 {
		t.Errorf("agent mode sent %d name lookups", lookups(api))
	}
	_, api = getUpdates(t, schemaOneAgentUpdates)
	qs := api.Queries(objectsPath)
	if len(qs) != 2 {
		t.Fatalf("table: %d queries, want 2", len(qs))
	}
	if q := qs[1]; q.Get("schemaIds") != schemaUpdateWindows || q.Get("scopes") != "environment" || q.Get("fields") != "objectId,value" {
		t.Errorf("lookup query = %v", q)
	}
}

func TestWindowLookupFailureWarns(t *testing.T) {
	routes := updatesRoutes()
	routes[updateWindowsRoute] = route{Status: 403, File: "error_403.json"}
	api := newFakeAPI(t, routes)
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "get", "settings", "--schema", schemaOneAgentUpdates)
	if r.Code != 0 || !strings.Contains(rowOf(t, r.Stdout, "oa-env"), "uw-weekly") || !strings.Contains(r.Stderr, "update window names unavailable") {
		t.Errorf("%s", r)
	}
}

// TestWindowNamesAcrossEnvironments pins that the names looked up by
// environments fetched in parallel reach every environment's table.
func TestWindowNamesAcrossEnvironments(t *testing.T) {
	a, b := newFakeAPI(t, updatesRoutes()), newFakeAPI(t, updatesRoutes())
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b}), "get", "settings", "--schema", schemaOneAgentUpdates, "--env", "a;b")
	if r.Code != 0 || strings.Count(r.Stdout, "Weekly maintenance") != 2 {
		t.Errorf("want the name in both sections:\n%s", r)
	}
}
