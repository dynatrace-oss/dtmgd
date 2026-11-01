package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestContextAndEnvCannotBeCombined pins that --context and --env, which
// both choose the contexts a command runs against, are refused together
// instead of one silently winning.
func TestContextAndEnvCannotBeCombined(t *testing.T) {
	a, b := newFakeAPI(t, nil), newFakeAPI(t, nil)
	cfg := withFakeEnvs(t, fakeEnv{"a", a}, fakeEnv{"b", b})
	for _, env := range []string{"a;b", "b", "ALL_ENVIRONMENTS"} {
		r := runCLI(t, cfg, "get", "problems", "--context", "a", "--env", env)
		if r.Code != 2 || !strings.Contains(r.Stderr, "--context") || !strings.Contains(r.Stderr, "--env") {
			t.Errorf("--env %q: %s", env, r)
		}
	}
	if a.totalHits()+b.totalHits() != 0 {
		t.Errorf("%d requests, want none", a.totalHits()+b.totalHits())
	}
	agent := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "get", "problems", "-c", "a", "--env", "a;b")
	if agent.Code != 2 || !strings.Contains(agent.Stdout, "validation_error") {
		t.Errorf("agent mode: %s", agent)
	}
}

// TestUpdateRefusesAnEnvironmentActiveGate pins that the cluster commands
// change cluster ActiveGates only: an environment ActiveGate's auto-update
// belongs to its environment's settings.
func TestUpdateRefusesAnEnvironmentActiveGate(t *testing.T) {
	routes := agRoutes(agGlobalFile)
	routes[agOnePrefix+"ag-env"] = route{Body: `{"id":"ag-env","type":"ENVIRONMENT"}`}
	routes[agOnePrefix+"ag-env/autoUpdate"] = route{Body: `{"setting":"INHERITED","effectiveSetting":"ENABLED"}`}
	routes["PUT /api/cluster/v2/activeGates/ag-env/autoUpdate"] = route{Status: 204}
	api, _, cfg := clusterSettingsFake(t, "readwrite-all", routes)
	r := runCLI(t, cfg, "update", "activegate-updates", "ag-env", "-f", tempFile(t, "setting: DISABLED\n"))
	if r.Code != 2 || !strings.Contains(r.Stderr, "ENVIRONMENT") || !strings.Contains(r.Stderr, schemaActiveGateUpdates) || api.Hits("PUT /api/cluster/v2/activeGates/ag-env/autoUpdate") != 0 {
		t.Errorf("%s", r)
	}
	if r := runCLI(t, cfg, "update", "activegate-updates", "ag-nope", "-f", tempFile(t, "setting: DISABLED\n")); r.Code != 4 || !strings.Contains(r.Stderr, "get activegate-updates") {
		t.Errorf("unknown ActiveGate: %s", r)
	}
}

// TestConfigViewShowsTheDefaultLevel pins that config view names the level
// of a context at the default, without writing it to the file.
func TestConfigViewShowsTheDefaultLevel(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)})
	r := runCLI(t, cfg, "config", "view", "-o", "json")
	var view struct {
		Contexts []struct {
			Context map[string]interface{} `json:"context"`
		} `json:"contexts"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &view); err != nil || len(view.Contexts) != 1 || view.Contexts[0].Context["safety-level"] != "readonly" {
		t.Errorf("%v\n%s", err, r)
	}
	if data, _ := os.ReadFile(cfg); strings.Contains(string(data), "safety-level") {
		t.Error("config view wrote the level to the file")
	}
	if table := runCLI(t, cfg, "config", "view"); !strings.Contains(table.Stdout, "safety-level: readonly") {
		t.Errorf("yaml view:\n%s", table.Stdout)
	}
}
