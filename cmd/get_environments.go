package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// EnvListItem is the table row for environments.
type EnvListItem struct {
	Name    string `table:"NAME"`
	Host    string `table:"HOST"`
	EnvID   string `table:"ENV-ID"`
	Version string `table:"VERSION"`
	Status  string `table:"STATUS"`
}

var getEnvironmentsCmd = &cobra.Command{
	Use:     "environments",
	Aliases: []string{"envs", "env"},
	Short:   "List configured Dynatrace Managed environments",
	Long: `List all contexts and verify connectivity to each Dynatrace Managed cluster.

Fetches the cluster version to confirm that the API token and network path are working.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		var items []EnvListItem

		for _, nc := range cfg.Contexts {
			item := EnvListItem{
				Name:  nc.Name,
				Host:  nc.Context.Host,
				EnvID: nc.Context.EnvID,
			}

			token, tokenErr := cfg.GetToken(nc.Context.TokenRef)
			if tokenErr != nil {
				item.Version = "—"
				item.Status = fmt.Sprintf("no token: %v", tokenErr)
				items = append(items, item)
				continue
			}

			c, clientErr := NewClientWithHostEnv(nc.Context.Host, nc.Context.EnvID, token)
			if clientErr != nil {
				item.Version = "—"
				item.Status = fmt.Sprintf("config error: %v", clientErr)
				items = append(items, item)
				continue
			}

			version, vErr := c.ClusterVersion()
			if vErr != nil {
				item.Version = "—"
				item.Status = fmt.Sprintf("error: %v", vErr)
			} else {
				item.Version = version
				item.Status = "OK"
			}
			items = append(items, item)
		}

		if len(items) == 0 {
			output.PrintInfo("No contexts configured. Run 'dtmgd config set-context' to add one.")
			return nil
		}

		return NewPrinterForResource("environments").PrintList(items)
	},
}

func init() {
	getCmd.AddCommand(getEnvironmentsCmd)
}
