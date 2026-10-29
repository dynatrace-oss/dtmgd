package client

import (
	"slices"
	"strconv"
	"strings"
)

// ParseClusterVersion returns the first three numeric parts of a cluster
// version such as "1.330.18.20251017-123456".
func ParseClusterVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.SplitN(v, ".", 4)
	if len(parts) < 3 {
		return out, false
	}
	for i := range out {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// LowestClusterVersion returns the lowest of versions that parses (see
// ParseClusterVersion), or "" when none does. A cluster in the middle of an
// upgrade runs several versions at once, and its lowest node's is the one a
// version floor has to judge.
func LowestClusterVersion(versions []string) string {
	lowest, lowestParts := "", [3]int{}
	for _, v := range versions {
		parts, ok := ParseClusterVersion(v)
		if !ok {
			continue
		}
		if lowest == "" || slices.Compare(parts[:], lowestParts[:]) < 0 {
			lowest, lowestParts = v, parts
		}
	}
	return lowest
}
