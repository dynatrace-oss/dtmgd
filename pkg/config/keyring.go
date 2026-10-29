package config

import (
	"errors"
	"fmt"
	"sync"

	"github.com/zalando/go-keyring"
)

const keyringService = "dtmgd"

// TokenStore provides OS keyring access for API tokens.
type TokenStore struct{}

// NewTokenStore creates a new TokenStore.
func NewTokenStore() *TokenStore {
	return &TokenStore{}
}

// GetToken retrieves a token from the OS keyring.
func (ts *TokenStore) GetToken(name string) (string, error) {
	return keyring.Get(keyringService, name)
}

// SetToken stores a token in the OS keyring.
func (ts *TokenStore) SetToken(name, token string) error {
	return keyring.Set(keyringService, name, token)
}

// DeleteToken removes a token from the OS keyring.
func (ts *TokenStore) DeleteToken(name string) error {
	return keyring.Delete(keyringService, name)
}

// probe caches whether the OS keyring works, so it is asked once per process
// rather than on every token lookup (concurrently, during --env fan-out).
var probe struct {
	sync.Mutex
	done bool
	err  error
}

// keyringStatus reports nil when the OS keyring is usable, or why it is not.
func keyringStatus() error {
	probe.Lock()
	defer probe.Unlock()
	if !probe.done {
		_, err := keyring.Get(keyringService, "__probe__")
		// ErrNotFound means the keyring works and the key doesn't exist.
		if errors.Is(err, keyring.ErrNotFound) {
			err = nil
		}
		probe.err, probe.done = err, true
	}
	return probe.err
}

// resetKeyringProbe forgets the cached probe result. Tests that swap the
// keyring backend call it.
func resetKeyringProbe() {
	probe.Lock()
	probe.done, probe.err = false, nil
	probe.Unlock()
}

// IsKeyringAvailable reports whether the OS keyring can store tokens.
func IsKeyringAvailable() bool {
	return keyringStatus() == nil
}

// KeyringBackend returns a human-readable name for the current keyring backend.
func KeyringBackend() string {
	return "OS keyring"
}

// MigrateTokensToKeyring moves plaintext tokens from config to the OS keyring.
// Returns the number migrated and the names of tokens skipped because they are
// ${VAR} references.
//
// Placeholder tokens are skipped rather than migrated: storing the current
// expansion in the keyring would silently convert a live indirection into a
// frozen snapshot, and the user would not find out until the variable changed.
func MigrateTokensToKeyring(cfg *Config) (int, []string, error) {
	ts := NewTokenStore()
	migrated := 0
	var skipped []string

	for i, nt := range cfg.Tokens {
		if nt.Token == "" {
			continue
		}
		if HasPlaceholder(nt.Token) {
			skipped = append(skipped, nt.Name)
			continue
		}
		if err := ts.SetToken(nt.Name, nt.Token); err != nil {
			return migrated, skipped, fmt.Errorf("failed to migrate token %q: %w", nt.Name, err)
		}
		cfg.Tokens[i].Token = ""
		migrated++
	}
	return migrated, skipped, nil
}
