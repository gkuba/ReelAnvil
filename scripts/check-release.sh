#!/usr/bin/env bash
# Check a release archive: required files present, forbidden files absent,
# no secrets, and a binary that reports its version. Usage: check-release.sh <archive>
set -euo pipefail
archive="${1:?usage: check-release.sh <archive.tar.gz>}"
list="$(tar -tzf "$archive")"
fail=0
bad() { echo "FAIL: $*" >&2; fail=1; }

for f in reelanvil README.md licensing/LICENSE licensing/LICENSING.md licensing/THIRD_PARTY_NOTICES.md; do
	grep -qE "^[^/]+/$f$" <<<"$list" || bad "missing $f"
done
# Only the allowlist may be inside.
while IFS= read -r p; do
	case "${p#*/}" in
	"" | licensing/ | reelanvil | README.md | licensing/LICENSE | licensing/LICENSING.md | licensing/THIRD_PARTY_NOTICES.md) ;;
	*) bad "unexpected file: $p" ;;
	esac
done <<<"$list"
grep -qiE 'STATUS\.md|BUILD_HISTORY|CHANGELOG|\.db$|\.db-|\.log$|\.env|\.token|settings/|/data/|logs/|\.(mp4|mkv|mov|webm|mp3|wav|srt|gguf|bin)$' <<<"$list" && bad "runtime, history or media files in the archive"

tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
tar -xzf "$archive" -C "$tmp"
dir="$(echo "$tmp"/*)"
if grep -rIlE 'AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----|ghp_[A-Za-z0-9]{30,}|sk-[A-Za-z0-9]{32,}' "$dir" >/dev/null; then
	bad "secret-like text in the archive"
fi
"$dir/reelanvil" version >/dev/null || bad "binary does not run"
[ "$fail" = 0 ] && echo "Release check passed: $archive"
exit "$fail"
