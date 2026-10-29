package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// licenseUsageKey is the list key of the per-environment license usage.
const licenseUsageKey = "totalLicenseUsageDataPerEnvironment"

// licenseUsageRow is a get license-usage table row.
type licenseUsageRow struct {
	Environment string `table:"ENVIRONMENT"`
	Name        string `table:"NAME"`
	HostUnits   string `table:"HOST-UNITS"`
	DEMUnits    string `table:"DEM-UNITS"`
	DDUs        string `table:"DDUS"`
}

func newGetLicenseUsageCmd(a *App) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "license-usage",
		Args:  noArgs,
		Short: "List each environment's total billed license usage (cluster context)",
		Long: `List each environment's total billed usage of host units, DEM units and Davis
data units, including usage from past contracts, from the Cluster Management
API. Needs a cluster context: one without env-id, whose token has the
ServiceProviderAPI scope. The table names each environment from the
cluster's environment list.

The API needs Dynatrace Managed 1.326+ and works only with Dynatrace classic
licensing; a cluster on the Dynatrace Platform Subscription (DPS) has no
classic license usage. "dtmgd describe license" shows the cluster's totals.

Examples:
  dtmgd get license-usage --context prod-cluster
  dtmgd get license-usage -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			names := map[string]string{}
			return a.runClusterFetch(fetchSpec{
				Verb:     "get",
				Resource: "license-usage",
				Fetch:    licenseFetch(a.licenseUsageFetch(names)),
				Rows:     licenseUsageRows(names),
				Limit:    limit,
				List:     true,
				Empty:    "No license usage reported.",
				NotFound: hintClassicLicensing,
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of environments to return")
	return clusterCommand(cmd)
}

// licenseUsageFetch is get license-usage's Fetch. For a person it also fills
// names with each environment's name by ID, from the environment list; a
// failure there only leaves the names out, with a warning.
func (a *App) licenseUsageFetch(names map[string]string) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		var body map[string]interface{}
		if err := c.GetCluster(licenseUsagePath, nil, &body); err != nil {
			return fetched{}, err
		}
		if body == nil {
			body = map[string]interface{}{}
		}
		f := listFetched(body, licenseUsageKey)
		if list := body[licenseUsageKey].([]interface{}); limit > 0 && len(list) > limit {
			body[licenseUsageKey] = list[:limit]
			f.HasMore, f.Count = true, limit
		}
		if a.humanOutput() {
			if err := environmentNames(c, names); err != nil {
				f.Warnings = append(f.Warnings, "environment names unavailable: "+err.Error())
			}
		}
		return f, nil
	}
}

// environmentNames fills names with the cluster's environment names by ID.
func environmentNames(c *client.Client, names map[string]string) error {
	body, err := c.GetClusterPaged("/cluster/v2/environments", map[string]string{"pageSize": "1000"}, client.Paging{ListKey: "environments"})
	if err != nil {
		return err
	}
	var list struct {
		Environments []clusterEnvironment `json:"environments"`
	}
	if err := client.DecodePaged(body, &list); err != nil {
		return err
	}
	for _, e := range list.Environments {
		names[e.ID] = e.Name
	}
	return nil
}

// licenseUsageRows is get license-usage's Rows, naming each environment from
// names ("—" when unknown).
func licenseUsageRows(names map[string]string) func(body interface{}) (interface{}, int, error) {
	return func(body interface{}) (interface{}, int, error) {
		var list struct {
			Usage []struct {
				EnvironmentUUID string   `json:"environmentUuid"`
				HostUnits       *float64 `json:"usageOfHostUnits"`
				DEMUnits        *float64 `json:"usageOfDemUnits"`
				DDUs            *float64 `json:"usageOfDduUnits"`
			} `json:"totalLicenseUsageDataPerEnvironment"`
		}
		if err := client.DecodePaged(body.(map[string]interface{}), &list); err != nil {
			return nil, 0, err
		}
		rows := make([]licenseUsageRow, len(list.Usage))
		for i, u := range list.Usage {
			name := names[u.EnvironmentUUID]
			if name == "" {
				name = "—"
			}
			rows[i] = licenseUsageRow{Environment: u.EnvironmentUUID, Name: name,
				HostUnits: optNumber(u.HostUnits), DEMUnits: optNumber(u.DEMUnits), DDUs: optNumber(u.DDUs)}
		}
		return rows, len(rows), nil
	}
}

// optNumber is formatNumber(*v), or "—" for nil.
func optNumber(v *float64) string {
	if v == nil {
		return "—"
	}
	return formatNumber(*v)
}
