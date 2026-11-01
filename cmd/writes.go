package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// A planned write's action, and what a write result reports. A dry run
// reports the planned action; a run that wrote reports what happened.
const (
	actionCreate    = "create"
	actionUpdate    = "update"
	actionDelete    = "delete"
	actionUnchanged = "unchanged"

	actionCreated   = "created"
	actionUpdated   = "updated"
	actionDeleted   = "deleted"
	actionValidated = "validated"
	actionFailed    = "failed"
)

// validateOnlyFloor is the first cluster version whose PUT honours
// validateOnly. An older one ignores the parameter and saves the change, so
// a validation must never reach it. Creates get the same floor: when POST
// started honouring it was not confirmed, and one floor is simpler.
var validateOnlyFloor = [3]int{1, 288, 0}

// writeResult is the outcome of one object's write, as every format shows it.
type writeResult struct {
	Context  string      `json:"context" yaml:"context"`
	Action   string      `json:"action" yaml:"action"`
	ObjectID string      `json:"objectId,omitempty" yaml:"objectId,omitempty"`
	SchemaID string      `json:"schemaId" yaml:"schemaId"`
	Scope    string      `json:"scope" yaml:"scope"`
	Summary  string      `json:"summary,omitempty" yaml:"summary,omitempty"`
	Error    *writeError `json:"error" yaml:"error"`
	// Current and Value are a dry run's planned change: the stored value and
	// the one that would be written.
	Current map[string]interface{} `json:"current,omitempty" yaml:"current,omitempty"`
	Value   map[string]interface{} `json:"value,omitempty" yaml:"value,omitempty"`
}

// writeError is why one object's write failed, in the API's own terms.
type writeError struct {
	Code                 int                   `json:"code" yaml:"code"`
	Message              string                `json:"message" yaml:"message"`
	ConstraintViolations []constraintViolation `json:"constraintViolations,omitempty" yaml:"constraintViolations,omitempty"`
}

// constraintViolation is one rule a value broke.
type constraintViolation struct {
	Path    string `json:"path" yaml:"path"`
	Message string `json:"message" yaml:"message"`
}

// writeReport is what every write command prints: the agent envelope's
// result, the json or yaml body, the rows of a table.
type writeReport struct {
	DryRun       bool          `json:"dryRun" yaml:"dryRun"`
	ValidateOnly bool          `json:"validateOnly" yaml:"validateOnly"`
	Results      []writeResult `json:"results" yaml:"results"`
}

// plannedWrite is one object's write as the GET-only plan resolved it.
type plannedWrite struct {
	Op                                                          config.Operation // empty for unchanged
	Action                                                      string
	ObjectID, SchemaID, Scope, ExternalID, UpdateToken, Summary string
	Value, Current                                              map[string]interface{}
}

// writeSpec describes one write command for runWrites.
type writeSpec struct {
	Verb, Resource string
	DryRun         bool
	ValidateOnly   bool
	// MultiEnv allows several contexts (--env "a;b", ALL_ENVIRONMENTS);
	// without it they are a usage error.
	MultiEnv bool
	// Plan resolves, with GETs only, what the command would write in the
	// context named name.
	Plan func(name string, c *client.Client) ([]plannedWrite, error)
}

// SafetyError is a write the context's safety-level does not allow. Nothing
// was sent; dtmgd exits 6.
type SafetyError struct {
	Context string
	Level   config.SafetyLevel
	Op      config.Operation
}

func (e *SafetyError) Error() string {
	return fmt.Sprintf("context %q is %s: it does not allow %s", e.Context, e.Level, e.Op)
}

// safetyBlocked is e with its hint: the command that raises the level.
func safetyBlocked(e *SafetyError) error {
	return withHint(e, fmt.Sprintf("Allow writes through this context with: dtmgd config set-context %s --safety-level readwrite-all (the token still needs settings.write)", e.Context))
}

// opAnyWrite names "any write" in the SafetyError of a readonly context,
// which is refused before dtmgd knows which writes a command would make.
const opAnyWrite config.Operation = "writes"

// contextPlan is what the plan pass learned about one context.
type contextPlan struct {
	writes     []plannedWrite
	version    string
	versionErr error
}

