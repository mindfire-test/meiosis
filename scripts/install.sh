#!/usr/bin/env bash
# Installs the latest meiosis release (mei and meiosisd) for the current
# platform by fetching the release archive and SHA256SUMS from GitHub, then
# installing the binaries into the user's PATH.
#
# Usage:
#   bash scripts/install.sh
#   curl -sSL <url> | bash
#
# Env overrides:
#   MEI_VERSION  - install a specific version (default: latest release)
#   MEI_REPO     - GitHub repo to fetch from (default: mindfire-test/meiosis)
#   MEI_PREFIX   - install directory (default: /usr/local/bin, falls back to
#                  $HOME/.local/bin when it cannot be written)
set -euo pipefail

version="${MEI_VERSION:-latest}"
repo="${MEI_REPO:-subhranshus-mindfire/meiosis}"
prefix="${MEI_PREFIX:-/usr/local/bin}"

say() { printf '\033[1;36m[mei install]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[mei install] error:\033[0m %s\n' "$*" >&2; exit 1; }

uname_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
uname_arch="$(uname -m)"
case "$uname_os" in
linux) goos="linux" ;;
darwin) goos="darwin" ;;
*) die "unsupported OS: $uname_os (linux/darwin only)" ;;
esac
case "$uname_arch" in
x86_64 | amd64) goarch="amd64" ;;
arm64 | aarch64) goarch="arm64" ;;
*) die "unsupported arch: $uname_arch (amd64/arm64 only)" ;;
esac

if [ "$version" = "latest" ]; then
	base="https://github.com/${repo}/releases/latest/download"
else
	base="https://github.com/${repo}/releases/download/${version}"
fi
archive="mei-${goos}-${goarch}.tar.gz"

say "platform: ${goos}/${goarch}, version: ${version}"
say "downloading ${archive} from ${repo}..."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -o "$tmp/$archive" "${base}/${archive}"
curl -fsSL -o "$tmp/SHA256SUMS.txt" "${base}/SHA256SUMS.txt"
(cd "$tmp" && sha256sum --check --status "SHA256SUMS.txt" --ignore-missing 2>/dev/null \
	|| grep "$archive" "$tmp/SHA256SUMS.txt" | sha256sum --check --status --ignore-missing -)

tar -xzf "$tmp/$archive" -C "$tmp"

if [ ! -w "$prefix" ] && [ "$prefix" = "/usr/local/bin" ]; then
	prefix="$HOME/.local/bin"
	say "/usr/local/bin not writable, installing to ${prefix}"
fi
mkdir -p "$prefix"
install -m 0755 "$tmp/mei-${goos}-${goarch}/mei" "$prefix/mei"
install -m 0755 "$tmp/mei-${goos}-${goarch}/meiosisd" "$prefix/meiosisd"

say "installed:"
"$prefix/mei" version
"$prefix/meiosisd" -version

if command -v mei >/dev/null 2>&1 && command -v meiosisd >/dev/null 2>&1; then
	say "done. next: mei init --ide antigravity"
else
	say "done. add ${prefix} to your PATH, then run: mei init --ide antigravity"
fi