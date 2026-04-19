package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

func TestDisplayIDRegexp(t *testing.T) {
	valid := []string{"S-1", "S-281", "S-9999", "s-42", "s-0"}
	for _, id := range valid {
		if !displayIDRegexp.MatchString(id) {
			t.Errorf("expected %q to match displayIDRegexp", id)
		}
	}
	invalid := []string{"S-", "S-abc", "9767149894821966314", "SP-42", "S42", ""}
	for _, id := range invalid {
		if displayIDRegexp.MatchString(id) {
			t.Errorf("expected %q NOT to match displayIDRegexp", id)
		}
	}
}

func TestResolveSecurityProblemID_NonDisplayID(t *testing.T) {
	// Non-display IDs must be returned unchanged without calling the client.
	// We pass nil to verify no HTTP call is attempted.
	cases := []string{"9767149894821966314", "abc-uuid-123", "SP-42", ""}
	for _, id := range cases {
		got := resolveSecurityProblemID(nil, id)
		if got != id {
			t.Errorf("resolveSecurityProblemID(nil, %q) = %q, want %q", id, got, id)
		}
	}
}

// newSecurityProblemsServer starts a test HTTP server that returns a fake
// security-problems list containing the provided entries.
func newSecurityProblemsServer(t *testing.T, entries []SecurityProblemEntry) (*httptest.Server, *client.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "securityProblems") {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SecurityProblemsResponse{
			SecurityProblems: entries,
		})
	}))
	c, err := client.New(srv.URL, "env1", "tok")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return srv, c
}

func TestResolveSecurityProblemID_DisplayIDFound(t *testing.T) {
	entries := []SecurityProblemEntry{
		{SecurityProblemID: "111222333444555666", DisplayID: "S-100"},
		{SecurityProblemID: "9767149894821966314", DisplayID: "S-281"},
		{SecurityProblemID: "999888777666555444", DisplayID: "S-500"},
	}
	srv, c := newSecurityProblemsServer(t, entries)
	defer srv.Close()

	got := resolveSecurityProblemID(c, "S-281")
	if got != "9767149894821966314" {
		t.Errorf("resolveSecurityProblemID(c, %q) = %q, want %q", "S-281", got, "9767149894821966314")
	}
}

func TestResolveSecurityProblemID_DisplayIDCaseInsensitive(t *testing.T) {
	entries := []SecurityProblemEntry{
		{SecurityProblemID: "9767149894821966314", DisplayID: "S-281"},
	}
	srv, c := newSecurityProblemsServer(t, entries)
	defer srv.Close()

	// Lower-case "s-281" must resolve the same as "S-281".
	got := resolveSecurityProblemID(c, "s-281")
	if got != "9767149894821966314" {
		t.Errorf("resolveSecurityProblemID(c, %q) = %q, want %q", "s-281", got, "9767149894821966314")
	}
}

func TestResolveSecurityProblemID_DisplayIDNotFound(t *testing.T) {
	entries := []SecurityProblemEntry{
		{SecurityProblemID: "9767149894821966314", DisplayID: "S-281"},
	}
	srv, c := newSecurityProblemsServer(t, entries)
	defer srv.Close()

	// An ID not present in the list must be returned unchanged so the API
	// returns a meaningful 404.
	got := resolveSecurityProblemID(c, "S-999")
	if got != "S-999" {
		t.Errorf("resolveSecurityProblemID(c, %q) = %q, want %q (fallback)", "S-999", got, "S-999")
	}
}

func TestResolveSecurityProblemID_ClientError(t *testing.T) {
	// Server returns 404 (not retried) → GetV2Paged returns an error →
	// resolveSecurityProblemID must fall back to returning the original ID.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := client.New(srv.URL, "env1", "tok")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	got := resolveSecurityProblemID(c, "S-42")
	if got != "S-42" {
		t.Errorf("resolveSecurityProblemID on client error = %q, want %q (fallback)", got, "S-42")
	}
}

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
