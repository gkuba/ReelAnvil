# ReelAnvil

**ReelAnvil** is a planned open-source, self-hosted application for turning authorized long-form video into reviewable short-form clips.

> **Project status:** Build 0.9.2 is a local development prototype, not a supported release. Campaign records can be added and tracked manually; the media, AI, worker and platform integrations are still planned.

## The idea

ReelAnvil aims to bring the ingestion, analysis, editing, and review of short-form video clips into one browser-managed workflow. The goal is to make repetitive media processing easier while keeping people in control of what gets published.

The planned workflow is:

1. Import video from authorized sources.
2. Transcribe and analyze the material.
3. Identify promising clip candidates using AI.
4. Independently review candidate selections.
5. Render clips with FFmpeg and perform technical quality checks.
6. Let a human review and approve the final clips before publishing.

## Planned features

- A self-hosted web interface for setup, jobs, queues, reviews, storage, and diagnostics
- Local-first processing with configurable AI and transcription services
- Real processing status, logs, and elapsed time rather than speculative ETAs
- Storage controls, job isolation, backups, and recovery workflows
- A responsive, dark-themed interface inspired by Nordic-industrial design
- Optional workflow organization and performance tracking without guaranteed-earnings claims

## Current prototype

The current development build provides the themed local web interface, persistent SQLite setup, campaign records and tracking, and a global icon choice independent of the active interface theme. Content Rewards and Vyro discovery are explicitly unavailable: ReelAnvil does not scrape sites, log in, apply to campaigns, find social posts, or submit content. Jobs, media processing, AI, and worker pages remain placeholders.

The server is unauthenticated and **must remain bound to loopback**. Do not expose it through a LAN, reverse proxy, tunnel, or public address.

## Intended technical direction

ReelAnvil is being designed primarily for **Debian 13 Linux**, with a native **Go** application, separate web-server and media-worker processes, and **SQLite** for persistent state. Planned media and AI integrations include FFmpeg, yt-dlp, faster-whisper, and Ollama. Where containers are needed, the intention is to use upstream images rather than distribute custom images or bundled models.

These are design targets and have **not** yet been validated as a completed deployment.

## Getting started

There is no supported installation or release yet. To build and run the local development prototype (Go 1.23 or newer):

```sh
./scripts/build-dev.sh
cd bin/Release
./reelanvil serve
```

Open <http://127.0.0.1:8080> and stop the server with **Ctrl+C**. See [development and test instructions](docs/DEVELOPMENT.md) for the Go checks, local database behavior, and review steps.

For current plans and decisions, see:

- [Build standards](docs/REELANVIL_BUILD_STANDARDS.md)
- [Project status](docs/STATUS.md)
- [Change history](docs/CHANGELOG.md)
- [Contributor and agent guide](AGENTS.md)
- [Licensing information](docs/licensing/LICENSING.md)

## Licensing

The project is targeting the **MIT License**. Final copyright attribution, third-party notices, and dependency audits must be completed before a public release. See [licensing documents](docs/licensing/).

## Project name

ReelAnvil is a working project name. Branding and trademark checks are part of the pre-release process.
