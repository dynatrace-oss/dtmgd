package client

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"github.com/go-resty/resty/v2"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/version"
)

// Client is the HTTP client for the Dynatrace Managed API.
type Client struct {
	http *resty.Client
	// cluster targets the cluster-level API. It is built alongside http and
	// carries the same proxy, retry, header and debug-hook configuration:
	// GetCluster used to call resty.New() for every request, which silently
	// bypassed all of it — most consequentially the proxy, so `get
	// environments` escaped corporate egress controls that every other
	// command respected.
	cluster      *resty.Client
	apiBaseURL   string // {host}/e/{env-id}/api
	clusterURL   string // {host}/api
	dashboardURL string // {host}/e/{env-id}
	// proxyDisplay is the proxy actually in effect, retained so that it can
	// be named in verbose output and in connection errors. Without it, "proxy
	// configured and working", "proxy configured and ignored", and "no proxy"
	// are indistinguishable from anything dtmgd prints.
	proxyDisplay string
}

// Options are the per-invocation settings every client is built with.
type Options struct {
	// Verbosity is the -v count: 1 logs each request, 2+ adds headers and
	// bodies.
	Verbosity int
}

// NewFromConfig creates a Client for the config's current context.
func NewFromConfig(cfg *config.Config) (*Client, error) {
	return NewForContext(cfg, cfg.CurrentContext, Options{})
}

// NewForContext creates a Client for the named context: it resolves the
// context, looks up its token, and applies its proxy and o.
//
// It is the one place clients are built. Single-environment commands,
// multi-environment fan-out and `get environments` used to each assemble
// their own, and drifted: fan-out dropped -v, and `get environments` dropped
// the proxy.
func NewForContext(cfg *config.Config, name string, o Options) (*Client, error) {
	if name == "" {
		return nil, fmt.Errorf("no current context set. Run 'dtmgd config set-context' to configure one")
	}
	nc, err := cfg.GetContext(name)
	if err != nil {
		if name == cfg.CurrentContext {
			return nil, fmt.Errorf("current context %q not found in config", name)
		}
		return nil, err
	}
	resolved, err := nc.Context.Resolve()
	if err != nil {
		var uerr *config.UnresolvedVarsError
		if errors.As(err, &uerr) {
			return nil, uerr.InContext(name)
		}
		return nil, err
	}
	token, err := cfg.GetToken(resolved.TokenRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get API token: %w", err)
	}
	c, err := New(resolved.Host, resolved.EnvID, token)
	if err != nil {
		return nil, err
	}
	c.SetProxy(resolved.HTTPProxyURL, resolved.HTTPSProxyURL)
	c.SetVerbosity(o.Verbosity)
	return c, nil
}

// New creates a Client for the given Managed host and environment ID.
func New(host, envID, token string) (*Client, error) {
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if envID == "" {
		return nil, fmt.Errorf("env-id is required")
	}
	if token == "" {
		return nil, fmt.Errorf("API token is required")
	}

	// Normalise host
	host = strings.TrimRight(host, "/")

	apiBaseURL := fmt.Sprintf("%s/e/%s/api", host, envID)
	clusterURL := fmt.Sprintf("%s/api", host)
	dashboardURL := fmt.Sprintf("%s/e/%s", host, envID)

	return &Client{
		http:         newRestyClient(apiBaseURL, token, requestTimeout),
		cluster:      newRestyClient(clusterURL, token, requestTimeout),
		apiBaseURL:   apiBaseURL,
		clusterURL:   clusterURL,
		dashboardURL: dashboardURL,
	}, nil
}

// newRestyClient builds a resty client with the settings every dtmgd request
// is expected to carry.
//
// It exists so the environment and cluster clients cannot drift apart: the
// cluster client was previously constructed inline in GetCluster with none of
// the retry, User-Agent or proxy configuration, which made `get environments`
// behave unlike every other command.
//
// The retry budget bounds the time to fail: at most 3 attempts, with waits of
// at most 5s between them, and failures that cannot heal on retry (see
// isRetryable) fail on the first attempt. Only failures before the request
// reached the cluster (a dial or TLS-handshake timeout, a reset connection)
// and 429/5xx answers are retried; a response that does not arrive within
// requestTimeout fails once, because retrying it would run the same slow
// query on the cluster again.
func newRestyClient(baseURL, token string, timeout time.Duration) *resty.Client {
	return resty.New().
		SetBaseURL(baseURL).
		SetHeader("Authorization", fmt.Sprintf("Api-Token %s", token)).
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", fmt.Sprintf("dtmgd/%s", version.Version)+agentmode.UserAgentSuffix()).
		SetRetryCount(2).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second).
		AddRetryCondition(isRetryable).
		SetLogger(quietLogger{}).
		SetTimeout(timeout)
}

