package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/internal/testsandbox"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// TestDefaultConfigPathIsSandboxed pins that TestMain moved the default config
// path and the home directory into a temporary directory, so a command run
// without --config can never reach a developer's real config or skills.
func TestDefaultConfigPathIsSandboxed(t *testing.T) {
	assertSandboxed(t, config.DefaultConfigPath())
}

func assertSandboxed(t *testing.T, cfgPath string) {
	t.Helper()
	root := testsandbox.Root
	if root == "" || !strings.HasPrefix(cfgPath, root+string(filepath.Separator)) {
		t.Fatalf("default config path %q is outside the sandbox %q", cfgPath, root)
	}
	home, err := os.UserHomeDir()
	if err != nil || !strings.HasPrefix(home, root+string(filepath.Separator)) {
		t.Fatalf("home directory %q (err %v) is outside the sandbox %q", home, err, root)
	}
}
