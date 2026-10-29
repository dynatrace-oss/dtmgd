package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// mixedFake is a config with environment context "a" and cluster context "c",
// c current unless current says otherwise, and the fakes behind them.
func mixedFake(t *testing.T, current string, envRoutes map[string]route) (cfg string, env, cluster *fakeAPI) {
	t.Helper()
	env = newFakeAPI(t, envRoutes)
	cluster = newFakeAPI(t, map[string]route{})
	return withFakeContexts(t, current, []fakeEnv{{"a", env}}, []fakeEnv{{"c", cluster}}), env, cluster
}

// TestEnvCommandRefusesClusterContext pins that an environment command run
// against a cluster context fails before any request, naming the context,
// why, and the environment contexts to use instead.
func TestEnvCommandRefusesClusterContext(t *testing.T) {
	cfg, env, cluster := mixedFake(t, "c", nil)
	r := runCLI(t, cfg, "get", "problems")
	if r.Code != 2 {
		t.Errorf("code = %d, want 2", r.Code)
	}
	for _, want := range []string{`context "c" is a cluster context (no env-id); "dtmgd get problems" needs an environment context`, "Environment contexts: a."} {
		if !strings.Contains(r.Stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.Stderr)
		}
	}
	if env.totalHits()+cluster.totalHits() != 0 {
		t.Error("a request was sent")
	}
}

// TestEnvCommandRefusesClusterContextInEnvList pins the same for a cluster
// context named in an --env list.
func TestEnvCommandRefusesClusterContextInEnvList(t *testing.T) {
	cfg, env, cluster := mixedFake(t, "a", nil)
	r := runCLI(t, cfg, "get", "problems", "--env", "a;c")
	if r.Code != 2 || !strings.Contains(r.Stderr, `context "c" is a cluster context`) {
		t.Errorf("code %d, stderr:\n%s", r.Code, r.Stderr)
	}
	if env.totalHits()+cluster.totalHits() != 0 {
		t.Error("a request was sent")
	}
}

// TestAllEnvironmentsIgnoresClusterContexts pins that ALL_ENVIRONMENTS fans
// out to environment contexts only: a config holding both kinds (as the
// owner's does) used to fail partly with "env-id is required".
func TestAllEnvironmentsIgnoresClusterContexts(t *testing.T) {
	cfg, _, cluster := mixedFake(t, "a", map[string]route{problemsPath: {File: "problems_small.json"}})
	r := runCLI(t, cfg, "get", "problems", "--env", "ALL_ENVIRONMENTS", "-o", "json")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr:\n%s", r.Code, r.Stderr)
	}
	var byEnv map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.Stdout), &byEnv); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%s", err, r.Stdout)
	}
	var keys []string
	for k := range byEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "a" {
		t.Errorf("keys = %v, want [a]", keys)
	}
	if cluster.totalHits() != 0 {
		t.Error("the cluster context was called")
	}
}

// TestWrongContextAgentEnvelope pins the agent's view of a wrong-kind
// context: a validation error whose suggestion names the contexts to use.
func TestWrongContextAgentEnvelope(t *testing.T) {
	cfg, _, _ := mixedFake(t, "c", nil)
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "get", "problems")
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("stdout is not one envelope: %v\n%s", err, r.Stdout)
	}
	if resp.OK || resp.Error == nil || resp.Error.Code != "validation_error" {
		t.Fatalf("envelope = %+v", resp)
	}
	if !strings.Contains(strings.Join(resp.Error.Suggestions, "\n"), "Environment contexts: a.") {
		t.Errorf("suggestions = %q", resp.Error.Suggestions)
	}
}

// TestWrongContextNoneOfTheOtherKind pins the suggestion when no environment
// context exists at all: how to create one.
func TestWrongContextNoneOfTheOtherKind(t *testing.T) {
	cluster := newFakeAPI(t, map[string]route{})
	cfg := withFakeContexts(t, "c", nil, []fakeEnv{{"c", cluster}})
	r := runCLI(t, cfg, "get", "problems")
	if r.Code != 2 || !strings.Contains(r.Stderr, "No environment contexts are configured. Create one with: dtmgd ctx set <name> --host <url> --env-id <id> --token-ref <ref>") {
		t.Errorf("code %d, stderr:\n%s", r.Code, r.Stderr)
	}
}

// TestCtxSetWithoutEnvIDCreatesClusterContext pins that a new context without
// --env-id is a cluster context, said so on stderr, rather than an error.
func TestCtxSetWithoutEnvIDCreatesClusterContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	r := runCLI(t, path, "ctx", "set", "c", "--host", "https://cluster.example.com", "--token-ref", "t")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr:\n%s", r.Code, r.Stderr)
	}
	if !strings.Contains(r.Stderr, `"c" is a cluster context (no --env-id): use it with cluster commands, e.g. dtmgd get environments --context c`) {
		t.Errorf("stderr:\n%s", r.Stderr)
	}
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	nc, err := cfg.GetContext("c")
	if err != nil || !nc.Context.IsCluster() {
		t.Errorf("context c = %+v, %v", nc, err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "env-id") {
		t.Errorf("saved config writes an empty env-id:\n%s", raw)
	}
}

// TestCtxListShowsScope pins the SCOPE column of the context list.
func TestCtxListShowsScope(t *testing.T) {
	cfg, _, _ := mixedFake(t, "a", nil)
	r := runCLI(t, cfg, "ctx")
	if r.Code != 0 || !strings.Contains(r.Stdout, "SCOPE") {
		t.Fatalf("code %d, stdout:\n%s", r.Code, r.Stdout)
	}
	for _, line := range strings.Split(r.Stdout, "\n") {
		fields := strings.Fields(strings.TrimPrefix(line, "*"))
		if len(fields) < 2 {
			continue
		}
		want := map[string]string{"a": "environment", "c": "cluster"}[fields[0]]
		if want != "" && fields[1] != want {
			t.Errorf("row %q: scope %q, want %q", line, fields[1], want)
		}
	}
}