// requestTimeout bounds a single attempt, from dial to the last byte of the
// response. Large metric and log queries can take tens of seconds on a busy
// cluster; 30s cut those off. It is above resty's 30s dial timeout, so a dial
// that hangs fails as a (retryable) dial timeout rather than as this one.
const requestTimeout = 60 * time.Second

// quietLogger discards resty's own log lines ("WARN RESTY …"), which used to
// reach stderr on every retried request regardless of -v. SetVerbosity
// replaces it.
type quietLogger struct{}

func (quietLogger) Errorf(string, ...interface{}) { /* discarded on purpose */ }
func (quietLogger) Warnf(string, ...interface{})  { /* discarded on purpose */ }
func (quietLogger) Debugf(string, ...interface{}) { /* discarded on purpose */ }

// stderrLogger is resty's logger under -v.
type stderrLogger struct{ l *log.Logger }

func (s stderrLogger) Errorf(f string, v ...interface{}) { s.l.Printf("ERROR "+f, v...) }
func (s stderrLogger) Warnf(f string, v ...interface{})  { s.l.Printf("WARN "+f, v...) }
func (s stderrLogger) Debugf(f string, v ...interface{}) { s.l.Printf("DEBUG "+f, v...) }

// clients returns every resty client this Client owns, so configuration
// applied after construction reaches all of them.
func (c *Client) clients() []*resty.Client {
	return []*resty.Client{c.http, c.cluster}
}

// isRetryable decides whether a failed request should be retried: rate limits
// and server errors, and transport failures that can heal on their own (a
// dial or TLS-handshake timeout, a reset or truncated connection). A refused
// connection, a name that does not resolve, a TLS failure or a proxy error
// will fail the same way again, so they are not retried; retrying them made a
// dead host take minutes to report. Nor is a response timeout ("Client.Timeout
// exceeded while awaiting headers", a read timeout): the cluster got the
// request and is still working on it, and a retry would run it again.
func isRetryable(r *resty.Response, err error) bool {
	if err != nil {
		return connectTimeout(err) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, io.ErrUnexpectedEOF)
	}
	sc := r.StatusCode()
	return sc == 429 || sc >= 500
}

// connectTimeout reports whether err is a timeout before the request was
// sent: dialing the cluster or proxy, or the TLS handshake.
func connectTimeout(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" && opErr.Timeout() {
		return true
	}
	// net/http's TLS-handshake timeout error is unexported; its text is the
	// only way to recognise it.
	return strings.Contains(err.Error(), "TLS handshake timeout")
}

// SetVerbosity enables debug-level request/response logging.
// Level 1: summary only. Level 2+: full headers and body.
func (c *Client) SetVerbosity(level int) {
	if level <= 0 {
		return
	}
	for _, rc := range c.clients() {
		rc.SetLogger(stderrLogger{log.New(os.Stderr, "resty: ", 0)})
		rc.SetPreRequestHook(func(_ *resty.Client, req *http.Request) error {
			// Naming the proxy on the request line is what makes an active
			// proxy distinguishable from none at all. Previously no verbosity
			// level printed it, so a user who suspected their proxy was being
			// bypassed had to reach for tcpdump to find out.
			via := ""
			if c.proxyDisplay != "" {
				via = fmt.Sprintf(" (via proxy %s)", c.proxyDisplay)
			}
			fmt.Fprintf(os.Stderr, "==> %s %s%s\n", req.Method, req.URL, via)
			if level >= 2 {
				printRequestHeaders(req.Header)
			}
			return nil
		})

		rc.OnAfterResponse(func(_ *resty.Client, resp *resty.Response) error {
			fmt.Fprintf(os.Stderr, "<== %d %s (%s)\n", resp.StatusCode(), resp.Status(), resp.Time())
			if level >= 2 {
				fmt.Fprintf(os.Stderr, "%s\n", resp.String())
			}
			return nil
		})
	}
}

// printRequestHeaders writes h to stderr for verbosity 2 and above, one header
// per line, with the credentials in Authorization and Proxy-Authorization
// redacted.
func printRequestHeaders(h http.Header) {
	for k, v := range h {
		if strings.EqualFold(k, "authorization") || strings.EqualFold(k, "proxy-authorization") {
			fmt.Fprintf(os.Stderr, "    %s: [REDACTED]\n", k)
		} else {
			fmt.Fprintf(os.Stderr, "    %s: %s\n", k, strings.Join(v, ", "))
		}
	}
}

// APIBaseURL returns the environment API base URL.
func (c *Client) APIBaseURL() string { return c.apiBaseURL }

// SetProxy configures HTTP/HTTPS proxy on the client. resty gets the URL as
// given; everything dtmgd prints shows it through redactProxy.
//
// The proxy is applied to the cluster client too. It previously was not, so
// `get environments` connected directly while every other command went
// through the proxy — leaving proxy audit logs with an incomplete picture of
// dtmgd activity, and bypassing egress controls in environments that require
// them.
func (c *Client) SetProxy(httpProxy, httpsProxy string) {
	proxy := httpsProxy
	if proxy == "" {
		proxy = httpProxy
	}
	if proxy == "" {
		return
	}
	c.proxyDisplay = redactProxy(proxy)
	for _, rc := range c.clients() {
		rc.SetProxy(proxy)
	}
}

