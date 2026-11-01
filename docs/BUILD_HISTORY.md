# ReelAnvil — Build History

One maintained record of what was built, how it was checked, and where it landed. Add the newest entry at the top. Keep entries short: what changed, what was verified, and what wasn't.

## 2026-10-31 — Build 0.9.2: LAN listening and network settings (uncommitted)

- **Changes:** listen address and allowed host names became saved settings (default `0.0.0.0:8080`) with a Network card on Settings → General; `POST /system/restart` checks the new address can be bound, renders a reconnect page, shuts down gracefully and re-executes the binary; a pending-restart banner on every page; Host policy accepts IP literals, the machine's name and listed names only when not on loopback; `--addr` / `--allowed-hosts` override per run; README not-for-internet disclaimer.
- **Verified:** `gofmt`, `go vet`, `go test -race` and `scripts/build-dev.sh` pass. On a scratch copy: saving port 18090 showed the banner, Restart reconnected the browser to `:18090`, and restarting back to `:18089` worked; restart page and Settings at 375px with no horizontal scroll; LAN-IP, hostname and foreign-Host requests returned 200/200/421 on a `0.0.0.0` scratch run.
- **Not verified:** a restart under systemd on the Debian host; access from a second machine on the LAN; govulncheck/staticcheck (CI).

## 2026-10-29 — Build 0.9.2: web UI restructure and fixes

- **Changes:** moved the inline HTML/CSS/JS out of Go strings into embedded `internal/app/web/` files with one layout; built-in palettes became Go data served as `/assets/themes.css`; CSP tightened to `script-src 'self'; style-src 'self'`; one-time cookie messages replaced `?message=` / `?error=`; SQLite schema v3 (date-only deadlines, platform/status validation in Go, ordered migrations); `--data-dir` / `--addr` flags, `version`, a fuller `doctor`, build version stamping; GitHub Actions CI; agent docs rewritten for the Claude Code workflow.
- **Bugs fixed:** manual campaigns stored View/Apply/Submit URLs one column off and left `created_at` empty (the v3 migration repairs affected rows); duplicating Dracula copied an outdated palette; duplicate-theme names could exceed 64 characters; duplicating a theme rewrote `config.json`; a campaign URL collision showed a raw SQLite error; untracking a missing campaign reported success; money input accepted `1.-5`.
- **Verified:** `gofmt`, `go vet`, `go test -race`, `go mod verify` and `scripts/build-dev.sh` pass. A v2 database with sample campaigns migrated to v3 on a scratch copy (URLs repaired, foreign key and cascade intact). Browser checks on the scratch copy covered every route, a computed-style comparison against Build 0.9.1, theme preview/save/duplicate/edit, the compact rail flyout, campaign add/edit errors, and 375px phone width with no overflow. The maintainer reviewed the build from `bin/Release`.
- **Not verified locally:** govulncheck and staticcheck (they run in CI).
- **Visual change:** Settings pages now use the same headings, labels, cards and sidebar as Campaigns.

## Builds 0.1–0.9.1 (2026-10-27 to 2026-10-28) — ChatGPT handoff era

These builds were designed in ChatGPT and applied, built and pushed by a local ChatGPT Work agent through a patch-and-handoff process. That history has been squashed into a single commit on `main`. The original commits remain at tag **`archive/chatgpt-build-0.9.1`**, and the detailed per-build validation records are in the maintainer's local archive.

| Build | Summary |
|---|---|
| 0.1 | Go server and Aurora UI shell, persistent JSON settings, `/healthz`. |
| 0.2 | Six-theme settings with legacy theme-ID migration. |
| 0.3 | Multi-tone theme palettes and metric cards. |
| 0.4 | Theme Manager: duplicate, edit, preview and save custom themes; backup restore. |
| 0.5 | Settings split into General, Themes, Connectors, Paths & Storage; SVG icons. |
| 0.6 | Compact navigation rail with flyouts; glass-style panels. |
| 0.7 | Dracula consolidated as `dracula-dark`; loopback-only binding, Host/origin checks, body cap, timeouts, security headers. |
| 0.7.1 | Accept `Origin: null` only with same-origin Fetch Metadata; shared-router POST tests. |
| 0.8 | SQLite foundation (pure-Go `modernc.org/sqlite`), WAL, startup read/write probe, schema v1. |
| 0.9 | Schema v2 campaign records and tracking, Campaigns and Overview pages, branding assets. |
| 0.9.1 | Unified sidebar hierarchy, global icon setting (settings v3), layout consistency fixes. |
