# ReelAnvil — Build History

One maintained record of what was built, how it was checked, and where it landed. Add the newest entry at the top. Keep entries short: what changed, what was verified, and what wasn't.

## 2026-11-12 — Build 0.9.2: pause and filler cutting, three-word captions, title card

- **Changes:** `pipeline/clipedit.go` (WordCues, FillerSpans, KeepSpans, Remap, CuesSRT, ASS); worker op `silences` and render with `keep` pieces and an ASS file; `renderClip` in the job with a whole-clip fallback; manifest `expected_ms` used by QA; Settings → Media → Clip rendering (`trim_silences`, `word_captions`, `title_card`, default on).
- **Verified:** gofmt, vet, `go test -race ./...` including a real-FFmpeg worker test (silence found, two tones joined to 4.4 s, bad piece rejected).
- **Deployed to pixelreel (live, port 4747) and tested, builds 07bb7d8, 280c4d0, b00dcd9:** jobs 6 and 7 cut and joined pieces but burned captions and the title card were blank: the media container has no fontconfig setup, so libass could not use the fonts in the folder (also, only icon fonts were in it). Fix: icon fonts skipped, fontconfig file given to FFmpeg, DejaVu Sans via Debian `fonts-dejavu-core` (installed by `upgrade`, which now installs newly required packages). Job 8 (STS-135 FD7, 14b models): title card and three-word captions visible in frames of the finished clip, 3 pieces joined, QA passed. That run accepted only 1 of 10 candidates (reviewer variability; job 5 accepted 3 of 6).
- **Not verified:** exact caption timing against speech (estimated per line); the other clips of jobs 6-8 by eye.

## 2026-11-12 — Build 0.9.2: release readiness (not committed until pushed below)

- **Changes:** digest pins (`compose.yaml.tmpl`, `pin_test.go`); `scripts/package-release.sh`, `check-release.sh`, `scan-secrets.sh` and CI steps; notices audit; README install guide; `themes_recovery_test.go` (8 tests); copy cleanup (footers, top bar, no-login wording).
- **Verified:** gofmt, vet, `go test -race -count=1 ./...`, staticcheck, secret scan, release archive built and checked (a bad archive with STATUS.md is rejected); phone-width (375 px) screenshots of Overview and Settings rendered in headless Chromium against a scratch data dir.
- **Not verified:** a clean-machine install from the final README on fresh Debian 13; the new digests were taken from the running pixelreel images, not re-pulled; no penetration test.

## 2026-11-12 — Build 0.9.2: end-of-answer check, clip manifests, Developer features (not committed)

- **Changes:** `pipeline.EndPrompt/ParseEnd/EndLimit` and `run.checkEnd` in the hunt step; `writeManifest` in the render step; `Settings.DevFeatures` with `devOnly` route gating, sidebar/media/review/jobs template guards and a pipeline Overview.
- **Verified:** gofmt, vet and `go test -race ./...` pass, including new tests for the end parser, the manifest and the hidden Campaigns area.
- **Deployed and tested on pixelreel (build 7e537f8):** job 5 (STS-135 FD7, 3 × 20–60 s, qwen2.5:14b) finished in 18m55s: 6 candidates, 3 accepted, 3 rendered with manifests, QA 3 of 3. The end check ran per pick and moved two ends (lines 16-18 and 97-100 to 101); the other picks kept their cut. Clip endings not yet judged by the maintainer.
- **Not verified:** browser check at phone width.

## 2026-11-07 — Build 0.9.2: pull card updates in place (not committed)

- **Changes:** the Settings → AI meta refresh is replaced by `initPoll` in `app.js`. It re-reads the page every 3 s and swaps the `#pull-card` element in place. When the pull ends, it refreshes the `data-refresh` hunter and reviewer lists once, keeping an unsaved choice. The CSP gains `connect-src 'self'` (fetch was blocked without it). Pull and Stop redirect to `/settings/ai#pull-card`.
- **Verified:** gofmt, vet, `go test -race`, staticcheck and `scripts/build-dev.sh` pass. Tests check `data-poll`, `data-refresh` and the absence of a meta refresh, plus the CSP and the redirect. Deployed to pixelreel; the CSP header was confirmed there. The maintainer re-pulled qwen2.5:14b and confirmed the card updates without flashing ("much better").
- **Not verified:** a phone-width check of the updating card.

