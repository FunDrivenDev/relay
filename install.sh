#!/bin/sh
# Installs relay, or updates it when run again, without sudo:
#
#   curl -fsSL https://raw.githubusercontent.com/FunDrivenDev/relay/main/install.sh | sh
#
# It downloads the latest release (RELAY_VERSION=1.2.3 for another) for
# Linux amd64 or arm64, checks its sha256 against the release's
# checksums.txt, installs relay into RELAY_INSTALL_DIR ($HOME/.local/bin by
# default), then runs relay doctor check.
set -eu

repo=FunDrivenDev/relay
dir=${RELAY_INSTALL_DIR:-$HOME/.local/bin}

say() { printf '%s\n' "$*" >&2; }
die() {
    say "relay install: $*"
    exit 1
}

os=$(uname -s)
[ "$os" = Linux ] || die "relay is built for Linux, not $os"
machine=$(uname -m)
case $machine in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "no relay build for $machine: amd64 and arm64 only" ;;
esac
command -v curl >/dev/null 2>&1 || die "curl is needed: sudo apt-get install -y curl"

if [ -n "${RELAY_VERSION:-}" ]; then
    version=${RELAY_VERSION#v}
    base="https://github.com/$repo/releases/download/v$version"
else
    version=latest
    base="https://github.com/$repo/releases/latest/download"
fi
archive="relay_linux_$arch.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM

say "downloading relay ($version, linux $arch)"
curl -fsSL -o "$tmp/$archive" "$base/$archive" || die "cannot download $base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || die "cannot download $base/checksums.txt"

want=$(grep " $archive\$" "$tmp/checksums.txt" | sed 's/ .*//')
[ -n "$want" ] || die "$archive is not in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$tmp/$archive" | sed 's/ .*//')
elif command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "$tmp/$archive" | sed 's/ .*//')
else
    die "sha256sum or shasum is needed to check the download"
fi
[ "$got" = "$want" ] || die "checksum mismatch for $archive: got $got, want $want"

tar -xzf "$tmp/$archive" -C "$tmp" relay || die "cannot extract relay from $archive"
mkdir -p "$dir"
# Replaced by a rename: a relay running from $dir keeps its own file.
cp "$tmp/relay" "$dir/.relay.new"
chmod 755 "$dir/.relay.new"
mv -f "$dir/.relay.new" "$dir/relay"
say "installed $("$dir/relay" --version) in $dir"

relay=relay
case ":$PATH:" in
    *":$dir:"*) ;;
    *)
        relay="$dir/relay"
        say "warning: $dir is not on PATH: add it to your shell's profile (export PATH=\"$dir:\$PATH\")"
        ;;
esac

say ""
if "$dir/relay" doctor check; then
    :
else
    say ""
    say "to install what is missing: $relay doctor fix"
fi
