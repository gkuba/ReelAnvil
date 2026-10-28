# ReelAnvil — Build and Verification History

This is the **single maintained build history**. Append dated records here rather than creating one document per build.

## Record format
- Date (UTC):
- Work handoff / patch identifier:
- Base GitHub repository, branch and SHA:
- Build environment / platform:
- Commands and test results (with evidence references):
- Deployment/functional checks:
- Outcome: success / failed / blocked
- Resulting commit SHA and push verification (only on success):
- Returned evidence archive:
- Status / checkpoint update reference:
- Known limitations or follow-up:

## History

### 2026-10-27 — Build 0.1 local UI prototype candidate

- Handoff: `ReelAnvil_Build_0.1_Work_Handoff.zip`; patch SHA-256 `48830ebf2c08f03a3b57d8fa1a21ec3a2c4f4b128fb48d0b2e49c3251336d76e`.
- Base repository/branch/commit: `gkuba/ReelAnvil`, local `dev`, `53d2a184e13e9088883ca740084088c12d590694` (created from local `main`; no `origin/dev` existed at start).
- Applied patch post-hashes matched all six handoff manifest entries. During smoke testing, `GET /settings` returned a redirect instead of rendering; corrected the handler to render on GET and reject unsupported methods. This is an additional local change beyond the supplied patch. Go formatting was applied.
- Environment: Linux AMD64, Go `go1.27.1-X:nodwarf5`; `go test ./...` passed; `go vet ./...` passed; `./scripts/build-dev.sh` succeeded. Development binary: `bin/Release/reelanvil`, SHA-256 `7a8802d862d0f54e5e8bc71eb312f287ad9df63ee6470a2e6f94686dbf735690`.
- Loopback validation at `127.0.0.1:18080`: `/healthz` returned HTTP 200 with `{"status":"ok"}`; overview, jobs, media, review, campaigns, services, diagnostics and settings returned HTTP 200; settings POST returned 303 and saved the instance name/theme; saved values were confirmed after server restart. Runtime output stayed under ignored `bin/Release/`.
- Outcome: local build and smoke checks successful after the settings-route correction. No remote deployment or user acceptance has occurred. The exact development commit and push are identified in the accompanying Chat handoff. `dist/` was not created.
- Evidence logs and binary hash: `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.1 Validation/`.
- Start/stop, developer commands, and maintainer checks: [`DEVELOPMENT.md`](DEVELOPMENT.md).
- Next: maintainer review/acceptance; then decide on remote validation and development push. Preserve this `dev` candidate and do not publish a release without explicit approval.

### 2026-10-27 — Build 0.2 six-theme prototype candidate

- Handoff: `ReelAnvil_Build_0.2_Work_Handoff.zip`; archive SHA-256 `00d5a031073597572353dad293aeae37bce59b96ce4a730aaad98e3d2edee7a9`; patch SHA-256 `11051737a2a5c9890b5dc229a379154488b00860958ab0268d27ad206a149d03`.
- Baseline repository/branch/commit: `gkuba/ReelAnvil`, local and `origin/dev`, `82fd874dea709cfa242b034c87af230db3beb329`; working tree was clean. Local GitHub-tracking refs agreed before patching.
- Both changed files matched the handoff's post-patch SHA-256 and size before formatting or further edits. `gofmt -l internal/app/app.go internal/app/app_test.go` returned no files.
- Environment: Linux AMD64, Go `go1.27.1-X:nodwarf5`; `go test ./...` passed, `go vet ./...` passed, and `./scripts/build-dev.sh` succeeded. Binary: `bin/Release/reelanvil`, SHA-256 `a9205a08dddccb846b773ae101a685fe49702f742b45ae568960ea38665a06d7`.
- Visual browser review confirmed all six theme choices immediately changed the settings-page palette and controls. Saving Tokyo Night returned the expected settings confirmation and persisted across a server restart. Runtime fixture checks confirmed `dark` -> `tomorrow-night`, `slate` -> `tokyo-night`, `pastel` -> `nocturne`, and an unrecognized value -> `aurora`; migrated values were persisted and rendered.
- `/healthz` returned HTTP 200 with `{"status":"ok"}`; `/settings`, `/`, `/jobs`, and `/diagnostics` returned HTTP 200. The test setting was restored to Aurora. The local server remains running at `127.0.0.1:18080` for maintainer review.
- The existing `bin/Release/` runtime files and binary were preserved before the build script cleared its target. Snapshot: `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.2 Validation/prebuild-bin-Release/`; source baseline archive and checksum are in the same folder.
- Outcome: local build and checks passed; maintainer accepted the served candidate. No remote deployment, release merge, or `dist/` package has occurred.
- Evidence: `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.2 Validation/`. Developer start/stop commands and maintainer check parameters: [`DEVELOPMENT.md`](DEVELOPMENT.md).
- Git delivery: accepted Build 0.2 source was committed and pushed to `dev`; exact commit and archive checksum are recorded in the Chat handoff package. No release action is authorized by this candidate.
- Next: Chat validation of the pushed source checkpoint. Local preview can be started from `bin/Release` with `REELANVIL_ADDR=127.0.0.1:18080 ./reelanvil serve` and stopped with Ctrl+C.

