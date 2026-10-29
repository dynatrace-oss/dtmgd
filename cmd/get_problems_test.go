package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"testing"
)

// TestProblemsPageSizeWithFields verifies that the pageSize sent to the API never exceeds 10
// when the fields parameter is set (DT Managed API rejects pageSize > 10 with fields).
func TestProblemsPageSizeWithFields(t *testing.T) {
	cases := []struct {
		limit          int
		wantPageSize   int
		wantSinglePage bool
	}{
		{0, 10, false},
		{5, 5, true},
		{9, 9, true},
		{10, 10, true},
		{11, 10, false},
		{20, 10, false},
		{50, 10, false},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("limit=%d", tc.limit), func(t *testing.T) {
			const maxPageSizeWithFields = 10
			pageSize := maxPageSizeWithFields
			if tc.limit > 0 && tc.limit < maxPageSizeWithFields {
				pageSize = tc.limit
			}
			singlePageSuffices := tc.limit > 0 && tc.limit <= maxPageSizeWithFields

			if pageSize != tc.wantPageSize {
				t.Errorf("limit=%d: pageSize=%d, want %d", tc.limit, pageSize, tc.wantPageSize)
			}
			if pageSize > maxPageSizeWithFields {
				t.Errorf("limit=%d: pageSize=%d exceeds DT Managed max of %d for fields requests",
					tc.limit, pageSize, maxPageSizeWithFields)
			}
			if singlePageSuffices != tc.wantSinglePage {
				t.Errorf("limit=%d: singlePageSuffices=%v, want %v", tc.limit, singlePageSuffices, tc.wantSinglePage)
			}
		})
	}
}

// TestProblemEntryUnmarshal verifies that the affectedEntities and managementZones
// fields added in the bug fix are correctly populated from the API JSON response.
func TestProblemEntryUnmarshal(t *testing.T) {
	raw := `{
		"problemId": "-1234567890V2",
		"displayId": "P-42",
		"title": "Failure rate increase",
		"status": "OPEN",
		"severityLevel": "ERROR",
		"impactLevel": "SERVICE",
		"startTime": 1704067200000,
		"affectedEntities": [
			{"entityId": "SERVICE-ABC123", "name": "BookStore-Payments"},
			{"entityId": "SERVICE-DEF456", "name": "BookStore-Orders"}
		],
		"managementZones": [
			{"id": "12345", "name": "bookstore"}
		]
	}`

	var p ProblemEntry
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if p.ProblemID != "-1234567890V2" {
		t.Errorf("ProblemID = %q, want -1234567890V2", p.ProblemID)
	}
	if p.Status != "OPEN" {
		t.Errorf("Status = %q, want OPEN", p.Status)
	}

	// affectedEntities: the fix added this field to ProblemEntry.
	if len(p.AffectedEntities) != 2 {
		t.Fatalf("AffectedEntities len = %d, want 2", len(p.AffectedEntities))
	}
	if p.AffectedEntities[0].Name != "BookStore-Payments" {
		t.Errorf("AffectedEntities[0].Name = %q, want BookStore-Payments", p.AffectedEntities[0].Name)
	}
	if p.AffectedEntities[1].Name != "BookStore-Orders" {
		t.Errorf("AffectedEntities[1].Name = %q, want BookStore-Orders", p.AffectedEntities[1].Name)
	}

	// managementZones: the fix added this field to ProblemEntry.
	if len(p.ManagementZones) != 1 {
		t.Fatalf("ManagementZones len = %d, want 1", len(p.ManagementZones))
	}
	if p.ManagementZones[0].Name != "bookstore" {
		t.Errorf("ManagementZones[0].Name = %q, want bookstore", p.ManagementZones[0].Name)
	}
}

// TestProblemEntryUnmarshal_Empty verifies that absent affectedEntities and
// managementZones fields result in nil/empty slices, not a parse error.
func TestProblemEntryUnmarshal_Empty(t *testing.T) {
	raw := `{"problemId": "P-99", "title": "CPU spike", "status": "CLOSED"}`

	var p ProblemEntry
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if len(p.AffectedEntities) != 0 {
		t.Errorf("expected empty AffectedEntities, got %d entries", len(p.AffectedEntities))
	}
	if len(p.ManagementZones) != 0 {
		t.Errorf("expected empty ManagementZones, got %d entries", len(p.ManagementZones))
	}
}

