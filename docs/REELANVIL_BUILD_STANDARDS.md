# ReelAnvil — Build Standards, Architecture & Product Guidelines

**Document version:** 0.4 (development workflow moved to Claude Code; built-in theme list updated)  
**Captured:** 2026-10-27  
**Updated:** 2026-10-29  
**Status:** Baseline specification for repository initialization; not an implementation claim  
**Primary target:** Debian 13, Linux, headless/self-hosted  
**Product benchmark:** A polished, Sonarr/SickBeard-style self-hosted application

> **Purpose:** Preserve the design decisions, illustrative layouts, examples, implementation guardrails, and unresolved questions discussed before the first build. An initial single-user deployment does **not** lower the standard of finish, reliability, or usability. The repository is planned to be public.


## Visual design foundation (approved)

ReelAnvil uses the approved Nordic-industrial concept as its **foundational design direction**, not a late-stage cosmetic addition. See `ReelAnvil_Brand_UI_Concept_v1.png` and `ReelAnvil_Theme_Directions_v1_1.md` in the same project folder. Implement shared design tokens and components from the initial application foundation.

- **Aurora** is the intended default: near-black/midnight navy surfaces, purple and periwinkle/blue accents, soft restrained glows, and steel-blue boundaries.
- Built-in themes (Build 0.9.2): **Aurora**, **Dracula**, **Tomorrow Night**, **Nocturne** and **Foundry**, plus user-made custom themes. Saved selections of the earlier Dark, Slate, Tokyo Night and Pastel themes switch to these automatically.
- Nordic influence should be subtle and contemporary: craftsmanship, steel, anvil/reel geometry, not a medieval novelty UI.
- Apply consistent styling to all real screens: dashboard, expandable job detail, library, campaigns, first-run wizard, services, diagnostics, settings, errors, and storage controls.
- Concept artwork is a visual reference, **not** a pixel-perfect functional specification. Functional states, genuine progress, elapsed time without ETA, accessibility, and usability take precedence over illustrative mockup content.

## 1. Product principles and non-negotiables

1. **Finished-product experience.** ReelAnvil must feel like an installed self-hosted application, not scripts with a thin web front end. Navigation, configuration, onboarding, diagnostics, errors, progress, and recovery are first-class features.
2. **Zero incremental paid dependencies.** No mandatory paid AI APIs, subscriptions, proprietary licenses, or additional hardware. AI coding assistants are development aids only, never runtime dependencies.
3. **Linux-first, portable by design.** Officially build and validate on Linux initially (Debian 13 first). Generalize code for Windows and macOS; expose clearly labeled experimental OS-specific settings/help where useful, but **do not claim those platforms are supported** until tested.
4. **One Go codebase and release.** The web application and media worker are separate **processes**, not separate projects or separately maintained releases. Proposed commands: `reelanvil serve`, `reelanvil worker`, `reelanvil doctor` (command names subject to final CLI design).
5. **Established upstream media/AI containers.** Prefer maintained, widely used images for Ollama, LinuxServer.io faster-whisper, and a combined yt-dlp + FFmpeg/ffprobe environment. **Do not maintain a custom media container image.** Pin tested image versions/digests; validate platform architectures and capabilities before selection.
6. **No routine config-file editing.** Normal installation, service setup, paths, quotas, model selection, and diagnostics should be accessible from the web UI. Advanced config/automation remains possible.
7. **Observable, recoverable work.** Every job has a durable state, current stage, measurable progress where available, elapsed time, logs, and safe recovery controls.
8. **Human review before publishing.** AI discovers and reviews clips; the user approves outputs. Do not assume unofficial automated posting or rights to arbitrary source material.
9. **Transparent behavior.** Clearly distinguish detected, configured, connected, verified, unsupported, experimental, mocked, and unavailable states. No fabricated completion percentages, ETA, AI confidence, or service health.
10. **Safe storage.** Preserve originals, isolate working files, enforce configured storage budgets as closely as the execution environment allows, and never silently delete valuable media.

## 2. Scope and intended workflow

ReelAnvil is a local/self-hosted campaign-based video clipping pipeline for eligible, authorized source footage. The primary use case is participating in already funded clipping/content-reward campaigns, rather than client outreach or agency sales. Actual payout is uncertain; the application must not imply earnings are guaranteed.

