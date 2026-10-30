package cmd

import (
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
