package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// Cluster ActiveGate auto-update lives at the Cluster API's own endpoints:
// the Settings 2.0 schema for it is not public. Paths are relative to
// {host}/api.
const (
	clusterAGPath           = "/cluster/v2/activeGates"
	clusterAGAutoUpdatePath = clusterAGPath + "/autoUpdate"
)

// The resource name, its usage with an optional ID, and its alias, shared by
// the get, describe, update and edit commands.
const (
	agResource = "activegate-updates"
	agUse      = agResource + " [activegate-id]"
	agAlias    = "ag-updates"
)

// hintListClusterActiveGates is the next step after an unknown ActiveGate ID.
const hintListClusterActiveGates = "List cluster ActiveGates with: dtmgd get activegate-updates"

// clusterAGUpdatesRow is a row of get activegate-updates: the global
// configuration, then one row per cluster ActiveGate. TARGET-VERSION and
// WINDOWS show only where the cluster returns them: it honours them only
// behind a server feature flag that is off by default.
type clusterAGUpdatesRow struct {
	ID            string `table:"ID"`
	Hostname      string `table:"HOSTNAME"`
	Version       string `table:"VERSION"`
	Setting       string `table:"SETTING"`
	Effective     string `table:"EFFECTIVE"`
	Status        string `table:"STATUS"`
	TargetVersion string `table:"TARGET-VERSION"`
	Windows       string `table:"WINDOWS,max=60"`
}

// agAutoUpdatePath is one ActiveGate's auto-update path.
func agAutoUpdatePath(id string) string {
	return clusterAGPath + "/" + url.PathEscape(id) + "/autoUpdate"
}

func newGetActiveGateUpdatesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     agUse,
		Aliases: []string{agAlias},
		Short:   "Show how the cluster's ActiveGates update",
		Long: `Show the cluster ActiveGates' automatic updates: the global setting, then
each cluster ActiveGate's own setting (ENABLED, DISABLED or INHERITED from the
global one), the setting in effect, and its update status. With an ID, that
ActiveGate's configuration.

TARGET-VERSION and WINDOWS appear only where the cluster returns them.

json and yaml print {"global": …, "activeGates": […]}, each as the Cluster API
returns it. Needs a cluster context whose token has ServiceProviderAPI.

Examples:
  dtmgd get activegate-updates --context prod-cluster
  dtmgd get activegate-updates <activegate-id> --context prod-cluster`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := fetchSpec{
				Verb:     "get",
				Resource: agResource,
				Fetch:    agUpdatesFetch,
				Rows:     agUpdatesRows(""),
				NotFound: hintListClusterActiveGates,
			}
			id, err := agID(args)
			if err != nil {
				return err
			}
			if id != "" {
				spec.Fetch = clusterObjectFetch(agAutoUpdatePath(id), nil)
				spec.Rows = agUpdatesRows(id)
			}
			return a.runClusterFetch(spec)
		},
	}
	return clusterCommand(cmd)
}

func newDescribeActiveGateUpdatesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     agUse,
		Aliases: []string{agAlias},
		Short:   "Show the cluster ActiveGates' auto-update configuration",
		Long: `Show the global auto-update configuration of the cluster's ActiveGates, or
one ActiveGate's, as the Cluster API returns it. update activegate-updates
takes the same object.

Examples:
  dtmgd describe activegate-updates --context prod-cluster
  dtmgd describe activegate-updates <activegate-id> -o yaml --context prod-cluster`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agID(args)
			if err != nil {
				return err
			}
			path := agConfigPath(id)
			return a.runClusterFetch(fetchSpec{
				Verb:     "describe",
				Resource: agResource,
				Fetch:    clusterObjectFetch(path, nil),
				NotFound: hintListClusterActiveGates,
			})
		},
	}
	return clusterCommand(cmd)
}

// agUpdatesFetch reads the global configuration and the cluster's own
// ActiveGates (type CLUSTER), which carry their auto-update setting and
// status.
func agUpdatesFetch(c *client.Client, _ int) (fetched, error) {
	var global map[string]interface{}
	if err := c.GetCluster(clusterAGAutoUpdatePath, nil, &global); err != nil {
		return fetched{}, err
	}
	var list map[string]interface{}
	if err := c.GetCluster(clusterAGPath, map[string]string{"type": "CLUSTER"}, &list); err != nil {
		return fetched{}, err
	}
	gates, _ := list["activeGates"].([]interface{})
	if gates == nil {
		gates = []interface{}{}
	}
	return fetched{Body: map[string]interface{}{"global": global, "activeGates": gates}, Total: len(gates)}, nil
}

