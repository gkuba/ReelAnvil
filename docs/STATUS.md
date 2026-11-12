# ReelAnvil — Project Status

**Last updated:** 2026-11-12
**Current build:** 0.9.2-dev, local development prototype on `dev`. Not a release.
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

Done 2026-11-12: per-clip end-of-answer check, clip manifests, Campaigns behind Settings → General → Developer features (off by default). Still to verify: the end check with a real model on the test host.

**Goal:** give ReelAnvil a URL or source file and get clips that are usable for a campaign. Clip quality comes first; campaign platform connections may be dropped.

1. Maintainer review of the Feedback / Retry / Lessons build on the test host.
2. Scores per criterion (hook, value, clarity, …) with reasons, plus hook / setup / body / outro labels, shown on Review. Prompt and page changes only.
3. A manifest beside each rendered clip (source, times, crop, captions) so a render can be checked or redone.
4. Word-by-word captions and a short title card over the first seconds (FFmpeg; check that the upstream image has libass).
5. Blurred-background fill as an option instead of cropping to 9:16.
6. Cut silences and filler words inside a clip (needs Whisper word timings).
7. Re-transcribe the finished clip with a larger Whisper model for more accurate captions.
8. A crop that follows the speaker. Face detection needs model weights, which can't be bundled; decide the approach first.
9. Media Library page (sources and clips across jobs) and cleanup of `temp/` for finished jobs (only with confirmation).

Ideas come from reading other open-source clippers; all code is our own (see `AGENTS.md`).

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
- Approved deferred Campaigns workflow (discovery, tracking, post association): [`design/Campaigns_Workflow.md`](design/Campaigns_Workflow.md). Manual tracking and recording posts for approved clips are implemented; discovery is not.
