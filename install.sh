#!/bin/sh
# Install dtmgd (Dynatrace Managed CLI) on Linux and macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/dynatrace-oss/dtmgd/main/install.sh | sh
#
# The archive is checked against the release's checksums.txt before anything
# is installed, and a failed, corrupted or incomplete download leaves an
# installed dtmgd as it was. checksums.txt comes from the same place as the
# archive, so this does not detect a compromised release or mirror: for that,
# verify the cosign signature (README, "Verifying a download").
#
# Environment variables:
#   DTMGD_VERSION        Version to install, e.g. 0.3.1 or v0.3.1 (default: the latest release;
#                        required with DTMGD_DOWNLOAD_BASE)
#   DTMGD_INSTALL_DIR    Install directory (default: ~/.local/bin on Linux, /usr/local/bin on macOS)
#   DTMGD_DOWNLOAD_BASE  Mirror of the release downloads, laid out as <base>/v<version>/<asset>
#                        (default: https://github.com/dynatrace-oss/dtmgd/releases/download)
#   GITHUB_TOKEN         Sent only to api.github.com, to avoid rate limits when finding the latest release

set -eu

REPO="dynatrace-oss/dtmgd"

die() {
	echo "Error: $*" >&2
	exit 1
}

# Everything runs from main, so a download of this script cut short by the
# network runs nothing.
main() {
	command -v curl >/dev/null 2>&1 || die "curl is required"

	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) die "unsupported OS: $(uname -s) (on Windows, use install.ps1)" ;;
	esac

	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "unsupported architecture: $(uname -m)" ;;
	esac

	if command -v sha256sum >/dev/null 2>&1; then
		sha256() { sha256sum "$1" | awk '{print $1}'; }
	elif command -v shasum >/dev/null 2>&1; then
		sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
	else
		die "sha256sum or shasum is needed to verify the download"
	fi

	if [ -n "${DTMGD_INSTALL_DIR:-}" ]; then
		install_dir=$DTMGD_INSTALL_DIR
	elif [ "$os" = darwin ]; then
		install_dir=/usr/local/bin
	else
		install_dir="$HOME/.local/bin"
	fi
	target="$install_dir/dtmgd"
	if [ -L "$target" ]; then
		die "$target is a symlink to $(readlink "$target"), probably a Homebrew install. Upgrade it with 'brew upgrade dtmgd', or set DTMGD_INSTALL_DIR to install somewhere else."
	fi

	if [ -n "${DTMGD_VERSION:-}" ]; then
		version=${DTMGD_VERSION#v}
	elif [ -n "${DTMGD_DOWNLOAD_BASE:-}" ]; then
		die "set DTMGD_VERSION with DTMGD_DOWNLOAD_BASE: the latest release is looked up on api.github.com, not on the mirror"
	else
		echo "Finding the latest release..."
		api="https://api.github.com/repos/$REPO/releases/latest"
		if [ -n "${GITHUB_TOKEN:-}" ]; then
			# The header goes to curl as a config on stdin: on its command line,
			# any user on the host could read the token in the process list.
			release=$(printf 'header = "Authorization: Bearer %s"\n' "$GITHUB_TOKEN" | curl -fsSL -K - "$api") || die "could not query $api"
		else
			release=$(curl -fsSL "$api") || die "could not query $api"
		fi
		tag=$(printf '%s\n' "$release" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
		[ -n "$tag" ] || die "could not find the latest release in $api"
		version=${tag#v}
	fi

	base=${DTMGD_DOWNLOAD_BASE:-https://github.com/$REPO/releases/download}
	base=${base%/}
	asset="dtmgd_${version}_${os}_${arch}.tar.gz"

	tmpdir=$(mktemp -d)
	staged=
	sudo=
	trap 'rm -rf "$tmpdir"; [ -z "$staged" ] || $sudo rm -f "$staged"' EXIT

	echo "Downloading dtmgd $version ($os/$arch)..."
	for f in "$asset" checksums.txt; do
		curl -fsSL -o "$tmpdir/$f" "$base/v$version/$f" || die "could not download $base/v$version/$f"
	done

	expected=$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1; exit }' "$tmpdir/checksums.txt")
	[ -n "$expected" ] || die "checksums.txt has no checksum for $asset"
	actual=$(sha256 "$tmpdir/$asset")
	[ "$expected" = "$actual" ] || die "checksum mismatch for $asset: expected $expected, got $actual"

	mkdir -p "$tmpdir/x"
	tar -xzf "$tmpdir/$asset" -C "$tmpdir/x" dtmgd || die "could not extract dtmgd from $asset"
	[ -f "$tmpdir/x/dtmgd" ] || die "$asset has no dtmgd binary"
	chmod 755 "$tmpdir/x/dtmgd"

	# Copy next to the target, then rename: the rename is atomic, so an
	# installed dtmgd is never left half-written.
	mkdir -p "$install_dir" 2>/dev/null || true
	if [ ! -w "$install_dir" ]; then
		echo "Installing to $install_dir (requires sudo)..."
		sudo=sudo
		$sudo mkdir -p "$install_dir"
	fi
	staged="$install_dir/.dtmgd.$$"
	$sudo cp "$tmpdir/x/dtmgd" "$staged" || die "could not write to $install_dir"
	$sudo mv -f "$staged" "$target" || die "could not install $target"
	staged=

	if [ "$os" = darwin ] && command -v xattr >/dev/null 2>&1; then
		$sudo xattr -d com.apple.quarantine "$target" 2>/dev/null || true
	fi

	echo
	"$target" version
	echo "Installed to $target"

	case ":$PATH:" in
	*":$install_dir:"*)
		first=$(command -v dtmgd || true)
		if [ -n "$first" ] && [ "$first" != "$target" ]; then
			echo
			echo "WARNING: $first comes first on your PATH, so 'dtmgd' still runs that one."
		fi
		;;
	*)
		echo
		echo "NOTE: $install_dir is not in your PATH. Add it, for example in ~/.bashrc or ~/.zshrc:"
		echo
		echo "  export PATH=\"\$PATH:$install_dir\""
		;;
	esac

	echo
	echo "Next: create a context, store its token, and check it (see https://github.com/$REPO#quick-start):"
	echo "  dtmgd config set-context <name> --host https://cluster.example.com --env-id <environment-id> --token-ref <name>-token"
	echo "  dtmgd config set-credentials <name>-token"
	echo "  dtmgd doctor"
}

main "$@"
