package cmd

import (
	"fmt"
	"io"
	"net/url"

	"github.com/spf13/cobra"
)

// SLODetail maps the /slo/{id} API response for human-readable describe output.
type SLODetail struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Status               string   `json:"status"`
	Enabled              bool     `json:"enabled"`
	Target               float64  `json:"target"`
	Warning              *float64 `json:"warning"`
	EvaluatedPct         float64  `json:"evaluatedPercentage"`
	ErrorBudget          float64  `json:"errorBudget"`
	Timeframe            string   `json:"timeframe"`
	EvaluationType       string   `json:"evaluationType"`
	Filter               string   `json:"filter"`
	MetricExpression     string   `json:"metricExpression"`
	RelatedOpenProblems  int      `json:"relatedOpenProblems"`
	RelatedTotalProblems int      `json:"relatedTotalProblems"`
	ErrorBudgetBurnRate  *struct {
		BurnRateValue float64 `json:"burnRateValue"`
		BurnRateType  string  `json:"burnRateType"`
	} `json:"errorBudgetBurnRate"`
}

func printSLODetail(w io.Writer, d SLODetail) {
	enabled := "no"
	if d.Enabled {
		enabled = "yes"
	}
	warn := "—"
	if d.Warning != nil {
		warn = fmt.Sprintf("%.2f%%", *d.Warning)
	}
	burnRate := "—"
	if d.ErrorBudgetBurnRate != nil {
		burnRate = fmt.Sprintf("%.2f (%s)", d.ErrorBudgetBurnRate.BurnRateValue, d.ErrorBudgetBurnRate.BurnRateType)
	}
	_, _ = fmt.Fprintf(w, "ID:                   %s\n", d.ID)
	_, _ = fmt.Fprintf(w, "Name:                 %s\n", d.Name)
	_, _ = fmt.Fprintf(w, "Status:               %s\n", d.Status)
	_, _ = fmt.Fprintf(w, "Enabled:              %s\n", enabled)
	_, _ = fmt.Fprintf(w, "Target:               %.2f%%\n", d.Target)
	_, _ = fmt.Fprintf(w, "Warning:              %s\n", warn)
	_, _ = fmt.Fprintf(w, "Evaluated:            %.4f%%\n", d.EvaluatedPct)
	_, _ = fmt.Fprintf(w, "Error Budget:         %.4f\n", d.ErrorBudget)
	_, _ = fmt.Fprintf(w, "Burn Rate:            %s\n", burnRate)
	_, _ = fmt.Fprintf(w, "Timeframe:            %s\n", d.Timeframe)
	_, _ = fmt.Fprintf(w, "Evaluation Type:      %s\n", d.EvaluationType)
	if d.Filter != "" {
		_, _ = fmt.Fprintf(w, "Filter:               %s\n", d.Filter)
	}
	if d.MetricExpression != "" {
		_, _ = fmt.Fprintf(w, "Metric Expression:    %s\n", d.MetricExpression)
	}
	if d.RelatedOpenProblems > 0 || d.RelatedTotalProblems > 0 {
		_, _ = fmt.Fprintf(w, "Related Problems:     %d open / %d total\n", d.RelatedOpenProblems, d.RelatedTotalProblems)
	}
}

func newDescribeSLOCmd(a *App) *cobra.Command {
	var opts struct {
		timeframe string
		from      string
		to        string
	}
	cmd := &cobra.Command{
		Use:     "slo <slo-id>",
		Aliases: []string{"slos"},
		Short:   "Show detailed information about a specific SLO",
		Long: `Show SLO details including evaluation, error budget, and burn rate.

Examples:
  dtmgd describe slo <id>
  dtmgd describe slo <id> --timeframe CURRENT
  dtmgd describe slo <id> --from now-2w --to now

--from/--to imply --timeframe GTF when --timeframe is not given.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{}
			if opts.timeframe != "" {
				params["timeFrame"] = opts.timeframe
			}
			if opts.from != "" {
				params["from"] = opts.from
			}
			if opts.to != "" {
				params["to"] = opts.to
			}
			// The API evaluates over the SLO's own timeframe and ignores from
			// and to unless timeFrame is GTF, so explicit dates imply GTF.
			if opts.timeframe == "" && (opts.from != "" || opts.to != "") {
				params["timeFrame"] = "GTF"
			}

			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "slo",
				Fetch:    describeFetch(fmt.Sprintf("/slo/%s", url.PathEscape(args[0])), params),
				Human:    detailHuman(printSLODetail),
			})
		},
	}
	cmd.Flags().StringVar(&opts.timeframe, "timeframe", "", "time frame: CURRENT or GTF")
	cmd.Flags().StringVar(&opts.from, "from", "", "start time (implies --timeframe GTF)")
	cmd.Flags().StringVar(&opts.to, "to", "", "end time (implies --timeframe GTF)")
	return withScopes(cmd, scopeSLORead)
}
