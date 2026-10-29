package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// DoctorRow is one context's result in `dtmgd doctor`.
type DoctorRow struct {
	Context string `json:"context" yaml:"context" table:"CONTEXT"`
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
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check connectivity, token and cluster version of each context",
		Long: `Check every configured context, or only the one named with --context: that it
resolves, that its API token is stored, that the cluster answers, and that the
cluster version is at least 1.328.0.

When the token cannot read the cluster version, doctor tries a metrics call
instead, as the Dynatrace Managed MCP server does, and reports a warning if
that works. Exits 1 if any context fails.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return a.runDoctor() },
	}
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

	rows := make([]DoctorRow, 0, len(contexts))
	var failed []string
	for _, nc := range contexts {
		row := a.checkContext(cfg, nc)
		if row.Status == doctorFail {
			failed = append(failed, row.Context)
		}
		rows = append(rows, row)
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
// first check that fails decides the row. `get environments` shares it.
func (a *App) checkContext(cfg *config.Config, nc config.NamedContext) DoctorRow {
	row := DoctorRow{Context: nc.Name, Host: nc.Context.Host, EnvID: nc.Context.EnvID}
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

	version, err := c.ClusterVersion()
	switch {
	case err == nil:
		row.Version = version
		row.Status, row.Detail = judgeVersion(version)
		if row.Version == "" {
			row.Version = "unknown"
		}
	case isAuthFailure(err) && metricsReadable(c):
		// The token works but lacks the scope for the version endpoints.
		row.Status, row.Version, row.Detail = doctorWarn, "unknown", detailVersionUnreadable
	default:
		return fail(err)
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
	parts, ok := parseClusterVersion(v)
	if !ok {
		return doctorWarn, fmt.Sprintf("cluster version %q not recognised", v)
	}
	if slices.Compare(parts[:], minClusterVersion[:]) < 0 {
		return doctorWarn, detailVersionTooOld
	}
	return doctorOK, ""
}

// parseClusterVersion returns the first three numeric parts of a cluster
// version such as "1.330.18.20251017-123456".
func parseClusterVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.SplitN(v, ".", 4)
	if len(parts) < 3 {
		return out, false
	}
	for i := range out {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// failDetail is err followed by the first line of its hint, if it has one. A
// first line that only introduces the next ("Store it with:") takes the next
// line along, so the row says what to run.
func failDetail(err error) string {
	lines := strings.Split(client.Diagnose(err), "\n")
	hint := strings.TrimSpace(lines[0])
	if strings.HasSuffix(hint, ":") && len(lines) > 1 {
		hint += " " + strings.TrimSpace(lines[1])
	}
	if hint == "" {
		return err.Error()
	}
	return strings.TrimSuffix(err.Error(), ".") + ". " + hint
}
