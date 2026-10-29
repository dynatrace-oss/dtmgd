package client

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestMain swaps in an in-memory keyring. MultiRequest and NewForContext read
// tokens through config.GetToken, which consults the OS keyring first; without
// the mock these tests would touch a developer's real credential store.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}
