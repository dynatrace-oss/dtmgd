package config

import (
	"fmt"

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

// IsKeyringAvailable returns true if the OS keyring is accessible.
func IsKeyringAvailable() bool {
	_, err := keyring.Get(keyringService, "__probe__")
	// ErrNotFound means keyring works but the key doesn't exist — that's fine.
	return err == nil || err == keyring.ErrNotFound
}

// KeyringBackend returns a human-readable name for the current keyring backend.
func KeyringBackend() string {
	return "OS keyring"
}

// MigrateTokensToKeyring moves plaintext tokens from config to the OS keyring.
// Returns the number of tokens migrated.
func MigrateTokensToKeyring(cfg *Config) (int, error) {
	ts := NewTokenStore()
	migrated := 0
	for i, nt := range cfg.Tokens {
		if nt.Token == "" {
			continue
		}
		if err := ts.SetToken(nt.Name, nt.Token); err != nil {
			return migrated, fmt.Errorf("failed to migrate token %q: %w", nt.Name, err)
		}
		cfg.Tokens[i].Token = ""
		migrated++
	}
	return migrated, nil
}
