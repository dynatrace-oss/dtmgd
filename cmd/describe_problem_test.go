package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestProblemDetailUnmarshal(t *testing.T) {
	raw := `{
		"problemId": "abc_123V2",
		"displayId": "P-99",
		"title": "Failure rate increase",
		"status": "CLOSED",
		"severityLevel": "ERROR",
		"impactLevel": "SERVICES",
		"startTime": 1775829300000,
		"endTime": 1775829600000,
		"rootCauseEntity": {
			"entityId": {"id": "SERVICE-ABC", "type": "SERVICE"},
			"name": "PaymentsController"
		},
		"impactedEntities": [
			{
				"entityId": {"id": "SERVICE-ABC", "type": "SERVICE"},
				"name": "PaymentsController"
			}
		],
		"managementZones": [
			{"id": "mz1", "name": "bookstore"},
			{"id": "mz2", "name": "[Kubernetes] aks-demo-live-arm"}
		],
		"evidenceDetails": {
			"totalCount": 2,
			"details": [
				{
					"displayName": "Failure rate",
					"evidenceType": "TRANSACTIONAL",
					"rootCauseRelevant": true,
					"startTime": 1775829300000,
					"entity": {
						"entityId": {"id": "SERVICE-ABC", "type": "SERVICE"},
						"name": "PaymentsController"
					}
				},
				{
					"displayName": "Deployment spec change",
					"evidenceType": "EVENT",
					"rootCauseRelevant": false,
					"startTime": 1775829300000,
					"entity": {
						"entityId": {"id": "APP-XYZ", "type": "CLOUD_APPLICATION"},
						"name": "payments"
					}
				}
			]
		},
		"k8s.cluster.name": ["aks-demo-live-arm"],
		"k8s.namespace.name": ["bookstore"]
	}`

	var d ProblemDetail
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if d.ProblemID != "abc_123V2" {
		t.Errorf("ProblemID: got %q", d.ProblemID)
	}
	if d.DisplayID != "P-99" {
		t.Errorf("DisplayID: got %q", d.DisplayID)
	}
	if d.Severity != "ERROR" {
		t.Errorf("Severity: got %q, want ERROR", d.Severity)
	}
	if d.RootCause == nil || d.RootCause.Name != "PaymentsController" {
		t.Errorf("RootCause: got %+v", d.RootCause)
	}
	if len(d.ImpactedEntities) != 1 || d.ImpactedEntities[0].Name != "PaymentsController" {
		t.Errorf("ImpactedEntities: got %+v", d.ImpactedEntities)
	}
	if len(d.ManagementZones) != 2 || d.ManagementZones[0].Name != "bookstore" {
		t.Errorf("ManagementZones: got %+v", d.ManagementZones)
	}
	if d.EvidenceDetails.TotalCount != 2 {
		t.Errorf("EvidenceDetails.TotalCount: got %d, want 2", d.EvidenceDetails.TotalCount)
	}
	if len(d.EvidenceDetails.Details) != 2 {
		t.Errorf("EvidenceDetails.Details len: got %d, want 2", len(d.EvidenceDetails.Details))
	}
	if d.EvidenceDetails.Details[0].RootCauseRelevant != true {
		t.Errorf("Details[0].RootCauseRelevant: got false, want true")
	}
	if len(d.K8sClusterName) != 1 || d.K8sClusterName[0] != "aks-demo-live-arm" {
		t.Errorf("K8sClusterName: got %+v", d.K8sClusterName)
	}
}

