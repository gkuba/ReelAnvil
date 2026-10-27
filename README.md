# ReelAnvil

**ReelAnvil** is a planned open-source, self-hosted application for turning authorized long-form video into reviewable short-form clips.

> **Project status:** Early planning and repository initialization. ReelAnvil is not yet available as a working application. Features and installation instructions below describe the intended direction, not completed functionality.

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

## Intended technical direction

ReelAnvil is being designed primarily for **Debian 13 Linux**, with a native **Go** application, separate web-server and media-worker processes, and **SQLite** for persistent state. Planned media and AI integrations include FFmpeg, yt-dlp, faster-whisper, and Ollama. Where containers are needed, the intention is to use upstream images rather than distribute custom images or bundled models.

These are design targets and have **not** yet been validated as a completed deployment.

## Getting started

There is no supported installation or runnable release yet. This repository is being established for source development and project documentation.

For current plans and decisions, see:

- [Build standards](docs/REELANVIL_BUILD_STANDARDS.md)
- [Project status](docs/STATUS.md)
- [Change history](docs/CHANGELOG.md)
- [Licensing information](docs/licensing/LICENSING.md)

## Licensing

The project is targeting the **MIT License**. Final copyright attribution, third-party notices, and dependency audits must be completed before a public release. See [licensing documents](docs/licensing/).

## Project name

ReelAnvil is a working project name. Branding and trademark checks are part of the pre-release process.
