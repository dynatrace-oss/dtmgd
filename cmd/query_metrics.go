package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// MetricQueryResponse models /api/v2/metrics/query.
type MetricQueryResponse struct {
	Resolution string              `json:"resolution"`
	Result     []MetricQueryResult `json:"result"`
}

type MetricQueryResult struct {
	MetricID string                  `json:"metricId"`
	Data     []MetricQueryDataPoints `json:"data"`
}

type MetricQueryDataPoints struct {
	Dimensions   []string          `json:"dimensions"`
	DimensionMap map[string]string `json:"dimensionMap"`
	Timestamps   []int64           `json:"timestamps"`
	Values       []*float64        `json:"values"`
}

var (
	qmMetric     string
	qmFrom       string
	qmTo         string
	qmResolution string
	qmEntity     string
)

var queryMetricsCmd = &cobra.Command{
	Use:     "metrics",
	Aliases: []string{"metric", "met"},
	Short:   "Query metric time-series data",
	Long: `Query metric data for a specific metric selector and time range.

Use 'dtmgd get metrics --search <keyword>' to find available metric IDs first.

Examples:
  dtmgd query metrics --metric builtin:service.response.time --from now-1h --to now
  dtmgd query metrics --env ALL_ENVIRONMENTS --metric builtin:host.cpu.usage --from now-1h`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if qmMetric == "" {
			return fmt.Errorf("--metric is required (e.g. builtin:service.response.time)")
		}
		if qmFrom == "" {
			return fmt.Errorf("--from is required")
		}
		if qmTo == "" {
			qmTo = "now"
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		params := map[string]string{
			"metricSelector": qmMetric,
			"from":           qmFrom,
			"to":             qmTo,
		}
		if qmResolution != "" {
			params["resolution"] = qmResolution
		}
		if qmEntity != "" {
			params["entitySelector"] = qmEntity
		}

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				var resp MetricQueryResponse
				if err := c.GetV2("/metrics/query", params, &resp); err != nil {
					return nil, err
				}
				return resp, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("metrics").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		var resp MetricQueryResponse
		if err := c.GetV2("/metrics/query", params, &resp); err != nil {
			return err
		}

		if agentMode() {
			return NewPrinterForResource("metrics").Print(resp)
		}
		if outputFormat == "yaml" {
			return NewPrinter().Print(resp)
		}
		if outputFormat == "table" || outputFormat == "wide" || outputFormat == "" {
			printMetricQuerySummary(resp)
			return nil
		}

		// json
		out, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	},
}

func printMetricQuerySummary(resp MetricQueryResponse) {
	output.PrintInfo("Resolution: %s", resp.Resolution)
	for _, res := range resp.Result {
		output.PrintInfo("\nMetric: %s", res.MetricID)
		for _, dp := range res.Data {
			dims := ""
			if len(dp.DimensionMap) > 0 {
				for k, v := range dp.DimensionMap {
					dims += fmt.Sprintf("%s=%s ", k, v)
				}
			} else if len(dp.Dimensions) > 0 {
				dims = fmt.Sprintf("%v", dp.Dimensions)
			}
			if dims != "" {
				output.PrintInfo("  Dimensions: %s", dims)
			}
			fmt.Printf("  %-26s  %s\n", "TIMESTAMP (UTC)", "VALUE")
			fmt.Printf("  %-26s  %s\n", "-------------------", "-----")
			for i, ts := range dp.Timestamps {
				val := "null"
				if i < len(dp.Values) && dp.Values[i] != nil {
					val = fmt.Sprintf("%.4f", *dp.Values[i])
				}
				fmt.Printf("  %-26s  %s\n", msToTime(ts), val)
			}
		}
	}
}

func init() {
	queryCmd.AddCommand(queryMetricsCmd)

	queryMetricsCmd.Flags().StringVar(&qmMetric, "metric", "", "metric selector (required), e.g. builtin:service.response.time")
	queryMetricsCmd.Flags().StringVar(&qmFrom, "from", "", "start time (required), e.g. now-1h")
	queryMetricsCmd.Flags().StringVar(&qmTo, "to", "", "end time (default: now)")
	queryMetricsCmd.Flags().StringVar(&qmResolution, "resolution", "", "data resolution (e.g. 5m, 1h, 1d)")
	queryMetricsCmd.Flags().StringVar(&qmEntity, "entity", "", "entity selector to filter metric data")
}