func TestFmtMillis(t *testing.T) {
	cases := []struct {
		ms   float64
		want string
	}{
		{0, "—"},
		{-1, "—"},
		{1775829300000, "2026-04-10 13:55:00 UTC"},
	}
	for _, tc := range cases {
		got := fmtMillis(tc.ms)
		if got != tc.want {
			t.Errorf("fmtMillis(%v) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}

func TestPrintProblemDetail(t *testing.T) {
	d := ProblemDetail{
		ProblemID:   "abc_123V2",
		DisplayID:   "P-99",
		Title:       "Failure rate increase",
		Status:      "CLOSED",
		Severity:    "ERROR",
		ImpactLevel: "SERVICES",
		StartTime:   1775829300000,
		EndTime:     1775829600000,
	}
	d.RootCause = &problemEntity{Name: "PaymentsController"}
	d.RootCause.EntityID.Type = "SERVICE"
	d.EvidenceDetails.TotalCount = 1
	d.EvidenceDetails.Details = []problemEvidence{
		{DisplayName: "Failure rate", EvidenceType: "TRANSACTIONAL", RootCauseRelevant: true},
	}
	d.EvidenceDetails.Details[0].Entity.Name = "PaymentsController"

	var buf bytes.Buffer
	printProblemDetail(&buf, d)
	out := buf.String()

	checks := []string{
		"abc_123V2",
		"P-99",
		"Failure rate increase",
		"ERROR",
		"CLOSED",
		"PaymentsController",
		"TRANSACTIONAL",
		"root cause",
		"2026-04-10",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("printProblemDetail output missing %q\n---\n%s", want, out)
		}
	}
}

// TestPrintProblemDetailEverySection pins printProblemDetail's exact output
// for each optional section: a root cause without a name is left out, the
// list sections join names, and evidence without an entity name shows the
// entity ID. The other cases pin what is left when every section is empty,
// and the root-cause line.
func TestPrintProblemDetailEverySection(t *testing.T) {
	full := ProblemDetail{
		ProblemID:        "p1",
		DisplayID:        "P-1",
		Title:            "Failure rate increase",
		Status:           "OPEN",
		Severity:         "ERROR",
		ImpactLevel:      "SERVICES",
		StartTime:        1775829300000,
		RootCause:        &problemEntity{EntityID: problemEntityID{ID: "SERVICE-1", Type: "SERVICE"}},
		ImpactedEntities: []problemEntity{{Name: "payments"}, {Name: ""}, {Name: "orders"}},
		ManagementZones:  []problemManagementZone{{ID: "1", Name: "bookstore"}, {ID: "2", Name: "prod"}},
		K8sClusterName:   []string{"aks-1"},
		K8sNamespaceName: []string{"shop", "infra"},
	}
	full.EvidenceDetails.TotalCount = 3
	full.EvidenceDetails.Details = []problemEvidence{
		{DisplayName: "Failure rate", EvidenceType: "TRANSACTIONAL", RootCauseRelevant: true, Entity: problemEntity{Name: "payments"}},
		{DisplayName: "CPU saturation", EvidenceType: "METRIC", Entity: problemEntity{EntityID: problemEntityID{ID: "HOST-1"}}},
	}
	minimal := ProblemDetail{ProblemID: "p2", EndTime: 1775829600000}
	minimal.EvidenceDetails.Details = []problemEvidence{{DisplayName: "ignored without a total"}}
	named := ProblemDetail{ProblemID: "p3", RootCause: &problemEntity{EntityID: problemEntityID{Type: "SERVICE"}, Name: "checkout"}}

	cases := []struct {
		name string
		d    ProblemDetail
		want string
	}{
		{"every section", full, "ID:                    p1\n" +
			"Display ID:            P-1\n" +
			"Title:                 Failure rate increase\n" +
			"Status:                OPEN\n" +
			"Severity:              ERROR\n" +
			"Impact:                SERVICES\n" +
			"Start:                 2026-04-10 13:55:00 UTC\n" +
			"\n" +
			"Impacted Entities:     payments, , orders\n" +
			"Management Zones:      bookstore, prod\n" +
			"Cluster:               aks-1\n" +
			"Namespace:             shop, infra\n" +
			"\n" +
			"Evidence (3):\n" +
			"  Failure rate                     [TRANSACTIONAL]    payments [root cause]\n" +
			"  CPU saturation                   [METRIC]           HOST-1\n"},
		{"nothing optional", minimal, "ID:                    p2\n" +
			"Display ID:            \n" +
			"Title:                 \n" +
			"Status:                \n" +
			"Severity:              \n" +
			"Impact:                \n" +
			"Start:                 —\n" +
			"End:                   2026-04-10 14:00:00 UTC\n" +
			"\n"},
		{"named root cause", named, "ID:                    p3\n" +
			"Display ID:            \n" +
			"Title:                 \n" +
			"Status:                \n" +
			"Severity:              \n" +
			"Impact:                \n" +
			"Start:                 —\n" +
			"\n" +
			"Root Cause:            checkout (SERVICE)\n"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		printProblemDetail(&buf, tc.d)
		if got := buf.String(); got != tc.want {
			t.Errorf("%s:\n--- want\n%s\n--- got\n%s", tc.name, tc.want, got)
		}
	}
}
