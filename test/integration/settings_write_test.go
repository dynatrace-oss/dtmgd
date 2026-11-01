//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// writeReport is the part of a write command's json output the checks read.
type writeReport struct {
	DryRun  bool `json:"dryRun"`
	Results []struct {
		Action   string `json:"action"`
		ObjectID string `json:"objectId"`
	} `json:"results"`
}

// testWindowPrefix names every update window the suite creates, so a run
// can find and remove what a crashed run left behind.
const testWindowPrefix = "dtmgd-it-"

// writeFile writes content to a temp file and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "object.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// windowFile is an apply file for a disabled daily test window: disabled,
// and in 2030, so it opens no update even while it exists.
func windowFile(t *testing.T, name string, duration int) string {
	return writeFile(t, fmt.Sprintf(`schemaId: %s
scope: environment
externalId: %s
value:
  enabled: false
  name: %s
  recurrence: DAILY
  dailyRecurrence:
    every: 1
    updateTime: {startTime: "03:00", timeZone: "GMT+00:00", duration: %d}
    recurrenceRange: {start: "2030-01-01T00:00:00Z", end: "2031-01-01T00:00:00Z"}
`, updateWindowsSchema, name, name, duration))
}

// applied runs a write and returns its report, failing on a non-zero exit.
func applied(t *testing.T, cfg, ctx string, args ...string) writeReport {
	t.Helper()
	r := writeRun(t, cfg, ctx, append(args, "-o", "json")...)
	if r.Code != 0 {
		t.Fatalf("%v: %s", args[:1], brief(r))
	}
	var rep writeReport
	if err := json.Unmarshal(r.Out, &rep); err != nil || len(rep.Results) != 1 {
		t.Fatalf("%v: unexpected report (%v)", args[:1], err)
	}
	return rep
}

// testWindows returns the object IDs of the update windows named with
// testWindowPrefix (or exactly name, when given).
func testWindows(t *testing.T, ctx, name string) []string {
	t.Helper()
	body := decode[map[string]any](t, "get", "settings", "--schema", updateWindowsSchema, "--scope", "environment", "--context", ctx, "-o", "json")
	var ids []string
	for _, item := range items(t, body, "items") {
		m, _ := item.(map[string]any)
		value, _ := m["value"].(map[string]any)
		n, _ := value["name"].(string)
		if (name == "" && strings.HasPrefix(n, testWindowPrefix)) || (name != "" && n == name) {
			id, _ := m["objectId"].(string)
			ids = append(ids, id)
		}
	}
	return ids
}

// TestReadonlyContextBlocksALiveWrite runs a write through the original
// config, where the write context is readonly: dtmgd refuses it (exit 6)
// before sending anything.
func TestReadonlyContextBlocksALiveWrite(t *testing.T) {
	_, ctx := writeConfig(t)
	orig := requireEnv(t, "DTMGD_IT_CONFIG")
	cfg, err := config.LoadFrom(orig)
	if err != nil {
		t.Fatal(err)
	}
	if nc, err := cfg.GetContext(ctx); err != nil || nc.Context.SafetyLevel != "" && nc.Context.SafetyLevel != string(config.SafetyReadOnly) {
		t.Skipf("the write context is not readonly in the config itself, so this check would write")
	}
	r := writeRun(t, orig, ctx, "apply", "-f", windowFile(t, testWindowPrefix+"blocked", 60))
	if r.Code != 6 {
		t.Errorf("apply through a readonly context: %s, want exit 6", brief(r))
	}
}

