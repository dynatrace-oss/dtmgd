package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

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
	// Position places the object in an ordered schema: first, last,
	// after:<ref> or before:<ref>.
	Position string `json:"position"`
	// pos is Position parsed, set by check.
	pos settingsPosition
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
updateToken?, position?} object, an array of them, or a list as
get settings -o yaml prints it, in one or several "---" documents. An
exported object applies back as it is, an agent-mode export (the envelope)
included.

For each object apply first finds out whether it exists, with GETs only:
  - with objectId: that object, which must have the file's schemaId and
    scope (if it is not found, apply creates it, which moves a file from one
    environment to another);
  - without: for a schema holding one object per scope, the object at the
    scope; otherwise the object with the same externalId, or with the same
    value of the schema's key property.
Two objects of one file may not write the same object.
An existing object is updated, with the updateToken from the file (which
catches changes since the export) or from the GET; one whose value is the
same is left unchanged. Anything else is created. Give objects of
multi-object schemas an externalId to make repeated applies update them
instead of creating duplicates.

In an ordered schema a new object goes last and an updated one keeps its
place, unless the object's position key says: first, last, after:<ref> or
before:<ref>, where <ref> is the objectId or externalId of an object at the
same scope, or the externalId of an object created earlier in the file.

A secret property holding the cluster's mask, as exports do, keeps the
stored secret on an update, but cannot create an object. Reports show
secrets as <secret>.

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
			objs, err := readApplyFile(opts.file, a.input())
			if err != nil {
				return err
			}
			return a.runWrites(writeSpec{
				ClusterToo:   true,
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
	docs, err := yamlDocuments(data)
	if err != nil {
		return nil, &UsageError{Err: fmt.Errorf("%s: not YAML or JSON: %w", path, err)}
	}
	var list []interface{}
	for _, doc := range docs {
		items, err := applyItems(doc)
		if err != nil {
			return nil, &UsageError{Err: fmt.Errorf("%s: %w", path, err)}
		}
		list = append(list, items...)
	}
	if len(list) == 0 {
		return nil, &UsageError{Err: fmt.Errorf("%s holds no objects", path)}
	}
	var objs []applyObject
	if err := jsonRoundTrip(list, &objs); err != nil {
		return nil, &UsageError{Err: fmt.Errorf("%s: %w", path, err)}
	}
	for i := range objs {
		if err := objs[i].check(); err != nil {
			return nil, &UsageError{Err: fmt.Errorf("%s: object %d: %w", path, i, err)}
		}
	}
	return objs, nil
}

// applyItems is the list of objects in one decoded document of an apply
// file. An agent envelope ({ok, result, …}, what an export in agent mode
// prints) is unwrapped first.
func applyItems(raw interface{}) ([]interface{}, error) {
	switch v := raw.(type) {
	case []interface{}:
		return v, nil
	case map[string]interface{}:
		if result, ok := v["result"]; ok && v["ok"] != nil {
			return applyItems(result)
		}
		if items, ok := v["items"].([]interface{}); ok {
			return items, nil
		}
		return []interface{}{v}, nil
	}
	return nil, errors.New("want an object, an array of objects, or a list with items")
}

// check refuses an object apply cannot place, and parses its position.
func (o *applyObject) check() error {
	switch {
	case o.SchemaID == "":
		return errors.New("no schemaId")
	case o.ObjectID == "" && o.Scope == "":
		return errors.New("neither objectId nor scope")
	case o.Value == nil:
		return errors.New("no value")
	}
	pos, err := parsePosition(o.Position)
	o.pos = pos
	return err
}

// applyPlanner plans one context's apply, caching schema definitions and
// listings across the file's objects.
type applyPlanner struct {
	c      *client.Client
	shapes map[string]settingsSchemaShape
	listed map[string][]settingsObject
	// order is each scope's objects in server order, for placing objects;
	// separate from listed, which the lookups use.
	order map[string][]orderEntry
	// cur is the 1-based plan index of the object being planned.
	cur int
}

// planApply resolves what apply would do with each object in one context.
// An object whose state cannot be read fails the whole plan, so nothing is
// written on a guess.
func planApply(c *client.Client, objs []applyObject) ([]plannedWrite, error) {
	p := &applyPlanner{c: c, shapes: map[string]settingsSchemaShape{}, listed: map[string][]settingsObject{}, order: map[string][]orderEntry{}}
	writes := make([]plannedWrite, 0, len(objs))
	targets := map[string]int{}
	for i, o := range objs {
		p.cur = i + 1
		w, err := p.plan(o)
		if err != nil {
			return nil, err
		}
		if t := p.target(w); t != "" {
			if j, dup := targets[t]; dup {
				return nil, &UsageError{Err: fmt.Errorf("objects %d and %d of the file write the same object; keep one of them", j, i)}
			}
			targets[t] = i
		}
		writes = append(writes, w)
	}
	p.finalPlaces(writes)
	p.attachShapes(writes)
	return writes, nil
}

// attachShapes gives each write the schema definition the planner read for
// it, so the secret guard reuses it.
func (p *applyPlanner) attachShapes(writes []plannedWrite) {
	for i := range writes {
		if s, ok := p.shapes[writes[i].SchemaID]; ok {
			writes[i].shape = &s
		}
	}
}

// finalPlaces sets each positioned write's reported New place from the
// virtual order after the whole file: a create is found by its plan index,
// an existing object by its objectId. Current of an existing object is its
// place in the server's listing before the run (the planning-time Current,
// kept for the unchanged check, follows earlier writes of the file), and
// what the requests carry stays as resolved at planning time.
func (p *applyPlanner) finalPlaces(writes []plannedWrite) {
	for i, w := range writes {
		if w.Position == nil {
			continue
		}
		p.setFinalPlace(i, w)
	}
}

// setFinalPlace sets New of write i, and Current of an existing object.
func (p *applyPlanner) setFinalPlace(i int, w plannedWrite) {
	for j, e := range p.order[orderKey(w.SchemaID, w.Scope)] {
		if (w.ObjectID == "" && e.Planned == i+1) || (w.ObjectID != "" && e.ObjectID == w.ObjectID) {
			w.Position.New = j + 1
			break
		}
	}
	if w.ObjectID != "" {
		if c := p.listedPlace(w.SchemaID, w.ObjectID); c > 0 {
			w.Position.Current = c
		}
	}
}

// listedPlace is the 1-based place of objectID in the listing the server
// gave for schemaID, before any write of the run; 0 when it was not listed.
func (p *applyPlanner) listedPlace(schemaID, objectID string) int {
	for key, l := range p.listed {
		if !strings.HasPrefix(key, schemaID+"\x00") {
			continue
		}
		for i, o := range l {
			if o.ObjectID == objectID {
				return i + 1
			}
		}
	}
	return 0
}

// target names the object w writes, so two writes to one object are found
// before anything is sent: the objectId of an update, or for a create the
// key the server would match it on (the scope of a one-per-scope schema,
// the externalId, the key property). "" for a create no server lookup
// matches.
func (p *applyPlanner) target(w plannedWrite) string {
	if w.ObjectID != "" {
		return "object\x00" + w.ObjectID
	}
	shape := p.shapes[w.SchemaID]
	base := w.SchemaID + "\x00" + w.Scope + "\x00"
	switch {
	case !shape.MultiObject:
		return "scope\x00" + base
	case w.ExternalID != "":
		return "external\x00" + base + w.ExternalID
	case shape.KeyProperty != "" && w.Value[shape.KeyProperty] != nil:
		return fmt.Sprintf("key\x00%s%v", base, w.Value[shape.KeyProperty])
	}
	return ""
}

// plan resolves one object: by its objectId, else by a lookup.
func (p *applyPlanner) plan(o applyObject) (plannedWrite, error) {
	if o.ObjectID == "" {
		return p.planByLookup(o)
	}
	var cur settingsObject
	err := settingsGet(p.c, settingsObjectsPath+"/"+url.PathEscape(o.ObjectID), nil, &cur)
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
	if err := sameObject(o, cur); err != nil {
		return plannedWrite{}, err
	}
	pl, err := p.placeExisting(o, cur)
	if err != nil {
		return plannedWrite{}, err
	}
	return updateOrUnchanged(o, cur, pl), nil
}

// sameObject refuses a file object whose objectId names an object of
// another schema or scope: the PUT would change that object, wherever the
// file says it goes.
func sameObject(o applyObject, cur settingsObject) error {
	hint := "To put this value at another scope, remove objectId (and updateToken) from the file; apply then creates or updates the object there."
	switch {
	case cur.SchemaID != "" && cur.SchemaID != o.SchemaID:
		return withHint(&UsageError{Err: fmt.Errorf("objectId %s is a %s object, but the file says %s", o.ObjectID, cur.SchemaID, o.SchemaID)}, hint)
	case o.Scope != "" && cur.Scope != "" && canonicalScope(o.Scope) != canonicalScope(cur.Scope):
		return withHint(&UsageError{Err: fmt.Errorf("objectId %s is stored at %s, but the file says scope %s", o.ObjectID, cur.Scope, o.Scope)}, hint)
	}
	return nil
}

// canonicalScope is the scope with the environment's two names made one.
func canonicalScope(scope string) string {
	if scope == "tenant" {
		return "environment"
	}
	return scope
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
		pl, err := p.placeExisting(o, *match)
		if err != nil {
			return plannedWrite{}, err
		}
		return updateOrUnchanged(o, *match, pl), nil
	}
	w := plannedWrite{Op: config.OpCreate, Action: actionCreate, SchemaID: o.SchemaID, Scope: o.Scope, ExternalID: o.ExternalID, Value: o.Value, Existing: listedIDs(listed)}
	if err := p.placeNew(&w, shape, o); err != nil {
		return plannedWrite{}, err
	}
	return w, nil
}

