package client

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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

// TestClusterVersionNoFallbackAfterTransportError pins that ClusterVersion
// tries the environment endpoint only after the cluster endpoint answered
// with an HTTP error. After a transport failure the host is the same, so the
// fallback could only fail the same way, after another full timeout: doctor
// took minutes on a hung host.
func TestClusterVersionNoFallbackAfterTransportError(t *testing.T) {
	var fallback atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1.0/onpremise/cluster":
			// Drop the connection without answering.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
		case "/e/env1/api/v1/config/clusterversion":
			fallback.Add(1)
			_, _ = w.Write([]byte(`{"version":"1.330.0.20251017-120000"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, "env1", "dt0c01.TEST")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ClusterVersion(); err == nil {
		t.Error("want the cluster endpoint's transport error")
	}
	if n := fallback.Load(); n != 0 {
		t.Errorf("environment version endpoint requested %d times after a transport error, want 0", n)
	}
}

// hitCounter is an httptest server answering 200 {} to everything, counting
// the requests it gets by path.
func hitCounter(t *testing.T, routes map[string]string) (*httptest.Server, *sync.Map) {
	t.Helper()
	var hits sync.Map
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := hits.LoadOrStore(r.URL.Path, new(atomic.Int32))
		n.(*atomic.Int32).Add(1)
		w.Header().Set("Content-Type", "application/json")
		body, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":404,"message":"not found"}}`)
			return
		}
		if strings.HasPrefix(body, "401 ") {
			w.WriteHeader(http.StatusUnauthorized)
			body = strings.TrimPrefix(body, "401 ")
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func hitCount(hits *sync.Map, path string) int32 {
	n, ok := hits.Load(path)
	if !ok {
		return 0
	}
	return n.(*atomic.Int32).Load()
}

func totalHits(hits *sync.Map) int32 {
	var total int32
	hits.Range(func(_, n any) bool { total += n.(*atomic.Int32).Load(); return true })
	return total
}

// TestNewClusterScoped pins that a context without env-id builds a client for
// the Cluster API alone: no environment API base and no UI base URL, so no UI
// link can come out as {host}/e/.
func TestNewClusterScoped(t *testing.T) {
	c, err := New("https://cluster.example.com/", "", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !c.ClusterScoped() || c.DashboardURL() != "" || c.APIBaseURL() != "" {
		t.Errorf("ClusterScoped %v, DashboardURL %q, APIBaseURL %q", c.ClusterScoped(), c.DashboardURL(), c.APIBaseURL())
	}
	if c.ClusterURL() != "https://cluster.example.com/api" {
		t.Errorf("ClusterURL = %q", c.ClusterURL())
	}
	if env, _ := New("https://h", "env1", "tok"); env.ClusterScoped() {
		t.Error("a client with an env-id is not cluster-scoped")
	}
}

// TestEnvironmentCallsRefusedOnClusterClient pins the backstop: an
// environment API call on a cluster-scoped client fails without a request,
// rather than going to {host}/e//api.
func TestEnvironmentCallsRefusedOnClusterClient(t *testing.T) {
	srv, hits := hitCounter(t, map[string]string{})
	c, err := New(srv.URL, "", "tok")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	calls := map[string]error{
		"GetV2":           c.GetV2("/problems", nil, &m),
		"GetV2WithValues": c.GetV2WithValues("/problems", url.Values{}, &m),
		"GetV1":           c.GetV1("/config/clusterversion", nil, &m),
	}
	_, calls["GetV2Paged"] = c.GetV2Paged("/problems", nil, Paging{ListKey: "problems"})
	for name, err := range calls {
		if !errors.Is(err, ErrClusterScoped) {
			t.Errorf("%s: err = %v, want ErrClusterScoped", name, err)
		}
	}
	if n := totalHits(hits); n != 0 {
		t.Errorf("%d requests sent, want 0", n)
	}
}

// TestGetClusterPagedFollowsNextPageKey pins Cluster API paging, which
// follows the environment API's protocol (checked live on 2026-10-29): the
// next page is asked for with nextPageKey alone.
func TestGetClusterPagedFollowsNextPageKey(t *testing.T) {
	var queries []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cluster/v2/environments" {
			http.NotFound(w, r)
			return
		}
		queries = append(queries, r.URL.Query())
		if r.URL.Query().Get("nextPageKey") == "k" {
			_, _ = io.WriteString(w, `{"environments":[{"id":"b"}],"totalCount":2}`)
			return
		}
		_, _ = io.WriteString(w, `{"environments":[{"id":"a"}],"totalCount":2,"nextPageKey":"k"}`)
	}))
	t.Cleanup(srv.Close)
	c, _ := New(srv.URL, "", "tok")
	got, err := c.GetClusterPaged("/cluster/v2/environments", map[string]string{"pageSize": "1"}, Paging{ListKey: "environments"})
	if err != nil {
		t.Fatal(err)
	}
	if envs := got["environments"].([]interface{}); len(envs) != 2 {
		t.Errorf("environments = %v", envs)
	}
	if _, ok := got["nextPageKey"]; ok {
		t.Error("an exhausted result keeps no nextPageKey")
	}
	want := []url.Values{{"pageSize": {"1"}}, {"nextPageKey": {"k"}}}
	if !reflect.DeepEqual(queries, want) {
		t.Errorf("queries = %v, want %v", queries, want)
	}
}