// runWrites is the one path every write command takes. It plans every
// selected context's writes with GETs only, checks every context's write
// level (and, for --validate-only, the cluster version) before writing to
// any of them, then writes, unless --dry-run, and reports. A write blocked
// in one context never leaves the others half changed.
func (a *App) runWrites(s writeSpec) error {
	if s.DryRun && s.ValidateOnly {
		return &UsageError{Err: errors.New("--dry-run and --validate-only cannot be combined: --dry-run sends no write at all")}
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	if err := a.checkWriteContexts(cfg, s); err != nil {
		return err
	}
	levels, err := a.contextLevels(cfg)
	if err != nil {
		return err
	}
	names, plans, err := a.planWrites(cfg, s)
	if err != nil {
		return err
	}
	if err := s.checkPlans(names, plans, levels); err != nil {
		return err
	}
	rep := writeReport{DryRun: s.DryRun, ValidateOnly: s.ValidateOnly}
	if s.DryRun {
		rep.Results = plannedResults(names, plans)
	} else if rep.Results, err = a.executeWrites(cfg, s, names, plans); err != nil {
		return err
	}
	return a.renderWriteReport(s, rep)
}

// envSpec is the --env value MultiRequest gets: the list for a
// multi-environment run, "" (the current context) otherwise.
func (a *App) envSpec() string {
	if a.isMultiEnv() {
		return a.Opts.Env
	}
	return ""
}

// checkWriteContexts refuses, before any request, cluster contexts and,
// for a single-context command, several contexts.
func (a *App) checkWriteContexts(cfg *config.Config, s writeSpec) error {
	if err := a.checkEnvVarContext(cfg); err != nil {
		return err
	}
	if err := a.checkEnvironmentContexts(cfg, fetchSpec{Verb: s.Verb, Resource: s.Resource}); err != nil {
		return err
	}
	if !s.MultiEnv && a.isMultiEnv() {
		return &UsageError{Err: fmt.Errorf("\"dtmgd %s %s\" changes one object in one context; give one context with --context", s.Verb, s.Resource)}
	}
	return nil
}

// contextLevels reads the write level of every selected context. An
// unknown level is a usage error, before any request.
func (a *App) contextLevels(cfg *config.Config) (map[string]config.SafetyLevel, error) {
	contexts, err := client.ResolveContexts(cfg, a.envSpec())
	if err != nil {
		return nil, err
	}
	levels := make(map[string]config.SafetyLevel, len(contexts))
	for _, nc := range contexts {
		level, err := nc.Context.Level()
		if err != nil {
			return nil, &UsageError{Err: fmt.Errorf("context %q: %w", nc.Name, err)}
		}
		levels[nc.Name] = level
	}
	return levels, nil
}

// planWrites runs s.Plan in every selected context, with the cluster
// version too for --validate-only. A readonly context is refused before
// any request: it allows no write at all, whatever the plan would find.
func (a *App) planWrites(cfg *config.Config, s writeSpec) ([]string, map[string]contextPlan, error) {
	if err := a.refuseReadonly(cfg, s); err != nil {
		return nil, nil, err
	}
	results, err := client.MultiRequestNamed(cfg, a.envSpec(), a.Opts.Concurrency, a.clientOptions(), func(name string, c *client.Client) (interface{}, error) {
		var p contextPlan
		var err error
		if p.writes, err = s.Plan(name, c); err != nil {
			return nil, err
		}
		if s.ValidateOnly {
			p.version, p.versionErr = c.ClusterVersion()
		}
		return p, nil
	})
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(results))
	plans := make(map[string]contextPlan, len(results))
	var failed []client.EnvResult
	for _, r := range results {
		names = append(names, r.Name)
		if r.Error != nil {
			failed = append(failed, r)
			continue
		}
		plans[r.Name] = r.Data.(contextPlan)
	}
	switch {
	case len(failed) == 1 && len(results) == 1:
		return nil, nil, failed[0].Error
	case len(failed) > 0:
		return nil, nil, &client.PartialFailureError{Failed: failed, Total: len(results)}
	}
	return names, plans, nil
}

