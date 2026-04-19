package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// ProblemDetail maps the /problems/{id} API response.
type ProblemDetail struct {
	ProblemID   string  `json:"problemId"`
	DisplayID   string  `json:"displayId"`
	Title       string  `json:"title"`
	Status      string  `json:"status"`
	Severity    string  `json:"severityLevel"`
	ImpactLevel string  `json:"impactLevel"`
	StartTime   float64 `json:"startTime"`
	EndTime     float64 `json:"endTime"`
	RootCause   *struct {
		EntityID struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"entityId"`
		Name string `json:"name"`
	} `json:"rootCauseEntity"`
	ImpactedEntities []struct {
		EntityID struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"entityId"`
		Name string `json:"name"`
	} `json:"impactedEntities"`
	ManagementZones []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"managementZones"`
	EvidenceDetails struct {
		TotalCount int `json:"totalCount"`
		Details    []struct {
			DisplayName       string  `json:"displayName"`
			EvidenceType      string  `json:"evidenceType"`
			RootCauseRelevant bool    `json:"rootCauseRelevant"`
			StartTime         float64 `json:"startTime"`
			Entity            struct {
				EntityID struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"entityId"`
				Name string `json:"name"`
			} `json:"entity"`
		} `json:"details"`
	} `json:"evidenceDetails"`
	// These fields are returned as arrays (one entry per cluster/namespace).
	K8sClusterName   []string `json:"k8s.cluster.name"`
	K8sNamespaceName []string `json:"k8s.namespace.name"`
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
func printProblemDetail(d ProblemDetail) {
	fmt.Printf("%-22s %s\n", "ID:", d.ProblemID)
	fmt.Printf("%-22s %s\n", "Display ID:", d.DisplayID)
	fmt.Printf("%-22s %s\n", "Title:", d.Title)
	fmt.Printf("%-22s %s\n", "Status:", d.Status)
	fmt.Printf("%-22s %s\n", "Severity:", d.Severity)
	fmt.Printf("%-22s %s\n", "Impact:", d.ImpactLevel)
	fmt.Printf("%-22s %s\n", "Start:", fmtMillis(d.StartTime))
	if d.EndTime > 0 {
		fmt.Printf("%-22s %s\n", "End:", fmtMillis(d.EndTime))
	}
	fmt.Println()

	if d.RootCause != nil && d.RootCause.Name != "" {
		fmt.Printf("%-22s %s (%s)\n", "Root Cause:", d.RootCause.Name, d.RootCause.EntityID.Type)
	}

	if len(d.ImpactedEntities) > 0 {
		var names []string
		for _, e := range d.ImpactedEntities {
			names = append(names, e.Name)
		}
		fmt.Printf("%-22s %s\n", "Impacted Entities:", strings.Join(names, ", "))
	}

	if len(d.ManagementZones) > 0 {
		var mzNames []string
		for _, mz := range d.ManagementZones {
			mzNames = append(mzNames, mz.Name)
		}
		fmt.Printf("%-22s %s\n", "Management Zones:", strings.Join(mzNames, ", "))
	}

	if len(d.K8sClusterName) > 0 {
		fmt.Printf("%-22s %s\n", "Cluster:", strings.Join(d.K8sClusterName, ", "))
	}
	if len(d.K8sNamespaceName) > 0 {
		fmt.Printf("%-22s %s\n", "Namespace:", strings.Join(d.K8sNamespaceName, ", "))
	}

	if d.EvidenceDetails.TotalCount > 0 {
		fmt.Println()
		fmt.Printf("Evidence (%d):\n", d.EvidenceDetails.TotalCount)
		for _, ev := range d.EvidenceDetails.Details {
			rcMark := ""
			if ev.RootCauseRelevant {
				rcMark = " [root cause]"
			}
			entityName := ev.Entity.Name
			if entityName == "" {
				entityName = ev.Entity.EntityID.ID
			}
			fmt.Printf("  %-32s %-18s %s%s\n",
				ev.DisplayName,
				"["+ev.EvidenceType+"]",
				entityName,
				rcMark,
			)
		}
	}
}

var describeProblemCmd = &cobra.Command{
	Use:     "problem <problem-id>",
	Aliases: []string{"prob"},
	Short:   "Show detailed information about a specific problem",
	Long: `Show full problem details including evidence, affected entities, and root cause.
Use the problemId (UUID format) from 'dtmgd get problems', not the display ID (P-XXXXX).
Negative-integer problem IDs (e.g. -6546711275898328738_1776193140000V2) are handled
automatically — no special syntax or "--" separator is required.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/problems/%s", url.PathEscape(args[0]))

		if isMultiEnv() {
			data, err := multiExec(cfg, func(c *client.Client) (interface{}, error) {
				var result map[string]interface{}
				if err := c.GetV2(path, nil, &result); err != nil {
					return nil, err
				}
				return result, nil
			})
			if err != nil {
				return err
			}
			return NewPrinterForResource("problem").Print(data)
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := c.GetV2(path, nil, &raw); err != nil {
			return err
		}

		if outputFormat == "json" || outputFormat == "yaml" || agentMode() {
			return NewPrinterForResource("problem").Print(raw)
		}

		// Re-decode to typed struct for human-readable output.
		b, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		var detail ProblemDetail
		if err := json.Unmarshal(b, &detail); err != nil {
			return err
		}
		printProblemDetail(detail)
		return nil
	},
}

func init() {
	describeCmd.AddCommand(describeProblemCmd)
}
