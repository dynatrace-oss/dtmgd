package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

const (
	deletePrefix = "DELETE /e/env1/api/v2/settings/objects/"
	// referencesPath is the listing of the schemas that reference an update
	// window, the IDs sorted and comma-joined as the check sends them.
	referencesPath = objectsPath + " schemaIds=" + schemaActiveGateUpdates + "," + schemaOneAgentUpdates
	notifPath      = objectsPath + " schemaIds=builtin:problem.notifications"
	envSchemas     = "/e/env1/api/v2/settings/schemas"
	refAlerting    = `{"properties":{"alertingProfile":{"type":"setting","referencedType":"builtin:alerting.profile"}}}`
	refWindow      = `{"types":{"W":{"properties":{"w":{"type":"list","items":{"type":"setting","referencedType":"builtin:deployment.management.update-windows"}}}}}}`
)

// deleteCatalog is the schemas deleteFake serves: a notification that
// references alerting profiles, and the two update schemas that reference
// update windows.
func deleteCatalog() map[string]string {
	return map[string]string{
		"builtin:alerting.profile":      `{"multiObject":true}`,
		schemaUpdateWindows:             `{"multiObject":true}`,
		"builtin:problem.notifications": refAlerting,
		schemaOneAgentUpdates:           refWindow,
		schemaActiveGateUpdates:         refWindow,
	}
}

// windowObject is the GET body of update window id.
func windowObject(id string) route {
	return route{Body: `{"objectId":"` + id + `","schemaId":"builtin:deployment.management.update-windows","scope":"environment","summary":"Weekly maintenance","updateToken":"server-` + id + `","value":{"name":"Weekly maintenance","recurrence":"WEEKLY"}}`}
}

// deleteFake is a writable context holding alerting object obj-alerting-1
// and update window uw-weekly, whose references come from refs.
func deleteFake(t *testing.T, level, refs string, extra ...map[string]route) (*fakeAPI, string) {
	t.Helper()
	routes := map[string]route{
		objPrefix + "obj-alerting-1":    alertingObject("obj-alerting-1", "Default"),
		objPrefix + "uw-weekly":         windowObject("uw-weekly"),
		deletePrefix + "obj-alerting-1": {Body: ""},
		deletePrefix + "uw-weekly":      {Body: ""},
		referencesPath:                  {Body: refs},
		notifPath:                       {Body: `{"items":[],"totalCount":0}`},
	}
	maps.Copy(routes, schemaCatalog(envSchemas, deleteCatalog()))
	for _, e := range extra {
		maps.Copy(routes, e)
	}
	if level == "" {
		api := newFakeAPI(t, routes)
		return api, withFakeEnvs(t, fakeEnv{"a", api})
	}
	return applyFake(t, level, routes)
}

func TestDeleteNeedsYesWithoutATerminal(t *testing.T) {
	api, cfg := deleteFake(t, "readwrite-all", "")
	if r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1"); r.Code != 2 || !strings.Contains(r.Stderr, "--yes") {
		t.Errorf("no terminal: %s", r)
	}
	if r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "delete", "settings", "obj-alerting-1"); r.Code != 2 {
		t.Errorf("agent mode: %s", r)
	}
	if api.Hits(deletePrefix+"obj-alerting-1") != 0 {
		t.Error("a delete was sent")
	}
}

func TestDeleteSendsTheUpdateToken(t *testing.T) {
	api, cfg := deleteFake(t, "readwrite-all", "")
	r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--yes", "-o", "json")
	rep := decodeReport(t, r.Stdout)
	qs := api.Queries(deletePrefix + "obj-alerting-1")
	if r.Code != 0 || len(qs) != 1 || qs[0].Get("updateToken") != "server-obj-alerting-1" || actions(rep) != "deleted" {
		t.Errorf("%s", r)
	}
}

func TestDeleteRefusesAReferencedWindow(t *testing.T) {
	refs := `{"items":[{"objectId":"oa-group","schemaId":"builtin:deployment.oneagent.updates","scope":"HOST_GROUP-0000000000000001","value":{"updateMode":"AUTOMATIC_DURING_MW","maintenanceWindows":[{"maintenanceWindow":"uw-weekly"}]}}],"totalCount":1}`
	api, cfg := deleteFake(t, "readwrite-all", refs)
	r := runCLI(t, cfg, "delete", "settings", "uw-weekly", "--yes")
	if r.Code != 1 || !strings.Contains(r.Stderr, schemaOneAgentUpdates) || !strings.Contains(r.Stderr, "HOST_GROUP-0000000000000001") || api.Hits(deletePrefix+"uw-weekly") != 0 {
		t.Errorf("%s", r)
	}
}

