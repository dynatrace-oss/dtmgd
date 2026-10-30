package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/internal/testsandbox"
)

// TestDefaultConfigPathIsSandboxed pins that TestMain moved the default config
// path and the home directory into a temporary directory, so Save and Load
// without a path can never reach a developer's real config.
func TestDefaultConfigPathIsSandboxed(t *testing.T) {
	root := testsandbox.Root
	sep := string(filepath.Separator)
	if p := DefaultConfigPath(); root == "" || !strings.HasPrefix(p, root+sep) {
		t.Fatalf("default config path %q is outside the sandbox %q", p, root)
	}
	if home, err := os.UserHomeDir(); err != nil || !strings.HasPrefix(home, root+sep) {
		t.Fatalf("home directory %q (err %v) is outside the sandbox %q", home, err, root)
	}
}
