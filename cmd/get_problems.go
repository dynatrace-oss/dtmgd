package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// ProblemListItem is the table row for a problem.
type ProblemListItem struct {
	ProblemID string `table:"PROBLEM-ID"`
	DisplayID string `table:"DISPLAY-ID"`
	Title     string `table:"TITLE"`
	Status    string `table:"STATUS"`
	Severity  string `table:"SEVERITY"`
	Impact    string `table:"IMPACT"`
	StartTime string `table:"START-TIME,wide"`
	RootCause string `table:"ROOT-CAUSE,wide"`
}

// ProblemsResponse models the /api/v2/problems response.
type ProblemsResponse struct {
	Problems   []ProblemEntry `json:"problems"`
	TotalCount int            `json:"totalCount"`
}

// ProblemEntry is a single problem in the list.
type ProblemEntry struct {
	ProblemID       string `json:"problemId"`
	DisplayID       string `json:"displayId"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	SeverityLevel   string `json:"severityLevel"`
	ImpactLevel     string `json:"impactLevel"`
	StartTime       int64  `json:"startTime"`
	RootCauseEntity *struct {
		EntityID interface{} `json:"entityId"`
		Name     string      `json:"name"`
	} `json:"rootCauseEntity"`
}

var (
	probFrom      string
	probTo        string
	probStatus    string
	probImpact    string
	probSelector  string
	probEntitySel string
	probLimit     int
	probSort      string
)

var getProblemsCmd = &cobra.Command{
	Use:     "problems",
	Aliases: []string{"prob", "problem"},
	Short:   "List problems from the Dynatrace Managed environment",
	Long: `List active or resolved problems. Defaults to the last 24 hours.

Filter examples:
  --status OPEN          only active problems
  --impact SERVICE        service-level impact
  --entity type(SERVICE),entityName.contains("checkout")
  --sort "+status"        open first

Multi-environment:
  dtmgd get problems --env ALL_ENVIRONMENTS
  dtmgd get problems --env "prod;staging"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return watchOrRun(func() error {
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}

			if probFrom == "" {
				probFrom = "now-24h"
			}
			if probTo == "" {
				probTo = "now"
			}

			params := map[string]string{
				"from": probFrom,
				"to":   probTo,
			}
			// Build problemSelector DSL: status(), impactLevel(), and any user-supplied selector
			var selectorParts []string
			if probStatus != "" {
				selectorParts = append(selectorParts, fmt.Sprintf("status(%q)", probStatus))
			}
			if probImpact != "" {
				selectorParts = append(selectorParts, fmt.Sprintf("impactLevel(%q)", probImpact))
			}
			if probSelector != "" {
				selectorParts = append(selectorParts, probSelector)
			}
			if sel := joinSelector(selectorParts...); sel != "" {
				params["problemSelector"] = sel
			}
			if probEntitySel != "" {
				params["entitySelector"] = probEntitySel
			}
			if probSort != "" {
				params["sort"] = probSort
			}
			pageSize := 50
			if probLimit > 0 {
				pageSize = probLimit
			}
			params["pageSize"] = fmt.Sprintf("%d", pageSize)

			// Multi-env mode
			if isMultiEnv() {
				data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
					return c.GetV2Paged("/problems", params, effectiveMaxPages(probLimit > 0))
				})
				if err != nil {
					return err
				}
				return NewPrinterForResource("problems").Print(data)
			}

			c, err := NewClientFromConfig(cfg)
			if err != nil {
				return err
			}

			raw, err := c.GetV2Paged("/problems", params, effectiveMaxPages(probLimit > 0))
			if err != nil {
				return err
			}

			var resp ProblemsResponse
			if err := client.DecodePaged(raw, &resp); err != nil {
				return err
			}

			problems := resp.Problems
			if probLimit > 0 && len(problems) > probLimit {
				problems = problems[:probLimit]
			}

			if len(problems) == 0 {
				output.PrintInfo("No problems found for the given filters.")
				return nil
			}

			if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
				return NewPrinterForResource("problems").Print(resp)
			}

			var items []ProblemListItem
			for _, p := range problems {
				rootCause := ""
				if p.RootCauseEntity != nil {
					eid := ""
					switch v := p.RootCauseEntity.EntityID.(type) {
					case string:
						eid = v
					case map[string]interface{}:
						if id, ok := v["id"].(string); ok {
							eid = id
						}
					}
					if eid != "" {
						rootCause = fmt.Sprintf("%s (%s)", p.RootCauseEntity.Name, eid)
					} else {
						rootCause = p.RootCauseEntity.Name
					}
				}
				st := ""
				if p.StartTime > 0 {
					st = time.UnixMilli(p.StartTime).UTC().Format("2006-01-02 15:04:05")
				}
				items = append(items, ProblemListItem{
					ProblemID: p.ProblemID,
					DisplayID: p.DisplayID,
					Title:     p.Title,
					Status:    p.Status,
					Severity:  p.SeverityLevel,
					Impact:    p.ImpactLevel,
					StartTime: st,
					RootCause: rootCause,
				})
			}

			if resp.TotalCount > len(problems) {
				output.PrintInfo("Showing %d of %d problems. Use --limit or more specific filters.", len(problems), resp.TotalCount)
			}
			return NewPrinter().PrintList(items)
		})
	},
}

func init() {
	getCmd.AddCommand(getProblemsCmd)

	getProblemsCmd.Flags().StringVar(&probFrom, "from", "", "start time (e.g. now-24h, 2024-01-01T00:00:00Z)")
	getProblemsCmd.Flags().StringVar(&probTo, "to", "", "end time (default: now)")
	getProblemsCmd.Flags().StringVar(&probStatus, "status", "", "filter by status: OPEN or CLOSED (added to problemSelector)")
	getProblemsCmd.Flags().StringVar(&probImpact, "impact", "", "filter by impact level: SERVICE, INFRASTRUCTURE, APPLICATION (added to problemSelector)")
	getProblemsCmd.Flags().StringVar(&probSelector, "selector", "", `problemSelector DSL (e.g. managementZones("bookstore"))`)
	getProblemsCmd.Flags().StringVar(&probEntitySel, "entity", "", "entitySelector to filter problems by entity")
	getProblemsCmd.Flags().IntVar(&probLimit, "limit", 0, "maximum number of problems to return")
	getProblemsCmd.Flags().StringVar(&probSort, "sort", "", "sort order (e.g. +status, -startTime)")
}
