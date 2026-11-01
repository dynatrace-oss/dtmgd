package cmd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// The update schemas get settings shows as readable tables.
const (
	schemaUpdateWindows     = "builtin:deployment.management.update-windows"
	schemaOneAgentUpdates   = "builtin:deployment.oneagent.updates"
	schemaActiveGateUpdates = "builtin:deployment.activegate.updates"
)

// The update modes that use update windows. OneAgent and ActiveGate spell
// them differently.
const (
	modeOneAgentDuringWindows   = "AUTOMATIC_DURING_MW"
	modeActiveGateDuringWindows = "AUTOMATIC_DURING_UW"
)

// noValue is the cell of a field the value does not have.
const noValue = "—"

// settingsObject is a stored Settings 2.0 object as get settings asks for it
// (settingsObjectFields), or an effective value (which has origin instead of
// objectId, scope and updateToken).
type settingsObject struct {
	ObjectID      string                 `json:"objectId"`
	SchemaID      string                 `json:"schemaId"`
	Scope         string                 `json:"scope"`
	ExternalID    string                 `json:"externalId"`
	Summary       string                 `json:"summary"`
	UpdateToken   string                 `json:"updateToken"`
	SchemaVersion string                 `json:"schemaVersion"`
	Origin        string                 `json:"origin"`
	Value         map[string]interface{} `json:"value"`
}

// genericSettingsRow is the table row for objects of any one schema.
type genericSettingsRow struct {
	ObjectID string `table:"OBJECT-ID"`
	Scope    string `table:"SCOPE"`
	Summary  string `table:"SUMMARY,max=80"`
}

// genericMultiSchemaRow is the table row for objects of several schemas.
type genericMultiSchemaRow struct {
	Schema   string `table:"SCHEMA"`
	ObjectID string `table:"OBJECT-ID"`
	Scope    string `table:"SCOPE"`
	Summary  string `table:"SUMMARY,max=80"`
}

// settingsView turns get settings' results into table rows. It picks the
// table by the schema of the objects it gets: a readable view for the update
// schemas, a generic one for any other schema or a mix of schemas.
type settingsView struct {
	// names are the update windows' names by object ID, for the WINDOWS
	// column; nil (or a missing ID) shows the object ID.
	names *windowNames
}

// rows is get settings' fetchSpec.Rows. body is a list (items) or one object.
func (v settingsView) rows(body interface{}) (interface{}, int, error) {
	m, _ := body.(map[string]interface{})
	var objs []settingsObject
	if err := decodeItems(m, &objs); err != nil {
		return nil, 0, err
	}
	schema, single := singleSchema(objs)
	if !single {
		return genericMultiSchemaRows(objs)
	}
	switch schema {
	case schemaUpdateWindows:
		return updateWindowRows(objs)
	case schemaOneAgentUpdates:
		return v.oneAgentUpdatesRows(objs)
	case schemaActiveGateUpdates:
		return v.activeGateUpdatesRows(objs)
	}
	return genericRows(objs)
}

// singleSchema returns the schema all objs belong to, and whether there is
// only one (true for no objects).
func singleSchema(objs []settingsObject) (string, bool) {
	if len(objs) == 0 {
		return "", true
	}
	for _, o := range objs[1:] {
		if o.SchemaID != objs[0].SchemaID {
			return "", false
		}
	}
	return objs[0].SchemaID, true
}

func genericRows(objs []settingsObject) (interface{}, int, error) {
	rows := make([]genericSettingsRow, 0, len(objs))
	for _, o := range objs {
		rows = append(rows, genericSettingsRow{ObjectID: o.ObjectID, Scope: o.Scope, Summary: o.Summary})
	}
	return rows, len(rows), nil
}

func genericMultiSchemaRows(objs []settingsObject) (interface{}, int, error) {
	rows := make([]genericMultiSchemaRow, 0, len(objs))
	for _, o := range objs {
		rows = append(rows, genericMultiSchemaRow{Schema: o.SchemaID, ObjectID: o.ObjectID, Scope: o.Scope, Summary: o.Summary})
	}
	return rows, len(rows), nil
}

