package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// SLOListItem is the table row for SLOs.
type SLOListItem struct {
	ID           string `table:"ID"`
	Name         string `table:"NAME"`
	Status       string `table:"STATUS"`
	EvaluatedPct string `table:"EVALUATED%"`
	TargetPct    string `table:"TARGET%"`
	Enabled      string `table:"ENABLED,wide"`
	Description  string `table:"DESCRIPTION,wide"`
}

// SLOsResponse models /api/v2/slo.
type SLOsResponse struct {
	Slo        []SLOEntry `json:"slo"`
	TotalCount int        `json:"totalCount"`
}

type SLOEntry struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Enabled        bool    `json:"enabled"`
	Status         string  `json:"status"`
	EvaluatedPctOf float64 `json:"evaluatedPercentage"`
	TargetSuccess  float64 `json:"target"`
}

var (
	sloEnabled string
	sloLimit   int
	sloEval    bool
)

var getSLOsCmd = &cobra.Command{
	Use:     "slos",
	Aliases: []string{"slo"},
	Short:   "List Service Level Objectives",
	Long: `List SLOs from the Dynatrace Managed environment.

Examples:
  dtmgd get slos
  dtmgd get slos --enabled false
  dtmgd get slos --env ALL_ENVIRONMENTS`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		params := map[string]string{}
		pageSize := 200
		if sloLimit > 0 {
			pageSize = sloLimit
		}
		params["pageSize"] = fmt.Sprintf("%d", pageSize)
		if sloEnabled != "" {
			params["enabledSlos"] = sloEnabled
		}
		if sloEval {
			params["evaluate"] = "true"
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
			items = append(items, SLOListItem{
				ID:           s.ID,
				Name:         s.Name,
				Status:       s.Status,
				EvaluatedPct: fmt.Sprintf("%.2f%%", s.EvaluatedPctOf),
				TargetPct:    fmt.Sprintf("%.2f%%", s.TargetSuccess),
				Enabled:      enabled,
				Description:  truncate(s.Description, 60),
			})
		}
		if resp.TotalCount > len(slos) {
			output.PrintInfo("Showing %d of %d SLOs.", len(slos), resp.TotalCount)
		}
		return NewPrinter().PrintList(items)
	},
}

func init() {
	getCmd.AddCommand(getSLOsCmd)

	getSLOsCmd.Flags().StringVar(&sloEnabled, "enabled", "", "filter by enabled status: true, false, or all")
	getSLOsCmd.Flags().IntVar(&sloLimit, "limit", 0, "maximum number of SLOs")
	getSLOsCmd.Flags().BoolVar(&sloEval, "evaluate", false, "evaluate SLO percentages (slower)")
}
