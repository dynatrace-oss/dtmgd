package testsandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
)

func TestEnterMovesHomeAndConfigIntoTheSandbox(t *testing.T) {
	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	sep := string(filepath.Separator)
	if !strings.HasPrefix(xdg.ConfigHome, Root+sep) {
		t.Errorf("config home %q is outside %q", xdg.ConfigHome, Root)
	}
	if home, err := os.UserHomeDir(); err != nil || !strings.HasPrefix(home, Root+sep) {
		t.Errorf("home %q (err %v) is outside %q", home, err, Root)
	}
	cleanup()
	if _, err := os.Stat(Root); !os.IsNotExist(err) {
		t.Errorf("cleanup left %q behind (stat err %v)", Root, err)
	}
}

func TestMainReturnsTheRunResult(t *testing.T) {
	if got := Main(func() int { return 7 }); got != 7 {
		t.Errorf("Main returned %d, want 7", got)
	}
}

// TestEnterClearsTheConfigSelection pins that DTMGD_CONFIG and DTMGD_CONTEXT
// exported in a developer's shell cannot point a test at a real config.
func TestEnterClearsTheConfigSelection(t *testing.T) {
	t.Setenv("DTMGD_CONFIG", "/somewhere/real/config")
	t.Setenv("DTMGD_CONTEXT", "prod")
	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, v := range []string{"DTMGD_CONFIG", "DTMGD_CONTEXT"} {
		if got := os.Getenv(v); got != "" {
			t.Errorf("%s = %q inside the sandbox", v, got)
		}
	}
}

// TestEnterRefusesALocalConfigAbove pins that a .dtmgd.yaml in the working
// directory or above it stops the tests: dtmgd finds it by walking up, so a
// test without --config would read and rewrite it.
func TestEnterRefusesALocalConfigAbove(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, ".dtmgd.yaml")
	if err := os.WriteFile(local, []byte("contexts: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	cleanup, err := Enter()
	if err == nil {
		cleanup()
		t.Fatal("Enter accepted a .dtmgd.yaml above the working directory")
	}
	if !strings.Contains(err.Error(), local) {
		t.Errorf("error %q does not name %s", err, local)
	}
}

// TestEnterRecordsTheOriginalValues pins Original, which a test needs to
// compare the real default path with the help text without opening it.
func TestEnterRecordsTheOriginalValues(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "/original/local")
	cleanup, err := Enter()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if got := Original["LOCALAPPDATA"]; got != "/original/local" {
		t.Errorf("Original[LOCALAPPDATA] = %q", got)
	}
}