// updateWindowRow is the table row for update windows.
type updateWindowRow struct {
	Name     string `table:"NAME"`
	Enabled  string `table:"ENABLED"`
	Schedule string `table:"SCHEDULE"`
	ObjectID string `table:"OBJECT-ID"`
}

// oneAgentUpdatesRow is the table row for OneAgent update settings.
type oneAgentUpdatesRow struct {
	Scope         string `table:"SCOPE"`
	UpdateMode    string `table:"UPDATE-MODE"`
	TargetVersion string `table:"TARGET-VERSION"`
	Revision      string `table:"REVISION"`
	Windows       string `table:"WINDOWS,max=60"`
	ObjectID      string `table:"OBJECT-ID"`
}

// activeGateUpdatesRow is the table row for ActiveGate update settings.
type activeGateUpdatesRow struct {
	Scope         string `table:"SCOPE"`
	UpdateMode    string `table:"UPDATE-MODE"`
	TargetVersion string `table:"TARGET-VERSION"`
	Windows       string `table:"WINDOWS,max=60"`
	ObjectID      string `table:"OBJECT-ID"`
}

func updateWindowRows(objs []settingsObject) (interface{}, int, error) {
	rows := make([]updateWindowRow, 0, len(objs))
	for _, o := range objs {
		rows = append(rows, updateWindowRow{
			Name:     cell(o.Value, "name"),
			Enabled:  enabledCell(o.Value),
			Schedule: updateWindowSchedule(o.Value),
			ObjectID: o.ObjectID,
		})
	}
	return rows, len(rows), nil
}

func (v settingsView) oneAgentUpdatesRows(objs []settingsObject) (interface{}, int, error) {
	rows := make([]oneAgentUpdatesRow, 0, len(objs))
	for _, o := range objs {
		rows = append(rows, oneAgentUpdatesRow{
			Scope:         o.Scope,
			UpdateMode:    cell(o.Value, "updateMode"),
			TargetVersion: cell(o.Value, "targetVersion"),
			Revision:      cell(o.Value, "revision"),
			Windows:       v.windowsCell(o.Value, modeOneAgentDuringWindows, "maintenanceWindows", "maintenanceWindow"),
			ObjectID:      o.ObjectID,
		})
	}
	return rows, len(rows), nil
}

func (v settingsView) activeGateUpdatesRows(objs []settingsObject) (interface{}, int, error) {
	rows := make([]activeGateUpdatesRow, 0, len(objs))
	for _, o := range objs {
		rows = append(rows, activeGateUpdatesRow{
			Scope:         o.Scope,
			UpdateMode:    cell(o.Value, "updateMode"),
			TargetVersion: cell(o.Value, "targetVersion"),
			Windows:       v.windowsCell(o.Value, modeActiveGateDuringWindows, "updateWindows", "updateWindow"),
			ObjectID:      o.ObjectID,
		})
	}
	return rows, len(rows), nil
}

// cell is a value field as a table cell, noValue when the value lacks it.
func cell(v map[string]interface{}, key string) string {
	switch t := v[key].(type) {
	case nil:
		return noValue
	case string:
		if t == "" {
			return noValue
		}
		return t
	default:
		return fmt.Sprint(t)
	}
}

// enabledCell is the value's enabled field as yes or no.
func enabledCell(v map[string]interface{}) string {
	enabled, ok := v["enabled"].(bool)
	switch {
	case !ok:
		return noValue
	case enabled:
		return "yes"
	}
	return "no"
}

// windowsCell names the update windows an update setting uses: those under
// listKey (each a reference under itemKey), in order, when its update mode
// is mode (the one that uses windows), else noValue.
func (v settingsView) windowsCell(value map[string]interface{}, mode, listKey, itemKey string) string {
	if cell(value, "updateMode") != mode {
		return noValue
	}
	list, _ := value[listKey].([]interface{})
	var names []string
	for _, item := range list {
		ref, _ := item.(map[string]interface{})
		if id, _ := ref[itemKey].(string); id != "" {
			names = append(names, v.names.name(id))
		}
	}
	if len(names) == 0 {
		return noValue
	}
	return strings.Join(names, ", ")
}

