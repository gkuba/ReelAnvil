# ReelAnvil — Changelog

This is the **single maintained project change log**. Record user-visible changes by released version, not every failed build. Keep development outcomes in `BUILD_HISTORY.md`.

## Unreleased
- Project architecture, approved UI direction, deployment and licensing standards established.
- Repository documentation and governance scaffolding proposed; **not an implemented application feature**.
- Build 0.4 development candidate adds a Theme Manager for duplicating built-in themes, editing seven semantic colors, previewing drafts, saving custom themes, and refreshing valid theme configuration. This is an unreleased local prototype, not a production-ready theme recovery system.
- Build 0.5 development candidate reorganizes Settings into General, Themes, Connectors, and Paths & Storage pages; removes Tokyo Night in favor of Dracula Classic and Dracula Lavender; migrates saved Tokyo Night selections and custom-theme bases to Aurora; and uses inline SVG settings icons. Connector and storage controls remain disabled placeholders.
- Build 0.6 development candidate adds a persistent compact navigation rail with grouped flyouts, glass-style panels, expanded SVG navigation icons, and neutral secondary actions. Dracula Lavender uses a blue-black surface palette. Campaign workflow remains design-only.
- Build 0.7 development candidate consolidates Dracula around the stable `dracula-dark` ID, migrates prior Lavender selections and custom bases, darkens built-in surfaces, clarifies preview swatches, and derives active navigation accents from the selected theme. The unauthenticated prototype now enforces loopback binding and adds local request and response guardrails.
- Build 0.7.1 development candidate fixes same-origin Settings and Theme Manager submissions when the browser serializes Origin as `null`, while continuing to reject cross-site Fetch Metadata and mismatched origins. It adds shared-router regression coverage for all current settings POST routes.
- Build 0.8 development candidate adds an embedded SQLite persistence foundation with schema version 1 and a startup read/write probe; no workflow or secret tables are included.
- Build 0.9 local candidate upgrades SQLite to schema version 2 for campaign source records and separate user tracking data; adds manual campaign tracking, filters, statuses, a campaign overview, a local mountain banner, and per-theme icon choices. Content Rewards and Vyro discovery remain unavailable; no external campaign API, login, submission, or post search is used.

<!-- Add dated, verified user-facing changes here after successful builds; do not claim releases prematurely. -->
