# ReelAnvil

**ReelAnvil** is a self-hosted web app that turns long videos you have the right to use into short vertical clips. You give it a video; local AI finds the strong moments, a separate AI review checks them, FFmpeg renders them, and you approve what is worth posting. Nothing is published for you.

> **Trusted network only.** ReelAnvil has no login. By default it listens on `0.0.0.0:4747` so other machines on your home or lab network can reach it. Anyone who can reach that port can use and change it. Never port-forward it, publish it through a tunnel or put it behind a public reverse proxy. Keep the port limited to your LAN in the host firewall (for example `ufw allow from 192.168.1.0/24 to any port 4747`).

## What it does

1. **Import** a video from the `incoming/` folder or an HTTPS link (you confirm you have the rights).
2. **Transcribe** it with faster-whisper.
3. **Hunt** for clips with a local Ollama model, then check where each answer really ends.
4. **Review** each pick with a second model that scores hook, completeness, standalone sense, interest and fit, and never sees the hunter's reasoning.
5. **Render** 9:16 clips with FFmpeg: blurred-background fill, long pauses and filler-only lines cut, three-word captions burned in, the title over the first seconds, and a `.srt` and a `.json` manifest beside each clip. Each part can be switched off in Settings → Media.
6. **Approve** on the Review page: play the clip in the browser, download it, approve or reject it. Notes can become lessons for later jobs, or trigger a retry.

Progress is real: stages, counts, elapsed time and logs. There are no time estimates. Storage limits block a job instead of deleting anything. Campaign tracking is unfinished and hidden until you switch on **Settings → General → Developer features**.

## Requirements

- **Debian 13** (trixie), 64-bit, with `sudo`. Other systems are untested and not supported.
- 16 GB RAM or more is comfortable for the 7B models; a GPU is optional (Ollama uses CPU otherwise, which is slower).
- Disk space for models, source videos and clips; set a limit under **Settings → Paths & Storage**.
- Internet access to pull the container images and models.

## Install on Debian 13

Download the release archive and its checksum from the project's GitHub releases page, then check and unpack it:

```sh
sha256sum -c reelanvil-VERSION-linux-amd64.tar.gz.sha256
```

```sh
tar -xzf reelanvil-VERSION-linux-amd64.tar.gz
```

```sh
cd reelanvil-VERSION-linux-amd64
```

Install. This installs Debian's `podman` and `podman-compose`, creates one `reelanvil` system user, writes `/opt/ReelAnvil` and starts two systemd services (web app and worker). It asks before each change; the first start pulls the container images, which can take several minutes.

```sh
sudo ./reelanvil install --service
```

Open `http://<this machine>:4747`, then:

1. **Settings → AI**: pull a model (the page recommends one for your RAM and GPU), then choose a hunter and a reviewer model.
2. **Settings → Transcription**: choose the Whisper model and language.
3. **Services** and **Diagnostics** should read "Ready".
4. **Media**: start a job.

Check the install at any time:

```sh
sudo ./reelanvil doctor
```

## Upgrade, repair, uninstall

Unpack the newer release and run its binary:

```sh
sudo ./reelanvil upgrade
```

`upgrade --dry-run` shows the plan and changes nothing. Settings, the database and media are never touched.

Fix a broken install (folders, units, services) without touching your data:

```sh
sudo ./reelanvil doctor --repair
```

Remove the services, the service user and the Podman packages the installer added, but keep your data:

```sh
sudo ./reelanvil uninstall
```

Also delete everything in `/opt/ReelAnvil` (settings, database, media, models); `--yes` never does this by itself:

```sh
sudo ./reelanvil uninstall --delete-data
```

## Back up and roll back

Everything lives in `/opt/ReelAnvil`: `server/settings/` (settings and themes), `server/data/` (the SQLite database) and the media folders. To back up, stop the services, copy the folder, start them again:

```sh
sudo systemctl stop reelanvil reelanvil-worker
```

```sh
sudo cp -a /opt/ReelAnvil /opt/ReelAnvil.backup
```

```sh
sudo systemctl start reelanvil-worker reelanvil
```

Each upgrade keeps the old binary as `/opt/ReelAnvil/server/reelanvil.previous`. To roll back, stop both services, copy it over `/opt/ReelAnvil/server/reelanvil`, and start them. If the new version migrated the database, also restore your backup folder: ReelAnvil refuses to open a database from a newer version than it knows.

## Container images

The installer's Compose file pulls images straight from their upstream registries; ReelAnvil does not bundle or repackage them. Ollama and Whisper are pinned by digest to the versions the release was tested with. The media image (FFmpeg and yt-dlp) tracks `:latest` so yt-dlp keeps up with site changes. To move a pin, see the comment at the top of `internal/install/assets/compose.yaml.tmpl`.

## Build from source

Go 1.23 or newer:

```sh
./scripts/build-dev.sh
```

```sh
cd bin/Release
```

```sh
./reelanvil serve
```

Open <http://127.0.0.1:4747> and stop with **Ctrl+C**. See [the development guide](docs/DEVELOPMENT.md) for checks, scratch data directories and review steps, and [AGENTS.md](AGENTS.md) for the project rules.

## Documents

- [Build standards](docs/REELANVIL_BUILD_STANDARDS.md), [project status](docs/STATUS.md), [change history](docs/CHANGELOG.md)
- [Security requirements](docs/SECURITY_REQUIREMENTS.md)
- [Licensing](docs/licensing/LICENSING.md) and [third-party notices](docs/licensing/THIRD_PARTY_NOTICES.md)

## License

MIT. See [docs/licensing/LICENSE](docs/licensing/LICENSE). You are responsible for having the rights to every video you import and for what you post.
