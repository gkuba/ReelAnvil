# ReelAnvil — Project Status

**Last reviewed:** 2026-10-27
**Phase:** Build 0.6 navigation and visual refinement candidate under maintainer review
**Source baseline:** Build 0.6 applied to clean `dev` commit `8a914fff66716c30fadbfe591edc500419040580`; supplied post-patch hashes matched before formatting. Automated tests, vet, and build passed; browser visual review is partial and interactive checks await maintainer review.
**Repository URL / branch:** `git@github.com:gkuba/ReelAnvil.git` / `dev`.
**Latest successful local build:** Build 0.6 development binary; exact binary hash and evidence are in `docs/BUILD_HISTORY.md` and `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.6 Validation/`.
**Library synchronization checkpoint:** Build 0.5 is the latest committed and pushed checkpoint. Build 0.6 awaits maintainer review before commit/push.

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
- [ ] Maintainer review of B0.6 navigation/flyout interaction and responsive behavior.
- [ ] Remote deployment remains unauthorized; do not deploy without explicit approval.
- [x] Commit/push B0.4 to `dev` and prepare the exact commit ZIP/handoff for Chat.
- [x] Commit/push B0.5 to `dev` and prepare the exact commit ZIP/handoff for Chat.
- [ ] Plan and implement the persistent SQLite application foundation and worker/API contracts.

## Blockers / open items
- Build 0.5 remains a local-only mockup; it has no SQLite, authentication, CSRF protection, AI/media integrations, jobs, or worker.
- Broader theme recovery, backup corruption, repair-lockout, interrupted-write, permissions, and malformed-config permutations remain deferred pre-release checks; see [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md).
- No remote deployment or production validation has been authorized or performed.
- Final copyright holder, third-party notices, vetted image digests and release packaging tests pending.

## Next verified milestone
Maintainer review of the local B0.6 candidate at `http://127.0.0.1:18085/settings/themes`. Recovery fault-injection tests remain pre-release work; remote deployment remains unauthorized. Stop the local preview with Ctrl+C.

## Approved deferred Campaigns workflow (post-Build 0.6)

Campaign discovery and local tracking are separate: multi-select available campaigns into a tracked list with requirements, dates and available budget information. Only tracked campaigns are eligible for post association. ReelAnvil-published posts associated with tracked campaigns capture post IDs/URLs automatically; manually published posts are searched only on explicit **Refresh Posts**. Present candidate thumbnails/links in the tracked listing and require explicit **Update Campaign** to submit fulfillment. See `docs/design/Campaigns_Workflow.md`. No integration is implemented by this design decision.
