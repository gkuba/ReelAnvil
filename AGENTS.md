# ReelAnvil — Agent Guide

Rules for any coding agent (Claude Code or otherwise) working in this repository. The maintainer directs the work and approves releases. `CLAUDE.md` adds Claude-specific notes on top of this file.

## Sources of truth

- **Product and architecture:** [`docs/REELANVIL_BUILD_STANDARDS.md`](docs/REELANVIL_BUILD_STANDARDS.md). Decisions recorded there win over assumptions. If a change would contradict one, stop and ask.
- **Current state:** [`docs/STATUS.md`](docs/STATUS.md): what exists, what's next, open blockers.
- **What changed:** [`docs/CHANGELOG.md`](docs/CHANGELOG.md) for user-visible changes; [`docs/BUILD_HISTORY.md`](docs/BUILD_HISTORY.md) for what was built, tested and pushed.
- **Code:** the local checkout at `/home/katala/Projects/ReelAnvil` is the working copy; GitHub (`gkuba/ReelAnvil`) is the shared mirror. Don't claim something was built, tested, committed or pushed unless you actually did it and saw it succeed.

## Branches and releases

- Day-to-day work happens on **`dev`** (or a short-lived feature branch merged into `dev`).
- A release **squash-merges `dev` into `main`**, then a fresh `dev` is started from the new `main`.
- Commit and push only when the maintainer asks. Never push to `main`, tag a release, or rewrite a shared branch without explicit approval for that specific action.
- The ChatGPT-era history (Builds 0.1–0.9.1) is preserved at tag `archive/chatgpt-build-0.9.1`.

## Build, test, and run

From the repository root:

```sh
gofmt -l .                 # must print nothing
go vet ./...
go test -race ./...
go mod verify
./scripts/build-dev.sh     # builds bin/Release/reelanvil with a version stamp
```

The maintainer tests by running the binary from `bin/Release`:

```sh
cd bin/Release && ./reelanvil serve      # http://127.0.0.1:8080, Ctrl+C to stop
```

- **`bin/Release/settings/`, `data/` and `logs/` are the maintainer's real data.** The build script replaces only the executable. Never write test data there. For browser or migration testing, copy the runtime to a scratch directory and use `./reelanvil serve --data-dir <copy> --addr 127.0.0.1:18089`.
- Before a change that migrates data, keep a copy of `bin/Release` until the maintainer confirms the new build works.
- CI (`.github/workflows/ci.yml`) runs the same checks plus govulncheck and staticcheck on pushes to `dev`/`main` and on PRs.

## Before calling a change done

1. Formatting, vet, race tests and the build pass.
2. New behaviour has tests; bug fixes have a regression test.
3. UI changes were checked in a browser at desktop and phone widths (no horizontal page scroll), against a scratch data directory.
4. `docs/CHANGELOG.md` (user-visible) and `docs/BUILD_HISTORY.md` (build/test/push record) are updated; `docs/STATUS.md` if the current state or next step changed.
5. The summary to the maintainer says plainly what was verified and what wasn't.

## Engineering boundaries

- One Go module. Separate processes: `reelanvil serve`, later `reelanvil worker`; `reelanvil doctor` for diagnostics. The web server never gets Docker socket access; only the authenticated worker talks to Docker.
- Debian 13 / Linux first. Windows and macOS are untested and must not be claimed as supported.
- SQLite for persistent state, with ordered, transactional migrations in `internal/store/store.go`. Refuse unknown future schema versions. Never silently delete user data.
- **The server is unauthenticated and must stay loopback-only.** Keep the Host check, same-origin POST checks, body cap, timeouts and security headers. Keep the CSP free of `unsafe-inline`: no inline `<script>`, `<style>` or `style=""` in templates (a test enforces this).
- The web UI lives in `internal/app/web/` (embedded templates, `app.css`, `app.js`). Built-in theme palettes are Go data in `internal/app/themes.go`; don't hard-code colours in CSS — use the theme tokens.
- Upstream Docker images only, pinned after validation; no custom images, bundled third-party binaries or model weights.
- Media ingestion must be authorized, originals preserved, jobs isolated by ID, and a human approves anything before it is posted. AI clip selection and independent AI review run in separate contexts.
- Real stage progress, elapsed time and logs; **never show an ETA** or invented percentages. Storage caps block jobs safely instead of deleting data.
- No required paid runtime services, no guaranteed-earnings claims, and no credentials, campaign data or media in source, logs, examples or release archives. Campaign discovery stays disabled until a platform's public API and permitted use are verified.
- No remote deployment or installation on another host without the maintainer's explicit approval.

## Documentation and packaging

- Root documentation: `README.md`, `AGENTS.md`, `CLAUDE.md`. Everything else goes in `docs/`.
- Licensing files live in `docs/licensing/` and ship with every release. Other `docs/` content is not bundled into runtime release artifacts.
- Release packaging uses an explicit allowlist with automated checks: license files present; status/history docs, secrets, logs, databases and media absent. The license audit and copyright holder are release gates.
- Maintain `STATUS.md`, `CHANGELOG.md` and `BUILD_HISTORY.md` in place; don't create per-build copies.
