package cmd

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// LogAggregateResponse models /api/v2/logs/aggregate.
// aggregationResult[groupByField][timeBucket][value] = count
type LogAggregateResponse struct {
	AggregationResult map[string]map[string]map[string]int64 `json:"aggregationResult"`
	Warnings          string                                 `json:"warnings"`
}

// LogCounts is the result of query log-counts, as json, yaml and agent
// output print it.
type LogCounts struct {
	LogCounts []LogCount `json:"logCounts" yaml:"logCounts"`
}

// LogCount is the number of log records at each level for one process group
// name. A nil count is unknown: the group fell outside the aggregation's
// top-N window for that level (see aggregateLevel). Total is nil when any
// level is unknown.
type LogCount struct {
	Service string `json:"service" yaml:"service"`
	Info    *int64 `json:"info" yaml:"info"`
	Warn    *int64 `json:"warn" yaml:"warn"`
	Error   *int64 `json:"error" yaml:"error"`
	Total   *int64 `json:"total" yaml:"total"`
}

// LogCountRow is a table row of LogCount; an unknown count shows as "-".
type LogCountRow struct {
	Service string `table:"SERVICE"`
	Info    string `table:"INFO"`
	Warn    string `table:"WARN"`
	Error   string `table:"ERROR"`
	Total   string `table:"TOTAL"`
}

// logLevels are the levels log-counts counts, in column order. Each is
// matched as a full-text term because structured field queries
// (loglevel:ERROR) are not supported on DT Managed Classic.
var logLevels = []string{"INFO", "WARN", "ERROR"}

func newQueryLogCountsCmd(a *App) *cobra.Command {
	var opts struct {
		entity      string
		from        string
		to          string
		maxServices int
	}
	cmd := &cobra.Command{
		Use:     "log-counts",
		Aliases: []string{"log-count", "logs-count"},
		Short:   "Aggregate log counts by service and level (INFO/WARN/ERROR)",
		Long: `Aggregate log records from /api/v2/logs/aggregate, grouped by process group and log level.

Returns INFO, WARN, and ERROR counts per process group, named by its display name. The
--entity selector finds the process groups; the aggregate call groups by
dt.entity.process_group, and the result is filtered to the groups the selector found.

On Dynatrace Managed Classic, logs are attributed to PROCESS_GROUP entities (not SERVICE
entities). If the given --entity selector targets SERVICE entities (type(SERVICE),...), the
command automatically derives an equivalent PROCESS_GROUP selector by substituting the type.

Each level's aggregation returns at most --max-services process groups. When a level
reaches that limit, a process group missing from it may still have records at that level;
its count is shown as "-" (null in json and yaml) rather than 0.

NOTE: On Dynatrace Managed Classic, structured field queries (e.g. loglevel:ERROR) are
not supported. Log level counts are approximated by searching for "INFO", "WARN", "ERROR"
as full-text terms in log content. This is accurate for standard Spring Boot / Java logs
where the level appears in each log line.

Examples:
  dtmgd query log-counts --entity 'type(SERVICE),tag("env:prod")' --from now-1h
  dtmgd query log-counts --entity 'type(PROCESS_GROUP),tag("env:prod")' --from now-30m --to now`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.entity == "" {
				return &UsageError{Err: fmt.Errorf("--entity is required (e.g. 'type(SERVICE),tag(\"env:prod\")')")}
			}
			from := opts.from
			if from == "" {
				from = "now-1h"
			}
			to := opts.to
			if to == "" {
				to = "now"
			}
			maxGroupValues := opts.maxServices
			if maxGroupValues <= 0 || maxGroupValues > 100 {
				maxGroupValues = 100
			}

			// On DT Managed Classic, logs are attributed to PROCESS_GROUP entities, not SERVICE.
			// If the user passed a SERVICE selector, auto-convert to PROCESS_GROUP.
			pgSelector := strings.Replace(opts.entity, "type(SERVICE)", "type(PROCESS_GROUP)", 1)

			return a.runFetch(fetchSpec{
				Verb:     "query",
				Resource: "log-counts",
				Fetch: func(c *client.Client, _ int) (fetched, error) {
					counts, err := fetchLogCounts(c, pgSelector, from, to, maxGroupValues)
					if err != nil {
						return fetched{}, err
					}
					return fetched{Body: counts, Total: len(counts.LogCounts)}, nil
				},
				Human: func(w io.Writer, body interface{}) error {
					output.PrintInfo("Log counts for entity selector: %s", opts.entity)
					output.PrintInfo("Time range: %s → %s", from, to)
					return a.printLogCounts(w, body.(LogCounts), maxGroupValues)
				},
			})
		},
	}
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entitySelector to scope log aggregation (required)")
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (default: now-1h), e.g. now-2h")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (default: now)")
	cmd.Flags().IntVar(&opts.maxServices, "max-services", 100, "maximum number of process groups the aggregation returns per level (max: 100)")
	return withScopes(cmd, "logs.read", "entities.read")
}

