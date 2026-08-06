package config

import (
	"os"
	"path/filepath"
)

// SourceKind describes how the active config file was chosen.
type SourceKind int

const (
	// SourceGlobal is the XDG default path, used when nothing else applies.
	SourceGlobal SourceKind = iota
	// SourceExplicit is a path the user named with --config.
	SourceExplicit
	// SourceLocalCWD is a .dtmgd.yaml in the working directory. This is the
	// documented project-local workflow: `dtmgd config init` writes it here.
	SourceLocalCWD
	// SourceLocalAncestor is a .dtmgd.yaml found by walking up from the
	// working directory. Nothing in the user's immediate surroundings shows
	// that this file exists, which is what makes it worth reporting.
	SourceLocalAncestor
)

// Source identifies the config file in use and how it was selected.
type Source struct {
	Path string
	Kind SourceKind
}

// Invisible reports whether the file was chosen implicitly from a directory
// the user is not in. Such a file can be planted by anyone who can write to a
// shared parent directory (/tmp, a group project tree, a CI workspace), and
// takes effect on the next dtmgd run with nothing on screen to show for it.
// Callers use this to decide whether the selection deserves a warning.
func (s Source) Invisible() bool {
	return s.Kind == SourceLocalAncestor
}

// ResolveSource reports which file Load would read, and how it got there.
// explicit is the value of --config, or "" when the flag was not given.
//
// The precedence matches Load: an explicit path wins, then a discovered
// .dtmgd.yaml, then the global default.
func ResolveSource(explicit string) Source {
	if explicit != "" {
		return Source{Path: explicit, Kind: SourceExplicit}
	}
	if local := FindLocalConfig(); local != "" {
		return Source{Path: local, Kind: classifyLocal(local)}
	}
	return Source{Path: DefaultConfigPath(), Kind: SourceGlobal}
}

// classifyLocal decides whether a discovered local config sits in the working
// directory or somewhere above it.
//
// On any error determining the working directory it returns
// SourceLocalAncestor: that is the reporting branch, so an unknown location is
// surfaced rather than passed over.
func classifyLocal(path string) SourceKind {
	cwd, err := os.Getwd()
	if err != nil {
		return SourceLocalAncestor
	}
	if sameFile(filepath.Join(cwd, LocalConfigName), path) {
		return SourceLocalCWD
	}
	return SourceLocalAncestor
}
