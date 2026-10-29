package cmd

import (
	"fmt"

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

// getProblemsOptions holds the flags of get problems.
type getProblemsOptions struct {
	from     string
	to       string
	status   string
	impact   string
	selector string
	entity   string
	limit    int
	sort     string
}

// validate checks the enum flags against the API's values and replaces each
// with its canonical (upper-case) form.
func (o *getProblemsOptions) validate() error {
	var err error
	if o.status, err = enumFlag("status", o.status, problemStatuses, true); err != nil {
		return err
	}
	o.impact, err = enumFlag("impact", o.impact, problemImpactLevels, true)
	return err
}

func newGetProblemsCmd(a *App) *cobra.Command {
	var opts getProblemsOptions
	var watch watchOptions
	cmd := &cobra.Command{
		Use:     "problems",
		Args:    noArgs,
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
			if err := opts.validate(); err != nil {
				return err
			}
			return a.watchOrRun(watch, func() error {
				return a.runFetch(fetchSpec{
					Verb:     "get",
					Resource: "problems",
					// DT Managed limits pageSize to 10 when the fields parameter
					// is set; paging fulfils larger or unlimited requests.
					Fetch: pagedFetch("/problems", problemsParams(opts), "problems", 10, a.Opts.MaxPages),
					Rows:  problemRows,
					Link:  uiPage(uiProblems),
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
	cmd.Flags().StringVar(&opts.impact, "impact", "", "filter by impact level: APPLICATION, ENVIRONMENT, INFRASTRUCTURE or SERVICES (added to problemSelector)")
	cmd.Flags().StringVar(&opts.selector, "selector", "", `problemSelector DSL (e.g. managementZones("production"))`)
	cmd.Flags().StringVar(&opts.entity, "entity", "", "entitySelector to filter problems by entity")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of problems to return")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "sort order (e.g. +status, -startTime)")
	return withScopes(cmd, scopeProblemsRead)
}

// problemsParams is the /problems query for the flags: the time range (by
// default the last 24 hours), the fields the table shows, and the filters
// given, with --status, --impact and --selector joined into one
// problemSelector.
func problemsParams(opts getProblemsOptions) map[string]string {
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
	return params
}

// problemRows turns a /problems body into table rows.
func problemRows(body interface{}) (interface{}, int, error) {
	var resp ProblemsResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return nil, 0, err
	}
	items := make([]ProblemListItem, 0, len(resp.Problems))
	for _, p := range resp.Problems {
		items = append(items, ProblemListItem{
			ProblemID: p.ProblemID,
			DisplayID: p.DisplayID,
			Title:     p.Title,
			Status:    p.Status,
			Severity:  p.SeverityLevel,
			Impact:    p.ImpactLevel,
			StartTime: msToTime(p.StartTime),
			RootCause: rootCauseLabel(p),
		})
	}
	return items, len(items), nil
}

// rootCauseLabel is a problem's ROOT-CAUSE cell: the root-cause entity's name,
// followed by its ID when the API gives one as a string or as an object's
// "id"; "" when the problem has no root cause.
func rootCauseLabel(p ProblemEntry) string {
	if p.RootCauseEntity == nil {
		return ""
	}
	eid := ""
	switch v := p.RootCauseEntity.EntityID.(type) {
	case string:
		eid = v
	case map[string]interface{}:
		if id, ok := v["id"].(string); ok {
			eid = id
		}
	}
	if eid == "" {
		return p.RootCauseEntity.Name
	}
	return fmt.Sprintf("%s (%s)", p.RootCauseEntity.Name, eid)
}
