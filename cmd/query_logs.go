package cmd

import (
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// LogsResponse models /api/v2/logs/search.
type LogsResponse struct {
	Results    []LogEntry `json:"results"`
	TotalCount int        `json:"totalCount"`
	SlicedBy   []string   `json:"slicedBy"`
}

type LogEntry struct {
	Timestamp int64                  `json:"timestamp"`
	Status    string                 `json:"status"`
	Event     map[string]interface{} `json:"event"`
	Content   string                 `json:"content"`
}

func newQueryLogsCmd(a *App) *cobra.Command {
	var opts struct {
		query  string
		from   string
		to     string
		limit  int
		sort   string
		entity string
	}
	cmd := &cobra.Command{
		Use:     "logs",
		Args:    noArgs,
		Aliases: []string{"log"},
		Short:   "Search log records",
		Long: `Search log records using simple text queries.

Managed clusters support basic text search. Do NOT use structured syntax
like "content:error" — just use the plain search term.

Examples:
  dtmgd query logs --query "error" --from now-1h --to now
  dtmgd query logs --query "timeout" --from now-30m --limit 50
  dtmgd query logs --env ALL_ENVIRONMENTS --query "error" --from now-1h`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.query == "" {
				return &UsageError{Err: fmt.Errorf("--query is required (e.g. \"error\")")}
			}
			if opts.from == "" {
				return &UsageError{Err: fmt.Errorf("--from is required (e.g. now-1h)")}
			}
			to := opts.to
			if to == "" {
				to = "now"
			}

			// Newest first unless --sort says otherwise: the API's and the
			// MCP server's default, sent explicitly.
			sort := opts.sort
			if sort == "" {
				sort = "-timestamp"
			}
			params := map[string]string{
				"query": opts.query,
				"from":  opts.from,
				"to":    to,
				"sort":  sort,
			}
			if opts.entity != "" {
				params["entitySelector"] = opts.entity
			}

			return a.runFetch(fetchSpec{
				Verb:     "query",
				Resource: "logs",
				Fetch:    logSearchFetch(params),
				Human:    printLogRecords,
				Link:     uiPage(uiLogs),
				Limit:    opts.limit,
				List:     true,
			})
		},
	}
	cmd.Flags().StringVar(&opts.query, "query", "", "text to search for in log content (required)")
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (required), e.g. now-1h")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (default: now)")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of records, at most 1000 (default: 100)")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "sort order: -timestamp (newest first, the default) or +timestamp")
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entitySelector to scope log search (e.g. 'type(PROCESS_GROUP),tag(\"env:prod\")'); not a documented /logs/search parameter, so check results against an unscoped search")
	return withScopes(cmd, scopeLogsRead)
}

// logSearchFetch is the Fetch of query logs: one /logs/search request with
// params and the limit logSearchLimit derives from the effective item limit.
func logSearchFetch(params map[string]string) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		p := make(map[string]string, len(params)+1)
		for k, v := range params {
			p[k] = v
		}
		p["limit"] = strconv.Itoa(logSearchLimit(limit))
		var body map[string]interface{}
		if err := c.GetV2("/logs/search", p, &body); err != nil {
			return fetched{}, err
		}
		return logsFetched(body), nil
	}
}

// Default and largest limit of one /logs/search request.
const (
	logSearchDefaultLimit = 100
	logSearchMaxLimit     = 1000
)

// logSearchLimit is the limit sent to /logs/search for an effective item
// limit (0 = none): the default without one, and never above the API's cap.
func logSearchLimit(limit int) int {
	switch {
	case limit <= 0:
		return logSearchDefaultLimit
	case limit > logSearchMaxLimit:
		return logSearchMaxLimit
	}
	return limit
}

// logsFetched wraps a /logs/search body. The endpoint pages by slice, so a
// nextSliceKey means more records match than the body holds. An absent
// results list becomes an empty one, so an empty result still prints it.
func logsFetched(body map[string]interface{}) fetched {
	results, ok := body["results"].([]interface{})
	if !ok {
		results = []interface{}{}
		body["results"] = results
	}
	key, _ := body["nextSliceKey"].(string)
	more := key != ""
	total := -1
	if tc, ok := body["totalCount"].(float64); ok {
		total = int(tc)
	} else if !more {
		total = len(results)
	}
	if total > len(results) {
		more = true
	}
	return fetched{Body: body, Total: total, HasMore: more, Count: len(results)}
}

// printLogRecords writes one "[time] [STATUS] content" line per record, with
// the record count and any truncation note on stderr.
func printLogRecords(w io.Writer, body interface{}) error {
	m := body.(map[string]interface{})
	var resp LogsResponse
	if err := client.DecodePaged(m, &resp); err != nil {
		return err
	}
	if len(resp.Results) == 0 {
		output.PrintInfo("No log records found.")
		return nil
	}
	output.PrintInfo("Found %d log records:", len(resp.Results))
	for _, entry := range resp.Results {
		ts := msToTime(entry.Timestamp)
		content := resolveLogContent(entry)
		if status := resolveLogStatus(entry); status != "" {
			_, _ = fmt.Fprintf(w, "[%s] [%s] %s\n", ts, status, content)
		} else {
			_, _ = fmt.Fprintf(w, "[%s] %s\n", ts, content)
		}
	}
	if f := logsFetched(m); f.HasMore {
		output.PrintInfo("%s. Use --limit or narrower filters.", showing(len(resp.Results), f.Total, "log records"))
	}
	return nil
}

// resolveLogContent returns the log content from a LogEntry.
// Falls back to entry.Event["content"] when Content is empty (DT Managed Classic format).
func resolveLogContent(entry LogEntry) string {
	if entry.Content != "" {
		return entry.Content
	}
	if c, ok := entry.Event["content"]; ok {
		return fmt.Sprintf("%v", c)
	}
	return ""
}

// resolveLogStatus returns the log status/level from a LogEntry.
// Falls back to entry.Event["status"] when Status is empty.
func resolveLogStatus(entry LogEntry) string {
	if entry.Status != "" {
		return entry.Status
	}
	if s, ok := entry.Event["status"]; ok {
		return fmt.Sprintf("%v", s)
	}
	return ""
}