### 2026-10-27 — Build 0.3 multi-tone theme refinement candidate

- Handoff: `ReelAnvil_Build_0.3_Work_Handoff.zip`; archive SHA-256 `7fba98752ad2f471ece56be617fa2d92018e8ee6d9631acef914a9e7ef04e079`; patch SHA-256 `ee53f22206b6b86aadc70cd235ba9626dd8384fe9bad8c43f3015d3df3cedacf`.
- Baseline: `gkuba/ReelAnvil`, `dev`, `4804bd8e7f7562c9a2d74a32aea1d16a54533173`; working tree clean and exact before patch. Both patch files matched the post-patch manifest SHA-256 and byte sizes before further changes.
- Environment: Linux AMD64, Go `go1.27.1-X:nodwarf5`. `go test ./...` and `go vet ./...` passed; `./scripts/build-dev.sh` succeeded. The default Go cache was read-only in this managed session, so Go checks used `GOCACHE=/tmp/reelanvil-go-build-cache`.
- Binary: `bin/Release/reelanvil`, SHA-256 `d00e22c21e923bb324066c33d2755c34d60d8654e36a3d85880665108fdfab4f`.
- Browser visual checks cycled all six theme choices; each changed the settings palette and all seven preview swatches. Dracula Dark showed a neutral near-black charcoal base. Overview and Diagnostics showed differentiated blue, teal, and purple metric surfaces.
- Tokyo Night saved successfully, survived page reload and server restart, then Aurora was restored and saved. `/healthz`, `/`, `/jobs`, `/media`, `/review`, `/campaigns`, `/services`, `/diagnostics`, and `/settings` each returned HTTP 200 at loopback `127.0.0.1:18081`.
- The prior `bin/Release/` snapshot was preserved before the development build. Evidence logs, input handoff, and runtime snapshot: `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.3 Validation/`.
- Outcome: automated, build, route, persistence, and visual checks passed; maintainer reviewed and accepted the candidate. No release merge, remote deployment, or `dist/` package was made.
- Review preview: from `/home/katala/Projects/ReelAnvil/bin/Release`, run `REELANVIL_ADDR=127.0.0.1:18081 ./reelanvil serve`; open `http://127.0.0.1:18081`; stop with Ctrl+C. Return setting is Aurora.
- Git delivery: accepted Build 0.3 source was committed and pushed to `dev`; exact commit and archive checksum are in the Chat handoff package.
- Next: Chat validates the pushed source checkpoint. Remote deployment remains unauthorized.

### 2026-10-27 — Build 0.4 theme refinement and Theme Manager candidate

