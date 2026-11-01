package cmd

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"
)

const flaggedGlobal = "cluster_ag_autoupdate_global_flagged.json"

// TestAGUpdateDropsWindowNames pins that an exported flagged configuration
// applies back: window names are read-only, so they are not sent, and the
// same windows (by id) are no change.
func TestAGUpdateDropsWindowNames(t *testing.T) {
	export := "globalSetting: DISABLED\ntargetVersion: latest\nupdateWindows: {windows: [{id: uw-c1, name: Cluster window}]}\n"
	api, cfg := agWriteFake(t, "readwrite-all", flaggedGlobal)
	if r := runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, export)); r.Code != 0 {
		t.Fatalf("%s", r)
	}
	windows := agBody(t, api, agGlobalPut)["updateWindows"].(map[string]interface{})["windows"].([]interface{})
	if w := windows[0].(map[string]interface{}); w["id"] != "uw-c1" || w["name"] != nil {
		t.Errorf("PUT window %v, want the id without the name", w)
	}
	same := strings.Replace(export, "DISABLED", "ENABLED", 1)
	r := runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, same), "-o", "json")
	if rep := decodeReport(t, r.Stdout); actions(rep) != "unchanged" {
		t.Errorf("the same configuration: %s", r)
	}
}

// TestAGUpdateRefusesDroppingWindows pins that a file leaving out the
// update windows of a cluster that honours them is refused: the cluster
// would clear them.
func TestAGUpdateRefusesDroppingWindows(t *testing.T) {
	api, cfg := agWriteFake(t, "readwrite-all", flaggedGlobal)
	r := runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, "globalSetting: ENABLED\ntargetVersion: \"1.350\"\n"))
	if r.Code != 2 || !strings.Contains(r.Stderr, "updateWindows") || api.Hits(agGlobalPut) != 0 {
		t.Errorf("%s", r)
	}
}

// TestClusterSettings404KeepsTheirHint pins that a settings 404 on a
// cluster context keeps its own next step, not only "wrong host".
func TestClusterSettings404KeepsTheirHint(t *testing.T) {
	_, _, cfg := clusterSettingsFake(t, "readwrite-all", nil)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"describe", "settings", "bad"}, "get settings"},
		{[]string{"describe", "settings-schema", "builtin:bad"}, "settings-schemas --search"},
		{[]string{"delete", "settings", "bad", "--yes"}, "get settings"},
		{[]string{"create", "settings", "-f", tempFile(t, "a: 1\n"), "--schema", "builtin:bad", "--scope", "cluster"}, "settings-schemas --search"},
	} {
		if r := runCLI(t, cfg, c.args...); r.Code != 4 || !strings.Contains(r.Stderr, c.want) {
			t.Errorf("%v: %s", c.args[:2], r)
		}
	}
}

// TestClusterContextWithEnvIsRefused pins that --context <cluster> with a
// multi-environment --env, or a cluster context inside --env, is refused
// before any request, saying why.
func TestClusterContextWithEnvIsRefused(t *testing.T) {
	api, env, cfg := clusterSettingsFake(t, "readwrite-all", createRoutes())
	file := tempFile(t, "schemaId: builtin:test.multi\nscope: environment\nvalue: {name: X}\n")
	for _, args := range [][]string{
		{"get", "settings", "--scope", "environment", "--context", "c", "--env", "a;b"},
		{"apply", "-f", file, "--context", "c", "--env", "a;b"},
	} {
		if r := runCLI(t, cfg, args...); r.Code != 2 {
			t.Errorf("%v: %s", args[:2], r)
		}
	}
	if r := runCLI(t, cfg, "get", "settings", "--scope", "cluster", "--env", "a;c"); r.Code != 2 || strings.Contains(r.Stderr, "needs an environment context") {
		t.Errorf("--env with a cluster context: %s", r)
	}
	if api.totalHits()+env.totalHits() != 0 {
		t.Errorf("%d requests, want none", api.totalHits()+env.totalHits())
	}
}