**End-to-end pipeline:**

1. Create/select a campaign and record its source rights, brief, rules, deadlines, and eligibility.
2. Import an authorized video URL or upload/ingest a local media file.
3. Validate and preserve the source; register metadata and storage ownership.
4. Transcribe speech and timestamps.
5. **AI stage 1 — Clip Hunter:** identify/rank candidate highlights, boundaries, hook, payoff, context, campaign fit.
6. **AI stage 2 — Independent Editorial Reviewer:** evaluate candidates against source transcript/context and campaign rules **without seeing AI 1's scores or rationale**; accept, request bounded revision, or reject. Same model with independent prompts is permitted initially, with correlated-error caveat.
7. Render clips with FFmpeg, formatting/framing and styled subtitles.
8. Perform technical QA (duration, dimensions, audio, captions, black/silent segments, integrity, campaign constraints).
9. Present finished clips for human approval and export; manual posting initially.
10. Track submitted clips, approval/rejection reasons, qualified views and actual earnings when entered/available.

## 3. Architecture and deployment contract

### 3.1 Logical diagram

```text
Browser (first-run wizard, dashboard, settings, library, jobs, reviews)
        |
        v
+-----------------------------------------------------------+
| ReelAnvil Go binary: `reelanvil serve`                     |
| HTTP UI + API | SQLite | campaign/job engine | scheduler   |
| quota accounting | discovery | configuration | diagnostics |
+-----------------------------------------------------------+
        | authenticated, narrow job API
        v
+-----------------------------------------------------------+
| Same Go binary: `reelanvil worker` (SEPARATE PROCESS)      |
| validated job types | Docker Engine client | media paths   |
| progress reporting | cancellation | quota enforcement     |
+-----------------------------------------------------------+
        | Docker Engine API (restricted worker only)
        v
+--------------------------+    +---------------------------+
| Existing yt-dlp + FFmpeg |    | LinuxServer faster-whisper|
| image: CLI job container |    | Wyoming transcription     |
+--------------------------+    +---------------------------+

ReelAnvil `serve` also talks to configurable Ollama HTTP endpoint
(official Ollama container OR existing native/remote Ollama).
```

### 3.2 Container count and packaging

- **Three third-party service/image categories:** Ollama, faster-whisper, and combined yt-dlp/FFmpeg. Ollama may be external, reducing local containers.
- **Our Go code runs natively** for the first Linux build: web server and worker as separate processes of the same executable. SQLite is embedded in the web application; no standalone SQLite container.
- The media image may be **invoked as short-lived job containers** by the worker, not necessarily a single continuously running media service. Container *images/services* and simultaneous *running container instances* are different counts; don't promise a fixed runtime container count.
- Docker Compose should orchestrate supported third-party long-running services. Provide native Go systemd units on Linux and clear install/startup instructions. Later deployment options may evolve without custom media images.
- The worker owns Docker runtime access. **Never expose Docker socket/API directly to the browser-facing Go server.** The worker's Docker access is effectively high privilege and must be protected; narrow authenticated API, no arbitrary command execution, controlled mounts, local-only binding by default, Docker endpoint configuration, resource limits, and careful threat review.
- **No assumption** that a community CLI image exposes HTTP endpoints. The native Go worker controls its lifecycle and parses job output.
- Use configurable endpoints and explicit connection tests for Ollama, Wyoming faster-whisper, worker, and Docker.
- Prefer version-pinned dependencies and reproducible builds; do not automatically track untested `latest` images in production.

### 3.3 Cross-platform contract

| Area | Linux (supported first) | Windows (experimental) | macOS (experimental) |
|---|---|---|---|
| Go binary | Native | Future native `.exe` | Future native binary |
| Docker | Engine/context | Docker Desktop/compatible | Docker Desktop/compatible |
| Paths | Unix paths | Drive letters, separators, shares | Unix host paths, Docker Desktop shares |
| Docker connection | Socket/context | Named pipe/context | Desktop socket/context |
| Architecture | AMD64 initially | AMD64 initially | AMD64/ARM64 image verification |
| GPU | Optional, not required | Optional, not guaranteed | Optional, not guaranteed |

