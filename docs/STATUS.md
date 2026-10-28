# ReelAnvil — Project Status

**Last reviewed:** 2026-10-27
**Phase:** Build 0.1 local UI prototype validation
**Source baseline:** Local `dev` created from `main` commit `53d2a184e13e9088883ca740084088c12d590694`; Build 0.1 candidate is locally validated.
**Repository URL / branch:** `git@github.com:gkuba/ReelAnvil.git` / `dev`.
**Latest successful verified build:** Build 0.1 local UI prototype; exact pushed source commit is identified in its Chat handoff package.
**Library synchronization checkpoint:** Chat handoff package prepared for independent validation and Library sync.

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
- [x] Prepare the Build 0.1 source checkpoint and Chat handoff on `dev`.
- [ ] Maintainer review/acceptance of the loopback UI prototype.
- [ ] Remote deployment remains unauthorized; do not deploy without explicit approval.
- [ ] Plan and implement the persistent SQLite application foundation and worker/API contracts.

## Blockers / open items
- Build 0.1 is a local-only mockup; it has no SQLite, authentication, CSRF protection, AI/media integrations, jobs, or worker.
- No remote deployment or production validation has been authorized or performed.
- Final copyright holder, third-party notices, vetted image digests and release packaging tests pending.

## Next verified milestone
Maintainer review of the loopback-only Build 0.1 candidate, then decide whether to authorize remote validation and a development push.
