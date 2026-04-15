package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
			resolveMetricEntityNames(c, &resp)
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

// MetricEntitySummaryRow is a table row for per-entity single-value metric results.
type MetricEntitySummaryRow struct {
	Entity string `table:"ENTITY"`
	ID     string `table:"ENTITY-ID"`
	Value  string `table:"VALUE"`
}

// isSingleValueResult returns true when every data series has exactly 1 timestamp slot.
// This is always true for resolution=Inf.
func isSingleValueResult(data []MetricQueryDataPoints) bool {
	if len(data) == 0 {
		return false
	}
	for _, dp := range data {
		if len(dp.Timestamps) != 1 {
			return false
		}
	}
	return true
}

// extractEntityLabel returns (displayName, entityID) from a dimensionMap.
// It pairs "dt.entity.X" (entity ID) with "dt.entity.X.name" (resolved name).
func extractEntityLabel(dimMap map[string]string) (name, id string) {
	for k, v := range dimMap {
		if strings.HasPrefix(k, "dt.entity.") && !strings.HasSuffix(k, ".name") {
			id = v
			name = dimMap[k+".name"]
			return
		}
	}
	// Fallback: use first value as id
	for _, v := range dimMap {
		id = v
		return
	}
	return
}

// resolveMetricEntityNames enriches every DimensionMap in resp with
// "dt.entity.X.name" entries by fetching display names from /api/v2/entities.
// Missing or failed lookups are silently ignored so output is always printed.
func resolveMetricEntityNames(c *client.Client, resp *MetricQueryResponse) {
	// Collect unique entity IDs and their dimension key prefix.
	seen := map[string]string{} // entityID -> dimKey (e.g. "dt.entity.service")
	for _, res := range resp.Result {
		for _, dp := range res.Data {
			for k, v := range dp.DimensionMap {
				if strings.HasPrefix(k, "dt.entity.") && !strings.HasSuffix(k, ".name") {
					seen[v] = k
				}
			}
		}
	}
	if len(seen) == 0 {
		return
	}

	// Build entitySelector: entityId("id1","id2",...)
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, fmt.Sprintf("%q", id))
	}
	selector := "entityId(" + strings.Join(ids, ",") + ")"

	var entResp struct {
		Entities []struct {
			EntityID    string `json:"entityId"`
			DisplayName string `json:"displayName"`
		} `json:"entities"`
	}
	params := map[string]string{
		"entitySelector": selector,
		"pageSize":       "500",
	}
	if err := c.GetV2("/entities", params, &entResp); err != nil {
		return // silently ignore; entity IDs will still be shown
	}

	// Build ID → name map.
	nameMap := make(map[string]string, len(entResp.Entities))
	for _, e := range entResp.Entities {
		nameMap[e.EntityID] = e.DisplayName
	}

	injectEntityNames(resp, nameMap)
}

// injectEntityNames writes "dt.entity.X.name" keys into every DimensionMap in resp
// using the provided entityID→displayName lookup. This is extracted for testability.
func injectEntityNames(resp *MetricQueryResponse, nameMap map[string]string) {
	for ri := range resp.Result {
		for di := range resp.Result[ri].Data {
			dm := resp.Result[ri].Data[di].DimensionMap
			for k, entityID := range dm {
				if strings.HasPrefix(k, "dt.entity.") && !strings.HasSuffix(k, ".name") {
					if name, ok := nameMap[entityID]; ok {
						dm[k+".name"] = name
					}
				}
			}
		}
	}
}

func printMetricQuerySummary(resp MetricQueryResponse) {
	output.PrintInfo("Resolution: %s", resp.Resolution)
	for _, res := range resp.Result {
		output.PrintInfo("\nMetric: %s", res.MetricID)
		if isSingleValueResult(res.Data) {
			printEntitySummaryTable(res.Data)
		} else {
			for _, dp := range res.Data {
				printTimeSeriesDataPoint(dp)
			}
		}
	}
}

func printEntitySummaryTable(data []MetricQueryDataPoints) {
	type row struct {
		name  string
		id    string
		value *float64
	}
	var rows []row
	for _, dp := range data {
		name, id := extractEntityLabel(dp.DimensionMap)
		var val *float64
		if len(dp.Values) == 1 {
			val = dp.Values[0]
		}
		rows = append(rows, row{name: name, id: id, value: val})
	}
	// Sort: value desc (nulls last), then name asc, then id asc for determinism
	sort.Slice(rows, func(i, j int) bool {
		vi, vj := rows[i].value, rows[j].value
		if vi == nil && vj != nil {
			return false
		}
		if vi != nil && vj == nil {
			return true
		}
		if vi != nil && vj != nil && *vi != *vj {
			return *vi > *vj
		}
		if rows[i].name != rows[j].name {
			return rows[i].name < rows[j].name
		}
		return rows[i].id < rows[j].id
	})
	var tableRows []MetricEntitySummaryRow
	for _, r := range rows {
		val := "null"
		if r.value != nil {
			val = fmt.Sprintf("%.2f", *r.value)
		}
		tableRows = append(tableRows, MetricEntitySummaryRow{
			Entity: r.name,
			ID:     r.id,
			Value:  val,
		})
	}
	_ = NewPrinter().PrintList(tableRows)
}

func printTimeSeriesDataPoint(dp MetricQueryDataPoints) {
	name, id := extractEntityLabel(dp.DimensionMap)
	label := ""
	if name != "" {
		label = fmt.Sprintf("%s (%s)", name, id)
	} else if id != "" {
		label = id
	} else if len(dp.Dimensions) > 0 {
		label = fmt.Sprintf("%v", dp.Dimensions)
	} else {
		var parts []string
		for k, v := range dp.DimensionMap {
			if !strings.HasSuffix(k, ".name") {
				parts = append(parts, k+"="+v)
			}
		}
		label = strings.Join(parts, " ")
	}
	if label != "" {
		output.PrintInfo("  Entity: %s", label)
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

func init() {
	queryCmd.AddCommand(queryMetricsCmd)

	queryMetricsCmd.Flags().StringVar(&qmMetric, "metric", "", "metric selector (required), e.g. builtin:service.response.time")
	queryMetricsCmd.Flags().StringVar(&qmFrom, "from", "", "start time (required), e.g. now-1h")
	queryMetricsCmd.Flags().StringVar(&qmTo, "to", "", "end time (default: now)")
	queryMetricsCmd.Flags().StringVar(&qmResolution, "resolution", "", "data resolution (e.g. 5m, 1h, 1d)")
	queryMetricsCmd.Flags().StringVar(&qmEntity, "entity", "", "entity selector to filter metric data")
}
