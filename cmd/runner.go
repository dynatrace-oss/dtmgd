package cmd

import (
	"fmt"
	"io"
	"strconv"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// agentDefaultLimit caps lists in agent mode when --limit is not given, so an
// agent asking for "the problems" gets a page it can read rather than every
// problem in the environment (dtctl uses the same default).
const agentDefaultLimit = 50

// fetched is one environment's result, in the API's own shape.
type fetched struct {
	Body    interface{} // printed as-is for json, yaml and agent output
	Total   int         // how many exist, or -1 when unknown
	HasMore bool        // more exist than Body holds

	// Warnings are problems that did not fail the fetch, such as a failed
	// name lookup for a table. renderHuman prints each to stderr after the
	// result; json, yaml and agent output do not show them.
	Warnings []string
}

// fetchSpec describes one command for runFetch: how to fetch a result from
// one environment, and how to show it to a person.
type fetchSpec struct {
	Verb, Resource string // agent context and messages, e.g. "get", "problems"

	// Fetch gets one environment's result. limit is the effective item limit
	// (0 = none): --limit, or the agent default for lists.
	Fetch func(c *client.Client, limit int) (fetched, error)

	// Rows turns one environment's Body into table rows ([]T of a struct with
	// table tags) and their count. With neither Rows nor Human, table mode
	// prints the Body as YAML.
	Rows func(body interface{}) (rows interface{}, n int, err error)

	// Human renders one environment's Body for a person, overriding Rows.
	Human func(w io.Writer, body interface{}) error

	Limit int    // --limit; 0 = none
	List  bool   // a list: the agent default limit applies
	Empty string // stderr note when Rows finds nothing
}

// runFetch is the one path every get, describe and query command takes:
// fetch from the environments the flags select, then render for the chosen
// format. It is what makes --limit, truncation notes, empty results,
// multi-environment output and partial failures behave the same everywhere.
func (a *App) runFetch(s fetchSpec) error {
	if err := limitFlag(s.Limit); err != nil {
		return err
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	limit := s.Limit
	if limit == 0 && s.List && a.agentMode() {
		limit = agentDefaultLimit
	}
	spec := ""
	if a.isMultiEnv() {
		spec = a.Opts.Env
	}
	results, err := client.MultiRequest(cfg, spec, a.Opts.Concurrency, a.clientOptions(), func(c *client.Client) (interface{}, error) {
		return s.Fetch(c, limit)
	})
	if err != nil {
		return err
	}
	if len(results) == 1 {
		if results[0].Error != nil {
			return results[0].Error
		}
		return a.renderOne(s, results[0].Data.(fetched))
	}
	return a.renderMulti(s, results)
}

// renderOne renders a single environment's result.
func (a *App) renderOne(s fetchSpec, f fetched) error {
	if a.agentMode() {
		return output.NewAgentPrinter(a.Out, a.responseContext(s, f)).Print(f.Body)
	}
	switch fm := a.format(); fm {
	case output.FormatJSON, output.FormatYAML:
		return output.NewPrinterTo(string(fm), a.Out).Print(f.Body)
	}
	return a.renderHuman(s, f)
}

// renderMulti renders several environments' results: a map keyed by context
// name for json, yaml and agent output (with {"error": …} for a failed
// environment), or a section per environment for a person. Any failure makes
// the call fail, after everything that did arrive has been printed.
func (a *App) renderMulti(s fetchSpec, results []client.EnvResult) error {
	merged, partial := mergeResults(results)
	if a.agentMode() {
		return a.renderMultiAgent(s, results, merged, partial)
	}

	switch fm := a.format(); fm {
	case output.FormatJSON, output.FormatYAML:
		if err := output.NewPrinterTo(string(fm), a.Out).Print(merged); err != nil {
			return err
		}
		return partial
	}
	return a.renderMultiHuman(s, results, partial)
}

// mergeResults keys each environment's Body by context name, with
// {"error": …} for a failed environment, and returns a
// *client.PartialFailureError naming the failures, or nil if there are none.
func mergeResults(results []client.EnvResult) (map[string]interface{}, error) {
	merged := map[string]interface{}{}
	var failed []client.EnvResult
	for _, r := range results {
		if r.Error != nil {
			merged[r.Name] = map[string]string{"error": r.Error.Error()}
			failed = append(failed, r)
		} else {
			merged[r.Name] = r.Data.(fetched).Body
		}
	}
	if len(failed) == 0 {
		return merged, nil
	}
	return merged, &client.PartialFailureError{Failed: failed, Total: len(results)}
}

// renderMultiAgent prints the merged results as one agent envelope. The
// envelope has no per-environment context, so failures and truncated lists
// are named in its warnings, in config order.
func (a *App) renderMultiAgent(s fetchSpec, results []client.EnvResult, merged map[string]interface{}, partial error) error {
	ctx := &agentmode.ResponseContext{Verb: s.Verb, Resource: s.Resource}
	for _, r := range results {
		if r.Error != nil {
			ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("%s: %v", r.Name, r.Error))
		} else if f := r.Data.(fetched); f.HasMore {
			ctx.HasMore = true
			ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("%s: %s", r.Name, truncationNote(s, f)))
		}
	}
	if partial == nil {
		return output.NewAgentPrinter(a.Out, ctx).Print(merged)
	}
	if err := output.WriteAgentPartial(a.Out, merged, errorDetail(partial, ""), ctx); err != nil {
		return err
	}
	return fmt.Errorf("%w: %w", errReported, partial)
}