const (
	nodesPath     = "/api/v1.0/onpremise/cluster"
	envVersionURL = "/e/env1/api/v1/config/clusterversion"
)

// TestClusterVersionIsLowestNode pins that the version comes from the node
// list /cluster actually returns (an array; dtmgd used to decode it as an
// object), and that a cluster mid-upgrade reports its lowest node.
func TestClusterVersionIsLowestNode(t *testing.T) {
	srv, _ := hitCounter(t, map[string]string{
		nodesPath: `[{"buildVersion":"1.330.18.20251017-120000"},{"buildVersion":"1.328.5.20250901-090000"}]`,
	})
	for _, envID := range []string{"env1", ""} {
		c, _ := New(srv.URL, envID, "tok")
		v, err := c.ClusterVersion()
		if err != nil || v != "1.328.5.20250901-090000" {
			t.Errorf("env-id %q: ClusterVersion = %q, %v", envID, v, err)
		}
	}
}

// TestClusterVersionClusterScopedNoFallback pins that a cluster context has
// nothing to fall back to: the environment endpoint needs an env-id.
func TestClusterVersionClusterScopedNoFallback(t *testing.T) {
	srv, hits := hitCounter(t, map[string]string{nodesPath: `401 {"error":{"code":401,"message":"Token Authentication failed"}}`})
	c, _ := New(srv.URL, "", "tok")
	_, err := c.ClusterVersion()
	var apiErr *ErrAPI
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		t.Errorf("err = %v, want API error 401", err)
	}
	if n := totalHits(hits); n != 1 {
		t.Errorf("%d requests, want only the node list", n)
	}
}

// TestClusterVersionNoNodes pins an empty node list: an environment context
// still asks its environment endpoint; a cluster context reports no version
// rather than an error.
func TestClusterVersionNoNodes(t *testing.T) {
	srv, hits := hitCounter(t, map[string]string{nodesPath: `[]`, envVersionURL: `{"version":"1.330.0"}`})
	env, _ := New(srv.URL, "env1", "tok")
	if v, err := env.ClusterVersion(); err != nil || v != "1.330.0" {
		t.Errorf("environment context: %q, %v", v, err)
	}
	cluster, _ := New(srv.URL, "", "tok")
	if v, err := cluster.ClusterVersion(); err != nil || v != "" {
		t.Errorf("cluster context: %q, %v", v, err)
	}
	if n := hitCount(hits, envVersionURL); n != 1 {
		t.Errorf("environment endpoint asked %d times, want 1 (by the environment context only)", n)
	}
}
