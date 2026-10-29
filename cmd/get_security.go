package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
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

// getSecurityProblemsOptions holds the flags of get security-problems.
type getSecurityProblemsOptions struct {
	risk     string
	status   string
	limit    int
	selector string
}

// validate checks the enum flags against the API's values and replaces each
// with its canonical (upper-case) form.
func (o *getSecurityProblemsOptions) validate() error {
	var err error
	if o.status, err = enumFlag("status", o.status, securityProblemStatuses, true); err != nil {
		return err
	}
	o.risk, err = enumFlag("risk", o.risk, securityRiskLevels, true)
	return err
}

func newGetSecurityProblemsCmd(a *App) *cobra.Command {
	var opts getSecurityProblemsOptions
	cmd := &cobra.Command{
		Use:     "security-problems",
		Aliases: []string{"sp", "security", "vuln"},
		Short:   "List security vulnerabilities",
		Long: `List security problems (CVE vulnerabilities) in the Dynatrace Managed environment.

Examples:
  dtmgd get security-problems
  dtmgd get security-problems --risk CRITICAL
  dtmgd get security-problems --selector 'managementZones("production")'
  dtmgd get security-problems --env ALL_ENVIRONMENTS`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.validate(); err != nil {
				return err
			}
			params := map[string]string{
				"fields": "+riskAssessment,+managementZones",
				"sort":   "-riskAssessment.riskScore",
			}

			// Build securityProblemSelector from all filters (status, risk, and explicit selector)
			// Note: the API list endpoint has no standalone status/riskLevel params — all
			// filtering goes through securityProblemSelector as a comma-separated DSL.
			var selectorParts []string
			if opts.status != "" {
				selectorParts = append(selectorParts, fmt.Sprintf("status(%q)", opts.status))
			}
			if opts.risk != "" {
				selectorParts = append(selectorParts, fmt.Sprintf("riskLevel(%q)", opts.risk))
			}
			if opts.selector != "" {
				selectorParts = append(selectorParts, opts.selector)
			}
			if len(selectorParts) > 0 {
				params["securityProblemSelector"] = joinSelector(selectorParts...)
			}

			return a.runFetch(fetchSpec{
				Verb:     "get",
				Resource: "security-problems",
				Fetch:    pagedFetch("/securityProblems", params, "securityProblems", 200, a.Opts.MaxPages),
				Rows:     securityProblemRows,
				Limit:    opts.limit,
				List:     true,
				Empty:    "No security problems found.",
			})
		},
	}
	cmd.Flags().StringVar(&opts.risk, "risk", "", "filter by risk level: CRITICAL, HIGH, MEDIUM, LOW or NONE")
	cmd.Flags().StringVar(&opts.status, "status", "", "filter by status: OPEN or RESOLVED (muted ones: --selector 'muted(\"TRUE\")')")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of results")
	cmd.Flags().StringVar(&opts.selector, "selector", "", `security problem selector, e.g. managementZones("production")`)
	return withScopes(cmd, scopeSecurityProblemsRead)
}

// securityProblemRows turns a /securityProblems body into table rows.
func securityProblemRows(body interface{}) (interface{}, int, error) {
	var resp SecurityProblemsResponse
	if err := client.DecodePaged(body.(map[string]interface{}), &resp); err != nil {
		return nil, 0, err
	}
	items := make([]SecurityProblemListItem, 0, len(resp.SecurityProblems))
	for _, sp := range resp.SecurityProblems {
		items = append(items, securityEntryToListItem(sp))
	}
	return items, len(items), nil
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
