package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// The Settings 2.0 API answers in two places with the same operations: the
// environment API ({host}/e/{env}/api/v2/settings/…) for an environment
// context, and the Cluster API ({host}/api/cluster/v2/settings/…) for a
// cluster context, whose scope is "cluster". Every settings call goes
// through these functions, which pick the API by the client, so the
// commands, the write runner and the lookups have one code path for both.
// Paths are the environment API's (settingsObjectsPath, …).

// clusterSettingsPrefix turns an environment settings path into the
// Cluster API's, relative to {host}/api.
const clusterSettingsPrefix = "/cluster/v2"

func settingsGet(c *client.Client, path string, params map[string]string, out interface{}) error {
	if c.ClusterScoped() {
		return c.GetCluster(clusterSettingsPrefix+path, params, out)
	}
	return c.GetV2(path, params, out)
}

func settingsPaged(c *client.Client, path string, params map[string]string, p client.Paging) (map[string]interface{}, error) {
	if c.ClusterScoped() {
		return c.GetClusterPaged(clusterSettingsPrefix+path, params, p)
	}
	return c.GetV2Paged(path, params, p)
}

// settingsPagedFetch is pagedFetch for a settings list, in either API.
func settingsPagedFetch(path string, params map[string]string, listKey string, pageCap, maxPages int) func(*client.Client, int) (fetched, error) {
	return pagedWith(settingsPaged, path, params, listKey, 0, pageCap, maxPages)
}

// settingsObjectFetch is describeFetch for one settings object or schema,
// in either API.
func settingsObjectFetch(path string) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, _ int) (fetched, error) {
		var m map[string]interface{}
		if err := settingsGet(c, path, nil, &m); err != nil {
			return fetched{}, err
		}
		return fetched{Body: m, Total: -1}, nil
	}
}

func settingsPost(c *client.Client, path string, params map[string]string, body, out interface{}) error {
	if c.ClusterScoped() {
		return c.PostCluster(clusterSettingsPrefix+path, params, body, out)
	}
	return c.PostV2(path, params, body, out)
}

func settingsPut(c *client.Client, path string, params map[string]string, body, out interface{}) error {
	if c.ClusterScoped() {
		return c.PutCluster(clusterSettingsPrefix+path, params, body, out)
	}
	return c.PutV2(path, params, body, out)
}

func settingsDelete(c *client.Client, path string, params map[string]string) error {
	if c.ClusterScoped() {
		return c.DeleteCluster(clusterSettingsPrefix+path, params)
	}
	return c.DeleteV2(path, params)
}

// hintClusterSettingsScopes is the 403 hint of a settings command on a
// cluster context. Cluster tokens made for the cluster commands carry only
// ServiceProviderAPI, which does not reach the cluster's settings.
const hintClusterSettingsScopes = "A cluster token needs settings.read (and settings.write to write) besides ServiceProviderAPI; add them to the token in the Cluster Management Console."

// hintObjectNotFound is the next step after an unknown settings object ID.
const hintObjectNotFound = "No settings object has this ID in this context; find object IDs with: dtmgd get settings --schema <schema-id>"

// settingsObjectByID is settingsObjectFetch for one object by ID, a 404
// hinting at how to find IDs.
func settingsObjectByID(id string) func(*client.Client, int) (fetched, error) {
	return hintOnNotFound(settingsObjectFetch(settingsObjectsPath+"/"+url.PathEscape(id)), hintObjectNotFound)
}

// getSettingsObject reads one object by ID into out, a 404 hinting at how
// to find IDs.
func getSettingsObject(c *client.Client, id string, out interface{}) error {
	err := settingsGet(c, settingsObjectsPath+"/"+url.PathEscape(id), nil, out)
	var apiErr *client.ErrAPI
	if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
		return withHint(err, hintObjectNotFound)
	}
	return err
}

// refuseClusterFanOut refuses, before any request, a cluster context in a
// multi-environment run: named with --context beside --env "a;b", or inside
// the --env list. A cluster context runs alone.
func (a *App) refuseClusterFanOut(cfg *config.Config) error {
	if !a.isMultiEnv() {
		return nil
	}
	names := []string{a.Opts.Context}
	if a.Opts.Env != "ALL_ENVIRONMENTS" {
		names = append(names, strings.Split(a.Opts.Env, ";")...)
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if nc, err := cfg.GetContext(name); err == nil && nc.Context.IsCluster() {
			return &UsageError{Err: fmt.Errorf("context %q is a cluster context, which cannot be part of a multi-environment --env; run it alone: --context %s", name, name)}
		}
	}
	return nil
}

// onClusterContext reports whether the command runs against one cluster
// context: no multi-environment --env, and a current context without
// env-id.
func (a *App) onClusterContext(cfg *config.Config) bool {
	if a.isMultiEnv() {
		return false
	}
	nc, err := cfg.GetContext(cfg.CurrentContext)
	return err == nil && nc.Context.IsCluster()
}

