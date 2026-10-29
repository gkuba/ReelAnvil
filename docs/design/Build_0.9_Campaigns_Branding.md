# Build 0.9 design decisions — approved for implementation

## Navigation
Overview: purple mountain hero banner, then compact Campaign Overview with Available / Tracked / Ending Soon and a short tracked-campaign table. Campaigns: Available and Tracked views, manual URL entry, local tracking, editing, filtering, sorting, provenance and freshness. Diagnostics: operational health only.

## Source model
Content Rewards (Whop-associated) and Vyro are initial connector identities. A documented public API for campaign discovery/enrollment/submission has **not** been established; do not assume one. Available count represents known discovered records, not all campaigns on external platforms. Manual records must be clearly labeled.

## Local tracking
Separate discovered platform data from local tracked-user data so refresh never overwrites local notes/status. Distinct user status Interested / Applied / Active / Completed / Archived, and external platform status only when verified. Prevent duplicate tracking by platform+stable ID or canonical URL. Display unknown budget/payout/deadline as unknown, not zero. Ending Soon only for known deadlines.

## External links
View Campaign opens original page; Apply/Join and Submit Content open verified corresponding URLs when supplied. Never invent endpoints. HTTPS only, safe external-tab behavior, Copy Link. No automatic submission.

## Branding
Default Blue-Violet Film Glow, optional Amethyst Glow and Molten Orange, configurable as one global icon choice under Settings → Themes, independent of interface theme. Purple mountain header on Overview. All assets local. Icon selection persists and updates sidebar/favicon. Generated source sheets require icon-only extraction and maintainer review; original orange prepared icon pack is included as an additional reference.

## Out of scope
OAuth, scraping undocumented authenticated endpoints, campaign application automation, social post matching, payouts, media processing, Docker worker orchestration, remote deployment.
