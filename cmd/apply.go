package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
	"github.com/dynatrace-oss/dtmgd/pkg/config"
)

// applyObject is one object of an apply file: dtctl's envelope, which is
// also the shape get settings -o yaml prints. Read-only fields (summary,
// created, …) are ignored.
type applyObject struct {
	ObjectID    string                 `json:"objectId"`
	ExternalID  string                 `json:"externalId"`
	SchemaID    string                 `json:"schemaId"`
	Scope       string                 `json:"scope"`
	Value       map[string]interface{} `json:"value"`
	UpdateToken string                 `json:"updateToken"`
}

func newApplyCmd(a *App) *cobra.Command {
	var opts struct {
		file                 string
		dryRun, validateOnly bool
	}
	cmd := &cobra.Command{
		Use:   "apply -f <file>",
		Short: "Create or update the Settings 2.0 objects in a file",
		Long: `Create or update the Settings 2.0 objects in a file: YAML or JSON (- for
stdin), holding one {objectId?, externalId?, schemaId, scope, value,
updateToken?} object, an array of them, or a list as get settings -o yaml
prints it. An exported object applies back as it is.

For each object apply first finds out whether it exists, with GETs only:
  - with objectId: that object (if it is not found, apply creates it, which
    moves a file from one environment to another);
  - without: for a schema holding one object per scope, the object at the
    scope; otherwise the object with the same externalId.
An existing object is updated, with the updateToken from the file (which
catches changes since the export) or from the GET; one whose value is the
same is left unchanged. Anything else is created. Give objects of
multi-object schemas an externalId to make repeated applies update them
instead of creating duplicates.

--dry-run shows the planned changes, with GETs only, at any write level.
--validate-only asks the cluster to check every change without saving it
(Dynatrace Managed 1.288 or later). Every selected context is checked before
any of them is written.

Examples:
  dtmgd describe settings <object-id> -o yaml > object.yaml   # export, edit, then:
  dtmgd apply -f object.yaml --dry-run
  dtmgd apply -f object.yaml
  dtmgd apply -f windows.yaml --env ALL_ENVIRONMENTS`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			objs, err := readApplyFile(opts.file, os.Stdin)
			if err != nil {
				return err
			}
			return a.runWrites(writeSpec{
				Verb:         "apply",
				Resource:     "settings",
				DryRun:       opts.dryRun,
				ValidateOnly: opts.validateOnly,
				MultiEnv:     true,
				Plan: func(_ string, c *client.Client) ([]plannedWrite, error) {
					return planApply(c, objs)
				},
			})
		},
	}
	cmd.Flags().StringVarP(&opts.file, "filename", "f", "", "file holding the objects, YAML or JSON (- for stdin)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "show the planned changes, sending GETs only")
	cmd.Flags().BoolVar(&opts.validateOnly, "validate-only", false, "have the cluster validate every change without saving it")
	_ = cmd.MarkFlagRequired("filename")
	return withWrites(cmd)
}

// readApplyFile reads the objects of an apply file: one object, an array,
// or a list response (its items; totalCount and nextPageKey are ignored).
func readApplyFile(path string, stdin io.Reader) ([]applyObject, error) {
	data, err := readInput(path, stdin)
	if err != nil {
		return nil, err
	}
	var raw interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, &UsageError{Err: fmt.Errorf("%s: not YAML or JSON: %w", path, err)}
	}
	list, err := applyItems(raw)
	if err != nil {
		return nil, &UsageError{Err: fmt.Errorf("%s: %w", path, err)}
	}
	var objs []applyObject
	if err := jsonRoundTrip(list, &objs); err != nil {
		return nil, &UsageError{Err: fmt.Errorf("%s: %w", path, err)}
	}
	for i, o := range objs {
		if err := o.check(); err != nil {
			return nil, &UsageError{Err: fmt.Errorf("%s: object %d: %w", path, i, err)}
		}
	}
	return objs, nil
}

// applyItems is the list of objects in a decoded apply file.
func applyItems(raw interface{}) ([]interface{}, error) {
	switch v := raw.(type) {
	case []interface{}:
		return v, nil
	case map[string]interface{}:
		if items, ok := v["items"].([]interface{}); ok {
			return items, nil
		}
		return []interface{}{v}, nil
	}
	return nil, errors.New("want an object, an array of objects, or a list with items")
}

// check refuses an object apply cannot place.
func (o applyObject) check() error {
	switch {
	case o.SchemaID == "":
		return errors.New("no schemaId")
	case o.ObjectID == "" && o.Scope == "":
		return errors.New("neither objectId nor scope")
	case o.Value == nil:
		return errors.New("no value")
	}
	return nil
}

// applyPlanner plans one context's apply, caching schema definitions and
// listings across the file's objects.
type applyPlanner struct {
	c      *client.Client
	shapes map[string]settingsSchemaShape
	listed map[string][]settingsObject
}

