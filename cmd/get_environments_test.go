package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/agentmode"
)

const (
	clusterEnvsPath = "GET /api/cluster/v2/environments"
	clusterEnvPath  = "GET /api/cluster/v2/environments/env-1"
)

// pagedEnvironments serves the three-environment list over two pages.
func pagedEnvironments() map[string]route {
	return map[string]route{
		clusterEnvsPath:                     {File: "cluster_environments_page1.json"},
		clusterEnvsPath + " nextPageKey=k2": {File: "cluster_environments_page2.json"},
	}
}

// clusterFake is a config with environment context "a" and cluster context
// "c" (current unless current says otherwise), the cluster answering from
// routes.
func clusterFake(t *testing.T, current string, routes map[string]route) (cfg string, env, cluster *fakeAPI) {
	t.Helper()
	env = newFakeAPI(t, map[string]route{})
	cluster = newFakeAPI(t, routes)
	return withFakeContexts(t, current, []fakeEnv{{"a", env}}, []fakeEnv{{"c", cluster}}), env, cluster
}

// wantStderr fails t for each of wants that r's stderr lacks.
func wantStderr(t *testing.T, r cliResult, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(r.Stderr, w) {
			t.Errorf("stderr lacks %q:\n%s", w, r.Stderr)
		}
	}
}

// TestGetEnvironmentsListsClusterEnvironments pins get environments as the
// cluster's environment list, followed across pages.
func TestGetEnvironmentsListsClusterEnvironments(t *testing.T) {
	cfg, _, cluster := clusterFake(t, "c", pagedEnvironments())
	r := runCLI(t, cfg, "get", "environments")
	assertGolden(t, "get_environments_table", r.String())
	if r.Code != 0 || cluster.Hits(clusterEnvsPath) != 2 {
		t.Errorf("code %d, hits %d", r.Code, cluster.Hits(clusterEnvsPath))
	}
}

// TestGetEnvironmentsFilterAndSort pins how the filter flags become the
// API's filter: name, state, then the raw --filter, comma-joined.
func TestGetEnvironmentsFilterAndSort(t *testing.T) {
	cfg, _, cluster := clusterFake(t, "c", pagedEnvironments())
	r := runCLI(t, cfg, "get", "environments", "--name", "shop", "--state", "enabled", "--filter", "tag(prod)", "--sort", "-NAME")
	if r.Code != 0 {
		t.Fatalf("code %d, stderr:\n%s", r.Code, r.Stderr)
	}
	qs := cluster.Queries(clusterEnvsPath)
	if len(qs) == 0 {
		t.Fatal("no request")
	}
	q := qs[0]
	if q.Get("filter") != "name(shop),state(ENABLED),tag(prod)" || q.Get("sort") != "-name" || q.Get("pageSize") != "1000" {
		t.Errorf("query = %v", q)
	}
}

// TestGetEnvironmentsRejectsReservedNameChars pins that a --name the filter
// syntax cannot carry is refused rather than sent mangled.
func TestGetEnvironmentsRejectsReservedNameChars(t *testing.T) {
	cfg, _, cluster := clusterFake(t, "c", pagedEnvironments())
	r := runCLI(t, cfg, "get", "environments", "--name", "a)b")
	if r.Code != 2 || cluster.totalHits() != 0 {
		t.Errorf("code %d, hits %d, stderr:\n%s", r.Code, cluster.totalHits(), r.Stderr)
	}
	wantStderr(t, r, "--name cannot contain ',', '(' or ')'; use --filter")
}

// TestGetEnvironmentsLimit pins --limit: one page of that size, and a note
// saying how many there are.
func TestGetEnvironmentsLimit(t *testing.T) {
	cfg, _, cluster := clusterFake(t, "c", pagedEnvironments())
	r := runCLI(t, cfg, "get", "environments", "--limit", "1")
	if q := cluster.Queries(clusterEnvsPath); len(q) != 1 || q[0].Get("pageSize") != "1" {
		t.Errorf("queries = %v", q)
	}
	if strings.Count(r.Stdout, "\n") != 2 {
		t.Errorf("want a header and one row:\n%s", r.Stdout)
	}
	wantStderr(t, r, "Showing 1 of 3 environments")
}

// TestGetEnvironmentsConsumptionColumns pins --consumption: the API is asked
// for it, and the table shows host units and DDUs, unlimited when the API
// sets no limit.
func TestGetEnvironmentsConsumptionColumns(t *testing.T) {
	cfg, _, cluster := clusterFake(t, "c", map[string]route{clusterEnvsPath: {File: "cluster_environments_consumption.json"}})
	r := runCLI(t, cfg, "get", "environments", "--consumption")
	if q := cluster.Queries(clusterEnvsPath); len(q) == 0 || q[0].Get("includeConsumptionInfo") != "true" {
		t.Errorf("queries = %v", q)
	}
	for _, want := range []string{"HOST-UNITS", "DDU-YEAR", "12 / 100", "5000.5 / 100000", "2.5 / unlimited", "—"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, r.Stdout)
		}
	}
}

