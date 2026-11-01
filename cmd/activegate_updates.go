package cmd

import (
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// Cluster ActiveGate auto-update lives at the Cluster API's own endpoints:
// the Settings 2.0 schema for it is not public. Paths are relative to
// {host}/api.
const (
	clusterAGPath           = "/cluster/v2/activeGates"
	clusterAGAutoUpdatePath = clusterAGPath + "/autoUpdate"
)

// hintListClusterActiveGates is the next step after an unknown ActiveGate ID.
const hintListClusterActiveGates = "List cluster ActiveGates with: dtmgd get activegate-updates"

// clusterAGUpdatesRow is a row of get activegate-updates: the global
// configuration, then one row per cluster ActiveGate. TARGET-VERSION and
// WINDOWS show only where the cluster returns them: it honours them only
// behind a server feature flag that is off by default.
type clusterAGUpdatesRow struct {
	ID            string `table:"ID"`
	Hostname      string `table:"HOSTNAME"`
	Version       string `table:"VERSION"`
	Setting       string `table:"SETTING"`
	Effective     string `table:"EFFECTIVE"`
	Status        string `table:"STATUS"`
	TargetVersion string `table:"TARGET-VERSION"`
	Windows       string `table:"WINDOWS,max=60"`
}

// agAutoUpdatePath is one ActiveGate's auto-update path.
func agAutoUpdatePath(id string) string {
	return clusterAGPath + "/" + url.PathEscape(id) + "/autoUpdate"
}

func newGetActiveGateUpdatesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "activegate-updates [activegate-id]",
		Aliases: []string{"ag-updates"},
		Short:   "Show how the cluster's ActiveGates update",
		Long: `Show the cluster ActiveGates' automatic updates: the global setting, then
each cluster ActiveGate's own setting (ENABLED, DISABLED or INHERITED from the
global one), the setting in effect, and its update status. With an ID, that
ActiveGate's configuration.

TARGET-VERSION and WINDOWS appear only where the cluster returns them.

json and yaml print {"global": …, "activeGates": […]}, each as the Cluster API
returns it. Needs a cluster context whose token has ServiceProviderAPI.

Examples:
  dtmgd get activegate-updates --context prod-cluster
  dtmgd get activegate-updates <activegate-id> --context prod-cluster`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := fetchSpec{
				Verb:     "get",
				Resource: "activegate-updates",
				Fetch:    agUpdatesFetch,
				Rows:     agUpdatesRows(""),
				NotFound: hintListClusterActiveGates,
			}
			if len(args) == 1 {
				spec.Fetch = clusterObjectFetch(agAutoUpdatePath(args[0]), nil)
				spec.Rows = agUpdatesRows(args[0])
			}
			return a.runClusterFetch(spec)
		},
	}
	return clusterCommand(cmd)
}

func newDescribeActiveGateUpdatesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "activegate-updates [activegate-id]",
		Aliases: []string{"ag-updates"},
		Short:   "Show the cluster ActiveGates' auto-update configuration",
		Long: `Show the global auto-update configuration of the cluster's ActiveGates, or
one ActiveGate's, as the Cluster API returns it. update activegate-updates
takes the same object.

Examples:
  dtmgd describe activegate-updates --context prod-cluster
  dtmgd describe activegate-updates <activegate-id> -o yaml --context prod-cluster`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := clusterAGAutoUpdatePath
			if len(args) == 1 {
				path = agAutoUpdatePath(args[0])
			}
			return a.runClusterFetch(fetchSpec{
				Verb:     "describe",
				Resource: "activegate-updates",
				Fetch:    clusterObjectFetch(path, nil),
				NotFound: hintListClusterActiveGates,
			})
		},
	}
	return clusterCommand(cmd)
}

// agUpdatesFetch reads the global configuration and the cluster's own
// ActiveGates (type CLUSTER), which carry their auto-update setting and
// status.
func agUpdatesFetch(c *client.Client, _ int) (fetched, error) {
	var global map[string]interface{}
	if err := c.GetCluster(clusterAGAutoUpdatePath, nil, &global); err != nil {
		return fetched{}, err
	}
	var list map[string]interface{}
	if err := c.GetCluster(clusterAGPath, map[string]string{"type": "CLUSTER"}, &list); err != nil {
		return fetched{}, err
	}
	gates, _ := list["activeGates"].([]interface{})
	if gates == nil {
		gates = []interface{}{}
	}
	return fetched{Body: map[string]interface{}{"global": global, "activeGates": gates}, Total: len(gates)}, nil
}

// agUpdatesRows turns get activegate-updates' body into rows: the global
// row and one per ActiveGate, or, for one ActiveGate (id), its row.
func agUpdatesRows(id string) func(body interface{}) (interface{}, int, error) {
	return func(body interface{}) (interface{}, int, error) {
		m, _ := body.(map[string]interface{})
		if id != "" {
			row := configRow(m)
			row.ID = id
			return []clusterAGUpdatesRow{row}, 1, nil
		}
		global, _ := m["global"].(map[string]interface{})
		row := configRow(global)
		row.ID, row.Setting, row.Effective = "(global)", cell(global, "globalSetting"), cell(global, "globalSetting")
		rows := []clusterAGUpdatesRow{row}
		gates, _ := m["activeGates"].([]interface{})
		for _, g := range gates {
			rows = append(rows, gateRow(g))
		}
		return rows, len(rows), nil
	}
}

// configRow is the row of an auto-update configuration object.
func configRow(cfg map[string]interface{}) clusterAGUpdatesRow {
	return clusterAGUpdatesRow{
		Hostname: noValue, Version: noValue, Status: noValue,
		Setting:       cell(cfg, "setting"),
		Effective:     cell(cfg, "effectiveSetting"),
		TargetVersion: cell(cfg, "targetVersion"),
		Windows:       configWindows(cfg),
	}
}

// gateRow is the row of one ActiveGate of the list.
func gateRow(g interface{}) clusterAGUpdatesRow {
	gate, _ := g.(map[string]interface{})
	settings := nested(gate, "autoUpdateSettings")
	return clusterAGUpdatesRow{
		ID: cell(gate, "id"), Hostname: cell(gate, "hostname"), Version: cell(gate, "version"),
		Setting: cell(settings, "setting"), Effective: cell(settings, "effectiveSetting"),
		Status: cell(gate, "autoUpdateStatus"), TargetVersion: noValue, Windows: noValue,
	}
}

// configWindows names the update windows of a configuration
// (updateWindows.windows[].name), or noValue without any.
func configWindows(cfg map[string]interface{}) string {
	list, _ := nested(cfg, "updateWindows")["windows"].([]interface{})
	var names []string
	for _, w := range list {
		win, _ := w.(map[string]interface{})
		names = append(names, cell(win, "name"))
	}
	if len(names) == 0 {
		return noValue
	}
	return strings.Join(names, ", ")
}
