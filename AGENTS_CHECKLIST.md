# ReelAnvil — Agent Initialization and Verification Checklist

**Use after Stage 1 initial confidence; initialization is read-only.**
Mark each check verified, not applicable (with reason), missing, or blocked. Do not infer completion from memory or filenames.

## 1. Mandatory agent documents
- [ ] Locate `/ReelAnvil/AGENTS.md` and `/ReelAnvil/AGENTS_CHECKLIST.md` in the Global Library.
- [ ] Read **both completely**; disclose if either is inaccessible or only partially read.
- [ ] Resolve which copy/revision is current if Library and repository differ; do not silently choose.

## 2. Authoritative documentation and decisions
- [ ] Read the latest `REELANVIL_BUILD_STANDARDS` completely (repository: `docs/REELANVIL_BUILD_STANDARDS.md`; Library versioned master if repository not initialized).
- [ ] Review approved UI direction and design reference, architecture and integrations, platform constraints, service/container choices, storage lifecycle and caps, job progress/no-ETA policy, independent AI review, human publishing approval.
- [ ] Review `docs/licensing/` policies and current license/dependency-audit status.
- [ ] Review `docs/STATUS.md`, `docs/CHANGELOG.md`, `docs/BUILD_HISTORY.md` and relevant design/architecture/test/release documentation.
- [ ] Identify completed vs planned features, blockers, open decisions, current development priority and last successful verified build.
- [ ] Check whether documentation reflects recent approved decisions; record gaps without modifying during startup.

## 3. GitHub source baseline — mandatory independent check
- [ ] Find the **explicit** repository URL, target branch and latest synchronization checkpoint; do not invent either.
- [ ] Independently inspect GitHub branch HEAD commit SHA and relevant tree/source structure, using GitHub access available to this chat.
- [ ] Compare actual GitHub HEAD SHA and branch with the latest Library checkpoint and `docs/STATUS.md`.
- [ ] Confirm the latest successful Work build/test/push evidence and SHA in `docs/BUILD_HISTORY.md` if present.
- [ ] Record matching, divergent, absent or inaccessible baseline; if mismatch, investigate before preparing source patches.
- [ ] Do **not** infer local repository state from GitHub alone. Local/GitHub sync is an expected Work invariant, not independently proven without local evidence.
- [ ] If the repository has not yet been created, mark source verification **not yet possible**, not successful.

## 4. Workflow and release readiness
- [ ] Confirm Work handoff contract: patch + HANDOFF, local apply/build/test, verified commit/push, evidence ZIP, ChatGPT review, Library checkpoint update.
- [ ] Check planned vs tested upstream Docker images and external model/service endpoints; no unsupported deployment claims.
- [ ] Check release packaging policy: only ReelAnvil artifacts and `docs/licensing/` notices bundled; other docs excluded; no third-party images/models/executables bundled.
- [ ] Identify unresolved license copyright holder, dependency notices, trademark conflict check and installer verification as release gates where applicable.

## Local Work process
- [ ] Confirm local checkout is the working source of truth and GitHub is its mirror/shared checkpoint; preserve unrelated changes.
- [ ] For builds, retain relevant logs and artifact hashes outside maintained source; report build, tests, deployment, and user validation as separate evidence stages.
- [ ] Do not deploy/install remotely without explicit maintainer authorization.
- [ ] After **every push**, prepare a commit ZIP, ZIP SHA-256, per-file path/size/Git-blob-SHA-1/SHA-256 manifest, and `HANDOFF.md` for Chat under `/home/katala/Documents/ChatGPT Outputs/ReelAnvil/deliverables/`.
- [ ] Verify package integrity and that its tracked contents match the exact pushed commit; keep current package in `deliverables/`, archive older complete revisions under `deliverables/archive/`, retaining the prior five.
- [ ] Keep generated output and temporary files out of maintained source/commits unless required; release-only merge and branch reset/recreation require the maintainer's release approval.

## 5. Final report
- [ ] Report updated 0–100% confidence by the **same seven categories** as Stage 1: goals/requirements; architecture/integrations; workflow; source baseline; status/priorities; documentation/decisions; overall understanding.
- [ ] State explicitly whether both agent files were fully read.
- [ ] Give GitHub repository, branch and SHA actually verified, or explain why not.
- [ ] State checkpoint match/mismatch/not available and any missing documents, conflicts, or knowledge gaps.
- [ ] Keep reports concise and honest. Confidence means understanding; verification and readiness are separate.

**Stop rule:** No source-dependent patch generation when the GitHub baseline is inaccessible or inconsistent. Planning and documentation discussion may continue with clear limitations.
