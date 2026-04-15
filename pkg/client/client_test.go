package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- New constructor ---

func TestNewErrors(t *testing.T) {
	_, err := New("", "env1", "tok")
	if err == nil {
		t.Error("empty host should error")
	}
	_, err = New("http://host", "", "tok")
	if err == nil {
		t.Error("empty envID should error")
	}
	_, err = New("http://host", "env1", "")
	if err == nil {
		t.Error("empty token should error")
	}
}

func TestNewURLFields(t *testing.T) {
	c, err := New("https://host.example.com", "env123", "mytoken")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if !strings.Contains(c.APIBaseURL(), "/e/env123/api") {
		t.Errorf("unexpected APIBaseURL: %s", c.APIBaseURL())
	}
	if !strings.HasSuffix(c.ClusterURL(), "/api") {
		t.Errorf("unexpected ClusterURL: %s", c.ClusterURL())
	}
	if !strings.Contains(c.DashboardURL(), "/e/env123") {
		t.Errorf("unexpected DashboardURL: %s", c.DashboardURL())
	}
}

func TestNewTrailingSlash(t *testing.T) {
	c, err := New("https://host.example.com/", "env1", "tok")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	// After stripping trailing slash, path segments must not have double-slash (beyond the protocol "://").
	url := c.APIBaseURL()
	withoutProto := url[len("https://"):]
	if strings.Contains(withoutProto, "//") {
		t.Errorf("URL path contains double slash: %s", url)
	}
}

// --- SetProxy ---

func TestSetProxy(t *testing.T) {
	c, _ := New("https://host.example.com", "env1", "tok")
	c.SetProxy("", "")                                      // no-op: proxy == ""
	c.SetProxy("http://proxy.example.com:8080", "")         // httpProxy only
	c.SetProxy("", "https://proxy.example.com:8080")        // httpsProxy takes priority
	c.SetProxy("http://a.example.com:80", "https://b:443") // both: https wins
}

// --- SetVerbosity ---

func TestSetVerbosityNoop(t *testing.T) {
	c, _ := New("https://host.example.com", "env1", "tok")
	c.SetVerbosity(0) // should be a no-op; no hooks added
}

func TestSetVerbosityLevel1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	c.SetVerbosity(1)
	var result map[string]string
	_ = c.GetV2("/test", nil, &result) // trigger the pre-request + after-response hooks
}

func TestSetVerbosityLevel2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": "val"})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	c.SetVerbosity(2) // covers header iteration + response body print (level >= 2)
	var result map[string]string
	_ = c.GetV2("/test", nil, &result)
}

// --- GetV2 ---

func TestGetV2Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v2/") {
			t.Errorf("expected /v2/ in path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	var result map[string]string
	if err := c.GetV2("/test", nil, &result); err != nil {
		t.Fatalf("GetV2 failed: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestGetV2WithParams(t *testing.T) {
	var gotFrom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFrom = r.URL.Query().Get("from")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	var result map[string]interface{}
	_ = c.GetV2("/test", map[string]string{"from": "now-1h"}, &result)
	if gotFrom != "now-1h" {
		t.Errorf("expected from=now-1h, got %q", gotFrom)
	}
}

func TestGetV2Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":403}}`, http.StatusForbidden) // 403 is not retried
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	var result map[string]interface{}
	err := c.GetV2("/test", nil, &result)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*ErrAPI)
	if !ok {
		t.Fatalf("expected *ErrAPI, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 403 {
		t.Errorf("expected 403, got %d", apiErr.StatusCode)
	}
}

// --- GetV1 ---

func TestGetV1Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/") {
			t.Errorf("expected /v1/ in path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	var result map[string]string
	if err := c.GetV1("/config/clusterversion", nil, &result); err != nil {
		t.Fatalf("GetV1 failed: %v", err)
	}
	if result["version"] != "1.2.3" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestGetV1Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	var result map[string]string
	if err := c.GetV1("/test", nil, &result); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetV1WithParams(t *testing.T) {
	var gotFilter string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFilter = r.URL.Query().Get("filter")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	var result map[string]interface{}
	_ = c.GetV1("/test", map[string]string{"filter": "active"}, &result)
	if gotFilter != "active" {
		t.Errorf("expected filter=active, got %q", gotFilter)
	}
}

// --- GetCluster ---

func TestGetClusterSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"version": "1.300"})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	var result map[string]string
	if err := c.GetCluster("/v1.0/onpremise/cluster", nil, &result); err != nil {
		t.Fatalf("GetCluster failed: %v", err)
	}
	if result["version"] != "1.300" {
		t.Errorf("expected 1.300, got %v", result)
	}
}

func TestGetClusterWithParams(t *testing.T) {
	var gotParam string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotParam = r.URL.Query().Get("filter")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	var result map[string]interface{}
	_ = c.GetCluster("/v1.0/test", map[string]string{"filter": "active"}, &result)
	if gotParam != "active" {
		t.Errorf("expected filter=active, got %q", gotParam)
	}
}

func TestGetClusterError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	var result map[string]string
	if err := c.GetCluster("/v1.0/test", nil, &result); err == nil {
		t.Fatal("expected error")
	}
}

// --- ClusterVersion ---

func TestClusterVersionFromClusterAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"version": "1.300.0"})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	v, err := c.ClusterVersion()
	if err != nil {
		t.Fatalf("ClusterVersion failed: %v", err)
	}
	if v != "1.300.0" {
		t.Errorf("expected 1.300.0, got %s", v)
	}
}

func TestClusterVersionFallback(t *testing.T) {
	// Cluster API returns 403; v1 fallback returns the version.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "onpremise") {
			http.Error(w, "forbidden", http.StatusForbidden)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"version": "1.280.0"})
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")
	v, err := c.ClusterVersion()
	if err != nil {
		t.Fatalf("ClusterVersion fallback failed: %v", err)
	}
	if v != "1.280.0" {
		t.Errorf("expected 1.280.0, got %s", v)
	}
}

// --- GetV2Paged (via c.GetV2Paged + DecodePaged) ---

func TestGetV2Paged(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")
		var resp map[string]interface{}
		if page == 1 {
			resp = map[string]interface{}{
				"items":       []interface{}{"a", "b"},
				"nextPageKey": "page2",
				"totalCount":  float64(4),
			}
		} else {
			resp = map[string]interface{}{
				"items":      []interface{}{"c", "d"},
				"totalCount": float64(4),
			}
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")

	type Response struct {
		Items      []string `json:"items"`
		TotalCount int      `json:"totalCount"`
	}
	raw, err := c.GetV2Paged("/items", nil, 0)
	if err != nil {
		t.Fatalf("GetV2Paged failed: %v", err)
	}
	var result Response
	if err := DecodePaged(raw, &result); err != nil {
		t.Fatalf("DecodePaged failed: %v", err)
	}
	if len(result.Items) != 4 {
		t.Errorf("expected 4 items, got %d: %v", len(result.Items), result.Items)
	}
}

func TestGetV2PagedMaxPages(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       []interface{}{"x"},
			"nextPageKey": "more",
		})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")

	_, err := c.GetV2Paged("/items", nil, 2)
	if err != nil {
		t.Fatalf("GetV2Paged failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 page requests, got %d", callCount)
	}
}

func TestGetV2PagedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest) // 400 is not retried
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "env1", "tok")

	_, err := c.GetV2Paged("/items", nil, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}
