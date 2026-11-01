//go:build integration

package integration

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

func TestClusterSettingsSchemasRead(t *testing.T) {
	body := decode[map[string]any](t, "get", "settings-schemas", "--search", "update-windows", "--context", cluster(t), "-o", "json")
	var ids []string
	for _, item := range items(t, body, "items") {
		m, _ := item.(map[string]any)
		id, _ := m["schemaId"].(string)
		ids = append(ids, id)
	}
	if !slices.Contains(ids, updateWindowsSchema) {
		t.Errorf("the cluster lists no update-window schema (%d matches)", len(ids))
	}
}

func TestClusterUpdateWindowsDecode(t *testing.T) {
	body := decode[map[string]any](t, "get", "settings", "--schema", updateWindowsSchema, "--scope", "cluster", "--context", cluster(t), "-o", "json")
	for i, item := range items(t, body, "items") {
		m, _ := item.(map[string]any)
		value, _ := m["value"].(map[string]any)
		if r, _ := value["recurrence"].(string); !slices.Contains([]string{"ONCE", "DAILY", "WEEKLY", "MONTHLY"}, r) {
			t.Errorf("window %d has recurrence %q", i, r)
		}
	}
}

// TestActiveGateUpdatesRead reads the cluster ActiveGates' auto-update;
// it reports enum values and counts only, never host names.
func TestActiveGateUpdatesRead(t *testing.T) {
	body := decode[map[string]any](t, "get", "activegate-updates", "--context", cluster(t), "-o", "json")
	global, _ := body["global"].(map[string]any)
	if s := global["globalSetting"]; s != "ENABLED" && s != "DISABLED" {
		t.Errorf("globalSetting = %v", s)
	}
	gates, _ := body["activeGates"].([]any)
	for i, g := range gates {
		if m, _ := g.(map[string]any); m["type"] != "CLUSTER" {
			t.Errorf("ActiveGate %d has type %v, want CLUSTER", i, m["type"])
		}
	}
}

// TestClusterWindowLifecycle creates, updates and deletes a disabled test
// window at cluster scope, and nothing else.
func TestClusterWindowLifecycle(t *testing.T) {
	cfg, ctx := writeConfigFor(t, "DTMGD_IT_WRITE_CLUSTER")
	for _, id := range testWindowsAt(t, ctx, "cluster", "") {
		writeRun(t, cfg, ctx, "delete", "settings", id, "--yes")
	}
	name := fmt.Sprintf("%s%d", testWindowPrefix, time.Now().Unix())
	created := applied(t, cfg, ctx, "apply", "-f", windowFileAt(t, "cluster", name, 60))
	id := created.Results[0].ObjectID
	if created.Results[0].Action != "created" || id == "" {
		t.Fatalf("first apply: action %q", created.Results[0].Action)
	}
	t.Cleanup(func() {
		if len(testWindowsAt(t, ctx, "cluster", name)) > 0 {
			writeRun(t, cfg, ctx, "delete", "settings", id, "--yes")
		}
	})
	if a := applied(t, cfg, ctx, "apply", "-f", windowFileAt(t, "cluster", name, 90)).Results[0].Action; a != "updated" {
		t.Errorf("apply with a new duration: action %q, want updated", a)
	}
	if a := applied(t, cfg, ctx, "delete", "settings", id, "--yes").Results[0].Action; a != "deleted" {
		t.Errorf("delete: action %q", a)
	}
	if left := testWindowsAt(t, ctx, "cluster", name); len(left) != 0 {
		t.Error("the window is still listed after the delete")
	}
}

// TestActiveGateUpdatesOnlyValidated flips the global auto-update setting
// with --validate-only and --dry-run only: nothing is saved.
func TestActiveGateUpdatesOnlyValidated(t *testing.T) {
	cfg, ctx := writeConfigFor(t, "DTMGD_IT_WRITE_CLUSTER")
	before := decode[map[string]any](t, "describe", "activegate-updates", "--context", ctx, "-o", "json")
	flipped := "DISABLED"
	if before["globalSetting"] == "DISABLED" {
		flipped = "ENABLED"
	}
	file := writeFile(t, "globalSetting: "+flipped+"\n")
	if a := applied(t, cfg, ctx, "update", "activegate-updates", "-f", file, "--validate-only").Results[0].Action; a != "validated" {
		t.Errorf("--validate-only: action %q", a)
	}
	if rep := applied(t, cfg, ctx, "update", "activegate-updates", "-f", file, "--dry-run"); !rep.DryRun {
		t.Error("--dry-run: report is not marked as a dry run")
	}
	after := decode[map[string]any](t, "describe", "activegate-updates", "--context", ctx, "-o", "json")
	if before["globalSetting"] != after["globalSetting"] {
		t.Error("globalSetting changed")
	}
}