// position is o's parsed position, as check set it.
func (o applyObject) position() settingsPosition {
	return o.pos
}

// placeNew places a create in its scope's order and records it there, so
// a later object can name it. The report and the request carry a position
// only when o asks for one.
func (p *applyPlanner) placeNew(w *plannedWrite, shape settingsSchemaShape, o applyObject) error {
	pos := o.position()
	if err := requireOrdered(o.SchemaID, shape, pos); err != nil || !shape.Ordered {
		return err
	}
	// Every create in an ordered schema is recorded in the scope's order,
	// positioned or not, because a later object may name it as an anchor.
	// The listing is cached per schema and scope (p.list), so this costs
	// one read per scope.
	order, err := p.scopeOrder(o.SchemaID, o.Scope)
	if err != nil {
		return err
	}
	pl, err := placeObject(order, "", pos)
	if err != nil {
		return err
	}
	if pos.Kind != "" {
		w.setPlacement(pl)
	}
	p.order[orderKey(o.SchemaID, o.Scope)] = moveEntry(order, "", pl.New, orderEntry{ExternalID: o.ExternalID, Planned: p.cur})
	return nil
}

// placeExisting places an existing object cur, or returns nil when o asks
// for no position. A move is recorded in the scope's order.
func (p *applyPlanner) placeExisting(o applyObject, cur settingsObject) (*placement, error) {
	pos := o.position()
	if pos.Kind == "" {
		return nil, nil
	}
	shape, err := p.shape(o.SchemaID)
	if err != nil {
		return nil, err
	}
	if err := requireOrdered(o.SchemaID, shape, pos); err != nil {
		return nil, err
	}
	scope := cur.Scope
	if scope == "" {
		scope = o.Scope
	}
	order, err := p.scopeOrder(o.SchemaID, scope)
	if err != nil {
		return nil, err
	}
	pl, err := placeObject(order, cur.ObjectID, pos)
	if err != nil {
		return &pl, err
	}
	if i := findEntry(order, cur.ObjectID); i >= 0 {
		p.order[orderKey(o.SchemaID, scope)] = moveEntry(order, cur.ObjectID, pl.New, order[i])
	}
	return &pl, nil
}

