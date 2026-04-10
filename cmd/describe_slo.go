package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

var (
	sloDescFrom      string
	sloDescTo        string
	sloDescTimeframe string
)

var describeSLOCmd = &cobra.Command{
	Use:     "slo <slo-id>",
	Aliases: []string{"slos"},
	Short:   "Show detailed information about a specific SLO",
	Long: `Show SLO details including evaluation, error budget, and burn rate.

Examples:
  dtmgd describe slo <id>
  dtmgd describe slo <id> --timeframe CURRENT
  dtmgd describe slo <id> --from now-2w --to now --timeframe GTF`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		params := map[string]string{}
		if sloDescTimeframe != "" {
			params["timeFrame"] = sloDescTimeframe
		}
		if sloDescFrom != "" {
			params["from"] = sloDescFrom
		}
		if sloDescTo != "" {
			params["to"] = sloDescTo
		}

		path := fmt.Sprintf("/slo/%s", url.PathEscape(args[0]))

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				var result map[string]interface{}
				if err := c.GetV2(path, params, &result); err != nil {
					return nil, err
				}
				return result, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("slo").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		var result map[string]interface{}
		if err := c.GetV2(path, params, &result); err != nil {
			return err
		}

		return NewPrinterForResource("slo").Print(result)
	},
}

func init() {
	describeCmd.AddCommand(describeSLOCmd)

	describeSLOCmd.Flags().StringVar(&sloDescTimeframe, "timeframe", "", "time frame: CURRENT or GTF")
	describeSLOCmd.Flags().StringVar(&sloDescFrom, "from", "", "start time (used with --timeframe GTF)")
	describeSLOCmd.Flags().StringVar(&sloDescTo, "to", "", "end time (used with --timeframe GTF)")
}
