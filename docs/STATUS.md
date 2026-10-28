# ReelAnvil — Project Status

**Last reviewed:** 2026-10-28
**Phase:** Build 0.7 theme and security hardening accepted for the `dev` checkpoint
**Source baseline:** Build 0.7 applied to clean `dev` commit `dcc790d16b2131f8a5065280f9aac6742b27218a`; supplied post-patch hashes matched before local security hardening. Go tests, vet, and build pass with the added loopback/request guards. The maintainer confirmed the build loads and requested the checkpoint update; remaining browser checks are listed below.
**Repository URL / branch:** `git@github.com:gkuba/ReelAnvil.git` / `dev`.
**Latest successful local build:** Build 0.7 development binary; exact binary hash and evidence are in `docs/BUILD_HISTORY.md` and `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.7 Validation/`.
**Library synchronization checkpoint:** Build 0.7 is the accepted `dev` checkpoint being synchronized to GitHub and Chat. The handoff records the exact pushed commit.
**Security boundary:** B0.7 refuses non-loopback listeners, validates local Host/origin metadata, caps POST bodies, and adds timeouts/response headers. Authentication is not implemented; network exposure remains disabled. See `SECURITY_REVIEW.md` in the B0.7 validation folder.

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
- [x] Apply and locally validate the Build 0.5 Settings and theme refinements.
- [x] Maintainer review/acceptance of the B0.5 Settings pages, theme migration, and responsive layout.
- [x] Apply Build 0.6 patch, verify manifest hashes, run Go checks, and build the local candidate.
- [x] Maintainer accepted the B0.6 candidate and requested the next build handoff.
- [x] Apply Build 0.7 patch and verify all supplied post-patch hashes.
- [x] Add and unit-test loopback-only binding, local Host and cross-origin POST guards, body size cap, timeouts, and security headers.
- [x] Maintainer confirmed B0.7 loads and requested the checkpoint update.
- [ ] Complete remaining B0.7 theme-cycle, custom-theme, flyout-state, and narrow-layout checks.
- [ ] Complete authentication and session-bound CSRF design before any future non-loopback deployment.
- [ ] Remote deployment remains unauthorized; do not deploy without explicit approval.
- [x] Commit/push B0.4 to `dev` and prepare the exact commit ZIP/handoff for Chat.
- [x] Commit/push B0.5 to `dev` and prepare the exact commit ZIP/handoff for Chat.
- [ ] Plan and implement the persistent SQLite application foundation and worker/API contracts.

## Blockers / open items
- Build 0.7 remains a local-only mockup; it has no SQLite, authentication, AI/media integrations, jobs, or worker. Local request checks are defense-in-depth and do not replace authentication.
- Formal penetration testing and external dependency/SAST/secret scanning have not been performed; security scanners were unavailable in the local environment.
- Broader theme recovery, backup corruption, repair-lockout, interrupted-write, permissions, and malformed-config permutations remain deferred pre-release checks; see [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md).
- No remote deployment or production validation has been authorized or performed.
- Final copyright holder, third-party notices, vetted image digests and release packaging tests pending.

## Next verified milestone
Chat validation of the pushed B0.7 source checkpoint. Complete remaining browser and recovery checks before release; remote deployment remains unauthorized. Stop the local preview with Ctrl+C.

## Approved deferred Campaigns workflow (post-Build 0.6)

Campaign discovery and local tracking are separate: multi-select available campaigns into a tracked list with requirements, dates and available budget information. Only tracked campaigns are eligible for post association. ReelAnvil-published posts associated with tracked campaigns capture post IDs/URLs automatically; manually published posts are searched only on explicit **Refresh Posts**. Present candidate thumbnails/links in the tracked listing and require explicit **Update Campaign** to submit fulfillment. See `docs/design/Campaigns_Workflow.md`. No integration is implemented by this design decision.