// updateWindowSchedule describes an update window's recurrence in one
// phrase, e.g. "weekly Mon 03:00 GMT+00:00, 60 min". A window whose
// recurrence block is missing shows its recurrence as is.
func updateWindowSchedule(v map[string]interface{}) string {
	recurrence, _ := v["recurrence"].(string)
	switch recurrence {
	case "ONCE":
		if r := nested(v, "onceRecurrence", "recurrenceRange"); r != nil {
			return fmt.Sprintf("once %v – %v", r["start"], r["end"])
		}
	case "DAILY":
		if b := nested(v, "dailyRecurrence"); b != nil {
			return recurringSchedule(b, "daily", "days", "")
		}
	case "WEEKLY":
		if b := nested(v, "weeklyRecurrence"); b != nil {
			return recurringSchedule(b, "weekly", "weeks", weekDays(nested(b, "selectedWeekDays")))
		}
	case "MONTHLY":
		if b := nested(v, "monthlyRecurrence"); b != nil {
			return recurringSchedule(b, "monthly", "months", fmt.Sprintf("on day %d", intOf(b["selectedMonthDay"])))
		}
	}
	return recurrence
}

// recurringSchedule is the phrase for a daily, weekly or monthly block:
// its cadence ("weekly", "every 2 weeks"), when (days of the week or the
// month, if any), then its start time, time zone and duration.
func recurringSchedule(block map[string]interface{}, once, unit, when string) string {
	parts := []string{once}
	if every := intOf(block["every"]); every > 1 {
		parts[0] = fmt.Sprintf("every %d %s", every, unit)
	}
	if when != "" {
		parts = append(parts, when)
	}
	if t := nested(block, "updateTime"); t != nil {
		parts = append(parts, fmt.Sprintf("%v %v, %d min", t["startTime"], t["timeZone"], intOf(t["duration"])))
	}
	return strings.Join(parts, " ")
}

// weekDays lists the selected days, Monday first, as "Mon,Fri".
func weekDays(selected map[string]interface{}) string {
	var days []string
	for _, d := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		if on, _ := selected[d].(bool); on {
			days = append(days, strings.ToUpper(d[:1])+d[1:3])
		}
	}
	return strings.Join(days, ",")
}

// nested follows keys through nested objects, nil when one is missing.
func nested(v map[string]interface{}, keys ...string) map[string]interface{} {
	for _, k := range keys {
		next, ok := v[k].(map[string]interface{})
		if !ok {
			return nil
		}
		v = next
	}
	return v
}

// intOf is a JSON number as an int, 0 when it is not one.
func intOf(v interface{}) int {
	f, _ := v.(float64)
	return int(f)
}

// windowNames maps update windows' object IDs to their names. The fetches
// of a multi-environment run fill it in parallel, so a mutex guards it;
// object IDs are unique across environments.
type windowNames struct {
	mu sync.Mutex
	m  map[string]string
}

func (n *windowNames) set(id, name string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.m == nil {
		n.m = map[string]string{}
	}
	n.m[id] = name
}

// name is the window's name, or id when it is unknown (a window deleted
// while settings still name it, or a failed lookup).
func (n *windowNames) name(id string) string {
	if n == nil {
		return id
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if name, ok := n.m[id]; ok {
		return name
	}
	return id
}

// lookupWindowNames fills n with the environment's update windows' names.
func lookupWindowNames(c *client.Client, n *windowNames) error {
	body, err := c.GetV2Paged(settingsObjectsPath, map[string]string{
		"schemaIds": schemaUpdateWindows,
		"scopes":    "environment",
		"fields":    "objectId,value",
		"pageSize":  "500",
	}, client.Paging{ListKey: "items"})
	if err != nil {
		return err
	}
	var windows []settingsObject
	if err := decodeItems(body, &windows); err != nil {
		return err
	}
	for _, w := range windows {
		if name, ok := w.Value["name"].(string); ok {
			n.set(w.ObjectID, name)
		}
	}
	return nil
}

// usesWindows reports whether a get settings body holds OneAgent or
// ActiveGate update settings, whose table names update windows.
func usesWindows(body interface{}) bool {
	m, _ := body.(map[string]interface{})
	var objs []settingsObject
	if decodeItems(m, &objs) != nil {
		return false
	}
	for _, o := range objs {
		if o.SchemaID == schemaOneAgentUpdates || o.SchemaID == schemaActiveGateUpdates {
			return true
		}
	}
	return false
}
