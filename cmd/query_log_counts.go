package cmd

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// LogAggregateResponse models /api/v2/logs/aggregate.
// aggregationResult[groupByField][timeBucket][value] = count
type LogAggregateResponse struct {
	AggregationResult map[string]map[string]map[string]int64 `json:"aggregationResult"`
	Warnings          string                                  `json:"warnings"`
}

// LogCountRow is a table row for log level counts per service.
type LogCountRow struct {
	Service string `table:"SERVICE"`
	Info    int64  `table:"INFO"`
	Warn    int64  `table:"WARN"`
	Error   int64  `table:"ERROR"`
	Total   int64  `table:"TOTAL"`
}

var (
	lcEntity         string
	lcFrom           string
	lcTo             string
	lcMaxGroupValues int
)

var queryLogCountsCmd = &cobra.Command{
	Use:     "log-counts",
	Aliases: []string{"log-count", "logs-count"},
	Short:   "Aggregate log counts by service and level (INFO/WARN/ERROR)",
	Long: `Aggregate log records from /api/v2/logs/aggregate, grouped by service and log level.

Returns INFO, WARN, and ERROR counts per service using entitySelector to scope results.
Log levels are detected via full-text matching in log content (Spring Boot log format).

NOTE: On Dynatrace Managed Classic, structured field queries (e.g. loglevel:ERROR) are
not supported. Log level counts are approximated by searching for "INFO", "WARN", "ERROR"
as full-text terms in log content. This is accurate for standard Spring Boot / Java logs
where the level appears in each log line.

Examples:
  dtmgd query log-counts --entity 'type(SERVICE),tag("[Environment]BookStore")' --from now-1h
  dtmgd query log-counts --entity 'type(SERVICE),tag("[Environment]BookStore")' --from now-30m --to now`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if lcEntity == "" {
			return fmt.Errorf("--entity is required (e.g. 'type(SERVICE),tag(\"[Environment]BookStore\")')")
		}
		if lcFrom == "" {
			lcFrom = "now-1h"
		}
		if lcTo == "" {
			lcTo = "now"
		}
		if lcMaxGroupValues <= 0 || lcMaxGroupValues > 100 {
			lcMaxGroupValues = 100
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		// Fetch entity display names for the given selector.
		entityNames := map[string]string{}
		var entResp EntitiesResponse
		entParams := map[string]string{
			"entitySelector": lcEntity,
			"fields":         "entityId,displayName",
			"pageSize":       "500",
		}
		if err := c.GetV2("/entities", entParams, &entResp); err == nil {
			for _, e := range entResp.Entities {
				entityNames[e.EntityID] = e.DisplayName
			}
		}

		// For each log level, call aggregate with groupBy=dt.entity.service.
		// On DT Managed Classic, full-text matching ("INFO", "WARN", "ERROR") is used
		// because structured field queries are not supported by the LQL engine.
		levelQueries := []struct{ level, query string }{
			{"INFO", "INFO"},
			{"WARN", "WARN"},
			{"ERROR", "ERROR"},
		}

		// serviceCounts[serviceID][level] = count
		serviceCounts := map[string]map[string]int64{}

		for _, lq := range levelQueries {
			params := url.Values{
				"entitySelector":  {lcEntity},
				"from":            {lcFrom},
				"to":              {lcTo},
				"timeBuckets":     {"1"},
				"maxGroupValues":  {fmt.Sprintf("%d", lcMaxGroupValues)},
				"groupBy":         {"dt.entity.service"},
				"query":           {lq.query},
			}
			var resp LogAggregateResponse
			if err := c.GetV2WithValues("/logs/aggregate", params, &resp); err != nil {
				return fmt.Errorf("aggregate %s: %w", lq.level, err)
			}

			// aggregationResult["dt.entity.service"][bucket][SERVICE-xxx] = count
			if svcBuckets, ok := resp.AggregationResult["dt.entity.service"]; ok {
				for _, bucketMap := range svcBuckets {
					for svcID, cnt := range bucketMap {
						if _, exists := serviceCounts[svcID]; !exists {
							serviceCounts[svcID] = map[string]int64{}
						}
						serviceCounts[svcID][lq.level] += cnt
					}
				}
			}
		}

		if len(serviceCounts) == 0 {
			output.PrintInfo("No log records found for the specified entity selector and time range.")
			return nil
		}

		// Build rows.
		rows := make([]LogCountRow, 0, len(serviceCounts))
		for svcID, levels := range serviceCounts {
			info := levels["INFO"]
			warn := levels["WARN"]
			errCount := levels["ERROR"]
			total := info + warn + errCount

			name := entityNames[svcID]
			if name == "" {
				name = svcID
			} else {
				// Strip common suffix noise like ".bookstore.svc"
				name = cleanServiceName(name)
			}

			rows = append(rows, LogCountRow{
				Service: name,
				Info:    info,
				Warn:    warn,
				Error:   errCount,
				Total:   total,
			})
		}

		// Sort by ERROR desc, then WARN desc, then Total desc.
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Error != rows[j].Error {
				return rows[i].Error > rows[j].Error
			}
			if rows[i].Warn != rows[j].Warn {
				return rows[i].Warn > rows[j].Warn
			}
			if rows[i].Total != rows[j].Total {
				return rows[i].Total > rows[j].Total
			}
			return rows[i].Service < rows[j].Service
		})

		if outputFormat == "json" || agentMode() {
			return NewPrinterForResource("log-counts").Print(rows)
		}

		// Print table.
		fmt.Printf("Log counts for entity selector: %s\n", lcEntity)
		fmt.Printf("Time range: %s → %s\n\n", lcFrom, lcTo)
		return NewPrinter().Print(rows)
	},
}

// cleanServiceName strips common Kubernetes service name suffixes for brevity.
func cleanServiceName(name string) string {
	suffixes := []string{".bookstore.svc.cluster.local", ".svc.cluster.local", ".bookstore"}
	for _, s := range suffixes {
		if strings.HasSuffix(name, s) {
			return strings.TrimSuffix(name, s)
		}
	}
	return name
}

func init() {
	queryCmd.AddCommand(queryLogCountsCmd)

	queryLogCountsCmd.Flags().StringVar(&lcEntity, "entity", "", "entitySelector to scope log aggregation (required)")
	queryLogCountsCmd.Flags().StringVar(&lcFrom, "from", "", "start time (default: now-1h), e.g. now-2h")
	queryLogCountsCmd.Flags().StringVar(&lcTo, "to", "", "end time (default: now)")
	queryLogCountsCmd.Flags().IntVar(&lcMaxGroupValues, "max-services", 100, "maximum number of services to return per level (max: 100)")
}
