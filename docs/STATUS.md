# ReelAnvil — Project Status

**Last reviewed:** 2026-10-27
**Phase:** Build 0.3 multi-tone theme and layout review
**Source baseline:** Build 0.3 multi-tone refinement reviewed and accepted by the maintainer, committed on `dev`; exact commit is in the Build 0.3 Chat handoff.
**Repository URL / branch:** `git@github.com:gkuba/ReelAnvil.git` / `dev`.
**Latest successful verified build:** Build 0.3 development binary built locally; exact binary hash and evidence are in `docs/BUILD_HISTORY.md` and the validation folder.
**Library synchronization checkpoint:** Build 0.3 source checkpoint is pushed and packaged for Chat validation.

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
- [ ] Remote deployment remains unauthorized; do not deploy without explicit approval.
- [x] Commit/push B0.3 to `dev` and prepare the full commit ZIP/handoff for Chat.
- [ ] Plan and implement the persistent SQLite application foundation and worker/API contracts.

## Blockers / open items
- Build 0.3 is a local-only mockup; it has no SQLite, authentication, CSRF protection, AI/media integrations, jobs, or worker.
- No remote deployment or production validation has been authorized or performed.
- Final copyright holder, third-party notices, vetted image digests and release packaging tests pending.

## Next verified milestone
Chat validation of the pushed Build 0.3 source checkpoint, followed by planning the persistent SQLite application foundation and worker/API contracts. Remote deployment remains unauthorized. Stop the local preview with Ctrl+C.
