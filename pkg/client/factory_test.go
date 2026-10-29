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
	// 4s, not 2s: Windows takes about 1-2s to refuse a loopback connect. The
	// old retry policy took 5.5s here, so 4s still tells the two apart.
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("took %s, want under 4s (no retries on connection refused)", d)
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
	_, err = c.GetV2Paged("/problems", nil, Paging{MaxPages: 0})
	if err == nil || !strings.Contains(err.Error(), "via proxy") {
		t.Errorf("want error naming the proxy, got %v", err)
	}
}

// TestProxyCredentialsAreRedacted pins that a proxy URL's password never
// reaches an error, -v output or (through the error) an agent envelope: the
// proxy was printed verbatim, so http://user:password@proxy put the password
// in stderr and in logs that capture it.
func TestProxyCredentialsAreRedacted(t *testing.T) {
	const secret = "s3cr3t"

	t.Run("errors", func(t *testing.T) {
		proxy := "http://alice:" + secret + "@" + closedAddr(t)
		c, err := New("http://managed.example.com", "env1", "dt0c01.TEST")
		if err != nil {
			t.Fatal(err)
		}
		c.SetProxy("", proxy)
		var out map[string]interface{}
		err = c.GetV2("/problems", nil, &out)
		if err == nil || !strings.Contains(err.Error(), "alice:xxxxx@") || strings.Contains(err.Error(), secret) {
			t.Errorf("GetV2 error = %v; want the proxy named with its password redacted", err)
		}
		if got := c.requestError("request", errNotReachable{}).Error(); strings.Contains(got, secret) || !strings.Contains(got, "alice:xxxxx@") {
			t.Errorf("requestError = %q", got)
		}
	})

	t.Run("verbose", func(t *testing.T) {
		// A forward proxy that answers every request itself.
		proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"problems":[]}`)
		}))
		defer proxySrv.Close()
		proxy := strings.Replace(proxySrv.URL, "http://", "http://alice:"+secret+"@", 1)
		for _, level := range []int{1, 2} {
			c, err := New("http://managed.example.com", "env1", "dt0c01.TEST")
			if err != nil {
				t.Fatal(err)
			}
			c.SetProxy("", proxy)
			c.SetVerbosity(level)
			var reqErr error
			stderr := captureClientStderr(t, func() {
				var out map[string]interface{}
				reqErr = c.GetV2("/problems", nil, &out)
			})
			if reqErr != nil {
				t.Fatalf("-v level %d: request through the proxy failed: %v", level, reqErr)
			}
			if strings.Contains(stderr, secret) || !strings.Contains(stderr, "via proxy http://alice:xxxxx@") {
				t.Errorf("-v level %d stderr:\n%s", level, stderr)
			}
		}
	})

	t.Run("unusual forms", func(t *testing.T) {
		for _, proxy := range []string{
			"alice:" + secret + "@proxy.example.com:8080",   // no scheme: parses as scheme "alice"
			"http://alice:" + secret + "@proxy example.com", // does not parse
		} {
			if got := RedactProxy(proxy); strings.Contains(got, secret) {
				t.Errorf("RedactProxy(%q) = %q", proxy, got)
			}
		}
		if got := RedactProxy("http://proxy.example.com:8080"); got != "http://proxy.example.com:8080" {
			t.Errorf("redactProxy changed a proxy without credentials: %q", got)
		}
	})
}

// TestVerboseBodyDumpRedactsLicenseKey pins that -vv's response dump, which
// prints bodies verbatim, does not print a license key: describe license
// masks it in every output format, and -vv used to undo that on stderr.
func TestVerboseBodyDumpRedactsLicenseKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"licenseKey" : "SECRET-KEY-1234","licenseType":"FULL"}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "", "dt0c01.TEST")
	c.SetVerbosity(2)
	stderr := captureClientStderr(t, func() {
		var out map[string]interface{}
		if err := c.GetCluster("/cluster/v2/clusterLicense", nil, &out); err != nil {
			t.Error(err)
		}
	})
	if strings.Contains(stderr, "SECRET-KEY-1234") || !strings.Contains(stderr, `"licenseKey":"[REDACTED]"`) || !strings.Contains(stderr, `"licenseType":"FULL"`) {
		t.Errorf("stderr:\n%s", stderr)
	}
}

// TestNewForContextEmptyResolvedEnvID pins the error for an environment
// context whose env-id is a ${VAR} reference set to "": it names the
// variable's result, rather than calling the context a cluster context.
func TestNewForContextEmptyResolvedEnvID(t *testing.T) {
	t.Setenv("DTMGD_TEST_EMPTY_ENV_ID", "")
	cfg := proxyConfig("", "https://h.example.com")
	cfg.Contexts[0].Context.EnvID = "${DTMGD_TEST_EMPTY_ENV_ID}"
	_, err := NewForContext(cfg, "a", Options{})
	if err == nil || !strings.Contains(err.Error(), `env-id "${DTMGD_TEST_EMPTY_ENV_ID}" resolved to an empty value`) || strings.Contains(err.Error(), "cluster context") {
		t.Errorf("err = %v", err)
	}
}
