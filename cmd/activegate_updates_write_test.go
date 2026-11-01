package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	agGlobalPut       = "PUT /api/cluster/v2/activeGates/autoUpdate"
	agGlobalValidator = "POST /api/cluster/v2/activeGates/autoUpdate/validator"
	agOnePut          = "PUT /api/cluster/v2/activeGates/ag-1/autoUpdate"
)

// agWriteFake is a cluster context at level with the ActiveGate routes,
// writable ones included, and globalFile as the global configuration.
func agWriteFake(t *testing.T, level, globalFile string) (*fakeAPI, string) {
	t.Helper()
	routes := agRoutes(globalFile)
	routes[agGlobalPut] = route{Status: 204}
	routes[agGlobalValidator] = route{Status: 204}
	routes[agOnePut] = route{Status: 204}
	api, _, cfg := clusterSettingsFake(t, level, routes)
	return api, cfg
}

// agBody is the JSON body of the first request to key.
func agBody(t *testing.T, api *fakeAPI, key string) map[string]interface{} {
	t.Helper()
	bodies := api.Bodies(key)
	if len(bodies) == 0 {
		t.Fatalf("no request to %s", key)
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(bodies[0]), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestUpdateActiveGateUpdatesGlobal(t *testing.T) {
	api, cfg := agWriteFake(t, "readwrite-all", agGlobalFile)
	r := runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, "globalSetting: DISABLED\n"), "-o", "json")
	if rep := decodeReport(t, r.Stdout); r.Code != 0 || actions(rep) != "updated" || agBody(t, api, agGlobalPut)["globalSetting"] != "DISABLED" {
		t.Errorf("%s", r)
	}
}

func TestUpdateActiveGateUpdatesValidateOnly(t *testing.T) {
	api, cfg := agWriteFake(t, "readwrite-all", agGlobalFile)
	r := runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, "globalSetting: DISABLED\n"), "--validate-only", "-o", "json")
	if rep := decodeReport(t, r.Stdout); r.Code != 0 || actions(rep) != "validated" || api.Hits(agGlobalValidator) != 1 || api.Hits(agGlobalPut) != 0 || api.Hits(clusterVersionPath) != 0 {
		t.Errorf("%s", r)
	}
	routes := agRoutes(agGlobalFile)
	routes[agGlobalValidator] = route{Status: 400, Body: `{"error":{"code":400,"message":"Constraints violated.","constraintViolations":[{"path":"globalSetting","message":"invalid"}]}}`}
	_, _, cfg = clusterSettingsFake(t, "readwrite-all", routes)
	r = runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, "globalSetting: DISABLED\n"), "--validate-only", "-o", "json")
	if rep := decodeReport(t, r.Stdout); r.Code == 0 || actions(rep) != "failed" || len(rep.Results[0].Error.ConstraintViolations) != 1 {
		t.Errorf("400: %s", r)
	}
}

func TestUpdateActiveGateUpdatesReadonly(t *testing.T) {
	api, cfg := agWriteFake(t, "", agGlobalFile)
	if r := runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, "globalSetting: DISABLED\n")); r.Code != 6 || api.totalHits() != 0 {
		t.Errorf("%s", r)
	}
}

// TestUpdateRefusesUnhonouredFields pins that a field the cluster would
// drop silently is refused, and sent where the cluster honours it.
func TestUpdateRefusesUnhonouredFields(t *testing.T) {
	file := tempFile(t, "globalSetting: ENABLED\ntargetVersion: previous\n")
	api, cfg := agWriteFake(t, "readwrite-all", agGlobalFile)
	if r := runCLI(t, cfg, "update", "activegate-updates", "-f", file); r.Code != 2 || !strings.Contains(r.Stderr, "targetVersion") || api.Hits(agGlobalPut) != 0 {
		t.Errorf("without the flag: %s", r)
	}
	api, cfg = agWriteFake(t, "readwrite-all", "cluster_ag_autoupdate_global_flagged.json")
	if r := runCLI(t, cfg, "update", "activegate-updates", "-f", file); r.Code != 0 || agBody(t, api, agGlobalPut)["targetVersion"] != "previous" {
		t.Errorf("with the flag: %s", r)
	}
}

func TestGlobalAutoUpdateRefusesInherited(t *testing.T) {
	api, cfg := agWriteFake(t, "readwrite-all", agGlobalFile)
	for _, body := range []string{"globalSetting: INHERITED\n", "setting: DISABLED\n"} {
		if r := runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, body)); r.Code != 2 {
			t.Errorf("%q: %s", body, r)
		}
	}
	if api.Hits(agGlobalPut) != 0 {
		t.Error("an invalid global configuration was sent")
	}
}

func TestUpdateActiveGateUpdatesPerAG(t *testing.T) {
	api, cfg := agWriteFake(t, "readwrite-all", agGlobalFile)
	if r := runCLI(t, cfg, "update", "activegate-updates", "ag-1", "-f", tempFile(t, "setting: DISABLED\n")); r.Code != 0 || agBody(t, api, agOnePut)["setting"] != "DISABLED" {
		t.Errorf("%s", r)
	}
}

func TestUpdateActiveGateUpdatesUnchanged(t *testing.T) {
	api, cfg := agWriteFake(t, "readwrite-all", agGlobalFile)
	r := runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, "globalSetting: ENABLED\n"), "-o", "json")
	if rep := decodeReport(t, r.Stdout); r.Code != 0 || actions(rep) != "unchanged" || api.Hits(agGlobalPut) != 0 {
		t.Errorf("%s", r)
	}
}

func TestEditActiveGateUpdates(t *testing.T) {
	editorScript(t, `globalSetting: DISABLED\n`)
	api, cfg := agWriteFake(t, "readwrite-all", agGlobalFile)
	if r := runInteractive(t, cfg, "", "edit", "activegate-updates"); r.Code != 0 || agBody(t, api, agGlobalPut)["globalSetting"] != "DISABLED" {
		t.Errorf("%s", r)
	}
	if r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "edit", "activegate-updates"); r.Code != 2 {
		t.Errorf("agent mode: %s", r)
	}
}

// clusterWindowFake is a writable cluster holding update window uw-c1,
// with globalFile as the ActiveGates' global auto-update configuration.
func clusterWindowFake(t *testing.T, globalFile string) (*fakeAPI, string) {
	t.Helper()
	routes := agRoutes(globalFile)
	routes[clusterObjPrefix+"uw-c1"] = route{Body: `{"objectId":"uw-c1","schemaId":"builtin:deployment.management.update-windows","scope":"cluster","updateToken":"t","value":{"name":"Cluster window"}}`}
	routes[clusterDelPrefix+"uw-c1"] = route{Body: ""}
	api, _, cfg := clusterSettingsFake(t, "readwrite-all", routes)
	return api, cfg
}

func TestDeleteRefusesAClusterWindowInUse(t *testing.T) {
	api, cfg := clusterWindowFake(t, "cluster_ag_autoupdate_global_flagged.json")
	if r := runCLI(t, cfg, "delete", "settings", "uw-c1", "--yes"); r.Code != 1 || !strings.Contains(r.Stderr, "global") || api.Hits(clusterDelPrefix+"uw-c1") != 0 {
		t.Errorf("%s", r)
	}
}

func TestDeleteClusterWindowWithoutTheFlag(t *testing.T) {
	api, cfg := clusterWindowFake(t, agGlobalFile)
	if r := runCLI(t, cfg, "delete", "settings", "uw-c1", "--yes"); r.Code != 0 || api.Hits(clusterDelPrefix+"uw-c1") != 1 {
		t.Errorf("%s", r)
	}
}
