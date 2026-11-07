# ReelAnvil — Changelog

User-visible changes. Build and test records live in [`BUILD_HISTORY.md`](BUILD_HISTORY.md). No version has been released yet.

## Unreleased

### Build 0.9.2 (development)
- The web UI is served from embedded templates, one stylesheet and one script. Settings pages now match the Campaigns pages' headings, forms, cards and sidebar.
- Stricter Content Security Policy: no inline scripts or styles.
- Saving settings and Test connection now work from other machines on the LAN. The server sends `Referrer-Policy: same-origin` instead of `no-referrer`, which made browsers send `Origin: null` and the server reject the form.
- Status messages after saving are one-time messages and can no longer be set through a link.
- Duplicating a theme keeps its real colours and a valid name (for example "Dracula (Copy)").
- Manual campaigns store View, Apply/Join and Submit links correctly; existing affected records are repaired automatically. Deadlines are stored as calendar dates.
- Clear messages when a campaign URL is already used, a campaign no longer exists, or a theme edit is invalid.
- `reelanvil serve --data-dir <dir> --addr <host:port>`, `reelanvil version`, and a more useful `reelanvil doctor`.
- ReelAnvil now listens on `0.0.0.0:4747` by default so it can be reached from your local network. **Settings → General → Network** sets the bind address, port and extra allowed host names; a **Restart ReelAnvil** button (and a banner on every page while a change is pending) applies them and reconnects the browser. `--addr` and `--allowed-hosts` override the saved values for one run. A startup warning and the DNS-rebinding defense remain. There is still no authentication: trusted LANs only, never the public internet.
- New `reelanvil install [--service]`, `upgrade [--dry-run]`, `doctor --repair [--clean]` and `uninstall` for Debian 13. Install creates a `reelanvil` system user and `/opt/ReelAnvil`, sets up rootless Podman with the Ollama, Whisper and media containers on loopback only, and (with `--service`) systemd units that serve the web app on port 4747. `uninstall` removes what install set up, Podman included; your settings and media are deleted only with `--delete-data`. The default port is now 4747. `doctor` also reports the media container's yt-dlp version against the latest release.

- New **Settings → AI**, **Transcription** and **Media** pages. They hold the Ollama address (default `127.0.0.1:11434`), the Whisper address (default `127.0.0.1:10300`) and the media engine (Podman, Docker or host programs, with FFmpeg and yt-dlp paths for host programs). Each has a **Test connection** button that tries the values as typed without saving them: Ollama shows its version and installed models, Whisper shows the speech recognition models it reports, and host programs are looked up (not run). Podman and Docker have nothing to test from the web app, because the worker owns the containers. Settings files from earlier builds gain these fields with the defaults.
- `install` and `upgrade` fill these fields in `server/settings/config.json` (existing values are kept). `doctor` now checks that the saved Ollama and Whisper addresses accept connections.
- **Settings → Paths & Storage** is now real: media root (default `/opt/ReelAnvil/media` on an installed host, a `media` folder beside the data directory for a plain `serve`) and a storage limit in GB (0 = no limit). The `incoming/`, `temp/`, `completed/`, `failed/` and `archive/` folders under the root are listed, and saving creates missing ones; nothing is ever deleted or moved. **Check** tests the path as typed (exists, writable, free space, space used against the limit) without saving it.
- **Settings → AI** lists the models installed in Ollama and lets you choose a hunter and a reviewer model. **Pull model** downloads a model through Ollama in the background (one at a time, stoppable) and shows Ollama's real status, bytes downloaded and elapsed time, never an ETA. **Test hunter model** asks the model for a short reply and shows the answer and how long it took.
- **Settings → Transcription** adds the Whisper model (default `tiny-int8`) and language (default `en`) and shows the models the service reports. They take effect on the container with `sudo reelanvil upgrade`; the web server never controls containers.
- **Settings → Connectors** no longer shows fake inputs: manual campaigns are available, platform discovery stays disabled until a platform's public API and permitted use are verified. No credentials are asked for.
- **Services** is a real status page: Web app, Ollama, Whisper, Media engine and Storage, each with a status, plus a ready / needs action summary with the exact command to run (for example `sudo reelanvil doctor --repair`) and a **Check again** link. Container engines are reported as checked by `reelanvil doctor`.
- Settings file version 5 (older files gain the new fields with defaults). The installer renders `compose.yaml` from these settings: the Whisper model and language, and the media mount at the saved media root. `install` and `upgrade` create the media folders for the `reelanvil` user (existing folders are never re-owned) and `doctor` checks that the media root exists and is writable by that user.

### Builds 0.1–0.9.1 (development)
- Local web interface with Overview, Campaigns, Settings (General, Themes, Connectors, Paths & Storage), Diagnostics and placeholder pages.
- Five built-in themes (Aurora, Dracula, Tomorrow Night, Nocturne, Foundry), custom themes with backup recovery, and a global application icon.
- Persistent compact navigation rail with flyouts; responsive layout.
- SQLite storage; manual campaign tracking with statuses, notes, filters and an Overview summary. Campaign discovery is unavailable by design.
- Loopback-only server with local request guards.
