package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"

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
	token        string
	// proxyURL is the proxy actually in effect, retained so that it can be
	// named in verbose output and in connection errors. Without it, "proxy
	// configured and working", "proxy configured and ignored", and "no proxy"
	// are indistinguishable from anything dtmgd prints.
	proxyURL string
	logger   *logrus.Logger
}

// NewFromConfig creates a Client from the current config context.
func NewFromConfig(cfg *config.Config) (*Client, error) {
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return nil, err
	}
	resolved, err := ctx.Resolve()
	if err != nil {
		var uerr *config.UnresolvedVarsError
		if errors.As(err, &uerr) {
			return nil, uerr.InContext(cfg.CurrentContext)
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

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	return &Client{
		http:         newRestyClient(apiBaseURL, token, 60*time.Second),
		cluster:      newRestyClient(clusterURL, token, 30*time.Second),
		apiBaseURL:   apiBaseURL,
		clusterURL:   clusterURL,
		dashboardURL: dashboardURL,
		token:        token,
		logger:       logger,
	}, nil
}

// newRestyClient builds a resty client with the settings every dtmgd request
// is expected to carry.
//
// It exists so the environment and cluster clients cannot drift apart: the
// cluster client was previously constructed inline in GetCluster with none of
// the retry, User-Agent or proxy configuration, which made `get environments`
// behave unlike every other command.
func newRestyClient(baseURL, token string, timeout time.Duration) *resty.Client {
	return resty.New().
		SetBaseURL(baseURL).
		SetHeader("Authorization", fmt.Sprintf("Api-Token %s", token)).
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", fmt.Sprintf("dtmgd/%s", version.Version)).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(10 * time.Second).
		AddRetryCondition(isRetryable).
		SetTimeout(timeout)
}

// clients returns every resty client this Client owns, so configuration
// applied after construction reaches all of them.
func (c *Client) clients() []*resty.Client {
	return []*resty.Client{c.http, c.cluster}
}

// isRetryable decides whether a failed request should be retried.
func isRetryable(r *resty.Response, err error) bool {
	if err != nil {
		return true
	}
	sc := r.StatusCode()
	return sc == 429 || sc >= 500
}

// SetVerbosity enables debug-level request/response logging.
// Level 1: summary only. Level 2+: full headers and body.
func (c *Client) SetVerbosity(level int) {
	if level <= 0 {
		return
	}
	c.logger.SetLevel(logrus.DebugLevel)
	c.logger.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp:       true,
		DisableLevelTruncation: true,
	})

	for _, rc := range c.clients() {
		rc.SetPreRequestHook(func(_ *resty.Client, req *http.Request) error {
			// Naming the proxy on the request line is what makes an active
			// proxy distinguishable from none at all. Previously no verbosity
			// level printed it, so a user who suspected their proxy was being
			// bypassed had to reach for tcpdump to find out.
			via := ""
			if c.proxyURL != "" {
				via = fmt.Sprintf(" (via proxy %s)", c.proxyURL)
			}
			fmt.Fprintf(os.Stderr, "==> %s %s%s\n", req.Method, req.URL, via)
			if level >= 2 {
				for k, v := range req.Header {
					if strings.EqualFold(k, "authorization") {
						fmt.Fprintf(os.Stderr, "    %s: [REDACTED]\n", k)
					} else {
						fmt.Fprintf(os.Stderr, "    %s: %s\n", k, strings.Join(v, ", "))
					}
				}
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

// APIBaseURL returns the environment API base URL.
func (c *Client) APIBaseURL() string { return c.apiBaseURL }

// SetProxy configures HTTP/HTTPS proxy on the client.
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
	c.proxyURL = proxy
	for _, rc := range c.clients() {
		rc.SetProxy(proxy)
	}
}

// requestError annotates a transport failure with the proxy it went through.
//
// A wrong proxy address otherwise surfaces as a bare "dial tcp: lookup
// bad-proxy.internal: no such host", which names a host the user never typed
// into their context and does not say why dtmgd was trying to reach it.
//
// The underlying error is wrapped rather than replaced, so DiagnoseError still
// matches on "no such host", "connection refused" and the rest.
func (c *Client) requestError(what string, err error) error {
	if c.proxyURL != "" {
		return fmt.Errorf("%s failed (via proxy %s): %w", what, c.proxyURL, err)
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
// Tries the cluster-level API first, falls back to the environment v1 API.
func (c *Client) ClusterVersion() (string, error) {
	var result struct {
		Version string `json:"version"`
	}
	if err := c.GetCluster("/v1.0/onpremise/cluster", nil, &result); err == nil && result.Version != "" {
		return result.Version, nil
	}
	// Fallback to environment-level v1 endpoint
	if err := c.GetV1("/config/clusterversion", nil, &result); err != nil {
		return "", err
	}
	return result.Version, nil
}