Provide separate **Windows** and **macOS** settings/help tabs for special configuration and compatibility checks; label them **Experimental / Unsupported**. No hard-coded `/var/run/docker.sock`, Linux-only shell assumptions in shared logic, or unportable host-to-container path concatenation. Baseline media processing is CPU-capable; GPU optimization is optional.

## 4. Sonarr-style user experience and information architecture

### 4.1 Navigation

Suggested top-level sections (names may evolve):

- **Overview** — status, actionable warnings, queue summary, storage overview, quick import.
- **Campaigns** — briefs, eligibility, requirements, linked source media, results.
- **Media Library** — discovered files, source details, storage locations, imports, scanning.
- **Jobs / Activity** — queued, running, blocked, failed, completed; expandable detail.
- **Review Queue** — AI candidates, editorial verdicts, previews, human approval.
- **Exports** — approved output clips and submission tracking.
- **Earnings** — actual campaign/submission/payout records, not projected guarantees.
- **Settings** — system, AI, transcription, media, storage, campaign defaults, platform tabs.
- **System / Diagnostics** — service health, logs, compatibility, version, checks.

Use a consistent visual system, clear hierarchy, responsive layouts, keyboard-friendly and accessible controls, readable status colors **plus text**, empty states, error messages, and confirmation dialogs for destructive actions. Avoid placeholder controls that imply functionality when none exists.

### 4.2 First-run setup wizard

The first-launch web experience should guide the user through:

1. **System:** OS/architecture, Docker connectivity/permissions, database and storage root, writable path tests, available/free disk.
2. **AI:** Ollama URL, discover installed models, assign hunter/reviewer models, test inference/connection.
3. **Transcription:** Wyoming faster-whisper endpoint, language/model options, check availability and timestamp behavior.
4. **Media:** combined yt-dlp/FFmpeg image/runtime checks, image availability, mount mapping, worker health, test tools.
5. **Defaults:** campaigns, output formats, caption presets, approval behavior, storage quotas.
6. **Summary:** ready / needs action / unsupported; exact suggested commands when user action is necessary; rerun checks.

Wizard can be reopened under Settings. The UI must distinguish **installed**, **configured**, **connected**, and **verified**. Never require elevated browser-server privileges just to make setup appear seamless.

### 4.3 Overview layout example (illustrative, NOT real data)

```text
+--------------------------------------------------------------------------+
| ReelAnvil                                     System: Services ready      |
+--------------------------------------------------------------------------+
| Overview | Campaigns | Media | Jobs | Review | Exports | Settings        |
+--------------------------------------------------------------------------+
| SYSTEM STATUS                                                            |
| [Ollama: Connected] [Whisper: Connected] [Worker: Connected] [DB: Healthy]|
|                                                                          |
| QUICK ACTIONS                        STORAGE                              |
| [Import from URL] [Upload video]     Used 68 GB / Reserved 17 GB / ...    |
|                                                                          |
| ACTIVE JOBS                       ALERTS                                  |
| Podcast Ep. 12   Rendering  76%     2 jobs blocked: processing quota      |
| Interview       Transcribing ...   [Review temporary files]              |
+--------------------------------------------------------------------------+
```

The dashboard must reflect actual state once connected. During initial UI development, mock data must be **visibly labeled** as sample/test data.

## 5. Media library and directory lifecycle

### 5.1 Naming and layout

The final name for the arrival directory is **UNDECIDED**. `incoming/` and `inbox/` are candidates; `new/` is not a committed name. Avoid embedding a hard-coded folder name in business logic; use semantic keys and configurable paths.

Illustrative layout:

```text
<configured-media-root>/
├── incoming/                 # provisional name: manual drops + validated downloads
├── temp/
│   ├── downloads/<job-id>/   # partial/staged downloads
│   ├── transcription/<job-id>/
│   ├── analysis/<job-id>/
│   └── rendering/<job-id>/
├── completed/                # finalized clips for review/export
├── failed/                   # retained recoverable artifacts as policy allows
└── archive/                  # preserved original source media
```

Directories are **logical storage areas** and may later map to separate configured host paths or volumes. A job has a stable unique ID and isolated working directories. A source original remains in a stable archive location once ingested; **do not repeatedly move/copy multi-GB originals between every stage**. Intermediate artifacts are created in stage-specific workspaces and promoted safely.

### 5.2 Discovery and file ownership