// fetchLogCounts counts log records per level for the process groups that
// pgSelector selects. A failed entity lookup or aggregation is returned.
func fetchLogCounts(c *client.Client, pgSelector, from, to string, maxGroupValues int) (LogCounts, error) {
	raw, err := c.GetV2Paged("/entities",
		map[string]string{"entitySelector": pgSelector, "pageSize": "500"},
		client.Paging{ListKey: "entities"})
	if err != nil {
		return LogCounts{}, fmt.Errorf("look up process groups: %w", err)
	}
	var ents EntitiesResponse
	if err := client.DecodePaged(raw, &ents); err != nil {
		return LogCounts{}, err
	}
	// Keyed by lowercase ID: the entities API returns "PROCESS_GROUP-ABC"
	// but logs aggregate returns "process_group-abc" for the same entity.
	names := make(map[string]string, len(ents.Entities))
	for _, e := range ents.Entities {
		name := e.DisplayName
		if name == "" {
			name = e.EntityID
		}
		names[strings.ToLower(e.EntityID)] = name
	}
	if len(names) == 0 {
		return LogCounts{LogCounts: []LogCount{}}, nil
	}

	// entitySelector is intentionally omitted from the aggregate call: on DT
	// Managed Classic it is a hidden parameter that does not filter the
	// result (it comes back empty). The groups are filtered client-side.
	levels := make(map[string]map[string]*int64, len(logLevels)) // level → pgID → count
	for _, level := range logLevels {
		params := url.Values{
			"from":           {from},
			"to":             {to},
			"timeBuckets":    {"1"},
			"maxGroupValues": {strconv.Itoa(maxGroupValues)},
			"groupBy":        {"dt.entity.process_group"},
			"query":          {level},
		}
		var resp LogAggregateResponse
		if err := c.GetV2WithValues("/logs/aggregate", params, &resp); err != nil {
			return LogCounts{}, fmt.Errorf("aggregate %s: %w", level, err)
		}
		levels[level] = aggregateLevel(resp, names, maxGroupValues)
	}
	return buildLogCounts(names, levels), nil
}

// aggregateLevel returns the count of each process group in names from one
// level's aggregation, nil where the count is unknown.
//
// The aggregation returns at most maxGroupValues groups, the ones with the
// most records. When it returns that many, a group missing from it may still
// have records at this level, so its count is unknown. When it returns fewer,
// every group with records is there, and a missing one has none.
func aggregateLevel(resp LogAggregateResponse, names map[string]string, maxGroupValues int) map[string]*int64 {
	found := map[string]int64{}
	groups := map[string]bool{}
	for _, bucket := range resp.AggregationResult["dt.entity.process_group"] {
		for pgID, n := range bucket {
			id := strings.ToLower(pgID)
			groups[id] = true
			if _, known := names[id]; known {
				found[id] += n
			}
		}
	}
	full := len(groups) >= maxGroupValues
	counts := make(map[string]*int64, len(names))
	for id := range names {
		if n, ok := found[id]; ok {
			counts[id] = &n
		} else if !full {
			counts[id] = new(int64)
		}
	}
	return counts
}

// buildLogCounts turns per-level counts into one LogCount per process group
// name, sorted by ERROR, WARN and total (descending), then name. Groups that
// share a display name are summed into one row; a sum with an unknown part is
// unknown. Every group appears, so one without records shows as 0 rather than
// being left out.
func buildLogCounts(names map[string]string, levels map[string]map[string]*int64) LogCounts {
	byName := map[string]*LogCount{}
	var order []string
	for id, name := range names {
		lc, ok := byName[name]
		if !ok {
			lc = &LogCount{Service: name, Info: new(int64), Warn: new(int64), Error: new(int64)}
			byName[name] = lc
			order = append(order, name)
		}
		lc.Info = addCount(lc.Info, levels["INFO"][id])
		lc.Warn = addCount(lc.Warn, levels["WARN"][id])
		lc.Error = addCount(lc.Error, levels["ERROR"][id])
	}
	out := make([]LogCount, 0, len(byName))
	for _, name := range order {
		lc := byName[name]
		lc.Total = addCount(addCount(lc.Info, lc.Warn), lc.Error)
		out = append(out, *lc)
	}
	sort.Slice(out, func(i, j int) bool {
		x, y := out[i], out[j]
		if a, b := sortKey(x.Error), sortKey(y.Error); a != b {
			return a > b
		}
		if a, b := sortKey(x.Warn), sortKey(y.Warn); a != b {
			return a > b
		}
		if a, b := sortKey(x.Total), sortKey(y.Total); a != b {
			return a > b
		}
		return x.Service < y.Service
	})
	return LogCounts{LogCounts: out}
}

// addCount adds two counts; the sum is unknown (nil) if either is.
func addCount(a, b *int64) *int64 {
	if a == nil || b == nil {
		return nil
	}
	sum := *a + *b
	return &sum
}

// sortKey orders unknown counts after every known one.
func sortKey(n *int64) int64 {
	if n == nil {
		return -1
	}
	return *n
}

// printLogCounts writes the counts as a table, with a stderr note for each
// level where some counts are unknown.
func (a *App) printLogCounts(w io.Writer, counts LogCounts, maxGroupValues int) error {
	if len(counts.LogCounts) == 0 {
		output.PrintInfo("No entities found for the specified entity selector.")
		return nil
	}
	cell := func(n *int64) string {
		if n == nil {
			return "-"
		}
		return strconv.FormatInt(*n, 10)
	}
	unknown := map[string]bool{}
	rows := make([]LogCountRow, 0, len(counts.LogCounts))
	for _, lc := range counts.LogCounts {
		rows = append(rows, LogCountRow{
			Service: lc.Service,
			Info:    cell(lc.Info),
			Warn:    cell(lc.Warn),
			Error:   cell(lc.Error),
			Total:   cell(lc.Total),
		})
		unknown["INFO"] = unknown["INFO"] || lc.Info == nil
		unknown["WARN"] = unknown["WARN"] || lc.Warn == nil
		unknown["ERROR"] = unknown["ERROR"] || lc.Error == nil
	}
	if err := output.NewPrinterToWithColumns(string(a.format()), w, a.columns()).PrintList(rows); err != nil {
		return err
	}
	for _, level := range logLevels {
		if unknown[level] {
			output.PrintInfo(`Some process groups are outside the top %d for %s; their counts are shown as "-".`, maxGroupValues, level)
		}
	}
	return nil
}