- Handoff: `ReelAnvil_Build_0.4_Work_Handoff.zip`; archive SHA-256 `e0f66d12020531a72b1f9a7567f42f39918aa4b62d73528bfd34ebde833cb98f`; patch SHA-256 `0a702200cd1823d623a07e95354031ae5e27f9d79fde76e8b16962dd4619c5f0`.
- Baseline: `gkuba/ReelAnvil`, `dev`, `3956fcb2bc5c3b03cc4ea634c1a95b1d13bad9dc`; local branch was clean and matched an independent GitHub `dev` check. The four changed source/test files matched the supplied post-patch hashes and byte counts before further edits. Work corrected two issues found during review: a missing primary themes file could overwrite an available backup with empty data, and an empty Theme Manager rendered unusable blank editor fields. Regression tests cover backup restoration and the empty-state prompt.
- Environment: Linux AMD64, Go `go1.27.1-X:nodwarf5`; `gofmt -l internal/app` returned no files; `GOCACHE=/tmp/reelanvil-go-build-cache go test ./...` passed; `go vet ./...` passed; `./scripts/build-dev.sh` succeeded.
- Binary: `bin/Release/reelanvil`, SHA-256 `69495dd4eddfd7776ca8885a0840913ce3d33350f625688fa039f261bc4ed5e1`.
- Browser checks covered all six built-ins. Aurora retained its previous B0.3 appearance; Dracula Dark showed cooler violet tones; Foundry showed steel, ember, and teal. Duplicating Aurora, renaming a custom theme, editing all seven hex colors, previewing without save, Cancel restoring saved values, and Save Changes all worked.
- The custom theme was selected and persisted through page reload and server restart. A valid manual edit to `settings/themes.json` was loaded by Refresh Themes, including updated name and primary color; the update also remained after server restart. Active theme was restored to Aurora. A named review theme remains under ignored runtime `bin/Release/settings/themes.json` for maintainer inspection.
- `/healthz`, `/`, `/jobs`, `/media`, `/review`, `/campaigns`, `/services`, `/diagnostics`, and `/settings` returned HTTP 200 at loopback `127.0.0.1:18082`.
- Basic unit tests for invalid hex, line-numbered JSON syntax, restoration of a missing primary from a valid backup, and the empty Theme Manager state passed. End-to-end fallback/recovery with invalid primary or backup, repair lockout, filesystem failures, interrupted writes, and malformed configuration permutations were not run; these are explicitly deferred to pre-release testing.
- Evidence, full prebuild `bin/Release` snapshot, baseline source archive, incoming handoff, JSON test data and logs are in `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.4 Validation/`.
- Outcome: local automated, build, route and requested basic visual/functional checks passed; maintainer approved the candidate, which is committed and pushed to `dev` with the matching Chat checkpoint package. No release merge, remote deployment or `dist/` package was made.
- Review server: from `/home/katala/Projects/ReelAnvil/bin/Release`, run `REELANVIL_ADDR=127.0.0.1:18082 ./reelanvil serve`; open `http://127.0.0.1:18082`; stop with Ctrl+C. Active setting is Aurora; Theme Manager contains the `Aurora Violet Refreshed` review profile.
- Next: Chat validates the pushed source checkpoint. Complete deferred recovery checks before release.

### 2026-10-27 — Build 0.5 Settings and theme refinement candidate

- Handoff: `ReelAnvil_Build_0.5_Work_Handoff.zip`; archive SHA-256 `0df0284af17c8ead97588194f1e9df7b56bb1d4ab8749c3b8fde3de70a660e0e`; patch SHA-256 `489e6678d5e87f440323f6a93b61062b4e4d947c6f3283f6f61b4ed947d3d67f`.
- Baseline: `gkuba/ReelAnvil`, `dev`, `8a14f0d0ac8fd8f5f8004f971d90c27b99da5582`; local branch was clean and matched GitHub. All five supplied post-patch files matched manifest hashes and byte counts before additional polish.
- Changes: Settings is split into General, Themes, Connectors, and Paths & Storage. Tokyo Night is removed from built-ins; its saved selection and custom-theme base migrate to Aurora. Dracula Classic and Dracula Lavender are selectable. Connector/storage fields remain disabled. Temporary settings glyphs were replaced with five inline SVG icons and a regression test.
- Environment: Linux AMD64, Go `go1.27.1-X:nodwarf5`; `gofmt -l internal/app` returned no files; `GOCACHE=/tmp/reelanvil-go-build-cache go test ./...` passed; `GOCACHE=/tmp/reelanvil-go-build-cache go vet ./...` passed; `./scripts/build-dev.sh` succeeded.
- Binary: `bin/Release/reelanvil`, SHA-256 `0c1fc2823d75faea13965fcb361ddeefe0c4b85d762434f2664154b9237b6baf`.
- Browser checks: all four Settings routes rendered with the correct active navigation item; connector/storage inputs were disabled. All six built-in themes changed their previews. Legacy Tokyo Night selection persisted as Aurora on startup; a Tokyo Night custom base rendered with Aurora-derived tokens. Refresh Themes left `config.json` unchanged; General saved an instance name while retaining Aurora. Desktop (1365 px) rendered the theme editor in two columns; narrow (390 px) wrapped navigation, used one column, and had no horizontal overflow.
- Runtime note: the pre-build saved config selected Tokyo Night. The B0.5 startup migration changed that saved selection to Aurora, as specified; the instance name and custom theme files were preserved. Pre-build and post-migration runtime snapshots are in `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.5 Validation/`.
- Deferred: broader theme recovery and failure-injection permutations remain pre-release checks. The maintainer approved this development checkpoint; it is committed and pushed to `dev` with the exact Chat package. No release merge, `dist/` package, or remote deployment was made.
- Review server: run `cd /home/katala/Projects/ReelAnvil/bin/Release && REELANVIL_ADDR=127.0.0.1:18085 ./reelanvil serve`; open `http://127.0.0.1:18085/settings/themes`; stop with Ctrl+C. Active theme is Aurora; custom theme `Aurora Violet Refreshed` remains available.
- Next: Chat validates the pushed source checkpoint.