- Scan the configured arrival directory at startup, on demand, and periodically; optional filesystem watcher is an optimization, not the sole discovery mechanism.
- Show **"Found X media items"**, scanned location, last scan time, new/known/invalid counts, and actionable errors.
- Downloads land in a temporary/staging area and are **atomically promoted** to incoming/registered status only when complete and validated. Manual file drops must be checked for stable size/closed writes before ingest.
- Identify duplicate files and paths without silently deleting them; validate media with ffprobe.
- Record authoritative media/job state and ownership in SQLite. Disk is the source of actual bytes; **folder location alone is not authoritative job status**.
- Use job-specific paths, sanitized filenames, canonical path validation, no symlink/path traversal escapes, safe cross-volume move/copy semantics, and idempotent recovery after crashes.
- Originals remain preserved unless the user explicitly approves deletion under a clear policy. Retention of disposable temp data is configurable.
- Workers/services should receive the minimum mounted paths required, with read-only access when possible.

### 5.3 Processing flow example

```text
Incoming / registered source
        |
        v
Temp: transcription  --> transcript + word timestamps
        |
        v
Temp: analysis       --> clip hunter --> independent editorial reviewer
        |
        v
Temp: rendering      --> clips, captions, FFmpeg progress
        |
        v
Technical QA         --> verify deliverables
        |
        +---- success ----> Completed --> Human approval --> Export
        |
        +---- failure ----> Failed / retryable artifacts

Original source remains in Archive after ingestion.
```

A source may produce multiple candidate/render jobs. Stage changes are transactional state transitions; never allow two workers to claim the same job without coordination.

## 6. Storage quota manager — hard requirement

### 6.1 Configuration modes

Users can select:

- **Total quota only:** e.g., all managed media capped at **200 GB**.
- **Per-area / pool quotas only:** e.g., **incoming + processing = 100 GB**, **archive = 200 GB**.
- **Both:** e.g., **total = 300 GB**, incoming/processing = 100 GB, archive = 200 GB; each applicable cap must be satisfied.
- Future optional folder-specific limits for incoming, temp stages, completed, archive, etc. Quotas should support **GB vs GiB** explicitly and optional unlimited values. Physical free-space protection always applies.

**Important:** The user's example of "100 GB new and processing but 200 GB archives" implies **300 GB potential combined allowance**, unless an additional global cap is set. Quota pools should not double-count bytes belonging to the same underlying file. Define accounting carefully for hardlinks, deduplication, symlinks, and separately mounted volumes.

### 6.2 Storage settings mockup (illustrative)

```text
+---------------------------------------------------------------------+
| Settings > Storage                                                  |
| Quota mode:  ( ) Total only  ( ) Pools only  (x) Total + pools      |
|                                                                     |
| Total managed media limit                         [ 300 ] [ GB v ] |
|                                                                     |
| Storage pools                                                       |
| Incoming + Processing   Uploads / downloads / temp [ 100 ] [ GB v ]|
| Archive                 Preserved source media     [ 200 ] [ GB v ]|
|                                                                     |
| Minimum host free space                          [  20 ] [ GB v ]  |
| [Test paths] [Rescan usage] [Review temp files] [Save settings]     |
+---------------------------------------------------------------------+
```

### 6.3 Usage, reservation, and blocking

Before a storage-consuming job starts, estimate and **reserve** capacity. During the job, monitor actual bytes and enforce quota and host free-space thresholds. For unknown output sizes or downloads, use incremental reservations/limits and abort safely before overrun where technically possible. **A userspace quota cannot guarantee an absolute filesystem hard limit against arbitrary external writes or in-flight buffers**; document best-effort enforcement and optionally support OS/filesystem-enforced quotas later.

Example display:

```text
Incoming + Processing: 100 GB limit
[================== used 68 GB =====][reserved 17 GB][free 15 GB]
Used: 68 GB     Reserved: 17 GB     Available: 15 GB
```

