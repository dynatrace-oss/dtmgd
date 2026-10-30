package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// inProject runs the test in a fresh project directory and returns it with an
// empty config. skills install writes relative to the working directory.
func inProject(t *testing.T) (dir, cfg string) {
	t.Helper()
	cfg = withTempConfig(t, "")
	dir = t.TempDir()
	t.Chdir(dir)
	return dir, cfg
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
}

func TestSkillsInstallForClaude(t *testing.T) {
	dir, cfg := inProject(t)
	r := runCLI(t, cfg, "skills", "install", "--for", "claude")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Installed Claude Code skill") || !strings.Contains(r.Stdout, "(project)") {
		t.Fatalf("install: %s", r)
	}
	assertFile(t, filepath.Join(dir, ".claude", "skills", "dtmgd", "SKILL.md"))
}

func TestSkillsInstallRefusesToOverwriteWithoutForce(t *testing.T) {
	_, cfg := inProject(t)
	runCLI(t, cfg, "skills", "install", "--for", "claude")

	r := runCLI(t, cfg, "skills", "install", "--for", "claude")
	if r.Code != 1 || !strings.Contains(r.Stderr, "--force") {
		t.Fatalf("second install without --force: %s", r)
	}
	r = runCLI(t, cfg, "skills", "install", "--for", "claude", "--force")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Updated Claude Code skill") {
		t.Fatalf("install --force: %s", r)
	}
}

func TestSkillsInstallGlobalGoesToTheHomeDirectory(t *testing.T) {
	_, cfg := inProject(t)
	r := runCLI(t, cfg, "skills", "install", "--for", "claude", "--global")
	if r.Code != 0 || !strings.Contains(r.Stdout, "(global)") {
		t.Fatalf("install --global: %s", r)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(home, ".claude", "skills", "dtmgd", "SKILL.md"))
	runCLI(t, cfg, "skills", "uninstall", "--for", "claude")
}

func TestSkillsInstallUsageErrors(t *testing.T) {
	_, cfg := inProject(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"cross-client with --for", []string{"--cross-client", "--for", "claude"}, "cannot be used together"},
		{"unknown agent", []string{"--for", "nope"}, "Supported agents: claude"},
	}
	for _, c := range cases {
		r := runCLI(t, cfg, append([]string{"skills", "install"}, c.args...)...)
		if r.Code != 1 || !strings.Contains(r.Stderr, c.want) {
			t.Errorf("%s: want exit 1 with %q, got %s", c.name, c.want, r)
		}
	}
}

// TestSkillsInstallNeedsForWithoutATerminal pins pentest note 5 end to end:
// an agent variable alone does not choose the directory when stdin is not a
// terminal.
func TestSkillsInstallNeedsForWithoutATerminal(t *testing.T) {
	if stdinIsTerminal() {
		t.Skip("stdin is a terminal; this test needs a non-interactive stdin")
	}
	dir, cfg := inProject(t)
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "skills", "install", "--no-agent")
	if r.Code != 1 || !strings.Contains(r.Stderr, "--for is required") {
		t.Fatalf("install without --for: %s", r)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude")); !os.IsNotExist(err) {
		t.Errorf("install without --for wrote .claude/ (stat err %v)", err)
	}
}

func TestSkillsInstallList(t *testing.T) {
	_, cfg := inProject(t)
	r := runCLI(t, cfg, "skills", "install", "--list")
	if r.Code != 0 {
		t.Fatalf("install --list: %s", r)
	}
	for _, name := range []string{"cross-client", "claude", "copilot", "cursor", "junie", "kiro", "opencode"} {
		if !strings.Contains(r.Stdout, name) {
			t.Errorf("install --list does not list %s:\n%s", name, r.Stdout)
		}
	}
}

func TestSkillsStatus(t *testing.T) {
	_, cfg := inProject(t)
	runCLI(t, cfg, "skills", "install", "--for", "claude")

	r := runCLI(t, cfg, "skills", "status", "--for", "claude", "-o", "json")
	var one []skillsStatusEntry
	if r.Code != 0 || json.Unmarshal([]byte(r.Stdout), &one) != nil || len(one) != 1 {
		t.Fatalf("status --for claude: %s", r)
	}
	if !one[0].Installed || one[0].Scope != "project" {
		t.Errorf("status --for claude = %+v, want installed in project scope", one[0])
	}

	r = runCLI(t, cfg, "skills", "status", "-o", "json")
	var all []skillsStatusEntry
	if r.Code != 0 || json.Unmarshal([]byte(r.Stdout), &all) != nil || len(all) != 7 {
		t.Fatalf("status: want 7 rows (cross-client and six agents): %s", r)
	}
	if r := runCLI(t, cfg, "skills", "status", "--for", "nope"); r.Code != 1 || !strings.Contains(r.Stderr, "unknown agent") {
		t.Errorf("status --for nope: %s", r)
	}
}

func TestSkillsUninstall(t *testing.T) {
	_, cfg := inProject(t)
	runCLI(t, cfg, "skills", "install", "--for", "claude")

	r := runCLI(t, cfg, "skills", "uninstall", "--for", "claude")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Removed:") {
		t.Fatalf("uninstall: %s", r)
	}
	r = runCLI(t, cfg, "skills", "uninstall", "--for", "claude")
	if r.Code != 0 || !strings.Contains(r.Stdout, "No Claude Code skill files found to remove.") {
		t.Fatalf("second uninstall: %s", r)
	}
	if r := runCLI(t, cfg, "skills", "uninstall", "--cross-client", "--for", "claude"); r.Code != 1 {
		t.Errorf("uninstall --cross-client --for: %s", r)
	}
}

func TestSkillsInstallAgentEnvelope(t *testing.T) {
	_, cfg := inProject(t)
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "skills", "install", "--for", "claude")
	var env struct {
		OK     bool                   `json:"ok"`
		Result map[string]interface{} `json:"result"`
	}
	if r.Code != 0 || json.Unmarshal([]byte(r.Stdout), &env) != nil {
		t.Fatalf("agent install: %s", r)
	}
	if !env.OK || env.Result["action"] != "installed" || env.Result["scope"] != "project" || env.Result["agent"] != "claude" {
		t.Errorf("envelope = %+v", env)
	}

	r = runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "skills", "uninstall", "--for", "claude")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"removed"`) {
		t.Errorf("agent uninstall: %s", r)
	}
}