// refuseReadonly blocks a write run through a readonly context before any
// request. A dry run and a validation save nothing, so they pass.
func (a *App) refuseReadonly(cfg *config.Config, s writeSpec) error {
	if s.DryRun || s.ValidateOnly {
		return nil
	}
	contexts, err := client.ResolveContexts(cfg, a.envSpec())
	if err != nil {
		return err
	}
	for _, nc := range contexts {
		if level, _ := nc.Context.Level(); level == config.SafetyReadOnly {
			return safetyBlocked(&SafetyError{Context: nc.Name, Level: level, Op: opAnyWrite})
		}
	}
	return nil
}

// checkPlans checks every context's planned writes against its level, or,
// for --validate-only (which saves nothing), the cluster version.
func (s writeSpec) checkPlans(names []string, plans map[string]contextPlan, levels map[string]config.SafetyLevel) error {
	if s.DryRun {
		return nil
	}
	for _, name := range names {
		if err := s.checkPlan(name, plans[name], levels[name]); err != nil {
			return err
		}
	}
	return nil
}

// checkPlan is checkPlans for one context.
func (s writeSpec) checkPlan(name string, p contextPlan, level config.SafetyLevel) error {
	if s.ValidateOnly {
		if !hasWrites(p.writes) {
			return nil
		}
		return versionGate(name, p)
	}
	for _, w := range p.writes {
		if w.Op != "" && !level.Allows(w.Op) {
			return safetyBlocked(&SafetyError{Context: name, Level: level, Op: w.Op})
		}
	}
	return nil
}

// hasWrites reports whether ws would send anything.
func hasWrites(ws []plannedWrite) bool {
	for _, w := range ws {
		if w.Op != "" {
			return true
		}
	}
	return false
}

// versionGate refuses a validation the context's cluster would save: one
// below validateOnlyFloor, or one whose version cannot be read.
func versionGate(name string, p contextPlan) error {
	const dryRunHint = "--dry-run shows the planned change with GETs only and needs only settings.read"
	if p.versionErr != nil {
		return withHint(&UsageError{Err: fmt.Errorf("context %q: cannot read the cluster version, which --validate-only checks first: %w", name, p.versionErr)},
			"Reading the version needs the DataExport scope on an environment token. "+dryRunHint)
	}
	ok, known := client.VersionAtLeast(p.version, validateOnlyFloor)
	switch {
	case !known:
		return withHint(&UsageError{Err: fmt.Errorf("context %q: cannot read the cluster version %q, which --validate-only checks first", name, p.version)}, dryRunHint)
	case !ok:
		return withHint(&UsageError{Err: fmt.Errorf("context %q runs Dynatrace Managed %s: before 1.288 a cluster ignores validateOnly and would save the change", name, p.version)}, dryRunHint)
	}
	return nil
}

// plannedResults is a dry run's report: each planned write with its
// current and new value.
func plannedResults(names []string, plans map[string]contextPlan) []writeResult {
	var results []writeResult
	for _, name := range names {
		for _, w := range plans[name].writes {
			r := w.result(name)
			r.Action, r.Current, r.Value = w.Action, w.Current, w.Value
			results = append(results, r)
		}
	}
	return results
}

// result is w's report entry, without its outcome.
func (w plannedWrite) result(context string) writeResult {
	return writeResult{Context: context, ObjectID: w.ObjectID, SchemaID: w.SchemaID, Scope: w.Scope, Summary: w.Summary}
}