func TestValidateGateHintOnCluster(t *testing.T) {
	routes := map[string]route{
		clusterWindowDef:   {Body: `{"multiObject":true}`},
		clusterObjects:     {Body: `{"items":[],"totalCount":0}`},
		clusterVersionPath: {Status: 403, File: "error_403.json"},
	}
	_, _, cfg := clusterSettingsFake(t, "readwrite-all", routes)
	file := tempFile(t, "schemaId: "+schemaUpdateWindows+"\nscope: cluster\nvalue: {name: X}\n")
	if r := runCLI(t, cfg, "apply", "-f", file, "--validate-only"); r.Code != 2 || !strings.Contains(r.Stderr, "ServiceProviderAPI") || strings.Contains(r.Stderr, "DataExport") {
		t.Errorf("%s", r)
	}
}

func TestClusterWindowDelete403NamesServiceProviderAPI(t *testing.T) {
	routes := agRoutes(flaggedGlobal)
	routes[agGlobal] = route{Status: 403, File: "error_403.json"}
	routes[clusterObjPrefix+"uw-c1"] = route{Body: `{"objectId":"uw-c1","schemaId":"builtin:deployment.management.update-windows","scope":"cluster","value":{}}`}
	maps.Copy(routes, schemaCatalog(clusterSchemaPrefix, map[string]string{schemaUpdateWindows: "{}"}))
	_, _, cfg := clusterSettingsFake(t, "readwrite-all", routes)
	if r := runCLI(t, cfg, "delete", "settings", "uw-c1", "--yes"); r.Code != 5 || !strings.Contains(r.Stderr, "ServiceProviderAPI") {
		t.Errorf("%s", r)
	}
}

// TestAGUpdateFileShape pins that a file that is not one configuration is
// refused before anything is sent, and that an empty ID is not the global
// configuration.
func TestAGUpdateFileShape(t *testing.T) {
	api, cfg := agWriteFake(t, "readwrite-all", agGlobalFile)
	body, _ := json.Marshal(map[string]interface{}{"global": map[string]string{"globalSetting": "ENABLED"}, "activeGates": []interface{}{}})
	for _, file := range []string{"{}\n", string(body), "globalSetting: ENABLED\nsurprise: 1\n"} {
		if r := runCLI(t, cfg, "update", "activegate-updates", "-f", tempFile(t, file)); r.Code != 2 {
			t.Errorf("%q: %s", file, r)
		}
	}
	for _, args := range [][]string{{"get", "activegate-updates", ""}, {"describe", "activegate-updates", ""}} {
		if r := runCLI(t, cfg, args...); r.Code != 2 {
			t.Errorf("%v with an empty ID: %s", args[:2], r)
		}
	}
	if api.Hits(agGlobalPut) != 0 {
		t.Error("a malformed configuration was sent")
	}
}

// TestClusterWindowCheckSkipsOddEntries pins that a list entry that is not
// an object does not crash the reference check.
func TestClusterWindowCheckSkipsOddEntries(t *testing.T) {
	routes := agRoutes(flaggedGlobal)
	routes[agList] = route{Body: `{"activeGates":[42]}`}
	routes[agGlobal] = route{Body: `{"globalSetting":"ENABLED","updateWindows":{"windows":[]}}`}
	routes[clusterObjPrefix+"uw-c1"] = route{Body: `{"objectId":"uw-c1","schemaId":"builtin:deployment.management.update-windows","scope":"cluster","value":{}}`}
	routes[clusterDelPrefix+"uw-c1"] = route{Body: ""}
	maps.Copy(routes, schemaCatalog(clusterSchemaPrefix, map[string]string{schemaUpdateWindows: "{}"}))
	api, _, cfg := clusterSettingsFake(t, "readwrite-all", routes)
	if r := runCLI(t, cfg, "delete", "settings", "uw-c1", "--yes"); r.Code != 0 || api.Hits(clusterDelPrefix+"uw-c1") != 1 {
		t.Errorf("%s", r)
	}
}
