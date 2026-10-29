package cmd

import (
	"net/url"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// Dynatrace UI pages a result links to: the pages the Dynatrace Managed MCP
// server points users to (src/capabilities/*-api.ts), relative to an
// environment's UI base URL, {host}/e/{env-id}. Where the MCP server writes a
// placeholder such as <problemId>, dtmgd fills in the result's own ID.
const (
	uiProblems        = "/ui/problems"
	uiProblemDetails  = "/#problems/problemdetails;pid="
	uiEntity          = "/ui/entity/"
	uiVulnerabilities = "/ui/security/vulnerabilities"
	uiVulnerability   = uiVulnerabilities + "/"
	uiSLOs            = "/ui/slo"
	uiLogs            = "/ui/log-monitoring"
	uiMetrics         = "/ui/metrics"
	uiDataExplorer    = "/ui/data-explorer"
)

// uiPage is a fetchSpec.Link to a fixed page, whatever the result.
func uiPage(path string) func(base string, body interface{}) string {
	return func(base string, _ interface{}) string { return base + path }
}

// uiItem is a fetchSpec.Link to the page of the item a describe result is:
// prefix followed by the body's idKey, the API's own ID for the item, or ""
// when the body has none. The API's ID is the one to link even when the
// command was given another (a security problem's display ID, say).
func uiItem(prefix, idKey string) func(base string, body interface{}) string {
	return func(base string, body interface{}) string {
		m, _ := body.(map[string]interface{})
		id, _ := m[idKey].(string)
		return uiID(prefix, id)(base, nil)
	}
}

// uiID is a fetchSpec.Link to the page of a known item: prefix followed by
// id, path-escaped so that no character of it can end the path or start a
// query or fragment. An empty id has no page.
func uiID(prefix, id string) func(base string, body interface{}) string {
	return func(base string, _ interface{}) string {
		if id == "" {
			return ""
		}
		return base + prefix + url.PathEscape(id)
	}
}

// uiLink is the Dynatrace UI link for one environment's result, or "" when
// the command has none or the environment has no UI base URL to build on.
func (s fetchSpec) uiLink(f fetched) string {
	if s.Link == nil || f.UIBase == "" {
		return ""
	}
	return s.Link(f.UIBase, f.Body)
}

// singleLinks is the agent context's links for a result rendered on its own
// (renderOne): {"ui": url}, or nil when there is no link.
func singleLinks(s fetchSpec, f fetched) map[string]string {
	if l := s.uiLink(f); l != "" {
		return map[string]string{"ui": l}
	}
	return nil
}

// envLinks is the agent context's links for results rendered as a map keyed
// by context name (renderMulti): each environment that answered gets its own
// link under its name, a failed one gets none, and nil means no environment
// has a link. One environment's link never stands for another's.
func envLinks(s fetchSpec, results []client.EnvResult) map[string]string {
	var links map[string]string
	for _, r := range results {
		if r.Error != nil {
			continue
		}
		l := s.uiLink(r.Data.(fetched))
		if l == "" {
			continue
		}
		if links == nil {
			links = map[string]string{}
		}
		links[r.Name] = l
	}
	return links
}

// printDescribeLink names a describe result's Dynatrace UI page on stderr,
// for a person, so stdout keeps only the detail. Lists print no link.
func printDescribeLink(s fetchSpec, f fetched) {
	if s.Verb != "describe" {
		return
	}
	if l := s.uiLink(f); l != "" {
		output.PrintInfo("Open in Dynatrace: %s", l)
	}
}
