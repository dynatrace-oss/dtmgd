package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	agList       = "GET /api/cluster/v2/activeGates"
	agGlobal     = "GET /api/cluster/v2/activeGates/autoUpdate"
	agOnePrefix  = "GET /api/cluster/v2/activeGates/"
	agGlobalFile = "cluster_ag_autoupdate_global.json"
)

// agRoutes serves the cluster's ActiveGates and the global auto-update
// configuration from globalFile.
func agRoutes(globalFile string) map[string]route {
	return map[string]route{
		agList:                          {File: "cluster_activegates.json"},
		agGlobal:                        {File: globalFile},
		agOnePrefix + "ag-1":            {Body: `{"id":"ag-1","type":"CLUSTER"}`},
		agOnePrefix + "ag-1/autoUpdate": {Body: `{"setting":"INHERITED","effectiveSetting":"ENABLED"}`},
		agOnePrefix + "ag-2/autoUpdate": {Body: `{"setting":"DISABLED","effectiveSetting":"DISABLED"}`},
	}
}

func TestActiveGateUpdatesTable(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", agRoutes(agGlobalFile))
	r := runCLI(t, cfg, "get", "activegate-updates")
	lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
	if r.Code != 0 || len(lines) != 4 || !strings.Contains(lines[1], "(global)") {
		t.Fatalf("want a header, the global row, then two ActiveGates:\n%s", r)
	}
	if !strings.Contains(lines[1], "—") {
		t.Errorf("global row without the flag shows target version and windows: %q", lines[1])
	}
	assertGolden(t, "get_activegate_updates_table", r.String())
}

func TestActiveGateUpdatesFlagged(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", agRoutes("cluster_ag_autoupdate_global_flagged.json"))
	r := runCLI(t, cfg, "get", "activegate-updates")
	if row := rowOf(t, r.Stdout, "(global)"); !strings.Contains(row, "latest") || !strings.Contains(row, "Cluster window") {
		t.Errorf("global row %q", row)
	}
}

// TestActiveGateUpdatesListsClusterActiveGates pins that the list asks for
// the cluster's own ActiveGates only.
func TestActiveGateUpdatesListsClusterActiveGates(t *testing.T) {
	cfg, _, api := clusterFake(t, "c", agRoutes(agGlobalFile))
	runCLI(t, cfg, "get", "activegate-updates")
	if qs := api.Queries(agList); len(qs) != 1 || qs[0].Get("type") != "CLUSTER" {
		t.Errorf("queries %v", qs)
	}
}

func TestActiveGateUpdatesByID(t *testing.T) {
	cfg, _, api := clusterFake(t, "c", agRoutes(agGlobalFile))
	r := runCLI(t, cfg, "get", "activegate-updates", "ag-1")
	if r.Code != 0 || api.Hits(agOnePrefix+"ag-1/autoUpdate") != 1 || dataRows(r.Stdout) != 1 || !strings.Contains(r.Stdout, "INHERITED") {
		t.Errorf("%s", r)
	}
}

func TestActiveGateUpdatesUnknownID(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", agRoutes(agGlobalFile))
	r := runCLI(t, cfg, "get", "activegate-updates", "ag-nope")
	if r.Code != 4 || !strings.Contains(r.Stderr, "get activegate-updates") {
		t.Errorf("%s", r)
	}
}

func TestActiveGateUpdatesNeedACluster(t *testing.T) {
	cfg, env, cluster := clusterFake(t, "a", agRoutes(agGlobalFile))
	if r := runCLI(t, cfg, "get", "activegate-updates"); r.Code != 2 || env.totalHits()+cluster.totalHits() != 0 {
		t.Errorf("%s", r)
	}
}

func TestActiveGateUpdatesJSONShape(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", agRoutes(agGlobalFile))
	r := runCLI(t, cfg, "get", "activegate-updates", "-o", "json")
	var body struct {
		Global      map[string]interface{}   `json:"global"`
		ActiveGates []map[string]interface{} `json:"activeGates"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &body); err != nil || body.Global["globalSetting"] != "ENABLED" || len(body.ActiveGates) != 2 {
		t.Errorf("%v\n%s", err, r)
	}
	if d := runCLI(t, cfg, "describe", "activegate-updates"); d.Code != 0 || !strings.Contains(d.Stdout, "globalSetting: ENABLED") {
		t.Errorf("describe: %s", d)
	}
}
