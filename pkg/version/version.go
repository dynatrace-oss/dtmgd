package version

import (
	"runtime/debug"
	"strings"
)

// Overridden at build time via ldflags. A build without them, such as
// go install, falls back to the module's build info (see fromBuildInfo).
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func init() {
	if bi, ok := debug.ReadBuildInfo(); ok {
		fromBuildInfo(bi)
	}
}

// fromBuildInfo fills each of Version, Commit and Date that still holds its
// default from bi: the module version that go install records, and the VCS
// revision and time that a build inside a git checkout records. Values set
// through ldflags always win.
func fromBuildInfo(bi *debug.BuildInfo) {
	if v := bi.Main.Version; Version == "dev" && v != "" && v != "(devel)" {
		Version = strings.TrimPrefix(v, "v")
	}
	for _, s := range bi.Settings {
		switch {
		case s.Key == "vcs.revision" && Commit == "none" && s.Value != "":
			Commit = s.Value[:min(len(s.Value), 7)]
		case s.Key == "vcs.time" && Date == "unknown" && s.Value != "":
			Date = s.Value
		}
	}
}
