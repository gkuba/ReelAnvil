# Pre-release checklist — Theme configuration and recovery

**Status:** Broader recovery and failure-injection checks are **Not Tested — Pre-release**. The maintainer controls when this checklist is executed. This build enforces loopback-only access, but do not expose the unauthenticated prototype to another host.

Build 0.4's happy-path duplicate/edit/save/refresh and active-theme persistence were checked locally. The automated suite also checks basic invalid hex rejection and line-numbered JSON syntax parsing. These checks do not complete the recovery checklist below.

## Primary file parsing and preservation

- [ ] **Not Tested — Pre-release:** Invalid JSON on startup reports the path and useful line/column; app remains usable with last-known-good backup.
- [ ] **Not Tested — Pre-release:** Invalid JSON on Refresh Themes leaves the primary file byte-for-byte untouched, logs the error and identifies the path in the UI.
- [ ] **Not Tested — Pre-release:** Unsupported schema, invalid theme object, duplicate/reserved IDs, invalid base, invalid color, invalid name, and malformed permutations are all rejected without partial activation.
- [ ] **Not Tested — Pre-release:** When both primary and backup are unusable, built-in themes remain available; theme writes stay disabled until repair and successful refresh.
- [ ] **Not Tested — Pre-release:** After repair and refresh, the app clears the error and enables custom theme writes without losing unrelated valid themes.
- [x] **Basic unit coverage passed in B0.4:** When the primary file is absent and the backup is valid, startup restores the primary from the backup without changing the backup. Full startup/recovery scenarios remain **Not Tested — Pre-release**.

## Backup and last-known-good recovery

- [ ] **Not Tested — Pre-release:** Valid backup is used after invalid primary; verify the selected built-in/custom theme behavior and all loaded values.
- [ ] **Not Tested — Pre-release:** Invalid backup is detected and does not replace or overwrite the invalid primary.
- [ ] **Not Tested — Pre-release:** Missing backup is handled safely and visibly; determine whether recovery is possible without silently inventing data.
- [ ] **Not Tested — Pre-release:** Saving/refreshing valid primary updates backup only after validation; backup content is a complete last-known-good snapshot.
- [ ] **Not Tested — Pre-release:** Recovery does not silently discard custom themes or unexpectedly switch an active theme.

## Filesystem and atomic writes

- [ ] **Not Tested — Pre-release:** Interrupted primary write leaves either the prior complete file or the new complete file, never truncated JSON.
- [ ] **Not Tested — Pre-release:** Interrupted backup write keeps a recoverable primary and reports the failure clearly.
- [ ] **Not Tested — Pre-release:** Read-only directory, unwritable file, disk-full, and rename/sync failures are surfaced; theme saving remains safely locked until repair.
- [ ] **Not Tested — Pre-release:** Temporary files are cleaned up when safe, and permissions on directories/files are appropriate for the supported deployment model.

## Theme manager and UI regression

- [x] **Unit-tested in B0.9.2:** Duplicate and collision-generated IDs stay valid and unique at boundary lengths, and copy names stay within 64 characters; reserved built-in IDs are rejected by validation.
- [ ] **Not Tested — Pre-release:** UI rejects malformed hex, missing colors, empty/oversized/newline names, and invalid edits without changing the saved theme.
- [ ] **Not Tested — Pre-release:** Cancel restores every saved field and does not persist draft data; Save persists all fields and survives reload/restart.
- [ ] **Not Tested — Pre-release:** Refreshing manually edited valid JSON updates available themes, active theme styling, editor fields, and backup consistently.
- [ ] **Not Tested — Pre-release:** Contrast, keyboard navigation, focus indication, mobile layout, and all built-in/custom CSS tokens regressions are reviewed across supported screen sizes.

## Security gate before non-loopback exposure

- [x] **B0.7 local guard implemented and unit-tested:** Loopback-only listener validation, Host checks, same-origin/Fetch Metadata checks for browser POSTs, 1 MiB POST-body cap, server timeouts, and basic response security headers.
- [ ] **Not Tested — Pre-release:** Complete a CSRF threat review, including browsers that omit Origin, Referer, and Fetch Metadata; implement authenticated session-bound tokens before any non-loopback bind is allowed.
- [ ] **Not Tested — Pre-release:** Review authentication/authorization and error/log data exposure before any shared or public network use.
- [x] **B0.7 unit-tested:** Default and configured listener addresses are restricted to loopback; non-loopback bind attempts fail.
- [ ] **Not Tested — Pre-release:** Network access may be enabled only after authentication, authorization, CSRF, TLS/termination, and explicit threat review are complete.

## Theme acceptance across restart
- [ ] Custom theme edit/save/reload survives restart.
- [ ] Applied theme and duplicate persist across restart.
