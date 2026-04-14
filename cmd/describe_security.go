package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

var describeSecurityProblemCmd = &cobra.Command{
	Use:     "security-problem <security-problem-id>",
	Aliases: []string{"sp", "vuln"},
	Short:   "Show detailed information about a security vulnerability",
	Long: `Show full CVE details, risk assessment, affected entities, and remediation hints
for a specific security problem.

Use the security problem ID (UUID) from 'dtmgd get security-problems', not the
display ID (S-XXXXX).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/securityProblems/%s", url.PathEscape(args[0]))
		params := map[string]string{
			"fields": "+riskAssessment,+managementZones,+codeLevelVulnerabilityDetails,+vulnerableComponents,+affectedEntities,+exposedEntities,+description",
		}

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
			return NewPrinterForResource("securityProblem").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		var result map[string]interface{}
		if err := c.GetV2(path, params, &result); err != nil {
			return err
		}

		return NewPrinterForResource("securityProblem").Print(result)
	},
}

func init() {
	describeCmd.AddCommand(describeSecurityProblemCmd)
}
