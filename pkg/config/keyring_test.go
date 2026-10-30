package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/dynatrace-oss/dtmgd/internal/testsandbox"
)

// TestMain swaps in an in-memory keyring provider for the whole test binary.
//
// Without this, any test that exercises SetToken/GetToken with a real OS
// keyring available (IsKeyringAvailable() checks the real backend) would
// read from or write to the developer's actual keyring. MockInit makes
// IsKeyringAvailable() consistently report true for the rest of the run, so
// every keyring-touching test below takes the keyring branch against the
// mock store instead of the plaintext-config branch.
//
// testsandbox.Main runs the tests with a temporary home and config
// directory, so Save and Load without a path cannot reach the real config.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(testsandbox.Main(m.Run))
}

// withoutKeyring makes IsKeyringAvailable() report false for the duration of
// one test, then restores the working mock installed by TestMain.
//
// Swapping the package-global provider is safe here because no test in this
// package calls t.Parallel, so they run sequentially.
func withoutKeyring(t *testing.T) {
	t.Helper()
	resetKeyringProbe()
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	t.Cleanup(func() { keyring.MockInit(); resetKeyringProbe() })
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

// TestGetTokenSurfacesKeyringErrors pins that a token stored in a keyring
// that cannot be read reports the keyring's error, instead of the misleading
// "token not found" it used to.
func TestGetTokenSurfacesKeyringErrors(t *testing.T) {
	resetKeyringProbe()
	keyring.MockInitWithError(errors.New("locked"))
	t.Cleanup(func() { keyring.MockInit(); resetKeyringProbe() })

	cfg := NewConfig()
	cfg.Tokens = []NamedToken{{Name: "t1", Token: ""}}
	_, err := cfg.GetToken("t1")
	if err == nil || !strings.Contains(err.Error(), "keyring: locked") {
		t.Errorf("err = %v, want it to carry the keyring error", err)
	}
}

// keyringReadsFail makes the keyring probe pass but every later keyring call
// fail with err, as a keyring that locks after startup does.
func keyringReadsFail(t *testing.T, err error) {
	t.Helper()
	probe.Lock()
	probe.done, probe.err = true, nil
	probe.Unlock()
	keyring.MockInitWithError(err)
	t.Cleanup(func() { keyring.MockInit(); resetKeyringProbe() })
}

// TestGetTokenSources pins where GetToken finds a token and what it says
// when it cannot: the keyring wins whenever it holds a non-empty token; an
// empty or missing keyring entry falls back to the config; and an empty
// config entry reports why the keyring did not supply it.
func TestGetTokenSources(t *testing.T) {
	resetKeyringProbe()
	t.Cleanup(resetKeyringProbe)
	for name, value := range map[string]string{"kr": "from-keyring", "kr-empty": ""} {
		if err := keyring.Set(keyringService, name, value); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = keyring.Delete(keyringService, name) })
	}
	cfg := &Config{Tokens: []NamedToken{
		{Name: "kr", Token: "plain-kr"},
		{Name: "kr-empty", Token: "plain-empty"},
		{Name: "blank", Token: ""},
		{Name: "plain", Token: "plain-value"},
	}}
	bare := &Config{}

	cases := []struct {
		name, ref string
		cfg       *Config
		want      string
		wantErr   string
	}{
		{"keyring first", "kr", cfg, "from-keyring", ""},
		{"keyring without a config entry", "kr", bare, "from-keyring", ""},
		{"empty keyring value falls back", "kr-empty", cfg, "plain-empty", ""},
		{"not in keyring, plaintext", "plain", cfg, "plain-value", ""},
		{"not in keyring, blank entry", "blank", cfg, "", `token "blank" not found in keyring (may need to re-add credentials)`},
		{"nowhere", "missing", cfg, "", `token "missing" not found`},
	}
	for _, tc := range cases {
		got, err := tc.cfg.GetToken(tc.ref)
		gotErr := ""
		if err != nil {
			gotErr = err.Error()
		}
		if got != tc.want || gotErr != tc.wantErr {
			t.Errorf("%s: GetToken(%q) = %q, %q; want %q, %q", tc.name, tc.ref, got, gotErr, tc.want, tc.wantErr)
		}
	}
}

// TestGetTokenKeyringReadFails pins GetToken when the keyring passed its
// probe but cannot be read: a blank config entry reports the keyring's error
// (wrapped), a plaintext entry is still used, and an unknown name is plain
// "not found". A keyring that fails its probe reports the probe's error.
func TestGetTokenKeyringReadFails(t *testing.T) {
	locked := errors.New("locked")
	cfg := &Config{Tokens: []NamedToken{{Name: "blank", Token: ""}, {Name: "plain", Token: "plain-value"}}}

	t.Run("read fails", func(t *testing.T) {
		keyringReadsFail(t, locked)
		_, err := cfg.GetToken("blank")
		if err == nil || err.Error() != `keyring: locked (token "blank" is stored there)` || !errors.Is(err, locked) {
			t.Errorf("blank entry: err = %v", err)
		}
		if got, err := cfg.GetToken("plain"); got != "plain-value" || err != nil {
			t.Errorf("plaintext entry: %q, %v", got, err)
		}
		if _, err := cfg.GetToken("missing"); err == nil || err.Error() != `token "missing" not found` {
			t.Errorf("unknown name: err = %v", err)
		}
	})

	t.Run("probe fails", func(t *testing.T) {
		withoutKeyring(t)
		_, err := cfg.GetToken("blank")
		want := fmt.Sprintf("keyring: %v (token \"blank\" is stored there)", keyring.ErrUnsupportedPlatform)
		if err == nil || err.Error() != want || !errors.Is(err, keyring.ErrUnsupportedPlatform) {
			t.Errorf("blank entry: err = %v, want %s", err, want)
		}
	})
}