- Storage-related inability to proceed is **Blocked — Storage Limit**, not a generic failure.
- Jobs must not start additional storage-consuming stages when insufficient capacity is available. Existing jobs should be paused/cancelled safely if their actual growth threatens the cap; distinguish soft stop from hard failure.
- Display **which** pool/cap blocked work, bytes needed, current used/reserved/free, and safe cleanup options.
- Example: **"Waiting for 8.2 GB of available processing space."**
- On cleanup or quota increase, allow manual or policy-controlled retry/resume; do not lose completed work.
- Temp cleanup shows files, sizes, age, associated jobs, and whether safe to delete. Require confirmation for destructive operations. Never silently delete originals or approved outputs.
- Periodically reconcile SQLite accounting against actual filesystem bytes and report untracked/orphaned files.
- Keep a separate configurable minimum free host disk threshold. If multiple pools share a filesystem, all are constrained by that same physical free space.

### 6.4 Storage-blocked example

```text
! STORAGE LIMIT REACHED                         Jobs blocked
Processing pool reached 100 GB; 2 jobs waiting.
Disposable temporary files: 23.4 GB
Space required by next job: 8.2 GB
[Review Temporary Files] [Clean Up...] [Retry Blocked Jobs]
```

## 7. Jobs and activity — required expandable experience

### 7.1 Compact list + detailed expansion

Every job in the dashboard is clickable/expandable. The collapsed row shows:

- Descriptive media/job name and stable job ID.
- Current processing stage and concise activity detail.
- Status: **Queued, Running, Paused, Blocked, Failed, Completed** (plus cancellation states if needed).
- **Actual** progress percentage when measurable; otherwise an indeterminate activity indicator.
- **Elapsed time**, not estimated time remaining.

Expanded view includes the complete step sequence, current step, per-step state and details, per-step elapsed time, overall active elapsed time, source/campaign, output paths, storage usage/reservation, warnings, logs, errors, and safe controls (pause/resume/cancel/retry when actually supported).

### 7.2 Layout example (illustrative)

```text
+---------------------------------------------------------------------+
| JOBS / ACTIVITY                                                     |
|                                                                     |
| v Podcast Episode 12            CF-0142       [RUNNING]             |
|   Rendering clip 4 of 5                                            |
|   [=======================-------] 76%                              |
|   Total active elapsed: 18m 42s   Current step: 2m 18s              |
|                                                                     |
|   Processing steps                                                  |
|   [✓] Media import              Source validated and archived      |
|   [✓] Transcription             Transcript/timestamps generated    |
|   [✓] AI highlight discovery    8 candidates found                 |
|   [✓] Independent review        5 accepted / 3 rejected            |
|   [●] Rendering                 Clip 4 of 5; 2m 18s elapsed        |
|   [○] Technical QA              Waiting for renders                |
|   [Logs] [Outputs] [Pause] [Cancel]                                |
|                                                                     |
| > Interview Highlights          CF-0143       [RUNNING]             |
|   Transcription                 [========----------------] 28%     |
|                                                                     |
| > Creator Stream                CF-0144       [BLOCKED]             |
|   Waiting for 8.2 GB of processing quota                            |
+---------------------------------------------------------------------+
```

The 76%/28% values above are **fictional UI samples**, not algorithms for deriving real percentages. A global percentage must be shown only if based on a documented, defensible progress calculation. Otherwise show stage progress and an indeterminate overall indicator.

### 7.3 Progress and timing policy — confirmed decision

**DO NOT DISPLAY ETA OR ESTIMATED TIME REMAINING.** Job size, content complexity, concurrent workload, hardware, model, and processing environment make ETA unreliable. Display elapsed time instead.

| Stage | Progress source when supported |
|---|---|
| Download | Actual bytes transferred / known total; if unknown, activity + elapsed |
| Transcription | Reported processed audio duration / total, **only if Wyoming service exposes reliable progress**; otherwise activity + elapsed |
| AI discovery | Actual processed chunks/batches / known total, if applicable; otherwise activity + elapsed |
| Editorial review | Candidates reviewed / total candidates |
| Rendering | FFmpeg progress timestamp / expected output duration, accounting for multi-clip batches |
| Technical QA | Checks completed / known checks |

Track **overall active elapsed**, **current-stage elapsed**, **completed-stage durations**, and separately **queued/paused/blocked waiting duration**. Paused/blocked time must not inflate active processing time. Persist timestamps and durations through restarts; reconcile interrupted worker operations safely. Do not imply a stage is 100% complete until its output is validated/committed.

### 7.4 Example compact progress card

