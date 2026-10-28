# ReelAnvil — Project Status

**Last reviewed:** 2026-10-27
**Phase:** Build 0.4 theme refinement and Theme Manager validated on `dev`
**Source baseline:** Build 0.4 applied to accepted `dev` commit `3956fcb2bc5c3b03cc4ea634c1a95b1d13bad9dc`; local automated and manual validation passed, including a focused missing-primary backup-restore regression test. Maintainer approved the validated candidate for commit and push.
**Repository URL / branch:** `git@github.com:gkuba/ReelAnvil.git` / `dev`.
**Latest successful verified build:** Build 0.4 development binary built locally; exact binary hash and evidence are in `docs/BUILD_HISTORY.md` and `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.4 Validation/`.
**Library synchronization checkpoint:** Build 0.4 is approved for the development checkpoint; exact commit and Chat handoff are recorded in the matching deliverables package.

## Confirmed decisions
- Public, self-hosted, Linux-first (Debian 13) Go application with distinct server and media worker processes.
- The local repository is the Work source of truth; GitHub is its mirror and shared checkpoint for Chat.
- No custom Docker images or bundled third-party containers/models/executables; Compose pulls upstream.
- MIT license direction; final copyright attribution and linked dependency audit remain release gates.
- Nordic-industrial Aurora UI; real job progress and elapsed time, **no ETA**.
- `docs/licensing/` is included in release builds; other `docs/` files are not.
- One current `STATUS.md`, `CHANGELOG.md`, and `BUILD_HISTORY.md`, maintained in place.

## Current work
- [x] Create GitHub repository and push initial planning baseline to `main`.
- [x] Apply and locally validate the Build 0.1 Go/Aurora UI prototype on `dev`.
- [x] Apply and locally validate the Build 0.2 six-theme patch and legacy setting migrations.
- [x] Maintainer visual review/acceptance of the loopback UI prototype.
- [x] Apply and locally validate the Build 0.3 multi-tone theme refinement.
- [x] Maintainer review/acceptance of the B0.3 theme palettes, preview swatches, and metric-card layout.
- [x] Apply and locally validate the Build 0.4 Theme Manager and palette refinements.
- [x] Maintainer review/acceptance of the B0.4 Theme Manager UI and behavior.
- [ ] Remote deployment remains unauthorized; do not deploy without explicit approval.
- [x] Commit/push B0.4 to `dev` and prepare the exact commit ZIP/handoff for Chat.
- [ ] Plan and implement the persistent SQLite application foundation and worker/API contracts.

## Blockers / open items
- Build 0.4 remains a local-only mockup; it has no SQLite, authentication, CSRF protection, AI/media integrations, jobs, or worker.
- Broader theme recovery, backup corruption, repair-lockout, interrupted-write, permissions, and malformed-config permutations remain deferred pre-release checks; see [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md).
- No remote deployment or production validation has been authorized or performed.
- Final copyright holder, third-party notices, vetted image digests and release packaging tests pending.

## Next verified milestone
Chat validation of the pushed B0.4 source checkpoint. Recovery fault-injection tests remain pre-release work; remote deployment remains unauthorized. Stop the local preview with Ctrl+C.
