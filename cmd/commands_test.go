package cmd

import (
	"encoding/json"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// commandsBrief and commandsFull decode just the parts of the catalog the
// tests below check.
type commandsBrief struct {
	SchemaVersion int                 `json:"schema_version"`
	Tool          string              `json:"tool"`
	Verbs         map[string][]string `json:"verbs"`
}

type commandsFull struct {
	SchemaVersion int                        `json:"schema_version"`
	CommandModel  string                     `json:"command_model"`
	GlobalFlags   map[string]catalogFlagJSON `json:"global_flags"`
	Commands      []struct {
		Path           string                     `json:"path"`
		Aliases        []string                   `json:"aliases"`
		Short          string                     `json:"short"`
		RequiredScopes []string                   `json:"required_scopes"`
		Flags          map[string]catalogFlagJSON `json:"flags"`
	} `json:"commands"`
}

type catalogFlagJSON struct {
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

// TestCommandsBriefListsResources pins the brief catalog: each verb lists its
// resources, and cobra's generated help command is not one of them.
func TestCommandsBriefListsResources(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}), "commands", "--brief")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	var got commandsBrief
	if err := json.Unmarshal([]byte(r.Stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, r.Stdout)
	}
	if got.SchemaVersion != 1 || got.Tool != "dtmgd" {
		t.Errorf("header = %d %q", got.SchemaVersion, got.Tool)
	}
	for _, want := range []string{"problems", "security-problems"} {
		if !slices.Contains(got.Verbs["get"], want) {
			t.Errorf("verbs.get = %v, missing %q", got.Verbs["get"], want)
		}
	}
	for verb, names := range got.Verbs {
		if slices.Contains(names, "help") {
			t.Errorf("verbs.%s lists help: %v", verb, names)
		}
		if !slices.IsSorted(names) {
			t.Errorf("verbs.%s is not sorted: %v", verb, names)
		}
	}
	if len(got.Verbs["describe"]) == 0 || len(got.Verbs["query"]) == 0 {
		t.Errorf("verbs = %v, want describe and query too", got.Verbs)
	}
}

// TestCommandsFullHasScopesAndFlags pins that the full catalog carries each
// command's declared token scopes and its own flags, and leaves out the
// groups, cobra's help and completion, and the catalog itself.
func TestCommandsFullHasScopesAndFlags(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}), "commands")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	var got commandsFull
	if err := json.Unmarshal([]byte(r.Stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, r.Stdout)
	}
	if got.SchemaVersion != 1 || got.CommandModel != "verb-noun" {
		t.Errorf("header = %d %q", got.SchemaVersion, got.CommandModel)
	}
	if _, ok := got.GlobalFlags["--output"]; !ok {
		t.Errorf("global_flags has no --output: %v", got.GlobalFlags)
	}

	paths := map[string]bool{}
	found := false
	for _, c := range got.Commands {
		paths[c.Path] = true
		if c.Path != "get problems" {
			continue
		}
		found = true
		if !slices.Equal(c.RequiredScopes, []string{"problems.read"}) {
			t.Errorf("get problems required_scopes = %v", c.RequiredScopes)
		}
		if _, ok := c.Flags["--status"]; !ok {
			t.Errorf("get problems flags = %v, missing --status", c.Flags)
		}
		if _, ok := c.Flags["--output"]; ok {
			t.Error("get problems lists the inherited --output flag")
		}
		if !slices.Contains(c.Aliases, "prob") {
			t.Errorf("get problems aliases = %v", c.Aliases)
		}
	}
	if !found {
		t.Fatalf("no get problems entry in %v", paths)
	}
	for _, want := range []string{"doctor", "version", "ctx", "config view", "skills install"} {
		if !paths[want] {
			t.Errorf("catalog is missing %q", want)
		}
	}
	for _, unwanted := range []string{"get", "describe", "query", "config", "skills", "help", "completion", "commands"} {
		if paths[unwanted] {
			t.Errorf("catalog lists %q", unwanted)
		}
	}
}

// TestCommandsNotEnveloped pins that the catalog is never wrapped in an
// agent envelope: an agent reads it to learn the envelope, not through one.
func TestCommandsNotEnveloped(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}), "commands", "-A")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	var top map[string]interface{}
	if err := json.Unmarshal([]byte(r.Stdout), &top); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, r.Stdout)
	}
	if _, ok := top["schema_version"]; !ok {
		t.Errorf("no schema_version at top level: %v", top)
	}
	if _, ok := top["ok"]; ok {
		t.Errorf("catalog is enveloped: %v", top)
	}
}

// TestCommandsYAML pins that -o yaml gives the catalog as YAML.
func TestCommandsYAML(t *testing.T) {
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", newFakeAPI(t, nil)}), "commands", "--brief", "-o", "yaml")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr %q", r.Code, r.Stderr)
	}
	var top map[string]interface{}
	if err := yaml.Unmarshal([]byte(r.Stdout), &top); err != nil {
		t.Fatalf("stdout is not YAML: %v\n%s", err, r.Stdout)
	}
	if top["schema_version"] != 1 || top["verbs"] == nil {
		t.Errorf("YAML catalog = %v", top)
	}
}