```text
Podcast Episode 12                                  [RUNNING]
CF-0142 · Rendering clip 4 of 5
[=======================-------] 76%
Total active elapsed: 18m 42s     Current step elapsed: 2m 18s
```

### 7.5 Job states and failure handling

Persist job state in SQLite and define explicit allowed transitions. Job cancellation must be cooperative where possible, with escalation and cleanup if necessary. On restart, reconcile jobs that were marked running; avoid duplicate downloads/renders. Record errors with stage, diagnostic summary, logs, retryability, and artifacts. UI controls must be disabled/hidden when unsupported; no pretend buttons.

## 8. AI, transcription, and rendering standards

### 8.1 AI models and editorial independence

- Ollama is configurable by endpoint and model; local CPU-only use must be possible within hardware constraints.
- AI 1 and AI 2 have separate prompts/context and independently stored decisions.
- Reviewer must not receive AI 1 scores or rationale; it may receive candidate timestamps/content, original context, and campaign brief.
- Candidate schema: source ID, start/end, hook/payoff, context, campaign relevance, provenance, confidence/uncertainty if meaningful, revision history, reviewer verdict.
- Use structured outputs validated by Go. Bound revisions, timeouts, retries, and malformed-output handling.
- Never assume AI judgment guarantees quality, compliance, views, or payout.

### 8.2 Transcription

- Preferred initial candidate: maintained **LinuxServer.io faster-whisper** container with Wyoming protocol; Go requires a Wyoming adapter/client.
- Benchmark accuracy, word timestamps, model availability, CPU speed, memory, and API behavior on the Debian SER6 before declaring production-ready.
- Transcripts are durable artifacts with language, timing, model, and provenance metadata.

### 8.3 Media worker

- Preferred combined upstream yt-dlp + FFmpeg/ffprobe image candidate must be verified for maintenance, licensing, architecture, Deno/runtime support as needed, FFmpeg filters, subtitles, and safe execution.
- Go worker exposes narrow operations such as **download**, **probe**, **extract audio**, **render**, **verify**; not arbitrary shell/container commands.
- Capture stdout/stderr, structured progress, exit codes, and artifact manifests. Validate URLs against policy, filenames, path mounts, resource limits, and timeouts.
- Do not install yt-dlp/FFmpeg on host as mandatory dependencies; use existing container images.
- CPU-first rendering with deterministic crop/format presets. Face-aware auto-framing may be a later feature, not a release blocker.

### 8.4 QA and publishing

- Technical QA is distinct from editorial QA: verify format, duration, dimensions, audio, caption presence/timing, corruption, black/silent segments where feasible, and campaign requirements.
- Outputs enter human review before export/publication.
- No unofficial social posting automation assumed in v1. Record actual submissions and qualified performance when available.

## 9. Go engineering standards

- **Architecture:** modular packages and explicit interfaces for media worker, AI, transcription, filesystem, storage accounting, and job scheduling. Keep OS-specific behavior behind adapters.
- **Persistence:** SQLite embedded into Go, persistent DB directory, migrations, foreign keys, transaction boundaries, backups and recovery tests. Pin Go SQLite driver and toolchain versions.
- **Concurrency:** bounded queues, per-service limits, cancellation via contexts, timeouts, idempotency keys, transactional job claiming, safe worker restart behavior.
- **Security:** no Docker socket in `serve`; worker auth and local-only default; protect secrets; strict input validation; no arbitrary command execution; controlled filesystem access; no path traversal or unsafe symlink following; CSRF/session/auth strategy for browser access; avoid public exposure by default.
- **Observability:** structured logs with job IDs, user-facing error summaries, correlation IDs, health/readiness checks, diagnostic export with secrets redacted.
- **Configuration:** UI-backed persisted settings, validation before save, schema versioning, tested defaults, import/export/backup as appropriate; document which changes require restart.
- **Frontend:** server-rendered Go templates plus lightweight JS is preferred initially; avoid requiring Node/React solely for routine UI. Maintain coherent components, responsive behavior, accessibility, useful empty/loading/error states.
- **Testing:** Go unit/integration tests, SQLite migration tests, filesystem/quota edge cases, mock adapters, API contract tests, browser workflow tests, restart/recovery tests, container integration on target Debian host.
- **Builds:** reproducible Linux AMD64 first; Go cross-platform compile checks may be added without claiming Windows/macOS runtime support. Document toolchain and dependency versions.
- **Documentation:** installation, first-run wizard, service requirements, storage, quotas, job lifecycle, troubleshooting, security, upgrades, and known limitations.
- **Release quality:** no misleading sample values in production, no dead-end navigation, no destructive actions without confirmation, no claims of tested platforms that haven't been validated.

