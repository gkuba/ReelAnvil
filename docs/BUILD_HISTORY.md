# ReelAnvil — Build History

One maintained record of what was built, how it was checked, and where it landed. Add the newest entry at the top. Keep entries short: what changed, what was verified, and what wasn't.

## 2026-11-06 — Build 0.9.2: working Settings and Services pages (uncommitted)

- **Changes:** settings version 5 (`media_root`, `storage_limit_gb`, `hunter_model`, `reviewer_model`, `whisper_model`, `whisper_language`; version 4 files migrate by filling defaults, future versions are still refused). Paths & Storage (save, Check with write test, `Statfs` free space, bounded walk for used space, over-limit warning; saving creates missing `incoming/ temp/ completed/ failed/ archive/` at 0750 only when the root exists; arrival folder name is one constant). AI: installed-model selects for the two roles, background `/api/pull` stream owned by the server (one at a time, stoppable, cancelled on shutdown, 5-minute stall limit, bytes summed from Ollama's per-layer numbers), `/api/generate` model test (60 s, write deadline extended for that request). Transcription: model and language fields with the service's reported models. Connectors: honest static page. Services: five status cards checked in parallel with 3 s limits, summary with commands. Installer: `compose.yaml` is now `compose.yaml.tmpl` rendered from `config.json` (validated Whisper values and media root), `install`/`upgrade` create the media folders, `doctor` checks the media root (exists, writable by `reelanvil` via `runuser ... test -w`) and notes a compose file that no longer matches the settings.
- **Verified:** `gofmt -l .` empty, `go vet ./...`, `go test -race ./...`, `go mod verify`, `scripts/build-dev.sh` pass. New tests: v4→v5 migration and validation, media root rules (also compared with the installer's), storage check on temp dirs (sparse 1.5 GB file for the limit, entry cap, read-only dir), folder creation never touching existing files, Ollama stubs for tags, pull stream (progress, errors, shutdown, cancel, stall), generate (reply, 404, garbage, timeout), Wyoming stub on the Transcription page, Services page ready / needs action / hanging services, sidebar highlight for every page, cross-origin and method checks on the new routes, compose rendering and refusal of unsafe values, media folder ownership, doctor media root checks. A scratch `serve --data-dir <scratch> --addr 127.0.0.1:18089` run returned 200 for all 15 pages and created `media/` with the five folders; the server was stopped afterwards.
- **Not verified:** a real model pull and model test against the Ollama container, `upgrade` / `doctor` on the Debian 13 host with the new compose template, the pages in a browser at desktop and 375px widths, and the maintainer's visual review. The Services page was seen only with Ollama and Whisper down (scratch run).

## 2026-11-06 — Build 0.9.2: service settings pages and installer pre-fill (`baa7fe0`)

- **Changes:** settings version 4 with `ollama_address`, `whisper_address`, `media_engine`, `ffmpeg_path`, `ytdlp_path` (older files migrate by filling defaults; unknown future versions are still refused). New Settings → AI, Transcription and Media pages with save forms and Test connection buttons (Ollama `/api/version` + `/api/tags`; Whisper TCP dial then Wyoming `describe`; host programs via `exec.LookPath`, never run; no container socket access). `install` and `upgrade` write or complete `server/settings/config.json`; `doctor` checks the two service addresses accept a TCP connection (FAIL line, no repair). In `internal/app/services.go`, templates `ai.html`, `transcription.html`, `media-settings.html`, `internal/install/config.go`.
- **Verified:** `gofmt`, `go vet`, `go test -race`, `go mod verify` and `scripts/build-dev.sh` pass. Tests use httptest servers for Ollama and a `net.Listen` stub for Wyoming, plus migration, validation, cross-origin, sidebar-highlight and installer/doctor cases. A scratch `serve --data-dir` run returned 200 for the three pages and wrote the version 4 file.
- **Fix:** form posts from LAN clients over plain HTTP were rejected ("cross-origin request rejected"): `Referrer-Policy: no-referrer` makes browsers send `Origin: null`, and non-secure origins send no `Sec-Fetch-Site` to vouch for it. Now `Referrer-Policy: same-origin`, with a regression test.
- **Verified later:** pages at desktop and 375px on a scratch copy (no sideways scroll); `upgrade` and `doctor` on the Debian 13 host (all ok); both Test connection actions reach the real Ollama and Whisper containers.
- **Not verified:** the maintainer's visual review.

## 2026-11-06 — Build 0.9.2: Go installer (pushed to `dev`)

- **Changes:** `install [--service]`, `upgrade [--dry-run]`, `doctor [--repair [--clean]]` and `uninstall` with `--yes`, `--dir` (default `/opt/ReelAnvil`) and `--port` (default 4747); `serve` now defaults to `0.0.0.0:4747` too, so the Settings page shows the real port. In `internal/install/` and `cmd/reelanvil/install.go`, following the manual install runbook proven on the Debian 13 reference host.
- **Verified:** `gofmt`, `go vet`, `go test -race` and `scripts/build-dev.sh` pass. On the Debian 13 reference host: install, a real `upgrade`, `doctor --repair --clean`, and 5 clean loops (install `--service --yes` → validate → uninstall `--yes` incl. Podman → reboot → host snapshot identical to the pre-install baseline), all checks passing each time: LAN 200 on 4747, runs as `reelanvil`, 3 containers, Ollama, Whisper port, media FFmpeg/yt-dlp/Deno, 10300/11434 on loopback only. A bug review then found 10 issues (deleting an existing `--dir` or the user's own Podman storage, purging later dependents, root following symlinks in service-owned folders, `uninstall --yes` deleting data, `install --service` ignored on an existing install, uninstall reruns, `userdel` of a pre-existing user, ownership after a UID change, `doctor` exit code); all fixed with regression tests, then 1 more clean loop passed and Settings showed port 4747 on the host.
- **Not verified:** plain `install` without `--service` in the loops; upgrading from an older install; the ufw rule and Docker-only prompt (not built); the maintainer's visual review.

## 2026-10-31 — Build 0.9.2: LAN listening and network settings (`1ef80fd`, pushed to `dev`)

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