// TestUpdateWindowLifecycle creates, updates, reads and deletes a test
// window, and nothing else. Cleanup runs even when a step fails.
func TestUpdateWindowLifecycle(t *testing.T) {
	cfg, ctx := writeConfig(t)
	for _, id := range testWindows(t, ctx, "") {
		writeRun(t, cfg, ctx, "delete", "settings", id, "--yes")
	}
	name := fmt.Sprintf("%s%d", testWindowPrefix, time.Now().Unix())

	created := applied(t, cfg, ctx, "apply", "-f", windowFile(t, name, 60))
	id := created.Results[0].ObjectID
	if created.Results[0].Action != "created" || id == "" {
		t.Fatalf("first apply: action %q", created.Results[0].Action)
	}
	t.Cleanup(func() {
		if len(testWindows(t, ctx, name)) > 0 {
			writeRun(t, cfg, ctx, "delete", "settings", id, "--yes")
		}
	})
	if got := testWindows(t, ctx, name); len(got) != 1 || got[0] != id {
		t.Fatalf("the new window is listed %d times", len(got))
	}
	if a := applied(t, cfg, ctx, "apply", "-f", windowFile(t, name, 90)).Results[0].Action; a != "updated" {
		t.Errorf("apply with a new duration: action %q, want updated", a)
	}
	if a := applied(t, cfg, ctx, "apply", "-f", windowFile(t, name, 90)).Results[0].Action; a != "unchanged" {
		t.Errorf("the same apply again: action %q, want unchanged", a)
	}
	effective := decode[map[string]any](t, "get", "settings", "--effective", "--scope", "environment", "--schema", updateWindowsSchema, "--context", ctx, "-o", "json")
	found := false
	for _, item := range items(t, effective, "items") {
		m, _ := item.(map[string]any)
		value, _ := m["value"].(map[string]any)
		found = found || value["name"] == name
	}
	if !found {
		t.Error("the window is not among the effective values")
	}
	if a := applied(t, cfg, ctx, "delete", "settings", id, "--yes").Results[0].Action; a != "deleted" {
		t.Errorf("delete: action %q", a)
	}
	if left := testWindows(t, ctx, name); len(left) != 0 {
		t.Errorf("the window is still listed after the delete")
	}
}

// TestUpdateSettingsAreOnlyValidated changes the OneAgent and ActiveGate
// update modes with --validate-only and --dry-run only: the cluster checks
// the change and saves nothing.
func TestUpdateSettingsAreOnlyValidated(t *testing.T) {
	cfg, ctx := writeConfig(t)
	for _, schema := range []string{oneAgentSchema, activeGateSchema} {
		t.Run(schema, func(t *testing.T) { checkOnlyValidated(t, cfg, ctx, schema) })
	}
}

// checkOnlyValidated validates and dry-runs a change of schema's update
// mode at the environment, then checks that nothing changed.
func checkOnlyValidated(t *testing.T, cfg, ctx, schema string) {
	t.Helper()
	before := effectiveValue(t, ctx, schema)
	body, _ := json.Marshal(map[string]any{"schemaId": schema, "scope": "environment", "value": withOtherMode(before)})
	file := writeFile(t, string(body))
	if a := applied(t, cfg, ctx, "apply", "-f", file, "--validate-only").Results[0].Action; a != "validated" {
		t.Errorf("--validate-only: action %q", a)
	}
	if rep := applied(t, cfg, ctx, "apply", "-f", file, "--dry-run"); !rep.DryRun {
		t.Error("--dry-run: report is not marked as a dry run")
	}
	if after := effectiveValue(t, ctx, schema); !reflect.DeepEqual(before, after) {
		t.Error("the effective value changed")
	}
}

// withOtherMode is a copy of an update setting's value with another update
// mode: MANUAL, or AUTOMATIC when it already is MANUAL.
func withOtherMode(value map[string]any) map[string]any {
	changed := make(map[string]any, len(value))
	for k, v := range value {
		changed[k] = v
	}
	changed["updateMode"] = "MANUAL"
	if value["updateMode"] == "MANUAL" {
		changed["updateMode"] = "AUTOMATIC"
	}
	return changed
}

// effectiveValue is schema's effective value at the environment.
func effectiveValue(t *testing.T, ctx, schema string) map[string]any {
	t.Helper()
	body := decode[map[string]any](t, "get", "settings", "--effective", "--scope", "environment", "--schema", schema, "--context", ctx, "-o", "json")
	list := items(t, body, "items")
	if len(list) != 1 {
		t.Fatalf("%d effective values, want 1", len(list))
	}
	m, _ := list[0].(map[string]any)
	value, _ := m["value"].(map[string]any)
	return value
}

// TestClusterDryRunPlans pins that a settings write works on a cluster
// context: a dry run there plans a create (GETs only) through the Cluster
// API. A token without settings.read skips it.
func TestClusterDryRunPlans(t *testing.T) {
	cfg, _ := writeConfig(t)
	cluster := requireEnv(t, "DTMGD_IT_CLUSTER")
	r := writeRun(t, cfg, cluster, "apply", "-f", windowFile(t, testWindowPrefix+"cluster", 60), "--dry-run")
	skipWithoutScope(t, r, []string{"apply", "settings"})
	if r.Code != 0 {
		t.Errorf("a dry run against a cluster context: %s, want exit 0", brief(r))
	}
}