// agUpdatesRows turns get activegate-updates' body into rows: the global
// row and one per ActiveGate, or, for one ActiveGate (id), its row.
func agUpdatesRows(id string) func(body interface{}) (interface{}, int, error) {
	return func(body interface{}) (interface{}, int, error) {
		m, _ := body.(map[string]interface{})
		if id != "" {
			row := configRow(m)
			row.ID = id
			return []clusterAGUpdatesRow{row}, 1, nil
		}
		global, _ := m["global"].(map[string]interface{})
		row := configRow(global)
		row.ID, row.Setting, row.Effective = "(global)", cell(global, "globalSetting"), cell(global, "globalSetting")
		rows := []clusterAGUpdatesRow{row}
		gates, _ := m["activeGates"].([]interface{})
		for _, g := range gates {
			rows = append(rows, gateRow(g))
		}
		return rows, len(rows), nil
	}
}

// configRow is the row of an auto-update configuration object.
func configRow(cfg map[string]interface{}) clusterAGUpdatesRow {
	return clusterAGUpdatesRow{
		Hostname: noValue, Version: noValue, Status: noValue,
		Setting:       cell(cfg, "setting"),
		Effective:     cell(cfg, "effectiveSetting"),
		TargetVersion: cell(cfg, "targetVersion"),
		Windows:       configWindows(cfg),
	}
}

// gateRow is the row of one ActiveGate of the list.
func gateRow(g interface{}) clusterAGUpdatesRow {
	gate, _ := g.(map[string]interface{})
	settings := nested(gate, "autoUpdateSettings")
	return clusterAGUpdatesRow{
		ID: cell(gate, "id"), Hostname: cell(gate, "hostname"), Version: cell(gate, "version"),
		Setting: cell(settings, "setting"), Effective: cell(settings, "effectiveSetting"),
		Status: cell(gate, "autoUpdateStatus"), TargetVersion: noValue, Windows: noValue,
	}
}

// configWindows names the update windows of a configuration
// (updateWindows.windows[].name), or noValue without any.
func configWindows(cfg map[string]interface{}) string {
	list, _ := nested(cfg, "updateWindows")["windows"].([]interface{})
	var names []string
	for _, w := range list {
		win, _ := w.(map[string]interface{})
		names = append(names, cell(win, "name"))
	}
	if len(names) == 0 {
		return noValue
	}
	return strings.Join(names, ", ")
}

// agReadOnlyFields are fields of an auto-update configuration that the
// server sets: dropped from a file, ignored when comparing.
var agReadOnlyFields = []string{"metadata", "effectiveSetting"}

// agFlaggedFields take effect only where the cluster's GET returns them.
var agFlaggedFields = []string{"targetVersion", "updateWindows"}

// errEmptyAGID refuses an empty ActiveGate ID, which would otherwise mean
// the global configuration (an unset shell variable, say).
var errEmptyAGID = &UsageError{Err: errors.New("the ActiveGate ID is empty; leave it out to mean the global configuration")}

// agID is the ActiveGate a command names, "" for the global configuration.
func agID(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if args[0] == "" {
		return "", errEmptyAGID
	}
	return args[0], nil
}

// normalAGConfig is an auto-update configuration as dtmgd sends and
// compares it: without the fields the server sets (metadata,
// effectiveSetting, each window's name), windows by id only.
func normalAGConfig(m map[string]interface{}) map[string]interface{} {
	out := withoutFields(m, agReadOnlyFields)
	uw, ok := out["updateWindows"].(map[string]interface{})
	if !ok {
		return out
	}
	list, _ := uw["windows"].([]interface{})
	ids := make([]interface{}, 0, len(list))
	for _, w := range list {
		if win, ok := w.(map[string]interface{}); ok {
			ids = append(ids, map[string]interface{}{"id": win["id"]})
		}
	}
	out["updateWindows"] = map[string]interface{}{"windows": ids}
	return out
}

