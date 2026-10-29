package cmd

import (
	"encoding/json"
	"net/url"
	"reflect"
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

// TestQueryLogsFlagChecksAndErrors pins query logs' usage errors, checked
// before any request and --query first, and how an API error is reported.
func TestQueryLogsFlagChecksAndErrors(t *testing.T) {
	queryRequired := "== exit code: 2\n== stdout\n== stderr\nError: --query is required (e.g. \"error\")\n"
	cases := []struct {
		name string
		args []string
		hits int // requests sent
		want string
	}{
		{"neither flag", nil, 0, queryRequired},
		{"no --query", []string{"--from", "now-1h"}, 0, queryRequired},
		{"no --from", []string{"--query", "error"}, 0, "== exit code: 2\n== stdout\n== stderr\nError: --from is required (e.g. now-1h)\n"},
		{"API error", []string{"--query", "error", "--from", "now-1h"}, 1, "== exit code: 5\n" +
			"== stdout\n" +
			"== stderr\n" +
			"Error: API error 403: Token is missing required scope\n" +
			"Hint: Permission denied. Your API token may be missing required scopes.\n" +
			"  This command needs the token scope(s): logs.read.\n"},
	}
	for _, tc := range cases {
		api := newFakeAPI(t, map[string]route{logsPath: {Status: 403, File: "error_403.json"}})
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), append([]string{"query", "logs"}, tc.args...)...)
		if got := r.String(); got != tc.want {
			t.Errorf("%s:\n--- want\n%s\n--- got\n%s", tc.name, tc.want, got)
		}
		if got := api.Hits(logsPath); got != tc.hits {
			t.Errorf("%s: %d requests, want %d", tc.name, got, tc.hits)
		}
	}
}

// TestQueryLogsQueryParams pins the /logs/search query: --to defaults to now,
// and --sort and --entity are sent only when given.
func TestQueryLogsQueryParams(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want url.Values
	}{
		{"defaults", nil, url.Values{"query": {"error"}, "from": {"now-1h"}, "to": {"now"}, "limit": {"100"}}},
		{"every flag", []string{"--to", "now-5m", "--sort", "-timestamp", "--entity", "type(PROCESS_GROUP)"}, url.Values{
			"query": {"error"}, "from": {"now-1h"}, "to": {"now-5m"}, "limit": {"100"},
			"sort": {"-timestamp"}, "entitySelector": {"type(PROCESS_GROUP)"},
		}},
	}
	for _, tc := range cases {
		api := newFakeAPI(t, map[string]route{logsPath: {File: "logs.json"}})
		args := append([]string{"query", "logs", "--query", "error", "--from", "now-1h"}, tc.args...)
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), args...)
		qs := api.Queries(logsPath)
		if r.Code != 0 || len(qs) != 1 || !reflect.DeepEqual(qs[0], tc.want) {
			t.Errorf("%s: code %d, queries %v, want one with %v", tc.name, r.Code, qs, tc.want)
		}
	}
}