// refNotification is a listed notification object that references profile id.
func refNotification(id, profile string) string {
	return fmt.Sprintf(`{"objectId":%q,"schemaId":"builtin:problem.notifications","scope":"environment","summary":"Mail %s","value":{"alertingProfile":%q}}`, id, id, profile)
}

func refList(items ...string) route {
	return route{Body: `{"items":[` + strings.Join(items, ",") + `],"totalCount":` + fmt.Sprint(len(items)) + `}`}
}

func TestDeleteRefusesAReferencedObject(t *testing.T) {
	for _, flags := range [][]string{{"--yes"}, {"--dry-run"}} {
		api, cfg := deleteFake(t, "readwrite-all", "", map[string]route{notifPath: refList(refNotification("notif-1", "obj-alerting-1"))})
		r := runCLI(t, cfg, append([]string{"delete", "settings", "obj-alerting-1"}, flags...)...)
		for _, want := range []string{"builtin:problem.notifications", "notif-1", "Mail notif-1", "still used by 1 settings object", "outside Settings 2.0 are not checked"} {
			if !strings.Contains(r.Stderr, want) {
				t.Errorf("%v: stderr lacks %q: %s", flags, want, r)
			}
		}
		if r.Code != 1 || api.Hits(deletePrefix+"obj-alerting-1") != 0 {
			t.Errorf("%v: %s", flags, r)
		}
	}
}

func TestDeleteUnreferencedObject(t *testing.T) {
	api, cfg := deleteFake(t, "readwrite-all", "", map[string]route{notifPath: refList(refNotification("notif-1", "someone-else"))})
	if r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--yes"); r.Code != 0 || api.Hits(deletePrefix+"obj-alerting-1") != 1 {
		t.Errorf("%s", r)
	}
}

func TestDeleteNoReferencingSchemaListsNothing(t *testing.T) {
	defs := deleteCatalog()
	delete(defs, "builtin:problem.notifications")
	api, cfg := deleteFake(t, "readwrite-all", "", schemaCatalog(envSchemas, defs))
	if r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--yes"); r.Code != 0 || api.Hits(objectsPath) != 0 {
		t.Errorf("listing hits %d: %s", api.Hits(objectsPath), r)
	}
}

func TestDeleteFindsAReferenceOnALaterPage(t *testing.T) {
	page1 := `{"items":[` + refNotification("notif-1", "other") + `],"nextPageKey":"pk2","totalCount":2}`
	page2 := refList(refNotification("notif-2", "obj-alerting-1"))
	api, cfg := deleteFake(t, "readwrite-all", "", map[string]route{notifPath: {Body: page1}, objectsPath + " nextPageKey=pk2": page2})
	if r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--yes"); r.Code != 1 || !strings.Contains(r.Stderr, "notif-2") || api.Hits(deletePrefix+"obj-alerting-1") != 0 {
		t.Errorf("%s", r)
	}
}

func TestDeleteRefusalListsAtMostTen(t *testing.T) {
	var items []string
	for i := 1; i <= 12; i++ {
		items = append(items, refNotification(fmt.Sprintf("notif-%02d", i), "obj-alerting-1"))
	}
	_, cfg := deleteFake(t, "readwrite-all", "", map[string]route{notifPath: refList(items...)})
	r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--yes")
	if r.Code != 1 || !strings.Contains(r.Stderr, "used by 12 settings") || strings.Count(r.Stderr, "(notif-") != 10 || !strings.Contains(r.Stderr, ", …") {
		t.Errorf("%s", r)
	}
}

func TestDeleteSkipsAnUnreadableDefinition(t *testing.T) {
	routes := map[string]route{"GET " + envSchemas + "/builtin:unreadable": {Status: 404, Body: `{"error":{"code":404,"message":"gone"}}`}}
	defs := deleteCatalog()
	defs["builtin:unreadable"] = "{}"
	catalog := schemaCatalog(envSchemas, defs)
	maps.Copy(catalog, routes)
	api, cfg := deleteFake(t, "readwrite-all", "", catalog)
	r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--yes", "-o", "json")
	rep := decodeReport(t, r.Stdout)
	if r.Code != 0 || api.Hits(deletePrefix+"obj-alerting-1") != 1 || len(rep.Warnings) != 1 ||
		!strings.Contains(rep.Warnings[0], "schema builtin:unreadable could not be read (404); the delete check skipped it") {
		t.Errorf("%+v\n%s", rep.Warnings, r)
	}
	if r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--yes"); !strings.Contains(r.Stderr, "builtin:unreadable") {
		t.Errorf("no warning on stderr: %s", r)
	}
}

