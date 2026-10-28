# ReelAnvil — Project Status

**Last reviewed:** 2026-10-28
**Phase:** Build 0.7.1 settings POST regression fix; maintainer validated, ready for Chat source validation
**Source baseline:** Clean local `dev` checkpoint `0590e3630e80898e53cda1c7e6d4d63ae692c7f5`. The B0.7 browser regression was reproduced: a same-origin browser POST arrived with `Origin: null`, `Sec-Fetch-Site: same-origin`, and no Referer, then was rejected. The candidate permits this exact combination while retaining rejection of cross-site metadata and mismatched origins. Automated checks and real-browser settings workflows pass; maintainer confirmed the fix works.
**Repository URL / branch:** `git@github.com:gkuba/ReelAnvil.git` / `dev`.
**Latest successful local build:** Build 0.7.1 candidate binary at `bin/Release/reelanvil`; SHA-256 and test/browser evidence are recorded in `docs/BUILD_HISTORY.md` and `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.7.1 Validation/`.
**Library synchronization checkpoint:** B0.7 remains the last pushed `dev` checkpoint until the accepted B0.7.1 source checkpoint is pushed and packaged for Chat.
**Security boundary:** B0.7.1 retains loopback-only binding, local Host checks, body limits, timeouts, and response headers. A `null` Origin is accepted only with `Sec-Fetch-Site: same-origin`; authentication is still absent, so network exposure remains disabled. See the B0.7 and B0.7.1 validation records.

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
- [x] Reproduced the B0.7 Apply Theme and Duplicate Theme POST failures in a real browser and recorded only header classifications.
- [x] Apply the B0.7.1 opaque-Origin exception, limited to `Origin: null` plus same-origin Fetch Metadata; preserve cross-origin rejection.
- [x] Exercise settings and theme POST routes through the shared production router in automated success and rejection tests.
- [x] Run B0.7.1 automated checks and local browser workflows; restore the pre-test runtime settings.
- [x] Maintainer validated B0.7.1 candidate; checkpoint and Chat handoff are the next actions.
- [ ] Complete remaining B0.7 theme-cycle, custom-theme, flyout-state, and narrow-layout checks.
- [ ] Complete authentication and session-bound CSRF design before any future non-loopback deployment.
- [ ] Remote deployment remains unauthorized; do not deploy without explicit approval.
- [x] Commit/push B0.4 to `dev` and prepare the exact commit ZIP/handoff for Chat.
- [x] Commit/push B0.5 to `dev` and prepare the exact commit ZIP/handoff for Chat.
- [ ] Plan and implement the persistent SQLite application foundation and worker/API contracts.

## Blockers / open items
- Build 0.7 remains a local-only mockup; it has no SQLite, authentication, AI/media integrations, jobs, or worker. Local request checks are defense-in-depth and do not replace authentication.
- Formal penetration testing and external dependency/SAST/secret scanning have not been performed; security scanners were unavailable in the local environment.
- Browser build validation used alternate port `127.0.0.1:18089`; ports 18080 and 18085 were occupied, so those ports are covered by handler-level tests rather than this candidate's live-browser run.
- Broader theme recovery, backup corruption, repair-lockout, interrupted-write, permissions, and malformed-config permutations remain deferred pre-release checks; see [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md).
- No remote deployment or production validation has been authorized or performed.
- Final copyright holder, third-party notices, vetted image digests and release packaging tests pending.

## Next verified milestone
Chat validates the pushed B0.7.1 source checkpoint against the handoff commit and checksum. Complete remaining recovery checks before release; remote deployment remains unauthorized. The local preview is at `http://127.0.0.1:18089/settings/themes` and stops with Ctrl+C.

## Approved deferred Campaigns workflow (post-Build 0.6)

Campaign discovery and local tracking are separate: multi-select available campaigns into a tracked list with requirements, dates and available budget information. Only tracked campaigns are eligible for post association. ReelAnvil-published posts associated with tracked campaigns capture post IDs/URLs automatically; manually published posts are searched only on explicit **Refresh Posts**. Present candidate thumbnails/links in the tracked listing and require explicit **Update Campaign** to submit fulfillment. See `docs/design/Campaigns_Workflow.md`. No integration is implemented by this design decision.
