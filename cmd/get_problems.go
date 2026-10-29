package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
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
	var watch watchOptions
	cmd := &cobra.Command{
		Use:     "problems",
		Aliases: []string{"prob", "problem"},
		Short:   "List problems from the Dynatrace Managed environment",
		Long: `List active or resolved problems. Defaults to the last 24 hours.

Filter examples:
  --status OPEN          only active problems
  --impact SERVICES       service-level impact
  --entity type(SERVICE),entityName.contains("checkout")
  --sort "+status"        open first

Multi-environment:
  dtmgd get problems --env ALL_ENVIRONMENTS
  dtmgd get problems --env "prod;staging"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.watchOrRun(watch, func() error {
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

				return a.runFetch(fetchSpec{
					Verb:     "get",
					Resource: "problems",
					// DT Managed limits pageSize to 10 when the fields parameter
					// is set; paging fulfils larger or unlimited requests.
					Fetch: pagedFetch("/problems", params, "problems", 10, a.Opts.MaxPages),
					Rows:  problemRows,
					Limit: opts.limit,
					List:  true,
					Empty: "No problems found for the given filters.",
				})
			})
		},
	}
	addWatchFlags(cmd, &watch)
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (e.g. now-24h, 2024-01-01T00:00:00Z)")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (default: now)")
	cmd.Flags().StringVar(&opts.status, "status", "", "filter by status: OPEN or CLOSED (added to problemSelector)")
	cmd.Flags().StringVar(&opts.impact, "impact", "", "filter by impact level: SERVICES, INFRASTRUCTURE, APPLICATION (added to problemSelector)")
	cmd.Flags().StringVar(&opts.selector, "selector", "", `problemSelector DSL (e.g. managementZones("production"))`)
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entitySelector to filter problems by entity")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of problems to return")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "sort order (e.g. +status, -startTime)")
	return withScopes(cmd, "problems.read")
}

// problemRows turns a /problems body into table rows.
func problemRows(body interface{}) (interface{}, int, error) {
	var resp ProblemsResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return nil, 0, err
	}
	items := make([]ProblemListItem, 0, len(resp.Problems))
	for _, p := range resp.Problems {
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
	return items, len(items), nil
}
