//go:build integration

// Package integration runs dtmgd against a real Dynatrace Managed cluster:
// the checks that fake APIs cannot make, such as whether the cluster honours
// a filter parameter at all. It never runs in CI. See CONTRIBUTING.md
// ("Live integration tests") for how to configure and run it.
//
// The suite is read-only by construction: run refuses any verb other than
// get, describe, query, doctor and commands. On a failure it reports the exit
// code and the first line of stderr with URLs removed, never stdout, so no
// response data reaches a log.
package integration

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dynatrace-oss/dtmgd/cmd"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// result is what one dtmgd run produced.
type result struct {
	Out  []byte
	Err  string
	Code int
}

// readVerbs are the only commands the suite may run.
var readVerbs = map[string]bool{"get": true, "describe": true, "query": true, "doctor": true, "commands": true}

// run runs dtmgd with args against the config named by DTMGD_IT_CONFIG,
// with agent detection off and plain output. It fails the test, and runs
// nothing, unless args start with a read verb.
func run(t testing.TB, args ...string) result {
	t.Helper()
	if len(args) == 0 || !readVerbs[args[0]] {
		t.Fatalf("the integration suite runs read commands only, not %q", args)
		return result{}
	}
	full := append([]string{"--config", requireEnv(t, "DTMGD_IT_CONFIG"), "--no-agent", "--plain"}, args...)
	var out, errb bytes.Buffer
	code := cmd.Run(full, &out, &errb)
	return result{Out: out.Bytes(), Err: errb.String(), Code: code}
}

var (
	urlPattern  = regexp.MustCompile(`https?://\S+`)
	ipv4Pattern = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}(:\d+)?\b`)
	ipv6Pattern = regexp.MustCompile(`\[[0-9a-fA-F:.]+\](:\d+)?`)

	hostsOnce sync.Once
	hosts     []string
)

// configHosts returns the host names of every context in DTMGD_IT_CONFIG,
// read once, so brief can remove them from error text.
func configHosts() []string {
	hostsOnce.Do(func() {
		cfg, err := config.LoadFrom(os.Getenv("DTMGD_IT_CONFIG"))
		if err != nil {
			return
		}
		for _, nc := range cfg.Contexts {
			if u, err := url.Parse(os.ExpandEnv(nc.Context.Host)); err == nil && u.Hostname() != "" {
				hosts = append(hosts, u.Hostname())
			}
		}
	})
	return hosts
}

// brief describes a result for a failure message: the exit code and the
// first line of stderr, with URLs, the config's host names and IPv4 and
// bracketed IPv6 addresses removed, so none of them reaches a log. A DNS error, for one, repeats the
// host outside the URL.
func brief(r result) string {
	line, _, _ := strings.Cut(strings.TrimSpace(r.Err), "\n")
	line = urlPattern.ReplaceAllString(line, "<url>")
	for _, h := range configHosts() {
		line = strings.ReplaceAll(line, h, "<host>")
	}
	line = ipv6Pattern.ReplaceAllString(line, "<ip>")
	return "exit " + strconv.Itoa(r.Code) + ": " + ipv4Pattern.ReplaceAllString(line, "<ip>")
}

// requireEnv returns the variable's value, or skips the test when it is
// unset. An empty DTMGD_IT_CONFIG must never reach dtmgd: it would fall back
// to the default config path.
func requireEnv(t testing.TB, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("%s is not set", name)
	}
	return v
}

// envOr returns the variable's value, or def when it is unset.
func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// skipWithoutScope skips the test when the token lacks the scope a command
// needs (exit 5): that is the token's setup, not a dtmgd failure.
func skipWithoutScope(t testing.TB, r result, args []string) {
	t.Helper()
	if r.Code == 5 {
		t.Skipf("the token lacks a scope for %v (%s)", args[:2], brief(r))
	}
}

// decode runs dtmgd and decodes its JSON stdout. A token without the scope
// the command needs (exit 5) skips the test rather than failing it.
func decode[T any](t *testing.T, args ...string) T {
	t.Helper()
	r := run(t, args...)
	skipWithoutScope(t, r, args)
	var v T
	if r.Code != 0 {
		t.Fatalf("%v: %s", args[:2], brief(r))
	}
	if err := json.Unmarshal(r.Out, &v); err != nil {
		t.Fatalf("%v: stdout is not the expected JSON: %v", args[:2], err)
	}
	return v
}

// items returns body[key] as a list, failing when it is not one.
func items(t *testing.T, body map[string]any, key string) []any {
	t.Helper()
	list, ok := body[key].([]any)
	if !ok {
		t.Fatalf("response has no %q list", key)
	}
	return list
}

// total returns body["totalCount"], failing when it is missing.
func total(t *testing.T, body map[string]any) int {
	t.Helper()
	n, ok := body["totalCount"].(float64)
	if !ok {
		t.Fatal("response has no totalCount")
	}
	return int(n)
}

// fatalRecorder records Fatalf instead of stopping the test, so the harness
// self-test can check that run refuses a write verb.
type fatalRecorder struct {
	testing.TB
	msg string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.msg = format
}

// TestHarnessRefusesWriteVerbs needs no cluster: it pins that run never
// executes a command that could change the config, the keyring or skills.
func TestHarnessRefusesWriteVerbs(t *testing.T) {
	// A config of its own, so a broken refusal runs (and fails) instead of
	// skipping for want of DTMGD_IT_CONFIG.
	cfg := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(cfg, []byte("contexts: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DTMGD_IT_CONFIG", cfg)
	for _, verb := range []string{"config", "ctx", "skills", ""} {
		rec := &fatalRecorder{TB: t}
		r := run(rec, verb, "view")
		if rec.msg == "" || r.Code != 0 || len(r.Out) != 0 {
			t.Errorf("run(%q) was not refused", verb)
		}
	}
}

func TestBriefHidesHostsAndAddresses(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config")
	body := "contexts:\n- name: a\n  context:\n    host: https://cluster.example.com\n    env-id: e\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DTMGD_IT_CONFIG", cfg)
	hostsOnce, hosts = sync.Once{}, nil
	t.Cleanup(func() { hostsOnce, hosts = sync.Once{}, nil })

	got := brief(result{Code: 1, Err: "Error: Get \"https://cluster.example.com/e/x/api\": dial tcp: lookup cluster.example.com on 192.0.2.1:53: no such host\nHint: more"})
	if want := `exit 1: Error: Get "<url> dial tcp: lookup <host> on <ip>: no such host`; got != want {
		t.Errorf("brief = %q\nwant    %q", got, want)
	}
}

