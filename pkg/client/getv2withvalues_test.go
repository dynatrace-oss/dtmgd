package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestGetV2WithValuesQueryParams verifies that multi-value query params are sent correctly.
func TestGetV2WithValuesQueryParams(t *testing.T) {
	var receivedQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
	}))
	defer srv.Close()

	c, err := New(srv.URL, "env1", "tok")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	params := url.Values{
		"groupBy":        {"dt.entity.process_group"},
		"from":           {"now-1h"},
		"to":             {"now"},
		"timeBuckets":    {"1"},
		"maxGroupValues": {"100"},
	}

	var result map[string]string
	if err := c.GetV2WithValues("/logs/aggregate", params, &result); err != nil {
		t.Fatalf("GetV2WithValues() failed: %v", err)
	}

	// Verify all params were sent.
	checks := map[string]string{
		"groupBy":        "dt.entity.process_group",
		"from":           "now-1h",
		"to":             "now",
		"timeBuckets":    "1",
		"maxGroupValues": "100",
	}
	for k, want := range checks {
		if got := receivedQuery.Get(k); got != want {
			t.Errorf("param %q = %q, want %q", k, got, want)
		}
	}

	if result["result"] != "ok" {
		t.Errorf("response not decoded correctly: %v", result)
	}
}

// TestGetV2WithValuesMultiValue verifies that repeated param keys (e.g. groupBy=a&groupBy=b) work.
func TestGetV2WithValuesMultiValue(t *testing.T) {
	var receivedRaw string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRaw = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer srv.Close()

	c, err := New(srv.URL, "env1", "tok")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Multiple values for the same key.
	params := url.Values{
		"groupBy": {"k8s.container.name", "k8s.namespace.name"},
		"from":    {"now-1h"},
	}

	var result map[string]string
	if err := c.GetV2WithValues("/test/multi", params, &result); err != nil {
		t.Fatalf("GetV2WithValues() failed: %v", err)
	}

	// Both values for groupBy must appear in the raw query string.
	if !strings.Contains(receivedRaw, "groupBy=k8s.container.name") {
		t.Errorf("expected groupBy=k8s.container.name in query %q", receivedRaw)
	}
	if !strings.Contains(receivedRaw, "groupBy=k8s.namespace.name") {
		t.Errorf("expected groupBy=k8s.namespace.name in query %q", receivedRaw)
	}
}

// TestGetV2WithValuesAPIError verifies that non-2xx responses return an ErrAPI.
func TestGetV2WithValuesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":400,"message":"Bad Request"}}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	c, err := New(srv.URL, "env1", "tok")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	var result map[string]interface{}
	err = c.GetV2WithValues("/logs/aggregate", url.Values{"from": {"now-1h"}}, &result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*ErrAPI)
	if !ok {
		t.Fatalf("expected *ErrAPI, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}

// TestGetV2WithValuesURLPath verifies the /v2 prefix is applied to the path.
func TestGetV2WithValuesURLPath(t *testing.T) {
	var receivedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer srv.Close()

	c, err := New(srv.URL, "testenv", "tok")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	var result map[string]string
	if err := c.GetV2WithValues("/logs/aggregate", url.Values{}, &result); err != nil {
		t.Fatalf("GetV2WithValues() failed: %v", err)
	}

	// The path should contain /v2/logs/aggregate (prepended with env prefix by resty base URL).
	if !strings.Contains(receivedPath, "/v2/logs/aggregate") {
		t.Errorf("expected path to contain /v2/logs/aggregate, got %q", receivedPath)
	}
}
