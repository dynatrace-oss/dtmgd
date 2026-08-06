package config

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolveSuccess(t *testing.T) {
	t.Setenv("DT_MANAGED_HOST", "https://managed.example.com")
	t.Setenv("DT_ENV_ID", "abc12345")

	ctx := &Context{
		Host:         "${DT_MANAGED_HOST}",
		EnvID:        "${DT_ENV_ID}",
		TokenRef:     "prod-token",
		HTTPProxyURL: "http://user:pa$$w0rd@proxy.corp:8080",
	}

	got, err := ctx.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Host != "https://managed.example.com" {
		t.Errorf("Host = %q, want the expanded value", got.Host)
	}
	if got.EnvID != "abc12345" {
		t.Errorf("EnvID = %q, want the expanded value", got.EnvID)
	}
	if got.TokenRef != "prod-token" {
		t.Errorf("TokenRef = %q, want it unchanged", got.TokenRef)
	}
	// A literal $ must survive resolution.
	if got.HTTPProxyURL != "http://user:pa$$w0rd@proxy.corp:8080" {
		t.Errorf("HTTPProxyURL = %q, want the dollars preserved", got.HTTPProxyURL)
	}

	// Resolving must not mutate the receiver.
	if ctx.Host != "${DT_MANAGED_HOST}" {
		t.Errorf("Resolve mutated the source context: Host = %q", ctx.Host)
	}
}

func TestResolveUnsetSingle(t *testing.T) {
	ctx := &Context{Host: "${MISSING_HOST}", EnvID: "abc12345"}

	_, err := ctx.Resolve()
	if err == nil {
		t.Fatal("Resolve() succeeded with an unset variable")
	}

	var uerr *UnresolvedVarsError
	if !errors.As(err, &uerr) {
		t.Fatalf("error type = %T, want *UnresolvedVarsError", err)
	}
	if !reflect.DeepEqual(uerr.Vars, []string{"MISSING_HOST"}) {
		t.Errorf("Vars = %v, want [MISSING_HOST]", uerr.Vars)
	}
	if got, want := uerr.Error(), "${MISSING_HOST} is not set"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got, want := uerr.InContext("eu-prod").Error(),
		`context "eu-prod" needs ${MISSING_HOST}, which is not set`; got != want {
		t.Errorf("InContext() = %q, want %q", got, want)
	}
}

func TestResolveUnsetMultiple(t *testing.T) {
	ctx := &Context{Host: "${MISSING_HOST}", EnvID: "${MISSING_ENV}", TokenRef: "${MISSING_HOST}"}

	_, err := ctx.Resolve()
	if err == nil {
		t.Fatal("Resolve() succeeded with unset variables")
	}

	var uerr *UnresolvedVarsError
	if !errors.As(err, &uerr) {
		t.Fatalf("error type = %T, want *UnresolvedVarsError", err)
	}
	// First-appearance order, deduped across fields.
	if !reflect.DeepEqual(uerr.Vars, []string{"MISSING_HOST", "MISSING_ENV"}) {
		t.Errorf("Vars = %v, want [MISSING_HOST MISSING_ENV]", uerr.Vars)
	}
	if got, want := uerr.Error(), "${MISSING_HOST} and ${MISSING_ENV} are not set"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got, want := uerr.InContext("eu-prod").Error(),
		`context "eu-prod" needs ${MISSING_HOST} and ${MISSING_ENV}, which are not set`; got != want {
		t.Errorf("InContext() = %q, want %q", got, want)
	}
}

func TestResolveNoPlaceholders(t *testing.T) {
	// The no-op case: a context that was already expanded, or never used
	// placeholders, must resolve unchanged.
	ctx := &Context{Host: "https://managed.company.com", EnvID: "env-prod", TokenRef: "prod-token"}
	got, err := ctx.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Host != ctx.Host || got.EnvID != ctx.EnvID || got.TokenRef != ctx.TokenRef {
		t.Errorf("Resolve() altered a placeholder-free context: %+v", got)
	}
}

// --- ResolvedContext.APIBaseURL (moved from config_test.go) ---

func TestAPIBaseURL(t *testing.T) {
	tests := []struct {
		name string
		ctx  ResolvedContext
		want string
	}{
		{"normal", ResolvedContext{Host: "https://managed.company.com", EnvID: "abc123"}, "https://managed.company.com/e/abc123/api"},
		{"trailing slash", ResolvedContext{Host: "https://managed.company.com/", EnvID: "abc123"}, "https://managed.company.com/e/abc123/api"},
		{"multiple trailing slashes", ResolvedContext{Host: "https://managed.company.com///", EnvID: "abc123"}, "https://managed.company.com/e/abc123/api"},
		{"empty host", ResolvedContext{Host: "", EnvID: "abc123"}, ""},
		{"empty env id", ResolvedContext{Host: "https://managed.company.com", EnvID: ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.APIBaseURL(); got != tt.want {
				t.Errorf("APIBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClusterAPIBaseURL(t *testing.T) {
	tests := []struct {
		name string
		ctx  ResolvedContext
		want string
	}{
		{"normal", ResolvedContext{Host: "https://managed.company.com"}, "https://managed.company.com/api"},
		{"trailing slash", ResolvedContext{Host: "https://managed.company.com/"}, "https://managed.company.com/api"},
		{"empty host", ResolvedContext{Host: ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.ClusterAPIBaseURL(); got != tt.want {
				t.Errorf("ClusterAPIBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
