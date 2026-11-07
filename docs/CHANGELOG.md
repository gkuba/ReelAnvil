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
- **Services** is a real status page: Web app, Ollama, Whisper and Storage, plus Media engine when it is set to host programs, each with a status, plus a ready / needs action summary with the exact command to run (for example `sudo reelanvil doctor --repair`) and a **Check again** link. Podman and Docker get no card, since the web app cannot check containers; `reelanvil doctor` does.
- Settings file version 5 (older files gain the new fields with defaults). The installer renders `compose.yaml` from these settings: the Whisper model and language, and the media mount at the saved media root. `install` and `upgrade` create the media folders for the `reelanvil` user (existing folders are never re-owned) and `doctor` checks that the media root exists and is writable by that user.
- **Media**, **Jobs** and **Review** are real pages. **Media** starts a clip job from a file in the media root's `incoming/` folder or an HTTPS link (you confirm you have the rights; the original is kept in `archive/`), optionally for a tracked campaign. **Jobs** shows each job's stages (import, transcribe, hunt, review, render, QA) with real counts, elapsed time and the log, never an ETA, plus Cancel and Retry. Clip candidates are chosen by the hunter model and checked separately by the reviewer model; rendered 9:16 clips with captions go through an automatic QA check. **Review** plays each clip, offers the video and `.srt` downloads, and lets you approve or reject; nothing is posted by ReelAnvil.
- The hunter reads a long video a few minutes at a time, so clips can come from anywhere in it, not only the first minutes. The hunter and the reviewer are told that greetings, sound checks, sign-offs and housekeeping talk are not clips.
- Clips no longer stop while someone is still talking: the end moves forward to the end of the sentence, past the job's longest length if needed (up to 2 minutes, or the job's longest length if that is more). Clips are never cut shorter for this. The hunter and reviewer are told a clip must run until the speaker has finished.
- Approved clips from a campaign job can have a post recorded (link, views, earnings) and updated later. **Campaigns** shows each tracked campaign's jobs, clips, approved clips, posts, views and earnings, with a **New clip job** link.
- New `reelanvil worker` process: it runs the media steps (FFmpeg, yt-dlp, in the media container or as host programs) for the web app over a token-protected loopback API. The installed worker service now runs it after starting the containers.
- Database schema version 4 (jobs, steps, logs, candidates, clips, posts).

### Builds 0.1–0.9.1 (development)
- Local web interface with Overview, Campaigns, Settings (General, Themes, Connectors, Paths & Storage), Diagnostics and placeholder pages.
- Five built-in themes (Aurora, Dracula, Tomorrow Night, Nocturne, Foundry), custom themes with backup recovery, and a global application icon.
- Persistent compact navigation rail with flyouts; responsive layout.
- SQLite storage; manual campaign tracking with statuses, notes, filters and an Overview summary. Campaign discovery is unavailable by design.
- Loopback-only server with local request guards.
