//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The schemas the any-schema checks use.
const (
	profileSchema      = "builtin:alerting.profile"
	notificationSchema = "builtin:problem.notifications"
	availabilitySchema = "builtin:processavailability"
	networkZoneSchema  = "builtin:networkzones.zones"
)

// maskPattern is the cluster's mask for a secret property.
var maskPattern = regexp.MustCompile(`^\*\*\*[0-9a-f]{16}\*\*\*$`)

// testScope is the scope the suite's objects live at on ctx: the cluster's
// network zones sit at environment-default, everything else at environment.
func testScope(ctx string) string {
	if w := os.Getenv("DTMGD_IT_WRITE_CLUSTER"); w != "" && w == ctx {
		return "environment-default"
	}
	return "environment"
}

// listObjects lists the objects of schema at scope on ctx.
func listObjects(t testing.TB, ctx, schema, scope string) ([]map[string]any, result) {
	t.Helper()
	r := run(t, "get", "settings", "--schema", schema, "--scope", scope, "--context", ctx, "-o", "json")
	var body map[string]any
	if r.Code != 0 || json.Unmarshal(r.Out, &body) != nil {
		return nil, r
	}
	var out []map[string]any
	list, _ := body["items"].([]any)
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, r
}

// testObjects returns the objects of schema at scope on ctx whose
// externalId starts with testWindowPrefix ("dtmgd-it-"), in listing order.
func testObjects(t *testing.T, ctx, schema, scope string) []map[string]any {
	t.Helper()
	list, r := listObjects(t, ctx, schema, scope)
	skipWithoutScope(t, r, []string{"get", "settings"})
	if r.Code != 0 {
		t.Fatalf("listing %s: %s", schema, brief(r))
	}
	return withPrefix(list)
}

// withPrefix keeps the objects the suite created.
func withPrefix(list []map[string]any) []map[string]any {
	var out []map[string]any
	for _, m := range list {
		if id, _ := m["externalId"].(string); strings.HasPrefix(id, testWindowPrefix) {
			out = append(out, m)
		}
	}
	return out
}

// deleteTestObjects deletes every dtmgd-it- object of each schema, in the
// order given, and fails the test (without stopping it) on an error.
func deleteTestObjects(t testing.TB, cfg, ctx string, schemas ...string) {
	t.Helper()
	for _, schema := range schemas {
		list, r := listObjects(t, ctx, schema, testScope(ctx))
		if r.Code == 5 {
			// No settings.read: nothing can be listed. Say so, because a
			// token that lost the scope mid-run could leave objects behind.
			t.Logf("cleanup: cannot list %s without settings.read; check for leftover %s objects by hand", schema, testWindowPrefix)
			continue
		}
		if r.Code != 0 {
			t.Errorf("cleanup: listing %s: %s", schema, brief(r))
			continue
		}
		for _, m := range withPrefix(list) {
			id, _ := m["objectId"].(string)
			if d := writeRun(t, cfg, ctx, "delete", "settings", id, "--yes"); d.Code != 0 {
				t.Errorf("cleanup: deleting a %s object: %s", schema, brief(d))
			}
		}
	}
}

// cleanupTestObjects removes the suite's leftovers now, and registers the
// same removal with t.Cleanup. Schemas are deleted in the order given, so
// list the referencing schema before the referenced one.
func cleanupTestObjects(t *testing.T, cfg, ctx string, schemas ...string) {
	t.Helper()
	// A token without settings.read skips the test, as the other tests do.
	_, r := listObjects(t, ctx, schemas[0], testScope(ctx))
	skipWithoutScope(t, r, []string{"get", "settings"})
	deleteTestObjects(t, cfg, ctx, schemas...)
	t.Cleanup(func() { deleteTestObjects(t, cfg, ctx, schemas...) })
}

// expectAction fails unless the single result of rep has action want.
func expectAction(t *testing.T, step string, rep writeReport, want string) {
	t.Helper()
	if len(rep.Results) != 1 || rep.Results[0].Action != want {
		t.Errorf("%s: %d results, first action %q, want %s", step, len(rep.Results), firstAction(rep), want)
	}
}

func firstAction(rep writeReport) string {
	if len(rep.Results) == 0 {
		return ""
	}
	return rep.Results[0].Action
}

// ---- alerting profile and notification ----

const profileExternalID = testWindowPrefix + "profile"
const notificationExternalID = testWindowPrefix + "notification"

