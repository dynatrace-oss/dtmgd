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

var (
	qlQuery  string
	qlFrom   string
	qlTo     string
	qlLimit  int
	qlSort   string
	qlEntity string
)

var queryLogsCmd = &cobra.Command{
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
		if qlQuery == "" {
			return fmt.Errorf("--query is required (e.g. \"error\")")
		}
		if qlFrom == "" {
			return fmt.Errorf("--from is required (e.g. now-1h)")
		}
		if qlTo == "" {
			qlTo = "now"
		}

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		params := map[string]string{
			"query": qlQuery,
			"from":  qlFrom,
			"to":    qlTo,
		}
		limit := 100
		if qlLimit > 0 {
			limit = qlLimit
		}
		params["limit"] = fmt.Sprintf("%d", limit)
		if qlSort != "" {
			params["sort"] = qlSort
		}
		if qlEntity != "" {
			params["entitySelector"] = qlEntity
		}

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				raw, err := c.GetV2Paged("/logs/search", params, effectiveMaxPages(qlLimit > 0))
				if err != nil {
					return nil, err
				}
				return raw, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("logs").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		raw, err := c.GetV2Paged("/logs/search", params, effectiveMaxPages(qlLimit > 0))
		if err != nil {
			return err
		}

		var resp LogsResponse
		if err := client.DecodePaged(raw, &resp); err != nil {
			return err
		}

		if outputFormat == "json" || agentMode() {
			if agentMode() {
				return NewPrinterForResource("logs").Print(resp)
			}
			out, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		}
		if outputFormat == "yaml" {
			return NewPrinter().Print(resp)
		}

		// Human-readable text output
		if len(resp.Results) == 0 {
			output.PrintInfo("No log records found.")
			return nil
		}

		output.PrintInfo("Found %d log records:", len(resp.Results))
		fmt.Println()
		for _, entry := range resp.Results {
			ts := msToTime(entry.Timestamp)
			content := resolveLogContent(entry)
			status := resolveLogStatus(entry)
			if status != "" {
				fmt.Printf("[%s] [%s] %s\n", ts, status, content)
			} else {
				fmt.Printf("[%s] %s\n", ts, content)
			}
		}
		if resp.TotalCount > 0 && resp.TotalCount > len(resp.Results) {
			output.PrintInfo("\nShowing %d of %d records. Use --limit to retrieve more.", len(resp.Results), resp.TotalCount)
		}
		return nil
	},
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

func init() {
	queryCmd.AddCommand(queryLogsCmd)

	queryLogsCmd.Flags().StringVar(&qlQuery, "query", "", "text to search for in log content (required)")
	queryLogsCmd.Flags().StringVar(&qlFrom, "from", "", "start time (required), e.g. now-1h")
	queryLogsCmd.Flags().StringVar(&qlTo, "to", "", "end time (default: now)")
	queryLogsCmd.Flags().IntVar(&qlLimit, "limit", 0, "maximum number of records (default: 100)")
	queryLogsCmd.Flags().StringVar(&qlSort, "sort", "", "sort order (e.g. -timestamp for newest first)")
	queryLogsCmd.Flags().StringVar(&qlEntity, "entity", "", "entitySelector to scope log search (e.g. 'type(SERVICE),tag(\"[Environment]BookStore\")')")
}
