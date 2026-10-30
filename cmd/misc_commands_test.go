package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		r := runCLI(t, "", "completion", shell)
		if r.Code != 0 || !strings.Contains(r.Stdout, "dtmgd") {
			t.Errorf("completion %s: exit %d, %d bytes of stdout", shell, r.Code, len(r.Stdout))
		}
	}
	if r := runCLI(t, "", "completion", "tcsh"); r.Code != 2 {
		t.Errorf("completion tcsh: %s", r)
	}
}

func TestVersionCommand(t *testing.T) {
	r := runCLI(t, "", "version")
	if r.Code != 0 || !strings.HasPrefix(r.Stdout, "dtmgd ") || !strings.Contains(r.Stdout, "commit:") {
		t.Errorf("version: %s", r)
	}
}

// TestContextNameCompletion pins that every command taking a context name
// completes it from the config. The config comes through DTMGD_CONFIG: cobra
// completes the last argument, so runCLI's trailing --config would be it.
func TestContextNameCompletion(t *testing.T) {
	cfg := withTempConfig(t, twoContexts)
	for _, args := range [][]string{
		{"ctx", "delete"},
		{"config", "use-context"},
		{"config", "delete-context"},
	} {
		full := append([]string{"__complete"}, args...)
		full = append(full, "")
		r := runCLIWithEnv(t, map[string]string{"DTMGD_CONFIG": cfg}, "", full...)
		if r.Code != 0 || !strings.Contains(r.Stdout, "a\nb\n") {
			t.Errorf("completion of %v: %s", args, r)
		}
	}
}

// TestVersionAgentEnvelope pins that version answers an agent like every
// other command: an envelope whose result holds the version, commit and
// build date. It printed a bare text line.
func TestVersionAgentEnvelope(t *testing.T) {
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, "", "version")
	var env struct {
		OK     bool              `json:"ok"`
		Result map[string]string `json:"result"`
	}
	if r.Code != 0 || json.Unmarshal([]byte(r.Stdout), &env) != nil {
		t.Fatalf("version in agent mode is not an envelope: %s", r)
	}
	if !env.OK || env.Result["version"] == "" {
		t.Errorf("envelope = %+v, want ok and a version", env)
	}
	for _, k := range []string{"commit", "date"} {
		if _, ok := env.Result[k]; !ok {
			t.Errorf("envelope result has no %q", k)
		}
	}
}

func TestVersionDataFormats(t *testing.T) {
	r := runCLI(t, "", "version", "-o", "json")
	var info map[string]string
	if r.Code != 0 || json.Unmarshal([]byte(r.Stdout), &info) != nil || info["version"] == "" {
		t.Errorf("version -o json: %s", r)
	}
	r = runCLI(t, "", "version", "-o", "yaml")
	if r.Code != 0 || !strings.Contains(r.Stdout, "version: ") || !strings.Contains(r.Stdout, "commit: ") {
		t.Errorf("version -o yaml: %s", r)
	}
	r = runCLI(t, "", "version", "-o", "csv")
	if r.Code != 0 || !strings.HasPrefix(r.Stdout, "VERSION,COMMIT,BUILT\n") {
		t.Errorf("version -o csv: %s", r)
	}
}

// TestCurrentContextAgentEnvelope pins that config current-context and ctx
// current answer an agent with an envelope, keyed as in the config file.
// They printed the bare name, which an envelope parser cannot read.
func TestCurrentContextAgentEnvelope(t *testing.T) {
	cfg := withTempConfig(t, twoContexts)
	for _, args := range [][]string{{"config", "current-context"}, {"ctx", "current"}} {
		r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, args...)
		var env struct {
			OK     bool              `json:"ok"`
			Result map[string]string `json:"result"`
		}
		if r.Code != 0 || json.Unmarshal([]byte(r.Stdout), &env) != nil || !env.OK || env.Result["current-context"] != "a" {
			t.Errorf("%v in agent mode: %s", args, r)
		}
		r = runCLI(t, cfg, append(args, "-o", "json")...)
		var obj map[string]string
		if r.Code != 0 || json.Unmarshal([]byte(r.Stdout), &obj) != nil || obj["current-context"] != "a" {
			t.Errorf("%v -o json: %s", args, r)
		}
	}
}
