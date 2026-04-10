package client

import (
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

func TestUnwrapSingleOne(t *testing.T) {
	results := []EnvResult{{Name: "prod", Data: "hello", Error: nil}}
	data, err := UnwrapSingle(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != "hello" {
		t.Errorf("expected 'hello', got %v", data)
	}
}

func TestUnwrapSingleMultiple(t *testing.T) {
	results := []EnvResult{
		{Name: "prod", Data: "a"},
		{Name: "staging", Data: "b"},
	}
	data, err := UnwrapSingle(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", data)
	}
	if m["prod"] != "a" || m["staging"] != "b" {
		t.Errorf("unexpected map: %v", m)
	}
}

func TestIsSingleEnv(t *testing.T) {
	if !IsSingleEnv([]EnvResult{{Name: "a"}}) {
		t.Error("single result should be single env")
	}
	if IsSingleEnv([]EnvResult{{Name: "a"}, {Name: "b"}}) {
		t.Error("two results should not be single env")
	}
}