## 10. Development phases and acceptance gates

| Phase | Primary output | Minimum acceptance |
|---|---|---|
| 0. Governance | Repo structure, source-of-truth rules, agent documents | `AGENTS.md`/`CLAUDE.md`, CI, and branch workflow in place |
| 1. Foundation | Go server, SQLite, config, migrations, service interfaces | Launches on Debian; persistent settings; tests pass |
| 2. Finished-feeling UI shell | Overview, settings, wizard, media library, jobs, reviews | Consistent design; functional navigation; clearly labeled mock adapters |
| 3. Library + quotas + workflow | Scan/import, path mapping, file inventory, quota reservations, job state | Finds manually placed media; accurate counts; safe quota block/recovery |
| 4. Processing logic | AI discovery/review contracts, approvals, stage orchestration | Durable state, reproducible mock tests, no false progress |
| 5. Worker | Separate native Go process, authenticated API, Docker integration | Restricted operations, safe mounts, progress, cancellation |
| 6. Services and deployment | Ollama, Wyoming faster-whisper, combined yt-dlp/FFmpeg, Compose | End-to-end integration on Debian 13; health checks and pinned images |
| 7. Quality and release | Real authorized footage, QA, cleanup, restart/recovery, docs | No placeholder actions; known limitations; successful full workflow |

**Sequencing decision:** Prioritize the dashboard and core application first. **Define the worker API contracts early**, but implement the actual worker/container integration after the majority of the first-pass app/UI is ready. Do not let this sequence create a fake sense that mocks are production features.

## 11. Source of truth and development workflow

1. **Primary authority:** the maintainer's local Git repository under `Projects/`.
2. **Shared mirror:** GitHub `gkuba/ReelAnvil`. `dev` carries development; a release squash-merges `dev` into `main`, and a fresh `dev` is then started from `main`.
3. Development happens directly in the repository, with a coding agent (currently Claude Code) working alongside the maintainer. The agent edits, tests, builds `bin/Release` and reports what was verified. The maintainer reviews the running build and approves commits, pushes and releases. The rules are in `AGENTS.md`; Claude-specific notes are in `CLAUDE.md`.
4. Independent review comes from the maintainer's own testing, CI, and a fresh-context code review of the branch before a release, not from patch or ZIP handoffs.
5. Builds 0.1–0.9.1 were produced through a ChatGPT patch-and-handoff process. That history is preserved at tag `archive/chatgpt-build-0.9.1`.
6. Never claim a build, test, commit, push or deployment happened without evidence.

## 12. Open questions and decisions NOT yet finalized

1. Final incoming folder label: `incoming`, `inbox`, or another name.
2. Exact upstream combined yt-dlp/FFmpeg image, supported tags/digests, and ability to run required operations without derivative images.
3. Exact Linux packaging/install method for `serve` and `worker`, systemd unit design, upgrade strategy, and whether a native installer is worthwhile.
4. Worker transport/authentication details, Docker Engine permission isolation, and whether a restricted Docker proxy/rootless engine can be used.
5. Exact quota enforcement method, filesystem edge cases, accounting for hardlinks and shared mounts, per-folder limits vs grouped pools, and default limits.
6. Exact frontend component system, design tokens, layout/navigation details, accessibility testing.
7. Which faster-whisper model and Ollama models meet CPU-only quality/performance targets on SER6.
8. Detailed campaign eligibility, social platform submission rules, source licensing, and payout tracking integrations.
9. When Windows/macOS experimental builds become available; no commitment to support yet.

## 13. Definition of done for any user-facing feature

A feature is not complete merely because an API responds. It must have:

