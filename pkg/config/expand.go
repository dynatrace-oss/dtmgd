package config

import (
	"os"
	"regexp"
)

// placeholderRe matches a ${VAR} reference. This is deliberately not
// os.Expand: os.Expand also recognises bare $VAR, so it cannot express
// "${VAR} and nothing else". Restricting to the braced form is what keeps
// literal dollar signs — proxy passwords, prices — intact.
var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// bareRefRe matches a bare $VAR reference, which is no longer expanded.
var bareRefRe = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// HasPlaceholder reports whether s contains a ${VAR} reference.
func HasPlaceholder(s string) bool {
	return placeholderRe.MatchString(s)
}

// PlaceholderRefs returns the ${VAR} references found in s, in the order
// they appear, as the literal "${VAR}" text rather than the bare name.
//
// This exists so callers that need to warn about a clobbered value — e.g.
// "Replaced ${DT_API_TOKEN} in the config" — can echo just the matched
// reference(s) instead of the whole raw string. A mixed value such as
// "dt0c01.REAL${SUFFIX}" is half live secret, half placeholder; printing the
// raw value to the terminal would leak the secret half.
func PlaceholderRefs(s string) []string {
	return placeholderRe.FindAllString(s, -1)
}

// BareRefs returns bare $VAR references in s that name a currently-set
// environment variable, deduped and in first-appearance order.
//
// The set-variable requirement is what keeps this quiet: a plain regexp scan
// matches "$w0rd" inside "pa$$w0rd", and reporting that would warn about the
// very strings this package exists to preserve. A bare name that resolves to
// a real variable is almost certainly a reference the user expected to work.
func BareRefs(s string) []string {
	// Remove braced forms first so ${VAR} is never reported as bare.
	stripped := placeholderRe.ReplaceAllString(s, "")

	var refs []string
	seen := make(map[string]bool)
	for _, m := range bareRefRe.FindAllStringSubmatch(stripped, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		if _, ok := os.LookupEnv(name); !ok {
			continue
		}
		seen[name] = true
		refs = append(refs, name)
	}
	return refs
}

// expand replaces every ${VAR} in s with its environment value.
//
// unset lists variables that were referenced but not set, deduped and in
// first-appearance order. A set-but-empty variable is not unset — the user
// chose that value. Unset references expand to the empty string; every caller
// treats a non-empty unset slice as an error, so the partial result is never
// used. Returning it keeps this function total and free of error handling.
func expand(s string) (string, []string) {
	var unset []string
	seen := make(map[string]bool)

	expanded := placeholderRe.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-1]
		if value, ok := os.LookupEnv(name); ok {
			return value
		}
		if !seen[name] {
			seen[name] = true
			unset = append(unset, name)
		}
		return ""
	})

	return expanded, unset
}