// profileFile is an apply file for the test alerting profile.
func profileFile(t *testing.T, delay int) string {
	return writeFile(t, fmt.Sprintf(`schemaId: %s
scope: environment
externalId: %s
value:
  name: %s
  severityRules:
    - severityLevel: AVAILABILITY
      delayInMinutes: %d
      tagFilterIncludeMode: NONE
  eventFilters: []
`, profileSchema, profileExternalID, profileExternalID, delay))
}

// notificationFile is an apply file for the disabled test notification,
// which references profileID and carries a secret header. The secret is a
// made-up value.
func notificationFile(t *testing.T, externalID, profileID string) string {
	return writeFile(t, fmt.Sprintf(`schemaId: %s
scope: environment
externalId: %s
value:
  enabled: false
  type: WEBHOOK
  displayName: %s
  alertingProfile: %s
  webHookNotification:
    url: https://example.invalid/dtmgd-it
    acceptAnyCertificate: false
    notifyEventMergesEnabled: false
    notifyClosedProblems: true
    payload: '{"State":"{State}"}'
    headers:
      - name: X-Dtmgd-It
        secret: true
        secretValue: dtmgd-it-not-a-real-secret
`, notificationSchema, externalID, externalID, profileID))
}

// onlyObject returns the one test object of schema, failing otherwise.
func onlyObject(t *testing.T, ctx, schema string) map[string]any {
	t.Helper()
	got := testObjects(t, ctx, schema, "environment")
	if len(got) != 1 {
		t.Fatalf("%d dtmgd-it- objects of %s, want 1", len(got), schema)
	}
	return got[0]
}

// headerMask is the secret header's value of a notification object.
func headerMask(obj map[string]any) string {
	value, _ := obj["value"].(map[string]any)
	hook, _ := value["webHookNotification"].(map[string]any)
	headers, _ := hook["headers"].([]any)
	if len(headers) == 0 {
		return ""
	}
	h, _ := headers[0].(map[string]any)
	s, _ := h["secretValue"].(string)
	return s
}

// exportWith writes obj, as an export, to an apply file after mutate has
// changed it.
func exportWith(t *testing.T, obj map[string]any, mutate func(map[string]any)) string {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	var cp map[string]any
	if err := json.Unmarshal(data, &cp); err != nil {
		t.Fatal(err)
	}
	mutate(cp)
	out, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	return writeFile(t, string(out))
}

func TestAlertingProfileAndNotificationLifecycle(t *testing.T) {
	cfg, ctx := writeConfig(t)
	cleanupTestObjects(t, cfg, ctx, notificationSchema, profileSchema)

	created := applied(t, cfg, ctx, "apply", "-f", profileFile(t, 0))
	expectAction(t, "create profile", created, "created")
	profileID := created.Results[0].ObjectID
	expectAction(t, "re-apply profile", applied(t, cfg, ctx, "apply", "-f", profileFile(t, 0)), "unchanged")
	expectAction(t, "update profile", applied(t, cfg, ctx, "apply", "-f", profileFile(t, 5)), "updated")

	notified := applied(t, cfg, ctx, "apply", "-f", notificationFile(t, notificationExternalID, profileID))
	expectAction(t, "create notification", notified, "created")
	// A file holding the real secret cannot be compared with the stored one.
	expectAction(t, "re-apply notification", applied(t, cfg, ctx, "apply", "-f", notificationFile(t, notificationExternalID, profileID)), "updated")

	checkMaskKept(t, cfg, ctx)
	checkCopyRefused(t, cfg, ctx)
	checkDeleteRefused(t, cfg, ctx, profileID)

	for _, schema := range []string{notificationSchema, profileSchema} {
		obj := onlyObject(t, ctx, schema)
		id, _ := obj["objectId"].(string)
		expectAction(t, "delete "+schema, applied(t, cfg, ctx, "delete", "settings", id, "--yes"), "deleted")
		if left := testObjects(t, ctx, schema, "environment"); len(left) != 0 {
			t.Errorf("%d %s objects are still listed after the delete", len(left), schema)
		}
	}
}

// checkMaskKept applies the notification's export back with a new display
// name and checks that the stored secret stays: the cluster's mask is
// salted, so a newly stored secret would change it.
func checkMaskKept(t *testing.T, cfg, ctx string) {
	t.Helper()
	obj := onlyObject(t, ctx, notificationSchema)
	before := headerMask(obj)
	if !maskPattern.MatchString(before) {
		t.Fatalf("the exported secret is not a mask (%d characters)", len(before))
	}
	file := exportWith(t, obj, func(m map[string]any) {
		value, _ := m["value"].(map[string]any)
		value["displayName"] = notificationExternalID + "-renamed"
	})
	expectAction(t, "apply the export", applied(t, cfg, ctx, "apply", "-f", file), "updated")
	if headerMask(onlyObject(t, ctx, notificationSchema)) != before {
		t.Error("the secret's mask changed: the export's mask was stored as a new secret")
	}
}