// TestGetEnvironmentsConsumptionOnDPS pins the DPS answer to --consumption:
// the API's 422 keeps its exit code and gains a hint that says why.
func TestGetEnvironmentsConsumptionOnDPS(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", map[string]route{clusterEnvsPath: {Status: 422, File: "cluster_error_422_dps.json"}})
	r := runCLI(t, cfg, "get", "environments", "--consumption")
	if r.Code != 1 {
		t.Errorf("code = %d, want 1", r.Code)
	}
	wantStderr(t, r, "Dynatrace Platform Subscription", "Run without --consumption")
}

// TestGetEnvironmentsOnEnvironmentContext pins the breaking change's
// signpost: run against an environment context, get environments names the
// cluster contexts and the command that now does what it used to.
func TestGetEnvironmentsOnEnvironmentContext(t *testing.T) {
	cfg, env, cluster := clusterFake(t, "a", pagedEnvironments())
	r := runCLI(t, cfg, "get", "environments")
	if r.Code != 2 || env.totalHits()+cluster.totalHits() != 0 {
		t.Errorf("code %d, hits %d", r.Code, env.totalHits()+cluster.totalHits())
	}
	wantStderr(t, r, `"dtmgd get environments" needs a cluster context`, "Cluster contexts: c.", "dtmgd doctor")
}

// TestClusterCommandRejectsMultiEnv pins that cluster commands have no --env
// fan-out.
func TestClusterCommandRejectsMultiEnv(t *testing.T) {
	cfg, _, cluster := clusterFake(t, "c", pagedEnvironments())
	r := runCLI(t, cfg, "get", "environments", "--env", "ALL_ENVIRONMENTS")
	if r.Code != 2 || cluster.totalHits() != 0 {
		t.Errorf("code %d, hits %d", r.Code, cluster.totalHits())
	}
	wantStderr(t, r, `--env "ALL_ENVIRONMENTS" selects environments`)
}

// TestClusterCommand401Hint pins the hint for an environment token stored
// for a cluster context, which the Cluster API answers with 401 (checked
// live on 2026-10-29).
func TestClusterCommand401Hint(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", map[string]route{clusterEnvsPath: {Status: 401, File: "error_401.json"}})
	r := runCLI(t, cfg, "get", "environments")
	if r.Code != 3 {
		t.Errorf("code = %d, want 3", r.Code)
	}
	wantStderr(t, r, "ServiceProviderAPI", "an environment API token does not work here")
}

// TestClusterCommand403NamesScope pins the scope line of a cluster command's
// permission error.
func TestClusterCommand403NamesScope(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", map[string]route{clusterEnvsPath: {Status: 403, File: "error_403.json"}})
	r := runCLI(t, cfg, "get", "environments")
	if r.Code != 5 {
		t.Errorf("code = %d, want 5", r.Code)
	}
	wantStderr(t, r, "This command needs the token scope(s): ServiceProviderAPI.")
}

// TestClusterCommand404Hint pins the hint for a cluster context pointing at
// a host with no Cluster API, such as a node on port 9999.
func TestClusterCommand404Hint(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", map[string]route{})
	r := runCLI(t, cfg, "get", "environments")
	if r.Code != 4 {
		t.Errorf("code = %d, want 4", r.Code)
	}
	wantStderr(t, r, `No Cluster API answered at context "c"'s host`, "port 9999")
}

// TestDescribeEnvironment pins describe environment: storage info always
// asked for, YAML for a person, the API object as-is for json.
func TestDescribeEnvironment(t *testing.T) {
	cfg, _, cluster := clusterFake(t, "c", map[string]route{clusterEnvPath: {File: "cluster_environment.json"}})
	r := runCLI(t, cfg, "describe", "environment", "env-1")
	if r.Code != 0 || !strings.Contains(r.Stdout, "name: shop-prod") {
		t.Errorf("code %d, stdout:\n%s", r.Code, r.Stdout)
	}
	if q := cluster.Queries(clusterEnvPath); len(q) == 0 || q[0].Get("includeStorageInfo") != "true" || q[0].Has("includeConsumptionInfo") {
		t.Fatalf("queries = %v", q)
	}
	r = runCLI(t, cfg, "describe", "environment", "env-1", "-o", "json", "--consumption")
	var got, want interface{}
	raw, _ := os.ReadFile(filepath.Join("testdata", "api", "cluster_environment.json"))
	_ = json.Unmarshal(raw, &want)
	if err := json.Unmarshal([]byte(r.Stdout), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("json output differs from the API object (%v):\n%s", err, r.Stdout)
	}
	if q := cluster.Queries(clusterEnvPath); len(q) < 2 || q[1].Get("includeConsumptionInfo") != "true" {
		t.Errorf("--consumption queries = %v", q)
	}
}

