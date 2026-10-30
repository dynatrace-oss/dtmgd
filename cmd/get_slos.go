package cmd

import (
	"fmt"
	"io"
	"strconv"

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
	Description  string `table:"DESCRIPTION,wide,max=60"`
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

// Values the /slo endpoints accept, sent as spelled here.
var (
	sloTimeFrames    = []string{"CURRENT", "GTF"}
	sloEnabledStates = []string{"true", "false", "all"}
	sloSortOrders    = []string{"name", "-name"}
)

// getSLOsOptions holds the flags of get slos.
type getSLOsOptions struct {
	enabled    string
	limit      int
	evaluate   bool
	selector   string
	timeframe  string
	from       string
	to         string
	sort       string
	showGlobal bool
	demo       bool
	// showGlobalSet and demoSet record whether --show-global and --demo were
	// given; only then are they sent, false included.
	showGlobalSet bool
	demoSet       bool
}

// validate checks the enum flags against the API's values and replaces each
// with its canonical form.
func (o *getSLOsOptions) validate() error {
	var err error
	if o.timeframe, err = enumFlag("timeframe", o.timeframe, sloTimeFrames, true); err != nil {
		return err
	}
	if o.enabled, err = enumFlag("enabled", o.enabled, sloEnabledStates, false); err != nil {
		return err
	}
	o.sort, err = enumFlag("sort", o.sort, sloSortOrders, false)
	return err
}

// slosParams is the /slo query for the flags. Only given flags are sent, so
// the API's defaults apply otherwise; --from or --to without --timeframe
// imply GTF, because the API ignores them for any other timeframe.
func slosParams(opts getSLOsOptions) map[string]string {
	timeframe := opts.timeframe
	if timeframe == "" && (opts.from != "" || opts.to != "") {
		timeframe = "GTF"
	}
	params := map[string]string{}
	for key, v := range map[string]string{
		"enabledSlos": opts.enabled,
		"sloSelector": opts.selector,
		"timeFrame":   timeframe,
		"from":        opts.from,
		"to":          opts.to,
		"sort":        opts.sort,
	} {
		if v != "" {
			params[key] = v
		}
	}
	if opts.evaluate {
		params["evaluate"] = "true"
	}
	if opts.showGlobalSet {
		params["showGlobalSlos"] = strconv.FormatBool(opts.showGlobal)
	}
	if opts.demoSet {
		params["demo"] = strconv.FormatBool(opts.demo)
	}
	return params
}

func newGetSLOsCmd(a *App) *cobra.Command {
	var opts getSLOsOptions
	cmd := &cobra.Command{
		Use:     "slos",
		Args:    noArgs,
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
  dtmgd get slos --selector 'managementZone("production")' --evaluate
  dtmgd get slos --selector 'text("checkout")' --evaluate
  dtmgd get slos --enabled false
  dtmgd get slos --evaluate --from now-7d          # evaluate over the last 7 days
  dtmgd get slos --show-global=false               # leave out global SLOs
  dtmgd get slos --env ALL_ENVIRONMENTS

SLOs are evaluated over their own timeframe (--timeframe CURRENT, the
default). --from or --to evaluate over the given dates instead and imply
--timeframe GTF.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.validate(); err != nil {
				return err
			}
			opts.showGlobalSet = cmd.Flags().Changed("show-global")
			opts.demoSet = cmd.Flags().Changed("demo")
			// When --evaluate is active, the Dynatrace API enforces a hard limit of 25
			// SLOs per request. Subsequent pages use nextPageKey which encodes the
			// evaluate=true flag, so pagination still yields evaluated results.
			pageCap := 200
			if opts.evaluate {
				pageCap = sloEvalPageSize
			}

			spec := fetchSpec{
				Verb:     "get",
				Resource: "slos",
				Fetch:    pagedFetch("/slo", slosParams(opts), "slo", pageCap, a.Opts.MaxPages),
				Rows:     sloRows,
				Link:     uiPage(uiSLOs),
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
	cmd.Flags().StringVar(&opts.selector, "selector", "", "sloSelector DSL to filter SLOs (e.g. 'managementZone(\"production\")')")
	cmd.Flags().StringVar(&opts.timeframe, "timeframe", "", "time frame: CURRENT (the SLO's own, the default) or GTF (--from/--to)")
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (implies --timeframe GTF)")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (implies --timeframe GTF)")
	cmd.Flags().StringVar(&opts.sort, "sort", "", "sort order: name or -name (default: name)")
	cmd.Flags().BoolVar(&opts.showGlobal, "show-global", true, "include global SLOs whatever the filter (sent only when given; the API default is true)")
	cmd.Flags().BoolVar(&opts.demo, "demo", false, "return demo SLOs only (sent only when given)")
	return withScopes(cmd, scopeSLORead)
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
			Description:  s.Description,
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
