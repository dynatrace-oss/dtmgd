package cmd

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// clusterMetadataPath is the Cluster API's cluster metadata, relative to
// {host}/api.
const clusterMetadataPath = "/v1.0/onpremise/cluster/metadata"

// ClusterDetail is the part of describe cluster's body its summary shows.
// Nodes is nil when the node list could not be read.
type ClusterDetail struct {
	ClusterName string `json:"clusterName"`
	ClusterUUID string `json:"clusterUuid"`
	Nodes       *[]struct {
		BuildVersion   string `json:"buildVersion"`
		OperationState string `json:"operationState"`
	} `json:"nodes"`
}

func newDescribeClusterCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Args:  noArgs,
		Short: "Show a Dynatrace Managed cluster's name, version and nodes (cluster context)",
		Long: `Show a cluster's name and UUID, the version it runs (the lowest node's, with
every version listed while nodes differ, as during an upgrade), and how many
nodes it has in each state. Needs a cluster context: one without env-id,
whose token has the ServiceProviderAPI scope.

json and yaml output is the API's cluster metadata with its node list under
"nodes". "dtmgd get nodes" lists the nodes.

Examples:
  dtmgd describe cluster --context prod-cluster
  dtmgd describe cluster -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runClusterFetch(fetchSpec{
				Verb:     "describe",
				Resource: "cluster",
				Fetch:    fetchCluster,
				Human:    detailHuman(printClusterDetail),
			})
		},
	}
	return clusterCommand(cmd)
}

// fetchCluster is describe cluster's Fetch: the cluster metadata, with the
// node list under "nodes". The metadata is the cluster; the nodes only add to
// the summary, so failing to read them is a warning.
func fetchCluster(c *client.Client, _ int) (fetched, error) {
	var meta map[string]interface{}
	if err := c.GetCluster(clusterMetadataPath, nil, &meta); err != nil {
		return fetched{}, err
	}
	if meta == nil {
		meta = map[string]interface{}{}
	}
	f := fetched{Body: meta, Total: -1}
	if nodes, err := getNodes(c); err != nil {
		f.Warnings = append(f.Warnings, "node list unavailable: "+err.Error())
	} else {
		meta["nodes"] = nodes
	}
	return f, nil
}

func printClusterDetail(w io.Writer, d ClusterDetail) {
	version, nodes := "unknown", "unknown"
	if d.Nodes != nil {
		versions := map[string]int{}
		states := map[string]int{}
		for _, n := range *d.Nodes {
			versions[n.BuildVersion]++
			states[n.OperationState]++
		}
		version, nodes = versionSummary(versions), fmt.Sprintf("%d%s", len(*d.Nodes), stateSummary(states))
	}
	_, _ = fmt.Fprintf(w, "Name:      %s\n", d.ClusterName)
	_, _ = fmt.Fprintf(w, "UUID:      %s\n", d.ClusterUUID)
	_, _ = fmt.Fprintf(w, "Version:   %s\n", version)
	_, _ = fmt.Fprintf(w, "Nodes:     %s\n", nodes)
}

// versionSummary is a cluster's version from how many nodes run each: the
// lowest, followed while they differ by every version (see mixedVersions).
// A node that reports no version is left out.
func versionSummary(counts map[string]int) string {
	versions := make([]string, 0, len(counts))
	for v := range counts {
		if v != "" {
			versions = append(versions, v)
		}
	}
	lowest := client.LowestClusterVersion(versions)
	if lowest == "" {
		lowest = "unknown"
	}
	if mixed := mixedVersions(counts); mixed != "" {
		return fmt.Sprintf("%s (mixed: %s)", lowest, mixed)
	}
	return lowest
}

// mixedVersions lists a cluster's versions, highest first, each with how
// many nodes run it, or "" while its nodes agree. Nodes that report no
// version are left out.
func mixedVersions(counts map[string]int) string {
	versions := make([]string, 0, len(counts))
	for v := range counts {
		if v != "" {
			versions = append(versions, v)
		}
	}
	if len(versions) <= 1 {
		return ""
	}
	sort.Slice(versions, func(i, j int) bool { return versionAbove(versions[i], versions[j]) })
	parts := make([]string, len(versions))
	for i, v := range versions {
		parts[i] = fmt.Sprintf("%s on %s", v, plural(counts[v], "node"))
	}
	return strings.Join(parts, ", ")
}

// versionAbove orders versions highest first, those that do not parse last.
func versionAbove(a, b string) bool {
	pa, okA := client.ParseClusterVersion(a)
	pb, okB := client.ParseClusterVersion(b)
	if okA != okB {
		return okA
	}
	if c := slices.Compare(pa[:], pb[:]); c != 0 {
		return c > 0
	}
	return a > b
}

// stateSummary is " (2 RUNNING, 1 OFFLINE)": the nodes in each state, most
// first, or "" for no nodes.
func stateSummary(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	states := make([]string, 0, len(counts))
	for s := range counts {
		states = append(states, s)
	}
	sort.Slice(states, func(i, j int) bool {
		if counts[states[i]] != counts[states[j]] {
			return counts[states[i]] > counts[states[j]]
		}
		return states[i] < states[j]
	})
	parts := make([]string, len(states))
	for i, s := range states {
		parts[i] = fmt.Sprintf("%d %s", counts[s], s)
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// plural is "1 node" or "n nodes".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
