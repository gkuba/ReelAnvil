# ReelAnvil — Agent Operating Rules

**Status:** Project governance, pre-implementation. Read with `AGENTS_CHECKLIST.md`.
**Scope:** ReelAnvil development, review, documentation, releases, and ChatGPT ↔ Work handoffs.

## Authority and evidence
- The latest approved `docs/REELANVIL_BUILD_STANDARDS.md` governs product and architecture. Decisions recorded there override older drafts. Never silently replace a confirmed decision with an assumption.
- GitHub's designated branch and commit are the **authoritative verified source baseline** for ChatGPT development. The user's local repository is expected to be synchronized after Work's successful verified build and push.
- `docs/STATUS.md` is the single current-state register. `docs/CHANGELOG.md` records user-visible changes, and `docs/BUILD_HISTORY.md` records build/test/commit outcomes. These are maintained **in place**, not multiplied into per-chat files.
- A Library sync checkpoint records the GitHub repository/branch/SHA, documentation revision, synchronization date, and evidence; it is not itself proof of GitHub state.
- Distinguish verified evidence, inherited context, proposed changes, and unknowns. Never claim a build, test, integration, commit, or deployment occurred without evidence.

## Development handoff and change control
1. ChatGPT independently verifies GitHub and the Library checkpoint before source-dependent changes. No source patches against an unverified or conflicting baseline.
2. ChatGPT prepares narrowly scoped `.patch` files and a `HANDOFF.md` with exact baseline SHA, purpose, affected paths, prerequisites, test instructions, risks, expected results, and rollback guidance. Supply hashes/manifest where useful.
3. Work applies patches to the local repository, inspects diffs, builds, runs relevant tests, performs real deployment/SSH checks where authorized, and reports failures. No claiming success based only on a patch or compilation.
4. Only after a successful **verified** build does Work commit and push to GitHub. It returns the branch, commit SHA, test/build evidence, and a ZIP of actual committed changes/logs for review.
5. ChatGPT checks the returned evidence and GitHub SHA, then updates `docs/STATUS.md`, `docs/CHANGELOG.md`, `docs/BUILD_HISTORY.md`, and the Library checkpoint as appropriate. A generated patch or unpushed local build is **not** completed work.
6. Do not overwrite unrelated local changes, secrets, user data, or generated media. Escalate discrepancies before proceeding. Failed attempts belong in build history/status where relevant, not the release changelog.

## Local Work build, deployment, and GitHub handoff
- The local checkout at `/home/katala/Projects/ReelAnvil` is the authoritative working source. GitHub `main`/`dev` branches are mirrors and shared checkpoints for Chat; the Chat Global Library is a separate verified copy. The maintainer directs work and approves releases.
- Before source changes, inspect the working tree and preserve a dated source snapshot/manifest when the change warrants recovery. Never overwrite unrelated or uncommitted work. Keep generated build/test output out of maintained source and commits unless explicitly required.
- Build and validate locally, preserve relevant logs and artifact hashes outside the maintained source, and distinguish source review, compilation, automated tests, deployment, and user validation. Never claim a later stage from an earlier one.
- Remote deployment or installation is performed only after the maintainer explicitly authorizes access. Do not copy builds to a live host or install them there before that authorization.
- Push only the verified commit to the intended GitHub branch after confirming its destination and branch state. GitHub is a mirror/checkpoint, not the local source of truth.
- **After every GitHub push, prepare a complete commit ZIP and handoff for the maintainer to give Chat.** Store these under `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/deliverables/`. Include the exact pushed tracked tree as a ZIP, a SHA-256 checksum for the ZIP, a per-file manifest with paths/sizes/Git blob IDs and SHA-256 values, and a `HANDOFF.md` identifying repository, branch, full commit SHA, tracked-file count, ZIP checksum, scope, and honest build/test/deployment evidence. Verify ZIP integrity and local commit/tree identity before delivery. Do this for development pushes as well as release pushes.
- Keep only the current package in `deliverables/`; move prior complete packages together into `deliverables/archive/`. Retain the previous five revisions. Before pruning an older revision, verify the current package and five newest archives are intact; never delete ambiguous or irreplaceable evidence.
- Release builds, squash/merge to `main`, and creation of the next `dev` branch follow the maintainer's approved release workflow. Development build/push does not itself authorize deployment, release, or publication.

