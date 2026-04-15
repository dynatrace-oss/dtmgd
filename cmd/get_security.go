package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// SecurityProblemListItem is the table row for security problems.
type SecurityProblemListItem struct {
	SecurityID  string `table:"SECURITY-ID"`
	DisplayID   string `table:"DISPLAY-ID"`
	PackageName string `table:"PACKAGE"`
	Technology  string `table:"TECH"`
	RiskLevel   string `table:"RISK"`
	RiskScore   string `table:"SCORE,wide"`
	Status      string `table:"STATUS"`
	CVEs        string `table:"CVE,wide"`
}

// SecurityProblemsResponse models /api/v2/securityProblems.
type SecurityProblemsResponse struct {
	SecurityProblems []SecurityProblemEntry `json:"securityProblems"`
	TotalCount       int                    `json:"totalCount"`
}

type SecurityProblemEntry struct {
	SecurityProblemID string `json:"securityProblemId"`
	DisplayID         string `json:"displayId"`
	PackageName       string `json:"packageName"`
	Technology        string `json:"technology"`
	RiskAssessment    *struct {
		RiskLevel string  `json:"riskLevel"`
		RiskScore float64 `json:"riskScore"`
	} `json:"riskAssessment"`
	Status string   `json:"status"`
	CveIds []string `json:"cveIds"`
}

var (
	spRisk     string
	spStatus   string
	spLimit    int
	spSelector string
)

var getSecurityProblemsCmd = &cobra.Command{
	Use:     "security-problems",
	Aliases: []string{"sp", "security", "vuln"},
	Short:   "List security vulnerabilities",
	Long: `List security problems (CVE vulnerabilities) in the Dynatrace Managed environment.

Examples:
  dtmgd get security-problems
  dtmgd get security-problems --risk CRITICAL
  dtmgd get security-problems --selector 'managementZones("BookStore")'
  dtmgd get security-problems --env ALL_ENVIRONMENTS`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		params := map[string]string{}
		params["fields"] = "+riskAssessment,+managementZones"
		params["sort"] = "-riskAssessment.riskScore"
		pageSize := 200
		if spLimit > 0 {
			pageSize = spLimit
		}
		params["pageSize"] = fmt.Sprintf("%d", pageSize)

		// Build securityProblemSelector from all filters (status, risk, and explicit selector)
		// Note: the API list endpoint has no standalone status/riskLevel params — all
		// filtering goes through securityProblemSelector as a comma-separated DSL.
		var selectorParts []string
		if spStatus != "" {
			selectorParts = append(selectorParts, fmt.Sprintf("status(%q)", spStatus))
		}
		if spRisk != "" {
			selectorParts = append(selectorParts, fmt.Sprintf("riskLevel(%q)", spRisk))
		}
		if spSelector != "" {
			selectorParts = append(selectorParts, spSelector)
		}
		if len(selectorParts) > 0 {
			params["securityProblemSelector"] = joinSelector(selectorParts...)
		}

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				raw, err := c.GetV2Paged("/securityProblems", params, effectiveMaxPages(spLimit > 0))
				if err != nil {
					return nil, err
				}
				return raw, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("securityProblems").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		raw, err := c.GetV2Paged("/securityProblems", params, effectiveMaxPages(spLimit > 0))
		if err != nil {
			return err
		}

		var resp SecurityProblemsResponse
		if err := client.DecodePaged(raw, &resp); err != nil {
			return err
		}

		if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
			return NewPrinterForResource("securityProblems").Print(resp)
		}

		problems := resp.SecurityProblems
		if spLimit > 0 && len(problems) > spLimit {
			problems = problems[:spLimit]
		}

		if len(problems) == 0 {
			output.PrintInfo("No security problems found.")
			return nil
		}

		var items []SecurityProblemListItem
		for _, sp := range problems {
			items = append(items, securityEntryToListItem(sp))
		}
		if resp.TotalCount > len(problems) {
			output.PrintInfo("Showing %d of %d security problems.", len(problems), resp.TotalCount)
		}
		return NewPrinter().PrintList(items)
	},
}

// securityEntryToListItem converts a SecurityProblemEntry to a table row.
// The CVEs field shows the first CVE; if there are more, appends " +N".
func securityEntryToListItem(sp SecurityProblemEntry) SecurityProblemListItem {
	riskLevel := ""
	riskScore := ""
	if sp.RiskAssessment != nil {
		riskLevel = sp.RiskAssessment.RiskLevel
		riskScore = fmt.Sprintf("%.1f", sp.RiskAssessment.RiskScore)
	}
	cves := ""
	if len(sp.CveIds) > 0 {
		cves = sp.CveIds[0]
		if len(sp.CveIds) > 1 {
			cves += fmt.Sprintf(" +%d", len(sp.CveIds)-1)
		}
	}
	return SecurityProblemListItem{
		SecurityID:  sp.SecurityProblemID,
		DisplayID:   sp.DisplayID,
		PackageName: sp.PackageName,
		Technology:  sp.Technology,
		RiskLevel:   riskLevel,
		RiskScore:   riskScore,
		Status:      sp.Status,
		CVEs:        cves,
	}
}

func init() {
	getCmd.AddCommand(getSecurityProblemsCmd)

	getSecurityProblemsCmd.Flags().StringVar(&spRisk, "risk", "", "filter by risk level: LOW, MEDIUM, HIGH, CRITICAL")
	getSecurityProblemsCmd.Flags().StringVar(&spStatus, "status", "", "filter by status: OPEN, RESOLVED, MUTED")
	getSecurityProblemsCmd.Flags().IntVar(&spLimit, "limit", 0, "maximum number of results")
	getSecurityProblemsCmd.Flags().StringVar(&spSelector, "selector", "", `security problem selector, e.g. managementZones("BookStore")`)
}
