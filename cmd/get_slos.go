package cmd

import (
	"fmt"

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

var (
	sloEnabled  string
	sloLimit    int
	sloEval     bool
	sloSelector string
)

var getSLOsCmd = &cobra.Command{
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
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		// Decouple API page size from the user-visible result limit.
		// When --evaluate is active, the Dynatrace API enforces a hard limit of 25
		// SLOs per request. Subsequent pages use nextPageKey which encodes the
		// evaluate=true flag, so pagination still yields evaluated results.
		apiPageSize := 200
		if sloEval {
			apiPageSize = sloEvalPageSize
		}

		params := map[string]string{
			"pageSize": fmt.Sprintf("%d", apiPageSize),
		}
		if sloEnabled != "" {
			params["enabledSlos"] = sloEnabled
		}
		if sloEval {
			params["evaluate"] = "true"
		}
		if sloSelector != "" {
			params["sloSelector"] = sloSelector
		}

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				raw, err := c.GetV2Paged("/slo", params, effectiveMaxPages(sloLimit > 0))
				if err != nil {
					return nil, err
				}
				return raw, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("slos").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		raw, err := c.GetV2Paged("/slo", params, effectiveMaxPages(sloLimit > 0))
		if err != nil {
			return err
		}

		var resp SLOsResponse
		if err := client.DecodePaged(raw, &resp); err != nil {
			return err
		}

		if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
			return NewPrinterForResource("slos").Print(resp)
		}

		slos := resp.Slo
		if sloLimit > 0 && len(slos) > sloLimit {
			slos = slos[:sloLimit]
		}

		if len(slos) == 0 {
			output.PrintInfo("No SLOs found.")
			return nil
		}

		var items []SLOListItem
		for _, s := range slos {
			enabled := "yes"
			if !s.Enabled {
				enabled = "no"
			}
			warningPct := "n/a"
			if s.Warning != nil {
				warningPct = fmt.Sprintf("%.2f%%", *s.Warning)
			}
			evalPct := "N/A"
			if s.EvaluatedPctOf >= 0 {
				evalPct = fmt.Sprintf("%.2f%%", s.EvaluatedPctOf)
			}
			items = append(items, SLOListItem{
				ID:           s.ID,
				Name:         s.Name,
				Status:       s.Status,
				EvaluatedPct: evalPct,
				TargetPct:    fmt.Sprintf("%.2f%%", s.TargetSuccess),
				WarningPct:   warningPct,
				Enabled:      enabled,
				Description:  truncate(s.Description, 60),
			})
		}
		if resp.TotalCount > len(slos) {
			output.PrintInfo("Showing %d of %d SLOs.", len(slos), resp.TotalCount)
		}
		if err := NewPrinter().PrintList(items); err != nil {
			return err
		}

		// Show status summary only when evaluation was requested (otherwise status values
		// reflect cached last-evaluation and may be stale/misleading).
		if sloEval {
			counts := map[string]int{}
			for _, s := range slos {
				st := s.Status
				if st == "" {
					st = "NONE"
				}
				counts[st]++
			}
			output.PrintInfo("Status: %d SUCCESS (green)  %d WARNING (yellow)  %d FAILURE (red)  %d NONE",
				counts["SUCCESS"], counts["WARNING"], counts["FAILURE"], counts["NONE"])
		}
		return nil
	},
}

func init() {
	getCmd.AddCommand(getSLOsCmd)

	getSLOsCmd.Flags().StringVar(&sloEnabled, "enabled", "", "filter by enabled status: true, false, or all")
	getSLOsCmd.Flags().IntVar(&sloLimit, "limit", 0, "maximum number of results to show")
	getSLOsCmd.Flags().BoolVar(&sloEval, "evaluate", false, "evaluate SLO percentages (slower; max 25 per page, auto-paged)")
	getSLOsCmd.Flags().StringVar(&sloSelector, "selector", "", "sloSelector DSL to filter SLOs (e.g. 'managementZone(\"bookstore\")')")
}