func newUpdateActiveGateUpdatesCmd(a *App) *cobra.Command {
	var opts struct {
		file                 string
		dryRun, validateOnly bool
	}
	cmd := &cobra.Command{
		Use:     agUse + " -f <file>",
		Aliases: []string{agAlias},
		Short:   "Change how the cluster's ActiveGates update",
		Long: `Change the cluster ActiveGates' global auto-update configuration, or, with an
ID, one ActiveGate's. The file (YAML or JSON, - for stdin) holds the object
describe activegate-updates prints: {globalSetting: ENABLED|DISABLED} for the
global one, {setting: ENABLED|DISABLED|INHERITED} for one ActiveGate.

targetVersion and updateWindows are accepted only where the cluster's
configuration carries them (they take effect behind a server feature flag);
elsewhere the file is refused rather than the fields dropped.

--dry-run shows the change with GETs only. --validate-only has the cluster's
validator check it without saving. Needs a cluster context whose
safety-level allows updates, and a token with ServiceProviderAPI.

Examples:
  dtmgd describe activegate-updates -o yaml --context stage-cluster > global.yaml
  dtmgd update activegate-updates -f global.yaml --dry-run --context stage-cluster
  dtmgd update activegate-updates <activegate-id> -f gate.yaml --context stage-cluster`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readValueFile(opts.file, a.input())
			if err != nil {
				return err
			}
			id, err := agID(args)
			if err != nil {
				return err
			}
			return a.runWrites(writeSpec{
				Verb: "update", Resource: agResource,
				DryRun: opts.dryRun, ValidateOnly: opts.validateOnly,
				ClusterOnly: true, NotFound: hintListClusterActiveGates,
				Plan: func(_ string, c *client.Client) ([]plannedWrite, error) {
					return planActiveGateUpdate(c, id, value)
				},
			})
		},
	}
	cmd.Flags().StringVarP(&opts.file, "filename", "f", "", "file holding the configuration, YAML or JSON (- for stdin)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "show the change, sending GETs only")
	cmd.Flags().BoolVar(&opts.validateOnly, "validate-only", false, "have the cluster's validator check the change without saving it")
	_ = cmd.MarkFlagRequired("filename")
	return markMutating(clusterCommand(cmd))
}

func newEditActiveGateUpdatesCmd(a *App) *cobra.Command {
	var validateOnly bool
	cmd := &cobra.Command{
		Use:     agUse,
		Aliases: []string{agAlias},
		Short:   "Edit the cluster ActiveGates' auto-update in an editor",
		Long: `Open the cluster ActiveGates' global auto-update configuration (or one
ActiveGate's) in $EDITOR, and update it when the file changed. For people at a
terminal; agents use update activegate-updates -f.

Examples:
  dtmgd edit activegate-updates --context stage-cluster`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !a.isInteractive() {
				return withHint(&UsageError{Err: errNeedsTerminal}, "Use: dtmgd describe activegate-updates -o yaml > config.yaml, change it, then dtmgd update activegate-updates -f config.yaml")
			}
			if !validateOnly {
				if err := a.checkCanUpdate(); err != nil {
					return err
				}
			}
			id, err := agID(args)
			if err != nil {
				return err
			}
			return a.runWrites(writeSpec{
				Verb: "edit", Resource: agResource,
				ValidateOnly: validateOnly, ClusterOnly: true, NotFound: hintListClusterActiveGates,
				Plan: func(_ string, c *client.Client) ([]plannedWrite, error) {
					return a.planActiveGateEdit(c, id)
				},
			})
		},
	}
	cmd.Flags().BoolVar(&validateOnly, "validate-only", false, "have the cluster's validator check the edited configuration without saving it")
	return markMutating(clusterCommand(cmd))
}

// agConfigPath is the auto-update path of ActiveGate id, or the global one.
func agConfigPath(id string) string {
	if id == "" {
		return clusterAGAutoUpdatePath
	}
	return agAutoUpdatePath(id)
}

// readAGConfig reads an auto-update configuration, a 404 hinting at the
// ActiveGate list.
func readAGConfig(c *client.Client, id string) (map[string]interface{}, error) {
	var cur map[string]interface{}
	err := c.GetCluster(agConfigPath(id), nil, &cur)
	var apiErr *client.ErrAPI
	if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
		return nil, withHint(err, hintListClusterActiveGates)
	}
	return cur, err
}

// planActiveGateUpdate plans writing value as ActiveGate id's auto-update
// configuration (the global one for ""): refused when it holds fields the
// cluster would drop or values that configuration cannot take, unchanged
// when it equals the current one.
func planActiveGateUpdate(c *client.Client, id string, value map[string]interface{}) ([]plannedWrite, error) {
	cur, err := readAGConfig(c, id)
	if err != nil {
		return nil, err
	}
	value = normalAGConfig(value)
	if err := checkAutoUpdateFile(id, cur, value); err != nil {
		return nil, err
	}
	scope := id
	if scope == "" {
		scope = "global"
	}
	w := plannedWrite{
		Op: config.OpUpdate, Action: actionUpdate,
		SchemaID: "cluster-activegate-autoupdate", Scope: scope,
		Value: value, Current: cur,
		Exec: func(c *client.Client, validateOnly bool) writeResult {
			return sendAGConfig(c, agConfigPath(id), value, validateOnly)
		},
	}
	if reflect.DeepEqual(value, normalAGConfig(cur)) {
		w.Op, w.Action = "", actionUnchanged
	}
	return []plannedWrite{w}, nil
}

