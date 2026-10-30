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
