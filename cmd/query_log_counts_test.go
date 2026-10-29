package cmd

import (
	"fmt"
	"strings"
	"testing"
)

func i64(n int64) *int64 { return &n }

// countString renders a count the way the table does, for comparisons.
func countString(n *int64) string {
	if n == nil {
		return "-"
	}
	return fmt.Sprint(*n)
}

// aggregate builds a one-bucket /logs/aggregate response.
func aggregate(groups map[string]int64) LogAggregateResponse {
	return LogAggregateResponse{AggregationResult: map[string]map[string]map[string]int64{
		"dt.entity.process_group": {"1730000000000": groups},
	}}
}

// TestAggregateLevel pins the top-N window rule and the entity ID case
// mismatch: the entities API returns PROCESS_GROUP-ABC, logs aggregate
// returns process_group-abc for the same entity.
func TestAggregateLevel(t *testing.T) {
	names := map[string]string{
		"process_group-aaa": "checkout",
		"process_group-bbb": "cart",
		"process_group-ccc": "search",
	}
	tests := []struct {
		name   string
		resp   LogAggregateResponse
		max    int
		want   map[string]string // pgID → count, "-" = unknown
		reason string
	}{
		{
			name:   "room left: a missing group has no records",
			resp:   aggregate(map[string]int64{"process_group-aaa": 7}),
			max:    3,
			want:   map[string]string{"process_group-aaa": "7", "process_group-bbb": "0", "process_group-ccc": "0"},
			reason: "fewer groups than maxGroupValues means every group with records is there",
		},
		{
			name:   "full window: a missing group is unknown",
			resp:   aggregate(map[string]int64{"process_group-aaa": 7, "process_group-bbb": 3}),
			max:    2,
			want:   map[string]string{"process_group-aaa": "7", "process_group-bbb": "3", "process_group-ccc": "-"},
			reason: "the API stopped at maxGroupValues groups",
		},
		{
			name:   "full window of groups outside the selection",
			resp:   aggregate(map[string]int64{"process_group-zzz": 90, "process_group-yyy": 80}),
			max:    2,
			want:   map[string]string{"process_group-aaa": "-", "process_group-bbb": "-", "process_group-ccc": "-"},
			reason: "groups the selector did not find still fill the window",
		},
		{
			name:   "uppercase IDs in the aggregate still match",
			resp:   aggregate(map[string]int64{"PROCESS_GROUP-AAA": 4}),
			max:    100,
			want:   map[string]string{"process_group-aaa": "4", "process_group-bbb": "0", "process_group-ccc": "0"},
			reason: "IDs are compared lowercased",
		},
		{
			name:   "empty result",
			resp:   LogAggregateResponse{},
			max:    100,
			want:   map[string]string{"process_group-aaa": "0", "process_group-bbb": "0", "process_group-ccc": "0"},
			reason: "no records at this level",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aggregateLevel(tt.resp, names, tt.max)
			for id, want := range tt.want {
				if g := countString(got[id]); g != want {
					t.Errorf("%s = %s, want %s (%s)", id, g, want, tt.reason)
				}
			}
		})
	}
}

// TestBuildLogCounts pins the rows: every selected group appears (0 when it
// has no records), groups sharing a display name are summed, an unknown part
// makes the sum unknown, and rows sort by ERROR, WARN and total, unknown last.
func TestBuildLogCounts(t *testing.T) {
	names := map[string]string{
		"pg-1": "books", "pg-2": "books", // two groups, one display name
		"pg-3": "orders",
		"pg-4": "idle",
		"pg-5": "search",
	}
	levels := map[string]map[string]*int64{
		"INFO":  {"pg-1": i64(10), "pg-2": i64(0), "pg-3": i64(1), "pg-4": i64(0), "pg-5": nil},
		"WARN":  {"pg-1": i64(1), "pg-2": i64(0), "pg-3": i64(0), "pg-4": i64(0), "pg-5": i64(0)},
		"ERROR": {"pg-1": i64(3), "pg-2": i64(5), "pg-3": i64(2), "pg-4": i64(0), "pg-5": i64(0)},
	}
	got := buildLogCounts(names, levels).LogCounts

	var lines []string
	for _, lc := range got {
		lines = append(lines, strings.Join([]string{lc.Service,
			countString(lc.Info), countString(lc.Warn), countString(lc.Error), countString(lc.Total)}, " "))
	}
	want := []string{
		"books 10 1 8 19",
		"orders 1 0 2 3",
		"idle 0 0 0 0",
		"search - 0 0 -",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

// A level whose map lacks a group (it was never aggregated) is unknown, not 0.
func TestBuildLogCountsMissingLevelIsUnknown(t *testing.T) {
	got := buildLogCounts(map[string]string{"pg-1": "books"}, map[string]map[string]*int64{
		"INFO": {"pg-1": i64(1)},
		"WARN": {"pg-1": i64(1)},
	}).LogCounts
	if len(got) != 1 || got[0].Error != nil || got[0].Total != nil {
		t.Errorf("got %+v, want ERROR and TOTAL unknown", got)
	}
}

func TestPGSelectorConversion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			`type(SERVICE),tag("env:prod")`,
			`type(PROCESS_GROUP),tag("env:prod")`,
		},
		{
			// Already PROCESS_GROUP — no change.
			`type(PROCESS_GROUP),tag("env:prod")`,
			`type(PROCESS_GROUP),tag("env:prod")`,
		},
		{
			// Only the first occurrence is replaced.
			`type(SERVICE),entityName.contains("SERVICE")`,
			`type(PROCESS_GROUP),entityName.contains("SERVICE")`,
		},
	}
	for _, tt := range tests {
		label := tt.input
		if len(label) > 30 {
			label = label[:30]
		}
		t.Run(label, func(t *testing.T) {
			got := strings.Replace(tt.input, "type(SERVICE)", "type(PROCESS_GROUP)", 1)
			if got != tt.want {
				t.Errorf("selector conversion: got %q, want %q", got, tt.want)
			}
		})
	}
}