// executeWrites sends every context's planned writes, one request per
// object: a batch create is not atomic, and per-object requests give every
// object its own outcome.
func (a *App) executeWrites(cfg *config.Config, s writeSpec, names []string, plans map[string]contextPlan) ([]writeResult, error) {
	results, err := client.MultiRequestNamed(cfg, a.envSpec(), a.Opts.Concurrency, a.clientOptions(), func(name string, c *client.Client) (interface{}, error) {
		var out []writeResult
		for _, w := range plans[name].writes {
			r := executeWrite(c, w, s.ValidateOnly)
			r.Context = name
			out = append(out, r)
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	byName := make(map[string][]writeResult, len(results))
	for _, r := range results {
		if r.Error != nil {
			byName[r.Name] = failAll(r.Name, plans[r.Name].writes, r.Error)
			continue
		}
		byName[r.Name], _ = r.Data.([]writeResult)
	}
	var all []writeResult
	for _, name := range names {
		all = append(all, byName[name]...)
	}
	return all, nil
}

// failAll reports every write of a context that could not be reached.
func failAll(context string, ws []plannedWrite, err error) []writeResult {
	out := make([]writeResult, 0, len(ws))
	for _, w := range ws {
		r := w.result(context)
		r.Action, r.Error = actionFailed, toWriteError(err)
		out = append(out, r)
	}
	return out
}

// settingsWriteItem is one item of a write's answer: a POST's per-object
// results, or a PUT's single result.
type settingsWriteItem struct {
	Code     int         `json:"code"`
	ObjectID string      `json:"objectId"`
	Error    *writeError `json:"error"`
}

// executeWrite sends one planned write and reports its outcome.
func executeWrite(c *client.Client, w plannedWrite, validateOnly bool) writeResult {
	r := w.result("")
	var params map[string]string
	if validateOnly {
		params = map[string]string{"validateOnly": "true"}
	}
	var err error
	var item settingsWriteItem
	done := map[string]string{actionCreate: actionCreated, actionUpdate: actionUpdated, actionDelete: actionDeleted}[w.Action]
	switch w.Action {
	case actionCreate:
		obj := map[string]interface{}{"schemaId": w.SchemaID, "scope": w.Scope, "value": w.Value}
		if w.ExternalID != "" {
			obj["externalId"] = w.ExternalID
		}
		var items []settingsWriteItem
		err = c.PostV2(settingsObjectsPath, params, []interface{}{obj}, &items)
		if len(items) > 0 {
			item = items[0]
		}
	case actionUpdate:
		body := map[string]interface{}{"value": w.Value}
		if w.UpdateToken != "" {
			body["updateToken"] = w.UpdateToken
		}
		err = c.PutV2(settingsObjectsPath+"/"+url.PathEscape(w.ObjectID), params, body, &item)
	case actionDelete:
		err = c.DeleteV2(settingsObjectsPath+"/"+url.PathEscape(w.ObjectID), deleteParams(w.UpdateToken))
	default:
		r.Action = actionUnchanged
		return r
	}
	return finishWrite(r, err, item, done, validateOnly)
}

// deleteParams is a DELETE's query: the update token, when there is one.
func deleteParams(token string) map[string]string {
	if token == "" {
		return nil
	}
	return map[string]string{"updateToken": token}
}

// finishWrite fills r from a write's answer.
func finishWrite(r writeResult, err error, item settingsWriteItem, done string, validateOnly bool) writeResult {
	switch {
	case err != nil:
		r.Action, r.Error = actionFailed, toWriteError(err)
	case item.Code >= 400:
		r.Action, r.Error = actionFailed, item.Error
		if r.Error == nil {
			r.Error = &writeError{Code: item.Code, Message: fmt.Sprintf("HTTP %d", item.Code)}
		}
	case validateOnly:
		r.Action = actionValidated
	default:
		r.Action = done
		if item.ObjectID != "" {
			r.ObjectID = item.ObjectID
		}
	}
	return r
}

// toWriteError reads why a write failed from its error: the per-item array
// or the standard error envelope of an API error, else the error's text.
func toWriteError(err error) *writeError {
	var apiErr *client.ErrAPI
	if !errors.As(err, &apiErr) {
		return &writeError{Message: err.Error()}
	}
	var items []settingsWriteItem
	if json.Unmarshal([]byte(apiErr.Body), &items) == nil && len(items) > 0 && items[0].Error != nil {
		return items[0].Error
	}
	var envelope struct {
		Error *writeError `json:"error"`
	}
	if json.Unmarshal([]byte(apiErr.Body), &envelope) == nil && envelope.Error != nil {
		return envelope.Error
	}
	return &writeError{Code: apiErr.StatusCode, Message: apiErr.Error()}
}

// writeRow is a write result's table row.
type writeRow struct {
	Action   string `table:"ACTION,status"`
	ObjectID string `table:"OBJECT-ID"`
	Schema   string `table:"SCHEMA"`
	Scope    string `table:"SCOPE"`
	Summary  string `table:"SUMMARY,max=60"`
}

// writeContextRow is writeRow with the context, for several contexts.
type writeContextRow struct {
	Context  string `table:"CONTEXT"`
	Action   string `table:"ACTION,status"`
	ObjectID string `table:"OBJECT-ID"`
	Schema   string `table:"SCHEMA"`
	Scope    string `table:"SCOPE"`
	Summary  string `table:"SUMMARY,max=60"`
}

// renderWriteReport prints rep in the chosen format. Any failed object
// makes the command fail, after every result has been printed.
func (a *App) renderWriteReport(s writeSpec, rep writeReport) error {
	failErr := writeFailures(rep)
	hint := writeHint(rep)
	if a.agentMode() {
		ctx := &agentmode.ResponseContext{Verb: s.Verb, Resource: s.Resource}
		if failErr == nil {
			return output.NewAgentPrinter(a.Out, ctx).Print(rep)
		}
		if err := output.WriteAgentPartial(a.Out, rep, errorDetail(failErr, hint), ctx); err != nil {
			return err
		}
		return fmt.Errorf("%w: %w", errReported, failErr)
	}
	if err := a.printWriteReport(rep); err != nil {
		return err
	}
	if failErr == nil {
		return nil
	}
	a.printWriteFailures(rep)
	if hint != "" {
		output.PrintHint("%s", hint)
	}
	return fmt.Errorf("%w: %w", errReported, failErr)
}

// printWriteReport prints rep as json or yaml, or as a table for a person.
func (a *App) printWriteReport(rep writeReport) error {
	switch fm := a.format(); fm {
	case output.FormatJSON, output.FormatYAML:
		return output.NewPrinterTo(string(fm), a.Out).Print(rep)
	}
	p := output.NewPrinterToWithColumns(string(a.format()), a.Out, a.columns())
	if !a.isMultiEnv() {
		rows := make([]writeRow, 0, len(rep.Results))
		for _, r := range rep.Results {
			rows = append(rows, writeRow{Action: r.Action, ObjectID: r.ObjectID, Schema: r.SchemaID, Scope: r.Scope, Summary: r.Summary})
		}
		return p.PrintList(rows)
	}
	rows := make([]writeContextRow, 0, len(rep.Results))
	for _, r := range rep.Results {
		rows = append(rows, writeContextRow{Context: r.Context, Action: r.Action, ObjectID: r.ObjectID, Schema: r.SchemaID, Scope: r.Scope, Summary: r.Summary})
	}
	return p.PrintList(rows)
}

// printWriteFailures names every failed object on stderr, with the rules
// its value broke.
func (a *App) printWriteFailures(rep writeReport) {
	for _, r := range rep.Results {
		if r.Action != actionFailed || r.Error == nil {
			continue
		}
		target := r.ObjectID
		if target == "" {
			target = r.Scope
		}
		output.PrintHumanError("%s %s %s: %s", r.Context, r.SchemaID, target, r.Error.Message)
		for _, v := range r.Error.ConstraintViolations {
			_, _ = fmt.Fprintf(a.Err, "    %s: %s\n", v.Path, v.Message)
		}
	}
}

// writeFailures is the error a report with failed objects ends in: one
// failure per object, so the exit code is that of the API error when every
// object failed the same way, else 1. nil when nothing failed.
func writeFailures(rep writeReport) error {
	var failed []client.EnvResult
	for _, r := range rep.Results {
		if r.Action != actionFailed {
			continue
		}
		err := errors.New("write failed")
		if r.Error != nil {
			err = errors.New(r.Error.Message)
			if r.Error.Code >= 400 {
				err = client.APIError(r.Error.Code, r.Error.Message)
			}
		}
		failed = append(failed, client.EnvResult{Name: r.Context, Error: err})
	}
	if len(failed) == 0 {
		return nil
	}
	return &client.PartialFailureError{Failed: failed, Total: len(rep.Results)}
}

// writeHint is the next step after failed writes, or "".
func writeHint(rep writeReport) string {
	for _, r := range rep.Results {
		if r.Error != nil && r.Error.Code == 409 {
			return "The object changed after it was read: re-read it (dtmgd describe settings <object-id> -o yaml) and apply again."
		}
	}
	return ""
}