## 2026-11-07 — Build 0.9.2: recommended models on the Pull card (not committed)

- **Changes:** `internal/app/hostinfo.go` reads MemTotal from `/proc/meminfo` and GPU vendor names from `/sys/class/drm/card*/device/vendor` (paths injectable), asks Ollama `/api/ps` for GPU/CPU, and holds a small catalog (qwen2.5 3b/7b/14b; download sizes 1.9/4.7/9.0 GB confirmed on ollama.com tags). Pull card: facts list, dropdown (`pull_model`, default = Recommended), "Other model" field (`pull_other`, wins when non-empty), Download button. No GPU setup added.
- **Verified:** new tests for meminfo parse, vendor map, recommendation at 4/8/12/20/27 GB and unknown RAM, `/api/ps` GPU/CPU/none/unreachable, page markup, pull from both fields.
- **Also verified:** gofmt, vet, `go test -race`, `go mod verify`, staticcheck, `scripts/build-dev.sh` pass. Settings → AI checked in headless Chromium against a scratch copy of `bin/Release` (port 18089, no model pulled): card renders at 1400px and 375px, nothing cut off or scrolling sideways in screenshots.
- **Not verified:** a numeric `scrollWidth` check (screenshots only); a real pull from the dropdown; GPU/CPU text against a loaded model; the maintainer's review.

## 2026-11-06 — Build 0.9.2: reviewer scores and acceptance threshold (not committed)

- **Changes:** reviewer prompt shows numbered clip lines and asks for five 1–10 scores with reasons, labelled parts and a verdict; `ParseReview` requires all five scores (else the reviewer is asked again, then reject) and drops invalid parts; `Gate` rejects when any score is below the setting and uses "Criterion n/10: reason" as the retry note. Schema v6 (`candidates.scores`, `candidates.parts`, JSON; parts stored as clip-relative ms). Settings → AI "Lowest score to accept" (`min_review_score`, 1–10, default 6, 0/missing loads as 6). Job log shows the scores per candidate. Review cards show score chips, reasons and part times.
- **Verified:** `gofmt -l .` empty, `go vet ./...`, `go test -race ./...`, `go mod verify`, staticcheck, `scripts/build-dev.sh` pass. New tests: parse (valid, missing, out of range, invalid parts), gate, v5→v6 migration, reviewStep rejecting a model "accept" with one low score and retrying with that reason, setting validation, review page chips and parts. Review page checked in headless Chromium at 1400px and 375px against a scratch copy of `bin/Release` (test clip inserted in the scratch DB only): chips, reasons and parts render, no horizontal scroll.
- **Model check (pixelreel Ollama, reviewer prompt only, not a full job):** qwen2.5:3b gave all five scores in 5 of 5 replies but scored a good test clip 1–4 on hook, so a gate of 6 would reject nearly everything. qwen2.5:7b (pulled on pixelreel) scored the same clip 7–10 and accepted it 3 of 3 times (20–30 s each), and scored a greeting clip 2–5 and rejected it 3 of 3 times.
- **Not verified:** a full job with the 7B model; the maintainer's review.

## 2026-11-06 — Build 0.9.2: clip feedback, lessons and retry (not committed)

- **Changes:** schema v5 (`lessons`; `jobs.redo_clip_id`, `redo_note`, `attempt`; `candidates.attempt`). Review form Feedback/Retry switches, Lessons tab with delete, "Attempt N" on clips. `DecideClip` upserts or deletes a clip's lesson; `CreateRedoJob` copies source, campaign and transcript and marks import/transcribe done. `HunterPrompt`/`ReviewPrompt` take lessons; `FixPrompt` asks for one clip that fixes a note, within 90 s of the original. `reviewStep` auto-retries rejected candidates while accepted < clip count (Settings → AI, 0–3, default 2). `.tabs` wraps at phone width. Built by a sonnet subagent, checked by the main session.
- **Verified:** `gofmt -l .` empty, `go vet ./...`, `go test -race ./...`, `go mod verify`, staticcheck, `scripts/build-dev.sh` pass. New tests cover the v4→v5 migration, lesson upsert/delete, redo jobs, prompts with lessons, the fix window, auto-retry with a fake Ollama (accept on retry, give up after the limit, no retry when enough), the Review switches and the Lessons tab. Review, Lessons and Settings → AI checked in a browser at desktop and 375 px on a scratch data dir. `bin/Release` copied to `bin/Release-backup-2026-11-06-pre-v5` (its database is at schema v3).
- **Not verified:** a real model run of a retry or auto-retry; the maintainer's review.

