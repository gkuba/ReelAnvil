#!/usr/bin/env bash
# Build the local development binary into bin/Release. Runtime data already in
# bin/Release (settings/, data/, logs/) is left untouched.
set -euo pipefail
cd "$(dirname "$0")/.."
root="$(pwd -P)"
target="$root/bin/Release"
[[ "$target" == "$root/bin/Release" && "$root" != / ]] || { echo 'unsafe build path' >&2; exit 1; }

# Version label: the base version from the source plus the commit, marked
# -dirty when the working tree has uncommitted changes.
base="$(sed -n 's/^var Version = "\(.*\)"$/\1/p' internal/app/app.go)"
version="$base"
if commit="$(git rev-parse --short HEAD 2>/dev/null)"; then
	version="$base+$commit"
	git diff --quiet HEAD -- 2>/dev/null || version="$version-dirty"
fi

mkdir -p "$target"
tmp="$target/.reelanvil.tmp.$$"
trap 'rm -f -- "$tmp"' EXIT
CGO_ENABLED=0 go build -trimpath -ldflags "-X github.com/gkuba/ReelAnvil/internal/app.Version=$version" -o "$tmp" ./cmd/reelanvil
mv -f -- "$tmp" "$target/reelanvil"
trap - EXIT
printf 'Build complete: %s (%s)\n' "$target/reelanvil" "$version"
printf 'Run: cd bin/Release && ./reelanvil serve\n'