// narrowed checks that a filter narrowed a result of all items to filtered.
// An empty result, or a filter that kept everything, proves nothing either
// way (the cluster may have ignored it), so the test skips; more items than
// without the filter fails it.
func narrowed(t testing.TB, what string, filtered, all int) {
	t.Helper()
	switch {
	case all == 0:
		t.Skipf("%s: nothing to filter, so the filter cannot be checked", what)
	case filtered > all:
		t.Errorf("%s: the filter matched %d of %d", what, filtered, all)
	case filtered == all:
		t.Skipf("%s: the filter kept all %d, which an ignored filter would too", what, all)
	}
}

// outcome records how a check ended, for the harness self-tests.
type outcome struct {
	testing.TB
	skipped, failed bool
}

func (o *outcome) Helper()                        {}
func (o *outcome) Skipf(string, ...any)           { o.skipped = true }
func (o *outcome) Errorf(string, ...any)          { o.failed = true }
func (o *outcome) Fatalf(string, ...any)          { o.failed = true }
func (o *outcome) result() (skipped, failed bool) { return o.skipped, o.failed }

// TestNarrowedPassesOnlyWhenTheFilterNarrowed pins Review Focus 5: a check
// that cannot tell a working filter from an ignored one skips, never passes.
func TestNarrowedPassesOnlyWhenTheFilterNarrowed(t *testing.T) {
	for _, c := range []struct {
		filtered, all   int
		skipped, failed bool
	}{
		{0, 0, true, false},
		{5, 5, true, false},
		{6, 5, false, true},
		{2, 5, false, false},
		{0, 5, false, false},
	} {
		o := &outcome{TB: t}
		narrowed(o, "x", c.filtered, c.all)
		if s, f := o.result(); s != c.skipped || f != c.failed {
			t.Errorf("narrowed(%d of %d): skipped=%v failed=%v, want %v %v", c.filtered, c.all, s, f, c.skipped, c.failed)
		}
	}
}

// TestBriefHidesIPv6Addresses: a dial error names an IPv6 literal in brackets.
func TestBriefHidesIPv6Addresses(t *testing.T) {
	got := brief(result{Code: 1, Err: "Error: dial tcp [2001:db8::1]:443: connect: refused"})
	if want := "exit 1: Error: dial tcp <ip>: connect: refused"; got != want {
		t.Errorf("brief = %q, want %q", got, want)
	}
}
