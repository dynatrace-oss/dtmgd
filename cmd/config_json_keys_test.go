package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

const keysConfig = `apiVersion: dtmgd.io/v1
kind: Config
current-context: a
contexts:
- name: a
  context:
    host: https://cluster.example.com
    env-id: env1
    token-ref: a-token
- name: c
  context:
    host: https://cluster.example.com
    token-ref: c-token
tokens:
- name: a-token
  token: dt0c01.PLAIN.SECRET
`

// assertKeys fails unless m has every key in want.
func assertKeys(t *testing.T, what string, m map[string]any, want ...string) {
	t.Helper()
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("%s has no %q key: %v", what, k, m)
		}
	}
}

// configViewJSON returns config view's object: from -o json, or from the
// agent envelope's result.
func configViewJSON(t *testing.T, agent bool) map[string]any {
	t.Helper()
	cfg := withTempConfig(t, keysConfig)
	var r cliResult
	if agent {
		r = runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "config", "view")
	} else {
		r = runCLI(t, cfg, "config", "view", "-o", "json")
	}
	if strings.Contains(r.Stdout, "PLAIN.SECRET") {
		t.Fatal("config view printed the token")
	}
	var v map[string]any
	if r.Code != 0 || json.Unmarshal([]byte(r.Stdout), &v) != nil {
		t.Fatalf("config view: %s", r)
	}
	if agent {
		v, _ = v["result"].(map[string]any)
	}
	return v
}

// TestConfigViewUsesTheConfigFileKeys pins that config view's JSON (and the
// agent envelope's result) is keyed as the config file is: current-context,
// env-id, token-ref. It used Go field names (CurrentContext, EnvID).
func TestConfigViewUsesTheConfigFileKeys(t *testing.T) {
	for _, agent := range []bool{false, true} {
		v := configViewJSON(t, agent)
		assertKeys(t, "config", v, "apiVersion", "kind", "current-context", "contexts", "tokens")
		contexts, _ := v["contexts"].([]any)
		if len(contexts) != 2 {
			t.Fatalf("contexts = %v", v["contexts"])
		}
		nc, _ := contexts[0].(map[string]any)
		assertKeys(t, "context entry", nc, "name", "context")
		c, _ := nc["context"].(map[string]any)
		assertKeys(t, "context", c, "host", "env-id", "token-ref")
		cluster, _ := contexts[1].(map[string]any)["context"].(map[string]any)
		if _, ok := cluster["env-id"]; ok {
			t.Errorf("a cluster context has an env-id key, as in the file it should not: %v", cluster)
		}
	}
}

// TestContextListUsesConfigFileKeys pins ctx's and get-contexts' JSON:
// config-file keys, and current as a boolean rather than the table's "*".
func TestContextListUsesConfigFileKeys(t *testing.T) {
	cfg := withTempConfig(t, keysConfig)
	for _, args := range [][]string{{"ctx", "-o", "json"}, {"config", "get-contexts", "-o", "json"}} {
		r := runCLI(t, cfg, args...)
		var rows []map[string]any
		if r.Code != 0 || json.Unmarshal([]byte(r.Stdout), &rows) != nil || len(rows) != 2 {
			t.Fatalf("%v: %s", args, r)
		}
		assertKeys(t, "row", rows[0], "current", "name", "scope", "host", "env-id", "token-ref")
		if rows[0]["current"] != true || rows[1]["current"] != false {
			t.Errorf("%v: current = %v, %v; want true, false", args, rows[0]["current"], rows[1]["current"])
		}
	}
	r := runCLI(t, cfg, "ctx")
	if !strings.Contains(r.Stdout, "CURRENT") || !strings.Contains(r.Stdout, "*") {
		t.Errorf("the table lost its CURRENT * column:\n%s", r.Stdout)
	}
}
