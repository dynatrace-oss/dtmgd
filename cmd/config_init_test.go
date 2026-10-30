package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

const twoContexts = `apiVersion: dtmgd.io/v1
kind: Config
current-context: a
contexts:
- name: a
  context:
    host: https://cluster.example.com
    env-id: env1
    token-ref: a-token
- name: b
  context:
    host: https://cluster.example.com
    env-id: env2
    token-ref: a-token
`

func TestConfigInitWritesATemplate(t *testing.T) {
	dir, cfg := inProject(t)
	r := runCLI(t, cfg, "config", "init")
	if r.Code != 0 || !strings.Contains(r.Stderr, "Created .dtmgd.yaml") {
		t.Fatalf("config init: %s", r)
	}
	got := readFile(t, filepath.Join(dir, config.LocalConfigName))
	for _, want := range []string{"current-context: my-cluster", "${DT_MANAGED_HOST}", "${DT_API_TOKEN}"} {
		if !strings.Contains(got, want) {
			t.Errorf("template lacks %q:\n%s", want, got)
		}
	}
}

func TestConfigInitRefusesToOverwriteWithoutForce(t *testing.T) {
	dir, cfg := inProject(t)
	runCLI(t, cfg, "config", "init")

	if r := runCLI(t, cfg, "config", "init"); r.Code != 1 || !strings.Contains(r.Stderr, "already exists") {
		t.Fatalf("second config init: %s", r)
	}
	if r := runCLI(t, cfg, "config", "init", "--force", "--context", "foo"); r.Code != 0 {
		t.Fatalf("config init --force: %s", r)
	}
	if got := readFile(t, filepath.Join(dir, config.LocalConfigName)); !strings.Contains(got, "current-context: foo") {
		t.Errorf("--context foo not in the template:\n%s", got)
	}
}

func TestConfigMigrateTokensMovesPlaintextTokensToTheKeyring(t *testing.T) {
	const secret = "dt0c01.MIGRATE.ME"
	cfg := withTempConfig(t, twoContexts+"tokens:\n- name: a-token\n  token: "+secret+"\n")
	t.Cleanup(func() { _ = config.NewTokenStore().DeleteToken("a-token") })

	r := runCLI(t, cfg, "config", "migrate-tokens")
	if r.Code != 0 || !strings.Contains(r.Stderr, "Migrated 1 token(s)") {
		t.Fatalf("migrate-tokens: %s", r)
	}
	if strings.Contains(readFile(t, cfg), secret) {
		t.Error("the token is still in the config file")
	}
	if got, err := config.NewTokenStore().GetToken("a-token"); err != nil || got != secret {
		t.Errorf("keyring token = %q, %v; want the migrated token", got, err)
	}
	if r := runCLI(t, cfg, "config", "migrate-tokens"); r.Code != 0 || !strings.Contains(r.Stderr, "No tokens to migrate") {
		t.Errorf("second migrate-tokens: %s", r)
	}
}

func TestConfigMigrateTokensSkipsVariableReferences(t *testing.T) {
	cfg := withTempConfig(t, twoContexts+"tokens:\n- name: a-token\n  token: ${DT_API_TOKEN}\n")
	before := readFile(t, cfg)

	r := runCLI(t, cfg, "config", "migrate-tokens")
	if r.Code != 0 || !strings.Contains(r.Stderr, `Skipped "a-token"`) || !strings.Contains(r.Stderr, "No tokens to migrate") {
		t.Fatalf("migrate-tokens with a ${VAR} token: %s", r)
	}
	if readFile(t, cfg) != before {
		t.Error("migrate-tokens rewrote a config it had nothing to migrate from")
	}
}

func TestCurrentContextCommands(t *testing.T) {
	cfg := withTempConfig(t, twoContexts)
	for _, args := range [][]string{{"config", "current-context"}, {"ctx", "current"}} {
		if r := runCLI(t, cfg, args...); r.Code != 0 || r.Stdout != "a\n" {
			t.Errorf("%v: %s", args, r)
		}
	}
}

// TestSaveConfigWritesTheLocalConfig pins that a command changing the config
// without --config saves to the .dtmgd.yaml it loaded, not to the global file.
func TestSaveConfigWritesTheLocalConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	local := filepath.Join(dir, config.LocalConfigName)
	if err := os.WriteFile(local, []byte(twoContexts), 0o600); err != nil {
		t.Fatal(err)
	}

	if r := runCLI(t, "", "config", "use-context", "b"); r.Code != 0 {
		t.Fatalf("use-context: %s", r)
	}
	if got := readFile(t, local); !strings.Contains(got, "current-context: b") {
		t.Errorf("local config not updated:\n%s", got)
	}
	if _, err := os.Stat(config.DefaultConfigPath()); !os.IsNotExist(err) {
		t.Errorf("use-context also wrote the global config (stat err %v)", err)
	}
}

// TestSaveConfigFallsBackToTheGlobalConfig pins the last branch of saveConfig:
// no --config, no DTMGD_CONFIG and no .dtmgd.yaml means the global config,
// which the test sandbox has moved into a temporary directory.
func TestSaveConfigFallsBackToTheGlobalConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	global := config.DefaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(global), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(global, []byte(twoContexts), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(global) })

	if r := runCLI(t, "", "config", "use-context", "b"); r.Code != 0 {
		t.Fatalf("use-context: %s", r)
	}
	if got := readFile(t, global); !strings.Contains(got, "current-context: b") {
		t.Errorf("global config not updated:\n%s", got)
	}
}
