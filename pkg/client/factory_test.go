package client

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// proxyConfig returns a config with one context per host, each with a
// plaintext token. proxy, when set, is applied to every context.
func proxyConfig(proxy string, hosts ...string) *config.Config {
	cfg := config.NewConfig()
	for i, h := range hosts {
		name := string(rune('a' + i))
		cfg.Contexts = append(cfg.Contexts, config.NamedContext{Name: name, Context: config.Context{
			Host: h, EnvID: "env1", TokenRef: "tok-" + name, HTTPSProxyURL: proxy,
		}})
		cfg.Tokens = append(cfg.Tokens, config.NamedToken{Name: "tok-" + name, Token: "dt0c01.TEST"})
	}
	if len(hosts) > 0 {
		cfg.CurrentContext = "a"
	}
	return cfg
}

// closedAddr returns an address nothing listens on, so a dial is refused.
func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// captureClientStderr returns what fn wrote to os.Stderr.
func captureClientStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	fn()
	os.Stderr = orig
	_ = w.Close()
	return <-done
}

// TestDeadHostFailsFast pins that a refused connection is not retried: a dead
// host used to take several retries (and up to minutes) to fail.
func TestDeadHostFailsFast(t *testing.T) {
	c, err := New("http://"+closedAddr(t), "env1", "dt0c01.TEST")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var out map[string]interface{}
	if err := c.GetV2("/problems", nil, &out); err == nil {
		t.Fatal("want an error from a refused connection")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %s, want under 2s (no retries on connection refused)", d)
	}
}

// TestNoRetryOnDNSFailure pins that a name that cannot resolve fails at once,
// and that the DNS error stays inspectable.
func TestNoRetryOnDNSFailure(t *testing.T) {
	c, err := New("http://nonexistent.invalid", "env1", "dt0c01.TEST")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var out map[string]interface{}
	err = c.GetV2("/problems", nil, &out)
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		t.Fatalf("want a *net.DNSError, got %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s, want under 5s", d)
	}
}

// TestMultiRequestAppliesVerbosity pins that -v reaches every environment's
// client, not only the single-environment path.
func TestMultiRequestAppliesVerbosity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	cfg := proxyConfig("", srv.URL, srv.URL)

	stderr := captureClientStderr(t, func() {
		_, err := MultiRequest(cfg, "a;b", 2, Options{Verbosity: 1}, func(c *Client) (interface{}, error) {
			var out map[string]interface{}
			return out, c.GetV2("/problems", nil, &out)
		})
		if err != nil {
			t.Error(err)
		}
	})
	if n := strings.Count(stderr, "==> GET"); n != 2 {
		t.Errorf("got %d request lines, want 2:\n%s", n, stderr)
	}
}

// TestNewForContextAppliesProxy pins that the factory applies the context's
// proxy, whichever command builds the client.
func TestNewForContextAppliesProxy(t *testing.T) {
	proxy := "http://" + closedAddr(t)
	c, err := NewForContext(proxyConfig(proxy, "http://managed.example.com"), "a", Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	err = c.GetV2("/problems", nil, &out)
	if err == nil || !strings.Contains(err.Error(), "via proxy "+proxy) {
		t.Errorf("want error naming the proxy, got %v", err)
	}
}

// TestGetV2PagedKeepsProxyAnnotation pins that paged requests report the
// proxy in their errors too.
func TestGetV2PagedKeepsProxyAnnotation(t *testing.T) {
	proxy := "http://" + closedAddr(t)
	c, err := NewForContext(proxyConfig(proxy, "http://managed.example.com"), "a", Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetV2Paged("/problems", nil, 0)
	if err == nil || !strings.Contains(err.Error(), "via proxy") {
		t.Errorf("want error naming the proxy, got %v", err)
	}
}
