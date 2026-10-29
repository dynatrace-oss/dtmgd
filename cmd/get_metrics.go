package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
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
		Args:    noArgs,
		Aliases: []string{"met", "metric"},
		Short:   "List available metric descriptors",
		Long: `List available metrics in the Managed environment, optionally filtered by text
or entity selector.

Examples:
  dtmgd get metrics --search response.time
  dtmgd get metrics --search cpu --entity 'type(HOST)'
  dtmgd get metrics --env ALL_ENVIRONMENTS --search cpu`,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{}
			if opts.search != "" {
				params["text"] = opts.search
			}
			if opts.entity != "" {
				params["entitySelector"] = opts.entity
			}

			return a.runFetch(fetchSpec{
				Verb:     "get",
				Resource: "metrics",
				Fetch:    pagedFetch("/metrics", params, "metrics", 500, a.Opts.MaxPages),
				Rows:     metricRows,
				Link:     uiPage(uiMetrics),
				Limit:    opts.limit,
				List:     true,
				Empty:    "No metrics found.",
			})
		},
	}
	cmd.Flags().StringVar(&opts.search, "search", "", "text search in metric names/descriptions")
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entity selector to filter metrics")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of metrics")
	return withScopes(cmd, scopeMetricsRead)
}

// metricRows turns a /metrics body into table rows.
func metricRows(body interface{}) (interface{}, int, error) {
	var resp MetricsDescriptorResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return nil, 0, err
	}
	items := make([]MetricListItem, 0, len(resp.Metrics))
	for _, m := range resp.Metrics {
		items = append(items, MetricListItem{
			MetricID:    m.MetricID,
			DisplayName: m.DisplayName,
			Unit:        m.Unit,
			Description: truncate(m.Description, 80),
		})
	}
	return items, len(items), nil
}
