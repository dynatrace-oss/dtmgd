package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSecurityProblemDetailUnmarshal(t *testing.T) {
	raw := `{
		"securityProblemId": "abc-123",
		"displayId": "S-99",
		"title": "Test vulnerability",
		"status": "OPEN",
		"technology": "JAVA",
		"packageName": "org.example:lib",
		"cveIds": ["CVE-2024-0001","CVE-2024-0002"],
		"url": "https://example.com/s/99",
		"riskAssessment": {
			"riskLevel": "HIGH",
			"riskScore": 8.5,
			"baseRiskLevel": "CRITICAL",
			"baseRiskScore": 9.8,
			"baseRiskVector": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H",
			"publicExploit": "AVAILABLE",
			"dataAssets": "REACHABLE",
			"exposure": "NOT_DETECTED"
		},
		"vulnerableComponents": [
			{
				"displayName": "org.example:lib:1.2.3",
				"numberOfAffectedEntities": 3,
				"affectedEntities": ["pid1","pid2","pid3"]
			}
		],
		"affectedEntities": ["pid1","pid2","pid3"],
		"managementZones": [
			{"id": "mz1", "name": "bookstore"}
		]
	}`

	var d SecurityProblemDetail
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if d.SecurityProblemID != "abc-123" {
		t.Errorf("SecurityProblemID: got %q, want %q", d.SecurityProblemID, "abc-123")
	}
	if d.DisplayID != "S-99" {
		t.Errorf("DisplayID: got %q, want %q", d.DisplayID, "S-99")
	}
	if len(d.CVEIds) != 2 || d.CVEIds[0] != "CVE-2024-0001" {
		t.Errorf("CVEIds: got %v", d.CVEIds)
	}
	if d.RiskAssessment.RiskLevel != "HIGH" {
		t.Errorf("RiskLevel: got %q, want HIGH", d.RiskAssessment.RiskLevel)
	}
	if d.RiskAssessment.RiskScore != 8.5 {
		t.Errorf("RiskScore: got %v, want 8.5", d.RiskAssessment.RiskScore)
	}
	if len(d.VulnerableComponents) == 0 || d.VulnerableComponents[0].DisplayName != "org.example:lib:1.2.3" {
		t.Errorf("VulnerableComponents: got %+v", d.VulnerableComponents)
	}
	if len(d.AffectedEntities) != 3 {
		t.Errorf("AffectedEntities: got %v, want 3", len(d.AffectedEntities))
	}
	if len(d.ManagementZones) == 0 || d.ManagementZones[0].Name != "bookstore" {
		t.Errorf("ManagementZones: got %+v", d.ManagementZones)
	}
}

func TestPrintSecurityProblemDetail(t *testing.T) {
	d := SecurityProblemDetail{
		SecurityProblemID: "abc-123",
		DisplayID:         "S-99",
		Title:             "Test vulnerability",
		Status:            "OPEN",
		Technology:        "JAVA",
		PackageName:       "org.example:lib",
		CVEIds:            []string{"CVE-2024-0001"},
		URL:               "https://example.com/s/99",
		AffectedEntities:  []string{"pid1", "pid2"},
	}
	d.RiskAssessment.RiskLevel = "HIGH"
	d.RiskAssessment.RiskScore = 8.5
	d.RiskAssessment.BaseRiskLevel = "CRITICAL"
	d.RiskAssessment.BaseRiskScore = 9.8
	d.RiskAssessment.PublicExploit = "NOT_AVAILABLE"
	d.RiskAssessment.DataAssets = "REACHABLE"
	d.RiskAssessment.Exposure = "NOT_DETECTED"

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printSecurityProblemDetail(d)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	checks := []string{
		"abc-123",
		"S-99",
		"CVE-2024-0001",
		"HIGH",
		"8.5",
		"REACHABLE",
		"2", // affected processes
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("printSecurityProblemDetail output missing %q\n---\n%s", want, out)
		}
	}
}
