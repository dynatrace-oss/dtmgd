package cmd

import (
	"encoding/json"
	"fmt"

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
				return fmt.Errorf("--query is required (e.g. \"error\")")
			}
			if opts.from == "" {
				return fmt.Errorf("--from is required (e.g. now-1h)")
			}
			to := opts.to
			if to == "" {
				to = "now"
			}

			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}

			params := map[string]string{
				"query": opts.query,
				"from":  opts.from,
				"to":    to,
			}
			limit := 100
			if opts.limit > 0 {
				limit = opts.limit
			}
			params["limit"] = fmt.Sprintf("%d", limit)
			if opts.sort != "" {
				params["sort"] = opts.sort
			}
			if opts.entity != "" {
				params["entitySelector"] = opts.entity
			}

			if a.isMultiEnv() {
				data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
					raw, err := c.GetV2Paged("/logs/search", params, a.maxPagesFor(opts.limit > 0))
					if err != nil {
						return nil, err
					}
					return raw, nil
				})
				if err != nil {
					return err
				}
				return a.printer("logs").Print(data)
			}

			c, err := a.newClient(cfg)
			if err != nil {
				return err
			}

			raw, err := c.GetV2Paged("/logs/search", params, a.maxPagesFor(opts.limit > 0))
			if err != nil {
				return err
			}

			var resp LogsResponse
			if err := client.DecodePaged(raw, &resp); err != nil {
				return err
			}

			if a.Opts.Output == "json" || a.agentMode() {
				if a.agentMode() {
					return a.printer("logs").Print(resp)
				}
				out, err := json.MarshalIndent(resp, "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(a.Out, string(out))
				return nil
			}
			if a.Opts.Output == "yaml" {
				return a.printer("").Print(resp)
			}

			// Human-readable text output
			if len(resp.Results) == 0 {
				output.PrintInfo("No log records found.")
				return nil
			}

			output.PrintInfo("Found %d log records:", len(resp.Results))
			_, _ = fmt.Fprintln(a.Out)
			for _, entry := range resp.Results {
				ts := msToTime(entry.Timestamp)
				content := resolveLogContent(entry)
				status := resolveLogStatus(entry)
				if status != "" {
					_, _ = fmt.Fprintf(a.Out, "[%s] [%s] %s\n", ts, status, content)
				} else {
					_, _ = fmt.Fprintf(a.Out, "[%s] %s\n", ts, content)
				}
			}
			if resp.TotalCount > 0 && resp.TotalCount > len(resp.Results) {
				output.PrintInfo("\nShowing %d of %d records. Use --limit to retrieve more.", len(resp.Results), resp.TotalCount)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.query, "query", "", "text to search for in log content (required)")
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (required), e.g. now-1h")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (default: now)")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of records (default: 100)")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "sort order (e.g. -timestamp for newest first)")
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entitySelector to scope log search (e.g. 'type(SERVICE),tag(\"[Environment]BookStore\")')")
	return cmd
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
