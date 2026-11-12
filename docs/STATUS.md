# ReelAnvil — Project Status

**Last updated:** 2026-11-12
**Current build:** 0.9.2-dev on `dev`, release-ready pending the maintainer's review.
**Repository:** `git@github.com:gkuba/ReelAnvil.git` — `dev` for development, `main` for squashed releases/checkpoints.
**Run it:** `cd bin/Release && ./reelanvil serve`, then open <http://127.0.0.1:4747>.

## What exists today

- Go web server (`reelanvil serve`), listening on `0.0.0.0:4747` by default for trusted LANs; bind address, port and allowed host names are editable in Settings → General → Network, applied with a Restart ReelAnvil button, with Host checks, same-origin POST checks, a 1 MiB body cap, timeouts, and a CSP without `unsafe-inline`. No authentication.
- Services page: Web app, Ollama, Whisper and Storage status (plus Media engine when it is host programs) with a ready / needs action summary and exact commands. Container engines are left to `reelanvil doctor` because the web server has no container access.
- Embedded web UI: one shared layout and sidebar (expanded or compact rail with flyouts), responsive down to phone width.
- Settings (version 5): instance name; AI (Ollama address, hunter and reviewer model roles, background model pull with real bytes and elapsed time, model test), Transcription (Whisper address, model, language), Media (engine, host program paths), Paths & Storage (media root, storage limit, folder check) and Connectors (honest status page) with Test connection / Check buttons, pre-filled by `install`/`upgrade`; five built-in themes (Aurora, Dracula, Tomorrow Night, Nocturne, Foundry); custom themes stored in `settings/themes.json` with a last-known-good backup; a global application icon.
- SQLite (schema v4): manual campaign records with HTTPS-only links, local tracking status and notes, filters and sorting, and an Overview summary; jobs, steps, logs, clip candidates, clips and posts. Campaign discovery is deliberately disabled.
- Media pipeline: **Media** starts a job from `incoming/` or an HTTPS link (rights confirmation, optional campaign); **Jobs** shows import → transcribe → hunt → review → render → QA with real counts, elapsed time, log, Cancel and Retry; **Review** plays QA-passed clips, offers video and `.srt` downloads, approve / reject, and records and updates posts (link, views, earnings) for campaign clips; **Campaigns** shows each campaign's jobs, clips, approvals, posts, views and earnings. The hunter and reviewer models run in separate requests; the reviewer never sees the hunter's reasoning. The storage limit blocks a render instead of deleting anything.
- `reelanvil worker`: runs FFmpeg / yt-dlp steps for the web app over a loopback, token-protected API (`settings/worker.token`); the installed worker service starts the containers, then runs it.
- CLI: `serve`, `worker`, `doctor`, `version`, and the Debian 13 system installer (renders `compose.yaml` from the saved Whisper model, language and media root; creates the media folders; `doctor` checks the media root) `install [--service]`, `upgrade`, `doctor --repair [--clean]`, `uninstall` (user `reelanvil`, `/opt/ReelAnvil`, rootless Podman, port 4747; passed 5 clean install/uninstall loops on the reference host); `--data-dir`, `--addr` and `--allowed-hosts` flags (per-run overrides of the saved network settings).
- Tests cover security guards, migrations, settings and theme recovery, campaign validation, rendering and CSP compliance. CI runs on GitHub Actions.

## State of done (first release build)

Set by the maintainer on 2026-11-06. The release build is done when:

- **Hands-off pipeline.** The user picks a campaign and either drops media in `incoming/` or gives a URL, then presses Go. The software and models run everything up to Review with no one driving it, including automatic retries of rejected clips (capped per clip).
- **Feedback that does something.** On Review, a **Feedback** switch turns an approved clip into a good example and a rejected one into a lesson to avoid; lessons feed later jobs. A **Retry** switch on a reject remakes the clip using the note. A note with both switches off is kept for the user only. The user stops a clip's retries by rejecting without Retry.
- **Campaign platforms connected.** Campaigns from Content Rewards (Whop) and Vyro can be browsed in ReelAnvil through their public read APIs. Anything that needs a login (accepting a contract, submitting) is noted and decided after the first release build, in an extended test.
- **The user approves and posts.** Nothing is published or submitted without the user.
- A testable build in `bin/Release` for the maintainer's review.

## Next

**Goal:** give ReelAnvil a URL or source file and get clips that are usable for a campaign. Done so far: hands-off pipeline, scores and feedback, end-of-answer check, manifests, pause and filler cutting, three-word captions, title card, blurred fill (see `CHANGELOG.md`).

Still open, all after the first release:
1. Exact word times and cutting fillers inside sentences: needs a transcription service that returns word timestamps.
2. Re-transcribe the finished clip with a larger Whisper model for more accurate captions.
3. A crop that follows the speaker. Face detection needs model weights, which can't be bundled; decide the approach first.
4. Media Library page (sources and clips across jobs) and `temp/` cleanup for finished jobs (only with confirmation).

## Release state (2026-11-12)

Done: Debian 13 installer with 5 clean install/uninstall loops, Ollama and Whisper images pinned by digest, release archive built from an allowlist and checked (`scripts/package-release.sh`, `scripts/check-release.sh`), secret scan (`scripts/scan-secrets.sh`) in CI, dependency audit and notices (`docs/licensing/`), install, upgrade, backup and rollback guide in `README.md`, theme recovery automated checks.

Decided by the maintainer: **no authentication, trusted LAN only** (documented everywhere); **no trademark search or registration**; Campaigns stay a Developer feature (off by default); the media image tracks `:latest` so yt-dlp stays current.

Left for the maintainer: review the clips and the theme pages in a browser (`docs/PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`), tune the model prompts, then squash-merge `dev` into `main`, set the release version and tag. A clean-machine install from the final README on a fresh Debian 13 host has not been run (pixelreel is the reference host); no penetration test has been done.

## Confirmed decisions

- Public, self-hosted, Linux-first (Debian 13) Go application; separate server and media-worker processes from one codebase.
- Upstream Docker images only; no custom images or bundled models/executables.
- MIT license direction.
- Nordic-industrial dark UI; real progress and elapsed time, **never an ETA**.
- Human approval before anything is published or submitted.
- `docs/licensing/` ships in release builds; other `docs/` files do not.
- Approved deferred Campaigns workflow (discovery, tracking, post association): [`design/Campaigns_Workflow.md`](design/Campaigns_Workflow.md). Manual tracking and recording posts for approved clips are implemented; discovery is not.