// planActiveGateEdit lets a person edit ActiveGate id's configuration, then
// plans writing it.
func (a *App) planActiveGateEdit(c *client.Client, id string) ([]plannedWrite, error) {
	cur, err := readAGConfig(c, id)
	if err != nil {
		return nil, err
	}
	edited, changed, err := a.editValue(normalAGConfig(cur))
	if err != nil {
		return nil, err
	}
	if !changed {
		output.PrintInfo("Edit cancelled: the configuration is unchanged.")
	}
	return planActiveGateUpdate(c, id, edited)
}

// checkAutoUpdateFile refuses a configuration the cluster would take
// differently than the file says.
func checkAutoUpdateFile(id string, cur, value map[string]interface{}) error {
	if err := checkAGShape(id, value); err != nil {
		return err
	}
	return checkAGFlagged(cur, value)
}

// checkAGShape refuses a file that is not one configuration: the global
// one takes globalSetting ENABLED or DISABLED, one ActiveGate's setting
// ENABLED, DISABLED or INHERITED, each besides targetVersion and
// updateWindows.
func checkAGShape(id string, value map[string]interface{}) error {
	key, allowed := "globalSetting", "ENABLED, DISABLED"
	if id != "" {
		key, allowed = "setting", "ENABLED, DISABLED, INHERITED"
	}
	for k := range value {
		if k != key && k != "targetVersion" && k != "updateWindows" {
			return &UsageError{Err: fmt.Errorf("unknown field %q: this configuration takes %s (%s), targetVersion and updateWindows", k, key, allowed)}
		}
	}
	v, _ := value[key].(string)
	if !strings.Contains(", "+allowed+",", ", "+v+",") || v == "" {
		return &UsageError{Err: fmt.Errorf("%s must be one of %s, not %v", key, allowed, value[key])}
	}
	return nil
}

// checkAGFlagged refuses targetVersion or updateWindows where the cluster
// would drop them (its GET does not return them), and a file that leaves
// out the update windows of a cluster that honours them: it would clear
// them.
func checkAGFlagged(cur, value map[string]interface{}) error {
	for _, f := range agFlaggedFields {
		_, inFile := value[f]
		_, honoured := cur[f]
		if inFile && !honoured {
			return &UsageError{Err: fmt.Errorf("this cluster ignores %s in ActiveGate auto-update (it takes effect only where the configuration already carries it); remove it from the file", f)}
		}
	}
	if _, honoured := cur["updateWindows"]; honoured {
		if _, inFile := value["updateWindows"]; !inFile {
			return &UsageError{Err: errors.New("this cluster honours updateWindows, and a configuration without them clears them: keep updateWindows in the file (describe activegate-updates -o yaml shows them)")}
		}
	}
	return nil
}

// sendAGConfig PUTs a configuration, or POSTs it to the validator, which
// checks it without saving.
func sendAGConfig(c *client.Client, path string, value map[string]interface{}, validateOnly bool) writeResult {
	var err error
	action := actionUpdated
	if validateOnly {
		err, action = c.PostCluster(path+"/validator", nil, value, nil), actionValidated
	} else {
		err = c.PutCluster(path, nil, value, nil)
	}
	if err != nil {
		return writeResult{Action: actionFailed, Error: toWriteError(err)}
	}
	return writeResult{Action: action}
}

// withoutFields is a copy of m without fields.
func withoutFields(m map[string]interface{}, fields []string) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	for _, f := range fields {
		delete(out, f)
	}
	return out
}

// clusterWindowReferences lists the cluster ActiveGate auto-update
// configurations ("global", "ActiveGate <id>") that name cluster update
// window windowID. A cluster whose configuration carries no updateWindows
// ignores them, so nothing references a window there.
func clusterWindowReferences(c *client.Client, windowID string) ([]string, error) {
	global, err := readAGConfig(c, "")
	if err != nil {
		return nil, err
	}
	if _, honoured := global["updateWindows"]; !honoured {
		return nil, nil
	}
	var refs []string
	if namesWindow(global, windowID) {
		refs = append(refs, "global")
	}
	var list map[string]interface{}
	if err := c.GetCluster(clusterAGPath, map[string]string{"type": "CLUSTER"}, &list); err != nil {
		return nil, err
	}
	gates, _ := list["activeGates"].([]interface{})
	for _, g := range gates {
		gate, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		id := cell(gate, "id")
		cfg, err := readAGConfig(c, id)
		if err != nil {
			return nil, err
		}
		if namesWindow(cfg, windowID) {
			refs = append(refs, "ActiveGate "+id)
		}
	}
	return refs, nil
}

// namesWindow reports whether a configuration's updateWindows name windowID.
func namesWindow(cfg map[string]interface{}, windowID string) bool {
	list, _ := nested(cfg, "updateWindows")["windows"].([]interface{})
	for _, w := range list {
		if win, _ := w.(map[string]interface{}); win["id"] == windowID {
			return true
		}
	}
	return false
}
