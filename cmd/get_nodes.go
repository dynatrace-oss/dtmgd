package cmd

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// The Cluster API's node endpoints, relative to {host}/api.
const (
	nodesPath             = "/v1.0/onpremise/cluster"
	nodeConfigurationPath = "/v1.0/onpremise/cluster/configuration"
)

// clusterNode is the part of a node, as get nodes returns it, that the table
// shows: the node list's own fields, and the node's entry from the
// configuration endpoint under "configuration" (nil when unknown).
type clusterNode struct {
	ID                   int    `json:"id"`
	ClusterMemberAddress string `json:"clusterMemberAddress"`
	OperationState       string `json:"operationState"`
	BuildVersion         string `json:"buildVersion"`
	OSInfo               string `json:"osInfo"`
	Configuration        *struct {
		IPAddress  string `json:"ipAddress"`
		WebUI      bool   `json:"webUI"`
		Agent      bool   `json:"agent"`
		Datacenter string `json:"datacenter"`
	} `json:"configuration"`
}

// nodeRow is a get nodes table row.
type nodeRow struct {
	ID         int    `table:"ID"`
	Address    string `table:"ADDRESS"`
	State      string `table:"STATE"`
	Version    string `table:"VERSION"`
	Datacenter string `table:"DATACENTER"`
	Roles      string `table:"ROLES"`
	OS         string `table:"OS,wide"`
}

func newGetNodesCmd(a *App) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:     "nodes",
		Args:    noArgs,
		Aliases: []string{"node"},
		Short:   "List the nodes of a Dynatrace Managed cluster (cluster context)",
		Long: `List a cluster's nodes from the Cluster Management API: each node's state and
version, and from the node configuration its address, datacenter and roles
(web UI, agent traffic). Needs a cluster context: one without env-id, whose
token has the ServiceProviderAPI scope.

json and yaml output is the API's node list, each node with its
configuration entry under "configuration".

Examples:
  dtmgd get nodes --context prod-cluster
  dtmgd get nodes -o wide`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runClusterFetch(fetchSpec{
				Verb:     "get",
				Resource: "nodes",
				Fetch:    fetchNodes,
				Rows:     nodeRows,
				Limit:    limit,
				List:     true,
				Empty:    "The cluster reported no nodes.",
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of nodes to return")
	return clusterCommand(cmd)
}

// fetchNodes is get nodes' Fetch: the node list, each node joined with its
// configuration. The configuration only adds columns, so failing to read it
// is a warning, not a failure.
func fetchNodes(c *client.Client, limit int) (fetched, error) {
	nodes, err := getNodes(c)
	if err != nil {
		return fetched{}, err
	}
	f := fetched{Total: len(nodes)}
	var conf struct {
		ClusterNodes []map[string]interface{} `json:"clusterNodes"`
	}
	if err := c.GetCluster(nodeConfigurationPath, nil, &conf); err != nil {
		f.Warnings = append(f.Warnings, "node roles unavailable: "+err.Error())
	} else {
		joinNodeConfiguration(nodes, conf.ClusterNodes)
	}
	if limit > 0 && len(nodes) > limit {
		nodes, f.HasMore = nodes[:limit], true
	}
	f.Body, f.Count = nodes, len(nodes)
	return f, nil
}

// getNodes is the Cluster API's node list (an array, never nil).
func getNodes(c *client.Client) ([]interface{}, error) {
	var nodes []interface{}
	if err := c.GetCluster(nodesPath, nil, &nodes); err != nil {
		return nil, err
	}
	if nodes == nil {
		nodes = []interface{}{}
	}
	return nodes, nil
}

// joinNodeConfiguration puts each node's configuration entry, the one with
// the same id, under the node's "configuration" key.
func joinNodeConfiguration(nodes []interface{}, configs []map[string]interface{}) {
	byID := map[float64]map[string]interface{}{}
	for _, nc := range configs {
		if id, ok := nc["id"].(float64); ok {
			byID[id] = nc
		}
	}
	for _, n := range nodes {
		node, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := node["id"].(float64); ok && byID[id] != nil {
			node["configuration"] = byID[id]
		}
	}
}

// nodeRows is get nodes' Rows.
func nodeRows(body interface{}) (interface{}, int, error) {
	var nodes []clusterNode
	if err := decodeBody(body, &nodes); err != nil {
		return nil, 0, err
	}
	rows := make([]nodeRow, len(nodes))
	for i, n := range nodes {
		rows[i] = nodeRow{ID: n.ID, Address: n.ClusterMemberAddress, State: n.OperationState, Version: n.BuildVersion,
			Datacenter: "—", Roles: "—", OS: n.OSInfo}
		if conf := n.Configuration; conf != nil {
			if conf.IPAddress != "" {
				rows[i].Address = conf.IPAddress
			}
			if conf.Datacenter != "" {
				rows[i].Datacenter = conf.Datacenter
			}
			rows[i].Roles = nodeRoles(conf.WebUI, conf.Agent)
		}
	}
	return rows, len(rows), nil
}

// nodeRoles names what a node serves, or "—" for neither.
func nodeRoles(webUI, agent bool) string {
	var roles []string
	if webUI {
		roles = append(roles, "web UI")
	}
	if agent {
		roles = append(roles, "agents")
	}
	if len(roles) == 0 {
		return "—"
	}
	return strings.Join(roles, ", ")
}

// decodeBody decodes a result body, as fetched from the API, into out.
func decodeBody(body, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