## 2026-11-06 — Build 0.9.2: staticcheck findings fixed (not committed)

- **Changes:** CI's staticcheck step has failed since `64f802b`. Removed the unused `Server.mediaRoot`; the `..` media-root error no longer ends in punctuation (ST1005); a failure to create the default media folders at startup is now logged instead of dropped (SA4006).
- **Verified:** `gofmt -l .` empty, `go vet ./...`, `go test -race ./...`, `go mod verify`, `scripts/build-dev.sh` pass.
- **Not verified:** staticcheck itself (not installed locally); CI.

## 2026-11-06 — Build 0.9.2: clips run until the speaker finishes (not committed)

- **Changes:** transcript lines are cut at pauses, not sentences, so a clip could stop mid-sentence (job 2's 6:05–6:55 clip ended on "…awestruck at what…"). After the length fit, `finishSentence` moves the end forward to the first line ending in `.`, `!` or `?` (a trailing ellipsis doesn't count), past the job maximum up to `FinishCapS` (120 s) or the job maximum if larger; it never moves the end back. The hunter prompt says to end only when the speakers have finished the answer or exchange; the reviewer rejects clips that stop before the speaker is done. The job maximum stays a target, not a hard stop (maintainer: YouTube allows more than 90 s, most clips under 2 min).
- **Verified:** `gofmt -l .` empty, `go vet ./...`, `go test -race ./...`, `scripts/build-dev.sh` pass; new test covers extending past the maximum, an already finished line, the 120 s cap and a larger job maximum.
- **Not verified:** a re-run on the Debian 13 host; the maintainer's review of the clips.

## 2026-11-06 — Build 0.9.2: media pipeline, worker, Review and campaign posts (not committed)

