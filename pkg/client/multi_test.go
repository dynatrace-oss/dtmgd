package client

import (
	"reflect"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

func testConfig() *config.Config {
	return &config.Config{
		CurrentContext: "prod",
		Contexts: []config.NamedContext{
			{Name: "prod", Context: config.Context{Host: "https://prod.example.com", EnvID: "env1", TokenRef: "t1"}},
			{Name: "staging", Context: config.Context{Host: "https://staging.example.com", EnvID: "env2", TokenRef: "t2"}},
		},
		Tokens: []config.NamedToken{
			{Name: "t1", Token: "token1"},
			{Name: "t2", Token: "token2"},
		},
	}
}

func TestResolveContextsEmpty(t *testing.T) {
	cfg := testConfig()
	ctxs, err := resolveContexts(cfg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctxs) != 1 || ctxs[0].Name != "prod" {
		t.Errorf("expected current context 'prod', got %v", ctxs)
	}
}

func TestResolveContextsAll(t *testing.T) {
	cfg := testConfig()
	ctxs, err := resolveContexts(cfg, "ALL_ENVIRONMENTS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctxs) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(ctxs))
	}
}

func TestResolveContextsSemicolon(t *testing.T) {
	cfg := testConfig()
	ctxs, err := resolveContexts(cfg, "prod;staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctxs) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(ctxs))
	}
}

func TestResolveContextsNotFound(t *testing.T) {
	cfg := testConfig()
	_, err := resolveContexts(cfg, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent context")
	}
}

func TestResolveContextsNoCurrentContext(t *testing.T) {
	cfg := testConfig()
	cfg.CurrentContext = ""
	_, err := resolveContexts(cfg, "")
	if err == nil {
		t.Error("expected error when no current context")
	}
}

func TestResolveContextsEmptySemicolon(t *testing.T) {
	cfg := testConfig()
	ctxs, err := resolveContexts(cfg, "prod;")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctxs) != 1 || ctxs[0].Name != "prod" {
		t.Errorf("expected 1 context 'prod', got %v", ctxs)
	}
}

// TestResolveContextsResultsAndErrors pins resolveContexts' exact results and
// error texts: names are trimmed, kept in the order given (duplicates too),
// and empty ones skipped; an unknown name lists every configured context.
func TestResolveContextsResultsAndErrors(t *testing.T) {
	names := func(ncs []config.NamedContext) []string {
		out := []string{}
		for _, nc := range ncs {
			out = append(out, nc.Name)
		}
		return out
	}
	noCurrent := testConfig()
	noCurrent.CurrentContext = ""
	staleCurrent := testConfig()
	staleCurrent.CurrentContext = "gone"
	empty := &config.Config{}

	cases := []struct {
		name    string
		cfg     *config.Config
		spec    string
		want    []string
		wantErr string
	}{
		{"current", testConfig(), "", []string{"prod"}, ""},
		{"no current context", noCurrent, "", nil, "no current context set. Run 'dtmgd config set-context' to configure one"},
		{"current context missing", staleCurrent, "", nil, `current context "gone" not found in config`},
		{"all", testConfig(), "ALL_ENVIRONMENTS", []string{"prod", "staging"}, ""},
		{"all, none configured", empty, "ALL_ENVIRONMENTS", nil, "no contexts configured"},
		{"order, spaces, duplicates", testConfig(), " staging ; ;prod;staging", []string{"staging", "prod", "staging"}, ""},
		{"only separators", testConfig(), ";", []string{}, ""},
		{"unknown name", testConfig(), "prod;qa;nope", nil, `context "qa" not found. Available: prod, staging`},
		{"unknown name, none configured", empty, "prod", nil, `context "prod" not found. Available: `},
	}
	for _, tc := range cases {
		got, err := resolveContexts(tc.cfg, tc.spec)
		gotErr := ""
		if err != nil {
			gotErr = err.Error()
		}
		if gotErr != tc.wantErr {
			t.Errorf("%s: err = %q, want %q", tc.name, gotErr, tc.wantErr)
		}
		if tc.wantErr != "" {
			if got != nil {
				t.Errorf("%s: result %v alongside an error", tc.name, got)
			}
			continue
		}
		if !reflect.DeepEqual(names(got), tc.want) {
			t.Errorf("%s: contexts = %v, want %v", tc.name, names(got), tc.want)
		}
	}
	// An all-separator spec yields nil, not an empty slice.
	if got, _ := resolveContexts(testConfig(), ";"); got != nil {
		t.Errorf("only separators: %#v, want nil", got)
	}
}
