package cmd

import (
	"fmt"
	"strings"
)

// settingsPosition is where an object goes among the objects of an ordered
// schema at one scope. Kind is "first", "last", "after" or "before"; Ref
// (an objectId or an externalId) belongs to the last two. The zero value
// means no position: a create goes last and an update keeps its place.
type settingsPosition struct{ Kind, Ref string }

const positionForms = "first, last, after:<ref>, before:<ref>"

// parsePosition reads a position: "", first, last, after:<ref> or
// before:<ref>, where <ref> is everything after the first colon.
func parsePosition(s string) (settingsPosition, error) {
	switch s {
	case "":
		return settingsPosition{}, nil
	case "first", "last":
		return settingsPosition{Kind: s}, nil
	}
	for _, kind := range []string{"after", "before"} {
		if ref, ok := strings.CutPrefix(s, kind+":"); ok && ref != "" {
			return settingsPosition{Kind: kind, Ref: ref}, nil
		}
	}
	return settingsPosition{}, fmt.Errorf("position %q is not one of %s", s, positionForms)
}

func (p settingsPosition) String() string {
	if p.Ref == "" {
		return p.Kind
	}
	return p.Kind + ":" + p.Ref
}

// orderEntry is one object in a scope's order. Planned is 0 for an object
// that exists, or the 1-based plan index of a create planned earlier.
type orderEntry struct {
	ObjectID, ExternalID string
	Planned              int
}

// placement is what a write sends to put an object where it was asked to
// go, and the 1-based places it moves between. Current is 0 for a create.
// AnchorPlanned is 0, or the plan index whose objectId fills the anchor
// when the write executes.
type placement struct {
	InsertAfter, InsertBefore *string
	Current, New              int
	AnchorPlanned             int
}

// positionChange is the move a report shows.
type positionChange struct {
	Current int `json:"current,omitempty" yaml:"current,omitempty"`
	New     int `json:"new" yaml:"new"`
}

// orderOf is the order of listed objects, in listing order.
func orderOf(listed []settingsObject) []orderEntry {
	order := make([]orderEntry, len(listed))
	for i, l := range listed {
		order[i] = orderEntry{ObjectID: l.ObjectID, ExternalID: l.ExternalID}
	}
	return order
}

// requireOrdered refuses a position on a schema that has no order.
func requireOrdered(schemaID string, shape settingsSchemaShape, pos settingsPosition) error {
	if pos.Kind != "" && !shape.Ordered {
		return &UsageError{Err: fmt.Errorf("position works only on ordered schemas; %s is not ordered", schemaID)}
	}
	return nil
}

// placeObject works out what to send to put an object at pos in order, the
// scope's objects as the server lists them. selfID is the object being
// moved, or "" for a create.
func placeObject(order []orderEntry, selfID string, pos settingsPosition) (placement, error) {
	var pl placement
	others := make([]orderEntry, 0, len(order))
	for i, e := range order {
		if selfID != "" && e.ObjectID == selfID {
			pl.Current = i + 1
			continue
		}
		others = append(others, e)
	}
	if pos.Kind == "" {
		pl.New = pl.Current
		if selfID == "" {
			pl.New = len(others) + 1
		}
		return pl, nil
	}
	at := -1 // index of the anchor among others
	if pos.Ref != "" {
		var err error
		if at, err = anchorIndex(order, others, selfID, pos); err != nil {
			return pl, err
		}
	}
	update := selfID != ""
	empty := ""
	switch pos.Kind {
	case "first":
		pl.InsertAfter, pl.New = &empty, 1
	case "last":
		pl.New = len(others) + 1
		if update {
			pl.InsertBefore = &empty
		}
	case "after":
		pl.InsertAfter, pl.New = &others[at].ObjectID, at+2
	case "before":
		pl.New = at + 1
		switch {
		case update:
			pl.InsertBefore = &others[at].ObjectID
		case at == 0:
			pl.InsertAfter = &empty
		default:
			pl.InsertAfter = &others[at-1].ObjectID
		}
	}
	return pl, nil
}

// anchorIndex finds pos's anchor, an index into others (the order without
// self).
func anchorIndex(order, others []orderEntry, selfID string, pos settingsPosition) (int, error) {
	anchor, ok := findEntry(order, pos.Ref)
	if !ok {
		return 0, withHint(fmt.Errorf("position %s: no object %q at this scope", pos, pos.Ref),
			"List the objects and their order: dtmgd get settings --schema <schema-id> --scope <scope>")
	}
	if selfID != "" && anchor.ObjectID == selfID {
		return 0, &UsageError{Err: fmt.Errorf("position %s: an object cannot be placed relative to itself", pos)}
	}
	for i, o := range others {
		if o.ObjectID == anchor.ObjectID {
			return i, nil
		}
	}
	return 0, fmt.Errorf("position %s: no object %q at this scope", pos, pos.Ref)
}

// findEntry is the entry whose objectId is ref, else the one whose
// externalId is.
func findEntry(order []orderEntry, ref string) (orderEntry, bool) {
	for _, e := range order {
		if e.ObjectID == ref {
			return e, true
		}
	}
	for _, e := range order {
		if e.ExternalID == ref {
			return e, true
		}
	}
	return orderEntry{}, false
}

// setPlacement records pl on w: the fields the request carries, and the
// move the report shows.
func (w *plannedWrite) setPlacement(pl placement) {
	w.InsertAfter, w.InsertBefore = pl.InsertAfter, pl.InsertBefore
	w.Position = &positionChange{Current: pl.Current, New: pl.New}
}

// addPlacement puts w's position fields into a request body: a create
// carries insertAfter only, an update either one.
func (w plannedWrite) addPlacement(body map[string]interface{}) {
	if w.InsertAfter != nil {
		body["insertAfter"] = *w.InsertAfter
	}
	if w.InsertBefore != nil {
		body["insertBefore"] = *w.InsertBefore
	}
}