func TestDeleteFailsWhenTheScanFails(t *testing.T) {
	denied := `{"error":{"code":403,"message":"denied"}}`
	badReq := `{"error":{"code":400,"message":"nope"}}`
	cases := map[string]struct {
		key    string
		status int
		body   string
		hint   string
	}{
		"listing 400":    {notifPath, 400, badReq, ""},
		"listing 403":    {notifPath, 403, denied, "Hint: Checking which settings use this object needs settings.read."},
		"definition 400": {"GET " + envSchemas + "/builtin:problem.notifications", 400, badReq, ""},
		"definition 403": {"GET " + envSchemas + "/builtin:problem.notifications", 403, denied, "Hint: " + hintReadSchemas},
	}
	for name, tc := range cases {
		api, cfg := deleteFake(t, "readwrite-all", "", map[string]route{tc.key: {Status: tc.status, Body: tc.body}})
		r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--yes")
		if r.Code == 0 || api.Hits(deletePrefix+"obj-alerting-1") != 0 || !strings.Contains(r.Stderr, tc.hint) {
			t.Errorf("%s: %s", name, r)
		}
	}
}

func TestDeleteAgentModeCarriesTheWarnings(t *testing.T) {
	defs := deleteCatalog()
	defs["builtin:unreadable"] = "{}"
	catalog := schemaCatalog(envSchemas, defs)
	catalog["GET "+envSchemas+"/builtin:unreadable"] = route{Status: 404, Body: `{"error":{"code":404,"message":"gone"}}`}
	_, cfg := deleteFake(t, "readwrite-all", "", catalog)
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "delete", "settings", "obj-alerting-1", "--yes")
	var env struct {
		Context struct {
			Warnings []string `json:"warnings"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &env); err != nil || r.Code != 0 || len(env.Context.Warnings) != 1 || !strings.Contains(env.Context.Warnings[0], "builtin:unreadable") {
		t.Errorf("%v\n%s", err, r)
	}
}

func TestDeleteUnreferencedWindow(t *testing.T) {
	refs := `{"items":[{"objectId":"ag-env","schemaId":"builtin:deployment.activegate.updates","scope":"environment","value":{"updateMode":"AUTOMATIC_DURING_UW","updateWindows":[{"updateWindow":"uw-daily"}]}}],"totalCount":1}`
	api, cfg := deleteFake(t, "readwrite-all", refs)
	if r := runCLI(t, cfg, "delete", "settings", "uw-weekly", "--yes"); r.Code != 0 || api.Hits(deletePrefix+"uw-weekly") != 1 {
		t.Errorf("%s", r)
	}
}

func TestDeleteDryRun(t *testing.T) {
	api, cfg := deleteFake(t, "", "")
	r := runCLI(t, cfg, "delete", "settings", "obj-alerting-1", "--dry-run", "-o", "json")
	if rep := decodeReport(t, r.Stdout); r.Code != 0 || actions(rep) != "delete" || api.Hits(deletePrefix+"obj-alerting-1") != 0 {
		t.Errorf("%s", r)
	}
}

func TestDeleteOneContextOnly(t *testing.T) {
	a, b := newFakeAPI(t, nil), newFakeAPI(t, nil)
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-all", "b": "readwrite-all"}, fakeEnv{"a", a}, fakeEnv{"b", b})
	if r := runCLI(t, cfg, "delete", "settings", "x", "--yes", "--env", "a;b"); r.Code != 2 || a.totalHits()+b.totalHits() != 0 {
		t.Errorf("%s", r)
	}
}

func TestConfirmDeletePrompt(t *testing.T) {
	for _, v := range agentmode.EnvVars() {
		t.Setenv(v, "")
	}
	for input, want := range map[string]error{"y\n": nil, "yes\n": nil, "n\n": errCancelled, "": errCancelled} {
		a := newTestApp("")
		a.interactive = func() bool { return true }
		a.stdin = strings.NewReader(input)
		var errBuf bytes.Buffer
		a.Err = &errBuf
		if err := a.confirmDelete("Delete it?", false); !errors.Is(err, want) {
			t.Errorf("input %q: err %v, want %v", input, err, want)
		}
		if !strings.Contains(errBuf.String(), "Delete it? [y/N]") {
			t.Errorf("input %q: prompt %q", input, errBuf.String())
		}
	}
}
