#!/usr/bin/env bash
# drift-check.sh — report Go toolchain, dependency and vulnerability drift.
#
# Writes a Markdown report to stdout. Exit status:
#   0  no drift
#   1  drift found (a newer toolchain patch, a linked module behind, or a
#      known vulnerability in code dtmgd calls)
#   2  a check could not run (network, proxy, tool failure)
#
# Run by .github/workflows/drift.yml, which turns the report into an issue.
# Bot-opened PRs would not trigger CI here, so the fix itself is a human PR.
#
# GOVULNCHECK overrides the govulncheck command (default: govulncheck).
set -euo pipefail
trap 'echo "drift-check: unexpected failure on line $LINENO" >&2; exit 2' ERR

status=0
raise() {
	local level=$1
	if ((level > status)); then status=$level; fi
}

echo "# Toolchain and dependency drift"
echo
echo "Checked $(date -u +%F) at \`$(git rev-parse --short HEAD 2>/dev/null || echo unknown)\`."

# --- Go toolchain: is the pinned toolchain the latest patch of its line? ---
echo
echo "## Go toolchain"
echo
current=$(go mod edit -json | jq -r '.Toolchain // ("go" + .Go)')
line=$(sed -E 's/^(go[0-9]+\.[0-9]+).*/\1/' <<<"$current")
if releases=$(curl -fsS --max-time 30 'https://go.dev/dl/?mode=json&include=all' 2>/dev/null); then
	# sort -V rather than trusting the API's order.
	latest=$(jq -r --arg p "$line." '.[] | select(.stable) | .version | select(startswith($p))' <<<"$releases" | sort -V | tail -1)
	if [[ -z $latest ]]; then
		echo "Could not check: no stable $line release found on go.dev."
		raise 2
	elif [[ $latest != "$current" ]]; then
		echo "Pinned \`$current\`, latest is \`$latest\`. Fix: \`go mod edit -toolchain=$latest\`."
		raise 1
	else
		echo "Pinned \`$current\` is the latest $line release."
	fi
else
	echo "Could not check: go.dev was unreachable."
	raise 2
fi

# --- Modules built into dtmgd that have a newer version ---
# Modules that are only in the module graph (no go.sum entry, never linked)
# are deliberately left out: see the Snyk/Dependabot notes in .claude/CLAUDE.md.
echo
echo "## Linked modules with newer versions"
echo
if linked=$(go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}{{end}}{{end}}' ./... 2>&1) &&
	outdated=$(go list -m -u -f '{{if and .Update (not .Main)}}{{.Path}} {{.Version}} {{.Update.Version}}{{end}}' all 2>&1); then
	rows=$(awk 'NR == FNR { if ($1 != "") seen[$1] = 1; next }
		($1 in seen) { printf "| `%s` | %s | %s |\n", $1, $2, $3 }' \
		<(sort -u <<<"$linked") <(printf '%s\n' "$outdated"))
	if [[ -n $rows ]]; then
		echo "| Module | Current | Latest |"
		echo "|---|---|---|"
		echo "$rows"
		echo
		echo "Fix: \`go get -u ./... && go mod tidy\`."
		raise 1
	else
		echo "None. Every module built into dtmgd is at its latest version."
	fi
else
	echo "Could not check: \`go list\` failed."
	raise 2
fi

# --- Known vulnerabilities in code dtmgd calls ---
echo
echo "## Known vulnerabilities (govulncheck)"
echo
read -r -a vulncmd <<<"${GOVULNCHECK:-govulncheck}"
if vout=$("${vulncmd[@]}" ./... 2>&1); then
	vstatus=0
else
	vstatus=$?
fi
case $vstatus in
0)
	echo "None found."
	;;
3)
	echo "govulncheck found vulnerabilities in code dtmgd calls:"
	echo
	echo '```'
	echo "$vout"
	echo '```'
	raise 1
	;;
*)
	echo "Could not check: govulncheck exited $vstatus."
	echo
	echo '```'
	tail -20 <<<"$vout"
	echo '```'
	raise 2
	;;
esac

exit "$status"
