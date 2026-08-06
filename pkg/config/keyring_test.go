package config

import (
	"os"
	"reflect"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestMain swaps in an in-memory keyring provider for the whole test binary.
//
// Without this, any test that exercises SetToken/GetToken with a real OS
// keyring available (IsKeyringAvailable() checks the real backend) would
// read from or write to the developer's actual keyring. MockInit makes
// IsKeyringAvailable() consistently report true for the rest of the run, so
// every keyring-touching test below takes the keyring branch against the
// mock store instead of the plaintext-config branch.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

// TestMigrateTokensSkipsPlaceholders verifies the accounting without touching
// the OS keyring: with every token either empty or a placeholder, SetToken is
// never reached, so the test is safe on headless CI.
func TestMigrateTokensSkipsPlaceholders(t *testing.T) {
	cfg := &Config{
		Tokens: []NamedToken{
			{Name: "prod-token", Token: "${DT_API_TOKEN}"},
			{Name: "keyring-token", Token: ""},
			{Name: "eu-token", Token: "${EU_TOKEN}"},
		},
	}

	migrated, skipped, err := MigrateTokensToKeyring(cfg)
	if err != nil {
		t.Fatalf("MigrateTokensToKeyring() error = %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0", migrated)
	}
	if !reflect.DeepEqual(skipped, []string{"prod-token", "eu-token"}) {
		t.Errorf("skipped = %v, want [prod-token eu-token]", skipped)
	}
	// Placeholders must survive so the indirection is not silently dropped.
	if cfg.Tokens[0].Token != "${DT_API_TOKEN}" {
		t.Errorf("placeholder token was modified: %q", cfg.Tokens[0].Token)
	}
	if cfg.Tokens[2].Token != "${EU_TOKEN}" {
		t.Errorf("placeholder token was modified: %q", cfg.Tokens[2].Token)
	}
}
