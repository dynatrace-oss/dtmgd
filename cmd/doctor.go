package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// DoctorRow is one context's result in `dtmgd doctor`.
type DoctorRow struct {
	Context string `json:"context" yaml:"context" table:"CONTEXT"`
	Scope   string `json:"scope" yaml:"scope" table:"SCOPE"` // environment or cluster
	Host    string `json:"host" yaml:"host" table:"HOST"`
	EnvID   string `json:"envId" yaml:"envId" table:"ENV-ID"`
	Status  string `json:"status" yaml:"status" table:"STATUS"`
	Version string `json:"version" yaml:"version" table:"VERSION"`
	Detail  string `json:"detail" yaml:"detail" table:"DETAIL"`
}

// The values of DoctorRow.Status.
const (
	doctorOK   = "ok"
	doctorWarn = "warn"
	doctorFail = "fail"
)

// minClusterVersion is the oldest Dynatrace Managed version the Dynatrace
// Managed MCP server supports. dtmgd mirrors that server, so doctor warns
// below it.
var minClusterVersion = [3]int{1, 328, 0}

const (
	detailVersionUnreadable = "cluster version not readable with this token; assumed ≥ 1.328.0"
	detailVersionTooOld     = "below 1.328.0, the minimum the Dynatrace Managed MCP server supports"
)

func newDoctorCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check connectivity, token and cluster version of each context",
		Long: `Check every configured context, or only the one named with --context: that it
resolves, that its API token is stored, that the cluster answers, and that the
cluster version is at least 1.328.0.

When the token cannot read the cluster version, doctor tries a metrics call
instead, as the Dynatrace Managed MCP server does, and reports a warning if
that works. A cluster context (no env-id) is checked against the Cluster API:
its token must have the ServiceProviderAPI scope. Exits 1 if any context
fails.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return a.runDoctor() },
	}
	// DataExport reads the cluster version; metrics.read is the fallback
	// check; ServiceProviderAPI is a cluster context's.
	return withScopes(cmd, scopeDataExport, scopeMetricsRead, scopeServiceProviderAPI)
}

// runDoctor checks the selected contexts and prints one row for each.
func (a *App) runDoctor() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	contexts := cfg.Contexts
	if a.Opts.Context != "" {
		nc, err := cfg.GetContext(a.Opts.Context)
		if err != nil {
			return err
		}
		contexts = []config.NamedContext{*nc}
	}
	if len(contexts) == 0 {
		return errors.New("no contexts configured. Run 'dtmgd config set-context' to add one")
	}

	rows := a.checkContexts(cfg, contexts)
	var failed []string
	for _, row := range rows {
		if row.Status == doctorFail {
			failed = append(failed, row.Context)
		}
	}
	if err := a.printDoctor(rows, failed); err != nil {
		return err
	}
	if len(failed) > 0 {
		// The rows say what failed; Run only sets the exit code.
		return fmt.Errorf("%w: one or more contexts failed", errReported)
	}
	return nil
}

// checkContexts runs checkContext on each context, at most --concurrency at
// once, and returns the rows in the contexts' order. Checked one after the
// other, a config of hung hosts took one full timeout per context.
func (a *App) checkContexts(cfg *config.Config, contexts []config.NamedContext) []DoctorRow {
	limit := a.Opts.Concurrency
	if limit < 1 {
		limit = client.DefaultConcurrency
	}
	rows := make([]DoctorRow, len(contexts))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, nc := range contexts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rows[i] = a.checkContext(cfg, nc)
		}()
	}
	wg.Wait()
	return rows
}

// printDoctor prints doctor's rows for the chosen format. In agent mode a
// failed context makes the envelope ok:false, still carrying every row.
func (a *App) printDoctor(rows []DoctorRow, failed []string) error {
	if a.agentMode() {
		total := len(rows)
		ctx := &agentmode.ResponseContext{Verb: "doctor", Resource: "contexts", Total: &total}
		if len(failed) == 0 {
			return output.NewAgentPrinter(a.Out, ctx).Print(rows)
		}
		d := &agentmode.ErrorDetail{
			Code:    "error",
			Message: fmt.Sprintf("%d of %d contexts failed: %s", len(failed), total, strings.Join(failed, ", ")),
		}
		return output.WriteAgentPartial(a.Out, rows, d, ctx)
	}
	switch fm := a.format(); fm {
	case output.FormatJSON, output.FormatYAML:
		return output.NewPrinterTo(string(fm), a.Out).Print(rows)
	}
	return output.NewPrinterToWithColumns(string(a.format()), a.Out, a.columns()).PrintList(rows)
}

// checkContext runs doctor's checks against one context, in order: it
// resolves, its token is stored, and the cluster reports its version. The
// first check that fails decides the row.
func (a *App) checkContext(cfg *config.Config, nc config.NamedContext) DoctorRow {
	row := DoctorRow{Context: nc.Name, Scope: contextScope(nc.Context), Host: nc.Context.Host, EnvID: nc.Context.EnvID}
	fail := func(err error) DoctorRow {
		row.Status, row.Detail = doctorFail, failDetail(err)
		return row
	}

	resolved, err := nc.Context.Resolve()
	if err != nil {
		return fail(err)
	}
	// Show what the context actually points at once resolved.
	row.Host, row.EnvID = resolved.Host, resolved.EnvID

	if _, err := cfg.GetToken(resolved.TokenRef); err != nil {
		return fail(fmt.Errorf("no token: %w", err))
	}
	c, err := client.NewForContext(cfg, nc.Name, a.clientOptions())
	if err != nil {
		return fail(fmt.Errorf("config error: %w", err))
	}

	if nc.Context.IsCluster() {
		return checkClusterVersion(row, c)
	}
	return checkEnvironmentVersion(row, c)
}

// checkEnvironmentVersion is checkContext's last check on an environment
// context: the cluster version, or, when the token cannot read it, whether it
// can read metrics.
func checkEnvironmentVersion(row DoctorRow, c *client.Client) DoctorRow {
	version, err := c.ClusterVersion()
	switch {
	case err == nil:
		return versionRow(row, version)
	case isAuthFailure(err) && metricsReadable(c):
		// The token works but lacks the scope for the version endpoints.
		row.Status, row.Version, row.Detail = doctorWarn, "unknown", detailVersionUnreadable
		return row
	}
	row.Status, row.Detail = doctorFail, failDetail(err)
	return row
}

// checkClusterVersion is checkContext's last check on a cluster context: the
// version from the Cluster API's node list. There is no metrics fallback,
// which is an environment API call, and a failure says what a cluster context
// needs.
func checkClusterVersion(row DoctorRow, c *client.Client) DoctorRow {
	version, err := c.ClusterVersion()
	if err != nil {
		row.Status, row.Detail = doctorFail, failDetail(clusterFailure(err, row.Context))
		return row
	}
	return versionRow(row, version)
}

// clusterFailure gives a cluster context's failed check the hint a cluster
// command would give it, and a 403 the scope a cluster context's token needs.
func clusterFailure(err error, ctxName string) error {
	var apiErr *client.ErrAPI
	if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
		return withHint(err, "Permission denied. A cluster context's token needs the ServiceProviderAPI scope.")
	}
	return clusterHint(err, ctxName, fetchSpec{})
}

// versionRow is row for a cluster reporting version ("" when unknown).
func versionRow(row DoctorRow, version string) DoctorRow {
	row.Version = version
	row.Status, row.Detail = judgeVersion(version)
	if row.Version == "" {
		row.Version = "unknown"
	}
	return row
}

// metricsReadable reports whether c can list metrics: the call the Managed
// MCP server falls back to when the cluster version is not readable.
func metricsReadable(c *client.Client) bool {
	var m map[string]interface{}
	return c.GetV2("/metrics", map[string]string{"pageSize": "1"}, &m) == nil
}

// isAuthFailure reports whether err is an HTTP 401 or 403.
func isAuthFailure(err error) bool {
	var apiErr *client.ErrAPI
	return errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403)
}

// judgeVersion is the status and detail for a cluster reporting version v.
func judgeVersion(v string) (status, detail string) {
	parts, ok := client.ParseClusterVersion(v)
	if !ok {
		return doctorWarn, fmt.Sprintf("cluster version %q not recognised", v)
	}
	if slices.Compare(parts[:], minClusterVersion[:]) < 0 {
		return doctorWarn, detailVersionTooOld
	}
	return doctorOK, ""
}

// failDetail is err followed by the first line of its hint, if it has one. A
// first line that only introduces the next ("Store it with:") takes the next
// line along, so the row says what to run.
func failDetail(err error) string {
	lines := strings.Split(hintFor(err), "\n")
	hint := strings.TrimSpace(lines[0])
	if strings.HasSuffix(hint, ":") && len(lines) > 1 {
		hint += " " + strings.TrimSpace(lines[1])
	}
	if hint == "" {
		return err.Error()
	}
	return strings.TrimSuffix(err.Error(), ".") + ". " + hint
}
