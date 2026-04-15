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
	Warnings          string                                 `json:"warnings"`
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
	Long: `Aggregate log records from /api/v2/logs/aggregate, grouped by process group and log level.

Returns INFO, WARN, and ERROR counts per service using an entitySelector to discover the
relevant process groups, then grouping by dt.entity.process_group in the aggregate call.

On Dynatrace Managed Classic, logs are attributed to PROCESS_GROUP entities (not SERVICE
entities). If the given --entity selector targets SERVICE entities (type(SERVICE),...), the
command automatically derives an equivalent PROCESS_GROUP selector by substituting the type.

Log levels are detected via full-text matching in log content (Spring Boot log format).

NOTE: On Dynatrace Managed Classic, structured field queries (e.g. loglevel:ERROR) are
not supported. Log level counts are approximated by searching for "INFO", "WARN", "ERROR"
as full-text terms in log content. This is accurate for standard Spring Boot / Java logs
where the level appears in each log line.

Examples:
  dtmgd query log-counts --entity 'type(SERVICE),tag("[Environment]BookStore")' --from now-1h
  dtmgd query log-counts --entity 'type(PROCESS_GROUP),tag("[Environment]BookStore")' --from now-30m --to now`,
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

		// On DT Managed Classic, logs are attributed to PROCESS_GROUP entities, not SERVICE.
		// If the user passed a SERVICE selector, auto-convert to PROCESS_GROUP.
		pgSelector := strings.Replace(lcEntity, "type(SERVICE)", "type(PROCESS_GROUP)", 1)

		// Fetch process group entity IDs and display names.
		entityNames := map[string]string{} // pgID → display name
		var entResp EntitiesResponse
		entParams := map[string]string{
			"entitySelector": pgSelector,
			"pageSize":       "500",
		}
		if err := c.GetV2("/entities", entParams, &entResp); err == nil {
			for _, e := range entResp.Entities {
				// Store with lowercase key: entities API returns "PROCESS_GROUP-UPPERCASE"
				// but logs aggregate returns "process_group-lowercase" for the same entity.
				entityNames[strings.ToLower(e.EntityID)] = cleanPGName(e.DisplayName)
			}
		}

		if len(entityNames) == 0 {
			output.PrintInfo("No entities found for the specified entity selector.")
			return nil
		}

		// For each log level, call aggregate with groupBy=dt.entity.process_group.
		// entitySelector is intentionally omitted: on DT Managed Classic it is a hidden
		// parameter that does not actually filter the aggregate result (returns empty).
		// Instead, we fetch all process groups that match, then filter client-side.
		//
		// Log levels use full-text matching ("INFO", "WARN", "ERROR") because
		// structured field queries (loglevel:ERROR) are not supported on DT Managed Classic.
		levelQueries := []struct{ level, query string }{
			{"INFO", "INFO"},
			{"WARN", "WARN"},
			{"ERROR", "ERROR"},
		}

		// pgCounts[pgID][level] = count
		pgCounts := map[string]map[string]int64{}

		for _, lq := range levelQueries {
			params := url.Values{
				"from":           {lcFrom},
				"to":             {lcTo},
				"timeBuckets":    {"1"},
				"maxGroupValues": {fmt.Sprintf("%d", lcMaxGroupValues)},
				"groupBy":        {"dt.entity.process_group"},
				"query":          {lq.query},
			}
			var resp LogAggregateResponse
			if err := c.GetV2WithValues("/logs/aggregate", params, &resp); err != nil {
				return fmt.Errorf("aggregate %s: %w", lq.level, err)
			}

			// aggregationResult["dt.entity.process_group"][bucket][PROCESS_GROUP-xxx] = count
			if pgBuckets, ok := resp.AggregationResult["dt.entity.process_group"]; ok {
				for _, bucketMap := range pgBuckets {
					for pgID, cnt := range bucketMap {
						// Client-side filter: only include process groups from our entity lookup.
						if _, known := entityNames[pgID]; !known {
							continue
						}
						if _, exists := pgCounts[pgID]; !exists {
							pgCounts[pgID] = map[string]int64{}
						}
						pgCounts[pgID][lq.level] += cnt
					}
				}
			}
		}

		// Build rows: include zero rows for all known entities so that services
		// with no log ingestion are visible (rather than silently omitted).
		rows := make([]LogCountRow, 0, len(entityNames))
		for pgID, name := range entityNames {
			levels := pgCounts[pgID] // nil if no logs found for this PG
			info := levels["INFO"]
			warn := levels["WARN"]
			errCount := levels["ERROR"]
			total := info + warn + errCount

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

// cleanPGName extracts a short service name from a Dynatrace process group display name.
// Process group display names on DT Managed have the form:
//
//	"SpringBoot BookStore-Orders com.dynatrace.orders.OrdersApplication orders-*"
//
// This function extracts the short name after "BookStore-" (lowercased),
// or falls back to the raw display name if the pattern is not found.
func cleanPGName(name string) string {
	const marker = "BookStore-"
	if idx := strings.Index(name, marker); idx >= 0 {
		rest := name[idx+len(marker):]
		if spIdx := strings.Index(rest, " "); spIdx >= 0 {
			return strings.ToLower(rest[:spIdx])
		}
		return strings.ToLower(rest)
	}
	// Fallback: strip common k8s service suffixes.
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
