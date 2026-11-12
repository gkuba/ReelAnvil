#!/usr/bin/env bash
# Build the release archive in dist/ from an explicit allowlist: the binary,
# README.md and docs/licensing/. Nothing else is ever added. Then run the
# release check on it. Usage: scripts/package-release.sh [version]
set -euo pipefail
cd "$(dirname "$0")/.."
version="${1:-$(sed -n 's/^var Version = "\(.*\)"$/\1/p' internal/app/app.go)}"
[[ "$version" =~ ^[0-9A-Za-z._+-]+$ ]] || { echo "bad version: $version" >&2; exit 1; }
name="reelanvil-$version-linux-amd64"
stage="$(mktemp -d)"
trap 'rm -rf -- "$stage"' EXIT
mkdir -p "$stage/$name/licensing" dist

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
	-ldflags "-X github.com/gkuba/ReelAnvil/internal/app.Version=$version" \
	-o "$stage/$name/reelanvil" ./cmd/reelanvil
cp README.md "$stage/$name/README.md"
cp docs/licensing/LICENSE docs/licensing/LICENSING.md docs/licensing/THIRD_PARTY_NOTICES.md "$stage/$name/licensing/"

# Reproducible archive: fixed order, owner and times.
tar --sort=name --owner=0 --group=0 --numeric-owner --mtime='2026-01-01 00:00:00' \
	-czf "dist/$name.tar.gz" -C "$stage" "$name"
(cd dist && sha256sum "$name.tar.gz" > "$name.tar.gz.sha256")
scripts/check-release.sh "dist/$name.tar.gz"
printf 'Release archive: dist/%s.tar.gz\n' "$name"
