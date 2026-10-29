package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// hintNoEnvironment is describe environment's hint for a 404, which the
// Cluster API gives both for an unknown ID and for a host without it.
const hintNoEnvironment = "No environment %q on this cluster, or no Cluster API at this context's host. List environments with: dtmgd get environments"

func newDescribeEnvironmentCmd(a *App) *cobra.Command {
	var consumption bool
	cmd := &cobra.Command{
		Use:     "environment <environment-id>",
		Aliases: []string{"env"},
		Short:   "Show one environment of a Dynatrace Managed cluster (cluster context)",
		Long: `Show an environment's properties, quotas and storage limits from the Cluster
Management API. Needs a cluster context: one without env-id, whose token has
the ServiceProviderAPI scope. Find IDs with "dtmgd get environments".

--consumption adds current usage to the quotas. It is not available on a
cluster licensed with the Dynatrace Platform Subscription (DPS).

Examples:
  dtmgd describe environment 0b5c4e1a-2f3d-4c6b-9a7e-1d2c3b4a5f60 --context prod-cluster
  dtmgd describe environment <id> --consumption -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"includeStorageInfo": "true"}
			if consumption {
				params["includeConsumptionInfo"] = "true"
			}
			fetch := clusterObjectFetch("/cluster/v2/environments/"+url.PathEscape(args[0]), params)
			if consumption {
				fetch = hintOnStatus(fetch, 422, hintDPSConsumption)
			}
			return a.runClusterFetch(fetchSpec{
				Verb:     "describe",
				Resource: "environment",
				Fetch:    fetch,
				NotFound: fmt.Sprintf(hintNoEnvironment, args[0]),
			})
		},
	}
	cmd.Flags().BoolVar(&consumption, "consumption", false, "include current consumption (not on DPS-licensed clusters)")
	return clusterCommand(cmd)
}
