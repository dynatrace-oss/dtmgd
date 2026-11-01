package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// contextLevels runs ctx -o json and returns each context's safety-level.
func contextLevels(t *testing.T, cfg string) map[string]string {
	t.Helper()
	r := runCLI(t, cfg, "ctx", "-o", "json")
	var items []struct {
		Name        string `json:"name"`
		SafetyLevel string `json:"safety-level"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &items); err != nil {
		t.Fatalf("%v\n%s", err, r)
	}
	levels := map[string]string{}
	for _, it := range items {
		levels[it.Name] = it.SafetyLevel
	}
	return levels
}

func TestSetContextSafetyLevel(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}, fakeEnv{"b", newFakeAPI(t, nil)})
	if r := runCLI(t, cfg, "config", "set-context", "a", "--safety-level", "readwrite-all"); r.Code != 0 {
		t.Fatalf("%s", r)
	}
	if r := runCLI(t, cfg, "ctx", "set", "b", "--safety-level", "readwrite-mine"); r.Code != 0 {
		t.Fatalf("%s", r)
	}
	if got := contextLevels(t, cfg); got["a"] != "readwrite-all" || got["b"] != "readwrite-mine" {
		t.Errorf("levels = %v", got)
	}
	r := runCLI(t, cfg, "config", "set-context", "a", "--safety-level", "read-write")
	if r.Code != 2 || !strings.Contains(r.Stderr, "dangerously-unrestricted") {
		t.Errorf("a typo: %s", r)
	}
	if got := contextLevels(t, cfg); got["a"] != "readwrite-all" {
		t.Errorf("a typo changed the config: %v", got)
	}
}

// TestContextsDefaultToReadonly pins the default: a context without the key
// is readonly, and the listing says so.
func TestContextsDefaultToReadonly(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)})
	if got := contextLevels(t, cfg); got["a"] != "readonly" {
		t.Errorf("levels = %v", got)
	}
	if r := runCLI(t, cfg, "ctx"); !strings.Contains(r.Stdout, "SAFETY-LEVEL") || !strings.Contains(r.Stdout, "readonly") {
		t.Errorf("table:\n%s", r.Stdout)
	}
}

func TestDoctorReportsSafetyLevel(t *testing.T) {
	cfg := withFakeEnvs(t, fakeEnv{"a", versionAPI(t, "1.330.0.20251017-120000")})
	rows, r := doctorRows(t, cfg)
	if rows["a"].SafetyLevel != "readonly" {
		t.Errorf("rows %v: %s", rows, r)
	}
}
