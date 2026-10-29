package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// sloEvalPageSize is the maximum number of SLOs the Dynatrace API can evaluate per request.
const sloEvalPageSize = 25

// SLOListItem is the table row for SLOs.
type SLOListItem struct {
	ID           string `table:"ID"`
	Name         string `table:"NAME"`
	Status       string `table:"STATUS"`
	EvaluatedPct string `table:"EVALUATED%"`
	TargetPct    string `table:"TARGET%"`
	WarningPct   string `table:"WARNING%,wide"`
	Enabled      string `table:"ENABLED,wide"`
	Description  string `table:"DESCRIPTION,wide"`
}

// SLOsResponse models /api/v2/slo.
type SLOsResponse struct {
	Slo        []SLOEntry `json:"slo"`
	TotalCount int        `json:"totalCount"`
}

type SLOEntry struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Enabled        bool     `json:"enabled"`
	Status         string   `json:"status"`
	EvaluatedPctOf float64  `json:"evaluatedPercentage"`
	TargetSuccess  float64  `json:"target"`
	Warning        *float64 `json:"warning"`
}

func newGetSLOsCmd(a *App) *cobra.Command {
	var opts struct {
		enabled  string
		limit    int
		evaluate bool
		selector string
	}
	cmd := &cobra.Command{
		Use:     "slos",
		Aliases: []string{"slo"},
		Short:   "List Service Level Objectives",
		Long: `List SLOs from the Dynatrace Managed environment.

The --selector flag accepts the Dynatrace sloSelector DSL for server-side filtering:
  name("...")            exact name match
  text("...")            substring name search
  managementZone("...")  filter by management zone name
  managementZoneID("...") filter by management zone numeric ID
  healthState("HEALTHY"|"UNHEALTHY")

Examples:
  dtmgd get slos
  dtmgd get slos --selector 'managementZone("bookstore")' --evaluate
  dtmgd get slos --selector 'text("BookController")' --evaluate
  dtmgd get slos --enabled false
  dtmgd get slos --env ALL_ENVIRONMENTS`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// When --evaluate is active, the Dynatrace API enforces a hard limit of 25
			// SLOs per request. Subsequent pages use nextPageKey which encodes the
			// evaluate=true flag, so pagination still yields evaluated results.
			pageCap := 200
			if opts.evaluate {
				pageCap = sloEvalPageSize
			}

			params := map[string]string{}
			if opts.enabled != "" {
				params["enabledSlos"] = opts.enabled
			}
			if opts.evaluate {
				params["evaluate"] = "true"
			}
			if opts.selector != "" {
				params["sloSelector"] = opts.selector
			}

			spec := fetchSpec{
				Verb:     "get",
				Resource: "slos",
				Fetch:    pagedFetch("/slo", params, "slo", pageCap, a.Opts.MaxPages),
				Rows:     sloRows,
				Limit:    opts.limit,
				List:     true,
				Empty:    "No SLOs found.",
			}
			// Show the status summary only when evaluation was requested (otherwise
			// status values reflect cached last-evaluation and may be stale or
			// misleading). It is a human note, so json, yaml and agent output,
			// which never reach Human, are unaffected.
			if opts.evaluate {
				table := spec
				spec.Human = func(_ io.Writer, body interface{}) error {
					// listFetched re-derives the same Total/HasMore pagedFetch
					// computed, so the table keeps its empty and truncation notes.
					if err := a.renderHuman(table, listFetched(body.(map[string]interface{}), "slo")); err != nil {
						return err
					}
					return printSLOStatusSummary(body)
				}
			}
			return a.runFetch(spec)
		},
	}
	cmd.Flags().StringVar(&opts.enabled, "enabled", "", "filter by enabled status: true, false, or all")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of results to show")
	cmd.Flags().BoolVar(&opts.evaluate, "evaluate", false, "evaluate SLO percentages (slower; max 25 per page, auto-paged)")
	cmd.Flags().StringVar(&opts.selector, "selector", "", "sloSelector DSL to filter SLOs (e.g. 'managementZone(\"bookstore\")')")
	return withScopes(cmd, "slo.read")
}

// sloRows turns a /slo body into table rows.
func sloRows(body interface{}) (interface{}, int, error) {
	var resp SLOsResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return nil, 0, err
	}
	items := make([]SLOListItem, 0, len(resp.Slo))
	for _, s := range resp.Slo {
		enabled := "yes"
		if !s.Enabled {
			enabled = "no"
		}
		warningPct := "n/a"
		if s.Warning != nil {
			warningPct = fmt.Sprintf("%.2f%%", *s.Warning)
		}
		items = append(items, SLOListItem{
			ID:           s.ID,
			Name:         s.Name,
			Status:       s.Status,
			EvaluatedPct: formatEvaluatedPct(s.EvaluatedPctOf),
			TargetPct:    fmt.Sprintf("%.2f%%", s.TargetSuccess),
			WarningPct:   warningPct,
			Enabled:      enabled,
			Description:  truncate(s.Description, 60),
		})
	}
	return items, len(items), nil
}

// printSLOStatusSummary prints, to stderr, how many of the SLOs in a /slo
// body are in each status. It prints nothing for an empty body.
func printSLOStatusSummary(body interface{}) error {
	var resp SLOsResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return err
	}
	if len(resp.Slo) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, s := range resp.Slo {
		st := s.Status
		if st == "" {
			st = "NONE"
		}
		counts[st]++
	}
	output.PrintInfo("Status: %d SUCCESS (green)  %d WARNING (yellow)  %d FAILURE (red)  %d NONE",
		counts["SUCCESS"], counts["WARNING"], counts["FAILURE"], counts["NONE"])
	return nil
}

// formatEvaluatedPct formats the SLO evaluated percentage for display.
// Dynatrace returns -1 when the SLO cannot be evaluated (e.g. service was down
// for the entire evaluation window). We show "N/A" in that case.
func formatEvaluatedPct(v float64) string {
	if v < 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.2f%%", v)
}
