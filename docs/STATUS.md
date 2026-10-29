# ReelAnvil — Project Status

**Last reviewed:** 2026-10-28
**Phase:** Build 0.9.1 layout consistency patch; validated and pushed to `dev`
**Source baseline:** Build 0.9.1 handoff baseline `011e75fd0666db7c9ddd5d94ec8ac8039cd3d3ed` on `dev`; local checkout was clean, and GitHub `dev` matched before implementation.
**Repository URL / branch:** `git@github.com:gkuba/ReelAnvil.git` / `dev`.
**Latest successful local build:** Build 0.9.1 candidate at `bin/Release/reelanvil`; current validation evidence is in `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build Archives/Build 0.9.1 Layout Consistency Validation/`.
**Library synchronization checkpoint:** Build 0.9.1 layout patch is verified on local and GitHub `dev` at `7073dd1374dc7924edd2358184b29370ef51afea`; exact packaged checkpoint SHA is recorded in the current Chat handoff package.
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
- [x] Apply the B0.8 SQLite foundation patch and pin the pure-Go SQLite driver.
- [x] Complete B0.8 database setup, failure-mode, persistence, and executable restart validation.
- [x] Complete B0.8 security/dependency source review, race tests, vet, module verification, executable build, and disposable database restart/failure validation.
- [x] Complete local B0.8 database validation and prepare the verified source checkpoint for Chat.
- [x] Verify the Build 0.9 checkout was clean at its supplied baseline; preserve pre-change source and runtime snapshots.
- [x] Implement the SQLite v1-to-v2 campaign migration, manual records, tracked status/notes, URL validation, and source-refresh preservation boundary.
- [x] Implement the Campaigns UI, data-driven Overview, local branding assets, and per-theme icon settings.
- [x] Finish disposable-runtime migration and browser checks at desktop and narrow widths; keep the existing `bin/Release` user database untouched during candidate validation.
- [x] After successful validation, checkpoint Build 0.9 to `dev` and prepare the exact source ZIP and handoff package for Chat.
- [x] Verify Build 0.9.1 handoff hashes and clean local/GitHub baseline at `011e75fd0666db7c9ddd5d94ec8ac8039cd3d3ed`.
- [x] Implement shared approved sidebar hierarchy and global theme/icon save with safe settings migration.
- [x] Complete Go checks, disposable settings/runtime migration tests, browser navigation and save/refresh/restart checks; capture requested desktop and narrow screenshots.
- [x] Apply the targeted Build 0.9.1 layout consistency patch and correct its mobile breakpoint specificity at 650px.
- [x] Pass Go tests, race tests, vet, module verification, build, and 88 route/layout combinations across four widths and both rail states.
- [x] Commit/push the validated layout patch and prepare the exact Chat checkpoint package.

## Blockers / open items
- The pushed B0.7.1 checkpoint remains a local-only mockup without SQLite, authentication, AI/media integrations, jobs, or worker. The local B0.8 candidate adds only a minimal SQLite foundation. Local request checks are defense-in-depth and do not replace authentication.
- Formal penetration testing and external dependency/SAST/secret scanning have not been performed; security scanners were unavailable in the local environment.
- Browser build validation used alternate port `127.0.0.1:18089`; ports 18080 and 18085 were occupied, so those ports are covered by handler-level tests rather than this candidate's live-browser run.
- Broader theme recovery, backup corruption, repair-lockout, interrupted-write, permissions, and malformed-config permutations remain deferred pre-release checks; see [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md).
- No remote deployment or production validation has been authorized or performed.
- Final copyright holder, third-party notices, vetted image digests and release packaging tests pending.
- Build 0.9 adds no authentication or external connectors; campaign discovery remains unavailable and campaign URLs are user-provided HTTPS values. No remote deployment is authorized.

## Next verified milestone
Build 0.9.1 layout consistency patch is locally validated and pushed to `dev` at `7073dd1374dc7924edd2358184b29370ef51afea`; Chat validation is next. The single Chat package ZIP is in `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/deliverables/`. To run the local prototype, use `cd /home/katala/Projects/ReelAnvil/bin/Release && ./reelanvil serve`, open <http://127.0.0.1:8080>, and stop with Ctrl+C. Startup upgrades schema v1 transactionally to v2. No remote deployment is authorized.

## Approved deferred Campaigns workflow (post-Build 0.6)

Campaign discovery and local tracking are separate: multi-select available campaigns into a tracked list with requirements, dates and available budget information. Only tracked campaigns are eligible for post association. ReelAnvil-published posts associated with tracked campaigns capture post IDs/URLs automatically; manually published posts are searched only on explicit **Refresh Posts**. Present candidate thumbnails/links in the tracked listing and require explicit **Update Campaign** to submit fulfillment. See `docs/design/Campaigns_Workflow.md`. No integration is implemented by this design decision.
