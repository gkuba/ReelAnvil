# Theme configuration and recovery checklist

**Status (2026-11-12):** file-level recovery and failure-injection checks are automated and passing (`internal/app/themes_recovery_test.go`, `themes_test.go`). The browser checks at the end are for the maintainer's visual review.

## Automated (passing)

- [x] Invalid JSON on startup reports the path and line; the last-known-good backup is used; the primary file is left byte-for-byte untouched; writes stay locked until repair (`TestInvalidPrimaryUsesBackupAndKeepsPrimaryBytes`).
- [x] Both files unusable: built-in themes stay available, nothing is overwritten, writes locked; after repair and refresh the error clears, all valid themes load and writes work (`TestBothFilesUnusableLeavesBuiltinsAndLocksWrites`).
- [x] Primary missing and backup valid: the primary is restored and the backup is unchanged (`TestMissingPrimaryRestoresValidBackup`). Primary missing and backup invalid: nothing is invented or replaced (`TestMissingPrimaryWithInvalidBackupIsNotInvented`).
- [x] A valid refresh updates the backup to a complete snapshot (`TestValidRefreshUpdatesBackupToSnapshot`).
- [x] Unsupported schema, reserved and duplicate IDs, invalid base, invalid color, empty, long and multi-line names are rejected without partial activation (`TestInvalidThemesRejectedWithoutPartialActivation`).
- [x] A read-only directory fails the save, keeps the primary unchanged and does not activate the new theme (`TestUnwritableDirectoryKeepsPrimaryAndReportsError`).
- [x] Saves are atomic, leave no temporary files, and write private (0600) files (`TestSaveLeavesNoTemporaryFilesAndPrivatePermissions`).
- [x] Custom themes survive a restart (`TestCustomThemesSurviveRestart`).
- [x] Duplicate IDs and copy names stay valid and unique at the length limits.

## Maintainer visual review (browser, desktop and 375 px)

- [ ] Theme Manager rejects bad hex, missing colors and bad names without changing the saved theme; Cancel restores every field; Save persists and survives reload.
- [ ] Contrast, keyboard navigation and focus indication across the built-in and a custom theme.

## Network exposure

ReelAnvil is for a trusted LAN only and has no login by design. It keeps the Host check, same-origin POST checks, the body cap, timeouts, security headers and the startup warning. Authentication is not planned.
