# ReelAnvil — Development Guide

## Build and check

Run from the repository root (where `go.mod` is):

```sh
gofmt -l .
go vet ./...
go test -race ./...
go mod verify
./scripts/build-dev.sh
```

`build-dev.sh` atomically replaces only `bin/Release/reelanvil` and stamps it with a version such as `0.9.2-dev+936d6e6-dirty` (base version, commit, and whether the tree had uncommitted changes). It leaves `bin/Release/settings/`, `data/` and `logs/` untouched. Don't run Go commands from inside `bin/Release`; they work on source packages, not the binary.

CI runs the same checks plus `govulncheck` and `staticcheck` (`.github/workflows/ci.yml`).

## Run and stop

```sh
cd bin/Release
./reelanvil serve
```

Open <http://127.0.0.1:8080>; stop with **Ctrl+C**. Other options:

```sh
./reelanvil serve --addr 127.0.0.1:18080        # or REELANVIL_ADDR=...
./reelanvil serve --data-dir /path/to/test-data  # or REELANVIL_DATA_DIR=...
./reelanvil doctor                               # read-only checks of the data directory
./reelanvil version
```

The server refuses any non-loopback address. It has no authentication; don't expose it through a LAN, reverse proxy or tunnel.

## Data directory

Everything runtime lives under the data directory (the current directory by default):

| Path | Contents |
|---|---|
| `settings/config.json` | Instance name, active theme, icon (settings version 3; older versions migrate on start) |
| `settings/themes.json`, `themes.json.bak` | Custom themes and the last-known-good backup |
| `data/reelanvil.db` | SQLite database, WAL mode, schema version 3 |
| `logs/reelanvil.log` | Server log |

Database migrations run on startup inside one transaction. A failure leaves the database unchanged, and a database from a newer build is refused. Because the database uses WAL, stop the server before copying it as a backup. An older build can't open a newer schema, so rolling back means restoring the backed-up `data/` folder along with the binary.

**Testing against a scratch copy:** never use `bin/Release` for test writes. Copy it and point the server at the copy:

```sh
cp -a bin/Release /tmp/reelanvil-test
bin/Release/reelanvil serve --data-dir /tmp/reelanvil-test --addr 127.0.0.1:18089
```

## Code layout

| Path | Purpose |
|---|---|
| `cmd/reelanvil/` | CLI entry point and flags |
| `internal/app/app.go` | Settings, server startup, request guards, routes |
| `internal/app/render.go` | Template sets, page data, flash messages, static assets |
| `internal/app/themes.go` | Built-in palettes, custom theme storage and recovery, `/assets/themes.css` |
| `internal/app/campaigns.go` | Campaign and Overview pages and form handlers |
| `internal/app/web/templates/` | `layout.html`, `sidebar.html` and one file per page (each defines `content`) |
| `internal/app/web/static/` | `app.css` and `app.js`, served at `/assets/` |
| `internal/store/` | SQLite open, migrations, campaign records |

To add a page: create `web/templates/<name>.html` defining `content`, add the name to `pages` in `render.go`, and route to it with `s.render(w, "<name>", data)`, where `data` embeds `pageBase`. Use the shared classes and theme tokens; inline `<script>`, `<style>` and `style=""` are blocked by the CSP and by a test.

To change the schema: append a function to `migrations` in `internal/store/store.go` and add a test that migrates a database built at the previous version.

## Maintainer review checklist

Run against a scratch copy unless you mean to change your real data.

- **Navigation:** Overview, Campaigns, Jobs, Media (Library, Review), Diagnostics, Settings (General, Themes, Connectors, Paths & Storage, Services). Each renders, the active item is highlighted, groups expand, and the compact rail shows flyouts. At phone width the navigation wraps and the page doesn't scroll sideways.
- **Settings:** saving the instance name shows a confirmation and survives a restart.
- **Themes:** changing the theme previews instantly; saving persists it. Duplicate a built-in theme, edit its colours, check that Cancel restores the saved values and Save persists them. The icon choice stays the same across theme changes and matches the favicon.
- **Campaigns:** add a campaign with an HTTPS URL and optional View/Apply/Submit links, amounts, deadline and notes; each link opens the right target. Change status and notes, untrack and re-track (notes and status kept), and try saving with another campaign's URL (readable error). Non-HTTPS URLs are rejected.
- **Overview:** counts and the tracked list match Campaigns.
- **Diagnostics:** shows uptime, version, data directory and listen address.
- `GET /healthz` returns `{"status":"ok"}`.

Theme recovery and failure-injection cases are pre-release checks: [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md).
