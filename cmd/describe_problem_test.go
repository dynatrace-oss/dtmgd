package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
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
	d.RootCause = &struct {
		EntityID struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"entityId"`
		Name string `json:"name"`
	}{Name: "PaymentsController"}
	d.RootCause.EntityID.Type = "SERVICE"
	d.EvidenceDetails.TotalCount = 1
	d.EvidenceDetails.Details = []struct {
		DisplayName       string  `json:"displayName"`
		EvidenceType      string  `json:"evidenceType"`
		RootCauseRelevant bool    `json:"rootCauseRelevant"`
		StartTime         float64 `json:"startTime"`
		Entity            struct {
			EntityID struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"entityId"`
			Name string `json:"name"`
		} `json:"entity"`
	}{
		{DisplayName: "Failure rate", EvidenceType: "TRANSACTIONAL", RootCauseRelevant: true},
	}
	d.EvidenceDetails.Details[0].Entity.Name = "PaymentsController"

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	printProblemDetail(d)
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
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
