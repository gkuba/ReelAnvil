# ReelAnvil development and Build 0.1 run guide

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

## Build 0.1 maintainer checks

- Visit Overview, Jobs, Media Library, Review, Campaigns, Services, Diagnostics, and Settings; check that each route renders and navigation works.
- In Settings, change the instance name and theme, save, and confirm the values display. Restart with Ctrl+C and the start command above; confirm the values persist.
- Confirm the overview labels itself as a prototype and does not imply real imports, jobs, reviews, services, or processing.
- Confirm `GET /healthz` returns JSON `{"status":"ok"}`.

Only Overview, Settings, Diagnostics, and the route shell exist in this prototype; most other sections are visual/navigation placeholders. It does not process media, connect external services, create jobs, or publish clips. Runtime settings and logs live under `bin/Release/settings/` and `bin/Release/logs/`. Removing those generated directories resets prototype settings and logs.
