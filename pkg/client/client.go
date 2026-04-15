package client

import (
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
	http         *resty.Client
	apiBaseURL   string // {host}/e/{env-id}/api
	clusterURL   string // {host}/api
	dashboardURL string // {host}/e/{env-id}
	token        string
	logger       *logrus.Logger
}

// NewFromConfig creates a Client from the current config context.
func NewFromConfig(cfg *config.Config) (*Client, error) {
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return nil, err
	}
	token, err := cfg.GetToken(ctx.TokenRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get API token: %w", err)
	}
	c, err := New(ctx.Host, ctx.EnvID, token)
	if err != nil {
		return nil, err
	}
	c.SetProxy(ctx.HTTPProxyURL, ctx.HTTPSProxyURL)
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

	ua := fmt.Sprintf("dtmgd/%s", version.Version)

	httpClient := resty.New().
		SetBaseURL(apiBaseURL).
		SetHeader("Authorization", fmt.Sprintf("Api-Token %s", token)).
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", ua).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(10 * time.Second).
		AddRetryCondition(isRetryable).
		SetTimeout(60 * time.Second)

	return &Client{
		http:         httpClient,
		apiBaseURL:   apiBaseURL,
		clusterURL:   clusterURL,
		dashboardURL: dashboardURL,
		token:        token,
		logger:       logger,
	}, nil
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

	c.http.SetPreRequestHook(func(_ *resty.Client, req *http.Request) error {
		fmt.Fprintf(os.Stderr, "==> %s %s\n", req.Method, req.URL)
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

	c.http.OnAfterResponse(func(_ *resty.Client, resp *resty.Response) error {
		fmt.Fprintf(os.Stderr, "<== %d %s (%s)\n", resp.StatusCode(), resp.Status(), resp.Time())
		if level >= 2 {
			fmt.Fprintf(os.Stderr, "%s\n", resp.String())
		}
		return nil
	})
}

// APIBaseURL returns the environment API base URL.
func (c *Client) APIBaseURL() string { return c.apiBaseURL }

// SetProxy configures HTTP/HTTPS proxy on the client.
func (c *Client) SetProxy(httpProxy, httpsProxy string) {
	proxy := httpsProxy
	if proxy == "" {
		proxy = httpProxy
	}
	if proxy != "" {
		c.http.SetProxy(proxy)
	}
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
		return fmt.Errorf("request failed: %w", err)
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
		return fmt.Errorf("request failed: %w", err)
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
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return APIError(resp.StatusCode(), resp.String())
	}
	return nil
}

// GetCluster performs a GET against the cluster-level API (/api/v1.0/onpremise/...).
// Uses a temporary resty client rooted at clusterURL.
func (c *Client) GetCluster(path string, params map[string]string, result interface{}) error {
	tmpClient := resty.New().
		SetBaseURL(c.clusterURL).
		SetHeader("Authorization", fmt.Sprintf("Api-Token %s", c.token)).
		SetHeader("Content-Type", "application/json").
		SetTimeout(30 * time.Second)

	req := tmpClient.R().SetResult(result)
	for k, v := range params {
		req.SetQueryParam(k, v)
	}
	resp, err := req.Get(path)
	if err != nil {
		return fmt.Errorf("cluster request failed: %w", err)
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
