package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	clusterNodesPath    = "GET /api/v1.0/onpremise/cluster"
	clusterNodesCfgPath = "GET /api/v1.0/onpremise/cluster/configuration"
	clusterMetaPath     = "GET /api/v1.0/onpremise/cluster/metadata"
)

// nodeRoutes serves the three-node cluster: its nodes, their configuration
// and its metadata.
func nodeRoutes() map[string]route {
	return map[string]route{
		clusterNodesPath:    {File: "cluster_nodes.json"},
		clusterNodesCfgPath: {File: "cluster_nodes_configuration.json"},
		clusterMetaPath:     {File: "cluster_metadata.json"},
	}
}

// withRoute is routes with key answering rt instead.
func withRoute(routes map[string]route, key string, rt route) map[string]route {
	routes[key] = rt
	return routes
}

// TestGetNodesTable pins get nodes' table: each node's state and version
// from the node list, its address, datacenter and roles from its
// configuration.
func TestGetNodesTable(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", nodeRoutes())
	assertGolden(t, "get_nodes_table", runCLI(t, cfg, "get", "nodes").String())
}

// TestGetNodesJSONCarriesConfiguration pins the json shape: the API's node
// list, each node with its configuration under "configuration".
func TestGetNodesJSONCarriesConfiguration(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", nodeRoutes())
	r := runCLI(t, cfg, "get", "nodes", "-o", "json")
	var nodes []map[string]interface{}
	if err := json.Unmarshal([]byte(r.Stdout), &nodes); err != nil || len(nodes) != 3 {
		t.Fatalf("stdout is not three nodes (%v):\n%s", err, r.Stdout)
	}
	conf, _ := nodes[0]["configuration"].(map[string]interface{})
	if nodes[0]["id"] != float64(1) || conf["datacenter"] != "dc-1" || nodes[0]["buildVersion"] == nil {
		t.Errorf("node 1 = %v", nodes[0])
	}
}

// TestGetNodesConfigurationUnavailable pins that the node list does not
// depend on the configuration call: without it, the roles are unknown and a
// warning says why.
func TestGetNodesConfigurationUnavailable(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", withRoute(nodeRoutes(), clusterNodesCfgPath, route{Status: 403, File: "error_403.json"}))
	r := runCLI(t, cfg, "get", "nodes")
	if r.Code != 0 || !strings.Contains(r.Stdout, "10.0.0.1:5701") {
		t.Errorf("code %d, stdout:\n%s", r.Code, r.Stdout)
	}
	if strings.Contains(r.Stdout, "dc-1") || !strings.Contains(r.Stdout, "—") {
		t.Errorf("want unknown datacenter and roles:\n%s", r.Stdout)
	}
	wantStderr(t, r, "node roles unavailable: API error 403")
}

// TestGetNodesLimit pins --limit on a list the API returns whole.
func TestGetNodesLimit(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", nodeRoutes())
	r := runCLI(t, cfg, "get", "nodes", "--limit", "2")
	if strings.Count(r.Stdout, "\n") != 3 {
		t.Errorf("want a header and two rows:\n%s", r.Stdout)
	}
	wantStderr(t, r, "Showing 2 of 3 nodes")
}

// TestDescribeClusterMixedVersions pins describe cluster's summary of a
// cluster mid-upgrade: its lowest version, the versions it runs, and its
// nodes' states.
func TestDescribeClusterMixedVersions(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", nodeRoutes())
	assertGolden(t, "describe_cluster_human", runCLI(t, cfg, "describe", "cluster").String())
}

// TestDescribeClusterJSON pins describe cluster's json: the metadata object
// with the node list under "nodes".
func TestDescribeClusterJSON(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", nodeRoutes())
	r := runCLI(t, cfg, "describe", "cluster", "-o", "json")
	var got struct {
		ClusterName string        `json:"clusterName"`
		Nodes       []interface{} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &got); err != nil || got.ClusterName != "test-cluster" || len(got.Nodes) != 3 {
		t.Errorf("got %+v (%v):\n%s", got, err, r.Stdout)
	}
}

// TestDescribeClusterNodesUnavailable pins that describe cluster still shows
// the cluster when its node list cannot be read.
func TestDescribeClusterNodesUnavailable(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", withRoute(nodeRoutes(), clusterNodesPath, route{Status: 403, File: "error_403.json"}))
	r := runCLI(t, cfg, "describe", "cluster")
	if r.Code != 0 || !strings.Contains(r.Stdout, "test-cluster") || !strings.Contains(r.Stdout, "Version:   unknown") {
		t.Errorf("code %d, stdout:\n%s", r.Code, r.Stdout)
	}
	wantStderr(t, r, "node list unavailable: API error 403")
}
