#!/bin/sh
# Install dtmgd (Dynatrace Managed CLI) on Linux and macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/dynatrace-oss/dtmgd/main/install.sh | sh
#
# The archive is checked against the release's checksums.txt before anything
# is installed; a failed or tampered download leaves an installed dtmgd as it
# was.
#
# Environment variables:
#   DTMGD_VERSION        Version to install, e.g. 0.3.1 or v0.3.1 (default: the latest release)
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

if [ -n "${DTMGD_VERSION:-}" ]; then
	version=${DTMGD_VERSION#v}
else
	echo "Finding the latest release..."
	api="https://api.github.com/repos/$REPO/releases/latest"
	if [ -n "${GITHUB_TOKEN:-}" ]; then
		release=$(curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" "$api") || die "could not query $api"
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
trap 'rm -rf "$tmpdir"' EXIT

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

# Copy next to the target, then rename: the rename is atomic, so a running or
# installed dtmgd is never left half-written.
sudo=
mkdir -p "$install_dir" 2>/dev/null || true
if [ ! -w "$install_dir" ]; then
	echo "Installing to $install_dir (requires sudo)..."
	sudo=sudo
	$sudo mkdir -p "$install_dir"
fi
staged="$install_dir/.dtmgd.$$"
$sudo cp "$tmpdir/x/dtmgd" "$staged" || die "could not write to $install_dir"
$sudo mv -f "$staged" "$install_dir/dtmgd" || {
	$sudo rm -f "$staged"
	die "could not install $install_dir/dtmgd"
}

if [ "$os" = darwin ] && command -v xattr >/dev/null 2>&1; then
	$sudo xattr -d com.apple.quarantine "$install_dir/dtmgd" 2>/dev/null || true
fi

echo
"$install_dir/dtmgd" version
echo "Installed to $install_dir/dtmgd"

case ":$PATH:" in
*":$install_dir:"*) ;;
*)
	echo
	echo "NOTE: $install_dir is not in your PATH. Add it, for example in ~/.bashrc or ~/.zshrc:"
	echo
	echo "  export PATH=\"\$PATH:$install_dir\""
	;;
esac

echo
echo "Next: create a context and check it (see https://github.com/$REPO#quick-start):"
echo "  dtmgd ctx set <name> --host https://cluster.example.com --env-id <environment-id>"
echo "  dtmgd config set-credentials <name>"
echo "  dtmgd doctor"
