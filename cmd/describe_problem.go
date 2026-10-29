package cmd

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ProblemDetail maps the /problems/{id} API response.
type ProblemDetail struct {
	ProblemID        string                  `json:"problemId"`
	DisplayID        string                  `json:"displayId"`
	Title            string                  `json:"title"`
	Status           string                  `json:"status"`
	Severity         string                  `json:"severityLevel"`
	ImpactLevel      string                  `json:"impactLevel"`
	StartTime        float64                 `json:"startTime"`
	EndTime          float64                 `json:"endTime"`
	RootCause        *problemEntity          `json:"rootCauseEntity"`
	ImpactedEntities []problemEntity         `json:"impactedEntities"`
	ManagementZones  []problemManagementZone `json:"managementZones"`
	EvidenceDetails  problemEvidenceDetails  `json:"evidenceDetails"`
	// These fields are returned as arrays (one entry per cluster/namespace).
	K8sClusterName   []string `json:"k8s.cluster.name"`
	K8sNamespaceName []string `json:"k8s.namespace.name"`
}

// problemEntityID identifies an entity a problem refers to.
type problemEntityID struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// problemEntity is an entity a problem names: its root cause, an impacted
// entity, or the entity behind a piece of evidence.
type problemEntity struct {
	EntityID problemEntityID `json:"entityId"`
	Name     string          `json:"name"`
}

// problemManagementZone is a management zone a problem belongs to.
type problemManagementZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// problemEvidenceDetails is the evidence the problem was detected on.
type problemEvidenceDetails struct {
	TotalCount int               `json:"totalCount"`
	Details    []problemEvidence `json:"details"`
}

// problemEvidence is one piece of evidence for a problem.
type problemEvidence struct {
	DisplayName       string        `json:"displayName"`
	EvidenceType      string        `json:"evidenceType"`
	RootCauseRelevant bool          `json:"rootCauseRelevant"`
	StartTime         float64       `json:"startTime"`
	Entity            problemEntity `json:"entity"`
}

// fmtMillis converts a Unix-millisecond float64 timestamp to a human-readable UTC string.
func fmtMillis(ms float64) string {
	if ms <= 0 {
		return "—"
	}
	t := time.UnixMilli(int64(ms)).UTC()
	return t.Format("2006-01-02 15:04:05 UTC")
}

// printProblemDetail renders a ProblemDetail as a human-readable summary.
func printProblemDetail(w io.Writer, d ProblemDetail) {
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "ID:", d.ProblemID)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Display ID:", d.DisplayID)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Title:", d.Title)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Status:", d.Status)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Severity:", d.Severity)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Impact:", d.ImpactLevel)
	_, _ = fmt.Fprintf(w, "%-22s %s\n", "Start:", fmtMillis(d.StartTime))
	if d.EndTime > 0 {
		_, _ = fmt.Fprintf(w, "%-22s %s\n", "End:", fmtMillis(d.EndTime))
	}
	_, _ = fmt.Fprintln(w)

	if d.RootCause != nil && d.RootCause.Name != "" {
		_, _ = fmt.Fprintf(w, "%-22s %s (%s)\n", "Root Cause:", d.RootCause.Name, d.RootCause.EntityID.Type)
	}

	if len(d.ImpactedEntities) > 0 {
		var names []string
		for _, e := range d.ImpactedEntities {
			names = append(names, e.Name)
		}
		_, _ = fmt.Fprintf(w, "%-22s %s\n", "Impacted Entities:", strings.Join(names, ", "))
	}

	if len(d.ManagementZones) > 0 {
		var mzNames []string
		for _, mz := range d.ManagementZones {
			mzNames = append(mzNames, mz.Name)
		}
		_, _ = fmt.Fprintf(w, "%-22s %s\n", "Management Zones:", strings.Join(mzNames, ", "))
	}

	if len(d.K8sClusterName) > 0 {
		_, _ = fmt.Fprintf(w, "%-22s %s\n", "Cluster:", strings.Join(d.K8sClusterName, ", "))
	}
	if len(d.K8sNamespaceName) > 0 {
		_, _ = fmt.Fprintf(w, "%-22s %s\n", "Namespace:", strings.Join(d.K8sNamespaceName, ", "))
	}

	if d.EvidenceDetails.TotalCount > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "Evidence (%d):\n", d.EvidenceDetails.TotalCount)
		for _, ev := range d.EvidenceDetails.Details {
			rcMark := ""
			if ev.RootCauseRelevant {
				rcMark = " [root cause]"
			}
			entityName := ev.Entity.Name
			if entityName == "" {
				entityName = ev.Entity.EntityID.ID
			}
			_, _ = fmt.Fprintf(w, "  %-32s %-18s %s%s\n",
				ev.DisplayName,
				"["+ev.EvidenceType+"]",
				entityName,
				rcMark,
			)
		}
	}
}

func newDescribeProblemCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "problem <problem-id>",
		Aliases: []string{"prob"},
		Short:   "Show detailed information about a specific problem",
		Long: `Show full problem details including evidence, affected entities, and root cause.
Use the problemId (UUID format) from 'dtmgd get problems', not the display ID (P-XXXXX).
Negative-integer problem IDs (e.g. -6546711275898328738_1776193140000V2) are handled
automatically — no special syntax or "--" separator is required.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := fmt.Sprintf("/problems/%s", url.PathEscape(args[0]))
			return a.runFetch(fetchSpec{
				Verb:     "describe",
				Resource: "problem",
				Fetch:    describeFetch(path, nil),
				Human:    detailHuman(printProblemDetail),
			})
		},
	}
	return withScopes(cmd, scopeProblemsRead)
}
