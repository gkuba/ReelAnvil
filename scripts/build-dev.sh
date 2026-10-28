#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
root="$(pwd -P)"
target="$root/bin/Release"
[[ "$target" == "$root/bin/Release" && "$root" != / ]] || { echo 'unsafe build path' >&2; exit 1; }
rm -rf -- "$target"
mkdir -p "$target"
CGO_ENABLED=0 go build -trimpath -o "$target/reelanvil" ./cmd/reelanvil
printf 'Build complete: %s\n' "$target/reelanvil"
printf 'Run: cd bin/Release && ./reelanvil serve\n'
