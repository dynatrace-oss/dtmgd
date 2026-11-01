package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const (
	orderedSchema = "builtin:test.ordered"
	orderedDef    = "GET /e/env1/api/v2/settings/schemas/" + orderedSchema
	orderedList   = objectsPath + " schemaIds=" + orderedSchema
)

func TestParsePosition(t *testing.T) {
	good := map[string]settingsPosition{
		"":              {},
		"first":         {Kind: "first"},
		"last":          {Kind: "last"},
		"after:abc:def": {Kind: "after", Ref: "abc:def"},
		"before:x":      {Kind: "before", Ref: "x"},
	}
	for in, want := range good {
		if got, err := parsePosition(in); err != nil || got != want {
			t.Errorf("parsePosition(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"middle", "after:", "before", "first:x"} {
		_, err := parsePosition(in)
		if err == nil || !strings.Contains(err.Error(), "first, last, after:<ref>, before:<ref>") {
			t.Errorf("parsePosition(%q) error = %v, want one naming the four forms", in, err)
		}
	}
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestPlaceObject(t *testing.T) {
	order := []orderEntry{{ObjectID: "o1", ExternalID: "x1"}, {ObjectID: "o2"}, {ObjectID: "o3"}}
	cases := []struct {
		name          string
		self          string
		pos           settingsPosition
		after, before string
		cur, nw       int
	}{
		{"create first", "", settingsPosition{Kind: "first"}, "", "<nil>", 0, 1},
		{"create last", "", settingsPosition{Kind: "last"}, "<nil>", "<nil>", 0, 4},
		{"create none", "", settingsPosition{}, "<nil>", "<nil>", 0, 4},
		{"create before first", "", settingsPosition{Kind: "before", Ref: "o1"}, "", "<nil>", 0, 1},
		{"create before o3", "", settingsPosition{Kind: "before", Ref: "o3"}, "o2", "<nil>", 0, 3},
		{"create after externalId", "", settingsPosition{Kind: "after", Ref: "x1"}, "o1", "<nil>", 0, 2},
		{"update o3 first", "o3", settingsPosition{Kind: "first"}, "", "<nil>", 3, 1},
		{"update o1 last", "o1", settingsPosition{Kind: "last"}, "<nil>", "", 1, 3},
		{"update o2 before o1", "o2", settingsPosition{Kind: "before", Ref: "o1"}, "<nil>", "o1", 2, 1},
		{"update o1 after o3", "o1", settingsPosition{Kind: "after", Ref: "o3"}, "o3", "<nil>", 1, 3},
		{"update keeps place", "o2", settingsPosition{}, "<nil>", "<nil>", 2, 2},
	}
	for _, c := range cases {
		got, err := placeObject(order, c.self, c.pos)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if str(got.InsertAfter) != c.after || str(got.InsertBefore) != c.before || got.Current != c.cur || got.New != c.nw {
			t.Errorf("%s: after %s before %s current %d new %d; want %s %s %d %d", c.name,
				str(got.InsertAfter), str(got.InsertBefore), got.Current, got.New, c.after, c.before, c.cur, c.nw)
		}
	}
	var usage *UsageError
	if _, err := placeObject(order, "o1", settingsPosition{Kind: "after", Ref: "o1"}); !errors.As(err, &usage) {
		t.Errorf("a position after itself: %v, want a UsageError", err)
	}
	_, err := placeObject(order, "", settingsPosition{Kind: "after", Ref: "nope"})
	if err == nil || !strings.Contains(err.Error(), `no object "nope" at this scope`) || !strings.Contains(explicitHint(err), "dtmgd get settings --schema") {
		t.Errorf("a missing anchor: %v (hint %q)", err, explicitHint(err))
	}
}

// orderedRoutes serves the ordered schema with objects o1 (ext x1), o2, o3.
func orderedRoutes() map[string]route {
	return map[string]route{
		orderedDef: {Body: `{"multiObject":true,"ordered":true}`},
		orderedList: {Body: `{"items":[
			{"objectId":"o1","externalId":"x1","scope":"environment","updateToken":"t1","value":{"name":"one"}},
			{"objectId":"o2","externalId":"x2","scope":"environment","updateToken":"t2","value":{"name":"two"}},
			{"objectId":"o3","externalId":"x3","scope":"environment","updateToken":"t3","value":{"name":"three"}}],"totalCount":3}`},
		postObjects:        {Body: `[{"code":200,"objectId":"new-1"}]`},
		objPrefix + "o1":   {Body: `{"objectId":"o1","schemaId":"builtin:test.ordered","scope":"environment","updateToken":"t1","value":{"name":"one"}}`},
		objPrefix + "o2":   {Body: `{"objectId":"o2","schemaId":"builtin:test.ordered","scope":"environment","updateToken":"t2","value":{"name":"two"}}`},
		objPrefix + "o3":   {Body: `{"objectId":"o3","schemaId":"builtin:test.ordered","scope":"environment","updateToken":"t3","value":{"name":"three"}}`},
		putPrefix + "o1":   {Body: `{"code":200,"objectId":"o1"}`},
		putPrefix + "o2":   {Body: `{"code":200,"objectId":"o2"}`},
		putPrefix + "o3":   {Body: `{"code":200,"objectId":"o3"}`},
		clusterVersionPath: {Body: nodeList("1.300.0.20250101-000000")},
	}
}

// orderedApply writes a one-object apply file and runs apply on it.
func orderedApply(t *testing.T, cfg, doc string, extra ...string) (cliResult, writeReport) {
	t.Helper()
	return applyJSON(t, cfg, tempFile(t, doc), extra...)
}

func TestApplyPositionBodies(t *testing.T) {
	creates := map[string]string{
		"first": "", "before:o1": "", "before:o3": "o2", "after:x1": "o1", "after:o3": "o3",
	}
	for pos, want := range creates {
		api, cfg := applyFake(t, "readwrite-all", orderedRoutes())
		r, rep := orderedApply(t, cfg, "externalId: fresh\nschemaId: "+orderedSchema+"\nscope: environment\nposition: "+pos+"\nvalue: {name: new}\n")
		if r.Code != 0 || actions(rep) != "created" {
			t.Fatalf("%s: %s", pos, r)
		}
		var items []map[string]interface{}
		if err := json.Unmarshal([]byte(api.Bodies(postObjects)[0]), &items); err != nil {
			t.Fatal(err)
		}
		if got, ok := items[0]["insertAfter"]; !ok || got != want {
			t.Errorf("create %s: insertAfter %v (present %v), want %q", pos, got, ok, want)
		}
	}
	api, cfg := applyFake(t, "readwrite-all", orderedRoutes())
	orderedApply(t, cfg, "externalId: fresh\nschemaId: "+orderedSchema+"\nscope: environment\nposition: last\nvalue: {name: new}\n")
	if b := api.Bodies(postObjects)[0]; strings.Contains(b, "insertAfter") || strings.Contains(b, "insertBefore") {
		t.Errorf("create last sent a position: %s", b)
	}

	updates := []struct{ doc, id, key, want string }{
		{"position: first", "o3", "insertAfter", ""},
		{"position: last", "o1", "insertBefore", ""},
		{"position: before:o1", "o2", "insertBefore", "o1"},
		{"position: after:x3", "o1", "insertAfter", "o3"},
	}
	for _, u := range updates {
		api, cfg := applyFake(t, "readwrite-all", orderedRoutes())
		r, rep := orderedApply(t, cfg, "objectId: "+u.id+"\nschemaId: "+orderedSchema+"\nscope: environment\n"+u.doc+"\nvalue: {name: moved}\n")
		if r.Code != 0 || actions(rep) != "updated" {
			t.Fatalf("%s: %s", u.doc, r)
		}
		b := putBody(t, api, u.id)
		other := "insertBefore"
		if u.key == other {
			other = "insertAfter"
		}
		if got, ok := b[u.key]; !ok || got != u.want {
			t.Errorf("%s: %s = %v (present %v), want %q", u.doc, u.key, got, ok, u.want)
		}
		if _, both := b[other]; both {
			t.Errorf("%s: PUT body holds both insertAfter and insertBefore: %v", u.doc, b)
		}
	}
}

func TestApplyPositionUnchangedPlaceIsUnchanged(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", orderedRoutes())
	r, rep := orderedApply(t, cfg, "objectId: o1\nschemaId: "+orderedSchema+"\nscope: environment\nposition: first\nvalue: {name: one}\n")
	if r.Code != 0 || actions(rep) != "unchanged" || api.Hits(putPrefix+"o1") != 0 {
		t.Errorf("%s", r)
	}
}

func TestApplyPositionMoveWithSameValueIsUpdate(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", orderedRoutes())
	doc := "objectId: o3\nschemaId: " + orderedSchema + "\nscope: environment\nposition: first\nvalue: {name: three}\n"
	r, rep := orderedApply(t, cfg, doc, "--dry-run")
	if r.Code != 0 || actions(rep) != "update" {
		t.Fatalf("%s", r)
	}
	if p := rep.Results[0].Position; p == nil || p.Current != 3 || p.New != 1 {
		t.Errorf("position %+v, want current 3 new 1", p)
	}
	if api.Hits(putPrefix+"o3") != 0 {
		t.Error("a dry run sent a PUT")
	}
	r, rep = orderedApply(t, cfg, doc)
	if r.Code != 0 || actions(rep) != "updated" || api.Hits(putPrefix+"o3") != 1 {
		t.Errorf("%s", r)
	}
}

func TestPositionMoveAndValueChangeInOnePut(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", orderedRoutes())
	r, _ := orderedApply(t, cfg, "objectId: o2\nschemaId: "+orderedSchema+"\nscope: environment\nposition: first\nvalue: {name: renamed}\n")
	b := putBody(t, api, "o2")
	if r.Code != 0 || api.Hits(putPrefix+"o2") != 1 || b["insertAfter"] != "" || b["value"] == nil {
		t.Errorf("%s: PUT %v (%d)", r, b, api.Hits(putPrefix+"o2"))
	}
}

func TestPositionRefusedOnUnorderedSchema(t *testing.T) {
	routes := createRoutes()
	routes[objectsPath+" schemaIds=builtin:test.multi"] = route{Body: `{"items":[],"totalCount":0}`}
	api, cfg := applyFake(t, "readwrite-all", routes)
	r, _ := orderedApply(t, cfg, "schemaId: builtin:test.multi\nscope: environment\nposition: first\nvalue: {name: x}\n")
	if r.Code != 2 || !strings.Contains(r.Stderr, "position works only on ordered schemas; builtin:test.multi is not ordered") || api.Hits(postObjects) != 0 {
		t.Errorf("%s", r)
	}
}

func TestCreateSettingsPositionFlag(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", orderedRoutes())
	r := runCLI(t, cfg, "create", "settings", "-f", tempFile(t, "name: fresh\n"), "--schema", orderedSchema, "--scope", "environment", "--position", "after:x2")
	if r.Code != 0 {
		t.Fatalf("%s", r)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(api.Bodies(postObjects)[0]), &items); err != nil {
		t.Fatal(err)
	}
	if items[0]["insertAfter"] != "o2" {
		t.Errorf("body %v, want insertAfter o2", items[0])
	}
}

func TestPositionAnchorMissingInOneContextWritesNothing(t *testing.T) {
	withAnchor := newFakeAPI(t, orderedRoutes())
	bare := orderedRoutes()
	bare[orderedList] = route{Body: `{"items":[{"objectId":"o9","externalId":"x9","scope":"environment","updateToken":"t9","value":{"name":"nine"}}],"totalCount":1}`}
	without := newFakeAPI(t, bare)
	cfg := withFakeEnvLevels(t, map[string]string{"a": "readwrite-all", "b": "readwrite-all"}, fakeEnv{"a", withAnchor}, fakeEnv{"b", without})
	r := runCLI(t, cfg, "create", "settings", "-f", tempFile(t, "name: fresh\n"), "--schema", orderedSchema, "--scope", "environment", "--position", "after:x1", "--env", "a;b")
	if r.Code != 1 || !strings.Contains(r.Stderr, `no object "x1" at this scope`) {
		t.Errorf("exit %d, want 1 naming the missing anchor\n%s", r.Code, r)
	}
	for _, api := range []*fakeAPI{withAnchor, without} {
		if api.Hits(postObjects) != 0 || api.Hits(putPrefix+"o1") != 0 {
			t.Errorf("a write was sent: %s", r)
		}
	}
}

func TestApplyWithoutPositionSendsNoPositionFields(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", orderedRoutes())
	orderedApply(t, cfg, "externalId: fresh\nschemaId: "+orderedSchema+"\nscope: environment\nvalue: {name: new}\n")
	orderedApply(t, cfg, "objectId: o2\nschemaId: "+orderedSchema+"\nscope: environment\nvalue: {name: renamed}\n")
	bodies := append(api.Bodies(postObjects), api.Bodies(putPrefix+"o2")...)
	if len(bodies) != 2 {
		t.Fatalf("%d bodies, want 2", len(bodies))
	}
	for _, b := range bodies {
		if strings.Contains(b, "insertAfter") || strings.Contains(b, "insertBefore") {
			t.Errorf("body has a position: %s", b)
		}
	}
}

func TestApplyBadPositionIsAUsageErrorBeforeAnyRequest(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", orderedRoutes())
	doc := "objectId: o1\nschemaId: " + orderedSchema + "\nscope: environment\nvalue: {name: one}\n---\n" +
		"objectId: o2\nschemaId: " + orderedSchema + "\nscope: environment\nposition: middle\nvalue: {name: two}\n"
	r, _ := orderedApply(t, cfg, doc)
	if r.Code != 2 || !strings.Contains(r.Stderr, "object 1") || !strings.Contains(r.Stderr, "middle") || api.totalHits() != 0 {
		t.Errorf("%s (%d requests)", r, api.totalHits())
	}
}

func TestApplyPositionByObjectIDRefusedOnUnorderedSchema(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", map[string]route{
		schemaPathS:         {Body: `{"schemaId":"builtin:test.multi","multiObject":true}`},
		objPrefix + "obj-1": {Body: `{"objectId":"obj-1","schemaId":"builtin:test.multi","scope":"environment","updateToken":"t","value":{"name":"a"}}`},
		putPrefix + "obj-1": {Body: `{"code":200,"objectId":"obj-1"}`},
	})
	r, _ := orderedApply(t, cfg, "objectId: obj-1\nschemaId: builtin:test.multi\nscope: environment\nposition: first\nvalue: {name: b}\n")
	if r.Code != 2 || !strings.Contains(r.Stderr, "position works only on ordered schemas") || api.Hits(putPrefix+"obj-1") != 0 {
		t.Errorf("%s", r)
	}
}

func TestDryRunReportsThePositionBlock(t *testing.T) {
	_, cfg := applyFake(t, "readwrite-all", orderedRoutes())
	r := runCLI(t, cfg, "create", "settings", "-f", tempFile(t, "name: fresh\n"), "--schema", orderedSchema,
		"--scope", "environment", "--position", "first", "--dry-run", "-o", "json")
	var rep struct {
		Results []struct {
			Position map[string]interface{} `json:"position"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &rep); err != nil || len(rep.Results) != 1 {
		t.Fatalf("%v\n%s", err, r)
	}
	pos := rep.Results[0].Position
	if _, has := pos["current"]; has || pos["new"] != float64(1) {
		t.Errorf("create position %v, want new 1 and no current", pos)
	}

	doc := "objectId: o3\nschemaId: " + orderedSchema + "\nscope: environment\nposition: first\nvalue: {name: three}\n"
	r = runCLI(t, cfg, "apply", "-f", tempFile(t, doc), "--dry-run", "-o", "yaml")
	if r.Code != 0 || !strings.Contains(r.Stdout, "position:\n      current: 3\n      new: 1") {
		t.Errorf("update yaml lacks the position block:\n%s", r)
	}
}

// anchorFile creates a (first) and then b (after:a), in one file.
const anchorFile = "externalId: a\nschemaId: " + orderedSchema + "\nscope: environment\nposition: first\nvalue: {name: a}\n---\n" +
	"externalId: b\nschemaId: " + orderedSchema + "\nscope: environment\nposition: after:a\nvalue: {name: b}\n"

func anchorRoutes(seq ...string) map[string]route {
	routes := orderedRoutes()
	routes[postObjects] = route{Seq: seq}
	return routes
}

func TestPositionAnchorEarlierInFile(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", anchorRoutes(`[{"code":200,"objectId":"new-a"}]`, `[{"code":200,"objectId":"new-b"}]`))
	r, rep := orderedApply(t, cfg, anchorFile)
	if r.Code != 0 || actions(rep) != "created,created" {
		t.Fatalf("%s", r)
	}
	bodies := api.Bodies(postObjects)
	if len(bodies) != 2 {
		t.Fatalf("%d POSTs, want 2", len(bodies))
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(bodies[1]), &items); err != nil {
		t.Fatal(err)
	}
	if items[0]["insertAfter"] != "new-a" {
		t.Errorf("second body %v, want insertAfter new-a", items[0])
	}

	_, cfg = applyFake(t, "readwrite-all", orderedRoutes())
	r, rep = orderedApply(t, cfg, anchorFile, "--dry-run")
	if r.Code != 0 || len(rep.Results) != 2 || rep.Results[0].Position.New != 1 || rep.Results[1].Position.New != 2 {
		t.Errorf("dry run positions: %s", r)
	}
}

func TestPositionAnchorNotWrittenFailsOnlyItsDependent(t *testing.T) {
	routes := orderedRoutes()
	routes[postObjects] = route{Status: 400, Body: `[{"code":400,"error":{"code":400,"message":"bad value"}}]`}
	api, cfg := applyFake(t, "readwrite-all", routes)
	r, rep := orderedApply(t, cfg, anchorFile)
	if actions(rep) != "failed,failed" || rep.Results[1].Error == nil || rep.Results[1].Error.Message != "its position anchor was not written" {
		t.Fatalf("%s", r)
	}
	if api.Hits(postObjects) != 1 {
		t.Errorf("%d POSTs, want 1", api.Hits(postObjects))
	}
}

func TestPositionAnchorUnderValidateOnly(t *testing.T) {
	api, cfg := applyFake(t, "readwrite-all", anchorRoutes(`[{"code":200}]`))
	r, rep := orderedApply(t, cfg, anchorFile, "--validate-only")
	if r.Code != 0 || actions(rep) != "validated,validated" {
		t.Fatalf("%s", r)
	}
	bodies := api.Bodies(postObjects)
	if len(bodies) != 2 || strings.Contains(bodies[1], "insertAfter") {
		t.Errorf("second body %v, want no insertAfter", bodies)
	}
	if want := "validated without its position: its anchor is created by this file"; rep.Results[1].Note != want || rep.Results[0].Note != "" {
		t.Errorf("notes %q, %q", rep.Results[0].Note, rep.Results[1].Note)
	}
}