## Engineering boundaries
- One Go codebase, separate `reelanvil serve`, `reelanvil worker`, `reelanvil doctor` processes (names provisional); web server never receives Docker socket access. Narrow Docker API access belongs only to authenticated worker.
- Debian 13 Linux-first; Windows/macOS experimental until tested. SQLite persistent state; explicit migrations, backup and recovery.
- Upstream-only Docker images, pinned after validation; no custom images, no bundled third-party containers, standalone executables, or model weights. Audit compiled Go/frontend dependencies.
- Media ingestion must be authorized; originals preserved; isolation by job ID; human approval before posting; AI candidate selection and independent AI review have separate contexts.
- Real stage progress, elapsed time and logs; **never show ETA** or fabricated percentages. Storage caps and reservation accounting must block jobs safely instead of silently deleting data.
- Full browser-configurable onboarding, diagnostics, services, queue, review, campaigns, storage, backups, and responsive themed UI. Aurora Nordic-industrial design is foundational, not an optional polish pass.
- No required paid runtime services; no claims of guaranteed earnings. No private credentials or campaign/media content in public source, logs, examples, or release archives.

## Documentation and release packaging
- Repository root documentation exceptions: `AGENTS.md` and `AGENTS_CHECKLIST.md` only. All other documentation belongs in `docs/` (including build standards, status, history, architecture, design, installation and release guides).
- All licensing files belong in `docs/licensing/` and **must be packaged in every distributed release**: `LICENSE`, `LICENSING.md`, `THIRD_PARTY_NOTICES.md`, `TRADEMARKS.md`, plus any dependency-specific notices required by audit. License copyright holder remains to be confirmed before release.
- Other `docs/` materials (including `STATUS.md`, `CHANGELOG.md`, `BUILD_HISTORY.md`, design notes, internal handoff records, build standards) are **not** bundled in runtime release artifacts. Publish appropriate user docs on GitHub/Pages separately if desired.
- Packaging must have an explicit allowlist and automated assertions: required license files present; internal status/history, secrets, logs, databases and media absent. License audit and release verification are mandatory gates.
- Preserve existing file history in Git; do not create duplicate per-build status or history documents. Update the current files and commit them with successful verified changes.

## Startup discipline
- New development chats first provide an initial categorized confidence report **before** reading project sources, then fully read both agent files and execute the checklist, then report updated categorized confidence and verification results.
- Initialization is read-only. Missing docs, inaccessible GitHub, absent repository/checkpoint or unverified local sync must be stated plainly. Discussion may proceed; source-dependent patching must wait for verified baseline.
- `/ReelAnvil/` is the ChatGPT Global Library folder for this project. Library copies mirror decisions and checkpoints; GitHub remains authoritative for code.

## Deliverables and archive retention
- Store the **current** ChatGPT-to-Work handoff package in Global Library `/ReelAnvil/deliverables/`; move prior completed/superseded handoffs into `/ReelAnvil/deliverables/archive/`.
- A revision means one complete handoff package (patches, HANDOFF instructions, manifests and supporting evidence), **not** each individual file.
- Retain at most **five older handoff revisions** in the project archive. Before pruning a sixth, confirm the current handoff and newer archived revisions are preserved. Delete only verified superseded handoff packages, never source history, build history, project documentation or irreplaceable evidence.
- If exported/linked deliverables appear at the Global Library root, organize ReelAnvil copies under `/archive/ReelAnvil/`, likewise retaining at most five older handoff revisions there.
- Provide a usable downloadable handoff file/attachment separately; a Global Library path alone is not a download link.
- Keep these folders clean as part of every new handoff, report any move/delete restrictions, and do not silently delete ambiguous files.
