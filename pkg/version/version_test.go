package version

import (
	"runtime/debug"
	"testing"
)

// withDefaults sets the three variables to what an unstamped build has, and
// restores them after the test.
func withDefaults(t *testing.T) {
	t.Helper()
	v, c, d := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = v, c, d })
	Version, Commit, Date = "dev", "none", "unknown"
}

func assertVersion(t *testing.T, version, commit, date string) {
	t.Helper()
	if Version != version || Commit != commit || Date != date {
		t.Errorf("got (%q, %q, %q), want (%q, %q, %q)", Version, Commit, Date, version, commit, date)
	}
}

func TestFromBuildInfoUsesTheModuleVersionOfGoInstall(t *testing.T) {
	withDefaults(t)
	// go install github.com/dynatrace-oss/dtmgd@v0.3.1: a module version, no VCS stamp.
	fromBuildInfo(&debug.BuildInfo{Main: debug.Module{Path: "github.com/dynatrace-oss/dtmgd", Version: "v0.3.1"}})
	assertVersion(t, "0.3.1", "none", "unknown")
}

func TestFromBuildInfoUsesTheVCSStampOfALocalBuild(t *testing.T) {
	withDefaults(t)
	fromBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "v0.3.1-0.20261029225836-ad0dc9391341+dirty"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "ad0dc93913418699172fdc2631342fc45b60e62d"},
			{Key: "vcs.time", Value: "2026-10-29T22:58:36Z"},
		},
	})
	assertVersion(t, "0.3.1-0.20261029225836-ad0dc9391341+dirty", "ad0dc93", "2026-10-29T22:58:36Z")
}

func TestFromBuildInfoKeepsDevWithoutAModuleVersion(t *testing.T) {
	// go test and go run report "(devel)"; some builds report "".
	for _, v := range []string{"(devel)", ""} {
		t.Run(v, func(t *testing.T) {
			withDefaults(t)
			fromBuildInfo(&debug.BuildInfo{Main: debug.Module{Version: v}})
			assertVersion(t, "dev", "none", "unknown")
		})
	}
}

func TestFromBuildInfoNeverOverridesLdflags(t *testing.T) {
	withDefaults(t)
	// GoReleaser sets all three through -ldflags; build info must not win.
	Version, Commit, Date = "0.3.1", "abc1234", "2026-10-30T00:00:00Z"
	fromBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "v9.9.9"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "ffffffffffffffffffffffffffffffffffffffff"},
			{Key: "vcs.time", Value: "2000-01-01T00:00:00Z"},
		},
	})
	assertVersion(t, "0.3.1", "abc1234", "2026-10-30T00:00:00Z")
}
