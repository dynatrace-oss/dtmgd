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

// withoutKeyring makes IsKeyringAvailable() report false for the duration of
// one test, then restores the working mock installed by TestMain.
//
// Swapping the package-global provider is safe here because no test in this
// package calls t.Parallel, so they run sequentially.
func withoutKeyring(t *testing.T) {
	t.Helper()
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	t.Cleanup(keyring.MockInit)
}

// TestSetTokenPlaintextBranch covers SetToken's no-keyring path: the branch
// taken on a headless Linux box with no secret-service backend, which is a
// real production path rather than a rare fallback.
//
// It needs withoutKeyring because TestMain installs a working mock for the
// whole binary, so IsKeyringAvailable() otherwise reports true for every test
// and this branch never executes.
func TestSetTokenPlaintextBranch(t *testing.T) {
	withoutKeyring(t)

	if IsKeyringAvailable() {
		t.Fatal("IsKeyringAvailable() = true, want false")
	}

	cfg := &Config{Tokens: []NamedToken{{Name: "existing", Token: "old-value"}}}

	// Updating an existing entry writes the token into the config, rather than
	// blanking it as the keyring branch does.
	if err := cfg.SetToken("existing", "updated-value"); err != nil {
		t.Fatalf("SetToken() error = %v", err)
	}
	if got := cfg.Tokens[0].Token; got != "updated-value" {
		t.Errorf("existing token = %q, want %q", got, "updated-value")
	}

	// Creating a new entry appends it, again with the value inline.
	if err := cfg.SetToken("fresh", "fresh-value"); err != nil {
		t.Fatalf("SetToken() error = %v", err)
	}
	if len(cfg.Tokens) != 2 {
		t.Fatalf("len(Tokens) = %d, want 2", len(cfg.Tokens))
	}
	if cfg.Tokens[1].Name != "fresh" || cfg.Tokens[1].Token != "fresh-value" {
		t.Errorf("appended token = %+v, want {fresh fresh-value}", cfg.Tokens[1])
	}

	// The value must be readable back through the plaintext branch of GetToken.
	got, err := cfg.GetToken("fresh")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "fresh-value" {
		t.Errorf("GetToken() = %q, want %q", got, "fresh-value")
	}
}

// TestSetTokenPlaintextBranchExpandsOnRead confirms that a ${VAR} reference
// stored in the config is still resolved at read time when no keyring is
// available — the placeholder must not be handed out verbatim as a token.
func TestSetTokenPlaintextBranchExpandsOnRead(t *testing.T) {
	withoutKeyring(t)
	t.Setenv("DT_PLAINTEXT_TEST_TOKEN", "dt0c01.RESOLVED")

	cfg := &Config{Tokens: []NamedToken{{Name: "prod", Token: "${DT_PLAINTEXT_TEST_TOKEN}"}}}

	got, err := cfg.GetToken("prod")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "dt0c01.RESOLVED" {
		t.Errorf("GetToken() = %q, want the expanded value", got)
	}
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
