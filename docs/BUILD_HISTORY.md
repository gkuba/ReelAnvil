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
