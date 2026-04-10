package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

var describeProblemCmd = &cobra.Command{
	Use:     "problem <problem-id>",
	Aliases: []string{"prob"},
	Short:   "Show detailed information about a specific problem",
	Long: `Show full problem details including evidence, affected entities, and root cause.
Use the problemId (UUID format) from 'dtmgd get problems', not the display ID (P-XXXXX).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/problems/%s", url.PathEscape(args[0]))

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				var result map[string]interface{}
				if err := c.GetV2(path, nil, &result); err != nil {
					return nil, err
				}
				return result, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("problem").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		var result map[string]interface{}
		if err := c.GetV2(path, nil, &result); err != nil {
			return err
		}

		return NewPrinterForResource("problem").Print(result)
	},
}

func init() {
	describeCmd.AddCommand(describeProblemCmd)
	describeProblemCmd.Flags().SetInterspersed(false)
}
