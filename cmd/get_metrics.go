package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// MetricListItem is the table row for metric descriptors.
type MetricListItem struct {
	MetricID    string `table:"METRIC-ID"`
	DisplayName string `table:"DISPLAY-NAME"`
	Unit        string `table:"UNIT"`
	Description string `table:"DESCRIPTION,wide"`
}

// MetricsDescriptorResponse models /api/v2/metrics.
type MetricsDescriptorResponse struct {
	Metrics    []MetricDescriptor `json:"metrics"`
	TotalCount int                `json:"totalCount"`
}

type MetricDescriptor struct {
	MetricID         string   `json:"metricId"`
	DisplayName      string   `json:"displayName"`
	Unit             string   `json:"unit"`
	Description      string   `json:"description"`
	AggregationTypes []string `json:"aggregationTypes"`
}

var (
	metSearch string
	metEntity string
	metLimit  int
)

var getMetricsCmd = &cobra.Command{
	Use:     "metrics",
	Aliases: []string{"met", "metric"},
	Short:   "List available metric descriptors",
	Long: `List available metrics in the Managed environment, optionally filtered by text
or entity selector.

Examples:
  dtmgd get metrics --search response.time
  dtmgd get metrics --search cpu --entity 'type(HOST)'
  dtmgd get metrics --env ALL_ENVIRONMENTS --search cpu`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		params := map[string]string{}
		pageSize := 500
		if metLimit > 0 {
			pageSize = metLimit
		}
		params["pageSize"] = fmt.Sprintf("%d", pageSize)
		if metSearch != "" {
			params["text"] = metSearch
		}
		if metEntity != "" {
			params["entitySelector"] = metEntity
		}

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				raw, err := c.GetV2Paged("/metrics", params, effectiveMaxPages(metLimit > 0))
				if err != nil {
					return nil, err
				}
				return raw, nil
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

		raw, err := c.GetV2Paged("/metrics", params, effectiveMaxPages(metLimit > 0))
		if err != nil {
			return err
		}

		var resp MetricsDescriptorResponse
		if err := client.DecodePaged(raw, &resp); err != nil {
			return err
		}

		if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
			return NewPrinterForResource("metrics").Print(resp)
		}

		metrics := resp.Metrics
		if metLimit > 0 && len(metrics) > metLimit {
			metrics = metrics[:metLimit]
		}

		if len(metrics) == 0 {
			output.PrintInfo("No metrics found.")
			return nil
		}

		var items []MetricListItem
		for _, m := range metrics {
			items = append(items, MetricListItem{
				MetricID:    m.MetricID,
				DisplayName: m.DisplayName,
				Unit:        m.Unit,
				Description: truncate(m.Description, 80),
			})
		}
		if resp.TotalCount > len(metrics) {
			output.PrintInfo("Showing %d of %d metrics. Use --limit to request more.", len(metrics), resp.TotalCount)
		}
		return NewPrinter().PrintList(items)
	},
}

func init() {
	getCmd.AddCommand(getMetricsCmd)

	getMetricsCmd.Flags().StringVar(&metSearch, "search", "", "text search in metric names/descriptions")
	getMetricsCmd.Flags().StringVar(&metEntity, "entity", "", "entity selector to filter metrics")
	getMetricsCmd.Flags().IntVar(&metLimit, "limit", 0, "maximum number of metrics")
}
