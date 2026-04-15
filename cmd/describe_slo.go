package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// SLODetail maps the /slo/{id} API response for human-readable describe output.
type SLODetail struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Status               string   `json:"status"`
	Enabled              bool     `json:"enabled"`
	Target               float64  `json:"target"`
	Warning              *float64 `json:"warning"`
	EvaluatedPct         float64  `json:"evaluatedPercentage"`
	ErrorBudget          float64  `json:"errorBudget"`
	Timeframe            string   `json:"timeframe"`
	EvaluationType       string   `json:"evaluationType"`
	Filter               string   `json:"filter"`
	MetricExpression     string   `json:"metricExpression"`
	RelatedOpenProblems  int      `json:"relatedOpenProblems"`
	RelatedTotalProblems int      `json:"relatedTotalProblems"`
	ErrorBudgetBurnRate  *struct {
		BurnRateValue float64 `json:"burnRateValue"`
		BurnRateType  string  `json:"burnRateType"`
	} `json:"errorBudgetBurnRate"`
}

func printSLODetail(d SLODetail) {
	enabled := "no"
	if d.Enabled {
		enabled = "yes"
	}
	warn := "—"
	if d.Warning != nil {
		warn = fmt.Sprintf("%.2f%%", *d.Warning)
	}
	burnRate := "—"
	if d.ErrorBudgetBurnRate != nil {
		burnRate = fmt.Sprintf("%.2f (%s)", d.ErrorBudgetBurnRate.BurnRateValue, d.ErrorBudgetBurnRate.BurnRateType)
	}
	fmt.Printf("ID:                   %s\n", d.ID)
	fmt.Printf("Name:                 %s\n", d.Name)
	fmt.Printf("Status:               %s\n", d.Status)
	fmt.Printf("Enabled:              %s\n", enabled)
	fmt.Printf("Target:               %.2f%%\n", d.Target)
	fmt.Printf("Warning:              %s\n", warn)
	fmt.Printf("Evaluated:            %.4f%%\n", d.EvaluatedPct)
	fmt.Printf("Error Budget:         %.4f\n", d.ErrorBudget)
	fmt.Printf("Burn Rate:            %s\n", burnRate)
	fmt.Printf("Timeframe:            %s\n", d.Timeframe)
	fmt.Printf("Evaluation Type:      %s\n", d.EvaluationType)
	if d.Filter != "" {
		fmt.Printf("Filter:               %s\n", d.Filter)
	}
	if d.MetricExpression != "" {
		fmt.Printf("Metric Expression:    %s\n", d.MetricExpression)
	}
	if d.RelatedOpenProblems > 0 || d.RelatedTotalProblems > 0 {
		fmt.Printf("Related Problems:     %d open / %d total\n", d.RelatedOpenProblems, d.RelatedTotalProblems)
	}
}

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

		if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
			return NewPrinterForResource("slo").Print(result)
		}

		// Table mode: unmarshal into typed struct for human-readable output.
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		var detail SLODetail
		if err := json.Unmarshal(b, &detail); err != nil {
			return err
		}
		printSLODetail(detail)
		return nil
	},
}

func init() {
	describeCmd.AddCommand(describeSLOCmd)

	describeSLOCmd.Flags().StringVar(&sloDescTimeframe, "timeframe", "", "time frame: CURRENT or GTF")
	describeSLOCmd.Flags().StringVar(&sloDescFrom, "from", "", "start time (used with --timeframe GTF)")
	describeSLOCmd.Flags().StringVar(&sloDescTo, "to", "", "end time (used with --timeframe GTF)")
}