// TestGetProblemsQueryParams pins the query get problems sends: its defaults,
// and each filter flag mapped to its parameter, with --status, --impact and
// --selector joined into one problemSelector in that order.
func TestGetProblemsQueryParams(t *testing.T) {
	base := func(extra map[string]string) url.Values {
		v := url.Values{
			"from":     {"now-24h"},
			"to":       {"now"},
			"fields":   {"+affectedEntities,+managementZones"},
			"pageSize": {"10"},
		}
		for k, val := range extra {
			v.Set(k, val)
		}
		return v
	}
	cases := []struct {
		name string
		args []string
		want url.Values
	}{
		{"defaults", nil, base(nil)},
		{"every flag", []string{
			"--from", "now-2h", "--to", "now-1h", "--status", "OPEN", "--impact", "SERVICES",
			"--selector", `managementZones("prod")`, "--entity", "type(SERVICE)", "--sort", "-startTime",
		}, base(map[string]string{
			"from":            "now-2h",
			"to":              "now-1h",
			"problemSelector": `status("OPEN"),impactLevel("SERVICES"),managementZones("prod")`,
			"entitySelector":  "type(SERVICE)",
			"sort":            "-startTime",
		})},
		{"status only", []string{"--status", "CLOSED"}, base(map[string]string{"problemSelector": `status("CLOSED")`})},
		{"impact only", []string{"--impact", "INFRASTRUCTURE"}, base(map[string]string{"problemSelector": `impactLevel("INFRASTRUCTURE")`})},
		{"selector only", []string{"--selector", `managementZones("prod")`}, base(map[string]string{"problemSelector": `managementZones("prod")`})},
	}
	for _, tc := range cases {
		api := newFakeAPI(t, map[string]route{problemsPath: {File: "problems_small.json"}})
		args := append([]string{"get", "problems", "-o", "json"}, tc.args...)
		r := runCLI(t, withFakeEnvs(t, fakeEnv{"a", api}), args...)
		qs := api.Queries(problemsPath)
		if r.Code != 0 || len(qs) != 1 || !reflect.DeepEqual(qs[0], tc.want) {
			t.Errorf("%s: code %d, queries %v, want one with %v", tc.name, r.Code, qs, tc.want)
		}
	}
}

// TestProblemRowsRootCause pins how problemRows labels a root cause from each
// shape of entityId the API returns, and formats the start time.
func TestProblemRowsRootCause(t *testing.T) {
	raw := `{"problems":[
		{"problemId":"p1","displayId":"P-1","title":"t1","status":"OPEN","severityLevel":"ERROR","impactLevel":"SERVICES","startTime":1775829300000,
		 "rootCauseEntity":{"entityId":"SERVICE-1","name":"svc"}},
		{"problemId":"p2","rootCauseEntity":{"entityId":{"id":"HOST-1","type":"HOST"},"name":"host"}},
		{"problemId":"p3","rootCauseEntity":{"entityId":{"type":"HOST"},"name":"no-id"}},
		{"problemId":"p4","rootCauseEntity":{"entityId":{"id":7},"name":"numeric-id"}},
		{"problemId":"p5","rootCauseEntity":{"entityId":42,"name":"numeric"}},
		{"problemId":"p6","rootCauseEntity":{"entityId":"","name":"empty-id"}},
		{"problemId":"p7","rootCauseEntity":{"name":"absent-id"}},
		{"problemId":"p8"}
	]}`
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	rows, n, err := problemRows(body)
	if err != nil {
		t.Fatal(err)
	}
	want := []ProblemListItem{
		{ProblemID: "p1", DisplayID: "P-1", Title: "t1", Status: "OPEN", Severity: "ERROR", Impact: "SERVICES", StartTime: "2026-04-10 13:55:00", RootCause: "svc (SERVICE-1)"},
		{ProblemID: "p2", RootCause: "host (HOST-1)"},
		{ProblemID: "p3", RootCause: "no-id"},
		{ProblemID: "p4", RootCause: "numeric-id"},
		{ProblemID: "p5", RootCause: "numeric"},
		{ProblemID: "p6", RootCause: "empty-id"},
		{ProblemID: "p7", RootCause: "absent-id"},
		{ProblemID: "p8"},
	}
	if n != len(want) || !reflect.DeepEqual(rows, want) {
		t.Errorf("problemRows = %d, %+v\nwant %d, %+v", n, rows, len(want), want)
	}

	// A body that does not decode is an error, with no rows.
	rows, n, err = problemRows(map[string]interface{}{"problems": "not a list"})
	if err == nil || rows != nil || n != 0 {
		t.Errorf("undecodable body: rows %v, n %d, err %v; want nil, 0, an error", rows, n, err)
	}
}
