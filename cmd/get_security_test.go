package cmd

import (
	"strings"
	"testing"
)

// TestSecurityEntryToListItem_NoCVEs verifies that an entry with no CVEs
// produces an empty CVEs field (not a panic or "+0" suffix).
func TestSecurityEntryToListItem_NoCVEs(t *testing.T) {
	sp := SecurityProblemEntry{
		SecurityProblemID: "S-1",
		DisplayID:         "S-1",
		PackageName:       "log4j",
		Technology:        "JAVA",
		Status:            "OPEN",
		CveIds:            nil,
	}
	item := securityEntryToListItem(sp)
	if item.CVEs != "" {
		t.Errorf("CVEs = %q, want empty string for entry with no CVEs", item.CVEs)
	}
}

// TestSecurityEntryToListItem_SingleCVE verifies that a single CVE is shown as-is.
func TestSecurityEntryToListItem_SingleCVE(t *testing.T) {
	sp := SecurityProblemEntry{
		SecurityProblemID: "S-2",
		DisplayID:         "S-2",
		CveIds:            []string{"CVE-2024-1597"},
		Status:            "OPEN",
	}
	item := securityEntryToListItem(sp)
	if item.CVEs != "CVE-2024-1597" {
		t.Errorf("CVEs = %q, want %q", item.CVEs, "CVE-2024-1597")
	}
	if strings.Contains(item.CVEs, "+") {
		t.Errorf("CVEs should not contain '+' for single CVE, got %q", item.CVEs)
	}
}

// TestSecurityEntryToListItem_MultiCVE verifies truncation: first CVE + " +N more".
func TestSecurityEntryToListItem_MultiCVE(t *testing.T) {
	sp := SecurityProblemEntry{
		SecurityProblemID: "S-3",
		DisplayID:         "S-3",
		CveIds:            []string{"CVE-2024-1111", "CVE-2024-2222", "CVE-2024-3333"},
		Status:            "OPEN",
	}
	item := securityEntryToListItem(sp)
	want := "CVE-2024-1111 +2"
	if item.CVEs != want {
		t.Errorf("CVEs = %q, want %q", item.CVEs, want)
	}
}

// TestSecurityEntryToListItem_RiskAssessmentNil verifies nil-safety for risk fields.
func TestSecurityEntryToListItem_RiskAssessmentNil(t *testing.T) {
	sp := SecurityProblemEntry{
		SecurityProblemID: "S-4",
		Status:            "OPEN",
		RiskAssessment:    nil,
	}
	item := securityEntryToListItem(sp)
	if item.RiskLevel != "" {
		t.Errorf("RiskLevel = %q, want empty for nil RiskAssessment", item.RiskLevel)
	}
	if item.RiskScore != "" {
		t.Errorf("RiskScore = %q, want empty for nil RiskAssessment", item.RiskScore)
	}
}

// TestSecurityEntryToListItem_RiskScore verifies score is formatted to 1 decimal.
func TestSecurityEntryToListItem_RiskScore(t *testing.T) {
	sp := SecurityProblemEntry{
		SecurityProblemID: "S-5",
		Status:            "OPEN",
		RiskAssessment: &struct {
			RiskLevel string  `json:"riskLevel"`
			RiskScore float64 `json:"riskScore"`
		}{RiskLevel: "CRITICAL", RiskScore: 9.7},
	}
	item := securityEntryToListItem(sp)
	if item.RiskLevel != "CRITICAL" {
		t.Errorf("RiskLevel = %q, want CRITICAL", item.RiskLevel)
	}
	if item.RiskScore != "9.7" {
		t.Errorf("RiskScore = %q, want \"9.7\"", item.RiskScore)
	}
}

// TestSecurityEntryToListItem_FieldMapping verifies all fields are correctly mapped.
func TestSecurityEntryToListItem_FieldMapping(t *testing.T) {
	sp := SecurityProblemEntry{
		SecurityProblemID: "S-100",
		DisplayID:         "S-100-display",
		PackageName:       "postgresql:42.3.8",
		Technology:        "JAVA",
		Status:            "OPEN",
		CveIds:            []string{"CVE-2024-1597"},
		RiskAssessment: &struct {
			RiskLevel string  `json:"riskLevel"`
			RiskScore float64 `json:"riskScore"`
		}{RiskLevel: "HIGH", RiskScore: 8.8},
	}
	item := securityEntryToListItem(sp)
	if item.SecurityID != "S-100" {
		t.Errorf("SecurityID = %q", item.SecurityID)
	}
	if item.DisplayID != "S-100-display" {
		t.Errorf("DisplayID = %q", item.DisplayID)
	}
	if item.PackageName != "postgresql:42.3.8" {
		t.Errorf("PackageName = %q", item.PackageName)
	}
	if item.Technology != "JAVA" {
		t.Errorf("Technology = %q", item.Technology)
	}
	if item.Status != "OPEN" {
		t.Errorf("Status = %q", item.Status)
	}
}