### 2026-10-27 — Build 0.6 navigation and visual refinement candidate

- Handoff: `ReelAnvil_Build_0.6_Work_Handoff.zip`; archive SHA-256 `81c61897dab32d5cdf31f13480724c3ee0f6299ed55ed686200b287ad536925c`.
- Baseline: `gkuba/ReelAnvil`, local `dev` and GitHub `dev`, `8a914fff66716c30fadbfe591edc500419040580`; worktree was clean. Patch dry-run passed and the four supplied post-patch file hashes/sizes matched before formatting.
- Changes: persistent compact navigation rail, grouped navigation flyouts, SVG navigation/actions, glass-style panels, neutral Refresh/Cancel actions, and Dracula Lavender surface refinements. Campaign workflow was documented as deferred design only.
- Runtime: exact custom theme `Aurora Violet Refreshed` (ID `aurora-copy`) removed from both `settings/themes.json` and its backup after saving complete pre-change snapshots. Active theme remained Aurora. Snapshots: `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.6 Validation/runtime-before/`.
- Environment: Linux AMD64, Go `go1.27.1-X:nodwarf5`; `go test ./...`, `go vet ./...`, and `./scripts/build-dev.sh` passed. Development binary SHA-256 `6f65ea87e54717395906712a6e2581d7aee28929c9b9da32da50ced2c1b2c05d`.
- Browser preview opened on Themes at `127.0.0.1:18085`. Visual inspection confirmed the page renders with Aurora, seven preview swatches, SVG actions, and Theme Manager empty state after profile removal. Full interactive rail/flyout, keyboard, narrow-screen, and theme-cycle checks remain for maintainer review because browser interaction controls were unavailable in this session.
- Outcome: source hashes, formatting, automated tests, vet, and development build passed. Candidate awaits maintainer visual/functional review; no commit, push, deployment, release merge, or `dist/` package has occurred.
- Review server: from `/home/katala/Projects/ReelAnvil/bin/Release`, run `REELANVIL_ADDR=127.0.0.1:18085 ./reelanvil serve`; open `http://127.0.0.1:18085/settings/themes`; stop with Ctrl+C. Active theme is Aurora; the custom review profile was removed as requested in the handoff.
- Evidence and snapshots: `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.6 Validation/`. Next: maintainer review, then Chat validation after approval and push.

### 2026-10-28 — Build 0.7 theme consolidation and security hardening candidate

- Handoff: `ReelAnvil_Build_0.7_Work_Handoff.zip`; archive SHA-256 `1ad07249143853785434e36749d546d48aae581c32f45df2bf9b5e853d2d9a9e`; patch SHA-256 `62c0840183c8f38ec8dbc592f6a83d3204ccdbc5d2784beee2e28e7f877f2237`.
- Baseline: local `dev` and verified GitHub `dev`, commit `dcc790d16b2131f8a5065280f9aac6742b27218a`. Working tree was clean. Patch dry-run passed; all four supplied post-patch size/hash entries matched before gofmt and additional Work security changes.
- Product changes: one Dracula built-in with stable ID `dracula-dark` and Lavender palette; legacy saved selections and custom bases migrate from `dracula-lavender`; built-in surfaces are darker; preview labels/order are documented separately from seven persisted editor fields; active navigation uses primary-accent-derived colors. Campaign workflow documentation remains unchanged and deferred.
- Security review found the unauthenticated prototype could be configured to listen beyond loopback and state-changing forms lacked cross-origin checks. Work hardening now refuses non-loopback listener addresses, rejects non-local Host and cross-origin browser POST requests, caps POST bodies at 1 MiB, adds HTTP timeouts and response security headers. This is still unauthenticated software and must not be exposed beyond loopback.
- Environment: Linux AMD64, Go `go1.27.1-X:nodwarf5`; `go test -race ./...`, `go vet ./...`, `go mod verify`, and `./scripts/build-dev.sh` passed. There are no third-party Go modules. `govulncheck`, `gosec`, `gitleaks`, and `trivy` were unavailable; no scanner result is claimed.
- Binary: `bin/Release/reelanvil`, SHA-256 `be83479421ff131ec974c8fb53932963da1db1c77ab26d032e5149e3dbb438d9`.
- Browser review confirmed five built-in options, the consolidated Dracula lavender palette and pink tertiary swatch, and compact Media flyout rendering/navigation. All other theme cycles, link pseudo-states, narrow layout, and interactive custom-theme migration checks remain pending. Local runtime had no custom theme records; migration and color preservation have unit coverage.
- Outcome: local build and source checks pass; candidate awaits maintainer review. No commit, push, remote deployment, release merge, or `dist/` package has occurred.
- Review server: from `/home/katala/Projects/ReelAnvil/bin/Release`, run `REELANVIL_ADDR=127.0.0.1:18085 ./reelanvil serve`; open `http://127.0.0.1:18085/settings/themes`; stop with Ctrl+C. Existing local settings were restored from the pre-build snapshot; active theme remains Aurora.
- Evidence, source baseline, and pre-build runtime snapshot: `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.7 Validation/`. Focused findings and remaining security boundary: `SECURITY_REVIEW.md` in that folder. Next: maintainer review; after acceptance, commit/push and prepare the exact Chat checkpoint package.

