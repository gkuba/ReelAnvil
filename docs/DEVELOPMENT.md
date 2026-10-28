# ReelAnvil development and local prototype run guide

## Build and source checks

Run Go commands from the repository root, where `go.mod` is located:

```sh
go test ./...
go vet ./...
./scripts/build-dev.sh
```

The development script atomically replaces only `bin/Release/reelanvil` and preserves runtime `settings/`, `data/`, `logs/`, and `temp/` directories. The SQLite database lives at `bin/Release/data/reelanvil.db` when started from that directory; current schema version is 2. On startup the app transactionally upgrades a version 1 database and preserves `storage_probe`; future versions are refused. Never copy a live database file as a backup because WAL may contain committed changes. Stop the server before a simple file backup; an online backup workflow is not implemented yet. Do not run `go run ./reelanvil`, `go test ./...`, or `go vet ./...` from `bin/Release`; those commands operate on Go source packages, not the compiled executable. The `dist/` directory is reserved for explicitly approved release builds.

## Run and stop the local prototype

Start the server in a terminal:

```sh
cd /home/katala/Projects/ReelAnvil/bin/Release
./reelanvil serve
```

The server listens on `127.0.0.1:8080` by default. Open <http://127.0.0.1:8080> in a browser. To choose another local port:

```sh
REELANVIL_ADDR=127.0.0.1:18080 ./reelanvil serve
```

Keep the terminal open while using the prototype. Stop it with **Ctrl+C**. This build has no background service or systemd unit. It enforces loopback-only listening and rejects non-local Host headers. Do not expose it publicly: it has no authentication, and the local request checks are defense-in-depth rather than a replacement for authentication.

## Maintainer checks

- Visit Overview, Jobs, Media Library, Review, Campaigns, Services, Diagnostics, and Settings; check that each route renders and navigation works.
- In Settings, change the instance name and theme, save, and confirm the values display. Restart with Ctrl+C and the start command above; confirm the values persist.
- Confirm the overview labels itself as a prototype and does not imply real imports, jobs, reviews, services, or processing.
- Confirm `GET /healthz` returns JSON `{"status":"ok"}`.

### Build 0.3 theme and layout review

- In Settings, cycle through Aurora, Dracula Dark, Tomorrow Night, Tokyo Night, Nocturne, and Foundry. Check the full page palette, sidebar/navigation, cards, controls, buttons, and all seven live preview swatches.
- Check that Dracula Dark uses a neutral near-black charcoal background and that Aurora balances blue, teal, and lavender accents.
- On Overview and Diagnostics, check that metric cards use distinguishable blue, teal, and purple semantic surfaces with readable text contrast.
- Save a theme, refresh, and restart the server to confirm persistence.
- This build remains a visual prototype. Media Library, Review, Campaigns, Services, and Jobs are placeholders; no media is processed and no production services are contacted.

### Build 0.4 Theme Manager review

- Confirm built-in themes remain selectable and read-only. Duplicate one, rename it, and edit all seven `#RRGGBB` values.
- Check that editing updates the seven draft swatches without saving. Cancel should restore the previously saved values; Save Changes should persist the name and colors.
- Select the custom theme in Interface theme, save, refresh the page, restart the server, and confirm the selection and custom values persist.
- If checking manual configuration refresh, back up `settings/themes.json` and `.bak`, make a valid JSON edit, then use **Refresh Themes**. Verify the custom-theme list and editor display the refreshed values.
- Do not run recovery/failure-injection cases during ordinary development. They are tracked in [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md) and must be completed before release. Invalid JSON can leave saving locked until repaired and refreshed.
- Theme files are user data under `bin/Release/settings/`. Preserve both `themes.json` and `themes.json.bak` across builds. The prototype rejects off-origin browser POSTs, caps form bodies at 1 MiB, and applies local-only Host checks and basic security headers. It still has no authentication; network access is disabled until an authenticated deployment design is implemented.

### Build 0.7 theme and local security review

- Confirm there is only one built-in Dracula option, with canonical ID `dracula-dark`; legacy `dracula-lavender` saved selections and custom theme bases migrate to it without changing custom colors.
- Check preview order (Background, Sidebar, Panels / Tiles, Primary, Secondary, Tertiary, Quaternary) separately from the seven editable persisted fields.
- Verify theme custom colors, active navigation contrast, and normal/visited/hover/focus/selected flyout links.
- Security guard tests confirm network bind addresses are rejected, non-local Host headers and cross-origin browser POSTs are blocked, POST request bodies are capped, and common headers are present. This remains unauthenticated local software; do not bypass the loopback bind restriction or expose it to another host.

### Build 0.9 campaign and branding review

- Visit Overview and Campaigns at desktop and narrow widths. Check the mountain header, Available / Tracked / Ending Soon counts, empty states, manual campaign entry, filters, sorting, and tracked table.
- Add a disposable campaign with a valid HTTPS URL, optional amounts/deadline, requirements, and notes. Confirm it starts as Interested; update status and details; untrack and confirm the record returns to Available with its notes/status retained; track it again and confirm prior metadata remains.
- Confirm unverified campaign discovery is clearly unavailable for Content Rewards and Vyro. No platform is contacted. Check user text is escaped, unsafe URL schemes are rejected, and View/Apply/Submit links only appear for supplied HTTPS targets.
- In Settings → Themes, set a different icon for a built-in theme and a custom theme, apply each theme, and confirm the sidebar icon and browser favicon follow the active theme across refresh and restart.
- Confirm an existing schema-v1 database starts successfully as schema v2 with its `storage_probe` retained. Test future schema rejection using a disposable database only.
- This candidate remains unauthenticated and loopback-only. No OAuth, discovery API, applications, post search, submission, jobs, media processing, or remote deployment is included.

### Build 0.5 Settings and theme review

- Visit General, Themes, Connectors, and Paths & Storage. Confirm the active settings link follows the current page, navigation uses the inline SVG icons, and all four routes render.
- General should save the instance name without changing the selected theme. Themes should show Aurora, Dracula Classic, Dracula Lavender, Tomorrow Night, Nocturne, and Foundry; Tokyo Night is retired. Existing saved Tokyo Night selections load and migrate to Aurora, and custom themes with a Tokyo Night base inherit Aurora in memory.
- Cycle all six built-ins and check the page palette and preview. Save a theme only if intentionally testing the setting; refresh themes and verify it does not modify instance/theme config. Existing custom themes should remain visible and editable.
- Connector and storage inputs are disabled explanatory placeholders and must not submit or persist values.
- Check the settings pages at desktop width and a narrow viewport. At 390 px, navigation should wrap, theme fields should use one column, and the page should not overflow horizontally; at desktop width, the custom color fields should use two columns.
- Do not run theme recovery/failure-injection cases during ordinary development. They remain tracked in [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md).

Only Overview, Settings, Diagnostics, and the route shell exist in this prototype; most other sections are visual/navigation placeholders. It does not process media, connect external services, create jobs, or publish clips. Runtime settings and logs live under `bin/Release/settings/` and `bin/Release/logs/`. Removing those generated directories resets prototype settings and logs.
