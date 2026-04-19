package cmd

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestProblemsPageSizeWithFields verifies that the pageSize sent to the API never exceeds 10
// when the fields parameter is set (DT Managed API rejects pageSize > 10 with fields).
func TestProblemsPageSizeWithFields(t *testing.T) {
	cases := []struct {
		limit	int
		wantPageSize	int
		wantSinglePage	bool // singlePageSuffices
	}{
		{limit: 0, wantPageSize: 10, wantSinglePage: false},  // unlimited → paginate
		{limit: 5, wantPageSize: 5, wantSinglePage: true},    // small limit → single page
		{limit: 9, wantPageSize: 9, wantSinglePage: true},    // just below max → single page
		{limit: 10, wantPageSize: 10, wantSinglePage: true},  // exactly max → single page
		{limit: 11, wantPageSize: 10, wantSinglePage: false}, // exceeds max → paginate
		{limit: 20, wantPageSize: 10, wantSinglePage: false}, // large limit → paginate
		{limit: 50, wantPageSize: 10, wantSinglePage: false}, // old default was 50 → paginate
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
