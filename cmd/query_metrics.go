package cmd

import (
	"fmt"
	"io"
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

func newQueryMetricsCmd(a *App) *cobra.Command {
	var opts struct {
		metric     string
		from       string
		to         string
		resolution string
		entity     string
	}
	cmd := &cobra.Command{
		Use:     "metrics",
		Aliases: []string{"metric", "met"},
		Short:   "Query metric time-series data",
		Long: `Query metric data for a specific metric selector and time range.

Use 'dtmgd get metrics --search <keyword>' to find available metric IDs first.

Examples:
  dtmgd query metrics --metric builtin:service.response.time --from now-1h --to now
  dtmgd query metrics --env ALL_ENVIRONMENTS --metric builtin:host.cpu.usage --from now-1h`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.metric == "" {
				return &UsageError{Err: fmt.Errorf("--metric is required (e.g. builtin:service.response.time)")}
			}
			if opts.from == "" {
				return &UsageError{Err: fmt.Errorf("--from is required (e.g. now-1h)")}
			}
			to := opts.to
			if to == "" {
				to = "now"
			}

			params := map[string]string{
				"metricSelector": opts.metric,
				"from":           opts.from,
				"to":             to,
			}
			if opts.resolution != "" {
				params["resolution"] = opts.resolution
			}
			if opts.entity != "" {
				params["entitySelector"] = opts.entity
			}

			return a.runFetch(fetchSpec{
				Verb:     "query",
				Resource: "metrics",
				Fetch: func(c *client.Client, _ int) (fetched, error) {
					var body map[string]interface{}
					if err := c.GetV2("/metrics/query", params, &body); err != nil {
						return fetched{}, err
					}
					if !a.humanOutput() {
						return fetched{Body: body, Total: -1}, nil
					}
					// A table names the entities behind each series. The
					// names come from a second request, so json, yaml and
					// agent output (the API's own object) never pay for it,
					// and a failed lookup leaves the IDs rather than failing.
					// Only Human sees this Body, so it is the typed form.
					var resp MetricQueryResponse
					if err := client.DecodePaged(body, &resp); err != nil {
						return fetched{}, err
					}
					var warnings []string
					if err := resolveMetricEntityNames(c, &resp); err != nil {
						warnings = append(warnings, fmt.Sprintf("Could not look up entity names (%v); showing entity IDs.", err))
					}
					return fetched{Body: resp, Total: -1, Warnings: warnings}, nil
				},
				Human: func(w io.Writer, body interface{}) error {
					return a.printMetricQuerySummary(w, body.(MetricQueryResponse))
				},
			})
		},
	}
	cmd.Flags().StringVar(&opts.metric, "metric", "", "metric selector (required), e.g. builtin:service.response.time")
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (required), e.g. now-1h")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (default: now)")
	cmd.Flags().StringVar(&opts.resolution, "resolution", "", "data resolution (e.g. 5m, 1h, 1d)")
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entity selector to filter metric data")
	return withScopes(cmd, scopeMetricsRead, scopeEntitiesRead)
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

// entityDimensionPrefix starts every entity dimension key ("dt.entity.service").
const entityDimensionPrefix = "dt.entity."

// extractEntityLabel returns (displayName, entityID) from a dimensionMap.
// It pairs "dt.entity.X" (entity ID) with "dt.entity.X.name" (resolved name).
func extractEntityLabel(dimMap map[string]string) (name, id string) {
	for k, v := range dimMap {
		if strings.HasPrefix(k, entityDimensionPrefix) && !strings.HasSuffix(k, ".name") {
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
// An entity the lookup does not return keeps its ID alone; a failed lookup is
// returned, leaving resp unchanged.
func resolveMetricEntityNames(c *client.Client, resp *MetricQueryResponse) error {
	// Collect unique entity IDs and their dimension key prefix.
	seen := map[string]string{} // entityID -> dimKey (e.g. "dt.entity.service")
	for _, res := range resp.Result {
		for _, dp := range res.Data {
			for k, v := range dp.DimensionMap {
				if strings.HasPrefix(k, entityDimensionPrefix) && !strings.HasSuffix(k, ".name") {
					seen[v] = k
				}
			}
		}
	}
	if len(seen) == 0 {
		return nil
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
		return err
	}

	// Build ID → name map.
	nameMap := make(map[string]string, len(entResp.Entities))
	for _, e := range entResp.Entities {
		nameMap[e.EntityID] = e.DisplayName
	}

	injectEntityNames(resp, nameMap)
	return nil
}

// injectEntityNames writes "dt.entity.X.name" keys into every DimensionMap in resp
// using the provided entityID→displayName lookup. This is extracted for testability.
func injectEntityNames(resp *MetricQueryResponse, nameMap map[string]string) {
	for ri := range resp.Result {
		for di := range resp.Result[ri].Data {
			dm := resp.Result[ri].Data[di].DimensionMap
			for k, entityID := range dm {
				if strings.HasPrefix(k, entityDimensionPrefix) && !strings.HasSuffix(k, ".name") {
					if name, ok := nameMap[entityID]; ok {
						dm[k+".name"] = name
					}
				}
			}
		}
	}
}

// printMetricQuerySummary writes a metric query result for a person: a table
// of single values per entity, or each series' timestamps and values.
func (a *App) printMetricQuerySummary(w io.Writer, resp MetricQueryResponse) error {
	_, _ = fmt.Fprintf(w, "Resolution: %s\n", resp.Resolution)
	for _, res := range resp.Result {
		_, _ = fmt.Fprintf(w, "\nMetric: %s\n", res.MetricID)
		if isSingleValueResult(res.Data) {
			if err := a.printEntitySummaryTable(w, res.Data); err != nil {
				return err
			}
			continue
		}
		for _, dp := range res.Data {
			printTimeSeriesDataPoint(w, dp)
		}
	}
	return nil
}

func (a *App) printEntitySummaryTable(w io.Writer, data []MetricQueryDataPoints) error {
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
	tableRows := make([]MetricEntitySummaryRow, 0, len(rows))
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
	return output.NewPrinterToWithColumns(string(a.format()), w, a.columns()).PrintList(tableRows)
}

func printTimeSeriesDataPoint(w io.Writer, dp MetricQueryDataPoints) {
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
		sort.Strings(parts)
		label = strings.Join(parts, " ")
	}
	if label != "" {
		_, _ = fmt.Fprintf(w, "  Entity: %s\n", label)
	}
	_, _ = fmt.Fprintf(w, "  %-26s  %s\n", "TIMESTAMP (UTC)", "VALUE")
	_, _ = fmt.Fprintf(w, "  %-26s  %s\n", "-------------------", "-----")
	for i, ts := range dp.Timestamps {
		val := "null"
		if i < len(dp.Values) && dp.Values[i] != nil {
			val = fmt.Sprintf("%.4f", *dp.Values[i])
		}
		_, _ = fmt.Fprintf(w, "  %-26s  %s\n", msToTime(ts), val)
	}
}
