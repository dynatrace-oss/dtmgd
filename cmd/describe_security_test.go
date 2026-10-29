package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"

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
		got, err := resolveSecurityProblemID(nil, id, 1, io.Discard)
		if err != nil || got != id {
			t.Errorf("resolveSecurityProblemID(nil, %q) = %q, %v; want %q, nil", id, got, err, id)
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

	got, err := resolveSecurityProblemID(c, "S-281", 0, io.Discard)
	if err != nil || got != "9767149894821966314" {
		t.Errorf("resolveSecurityProblemID(c, %q) = %q, %v; want %q, nil", "S-281", got, err, "9767149894821966314")
	}
}

func TestResolveSecurityProblemID_DisplayIDCaseInsensitive(t *testing.T) {
	entries := []SecurityProblemEntry{
		{SecurityProblemID: "9767149894821966314", DisplayID: "S-281"},
	}
	srv, c := newSecurityProblemsServer(t, entries)
	defer srv.Close()

	// Lower-case "s-281" must resolve the same as "S-281".
	got, err := resolveSecurityProblemID(c, "s-281", 0, io.Discard)
	if err != nil || got != "9767149894821966314" {
		t.Errorf("resolveSecurityProblemID(c, %q) = %q, %v; want %q, nil", "s-281", got, err, "9767149894821966314")
	}
}

func TestResolveSecurityProblemID_DisplayIDNotFound(t *testing.T) {
	entries := []SecurityProblemEntry{
		{SecurityProblemID: "9767149894821966314", DisplayID: "S-281"},
	}
	srv, c := newSecurityProblemsServer(t, entries)
	defer srv.Close()

	// A display ID that no security problem has is a not-found error: exit
	// code 4 and agent code not_found, like the API's 404 for an unknown UUID.
	got, err := resolveSecurityProblemID(c, "S-999", 0, io.Discard)
	if err == nil {
		t.Fatalf("resolveSecurityProblemID(c, %q) = %q, nil; want a not-found error", "S-999", got)
	}
	if err.Error() != "security problem S-999 not found" {
		t.Errorf("error = %q, want %q", err, "security problem S-999 not found")
	}
	if code := exitCode(err); code != client.ExitNotFound {
		t.Errorf("exitCode = %d, want %d", code, client.ExitNotFound)
	}
	if d := errorDetail(err, ""); d.Code != "not_found" {
		t.Errorf("agent code = %q, want not_found", d.Code)
	}
}

func TestResolveSecurityProblemID_ClientError(t *testing.T) {
	// The listing itself failing (404, not retried) is reported, not
	// swallowed: the display ID is never sent on as if it were a UUID.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := client.New(srv.URL, "env1", "tok")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	got, err := resolveSecurityProblemID(c, "S-42", 0, io.Discard)
	var apiErr *client.ErrAPI
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Errorf("resolveSecurityProblemID on client error = %q, %v; want the API 404", got, err)
	}
}

// securityProblemPages is a fake /securityProblems listing of three pages
// linked by nextPageKey; S-1 is on page 1 and S-3 on page 3.
func securityProblemPages() map[string]route {
	const list = apiV2 + "/securityProblems"
	return map[string]route{
		list:                     {Body: `{"totalCount":3,"pageSize":1,"nextPageKey":"p2","securityProblems":[{"securityProblemId":"UUID-1","displayId":"S-1"}]}`},
		list + " nextPageKey=p2": {Body: `{"totalCount":3,"pageSize":1,"nextPageKey":"p3","securityProblems":[{"securityProblemId":"UUID-2","displayId":"S-2"}]}`},
		list + " nextPageKey=p3": {Body: `{"totalCount":3,"pageSize":1,"securityProblems":[{"securityProblemId":"UUID-3","displayId":"S-3"}]}`},
	}
}

func newFakeClient(t *testing.T, api *fakeAPI) *client.Client {
	t.Helper()
	c, err := client.New(api.URL, "env1", "tok")
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return c
}