// planApply resolves what apply would do with each object in one context.
// An object whose state cannot be read fails the whole plan, so nothing is
// written on a guess.
func planApply(c *client.Client, objs []applyObject) ([]plannedWrite, error) {
	p := &applyPlanner{c: c, shapes: map[string]settingsSchemaShape{}, listed: map[string][]settingsObject{}}
	writes := make([]plannedWrite, 0, len(objs))
	for _, o := range objs {
		w, err := p.plan(o)
		if err != nil {
			return nil, err
		}
		writes = append(writes, w)
	}
	return writes, nil
}

// plan resolves one object: by its objectId, else by a lookup.
func (p *applyPlanner) plan(o applyObject) (plannedWrite, error) {
	if o.ObjectID == "" {
		return p.planByLookup(o)
	}
	var cur settingsObject
	err := p.c.GetV2(settingsObjectsPath+"/"+url.PathEscape(o.ObjectID), nil, &cur)
	var apiErr *client.ErrAPI
	switch {
	case errors.As(err, &apiErr) && apiErr.StatusCode == 404:
		// Not in this environment: create it here, unless the lookup finds
		// the object it would overwrite.
		if o.Scope == "" {
			return plannedWrite{}, fmt.Errorf("object %s is not in this environment, and the file gives no scope to create it in", o.ObjectID)
		}
		o.ObjectID, o.UpdateToken = "", ""
		return p.planByLookup(o)
	case err != nil:
		return plannedWrite{}, fmt.Errorf("object %s: %w", o.ObjectID, err)
	}
	return updateOrUnchanged(o, cur), nil
}

// planByLookup finds the object a POST would overwrite: for a single-object
// schema the one at the scope, else the one with the same externalId, else
// the one with the same value of the schema's key property. A match is
// updated; without one the object is created.
func (p *applyPlanner) planByLookup(o applyObject) (plannedWrite, error) {
	shape, err := p.shape(o.SchemaID)
	if err != nil {
		return plannedWrite{}, err
	}
	listed, err := p.list(o.SchemaID, o.Scope)
	if err != nil {
		return plannedWrite{}, err
	}
	if match := findMatch(shape, o, listed); match != nil {
		o.UpdateToken = ""
		return updateOrUnchanged(o, *match), nil
	}
	existing := make(map[string]bool, len(listed))
	for _, l := range listed {
		existing[l.ObjectID] = true
	}
	return plannedWrite{Op: config.OpCreate, Action: actionCreate, SchemaID: o.SchemaID, Scope: o.Scope, ExternalID: o.ExternalID, Value: o.Value, Existing: existing}, nil
}

// findMatch is the listed object o would overwrite, or nil.
func findMatch(shape settingsSchemaShape, o applyObject, listed []settingsObject) *settingsObject {
	if !shape.MultiObject {
		if len(listed) > 0 {
			return &listed[0]
		}
		return nil
	}
	for i, l := range listed {
		if o.ExternalID != "" && l.ExternalID == o.ExternalID {
			return &listed[i]
		}
		if k := shape.KeyProperty; k != "" && o.Value[k] != nil && reflect.DeepEqual(o.Value[k], l.Value[k]) {
			return &listed[i]
		}
	}
	return nil
}

// shape is schemaID's definition, read once.
func (p *applyPlanner) shape(schemaID string) (settingsSchemaShape, error) {
	if s, ok := p.shapes[schemaID]; ok {
		return s, nil
	}
	s, err := schemaShape(p.c, schemaID)
	if err == nil {
		p.shapes[schemaID] = s
	}
	return s, err
}

// list is every object of schemaID at scope, read once.
func (p *applyPlanner) list(schemaID, scope string) ([]settingsObject, error) {
	key := schemaID + "\x00" + scope
	if l, ok := p.listed[key]; ok {
		return l, nil
	}
	l, err := listSettings(p.c, schemaID, scope, "objectId,externalId,scope,summary,updateToken,value")
	if err == nil {
		p.listed[key] = l
	}
	return l, err
}

// updateOrUnchanged plans writing o over cur: an update with o's
// updateToken (or cur's), or nothing when the values are the same.
func updateOrUnchanged(o applyObject, cur settingsObject) plannedWrite {
	w := plannedWrite{
		Op: config.OpUpdate, Action: actionUpdate,
		ObjectID: cur.ObjectID, SchemaID: o.SchemaID, Scope: cur.Scope, Summary: cur.Summary,
		UpdateToken: o.UpdateToken, Value: o.Value, Current: cur.Value,
	}
	if w.Scope == "" {
		w.Scope = o.Scope
	}
	if w.UpdateToken == "" {
		w.UpdateToken = cur.UpdateToken
	}
	if reflect.DeepEqual(o.Value, cur.Value) {
		w.Op, w.Action = "", actionUnchanged
	}
	return w
}
