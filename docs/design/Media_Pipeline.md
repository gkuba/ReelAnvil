# Media pipeline and worker contract — first pass

**Status:** first working pass (Build 0.9.2-dev). Describes what the code does; open points are listed at the end.

## Processes

- `reelanvil serve` owns SQLite, the job engine, Ollama (Clip Hunter and Reviewer) and the Whisper (Wyoming) client. It has no container access.
- `reelanvil worker` is a separate process with the same binary. It runs only four media operations — **download**, **probe**, **audio**, **render** — using FFmpeg, ffprobe and yt-dlp. With the Podman or Docker engine it runs them through `exec` in the `reelanvil-media` container; with the host engine it runs the configured programs directly. It never runs a shell or a caller-supplied command.
- The worker talks to the server over a narrow JSON API on the server's own port. Requests must come from a loopback address and carry `Authorization: Bearer <token>`, where the token is `settings/worker.token` (created by `serve`, mode 0600). Both processes run as the same user.

## Worker API

| Request | Purpose |
|---|---|
| `POST /api/worker/claim` | Claim the oldest queued operation. `200` with `{id, kind, args}` or `204` when there is none. |
| `POST /api/worker/ops/{id}/progress` | `{done, total, detail}` and a heartbeat. `409` means the operation was cancelled: stop it. |
| `POST /api/worker/ops/{id}/finish` | `{ok, result, error}`. |

The worker checks every path itself: absolute, clean, inside the media root, no symlinked component; outputs only under `temp/`. URLs must be `https://`. An operation with no heartbeat for 2 minutes is marked failed.

## Job steps

| Step | What happens | Progress shown |
|---|---|---|
| Import | Incoming file: moved into `archive/<source-id>/`. URL: yt-dlp download into `temp/downloads/<job>/`, then moved to the archive. Then probed (needs video and audio). | Download bytes when yt-dlp reports a total |
| Transcription | Audio extracted to `temp/transcription/<job>/` with silence detection; the audio is cut at pauses into 3–12 s segments and each is sent to Whisper. | Segments done / total |
| Clip Hunter | Hunter model reads numbered segments plus the campaign brief and proposes clips as segment ranges. Go keeps each proposal's opening line and moves its last line to fit the length limits when it can, then checks ranges, lengths and overlaps; up to 3 attempts. Long transcripts are split into windows. | Windows done / total |
| Review | Reviewer model sees each candidate's text, the surrounding text and the campaign brief, never the hunter's titles, reasons or strength. Verdict accept or reject. | Candidates reviewed / total |
| Render | Vertical 1080×1920 (blurred fill, centred source), captions from the segment text burned in when a font is available, and an `.srt` sidecar. | FFmpeg output time / total clip time |
| QA | ffprobe checks size, video 1080×1920, audio, duration within 1 s. Passing clips move to `completed/<job>/`, failing ones to `failed/<job>/`. | Checks done / total |

Job states: Queued, Running, Blocked (storage limit), Failed, Cancelled, Completed. Elapsed time is measured per step; there is no ETA. A server restart marks a running job Failed ("interrupted"); Retry continues from the first unfinished step. The source in `archive/` is never deleted.

Caption timing is per segment (from measured pauses); inside a segment, lines are spread by text length. It is close, not word-exact.

## Human review and campaigns

Finished clips wait on **Review** for Approve or Reject. An approved clip can be downloaded, and when its job belongs to a campaign, a post can be recorded against that campaign (post URL, status Submitted / Accepted / Rejected, views and earnings when known). Nothing is posted or submitted automatically.

## Open points

- Bounded revision requests from the Reviewer are not implemented (accept or reject only).
- Storage limit: checked before import and render (blocks the job); no per-pool quotas or reservations yet.
- Browser uploads are not supported; drop files into `incoming/` or import an HTTPS URL.
