package cmd

import (
	"strings"
	"testing"
)

const (
	licenseRoute = "GET /api/cluster/v2/clusterLicense"
	licenseUsageRoute   = "GET /api/cluster/v2/clusterLicense/environment/total"
	testLicenseKey     = "TEST-KEY0-0000-9876"
)

// licenseRoutes serves the license, its per-environment usage, and the
// environment list the usage table takes names from.
func licenseRoutes() map[string]route {
	routes := pagedEnvironments()
	routes[licenseRoute] = route{File: "cluster_license.json"}
	routes[licenseUsageRoute] = route{File: "cluster_license_usage.json"}
	return routes
}

// TestDescribeLicenseMasksKey pins that no output format shows the license
// key, only its last four characters.
func TestDescribeLicenseMasksKey(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", licenseRoutes())
	runs := map[string]cliResult{
		"human": runCLI(t, cfg, "describe", "license"),
		"json":  runCLI(t, cfg, "describe", "license", "-o", "json"),
		"yaml":  runCLI(t, cfg, "describe", "license", "-o", "yaml"),
		"agent": runCLIWithEnv(t, map[string]string{"CLAUDECODE": "1"}, cfg, "describe", "license"),
	}
	for name, r := range runs {
		if strings.Contains(r.Stdout+r.Stderr, testLicenseKey) || !strings.Contains(r.Stdout, "****9876") {
			t.Errorf("%s: key not masked:\n%s", name, r.Stdout)
		}
	}
}

// TestDescribeLicenseHuman pins describe license's summary.
func TestDescribeLicenseHuman(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", licenseRoutes())
	assertGolden(t, "describe_license_human", runCLI(t, cfg, "describe", "license").String())
}

// TestDescribeLicenseOnDPS pins the answer on a cluster without classic
// licensing: the API's exit code, and a hint that says why.
func TestDescribeLicenseOnDPS(t *testing.T) {
	for _, tc := range []struct {
		rt   route
		code int
	}{
		{route{Status: 400, File: "cluster_error_400_classic.json"}, 1},
		{route{Status: 404, File: "error_404.json"}, 4},
	} {
		cfg, _, _ := clusterFake(t, "c", map[string]route{licenseRoute: tc.rt, licenseUsageRoute: tc.rt})
		for _, args := range [][]string{{"describe", "license"}, {"get", "license-usage"}} {
			r := runCLI(t, cfg, args...)
			if r.Code != tc.code || !strings.Contains(r.Stderr, "works only with Dynatrace classic licensing") {
				t.Errorf("%v on %d: code %d, stderr:\n%s", args, tc.rt.Status, r.Code, r.Stderr)
			}
		}
	}
}

// TestGetLicenseUsageTableWithNames pins the usage table, each environment
// named from the cluster's environment list.
func TestGetLicenseUsageTableWithNames(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", licenseRoutes())
	assertGolden(t, "get_license_usage_table", runCLI(t, cfg, "get", "license-usage").String())
}

// TestGetLicenseUsageNameLookupFails pins that the names only decorate the
// table: without them it still prints, with a warning.
func TestGetLicenseUsageNameLookupFails(t *testing.T) {
	cfg, _, _ := clusterFake(t, "c", withRoute(licenseRoutes(), clusterEnvsPath, route{Status: 403, File: "error_403.json"}))
	r := runCLI(t, cfg, "get", "license-usage")
	if r.Code != 0 || strings.Contains(r.Stdout, "shop-prod") || !strings.Contains(r.Stdout, "env-1") {
		t.Errorf("code %d, stdout:\n%s", r.Code, r.Stdout)
	}
	wantStderr(t, r, "environment names unavailable: API error 403")
}

// TestGetLicenseUsageJSONSkipsLookup pins that json output, which has no
// NAME column, costs no environment-list call.
func TestGetLicenseUsageJSONSkipsLookup(t *testing.T) {
	cfg, _, cluster := clusterFake(t, "c", licenseRoutes())
	r := runCLI(t, cfg, "get", "license-usage", "-o", "json")
	if r.Code != 0 || cluster.Hits(clusterEnvsPath) != 0 {
		t.Errorf("code %d, environment-list hits %d", r.Code, cluster.Hits(clusterEnvsPath))
	}
}

// TestMaskLicenseKey pins the mask.
func TestMaskLicenseKey(t *testing.T) {
	for in, want := range map[string]string{"": "", "abc": "****", "abcd": "****", "abcde": "****bcde"} {
		if got := maskLicenseKey(in); got != want {
			t.Errorf("maskLicenseKey(%q) = %q, want %q", in, got, want)
		}
	}
}
