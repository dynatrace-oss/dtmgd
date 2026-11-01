package cmd

import (
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
	return pagedWith(settingsPaged, path, params, listKey, pageCap, maxPages)
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
	if a.onClusterContext(cfg) {
		s.Forbidden = hintClusterSettingsScopes
		return a.runClusterFetch(s)
	}
	return a.runFetch(s)
}