// checkCopyRefused applies a copy of the export without its objectId and
// with a new externalId: a create from a mask, refused with exit 2.
func checkCopyRefused(t *testing.T, cfg, ctx string) {
	t.Helper()
	file := exportWith(t, onlyObject(t, ctx, notificationSchema), func(m map[string]any) {
		delete(m, "objectId")
		delete(m, "updateToken")
		m["externalId"] = notificationExternalID + "-copy"
	})
	r := writeRun(t, cfg, ctx, "apply", "-f", file)
	if r.Code != 2 || !strings.Contains(r.Err, "masked secret") {
		t.Errorf("a copy made from an export: %s, want exit 2 naming the masked secret (stderr mentions it: %v)", brief(r), strings.Contains(r.Err, "masked secret"))
	}
	if n := len(testObjects(t, ctx, notificationSchema, "environment")); n != 1 {
		t.Errorf("%d test notifications are listed after the refused copy, want 1", n)
	}
}

// checkDeleteRefused deletes the profile while the notification names it:
// exit 1, and the profile stays.
func checkDeleteRefused(t *testing.T, cfg, ctx, profileID string) {
	t.Helper()
	if r := writeRun(t, cfg, ctx, "delete", "settings", profileID, "--yes"); r.Code != 1 {
		t.Errorf("deleting a referenced profile: %s, want exit 1", brief(r))
	}
	if n := len(testObjects(t, ctx, profileSchema, "environment")); n != 1 {
		t.Errorf("%d test profiles are listed after the refused delete, want 1", n)
	}
}

// ---- ordered rules ----

const orderPrefix = testWindowPrefix + "order-"

// orderFile is an apply file for a disabled process availability rule;
// position is omitted when empty.
func orderFile(t *testing.T, letter, position string) string {
	pos := ""
	if position != "" {
		pos = "position: " + position + "\n"
	}
	return writeFile(t, fmt.Sprintf(`schemaId: %s
scope: environment
externalId: %s%s
%svalue:
  enabled: false
  name: %s%s
  operatingSystem: [LINUX]
  minimumProcesses: 1
  metadata: []
  rules:
    - ruleType: RuleTypeProcess
      property: executable
      condition: '$eq(dtmgd-it-no-such-process)'
`, availabilitySchema, orderPrefix, letter, pos, orderPrefix, letter))
}

// orderOf is the listing order of the order-test rules, as letters. Other
// rules may exist, so only the relative order of these counts.
func orderOf(t *testing.T, ctx string) []string {
	t.Helper()
	var letters []string
	for _, m := range testObjects(t, ctx, availabilitySchema, "environment") {
		id, _ := m["externalId"].(string)
		if l, ok := strings.CutPrefix(id, orderPrefix); ok {
			letters = append(letters, l)
		}
	}
	return letters
}

func expectOrder(t *testing.T, ctx, step string, want ...string) {
	t.Helper()
	if got := orderOf(t, ctx); !slices.Equal(got, want) {
		t.Errorf("%s: order %v, want %v", step, got, want)
	}
}

func TestOrderedRulesPosition(t *testing.T) {
	cfg, ctx := writeConfig(t)
	cleanupTestObjects(t, cfg, ctx, availabilitySchema)

	expectAction(t, "create a", applied(t, cfg, ctx, "apply", "-f", orderFile(t, "a", "")), "created")
	expectAction(t, "create b", applied(t, cfg, ctx, "apply", "-f", orderFile(t, "b", "")), "created")
	expectOrder(t, ctx, "a then b", "a", "b")

	expectAction(t, "create c first", applied(t, cfg, ctx, "apply", "-f", orderFile(t, "c", "first")), "created")
	expectOrder(t, ctx, "c first", "c", "a", "b")

	expectAction(t, "move b before c", applied(t, cfg, ctx, "apply", "-f", orderFile(t, "b", "before:"+orderPrefix+"c")), "updated")
	expectOrder(t, ctx, "b before c", "b", "c", "a")

	expectAction(t, "move a after b", applied(t, cfg, ctx, "apply", "-f", orderFile(t, "a", "after:"+orderPrefix+"b")), "updated")
	expectOrder(t, ctx, "a after b", "b", "a", "c")

	for _, m := range testObjects(t, ctx, availabilitySchema, "environment") {
		id, _ := m["objectId"].(string)
		expectAction(t, "delete", applied(t, cfg, ctx, "delete", "settings", id, "--yes"), "deleted")
	}
	if left := orderOf(t, ctx); len(left) != 0 {
		t.Errorf("%d rules are still listed after the deletes", len(left))
	}
}