// redactProxy is proxy as dtmgd shows it: with the password of any userinfo
// replaced by xxxxx. Errors and -v output used to print the proxy verbatim,
// so a proxy password reached stderr, logs and agent envelopes.
func redactProxy(proxy string) string {
	u, err := url.Parse(proxy)
	switch {
	case err == nil && u.User != nil:
		return u.Redacted()
	case err == nil && !strings.Contains(u.Opaque, "@"):
		return proxy // no userinfo
	case err != nil && !strings.Contains(proxy, "@"):
		return proxy // unparseable, but holds no userinfo
	}
	// Userinfo url.Parse did not recognise as such: without a scheme,
	// "alice:pw@proxy:8080" parses as scheme "alice". Drop all of it.
	return "xxxxx" + proxy[strings.LastIndex(proxy, "@"):]
}

// requestError annotates a transport failure with the proxy it went through.
//
// A wrong proxy address otherwise surfaces as a bare "dial tcp: lookup
// bad-proxy.internal: no such host", which names a host the user never typed
// into their context and does not say why dtmgd was trying to reach it.
//
// The underlying error is wrapped rather than replaced, so Diagnose still
// matches on "no such host", "connection refused" and the rest.
func (c *Client) requestError(what string, err error) error {
	if c.proxyDisplay != "" {
		return fmt.Errorf("%s failed (via proxy %s): %w", what, c.proxyDisplay, err)
	}
	return fmt.Errorf("%s failed: %w", what, err)
}

// ClusterURL returns the cluster-level API base URL.
func (c *Client) ClusterURL() string { return c.clusterURL }

// DashboardURL returns the human-facing environment dashboard URL.
func (c *Client) DashboardURL() string { return c.dashboardURL }

// GetV2 performs a GET request against the v2 environment API.
// path should begin with "/" (e.g. "/problems").
func (c *Client) GetV2(path string, params map[string]string, result interface{}) error {
	req := c.http.R().SetResult(result)
	for k, v := range params {
		req.SetQueryParam(k, v)
	}
	resp, err := req.Get("/v2" + path)
	if err != nil {
		return c.requestError("request", err)
	}
	if resp.IsError() {
		return APIError(resp.StatusCode(), resp.String())
	}
	return nil
}

// GetV2WithValues performs a GET against the v2 environment API, supporting
// repeated query parameter keys (e.g. groupBy=a&groupBy=b).
func (c *Client) GetV2WithValues(path string, params url.Values, result interface{}) error {
	req := c.http.R().SetResult(result)
	req.SetQueryParamsFromValues(params)
	resp, err := req.Get("/v2" + path)
	if err != nil {
		return c.requestError("request", err)
	}
	if resp.IsError() {
		return APIError(resp.StatusCode(), resp.String())
	}
	return nil
}

// GetV1 performs a GET request against the v1 environment API.
func (c *Client) GetV1(path string, params map[string]string, result interface{}) error {
	req := c.http.R().SetResult(result)
	for k, v := range params {
		req.SetQueryParam(k, v)
	}
	resp, err := req.Get("/v1" + path)
	if err != nil {
		return c.requestError("request", err)
	}
	if resp.IsError() {
		return APIError(resp.StatusCode(), resp.String())
	}
	return nil
}

// GetCluster performs a GET against the cluster-level API (/api/v1.0/onpremise/...).
//
// It uses the cluster client built in New, which shares the environment
// client's proxy, retry policy, User-Agent and verbosity hooks.
func (c *Client) GetCluster(path string, params map[string]string, result interface{}) error {
	req := c.cluster.R().SetResult(result)
	for k, v := range params {
		req.SetQueryParam(k, v)
	}
	resp, err := req.Get(path)
	if err != nil {
		return c.requestError("cluster request", err)
	}
	if resp.IsError() {
		return APIError(resp.StatusCode(), resp.String())
	}
	return nil
}

// ClusterVersion fetches the cluster version.
// Tries the cluster-level API first, and falls back to the environment v1 API
// when the cluster answered without a version or with an HTTP error (a token
// without cluster scopes gets 401/403 there). After a transport error it does
// not: the fallback is on the same host and would fail the same way, after
// another full timeout.
func (c *Client) ClusterVersion() (string, error) {
	var result struct {
		Version string `json:"version"`
	}
	err := c.GetCluster("/v1.0/onpremise/cluster", nil, &result)
	if err == nil && result.Version != "" {
		return result.Version, nil
	}
	var apiErr *ErrAPI
	if err != nil && !errors.As(err, &apiErr) {
		return "", err
	}
	if err := c.GetV1("/config/clusterversion", nil, &result); err != nil {
		return "", err
	}
	return result.Version, nil
}
