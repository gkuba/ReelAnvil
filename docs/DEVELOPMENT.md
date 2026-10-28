# ReelAnvil development and local prototype run guide

## Build and source checks

Run Go commands from the repository root, where `go.mod` is located:

```sh
go test ./...
go vet ./...
./scripts/build-dev.sh
```

The development script recreates only `bin/Release/` and writes the compiled executable to `bin/Release/reelanvil`. Do not run `go run ./reelanvil`, `go test ./...`, or `go vet ./...` from `bin/Release`; those commands operate on Go source packages, not the compiled executable. The `dist/` directory is reserved for explicitly approved release builds.

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

Keep the terminal open while using the prototype. Stop it with **Ctrl+C**. This build has no background service or systemd unit. It binds to loopback by default; do not expose it publicly.

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
- Theme files are user data under `bin/Release/settings/`. Preserve both `themes.json` and `themes.json.bak` across builds. Do not expose the current prototype beyond loopback; it has no authentication or CSRF protection.

### Build 0.5 Settings and theme review

- Visit General, Themes, Connectors, and Paths & Storage. Confirm the active settings link follows the current page, navigation uses the inline SVG icons, and all four routes render.
- General should save the instance name without changing the selected theme. Themes should show Aurora, Dracula Classic, Dracula Lavender, Tomorrow Night, Nocturne, and Foundry; Tokyo Night is retired. Existing saved Tokyo Night selections load and migrate to Aurora, and custom themes with a Tokyo Night base inherit Aurora in memory.
- Cycle all six built-ins and check the page palette and preview. Save a theme only if intentionally testing the setting; refresh themes and verify it does not modify instance/theme config. Existing custom themes should remain visible and editable.
- Connector and storage inputs are disabled explanatory placeholders and must not submit or persist values.
- Check the settings pages at desktop width and a narrow viewport. At 390 px, navigation should wrap, theme fields should use one column, and the page should not overflow horizontally; at desktop width, the custom color fields should use two columns.
- Do not run theme recovery/failure-injection cases during ordinary development. They remain tracked in [`PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md`](PRE_RELEASE_THEME_RECOVERY_CHECKLIST.md).

Only Overview, Settings, Diagnostics, and the route shell exist in this prototype; most other sections are visual/navigation placeholders. It does not process media, connect external services, create jobs, or publish clips. Runtime settings and logs live under `bin/Release/settings/` and `bin/Release/logs/`. Removing those generated directories resets prototype settings and logs.