// ---- reads ----

// exportSample reads every environment setting and keeps up to five items
// per schema, in the shape of the listing.
func exportSample(t *testing.T, ctx string) map[string]any {
	t.Helper()
	body := decode[map[string]any](t, "get", "settings", "--scope", "environment", "--context", ctx, "-o", "json")
	perSchema := map[string]int{}
	var kept []any
	for _, item := range items(t, body, "items") {
		m, _ := item.(map[string]any)
		schema, _ := m["schemaId"].(string)
		if perSchema[schema] < 5 {
			perSchema[schema]++
			kept = append(kept, item)
		}
	}
	body["items"] = kept
	return body
}

// actionCounts tallies the results of a dry run by action.
func actionCounts(t *testing.T, out []byte) map[string]int {
	t.Helper()
	var rep struct {
		Results []struct {
			Action string `json:"action"`
		} `json:"results"`
	}
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatalf("the dry run's stdout is not a report: %v", err)
	}
	counts := map[string]int{}
	for _, r := range rep.Results {
		counts[r.Action]++
	}
	return counts
}

// TestExportsRoundTrip reads settings, applies a sample of them back with
// --dry-run (GETs only), and expects every action to be unchanged. It
// reports counts only.
func TestExportsRoundTrip(t *testing.T) {
	ctx := env(t)
	cfg := requireEnv(t, "DTMGD_IT_CONFIG")
	sample := exportSample(t, ctx)
	data, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(items(t, sample, "items")) == 0 {
		t.Skip("the environment has no settings to round-trip")
	}
	file := filepath.Join(t.TempDir(), "export.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	r := writeRun(t, cfg, ctx, "apply", "-f", file, "--dry-run", "-o", "json")
	skipWithoutScope(t, r, []string{"apply", "settings"})
	if len(r.Out) == 0 {
		t.Fatalf("the dry run printed no report: %s", brief(r))
	}
	counts := actionCounts(t, r.Out)
	total := 0
	for _, n := range counts {
		total += n
	}
	if r.Code != 0 || counts["unchanged"] != total {
		t.Errorf("round trip of %d objects: exit %d, actions %v, want all unchanged", total, r.Code, counts)
	}
}

// TestClusterScopeListing lists settings at the cluster scope, with and
// without effective values, without naming a schema.
func TestClusterScopeListing(t *testing.T) {
	ctx := cluster(t)
	for _, args := range [][]string{
		{"get", "settings", "--scope", "cluster", "--context", ctx, "-o", "json"},
		{"get", "settings", "--effective", "--scope", "cluster", "--context", ctx, "-o", "json"},
	} {
		r := run(t, args...)
		skipWithoutScope(t, r, args)
		if r.Code != 0 {
			t.Errorf("%v: %s", args[:3], brief(r))
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(r.Out, &body); err != nil {
			t.Errorf("%v: stdout is not JSON: %v", args[:3], err)
		} else if _, ok := body["items"].([]any); !ok {
			t.Errorf("%v: the response has no items list", args[:3])
		}
	}
}

// ---- cluster network zone ----

// zoneFile is an apply file for the test network zone.
func zoneFile(t *testing.T) string {
	return writeFile(t, fmt.Sprintf(`schemaId: %s
scope: environment-default
externalId: %szone
value:
  id: %szone
  description: dtmgd integration test
  alternativeZones: []
  fallbackMode: NONE
`, networkZoneSchema, testWindowPrefix, testWindowPrefix))
}

func TestClusterNetworkZoneLifecycle(t *testing.T) {
	cfg, ctx := writeConfigFor(t, "DTMGD_IT_WRITE_CLUSTER")
	cleanupTestObjects(t, cfg, ctx, networkZoneSchema)

	created := applied(t, cfg, ctx, "apply", "-f", zoneFile(t))
	expectAction(t, "create zone", created, "created")
	expectAction(t, "re-apply zone", applied(t, cfg, ctx, "apply", "-f", zoneFile(t)), "unchanged")
	expectAction(t, "delete zone", applied(t, cfg, ctx, "delete", "settings", created.Results[0].ObjectID, "--yes"), "deleted")
	if left := testObjects(t, ctx, networkZoneSchema, "environment-default"); len(left) != 0 {
		t.Errorf("%d test zones are still listed after the delete", len(left))
	}
}
