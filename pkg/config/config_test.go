package config

import (
	"os"
	"path/filepath"
	"testing"
)

func makeTestConfig() *Config {
	return &Config{
		APIVersion:     "dtmgd.io/v1",
		Kind:           "Config",
		CurrentContext: "prod",
		Contexts: []NamedContext{
			{Name: "prod", Context: Context{
				Host:     "https://managed.company.com",
				EnvID:    "env-prod",
				TokenRef: "prod-token",
			}},
			{Name: "staging", Context: Context{
				Host:     "https://staging.company.com",
				EnvID:    "env-staging",
				TokenRef: "staging-token",
			}},
		},
		Tokens: []NamedToken{
			{Name: "prod-token", Token: "api-token-prod"},
			{Name: "staging-token", Token: "api-token-staging"},
		},
	}
}

// --- Context.APIBaseURL ---

func TestAPIBaseURL(t *testing.T) {
	tests := []struct {
		name string
		ctx  Context
		want string
	}{
		{
			name: "normal",
			ctx:  Context{Host: "https://managed.company.com", EnvID: "abc12345"},
			want: "https://managed.company.com/e/abc12345/api",
		},
		{
			name: "trailing slash on host",
			ctx:  Context{Host: "https://managed.company.com/", EnvID: "env1"},
			want: "https://managed.company.com/e/env1/api",
		},
		{
			name: "multiple trailing slashes",
			ctx:  Context{Host: "https://managed.company.com///", EnvID: "env1"},
			want: "https://managed.company.com/e/env1/api",
		},
		{
			name: "empty host",
			ctx:  Context{Host: "", EnvID: "env1"},
			want: "",
		},
		{
			name: "empty env-id",
			ctx:  Context{Host: "https://managed.company.com", EnvID: ""},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ctx.APIBaseURL()
			if got != tt.want {
				t.Errorf("APIBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Context.ClusterAPIBaseURL ---

func TestClusterAPIBaseURL(t *testing.T) {
	tests := []struct {
		name string
		ctx  Context
		want string
	}{
		{
			name: "normal",
			ctx:  Context{Host: "https://managed.company.com"},
			want: "https://managed.company.com/api",
		},
		{
			name: "trailing slash",
			ctx:  Context{Host: "https://managed.company.com/"},
			want: "https://managed.company.com/api",
		},
		{
			name: "empty host",
			ctx:  Context{Host: ""},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ctx.ClusterAPIBaseURL()
			if got != tt.want {
				t.Errorf("ClusterAPIBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Config.CurrentContextObj ---

func TestCurrentContextObj(t *testing.T) {
	cfg := makeTestConfig()

	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Host != "https://managed.company.com" {
		t.Errorf("expected prod host, got %q", ctx.Host)
	}
	if ctx.EnvID != "env-prod" {
		t.Errorf("expected env-prod, got %q", ctx.EnvID)
	}
}

func TestCurrentContextObjMissing(t *testing.T) {
	cfg := makeTestConfig()
	cfg.CurrentContext = "nonexistent"
	_, err := cfg.CurrentContextObj()
	if err == nil {
		t.Error("expected error for missing context, got nil")
	}
}

func TestCurrentContextObjEmpty(t *testing.T) {
	cfg := makeTestConfig()
	cfg.CurrentContext = ""
	_, err := cfg.CurrentContextObj()
	if err == nil {
		t.Error("expected error for empty current context, got nil")
	}
}

// --- Config.GetContext ---

func TestGetContext(t *testing.T) {
	cfg := makeTestConfig()

	nc, err := cfg.GetContext("staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nc.Context.EnvID != "env-staging" {
		t.Errorf("expected env-staging, got %q", nc.Context.EnvID)
	}
}

func TestGetContextNotFound(t *testing.T) {
	cfg := makeTestConfig()
	_, err := cfg.GetContext("noenv")
	if err == nil {
		t.Error("expected error for missing context, got nil")
	}
}

// --- Config.GetToken ---

func TestGetTokenFromConfig(t *testing.T) {
	// Only works when keyring is unavailable; on CI without keyring this should fall through.
	if IsKeyringAvailable() {
		t.Skip("keyring available — token lookup behavior depends on keyring state")
	}
	cfg := makeTestConfig()
	token, err := cfg.GetToken("prod-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "api-token-prod" {
		t.Errorf("expected api-token-prod, got %q", token)
	}
}

func TestGetTokenNotFound(t *testing.T) {
	if IsKeyringAvailable() {
		t.Skip("keyring available — GetToken may succeed from keyring")
	}
	cfg := makeTestConfig()
	_, err := cfg.GetToken("no-such-token")
	if err == nil {
		t.Error("expected error for missing token, got nil")
	}
}

// --- Config.SetContext ---

func TestSetContextAdd(t *testing.T) {
	cfg := makeTestConfig()

	cfg.SetContext("newenv", "https://new.company.com", "env-new", "new-token", "new environment")
	nc, err := cfg.GetContext("newenv")
	if err != nil {
		t.Fatalf("context not found after SetContext: %v", err)
	}
	if nc.Context.Host != "https://new.company.com" {
		t.Errorf("expected new host, got %q", nc.Context.Host)
	}
}

func TestSetContextUpdate(t *testing.T) {
	cfg := makeTestConfig()

	// Update only the host of the existing "prod" context.
	cfg.SetContext("prod", "https://updated.company.com", "", "", "")
	nc, err := cfg.GetContext("prod")
	if err != nil {
		t.Fatalf("context not found: %v", err)
	}
	if nc.Context.Host != "https://updated.company.com" {
		t.Errorf("expected updated host, got %q", nc.Context.Host)
	}
	// Existing fields not overwritten by empty string.
	if nc.Context.EnvID != "env-prod" {
		t.Errorf("EnvID should be unchanged, got %q", nc.Context.EnvID)
	}
}

// --- Config.DeleteContext ---

func TestDeleteContext(t *testing.T) {
	cfg := makeTestConfig()
	if err := cfg.DeleteContext("staging"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := cfg.GetContext("staging")
	if err == nil {
		t.Error("expected staging to be deleted")
	}
	if len(cfg.Contexts) != 1 {
		t.Errorf("expected 1 context remaining, got %d", len(cfg.Contexts))
	}
}

func TestDeleteContextNotFound(t *testing.T) {
	cfg := makeTestConfig()
	if err := cfg.DeleteContext("noenv"); err == nil {
		t.Error("expected error for missing context, got nil")
	}
}

// --- Config.NewConfig ---

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()
	if cfg.APIVersion != "dtmgd.io/v1" {
		t.Errorf("expected dtmgd.io/v1, got %q", cfg.APIVersion)
	}
	if cfg.Kind != "Config" {
		t.Errorf("expected Config, got %q", cfg.Kind)
	}
	if cfg.Contexts == nil {
		t.Error("Contexts should be initialized (not nil)")
	}
	if cfg.Tokens == nil {
		t.Error("Tokens should be initialized (not nil)")
	}
}

// --- LoadFrom / SaveTo round-trip ---

func TestLoadFromSaveToRoundTrip(t *testing.T) {
	cfg := makeTestConfig()

	tmp := filepath.Join(t.TempDir(), "config.yaml")
	if err := cfg.SaveTo(tmp); err != nil {
		t.Fatalf("SaveTo failed: %v", err)
	}

	loaded, err := LoadFrom(tmp)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}
	if loaded.CurrentContext != cfg.CurrentContext {
		t.Errorf("CurrentContext mismatch: got %q, want %q", loaded.CurrentContext, cfg.CurrentContext)
	}
	if len(loaded.Contexts) != len(cfg.Contexts) {
		t.Errorf("Contexts count mismatch: got %d, want %d", len(loaded.Contexts), len(cfg.Contexts))
	}
	if len(loaded.Tokens) != len(cfg.Tokens) {
		t.Errorf("Tokens count mismatch: got %d, want %d", len(loaded.Tokens), len(cfg.Tokens))
	}
	// Verify prod context survived round-trip.
	nc, err := loaded.GetContext("prod")
	if err != nil {
		t.Fatalf("prod context not found after round-trip: %v", err)
	}
	if nc.Context.Host != "https://managed.company.com" {
		t.Errorf("host mismatch: got %q", nc.Context.Host)
	}
}

func TestLoadFromMissingFile(t *testing.T) {
	_, err := LoadFrom("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestSaveToCreatesFile(t *testing.T) {
	cfg := NewConfig()
	tmp := filepath.Join(t.TempDir(), "new-config.yaml")
	if err := cfg.SaveTo(tmp); err != nil {
		t.Fatalf("SaveTo failed: %v", err)
	}
	if _, err := os.Stat(tmp); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
	// Verify file permissions (mode 0600).
	info, _ := os.Stat(tmp)
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600, got %04o", info.Mode().Perm())
	}
}
