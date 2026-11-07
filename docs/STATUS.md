# ReelAnvil — Project Status

**Last updated:** 2026-11-06
**Current build:** 0.9.2-dev, local development prototype on `dev`. Not a release.
**Repository:** `git@github.com:gkuba/ReelAnvil.git` — `dev` for development, `main` for squashed releases/checkpoints.
**Run it:** `cd bin/Release && ./reelanvil serve`, then open <http://127.0.0.1:4747>.

## What exists today

- Go web server (`reelanvil serve`), listening on `0.0.0.0:4747` by default for trusted LANs; bind address, port and allowed host names are editable in Settings → General → Network, applied with a Restart ReelAnvil button, with Host checks, same-origin POST checks, a 1 MiB body cap, timeouts, and a CSP without `unsafe-inline`. No authentication.
- Services page: Web app, Ollama, Whisper, Media engine and Storage status with a ready / needs action summary and exact commands. Container engines are left to `reelanvil doctor` because the web server has no container access.
- Embedded web UI: one shared layout and sidebar (expanded or compact rail with flyouts), responsive down to phone width.
- Settings (version 5): instance name; AI (Ollama address, hunter and reviewer model roles, background model pull with real bytes and elapsed time, model test), Transcription (Whisper address, model, language), Media (engine, host program paths), Paths & Storage (media root, storage limit, folder check) and Connectors (honest status page) with Test connection / Check buttons, pre-filled by `install`/`upgrade`; five built-in themes (Aurora, Dracula, Tomorrow Night, Nocturne, Foundry); custom themes stored in `settings/themes.json` with a last-known-good backup; a global application icon.
- SQLite (schema v3): manual campaign records with HTTPS-only links, local tracking status and notes, filters and sorting, and an Overview summary. Campaign discovery is deliberately disabled.
- CLI: `serve`, `doctor`, `version`, and the Debian 13 system installer (renders `compose.yaml` from the saved Whisper model, language and media root; creates the media folders; `doctor` checks the media root) `install [--service]`, `upgrade`, `doctor --repair [--clean]`, `uninstall` (user `reelanvil`, `/opt/ReelAnvil`, rootless Podman, port 4747; passed 5 clean install/uninstall loops on the reference host); `--data-dir`, `--addr` and `--allowed-hosts` flags (per-run overrides of the saved network settings).
- Tests cover security guards, migrations, settings and theme recovery, campaign validation, rendering and CSP compliance. CI runs on GitHub Actions.

## Placeholders (UI only, no function)

Jobs, Media Library and Review. No media is imported or processed; the saved model roles, Whisper settings, media root and storage limit are configuration only until the pipeline uses them. The Settings pages and Services are working.

## Next

1. Media pipeline vertical slice: import a local file → probe → transcript → choose a clip → render with FFmpeg → review page. Start with local stand-ins; container integration waits for the Debian 13 host.
2. Worker process API contract (`reelanvil worker`) and job state model with real stage progress (no ETA).
3. Enforce the storage limit when jobs exist (block jobs safely, never delete data); use the media root and folders in import and the worker.

## Open items and release gates

- **Authentication and session-bound CSRF** must exist before any deployment beyond a trusted LAN. Internet exposure is not authorized.
- Theme recovery failure-injection checks remain open: [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md).
- License copyright holder, third-party notice audit, trademark check, pinned image digests and release-packaging tests are pending.
- No penetration test yet. Secret scanning is not yet wired into CI.

## Confirmed decisions

- Public, self-hosted, Linux-first (Debian 13) Go application; separate server and media-worker processes from one codebase.
- Upstream Docker images only; no custom images or bundled models/executables.
- MIT license direction.
- Nordic-industrial dark UI; real progress and elapsed time, **never an ETA**.
- Human approval before anything is published or submitted.
- `docs/licensing/` ships in release builds; other `docs/` files do not.
- Approved deferred Campaigns workflow (discovery, tracking, post association): [`design/Campaigns_Workflow.md`](design/Campaigns_Workflow.md). Nothing beyond manual tracking is implemented.
