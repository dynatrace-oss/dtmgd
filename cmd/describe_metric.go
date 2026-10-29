package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

func newDescribeMetricCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "metric <metric-id>",
		Aliases: []string{"met"},
		Short:   "Show detailed information about a specific metric descriptor",
		Long: `Show all metadata for a metric including aggregation types, dimensions,
impact maps, and display units.

Example:
  dtmgd describe metric builtin:service.response.time`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/metrics/%s", url.PathEscape(args[0]))

			if a.isMultiEnv() {
				data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
					var result map[string]interface{}
					if err := c.GetV2(path, nil, &result); err != nil {
						return nil, err
					}
					return result, nil
				})
				if err != nil {
					return err
				}
				return a.printer("metric").Print(data)
			}

			c, err := a.newClient(cfg)
			if err != nil {
				return err
			}

			var result map[string]interface{}
			if err := c.GetV2(path, nil, &result); err != nil {
				return err
			}

			return a.printer("metric").Print(result)
		},
	}
	return cmd
}
