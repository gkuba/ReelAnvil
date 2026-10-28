# ReelAnvil — Project Status

**Last reviewed:** 2026-10-27
**Phase:** Build 0.2 six-theme local UI prototype validation
**Source baseline:** Build 0.2 six-theme prototype accepted by the maintainer and committed on `dev`; see the Build 0.2 handoff for the exact commit.
**Repository URL / branch:** `git@github.com:gkuba/ReelAnvil.git` / `dev`.
**Latest successful verified build:** Build 0.2 development binary built locally; exact binary hash and evidence are in `docs/BUILD_HISTORY.md` and the validation folder.
**Library synchronization checkpoint:** Build 0.2 source checkpoint is pushed and packaged for Chat validation.

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
- [ ] Remote deployment remains unauthorized; do not deploy without explicit approval.
- [x] Commit/push the validated candidate to `dev` and prepare the full commit ZIP/handoff for Chat.
- [ ] Plan and implement the persistent SQLite application foundation and worker/API contracts.

## Blockers / open items
- Build 0.2 is a local-only mockup; it has no SQLite, authentication, CSRF protection, AI/media integrations, jobs, or worker.
- No remote deployment or production validation has been authorized or performed.
- Final copyright holder, third-party notices, vetted image digests and release packaging tests pending.

## Next verified milestone
Chat validation of the pushed Build 0.2 source checkpoint, followed by planning the persistent SQLite application foundation and worker/API contracts. Remote deployment remains unauthorized. Stop the local preview with Ctrl+C.
