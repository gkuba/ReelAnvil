#!/usr/bin/env bash
# Scan tracked files for secret-like text and for files that must never be
# committed (databases, logs, media, tokens, env files). Exit 1 on a hit.
set -euo pipefail
cd "$(dirname "$0")/.."
fail=0
files="$(git ls-files)"
if grep -qE '(^|/)(\.env[^/]*|.*\.(db|db-wal|db-shm|log|token|pem|key|gguf|mp4|mkv|mov|webm|mp3|wav))$|(^|/)(settings|data|logs|temp)/' <<<"$files"; then
	echo "FAIL: runtime or media files are tracked:" >&2
	grep -E '(^|/)(\.env[^/]*|.*\.(db|db-wal|db-shm|log|token|pem|key|gguf|mp4|mkv|mov|webm|mp3|wav))$|(^|/)(settings|data|logs|temp)/' <<<"$files" >&2
	fail=1
fi
if git grep -nIE 'AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----|ghp_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,}|sk-[A-Za-z0-9]{32,}|xox[baprs]-[A-Za-z0-9-]{10,}' -- . ':!scripts/scan-secrets.sh' ':!scripts/check-release.sh'; then
	echo "FAIL: secret-like text in tracked files" >&2
	fail=1
fi
[ "$fail" = 0 ] && echo "Secret scan passed"
exit "$fail"
