package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

// This file is the command-level test harness: it runs the real CLI through
// Run against an httptest fake of the Dynatrace Managed API and compares what
// comes out with golden files. Regenerate goldens with
//
//	go test ./cmd/ -run <Test> -update
//
// and read the diff: a golden change is a behaviour change.

var update = flag.Bool("update", false, "rewrite golden files")

// route is one canned API response. File is relative to testdata/api/;
// Status 0 means 200.
type route struct {
	Status int
	File   string
	Body   string
}

// fakeAPI is an httptest server answering from a route table.
//
// Routes are keyed "METHOD /path" and match regardless of the query string.
// A follow-up page is keyed "METHOD /path nextPageKey=<key>", so a paged
// fixture can serve page 2 when the client sends that key.
type fakeAPI struct {
	URL string

	mu      sync.Mutex
	hits    map[string]int
	queries map[string][]url.Values
	headers map[string][]http.Header
}

func newFakeAPI(t *testing.T, routes map[string]route) *fakeAPI {
	t.Helper()
	f := &fakeAPI{hits: map[string]int{}, queries: map[string][]url.Values{}, headers: map[string][]http.Header{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		q := r.URL.Query()
		f.mu.Lock()
		f.hits[key]++
		f.queries[key] = append(f.queries[key], q)
		f.headers[key] = append(f.headers[key], r.Header.Clone())
		f.mu.Unlock()

		rt, ok := routes[key]
		if npk := q.Get("nextPageKey"); npk != "" {
			rt, ok = routes[key+" nextPageKey="+npk]
		}
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":404,"message":"not found"}}`)
			return
		}
		body := rt.Body
		if rt.File != "" {
			b, err := os.ReadFile(filepath.Join("testdata", "api", rt.File))
			if err != nil {
				t.Errorf("fixture %s: %v", rt.File, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			body = string(b)
		}
		if rt.Status != 0 {
			w.WriteHeader(rt.Status)
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

// Hits reports how many requests reached "METHOD /path".
func (f *fakeAPI) Hits(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[key]
}

// Queries returns the query strings sent to "METHOD /path", in order.
func (f *fakeAPI) Queries(key string) []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.queries[key]...)
}

// Headers returns the request headers sent to "METHOD /path", in order.
func (f *fakeAPI) Headers(key string) []http.Header {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]http.Header(nil), f.headers[key]...)
}

// fakeEnv names one context and the fake API it points at.
type fakeEnv struct {
	Name string
	API  *fakeAPI
}

// withFakeEnvs writes a temp config with one context per env (env-id env1,
// a plaintext token) and returns its path. The first env is current.
func withFakeEnvs(t *testing.T, envs ...fakeEnv) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "apiVersion: dtmgd.io/v1\nkind: Config\ncurrent-context: %s\ncontexts:\n", envs[0].Name)
	for _, e := range envs {
		fmt.Fprintf(&b, "  - name: %s\n    context:\n      host: %s\n      env-id: env1\n      token-ref: tok-%s\n", e.Name, e.API.URL, e.Name)
	}
	b.WriteString("tokens:\n")
	for _, e := range envs {
		fmt.Fprintf(&b, "  - name: tok-%s\n    token: dt0c01.TEST\n", e.Name)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// cliResult is what one CLI run produced.
type cliResult struct {
	Stdout, Stderr string
	Code           int
}

// String renders the result as a golden file body.
func (r cliResult) String() string {
	return fmt.Sprintf("== exit code: %d\n== stdout\n%s== stderr\n%s", r.Code, r.Stdout, r.Stderr)
}

var fakeURL = regexp.MustCompile(`http://127\.0\.0\.1:\d+`)

// runCLI runs dtmgd with args against the config at cfgPath and captures
// stdout, stderr and the exit code. Agent detection is cleared so output is
// human unless a test opts in, and fake server URLs are normalised to
// http://FAKE so goldens are stable.
func runCLI(t *testing.T, cfgPath string, args ...string) cliResult {
	t.Helper()
	return runCLIWithEnv(t, nil, cfgPath, args...)
}

// runCLIWithEnv is runCLI with env set after agent detection is cleared, so a
// test can opt into an agent environment (for example CLAUDECODE=1).
func runCLIWithEnv(t *testing.T, env map[string]string, cfgPath string, args ...string) cliResult {
	t.Helper()
	for _, v := range agentmode.EnvVars() {
		t.Setenv(v, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(&stdout, outR) }()
	go func() { defer wg.Done(); _, _ = io.Copy(&stderr, errR) }()

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	code := Run(append(append([]string(nil), args...), "--config", cfgPath), outW, errW)
	os.Stdout, os.Stderr = origOut, origErr

	_ = outW.Close()
	_ = errW.Close()
	wg.Wait()
	_ = outR.Close()
	_ = errR.Close()

	return cliResult{
		Stdout: fakeURL.ReplaceAllString(stdout.String(), "http://FAKE"),
		Stderr: fakeURL.ReplaceAllString(stderr.String(), "http://FAKE"),
		Code:   code,
	}
}

// newTestApp returns an App for tests that call command helpers directly.
// Building the command tree over it fills Opts with the flag defaults, as a
// run without flags would, and cfgPath stands in for --config. Output is
// discarded unless the test points Out at a buffer.
func newTestApp(cfgPath string) *App {
	a := &App{Out: io.Discard, Err: io.Discard}
	newRootCmd(a)
	a.Opts.ConfigPath = cfgPath
	return a
}

// assertGolden compares got with testdata/golden/<name>.golden, or rewrites
// the file under -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %v", path, err)
	}
	if string(want) != got {
		t.Errorf("%s does not match its golden\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}
