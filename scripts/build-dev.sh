#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
root="$(pwd -P)"
target="$root/bin/Release"
[[ "$target" == "$root/bin/Release" && "$root" != / ]] || { echo 'unsafe build path' >&2; exit 1; }
mkdir -p "$target"
tmp="$target/.reelanvil.tmp.$$"
trap 'rm -f -- "$tmp"' EXIT
CGO_ENABLED=0 go build -trimpath -o "$tmp" ./cmd/reelanvil
mv -f -- "$tmp" "$target/reelanvil"
trap - EXIT
printf 'Build complete: %s\n' "$target/reelanvil"
printf 'Run: cd bin/Release && ./reelanvil serve\n'
