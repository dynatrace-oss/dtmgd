package cmd

import (
	"slices"

	"github.com/dynatrace-oss/dtmgd/pkg/config"
	"github.com/dynatrace-oss/dtmgd/pkg/output"
)

// isDescribeProblemArgs returns true when args contains a "describe problem"
// (or "describe prob") subcommand prefix, indicating that a positional
// argument may be a negative-integer problem ID.
func isDescribeProblemArgs(args []string) bool {
	for i, a := range args {
		if (a == "describe" || a == "desc") && i+1 < len(args) {
			next := args[i+1]
			if next == "problem" || next == "prob" {
				return true
			}
		}
	}
	return false
}

// rewriteNegativeArgs inserts "--" before the first argument that begins with
// "-" followed by a digit (e.g. a negative-integer problem ID such as
// "-6546711275898328738_1776193140000V2"), preventing cobra/pflag from
// misinterpreting it as a flag.  Any flag-like arguments that appear after
// the negative-integer arg are moved in front of the "--" so that global
// flags such as -c, -o, and -v continue to work in their natural position.
//
// This rewrite is only applied for "describe problem" subcommands, where
// negative-integer IDs can legitimately appear as positional arguments.
func rewriteNegativeArgs(args []string) []string {
	if !isDescribeProblemArgs(args) {
		return args
	}
	negIdx := negativeArgIndex(args)
	if negIdx == -1 {
		return args
	}
	// "--" already present before the negative ID — nothing to do.
	if slices.Contains(args[:negIdx], "--") {
		return args
	}
	before := args[:negIdx]
	negID := args[negIdx]

	// Move flag-like tokens (starting with -<letter> or --<word>) that come
	// after the negative ID to before the "--", so they are still parsed by
	// cobra.  Stop at a bare "--" or any non-flag token.
	movedFlags, positionalAfter := leadingFlags(args[negIdx+1:])

	out := make([]string, 0, len(args)+1)
	out = append(out, before...)
	out = append(out, movedFlags...)
	out = append(out, "--")
	out = append(out, negID)
	out = append(out, positionalAfter...)
	return out
}

// negativeArgIndex is the index of the first argument that is "-" followed
// by a digit, such as a negative-integer problem ID, or -1 if there is none.
func negativeArgIndex(args []string) int {
	for i, a := range args {
		if len(a) >= 2 && a[0] == '-' && a[1] >= '0' && a[1] <= '9' {
			return i
		}
	}
	return -1
}

// leadingFlags splits the arguments after a negative ID into the flag-like
// tokens that lead them, each followed by its value if it has one, and the
// rest, which start at the first bare "--" or non-flag token.
func leadingFlags(after []string) (flags, rest []string) {
	for i := 0; i < len(after); {
		a := after[i]
		if a == "--" || !isFlagLike(a) {
			// Hard stop: pass remaining args (including any "--") through as-is.
			return flags, after[i:]
		}
		flags = append(flags, a)
		i++
		// If the next token does not look like a flag, treat it as the flag value.
		if i < len(after) && (len(after[i]) == 0 || after[i][0] != '-') {
			flags = append(flags, after[i])
			i++
		}
	}
	return flags, nil
}

// isFlagLike reports whether a looks like a flag: "--" followed by anything,
// or "-" followed by a letter.
func isFlagLike(a string) bool {
	return len(a) >= 2 && a[0] == '-' &&
		(a[1] == '-' || (a[1] >= 'a' && a[1] <= 'z') || (a[1] >= 'A' && a[1] <= 'Z'))
}

// warnBareRefs warns about bare $VAR references, which are no longer expanded.
//
// Without this the failure is silent and confusing: the literal string
// "$DT_API_TOKEN" becomes the token and the user sees an unexplained 401.
// BareRefs only reports names that are actually set, so incidental dollar
// signs in passwords do not trigger it.
func warnBareRefs(cfg *config.Config) {
	seen := make(map[string]bool)
	warn := func(s string) {
		for _, name := range config.BareRefs(s) {
			if seen[name] {
				continue
			}
			seen[name] = true
			output.PrintWarning("%q looks like an environment variable reference. Use ${%s}.", "$"+name, name)
		}
	}
	for _, nc := range cfg.Contexts {
		warn(nc.Context.Host)
		warn(nc.Context.EnvID)
		warn(nc.Context.TokenRef)
		warn(nc.Context.HTTPProxyURL)
		warn(nc.Context.HTTPSProxyURL)
	}
	for _, nt := range cfg.Tokens {
		warn(nt.Token)
	}
}
