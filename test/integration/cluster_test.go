//go:build integration

package integration

import (
	"strings"
	"testing"
)

// cluster is the cluster context under test (no env-id, a Cluster API
// token); the test skips without one.
func cluster(t *testing.T) string {
	t.Helper()
	return requireEnv(t, "DTMGD_IT_CLUSTER")
}

func TestClusterCommandsAnswer(t *testing.T) {
	ctx := cluster(t)
	for _, args := range [][]string{
		{"get", "nodes"},
		{"describe", "cluster"},
		{"describe", "backup"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			decode[any](t, append(args, "--context", ctx, "-o", "json")...)
		})
	}
}

func TestClusterEnvironments(t *testing.T) {
	body := decode[map[string]any](t, "get", "environments", "--context", cluster(t), "-o", "json")
	if len(items(t, body, "environments")) == 0 {
		t.Error("the cluster lists no environments")
	}
}

// TestLicenseKeyIsMasked pins that describe license never prints the
// license key. The value itself is never printed, not even on failure.
func TestLicenseKeyIsMasked(t *testing.T) {
	r := run(t, "describe", "license", "--context", cluster(t), "-o", "json")
	if r.Code != 0 {
		t.Skipf("describe license: %s (DPS-licensed clusters have no classic license)", brief(r))
	}
	body := decode[map[string]any](t, "describe", "license", "--context", cluster(t), "-o", "json")
	key, ok := body["licenseKey"].(string)
	if !ok {
		t.Skip("the license has no licenseKey field")
	}
	if !strings.HasPrefix(key, "****") || strings.Count(key, "*") < len(key)-4 {
		t.Error("licenseKey is not masked")
	}
}
