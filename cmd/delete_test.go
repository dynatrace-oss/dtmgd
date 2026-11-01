package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

const (
	deletePrefix   = "DELETE /e/env1/api/v2/settings/objects/"
	referencesPath = objectsPath + " schemaIds=" + schemaOneAgentUpdates + "," + schemaActiveGateUpdates
)

// windowObject is the GET body of update window id.
func windowObject(id string) route {
	return route{Body: `{"objectId":"` + id + `","schemaId":"builtin:deployment.management.update-windows","scope":"environment","summary":"Weekly maintenance","updateToken":"server-` + id + `","value":{"name":"Weekly maintenance","recurrence":"WEEKLY"}}`}
}

// deleteFake is a writable context holding alerting object obj-alerting-1
// and update window uw-weekly, whose references come from refs.
func deleteFake(t *testing.T, level, refs string) (*fakeAPI, string) {
	t.Helper()
	routes := map[string]route{
		objPrefix + "obj-alerting-1":    alertingObject("obj-alerting-1", "Default"),
		objPrefix + "uw-weekly":         windowObject("uw-weekly"),
		deletePrefix + "obj-alerting-1": {Body: ""},
		deletePrefix + "uw-weekly":      {Body: ""},
		referencesPath:                  {Body: refs},
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
