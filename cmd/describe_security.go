package cmd

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// displayIDRegexp matches security problem display IDs of the form S-NNN (case-insensitive).
var displayIDRegexp = regexp.MustCompile(`(?i)^s-\d+$`)

// resolveLookupPageSize is the page size of a display-ID lookup: the most
// /securityProblems returns per page, so a lookup takes as few requests as
// possible.
const resolveLookupPageSize = "500"

// resolveSecurityProblemID converts a display ID (e.g. "S-281") to the internal
// ID by listing the environment's security problems, stopping at the first
// page that has it; display IDs are per environment, so c must be the
// environment's own client. Any other id is returned unchanged, without a
// request. A failed listing returns its error, and a display ID no security
// problem has returns a not-found error (exit code 4).
func resolveSecurityProblemID(c *client.Client, id string, verbose int, errw io.Writer) (string, error) {
	if !displayIDRegexp.MatchString(id) {
		return id, nil
	}
	if verbose > 0 {
		_, _ = fmt.Fprintf(errw, "Resolving %s by listing security problems…\n", id)
	}
	// The first request sets the page size; later ones send only
	// nextPageKey, as the API requires.
	params := map[string]string{"pageSize": resolveLookupPageSize}
	for {
		var page struct {
			SecurityProblems []SecurityProblemEntry `json:"securityProblems"`
			NextPageKey      string                 `json:"nextPageKey"`
		}
		if err := c.GetV2("/securityProblems", params, &page); err != nil {
			return "", err
		}
		for _, sp := range page.SecurityProblems {
			if strings.EqualFold(sp.DisplayID, id) {
				return sp.SecurityProblemID, nil
			}
		}
		if page.NextPageKey == "" {
			return "", &lookupNotFoundError{msg: fmt.Sprintf("security problem %s not found", id)}
		}
		params = map[string]string{"nextPageKey": page.NextPageKey}
	}
}

// lookupNotFoundError is a display-ID lookup that found nothing. It unwraps
// to an API 404, so exit codes (4), agent error codes (not_found) and hints
// treat it as the API's own 404 for an unknown ID, while the message says
// what was looked for rather than claiming the API returned 404.
type lookupNotFoundError struct{ msg string }

func (e *lookupNotFoundError) Error() string { return e.msg }
func (e *lookupNotFoundError) Unwrap() error { return &client.ErrAPI{StatusCode: 404} }

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
func printSecurityProblemDetail(w io.Writer, d SecurityProblemDetail) {
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

	_, _ = fmt.Fprintf(w, "%-22s %s\n", "ID:", d.SecurityProblemID)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Display ID:", d.DisplayID)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Title:", d.Title)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "CVE IDs:", strings.Join(d.CVEIds, ", "))
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Package:", pkg)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Technology:", d.Technology)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Status:", d.Status)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%-22s %s (score: %.1f)\n", "Risk Level:", ra.RiskLevel, ra.RiskScore)
	_, _ = fmt.Fprintf(w, "%-22s %s (base score: %.1f)\n", "CVSS Base Level:", ra.BaseRiskLevel, ra.BaseRiskScore)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "CVSS Vector:", ra.BaseRiskVector)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Public Exploit:", ra.PublicExploit)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Data Assets:", ra.DataAssets)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Exposure:", ra.Exposure)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "%-22s %d\n", "Affected Processes:", affected)
	if len(mzNames) > 0 {
		_, _ = fmt.Fprintf(w, "%-22s %s\n", "Management Zones:", strings.Join(mzNames, ", "))
	}
	if shortDesc != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "Description: %s\n", shortDesc)
	}
	if d.URL != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "URL: %s\n", d.URL)
	}
}

func newDescribeSecurityProblemCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "security-problem <security-problem-id>",
		Aliases: []string{"sp", "vuln"},
		Short:   "Show detailed information about a security vulnerability",
		Long: `Show full CVE details, risk assessment, affected entities, and remediation hints
for a specific security problem.

Accepts either the display ID (e.g. S-281) from 'dtmgd get security-problems'
or the internal UUID. Display IDs are resolved automatically.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"fields": "+riskAssessment,+managementZones,+codeLevelVulnerabilityDetails,+vulnerableComponents,+affectedEntities,+exposedEntities,+description"}
			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "security-problem",
				// A display ID (S-NNN) is resolved with each environment's own
				// client: display IDs are local to an environment.
				Fetch: func(c *client.Client, limit int) (fetched, error) {
					id, err := resolveSecurityProblemID(c, args[0], a.Opts.Verbose, a.Err)
					if err != nil {
						return fetched{}, err
					}
					path := fmt.Sprintf("/securityProblems/%s", url.PathEscape(id))
					return describeFetch(path, params)(c, limit)
				},
				Human: detailHuman(printSecurityProblemDetail),
				Link:  uiItem(uiVulnerability, "securityProblemId"),
			})
		},
	}
	return withScopes(cmd, scopeSecurityProblemsRead)
}
