package cmd

import (
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// configFlagUsage is the --config flag's help text.
func configFlagUsage(t *testing.T) string {
	t.Helper()
	f := newRootCmd(&App{Out: io.Discard, Err: io.Discard}).PersistentFlags().Lookup("config")
	if f == nil {
		t.Fatal("no --config flag")
	}
	return f.Usage
}

// The help is static text (the generated reference must not depend on the
// machine), so it names every OS's default.
func TestConfigFlagHelpNamesEveryDefaultPath(t *testing.T) {
	usage := configFlagUsage(t)
	for _, want := range []string{
		"~/.config/dtmgd/config",
		"~/Library/Application Support/dtmgd/config",
		`%LOCALAPPDATA%\dtmgd\config`,
		"$XDG_CONFIG_HOME",
	} {
		if !strings.Contains(usage, want) {
			t.Errorf("--config help %q does not name %q", usage, want)
		}
	}
}

// TestConfigFlagHelpNamesThisOSDefault runs in every CI OS job, so the help
// cannot drift from where config.DefaultConfigPath really looks.
func TestConfigFlagHelpNamesThisOSDefault(t *testing.T) {
	if os.Getenv("XDG_CONFIG_HOME") != "" {
		t.Skip("XDG_CONFIG_HOME is set")
	}
	path := config.DefaultConfigPath()
	switch runtime.GOOS {
	case "windows":
		path = strings.Replace(path, os.Getenv("LOCALAPPDATA"), "%LOCALAPPDATA%", 1)
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home directory")
		}
		path = strings.Replace(path, home, "~", 1)
	}
	if usage := configFlagUsage(t); !strings.Contains(usage, path) {
		t.Errorf("--config help %q does not name this OS's default %q", usage, path)
	}
}
