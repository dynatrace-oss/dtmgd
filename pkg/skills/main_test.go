package skills

import (
	"os"
	"testing"

	"github.com/dynatrace-oss/dtmgd/internal/testsandbox"
)

// TestMain runs the tests with a temporary home directory: Status also looks
// at the user-wide skill locations, so a real dtmgd skill in your home made
// the "fresh dir" tests fail.
func TestMain(m *testing.M) {
	os.Exit(testsandbox.Main(m.Run))
}