- **Changes:** schema v4 (jobs, steps, log, sources, transcripts, candidates, reviews, clips, submissions, worker ops). `internal/pipeline` (segmenting at pauses, hunter and reviewer prompts and parsing, caption timing; the hunter keeps a proposal's opening line and fits its end to the length limits), `internal/worker` (download / probe / audio / render only, path and URL checks, Podman/Docker `exec` or host programs), `reelanvil worker` with a loopback Bearer-token API (`settings/worker.token`, 0600), job runner in the server (`internal/app/jobs.go`: steps, retry from the first unfinished step, cancel, storage-limit block). Pages: Media (new job from `incoming/` or HTTPS link, rights confirmation, campaign), Jobs (steps, real counts, elapsed, log, candidates, clips, Cancel/Retry), Review (player, downloads, approve/reject, record and update posts); Campaigns rows show jobs/clips/approved/posts/views/earnings with a New clip job link. The worker systemd unit now runs `reelanvil worker` after `compose up -d`. Placeholder template removed.
- **Verified:** `gofmt -l .` empty, `go vet ./...`, `go test -race ./...`, `go mod verify`, `scripts/build-dev.sh` pass. New tests: store media records and state rules, pipeline parsing (ranges, fitting, overlap, reviewer prompt hides hunter reasoning), worker path checks and a real FFmpeg probe/audio/render run, worker API auth, token file, job form, `underRoot`. Local end-to-end on a scratch data dir (`serve` + `worker` on `127.0.0.1:18089`, host programs, Ollama and Whisper on pixelreel through an SSH tunnel, qwen2.5:3b for both roles): an 18:34 NASA file went import → transcribe (140 segments) → hunt (3 candidates; the first attempt failed on clip lengths, fixed by end fitting) → review (1 of 3 accepted) → render (1080×1920, captions burned) → QA passed in 3m32s; approve, record post, update views and earnings worked and the campaign row showed the totals. Media, Jobs, Review and Campaigns checked in headless Chromium at 1400px and 375px. On the Debian 13 host after `upgrade` (worker unit active, Podman media engine, qwen2.5:3b): an archive.org HTTPS link went download → transcribe → hunt → review (1 of 3) → render → QA passed in 2m28s; approve, video (range request) and `.srt` downloads, and a recorded post on a test campaign worked.
- **Fix:** the first host test picked the broadcast greeting (0:00–0:21) of an 18:34 video: the whole transcript went to qwen2.5:3b in one prompt and all three picks came from the first 3 minutes. Hunter windows are now 4,000 characters (was 20,000), and both prompts carry a rule (`pipeline.NotAClip`) against greetings, voice checks, sign-offs and housekeeping. Tests: an 18-minute transcript splits into at least 4 parts (fails on the old value), both prompts include the rule. `gofmt`, `go vet`, `go test -race`, `go mod verify` and the build pass. Re-run on the Debian 13 host (job 2, same video, qwen2.5:3b for both roles): 6 candidates from 0:00 to 15:32 (was 0:00–3:03), the greeting rejected, 3 accepted (5:23 Expedition 18 story, 6:05 approach to the station, 8:25 final-mission interview), rendered and QA passed in 4m12s. The 3B model rates every pick 9/10 and once copied the rule into a hook; not yet watched by the maintainer.
- **Fix:** clips did not play in the browser: the CSP (`default-src 'none'`) had no `media-src`, so `<video>` was blocked. Now `media-src 'self'`, with a test.
- **Not verified:** the maintainer's visual review; the Docker engine; a storage-limit block in a real run. The test campaign, job and post on the host are test data and can be removed.

## 2026-11-06 — Build 0.9.2: Services hides the container media engine (`3161fb4`, pushed to `dev`)

- **Changes:** the Services page shows the Media engine card only when the engine is host programs; for Podman or Docker the web app can't check the container, so there is no card (`reelanvil doctor` checks it).
- **Verified:** `gofmt -l .` empty, `go vet ./...`, `go test -race ./...`, `scripts/build-dev.sh` pass; a test asserts no Media engine card for container engines. On the Debian 13 host after `upgrade`: `/services` shows Web app, Ollama, Whisper and Storage, status Ready.
- **Not verified:** the maintainer's visual review.

## 2026-11-06 — Build 0.9.2: working Settings and Services pages (`64f802b`, pushed to `dev`)

- **Changes:** settings version 5 (`media_root`, `storage_limit_gb`, `hunter_model`, `reviewer_model`, `whisper_model`, `whisper_language`; version 4 files migrate by filling defaults, future versions are still refused). Paths & Storage (save, Check with write test, `Statfs` free space, bounded walk for used space, over-limit warning; saving creates missing `incoming/ temp/ completed/ failed/ archive/` at 0750 only when the root exists; arrival folder name is one constant). AI: installed-model selects for the two roles, background `/api/pull` stream owned by the server (one at a time, stoppable, cancelled on shutdown, 5-minute stall limit, bytes summed from Ollama's per-layer numbers), `/api/generate` model test (60 s, write deadline extended for that request). Transcription: model and language fields with the service's reported models. Connectors: honest static page. Services: five status cards checked in parallel with 3 s limits, summary with commands. Installer: `compose.yaml` is now `compose.yaml.tmpl` rendered from `config.json` (validated Whisper values and media root), `install`/`upgrade` create the media folders, `doctor` checks the media root (exists, writable by `reelanvil` via `runuser ... test -w`) and notes a compose file that no longer matches the settings.
- **Verified:** `gofmt -l .` empty, `go vet ./...`, `go test -race ./...`, `go mod verify`, `scripts/build-dev.sh` pass. New tests: v4→v5 migration and validation, media root rules (also compared with the installer's), storage check on temp dirs (sparse 1.5 GB file for the limit, entry cap, read-only dir), folder creation never touching existing files, Ollama stubs for tags, pull stream (progress, errors, shutdown, cancel, stall), generate (reply, 404, garbage, timeout), Wyoming stub on the Transcription page, Services page ready / needs action / hanging services, sidebar highlight for every page, cross-origin and method checks on the new routes, compose rendering and refusal of unsafe values, media folder ownership, doctor media root checks. A scratch `serve --data-dir <scratch> --addr 127.0.0.1:18089` run returned 200 for all 15 pages and created `media/` with the five folders; the server was stopped afterwards.
- **Not verified:** a real model pull and model test against the Ollama container, `upgrade` / `doctor` on the Debian 13 host with the new compose template, the pages in a browser at desktop and 375px widths, and the maintainer's visual review. The Services page was seen only with Ollama and Whisper down (scratch run).

## 2026-11-06 — Build 0.9.2: service settings pages and installer pre-fill (`baa7fe0`)

- **Changes:** settings version 4 with `ollama_address`, `whisper_address`, `media_engine`, `ffmpeg_path`, `ytdlp_path` (older files migrate by filling defaults; unknown future versions are still refused). New Settings → AI, Transcription and Media pages with save forms and Test connection buttons (Ollama `/api/version` + `/api/tags`; Whisper TCP dial then Wyoming `describe`; host programs via `exec.LookPath`, never run; no container socket access). `install` and `upgrade` write or complete `server/settings/config.json`; `doctor` checks the two service addresses accept a TCP connection (FAIL line, no repair). In `internal/app/services.go`, templates `ai.html`, `transcription.html`, `media-settings.html`, `internal/install/config.go`.
- **Verified:** `gofmt`, `go vet`, `go test -race`, `go mod verify` and `scripts/build-dev.sh` pass. Tests use httptest servers for Ollama and a `net.Listen` stub for Wyoming, plus migration, validation, cross-origin, sidebar-highlight and installer/doctor cases. A scratch `serve --data-dir` run returned 200 for the three pages and wrote the version 4 file.
- **Fix:** form posts from LAN clients over plain HTTP were rejected ("cross-origin request rejected"): `Referrer-Policy: no-referrer` makes browsers send `Origin: null`, and non-secure origins send no `Sec-Fetch-Site` to vouch for it. Now `Referrer-Policy: same-origin`, with a regression test.
- **Verified later:** pages at desktop and 375px on a scratch copy (no sideways scroll); `upgrade` and `doctor` on the Debian 13 host (all ok); both Test connection actions reach the real Ollama and Whisper containers.
- **Not verified:** the maintainer's visual review.

## 2026-11-06 — Build 0.9.2: Go installer (pushed to `dev`)

- **Changes:** `install [--service]`, `upgrade [--dry-run]`, `doctor [--repair [--clean]]` and `uninstall` with `--yes`, `--dir` (default `/opt/ReelAnvil`) and `--port` (default 4747); `serve` now defaults to `0.0.0.0:4747` too, so the Settings page shows the real port. In `internal/install/` and `cmd/reelanvil/install.go`, following the manual install runbook proven on the Debian 13 reference host.
- **Verified:** `gofmt`, `go vet`, `go test -race` and `scripts/build-dev.sh` pass. On the Debian 13 reference host: install, a real `upgrade`, `doctor --repair --clean`, and 5 clean loops (install `--service --yes` → validate → uninstall `--yes` incl. Podman → reboot → host snapshot identical to the pre-install baseline), all checks passing each time: LAN 200 on 4747, runs as `reelanvil`, 3 containers, Ollama, Whisper port, media FFmpeg/yt-dlp/Deno, 10300/11434 on loopback only. A bug review then found 10 issues (deleting an existing `--dir` or the user's own Podman storage, purging later dependents, root following symlinks in service-owned folders, `uninstall --yes` deleting data, `install --service` ignored on an existing install, uninstall reruns, `userdel` of a pre-existing user, ownership after a UID change, `doctor` exit code); all fixed with regression tests, then 1 more clean loop passed and Settings showed port 4747 on the host.
- **Not verified:** plain `install` without `--service` in the loops; upgrading from an older install; the ufw rule and Docker-only prompt (not built); the maintainer's visual review.

## 2026-10-31 — Build 0.9.2: LAN listening and network settings (`1ef80fd`, pushed to `dev`)

- **Changes:** listen address and allowed host names became saved settings (default `0.0.0.0:8080`) with a Network card on Settings → General; `POST /system/restart` checks the new address can be bound, renders a reconnect page, shuts down gracefully and re-executes the binary; a pending-restart banner on every page; Host policy accepts IP literals, the machine's name and listed names only when not on loopback; `--addr` / `--allowed-hosts` override per run; README not-for-internet disclaimer.
- **Verified:** `gofmt`, `go vet`, `go test -race` and `scripts/build-dev.sh` pass. On a scratch copy: saving port 18090 showed the banner, Restart reconnected the browser to `:18090`, and restarting back to `:18089` worked; restart page and Settings at 375px with no horizontal scroll; LAN-IP, hostname and foreign-Host requests returned 200/200/421 on a `0.0.0.0` scratch run.
- **Not verified:** a restart under systemd on the Debian host; access from a second machine on the LAN; govulncheck/staticcheck (CI).

## 2026-10-29 — Build 0.9.2: web UI restructure and fixes

- **Changes:** moved the inline HTML/CSS/JS out of Go strings into embedded `internal/app/web/` files with one layout; built-in palettes became Go data served as `/assets/themes.css`; CSP tightened to `script-src 'self'; style-src 'self'`; one-time cookie messages replaced `?message=` / `?error=`; SQLite schema v3 (date-only deadlines, platform/status validation in Go, ordered migrations); `--data-dir` / `--addr` flags, `version`, a fuller `doctor`, build version stamping; GitHub Actions CI; agent docs rewritten for the Claude Code workflow.
- **Bugs fixed:** manual campaigns stored View/Apply/Submit URLs one column off and left `created_at` empty (the v3 migration repairs affected rows); duplicating Dracula copied an outdated palette; duplicate-theme names could exceed 64 characters; duplicating a theme rewrote `config.json`; a campaign URL collision showed a raw SQLite error; untracking a missing campaign reported success; money input accepted `1.-5`.
- **Verified:** `gofmt`, `go vet`, `go test -race`, `go mod verify` and `scripts/build-dev.sh` pass. A v2 database with sample campaigns migrated to v3 on a scratch copy (URLs repaired, foreign key and cascade intact). Browser checks on the scratch copy covered every route, a computed-style comparison against Build 0.9.1, theme preview/save/duplicate/edit, the compact rail flyout, campaign add/edit errors, and 375px phone width with no overflow. The maintainer reviewed the build from `bin/Release`.
- **Not verified locally:** govulncheck and staticcheck (they run in CI).
- **Visual change:** Settings pages now use the same headings, labels, cards and sidebar as Campaigns.

## Builds 0.1–0.9.1 (2026-10-27 to 2026-10-28) — ChatGPT handoff era

These builds were designed in ChatGPT and applied, built and pushed by a local ChatGPT Work agent through a patch-and-handoff process. That history has been squashed into a single commit on `main`. The original commits remain at tag **`archive/chatgpt-build-0.9.1`**, and the detailed per-build validation records are in the maintainer's local archive.

| Build | Summary |
|---|---|
| 0.1 | Go server and Aurora UI shell, persistent JSON settings, `/healthz`. |
| 0.2 | Six-theme settings with legacy theme-ID migration. |
| 0.3 | Multi-tone theme palettes and metric cards. |
| 0.4 | Theme Manager: duplicate, edit, preview and save custom themes; backup restore. |
| 0.5 | Settings split into General, Themes, Connectors, Paths & Storage; SVG icons. |
| 0.6 | Compact navigation rail with flyouts; glass-style panels. |
| 0.7 | Dracula consolidated as `dracula-dark`; loopback-only binding, Host/origin checks, body cap, timeouts, security headers. |
| 0.7.1 | Accept `Origin: null` only with same-origin Fetch Metadata; shared-router POST tests. |
| 0.8 | SQLite foundation (pure-Go `modernc.org/sqlite`), WAL, startup read/write probe, schema v1. |
| 0.9 | Schema v2 campaign records and tracking, Campaigns and Overview pages, branding assets. |
| 0.9.1 | Unified sidebar hierarchy, global icon setting (settings v3), layout consistency fixes. |
