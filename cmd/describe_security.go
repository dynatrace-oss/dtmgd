package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// displayIDRegexp matches security problem display IDs of the form S-NNN (case-insensitive).
var displayIDRegexp = regexp.MustCompile(`(?i)^s-\d+$`)

// resolveSecurityProblemID converts a display ID (e.g. "S-281") to the internal UUID
// by listing security problems. If id is already a UUID or cannot be resolved, id is
// returned unchanged so the API call can fail with a meaningful 404.
func resolveSecurityProblemID(c *client.Client, id string) string {
	if !displayIDRegexp.MatchString(id) {
		return id
	}
	raw, err := c.GetV2Paged("/securityProblems", map[string]string{"pageSize": "200"}, 0)
	if err != nil {
		return id
	}
	var resp SecurityProblemsResponse
	if err := client.DecodePaged(raw, &resp); err != nil {
		return id
	}
	upperID := strings.ToUpper(id)
	for _, sp := range resp.SecurityProblems {
		if strings.ToUpper(sp.DisplayID) == upperID {
			return sp.SecurityProblemID
		}
	}
	return id
}

// SecurityProblemDetail maps the /securityProblems/{id} API response.
type SecurityProblemDetail struct {
	SecurityProblemID string   `json:"securityProblemId"`
	DisplayID         string   `json:"displayId"`
	Title             string   `json:"title"`
	Status            string   `json:"status"`
	Technology        string   `json:"technology"`
	PackageName       string   `json:"packageName"`
	CVEIds            []string `json:"cveIds"`
	Description       string   `json:"description"`
	URL               string   `json:"url"`
	RiskAssessment    struct {
		RiskLevel      string  `json:"riskLevel"`
		RiskScore      float64 `json:"riskScore"`
		BaseRiskLevel  string  `json:"baseRiskLevel"`
		BaseRiskScore  float64 `json:"baseRiskScore"`
		BaseRiskVector string  `json:"baseRiskVector"`
		PublicExploit  string  `json:"publicExploit"`
		DataAssets     string  `json:"dataAssets"`
		Exposure       string  `json:"exposure"`
	} `json:"riskAssessment"`
	VulnerableComponents []struct {
		DisplayName      string   `json:"displayName"`
		FileName         string   `json:"fileName"`
		NumberOfAffected int      `json:"numberOfAffectedEntities"`
		AffectedEntities []string `json:"affectedEntities"`
	} `json:"vulnerableComponents"`
	AffectedEntities []string `json:"affectedEntities"`
	ManagementZones  []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"managementZones"`
}

// printSecurityProblemDetail renders a SecurityProblemDetail as a human-readable summary.
func printSecurityProblemDetail(d SecurityProblemDetail) {
	ra := d.RiskAssessment

	// Vulnerable package: prefer the versioned component display name.
	pkg := d.PackageName
	if len(d.VulnerableComponents) > 0 && d.VulnerableComponents[0].DisplayName != "" {
		pkg = d.VulnerableComponents[0].DisplayName
	}

	// Short description: first non-empty non-heading paragraph.
	shortDesc := ""
	for _, line := range strings.Split(d.Description, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		line = strings.TrimSpace(line)
		if len(line) > 40 {
			if len(line) > 220 {
				line = line[:217] + "…"
			}
			shortDesc = line
			break
		}
	}

	// Affected process count.
	affected := len(d.AffectedEntities)
	if affected == 0 && len(d.VulnerableComponents) > 0 {
		affected = d.VulnerableComponents[0].NumberOfAffected
	}

	// Management zone names.
	var mzNames []string
	for _, mz := range d.ManagementZones {
		mzNames = append(mzNames, mz.Name)
	}

	fmt.Printf("%-22s %s\n", "ID:", d.SecurityProblemID)
	fmt.Printf("%-22s %s\n", "Display ID:", d.DisplayID)
	fmt.Printf("%-22s %s\n", "Title:", d.Title)
	fmt.Printf("%-22s %s\n", "CVE IDs:", strings.Join(d.CVEIds, ", "))
	fmt.Printf("%-22s %s\n", "Package:", pkg)
	fmt.Printf("%-22s %s\n", "Technology:", d.Technology)
	fmt.Printf("%-22s %s\n", "Status:", d.Status)
	fmt.Println()
	fmt.Printf("%-22s %s (score: %.1f)\n", "Risk Level:", ra.RiskLevel, ra.RiskScore)
	fmt.Printf("%-22s %s (base score: %.1f)\n", "CVSS Base Level:", ra.BaseRiskLevel, ra.BaseRiskScore)
	fmt.Printf("%-22s %s\n", "CVSS Vector:", ra.BaseRiskVector)
	fmt.Printf("%-22s %s\n", "Public Exploit:", ra.PublicExploit)
	fmt.Printf("%-22s %s\n", "Data Assets:", ra.DataAssets)
	fmt.Printf("%-22s %s\n", "Exposure:", ra.Exposure)
	fmt.Println()
	fmt.Printf("%-22s %d\n", "Affected Processes:", affected)
	if len(mzNames) > 0 {
		fmt.Printf("%-22s %s\n", "Management Zones:", strings.Join(mzNames, ", "))
	}
	if shortDesc != "" {
		fmt.Println()
		fmt.Printf("Description: %s\n", shortDesc)
	}
	if d.URL != "" {
		fmt.Println()
		fmt.Printf("URL: %s\n", d.URL)
	}
}

var describeSecurityProblemCmd = &cobra.Command{
	Use:     "security-problem <security-problem-id>",
	Aliases: []string{"sp", "vuln"},
	Short:   "Show detailed information about a security vulnerability",
	Long: `Show full CVE details, risk assessment, affected entities, and remediation hints
for a specific security problem.

Accepts either the display ID (e.g. S-281) from 'dtmgd get security-problems'
or the internal UUID. Display IDs are resolved automatically.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		spFields := "+riskAssessment,+managementZones,+codeLevelVulnerabilityDetails,+vulnerableComponents,+affectedEntities,+exposedEntities,+description"

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				resolvedID := resolveSecurityProblemID(c, args[0])
				path := fmt.Sprintf("/securityProblems/%s", url.PathEscape(resolvedID))
				var result map[string]interface{}
				if err := c.GetV2(path, map[string]string{"fields": spFields}, &result); err != nil {
					return nil, err
				}
				return result, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("securityProblem").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		resolvedID := resolveSecurityProblemID(c, args[0])
		path := fmt.Sprintf("/securityProblems/%s", url.PathEscape(resolvedID))

		var raw map[string]interface{}
		if err := c.GetV2(path, map[string]string{"fields": spFields}, &raw); err != nil {
			return err
		}

		if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
			return NewPrinterForResource("securityProblem").Print(raw)
		}

		// Re-decode to typed struct for human-readable output.
		b, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		var detail SecurityProblemDetail
		if err := json.Unmarshal(b, &detail); err != nil {
			return err
		}
		printSecurityProblemDetail(detail)
		return nil
	},
}

func init() {
	describeCmd.AddCommand(describeSecurityProblemCmd)
}