// renderMultiHuman prints a section per environment for a person, a failed
// environment's error going to stderr in its place.
func (a *App) renderMultiHuman(s fetchSpec, results []client.EnvResult, partial error) error {
	for i, r := range results {
		if i > 0 {
			_, _ = fmt.Fprintln(a.Out)
		}
		_, _ = fmt.Fprintf(a.Out, "== %s ==\n", r.Name)
		if r.Error != nil {
			output.PrintHumanError("%s: %v", r.Name, r.Error)
			continue
		}
		if err := a.renderHuman(s, r.Data.(fetched)); err != nil {
			return err
		}
	}
	if partial != nil {
		// Each failure was shown in its own section above.
		return fmt.Errorf("%w: %w", errReported, partial)
	}
	return nil
}

// renderHuman renders one environment's result as a table, a custom summary
// or YAML, with a stderr note when the result is empty or truncated, followed
// by the fetch's warnings.
func (a *App) renderHuman(s fetchSpec, f fetched) error {
	if err := a.renderBody(s, f); err != nil {
		return err
	}
	for _, w := range f.Warnings {
		output.PrintWarning("%s", w)
	}
	return nil
}

// renderBody is renderHuman without the warnings.
func (a *App) renderBody(s fetchSpec, f fetched) error {
	switch {
	case s.Human != nil:
		return s.Human(a.Out, f.Body)
	case s.Rows != nil:
		rows, n, err := s.Rows(f.Body)
		if err != nil {
			return err
		}
		if n == 0 {
			if s.Empty != "" {
				output.PrintInfo("%s", s.Empty)
			}
			return nil
		}
		if err := output.NewPrinterToWithColumns(string(a.format()), a.Out, a.columns()).PrintList(rows); err != nil {
			return err
		}
		if f.HasMore {
			output.PrintInfo("%s. Use --limit or narrower filters.", showing(n, f.Total, s.Resource))
		}
		return nil
	}
	return output.NewPrinterTo(string(output.FormatYAML), a.Out).Print(f.Body)
}

// responseContext is the agent envelope context for one environment's result.
func (a *App) responseContext(s fetchSpec, f fetched) *agentmode.ResponseContext {
	ctx := &agentmode.ResponseContext{Verb: s.Verb, Resource: s.Resource, HasMore: f.HasMore}
	if f.Total >= 0 {
		total := f.Total
		ctx.Total = &total
	}
	if f.HasMore && s.Rows != nil {
		if _, n, err := s.Rows(f.Body); err == nil {
			ctx.Suggestions = append(ctx.Suggestions, showing(n, f.Total, s.Resource)+"; use --limit to see more")
		}
	}
	return ctx
}

// truncationNote says that f holds only part of the result: how many of how
// many when Rows can count the items, otherwise only that more exist.
func truncationNote(s fetchSpec, f fetched) string {
	if s.Rows != nil {
		if _, n, err := s.Rows(f.Body); err == nil {
			return showing(n, f.Total, s.Resource)
		}
	}
	if f.Total >= 0 {
		return fmt.Sprintf("Showing part of %d %s", f.Total, s.Resource)
	}
	return fmt.Sprintf("More %s are available", s.Resource)
}

// humanOutput reports whether results go to a person: table or wide output,
// not agent mode. Fetches use it to skip work only a table needs.
func (a *App) humanOutput() bool {
	if a.agentMode() {
		return false
	}
	f := a.format()
	return f != output.FormatJSON && f != output.FormatYAML
}

// showing is the "Showing n of total resource" phrase of a truncation note.
func showing(n, total int, resource string) string {
	if total < 0 {
		return fmt.Sprintf("Showing %d %s, more are available", n, resource)
	}
	return fmt.Sprintf("Showing %d of %d %s", n, total, resource)
}

// pagedFetch is the Fetch of a v2 list endpoint whose items are under
// listKey. pageCap is the largest pageSize the endpoint accepts: the request
// asks for min(limit, pageCap) items per page and pages until limit is met.
func pagedFetch(path string, params map[string]string, listKey string, pageCap, maxPages int) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		p := make(map[string]string, len(params)+1)
		for k, v := range params {
			p[k] = v
		}
		pageSize := pageCap
		if limit > 0 && limit < pageCap {
			pageSize = limit
		}
		p["pageSize"] = strconv.Itoa(pageSize)
		body, err := c.GetV2Paged(path, p, client.Paging{MaxPages: maxPages, Limit: limit, ListKey: listKey})
		if err != nil {
			return fetched{}, err
		}
		return listFetched(body, listKey), nil
	}
}

// listFetched wraps a list endpoint's merged body. An absent list becomes an
// empty one, so an empty result still prints its list.
func listFetched(body map[string]interface{}, listKey string) fetched {
	list, ok := body[listKey].([]interface{})
	if !ok {
		list = []interface{}{}
		body[listKey] = list
	}
	_, more := body["nextPageKey"]
	total := -1
	if tc, ok := body["totalCount"].(float64); ok {
		total = int(tc)
	} else if !more {
		total = len(list)
	}
	if total > len(list) {
		more = true
	}
	return fetched{Body: body, Total: total, HasMore: more}
}