- Functional UI and correct navigation, plus accessible loading/empty/error/success states.
- Persisted state and validated inputs where applicable.
- Real service status or **clearly identified mock mode**; no fictional metrics in normal operation.
- Appropriate permissions, safe paths, and confirmation for destructive operations.
- Diagnostics/logs and meaningful recovery paths.
- Unit/integration coverage, documented behavior, and manual validation on supported Debian environment.
- No misleading ETA, progress, storage counts, or platform support claims.
- A clean fit with the established design system and setup/settings workflow.

---

**Status of this file:** This is a comprehensive design capture based on the conversation, **not** a verified implementation or a replacement for the existing project's agent governance documents. Decisions labeled "proposed," "candidate," "illustrative," or "open" remain subject to testing and explicit agreement.

## 14. Public repository, installation, licensing and brand policy (confirmed direction)

### 14.1 Public-first release readiness

- **Public GitHub repository planned.** GitHub is the source of truth for source code, tagged releases, release checksums, issues and public documentation. The local development repository remains authoritative during the controlled build/handoff process described in section 11; publish/push deliberately.
- Never commit API credentials, tokens, private footage, user campaign information, SQLite databases, machine-specific paths or private logs. Provide sanitized example configuration, `.gitignore`, secret scanning and a security disclosure path.
- Provide a real README, architecture and supported-platform documentation, install/upgrade/rollback/backup/recovery guidance, changelog/release notes and contribution instructions. Do not claim platform support before testing.
- Keep deployment operable without a custom domain. Domain purchase and paid legal registrations are deferred until justified by project income or other business need.

### 14.2 Packaging and one-line installer

- Distribute ReelAnvil-owned code, native Go application binaries, application UI assets, our scripts and configuration/Compose manifests. **Do not bundle, mirror, repackage or redistribute upstream container images, model weights, or third-party standalone executables.**
- Docker Compose references vetted upstream images and registries; Docker pulls those images directly. Pin validated tags or digests; document sources and licenses. Do not create or maintain custom Docker images.
- Installation paths: documented manual install; versioned GitHub Release artifacts; optional convenient one-line install script. An eventual custom domain can point to a **GitHub Pages**-hosted static `install.sh`, which fetches verified GitHub Releases. GitHub raw-file URLs are not equivalent to custom-domain Pages hosting.
- Installer must be inspectable, repeatable/idempotent where feasible, explicit about privileges and paths, check prerequisites and architecture, verify artifact integrity, preserve existing configuration and media, and fail safely. Avoid blindly running development-branch code or overwriting user data. Do not present `curl | bash` as the only installation method.
- Keep normal service setup in the web wizard; installation should bootstrap the app, directories and prerequisites rather than hard-code the user's entire deployment.

### 14.3 ReelAnvil licensing and third-party boundaries

- **MIT is the selected licensing direction** for ReelAnvil-owned application code. Forking, modification, commercial use of the tool, and commercial sale of forks are permitted subject to MIT's preservation of copyright and license notices. MIT does **not** require prominent in-app attribution.
- Before public release, finalize the copyright holder and exact `LICENSE` notice. Copyright on original eligible work generally arises automatically in the US; paid federal copyright registration is not a release prerequisite.
- Third-party software retains its own licenses, copyrights and terms. Refer to `LICENSING.md` and `THIRD_PARTY_NOTICES.md` in the ReelAnvil Library and, at release, the repository. Referencing upstream Docker images is not the same as distributing them.
- **Important exception to “our code only”:** compiled Go binaries and bundled frontend assets may incorporate third-party code/assets. Audit actual dependencies, licenses, notices and any distribution obligations before public release; do not assert all compiled bytes are solely ours.
- The ReelAnvil name/logo and official-project identity are separate from MIT code permissions. Forks may identify their origin but must not misrepresent official status or endorsement. `TRADEMARKS.md` is branding guidance, **not** a claim of federal registration or guaranteed exclusive rights.
- No paid copyright/trademark registration is planned initially. Common-law trademark rights depend on real use and circumstances; a GitHub repository alone is not necessarily trademark use in commerce. Perform a fresh conflict check before public release; preliminary searches are not legal clearance.

### 14.4 Pre-publication acceptance gate

Before making a public release, verify: final license/copyright wording; dependency and notice audit; upstream image versions and sources; installer download integrity and privilege behavior; secrets/privacy scan; clean-machine Debian 13 installation; backup/upgrade/rollback procedures; and accurate supported/experimental platform labels. These are release gates, not claims of work already completed.

