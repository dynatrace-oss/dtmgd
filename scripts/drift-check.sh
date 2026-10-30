#!/usr/bin/env bash
# drift-check.sh — report Go toolchain, dependency and vulnerability drift.
#
# Writes a Markdown report to stdout. Exit status:
#   0  no drift
#   1  drift found (a newer toolchain patch, a linked module behind, a CI
#      tool pinned behind its latest release, or a known vulnerability in
#      code dtmgd calls)
#   2  a check could not run (network, proxy, tool failure)
#
# Run by .github/workflows/drift.yml, which turns the report into an issue.
# Bot-opened PRs would not trigger CI here, so the fix itself is a human PR.
#
# GOVULNCHECK overrides the govulncheck command (default: govulncheck). CI
# passes the binary built from the checksum-locked tools/ module.
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

# --- CI tools pinned by version (Dependabot cannot see a `version:` input) ---
echo
echo "## Pinned CI tools"
echo
latest_release() { # $1 = owner/repo; prints the latest release tag
	local auth=()
	if [[ -n ${GITHUB_TOKEN:-} ]]; then auth=(-H "Authorization: Bearer $GITHUB_TOKEN"); fi
	curl -fsS --max-time 30 "${auth[@]}" "https://api.github.com/repos/$1/releases/latest" | jq -r '.tag_name // empty'
}
# check_pins TOOL OWNER/REPO PIN... where each PIN is "file: version".
check_pins() {
	local tool=$1 repo=$2 latest versions
	shift 2
	if (($# == 0)); then
		echo "Could not check $tool: no pinned version found."
		raise 2
		return
	fi
	versions=$(printf '%s\n' "$@" | awk '{print $NF}' | sort -u)
	if ! latest=$(latest_release "$repo") || [[ -z $latest ]]; then
		echo "Could not check $tool: the GitHub API was unreachable."
		raise 2
		return
	fi
	if [[ $(wc -l <<<"$versions") -gt 1 ]]; then
		echo "$tool pins disagree; latest is \`$latest\`:"
		printf -- "- \`%s\`\n" "$@"
		raise 1
	elif [[ $versions != "$latest" ]]; then
		echo "$tool is pinned at \`$versions\`, latest is \`$latest\`. Fix: update every pin:"
		printf -- "- \`%s\`\n" "$@"
		raise 1
	else
		echo "$tool \`$latest\` is the latest release."
	fi
}
mapfile -t golangci_pins < <(
	grep -HoE 'version: v[0-9.]+ # keep in step with GOLANGCI_LINT_VERSION' .github/workflows/sast.yml | sed -E 's/ # .*//; s/:version: /: /' || true
	grep -HoE '^GOLANGCI_LINT_VERSION := v[0-9.]+' Makefile | sed -E 's/:GOLANGCI_LINT_VERSION := /: /' || true
)
mapfile -t goreleaser_pins < <(
	grep -HoE 'version: v[0-9.]+ # keep in step with [a-z. ]+\.yml' .github/workflows/{release,lint,build}.yml | sed -E 's/ # .*//; s/:version: /: /' || true
)
check_pins golangci-lint golangci/golangci-lint "${golangci_pins[@]}"
echo
check_pins GoReleaser goreleaser/goreleaser "${goreleaser_pins[@]}"

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
