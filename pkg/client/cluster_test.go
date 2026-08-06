package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetClusterUsesSharedClientSettings is the observable half of the
// GetCluster finding. The cluster request used to come from a bare
// resty.New(), so it carried none of the configuration every other request
// had: no User-Agent (cluster access logs could not attribute the traffic to
// dtmgd), no retry, no debug hooks, and — the reason this matters — no proxy.
func TestGetClusterUsesSharedClientSettings(t *testing.T) {
	var gotUA, gotAuth, gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"version":"1.300.0"}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "env1", "mytoken")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var result struct {
		Version string `json:"version"`
	}
	if err := c.GetCluster("/v1.0/onpremise/cluster", nil, &result); err != nil {
		t.Fatalf("GetCluster() error = %v", err)
	}

	if result.Version != "1.300.0" {
		t.Errorf("Version = %q, want the decoded body", result.Version)
	}
	if !strings.HasPrefix(gotUA, "dtmgd/") {
		t.Errorf("User-Agent = %q, want the dtmgd agent — cluster logs cannot attribute the request otherwise", gotUA)
	}
	if gotAuth != "Api-Token mytoken" {
		t.Errorf("Authorization = %q, want the API token header", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
}

// TestSetProxyReachesClusterClient pins the proxy fix directly. Setting a
// proxy that cannot be resolved must break the cluster request too — if it
// still succeeds, GetCluster is bypassing the proxy exactly as the finding
// described, and in a corporate deployment it would be bypassing egress
// controls with it.
//
// This test takes several seconds: the cluster client now shares the retry
// policy, so the unreachable proxy is retried three times with backoff. That
// delay is the fix working, not a hang.
func TestSetProxyReachesClusterClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`{"version":"1.300.0"}`)); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "env1", "mytoken")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	c.SetProxy("", "http://proxy.invalid.dtmgd-test:9999")

	var result struct {
		Version string `json:"version"`
	}
	err = c.GetCluster("/v1.0/onpremise/cluster", nil, &result)

	if err == nil {
		t.Fatal("GetCluster() succeeded through an unreachable proxy — the proxy is being bypassed")
	}
	// The error must name the proxy: a bare "no such host" for a hostname the
	// user never typed into their context is what made this hard to diagnose.
	if !strings.Contains(err.Error(), "proxy.invalid.dtmgd-test:9999") {
		t.Errorf("error does not name the proxy that was attempted: %v", err)
	}
}

// TestRequestErrorWithoutProxyIsUnchanged keeps the annotation from becoming
// noise when no proxy is configured.
func TestRequestErrorWithoutProxyIsUnchanged(t *testing.T) {
	c, err := New("https://managed.example.com", "env1", "tok")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got := c.requestError("request", errNotReachable{}).Error()

	if strings.Contains(got, "proxy") {
		t.Errorf("error mentions a proxy when none is set: %q", got)
	}
	if !strings.Contains(got, "request failed") {
		t.Errorf("error lost its context: %q", got)
	}
}

type errNotReachable struct{}

func (errNotReachable) Error() string { return "dial tcp: connection refused" }