// runSettingsFetch runs a settings read: through runClusterFetch on a
// cluster context, through runFetch otherwise.
func (a *App) runSettingsFetch(s fetchSpec) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	if err := a.refuseClusterFanOut(cfg); err != nil {
		return err
	}
	if a.onClusterContext(cfg) {
		s.Forbidden = hintClusterSettingsScopes
		return a.runClusterFetch(s)
	}
	return a.runFetch(s)
}

// settingsPageCap is the largest page the settings listings ask for.
const settingsPageCap = 500

// clusterScopeFallback wraps a settings listing made without schemaIds. The
// cluster cannot list by scope alone: it fails with 404 "No schema with
// schemaId <first schema>", and a schemaIds-less effectiveValues fails the
// same way. On that error, on a cluster only, the fallback lists every
// schema (8 at a time) and merges the results in schema ID order; a schema
// that cannot answer at the scope (cannotAnswerAtScope) is skipped, any other
// error fails. Total stays -1 when a schema had more than it returned.
func clusterScopeFallback(fetch func(*client.Client, int) (fetched, error), path string, params map[string]string, maxPages int) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		f, err := fetch(c, limit)
		var apiErr *client.ErrAPI
		if err == nil || !c.ClusterScoped() || !errors.As(err, &apiErr) || apiErr.StatusCode != 404 ||
			!strings.HasPrefix(apiErr.Message(), "No schema with schemaId") {
			return f, err
		}
		var list struct {
			Items []struct {
				SchemaID string `json:"schemaId"`
			} `json:"items"`
		}
		if err := settingsGet(c, settingsSchemasPath, nil, &list); err != nil {
			return fetched{}, err
		}
		ids := make([]string, 0, len(list.Items))
		for _, it := range list.Items {
			ids = append(ids, it.SchemaID)
		}
		sort.Strings(ids)
		// --limit is applied after merging, so a small --limit still asks
		// every schema. The schemas are read in parallel (8 at a time), so
		// none can know how many items the schemas before it in ID order
		// return, and the total and "more exist" need every schema's answer.
		return mergeSchemaListings(c, ids, limit, scopeOf(params), func(id string) func(*client.Client, int) (fetched, error) {
			p := make(map[string]string, len(params)+1)
			for k, v := range params {
				p[k] = v
			}
			p["schemaIds"] = id
			return settingsPagedFetch(path, p, "items", settingsPageCap, maxPages)
		})
	}
}

// cannotAnswerAtScope reports whether a per-schema answer means "this schema
// cannot answer at this scope": a 404, or the 400 "Not allowed to query
// scope …" of a schema whose allowed scopes are others (seen on effective
// values, for example realm-only schemas).
func cannotAnswerAtScope(err error) bool {
	var apiErr *client.ErrAPI
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == 404 ||
		(apiErr.StatusCode == 400 && strings.HasPrefix(apiErr.Message(), "Not allowed to query scope"))
}

// mergeSchemaListings runs the listing of each schema ID, 8 at a time, and
// concatenates the items in ID order.
func mergeSchemaListings(c *client.Client, ids []string, limit int, scope string, listingOf func(string) func(*client.Client, int) (fetched, error)) (fetched, error) {
	results := make([]fetched, len(ids))
	errs := make([]error, len(ids))
	sem := make(chan struct{}, definitionWorkers)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i], errs[i] = listingOf(id)(c, limit)
		}()
	}
	wg.Wait()
	items, more, answered, firstSkipped, err := collectListings(results, errs)
	if err != nil {
		return fetched{}, err
	}
	// A valid scope has at least one schema answering 200, even with no items.
	if answered == 0 && firstSkipped != nil {
		return fetched{}, withHint(firstSkipped, fmt.Sprintf("No settings schema of this cluster can be read at scope %s. Cluster scopes are cluster, environment-default, realm and metric; check the scopes with dtmgd describe settings-schema <schema-id>.", scope))
	}
	body := map[string]interface{}{"items": items}
	total := -1
	if !more {
		total = len(items)
		body["totalCount"] = total
	}
	if limit > 0 && len(items) > limit {
		items, more = items[:limit], true
		body["items"] = items
	}
	return fetched{Body: body, Total: total, HasMore: more, Count: len(items)}, nil
}

// scopeOf names the scope a settings listing asks for, for messages.
func scopeOf(params map[string]string) string {
	if s := params["scope"]; s != "" {
		return s
	}
	return params["scopes"]
}

// collectListings concatenates the per-schema answers in order. A schema
// that cannot answer at the scope is skipped (the first such error is
// returned for reporting); any other error fails.
func collectListings(results []fetched, errs []error) (items []interface{}, more bool, answered int, firstSkipped, err error) {
	items = []interface{}{}
	for i := range results {
		if errs[i] != nil {
			if !cannotAnswerAtScope(errs[i]) {
				return nil, false, 0, nil, errs[i]
			}
			if firstSkipped == nil {
				firstSkipped = errs[i]
			}
			continue
		}
		answered++
		body, _ := results[i].Body.(map[string]interface{})
		list, ok := body["items"].([]interface{})
		if !ok {
			return nil, false, 0, nil, errors.New("a per-schema settings listing came back without its items list")
		}
		items = append(items, list...)
		more = more || results[i].HasMore
	}
	return items, more, answered, firstSkipped, nil
}
