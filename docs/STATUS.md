# ReelAnvil — Project Status

**Last updated:** 2026-10-29
**Current build:** 0.9.2-dev, local development prototype on `dev`. Not a release.
**Repository:** `git@github.com:gkuba/ReelAnvil.git` — `dev` for development, `main` for squashed releases/checkpoints.
**Run it:** `cd bin/Release && ./reelanvil serve`, then open <http://127.0.0.1:8080>.

## What exists today

- Go web server (`reelanvil serve`) bound to loopback only, with Host checks, same-origin POST checks, a 1 MiB body cap, timeouts, and a CSP without `unsafe-inline`. No authentication.
- Embedded web UI: one shared layout and sidebar (expanded or compact rail with flyouts), responsive down to phone width.
- Settings: instance name; five built-in themes (Aurora, Dracula, Tomorrow Night, Nocturne, Foundry); custom themes stored in `settings/themes.json` with a last-known-good backup; a global application icon.
- SQLite (schema v3): manual campaign records with HTTPS-only links, local tracking status and notes, filters and sorting, and an Overview summary. Campaign discovery is deliberately disabled.
- CLI: `serve`, `doctor`, `version`; `--data-dir` and `--addr` flags.
- Tests cover security guards, migrations, settings and theme recovery, campaign validation, rendering and CSP compliance. CI runs on GitHub Actions.

## Placeholders (UI only, no function)

Jobs, Media Library, Review, Services, Connectors and the Paths & Storage fields. No media is imported or processed; no AI, worker or container integration exists yet.

## Next

1. Media pipeline vertical slice: import a local file → probe → transcript → choose a clip → render with FFmpeg → review page. Start with local stand-ins; container integration waits for the Debian 13 host.
2. Worker process API contract (`reelanvil worker`) and job state model with real stage progress (no ETA).
3. Storage paths and quota settings becoming real.

## Open items and release gates

- **Authentication and session-bound CSRF** must exist before any non-loopback deployment. Remote deployment is not authorized.
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
