package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

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
	negIdx := -1
	for i, a := range args {
		if len(a) >= 2 && a[0] == '-' && a[1] >= '0' && a[1] <= '9' {
			negIdx = i
			break
		}
	}
	if negIdx == -1 {
		return args
	}
	// "--" already present before the negative ID — nothing to do.
	for i := 0; i < negIdx; i++ {
		if args[i] == "--" {
			return args
		}
	}
	before := args[:negIdx]
	negID := args[negIdx]
	after := args[negIdx+1:]

	// Move flag-like tokens (starting with -<letter> or --<word>) that come
	// after the negative ID to before the "--", so they are still parsed by
	// cobra.  Stop at a bare "--" or any non-flag token.
	var movedFlags []string
	var positionalAfter []string
	for i := 0; i < len(after); {
		a := after[i]
		if a == "--" {
			// Hard stop: pass remaining args (including this "--") through as-is.
			positionalAfter = append(positionalAfter, after[i:]...)
			break
		}
		isFlagLike := len(a) >= 2 && a[0] == '-' &&
			(a[1] == '-' || (a[1] >= 'a' && a[1] <= 'z') || (a[1] >= 'A' && a[1] <= 'Z'))
		if isFlagLike {
			movedFlags = append(movedFlags, a)
			// If the next token does not look like a flag, treat it as the flag value.
			if i+1 < len(after) && (len(after[i+1]) == 0 || after[i+1][0] != '-') {
				movedFlags = append(movedFlags, after[i+1])
				i += 2
				continue
			}
			i++
		} else {
			positionalAfter = append(positionalAfter, after[i:]...)
			break
		}
	}

	out := make([]string, 0, len(args)+1)
	out = append(out, before...)
	out = append(out, movedFlags...)
	out = append(out, "--")
	out = append(out, negID)
	out = append(out, positionalAfter...)
	return out
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

func requireSubcommand(cmd *cobra.Command, args []string) error {
	if err := cmd.Help(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr)
	return fmt.Errorf("a subcommand is required")
}