// TestDescribeEnvironmentNotFound pins the hint for an unknown environment.
func TestDescribeEnvironmentNotFound(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", map[string]route{})
	r := runCLI(t, cfg, "describe", "environment", "nope")
	if r.Code != 4 {
		t.Errorf("code = %d, want 4", r.Code)
	}
	wantStderr(t, r, `No environment "nope" on this cluster`, "dtmgd get environments")
}

// TestGetEnvironmentsAgentEnvelope pins the agent envelope of the list.
func TestGetEnvironmentsAgentEnvelope(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", pagedEnvironments())
	r := runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "get", "environments")
	var resp agentmode.Response
	if err := json.Unmarshal([]byte(r.Stdout), &resp); err != nil {
		t.Fatalf("stdout is not one envelope: %v\n%s", err, r.Stdout)
	}
	if !resp.OK || resp.Context == nil || resp.Context.Resource != "environments" || resp.Context.Total == nil || *resp.Context.Total != 3 {
		t.Errorf("envelope = %+v, context %+v", resp, resp.Context)
	}
}

// TestCatalogMarksClusterCommands pins the catalog's cluster_scoped flag,
// which tells an agent which kind of context a command needs.
func TestCatalogMarksClusterCommands(t *testing.T) {
	r := runCLI(t, "", "commands", "-o", "json")
	var cat struct {
		Commands []map[string]interface{} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &cat); err != nil {
		t.Fatal(err)
	}
	byPath := map[string]map[string]interface{}{}
	for _, c := range cat.Commands {
		byPath[c["path"].(string)] = c
	}
	if byPath["get environments"]["cluster_scoped"] != true {
		t.Errorf("get environments = %v", byPath["get environments"])
	}
	if _, ok := byPath["get problems"]["cluster_scoped"]; ok {
		t.Errorf("get problems = %v", byPath["get problems"])
	}
}

// TestSetContextScopesExcludeClusterScope pins that set-context's scope list
// is the environment token's: the cluster scope is described separately.
func TestSetContextScopesExcludeClusterScope(t *testing.T) {
	root := testRoot()
	c, _, err := root.Find([]string{"config", "set-context"})
	if err != nil {
		t.Fatal(err)
	}
	block := c.Long[strings.Index(c.Long, "Required API token scopes:"):]
	block = block[:strings.Index(block, "\n\n")]
	if strings.Contains(block, scopeServiceProviderAPI) || !strings.Contains(strings.Join(allScopes(root), ","), scopeServiceProviderAPI) {
		t.Errorf("scope block:\n%s\nall scopes: %v", block, allScopes(root))
	}
}

// TestClusterCommand404HintNamesHostEverywhere pins Review Focus 1 for the
// commands with a 404 hint of their own: a wrong host (every cluster call
// 404s) is still named, beside the command's own reading of the 404.
func TestClusterCommand404HintNamesHostEverywhere(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", map[string]route{})
	for _, args := range [][]string{{"describe", "license"}, {"get", "license-usage"}, {"describe", "environment", "env-1"}} {
		r := runCLI(t, cfg, args...)
		if r.Code != 4 || !strings.Contains(r.Stderr, `context "c"'s host`) || !strings.Contains(r.Stderr, "port 9999") {
			t.Errorf("%v: code %d, stderr:\n%s", args, r.Code, r.Stderr)
		}
	}
}

// TestClusterHelpNamesEveryClusterCommand pins that the help describing
// cluster contexts names all of their commands, and --env-id says what
// leaving it out does.
func TestClusterHelpNamesEveryClusterCommand(t *testing.T) {
	root := testRoot()
	setContext, _, _ := root.Find([]string{"config", "set-context"})
	for _, name := range []string{"get environments", "describe environment", "get nodes", "describe cluster", "describe license", "get license-usage", "describe backup"} {
		if !strings.Contains(strings.Join(strings.Fields(setContext.Long), " "), name) {
			t.Errorf("set-context help does not name %q", name)
		}
	}
	for _, path := range [][]string{{"ctx", "set"}, {"config", "set-context"}} {
		c, _, _ := root.Find(path)
		if f := c.Flags().Lookup("env-id"); f == nil || !strings.Contains(f.Usage, "omit for a cluster context") {
			t.Errorf("%v --env-id usage = %+v", path, f)
		}
	}
}
