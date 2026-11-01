package client

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// writeServer answers every request with handler and counts the requests.
func writeServer(t *testing.T, handler http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "env1", "tok")
	if err != nil {
		t.Fatal(err)
	}
	return c, &hits
}

func TestWriteMethodsSendBodyAndQuery(t *testing.T) {
	var method, query string
	var body map[string]interface{}
	c, _ := writeServer(t, func(w http.ResponseWriter, r *http.Request) {
		method, query = r.Method, r.URL.RawQuery
		body = nil
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200}`)
	})
	var out map[string]interface{}
	if err := c.PostV2("/settings/objects", map[string]string{"validateOnly": "true"}, map[string]string{"a": "b"}, &out); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || query != "validateOnly=true" || body["a"] != "b" || out["code"] != float64(200) {
		t.Errorf("POST: %s %q %v %v", method, query, body, out)
	}
	if err := c.PutV2("/settings/objects/x", nil, map[string]string{"c": "d"}, nil); err != nil || method != http.MethodPut || body["c"] != "d" {
		t.Errorf("PUT: %v %s %v", err, method, body)
	}
	if err := c.DeleteV2("/settings/objects/x", map[string]string{"updateToken": "t1"}); err != nil || method != http.MethodDelete || query != "updateToken=t1" {
		t.Errorf("DELETE: %v %s %q", err, method, query)
	}
}

// TestWritesAreNotRetried pins that a write the server may have received is
// sent once: a 5xx or a connection dropped after sending is not retried.
func TestWritesAreNotRetried(t *testing.T) {
	c, hits := writeServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	calls := []func() error{
		func() error { return c.PostV2("/settings/objects", nil, []int{1}, nil) },
		func() error { return c.PutV2("/settings/objects/x", nil, map[string]int{}, nil) },
		func() error { return c.DeleteV2("/settings/objects/x", nil) },
	}
	for i, call := range calls {
		hits.Store(0)
		var apiErr *ErrAPI
		if err := call(); !errors.As(err, &apiErr) || hits.Load() != 1 {
			t.Errorf("call %d: err %v, %d requests", i, err, hits.Load())
		}
	}

	dropped, dropHits := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
	if err := dropped.PostV2("/settings/objects", nil, []int{1}, nil); err == nil || dropHits.Load() != 1 {
		t.Errorf("dropped connection: err %v, %d requests", err, dropHits.Load())
	}
}

func TestGetStillRetries(t *testing.T) {
	var seen atomic.Int32
	c, hits := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if seen.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	})
	var out map[string]interface{}
	if err := c.GetV2("/x", nil, &out); err != nil || hits.Load() != 2 {
		t.Errorf("err %v, %d requests", err, hits.Load())
	}
}

func TestWriteOnClusterScopedClient(t *testing.T) {
	c, hits := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	cluster, err := New(c.ClusterURL(), "", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if err := cluster.PostV2("/settings/objects", nil, []int{}, nil); !errors.Is(err, ErrClusterScoped) || hits.Load() != 0 {
		t.Errorf("err %v, %d requests", err, hits.Load())
	}
}

func TestVersionAtLeast(t *testing.T) {
	floor := [3]int{1, 288, 0}
	for v, want := range map[string][2]bool{
		"1.288.0.20240101-000000": {true, true},
		"1.348.5":                 {true, true},
		"1.287.99":                {false, true},
		"garbage":                 {false, false},
	} {
		if ok, known := VersionAtLeast(v, floor); ok != want[0] || known != want[1] {
			t.Errorf("VersionAtLeast(%q) = %v, %v", v, ok, known)
		}
	}
}

func TestMultiRequestNamedPassesNames(t *testing.T) {
	cfg := &config.Config{
		CurrentContext: "a",
		Contexts: []config.NamedContext{
			{Name: "a", Context: config.Context{Host: "https://a.example.com", EnvID: "e1", TokenRef: "t"}},
			{Name: "b", Context: config.Context{Host: "https://b.example.com", EnvID: "e2", TokenRef: "t"}},
		},
		Tokens: []config.NamedToken{{Name: "t", Token: "x"}},
	}
	results, err := MultiRequestNamed(cfg, "a;b", 0, Options{}, func(name string, _ *Client) (interface{}, error) { return name, nil })
	if err != nil || len(results) != 2 || results[0].Data != "a" || results[1].Data != "b" {
		t.Errorf("err %v, results %+v", err, results)
	}
	if got, err := ResolveContexts(cfg, ""); err != nil || len(got) != 1 || got[0].Name != "a" {
		t.Errorf("ResolveContexts(\"\") = %v, %v", got, err)
	}
}