// orderKey names a scope's order; the environment's two scope names are one.
func orderKey(schemaID, scope string) string {
	return schemaID + "\x00" + canonicalScope(scope)
}

// scopeOrder is the order of schemaID's objects at scope, built once.
func (p *applyPlanner) scopeOrder(schemaID, scope string) ([]orderEntry, error) {
	key := orderKey(schemaID, scope)
	if o, ok := p.order[key]; ok {
		return o, nil
	}
	listed, err := p.list(schemaID, scope)
	if err != nil {
		return nil, err
	}
	p.order[key] = orderOf(listed)
	return p.order[key], nil
}

// findMatch is the listed object o would overwrite, or nil, in the order
// the server matches: the scope's object of a one-per-scope schema, then
// the externalId, then the key property.
func findMatch(shape settingsSchemaShape, o applyObject, listed []settingsObject) *settingsObject {
	if !shape.MultiObject {
		if len(listed) > 0 {
			return &listed[0]
		}
		return nil
	}
	if o.ExternalID != "" {
		if m := firstListed(listed, func(l settingsObject) bool { return l.ExternalID == o.ExternalID }); m != nil {
			return m
		}
	}
	if k := shape.KeyProperty; k != "" && o.Value[k] != nil {
		return firstListed(listed, func(l settingsObject) bool { return reflect.DeepEqual(o.Value[k], l.Value[k]) })
	}
	return nil
}

// firstListed is the first listed object match accepts, or nil.
func firstListed(listed []settingsObject, match func(settingsObject) bool) *settingsObject {
	for i := range listed {
		if match(listed[i]) {
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
// updateToken (or cur's), or nothing when the values are the same and pl,
// when there is one, leaves the object where it is.
func updateOrUnchanged(o applyObject, cur settingsObject, pl *placement) plannedWrite {
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
	if reflect.DeepEqual(o.Value, cur.Value) && (pl == nil || pl.New == pl.Current) {
		w.Op, w.Action = "", actionUnchanged
		return w
	}
	if pl != nil {
		w.setPlacement(*pl)
	}
	return w
}