### 2026-10-28 — Build 0.7.1 settings POST security regression candidate

- Handoff: `ReelAnvil_Build_0.7.1_Expanded_Settings_POST_Handoff.zip`; archive SHA-256 `93b9ed9ccd56bd2a11239bfd986df7d6ffbd9bc16bbd0cee24e1be703a5ca47d`. The targeted test patch SHA-256 matched `d4c8b27ecdd4a9dff2f6464e91d99700849dccac8aaf74892c6f74ebe7a28ee3`; the expanded checklist patch matched `2550b2c4b1158ee02293463c6b086701a2d47f0b4104aa004fb46f30e33ac132`. The supplied expanded checklist patch had a malformed hunk and failed `git apply --check`; its two requested checklist entries were applied directly.
- Baseline: clean local `dev`, commit `0590e3630e80898e53cda1c7e6d4d63ae692c7f5`.
- Browser reproduction on B0.7: `POST /themes/duplicate` and `POST /settings` returned `cross-origin request rejected`. Sanitized diagnostics recorded only method, route, Host, scheme, and classifications: Host `127.0.0.1:8080`, scheme HTTP, Origin opaque literal `null`, Referer missing, and `Sec-Fetch-Site` same-origin. No cookies, tokens, or form contents were recorded. The temporary diagnostic code was removed before the candidate changes.
- Fix: allow the exact `Origin: null` plus `Sec-Fetch-Site: same-origin` combination; all nonempty Fetch Metadata values other than `same-origin` are still rejected, and ordinary Origin/Referer scheme and host comparisons remain unchanged. The shared router is now constructed in one helper used by Serve and tests, allowing all POST routes to be exercised through the production middleware and router.
- Tests cover duplicate requests at `127.0.0.1:18080` and `:18085`, including opaque Origin, matching Origin, Referer-only, absent Origin with same-origin metadata, absent metadata with opaque Origin, cross-site, same-site, foreign-origin, mismatched-port, and foreign-Referer cases. Additional shared-router tests cover Apply Theme, General settings, Theme Refresh, Duplicate Theme, and custom-theme Save paths, plus foreign-origin rejection without persistent changes for every current settings POST route.
- Environment: Linux AMD64; Go version and command logs are in `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/Build 0.7.1 Validation/`. `go test -race ./...`, `go vet ./...`, `go mod verify`, and `./scripts/build-dev.sh` passed.
- Candidate binary: `bin/Release/reelanvil`, SHA-256 `808c9c504b716f591f1c0d9d8d126d8235eea94edbe58a9ac428234a8191fc92`.
- Browser validation at `127.0.0.1:18089`: Duplicate Theme opened the editor and persisted; custom-theme Save Changes, Refresh Themes, Apply Theme (Dracula), and General Settings all completed successfully. A browser form from `localhost:18090` posting to `127.0.0.1:18089` was rejected; config, themes and backup hashes did not change from immediately before to after the rejected request. The usual `:18080` and `:18085` bind ports were occupied, so the candidate browser run used `:18089`; automated route tests cover the former ports.
- The pre-test `config.json`, `themes.json`, and `themes.json.bak` were restored byte-for-byte after browser checks; active theme is Aurora and there are no custom review profiles. The prior ReelAnvil log was restored from the B0.7 prebuild snapshot and B0.7.1 preview listener entries were retained. `data/` and `temp/` were empty and recreated. The candidate server is left running at `127.0.0.1:18089` for maintainer review.
- Outcome: maintainer confirmed the candidate works. Source is being checkpointed to `dev` and packaged as an exact commit ZIP for Chat validation. No release merge, `dist/` package, or remote deployment is included.
