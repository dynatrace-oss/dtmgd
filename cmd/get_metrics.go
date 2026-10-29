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

func newGetMetricsCmd(a *App) *cobra.Command {
	var opts struct {
		search string
		entity string
		limit  int
	}
	cmd := &cobra.Command{
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
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}

			params := map[string]string{}
			pageSize := 500
			if opts.limit > 0 {
				pageSize = opts.limit
			}
			params["pageSize"] = fmt.Sprintf("%d", pageSize)
			if opts.search != "" {
				params["text"] = opts.search
			}
			if opts.entity != "" {
				params["entitySelector"] = opts.entity
			}

			if a.isMultiEnv() {
				data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
					raw, err := c.GetV2Paged("/metrics", params, a.maxPagesFor(opts.limit > 0))
					if err != nil {
						return nil, err
					}
					return raw, nil
				})
				if err != nil {
					return err
				}
				return a.printer("metrics").Print(data)
			}

			c, err := a.newClient(cfg)
			if err != nil {
				return err
			}

			raw, err := c.GetV2Paged("/metrics", params, a.maxPagesFor(opts.limit > 0))
			if err != nil {
				return err
			}

			var resp MetricsDescriptorResponse
			if err := client.DecodePaged(raw, &resp); err != nil {
				return err
			}

			if a.Opts.Output == "json" || a.Opts.Output == "yaml" || a.agentMode() {
				return a.printer("metrics").Print(resp)
			}

			metrics := resp.Metrics
			if opts.limit > 0 && len(metrics) > opts.limit {
				metrics = metrics[:opts.limit]
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
			return a.printer("").PrintList(items)
		},
	}
	cmd.Flags().StringVar(&opts.search, "search", "", "text search in metric names/descriptions")
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entity selector to filter metrics")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of metrics")
	return cmd
}
