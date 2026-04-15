package cmd

import (
	"encoding/json"
	"testing"
)

// TestResolveLogContent_DirectContent verifies that Content field takes priority.
func TestResolveLogContent_DirectContent(t *testing.T) {
	entry := LogEntry{
		Content: "direct log message",
		Event:   map[string]interface{}{"content": "event content"},
	}
	got := resolveLogContent(entry)
	if got != "direct log message" {
		t.Errorf("resolveLogContent = %q, want %q", got, "direct log message")
	}
}

// TestResolveLogContent_FallbackToEvent verifies the Event["content"] fallback
// used on DT Managed Classic where Content is empty.
func TestResolveLogContent_FallbackToEvent(t *testing.T) {
	entry := LogEntry{
		Content: "",
		Event:   map[string]interface{}{"content": "fallback from event"},
	}
	got := resolveLogContent(entry)
	if got != "fallback from event" {
		t.Errorf("resolveLogContent = %q, want %q", got, "fallback from event")
	}
}

// TestResolveLogContent_BothEmpty verifies that empty entry returns empty string.
func TestResolveLogContent_BothEmpty(t *testing.T) {
	entry := LogEntry{Content: "", Event: map[string]interface{}{}}
	got := resolveLogContent(entry)
	if got != "" {
		t.Errorf("resolveLogContent = %q, want empty string", got)
	}
}

// TestResolveLogContent_NilEvent verifies that nil Event map doesn't panic.
func TestResolveLogContent_NilEvent(t *testing.T) {
	entry := LogEntry{Content: "msg", Event: nil}
	got := resolveLogContent(entry)
	if got != "msg" {
		t.Errorf("resolveLogContent = %q, want %q", got, "msg")
	}
}

// TestResolveLogContent_EventContentIsNonString verifies fmt.Sprintf("%v") fallback
// for non-string event content values.
func TestResolveLogContent_EventContentIsNonString(t *testing.T) {
	entry := LogEntry{
		Content: "",
		Event:   map[string]interface{}{"content": 42},
	}
	got := resolveLogContent(entry)
	if got != "42" {
		t.Errorf("resolveLogContent = %q, want \"42\"", got)
	}
}

// TestResolveLogStatus_DirectStatus verifies Status field takes priority.
func TestResolveLogStatus_DirectStatus(t *testing.T) {
	entry := LogEntry{
		Status: "ERROR",
		Event:  map[string]interface{}{"status": "INFO"},
	}
	got := resolveLogStatus(entry)
	if got != "ERROR" {
		t.Errorf("resolveLogStatus = %q, want ERROR", got)
	}
}

// TestResolveLogStatus_FallbackToEvent verifies the Event["status"] fallback.
func TestResolveLogStatus_FallbackToEvent(t *testing.T) {
	entry := LogEntry{
		Status: "",
		Event:  map[string]interface{}{"status": "WARN"},
	}
	got := resolveLogStatus(entry)
	if got != "WARN" {
		t.Errorf("resolveLogStatus = %q, want WARN", got)
	}
}

// TestResolveLogStatus_BothEmpty verifies empty status returns empty string (no prefix in output).
func TestResolveLogStatus_BothEmpty(t *testing.T) {
	entry := LogEntry{Status: "", Event: map[string]interface{}{}}
	got := resolveLogStatus(entry)
	if got != "" {
		t.Errorf("resolveLogStatus = %q, want empty string", got)
	}
}

// TestResolveLogStatus_NilEvent verifies no panic when Event is nil.
func TestResolveLogStatus_NilEvent(t *testing.T) {
	entry := LogEntry{Status: "INFO", Event: nil}
	got := resolveLogStatus(entry)
	if got != "INFO" {
		t.Errorf("resolveLogStatus = %q, want INFO", got)
	}
}

// TestLogEntryUnmarshal verifies JSON deserialization of LogEntry.
func TestLogEntryUnmarshal(t *testing.T) {
	raw := `{
		"timestamp": 1744660939001,
		"status": "ERROR",
		"content": "403 FORBIDDEN Purchase was rejected",
		"event": {"k8s.pod.name": "payments-xyz"}
	}`
	var entry LogEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if entry.Timestamp != 1744660939001 {
		t.Errorf("Timestamp = %d", entry.Timestamp)
	}
	if entry.Status != "ERROR" {
		t.Errorf("Status = %q", entry.Status)
	}
	if entry.Content != "403 FORBIDDEN Purchase was rejected" {
		t.Errorf("Content = %q", entry.Content)
	}
}
