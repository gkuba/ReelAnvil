@AGENTS.md

## Notes for Claude Code

- **Who decides:** the maintainer (gkuba) reviews by running the build from `bin/Release`. Their visual review is the acceptance step; automated tests don't replace it.
- **Terminal etiquette:** when a sudo or interactive command is needed, ask the maintainer to run it in their single terminal tab instead of running it yourself or opening new tabs.
- **Git:** don't add Claude as a co-author or contributor (no `Co-Authored-By` trailer) unless the maintainer asks. Commit or push only when asked.
- **Browser checks:** run a scratch copy of the runtime on `127.0.0.1:18089` with `--data-dir`; the maintainer's own server normally uses `:8080`. Check desktop width and the 375px mobile preset.
- **Feel of the project:** a finished-feeling, Sonarr-style self-hosted app with a Nordic-industrial dark UI (see `docs/design/`). Keep pages visually consistent — reuse the shared layout, `.card`, `.stats`, `.note`, `.btn` patterns and theme tokens rather than adding page-specific styling.
- **Where things are:**
  - `cmd/reelanvil/` — CLI (`serve`, `doctor`, `version`; flags `--data-dir`, `--addr`)
  - `internal/app/` — HTTP handlers, settings, themes, rendering (`render.go`), web assets in `web/`
  - `internal/store/` — SQLite store, migrations, campaign records
  - `scripts/build-dev.sh` — the only supported way to produce `bin/Release/reelanvil`
- **Next big area:** the media pipeline (import → transcript → clip candidates → FFmpeg render → review) and the worker process. The maintainer is still preparing the Debian 13 host, so container integration can't be tested yet; design the interfaces and test with local stand-ins until it's ready.
