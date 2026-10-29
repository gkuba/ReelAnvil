# ReelAnvil — Changelog

User-visible changes. Build and test records live in [`BUILD_HISTORY.md`](BUILD_HISTORY.md). No version has been released yet.

## Unreleased

### Build 0.9.2 (development)
- The web UI is served from embedded templates, one stylesheet and one script. Settings pages now match the Campaigns pages' headings, forms, cards and sidebar.
- Stricter Content Security Policy: no inline scripts or styles.
- Status messages after saving are one-time messages and can no longer be set through a link.
- Duplicating a theme keeps its real colours and a valid name (for example "Dracula (Copy)").
- Manual campaigns store View, Apply/Join and Submit links correctly; existing affected records are repaired automatically. Deadlines are stored as calendar dates.
- Clear messages when a campaign URL is already used, a campaign no longer exists, or a theme edit is invalid.
- `reelanvil serve --data-dir <dir> --addr <host:port>`, `reelanvil version`, and a more useful `reelanvil doctor`.

### Builds 0.1–0.9.1 (development)
- Local web interface with Overview, Campaigns, Settings (General, Themes, Connectors, Paths & Storage), Diagnostics and placeholder pages.
- Five built-in themes (Aurora, Dracula, Tomorrow Night, Nocturne, Foundry), custom themes with backup recovery, and a global application icon.
- Persistent compact navigation rail with flyouts; responsive layout.
- SQLite storage; manual campaign tracking with statuses, notes, filters and an Overview summary. Campaign discovery is unavailable by design.
- Loopback-only server with local request guards.
