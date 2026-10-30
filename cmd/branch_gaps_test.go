package cmd

import (
	"strings"
	"testing"
)

func runLogs(t *testing.T, body string, extra ...string) cliResult {
	t.Helper()
	api := newFakeAPI(t, map[string]route{logsPath: {Body: body}})
	args := append([]string{"query", "logs", "--query", "error", "--from", "now-1h"}, extra...)
	return runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), args...)
}

func TestQueryLogsNoRecords(t *testing.T) {
	r := runLogs(t, `{"sliceSize":0,"results":[]}`)
	if r.Code != 0 || r.Stdout != "" || !strings.Contains(r.Stderr, "No log records found.") {
		t.Errorf("no records: %s", r)
	}
}

// TestQueryLogsRecordWithoutStatus pins that a record without a status is
// printed without an empty [] between the time and the content.
func TestQueryLogsRecordWithoutStatus(t *testing.T) {
	r := runLogs(t, `{"sliceSize":1,"results":[{"timestamp":1730000000000,"content":"plain line"}]}`)
	if r.Code != 0 || r.Stdout != "[2024-10-27 03:33:20] plain line\n" {
		t.Errorf("record without status: %s", r)
	}
}

// TestQueryLogsTruncationNote pins the human note for a cut result: a
// nextSliceKey alone, and a totalCount above the records shown.
func TestQueryLogsTruncationNote(t *testing.T) {
	record := `{"timestamp":1730000000000,"status":"ERROR","content":"x"}`
	for name, body := range map[string]string{
		"slice key":   `{"sliceSize":1,"nextSliceKey":"S2","results":[` + record + `]}`,
		"total count": `{"sliceSize":1,"totalCount":5,"results":[` + record + `]}`,
	} {
		r := runLogs(t, body)
		if r.Code != 0 || !strings.Contains(r.Stderr, "Use --limit or narrower filters.") {
			t.Errorf("%s: %s", name, r)
		}
	}
	r := runLogs(t, `{"sliceSize":1,"totalCount":5,"results":[`+record+`]}`)
	if !strings.Contains(r.Stderr, "Showing 1 of 5 log records") {
		t.Errorf("total count note: %s", r)
	}
}

// TestTruncationNoteWithoutRows pins the two notes for a result whose items
// cannot be counted: part of a known total, or only that more exist.
func TestTruncationNoteWithoutRows(t *testing.T) {
	s := fetchSpec{Resource: "widgets"}
	if got := truncationNote(s, fetched{Total: 9, HasMore: true}); got != "Showing part of 9 widgets" {
		t.Errorf("known total: %q", got)
	}
	if got := truncationNote(s, fetched{Total: -1, HasMore: true}); got != "More widgets are available" {
		t.Errorf("unknown total: %q", got)
	}
}

// TestLogCountsNoEntities pins the note when the entity selector matches
// nothing: stdout stays empty, and the user learns why.
func TestLogCountsNoEntities(t *testing.T) {
	api, _ := logCountsAPI(t, `{"totalCount":0,"entities":[]}`, map[string]string{})
	r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), "query", "log-counts", "--entity", "type(SERVICE)")
	if r.Code != 0 || r.Stdout != "" || !strings.Contains(r.Stderr, "No entities found for the specified entity selector.") {
		t.Errorf("no entities: %s", r)
	}
}