// TestResolveStopsAtFirstMatchingPage: a match on page 1 costs one request,
// not a walk over every page.
func TestResolveStopsAtFirstMatchingPage(t *testing.T) {
	api := newFakeAPI(t, securityProblemPages())
	got, err := resolveSecurityProblemID(newFakeClient(t, api), "S-1", 0, io.Discard)
	if err != nil || got != "UUID-1" {
		t.Fatalf("resolve S-1 = %q, %v; want UUID-1, nil", got, err)
	}
	if n := api.Hits(apiV2 + "/securityProblems"); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

// TestResolveFollowsNextPageKey: the first request asks for 500 per page and
// later ones send only nextPageKey, as the API requires.
func TestResolveFollowsNextPageKey(t *testing.T) {
	api := newFakeAPI(t, securityProblemPages())
	got, err := resolveSecurityProblemID(newFakeClient(t, api), "S-3", 0, io.Discard)
	if err != nil || got != "UUID-3" {
		t.Fatalf("resolve S-3 = %q, %v; want UUID-3, nil", got, err)
	}
	qs := api.Queries(apiV2 + "/securityProblems")
	if len(qs) != 3 {
		t.Fatalf("requests = %d, want 3", len(qs))
	}
	if qs[0].Get("pageSize") != "500" {
		t.Errorf("first request = %v, want pageSize=500", qs[0])
	}
	for i, q := range qs[1:] {
		if len(q) != 1 || q.Get("nextPageKey") == "" {
			t.Errorf("request %d = %v, want only nextPageKey", i+2, q)
		}
	}
}

// TestResolvePropagatesErrors: a failed listing is returned as the error,
// never papered over with the display ID.
func TestResolvePropagatesErrors(t *testing.T) {
	api := newFakeAPI(t, map[string]route{apiV2 + "/securityProblems": {Status: 403, File: "error_403.json"}})
	got, err := resolveSecurityProblemID(newFakeClient(t, api), "S-1", 0, io.Discard)
	if err == nil {
		t.Fatalf("resolve on 403 = %q, nil; want an error", got)
	}
	if code := exitCode(err); code != client.ExitPermission {
		t.Errorf("exitCode = %d, want %d (%v)", code, client.ExitPermission, err)
	}
}

// TestResolveAnnouncesAtVerbose: -v says why a listing is being fetched;
// without it the lookup is silent.
func TestResolveAnnouncesAtVerbose(t *testing.T) {
	api := newFakeAPI(t, securityProblemPages())
	c := newFakeClient(t, api)

	var quiet bytes.Buffer
	if _, err := resolveSecurityProblemID(c, "S-1", 0, &quiet); err != nil {
		t.Fatal(err)
	}
	if quiet.Len() != 0 {
		t.Errorf("verbose 0 wrote %q, want nothing", quiet.String())
	}

	var loud bytes.Buffer
	if _, err := resolveSecurityProblemID(c, "S-1", 1, &loud); err != nil {
		t.Fatal(err)
	}
	if want := "Resolving S-1 by listing security problems…\n"; loud.String() != want {
		t.Errorf("verbose 1 wrote %q, want %q", loud.String(), want)
	}
}

// TestResolveNotFoundExits4: an unknown display ID exits 4 (not_found for
// agents) after the listing, without asking for /securityProblems/S-999.
func TestResolveNotFoundExits4(t *testing.T) {
	api := newFakeAPI(t, securityProblemPages())
	cfg := withFakeEnvs(t, fakeEnv{"a", api})

	r := runCLI(t, cfg, "describe", "security-problem", "S-999")
	if r.Code != client.ExitNotFound {
		t.Errorf("code = %d, want %d\n%s", r.Code, client.ExitNotFound, r)
	}
	if !strings.Contains(r.Stderr, "security problem S-999 not found") {
		t.Errorf("stderr = %q, want the not-found error", r.Stderr)
	}
	if n := api.Hits(apiV2 + "/securityProblems"); n != 3 {
		t.Errorf("listing requests = %d, want 3 (every page)", n)
	}
	if n := api.Hits(apiV2 + "/securityProblems/S-999"); n != 0 {
		t.Errorf("detail requests for the display ID = %d, want 0", n)
	}

	r = runCLI(t, cfg, "describe", "security-problem", "S-999", "-A")
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("agent: stdout is not one envelope: %v\n%s", err, r.Stdout)
	}
	if resp.OK || resp.Error == nil || resp.Error.Code != "not_found" {
		t.Errorf("agent: envelope = %+v, want ok:false with code not_found", resp)
	}
	if r.Code != client.ExitNotFound {
		t.Errorf("agent: code = %d, want %d", r.Code, client.ExitNotFound)
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

	var buf bytes.Buffer
	printSecurityProblemDetail(&buf, d)
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
