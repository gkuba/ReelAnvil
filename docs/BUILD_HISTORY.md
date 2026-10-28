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
