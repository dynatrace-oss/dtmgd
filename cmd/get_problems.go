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
	AffectedEntities []struct {
		EntityID interface{} `json:"entityId"`
		Name     string      `json:"name"`
	} `json:"affectedEntities"`
	ManagementZones []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"managementZones"`
}

func newGetProblemsCmd(a *App) *cobra.Command {
	var opts struct {
		from     string
		to       string
		status   string
		impact   string
		selector string
		entity   string
		limit    int
		sort     string
	}
	cmd := &cobra.Command{
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
			return a.watchOrRun(func() error {
				cfg, err := a.loadConfig()
				if err != nil {
					return err
				}

				from := opts.from
				if from == "" {
					from = "now-24h"
				}
				to := opts.to
				if to == "" {
					to = "now"
				}

				params := map[string]string{
					"from":   from,
					"to":     to,
					"fields": "+affectedEntities,+managementZones",
				}
				// Build problemSelector DSL: status(), impactLevel(), and any user-supplied selector
				var selectorParts []string
				if opts.status != "" {
					selectorParts = append(selectorParts, fmt.Sprintf("status(%q)", opts.status))
				}
				if opts.impact != "" {
					selectorParts = append(selectorParts, fmt.Sprintf("impactLevel(%q)", opts.impact))
				}
				if opts.selector != "" {
					selectorParts = append(selectorParts, opts.selector)
				}
				if sel := joinSelector(selectorParts...); sel != "" {
					params["problemSelector"] = sel
				}
				if opts.entity != "" {
					params["entitySelector"] = opts.entity
				}
				if opts.sort != "" {
					params["sort"] = opts.sort
				}
				// DT Managed limits pageSize to 10 when the fields parameter is set.
				// Cap at 10 and let pagination fulfil larger or unlimited requests.
				const maxPageSizeWithFields = 10
				pageSize := maxPageSizeWithFields
				if opts.limit > 0 && opts.limit < maxPageSizeWithFields {
					pageSize = opts.limit
				}
				params["pageSize"] = fmt.Sprintf("%d", pageSize)

				// Skip pagination only when a single page already covers the whole limit.
				// When opts.limit > maxPageSizeWithFields we must paginate to honour the request.
				singlePageSuffices := opts.limit > 0 && opts.limit <= maxPageSizeWithFields

				// Multi-env mode
				if a.isMultiEnv() {
					data, err := a.multiExec(cfg, func(c *client.Client) (interface{}, error) {
						return c.GetV2Paged("/problems", params, a.maxPagesFor(singlePageSuffices))
					})
					if err != nil {
						return err
					}
					return a.printer("problems").Print(data)
				}

				c, err := a.newClient(cfg)
				if err != nil {
					return err
				}

				raw, err := c.GetV2Paged("/problems", params, a.maxPagesFor(singlePageSuffices))
				if err != nil {
					return err
				}

				var resp ProblemsResponse
				if err := client.DecodePaged(raw, &resp); err != nil {
					return err
				}

				problems := resp.Problems
				if opts.limit > 0 && len(problems) > opts.limit {
					problems = problems[:opts.limit]
				}

				if len(problems) == 0 {
					output.PrintInfo("No problems found for the given filters.")
					return nil
				}

				if a.Opts.Output == "json" || a.Opts.Output == "yaml" || a.agentMode() {
					return a.printer("problems").Print(resp)
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
				return a.printer("").PrintList(items)
			})
		},
	}
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (e.g. now-24h, 2024-01-01T00:00:00Z)")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (default: now)")
	cmd.Flags().StringVar(&opts.status, "status", "", "filter by status: OPEN or CLOSED (added to problemSelector)")
	cmd.Flags().StringVar(&opts.impact, "impact", "", "filter by impact level: SERVICE, INFRASTRUCTURE, APPLICATION (added to problemSelector)")
	cmd.Flags().StringVar(&opts.selector, "selector", "", `problemSelector DSL (e.g. managementZones("bookstore"))`)
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entitySelector to filter problems by entity")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of problems to return")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "sort order (e.g. +status, -startTime)")
	return cmd
}
