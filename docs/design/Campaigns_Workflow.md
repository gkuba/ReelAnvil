# Campaign discovery, tracking and fulfillment — deferred design

**Status:** Approved product direction. Build 0.9 implements local campaign records, manual tracking, and source/user-state separation. Platform discovery, credentials, applications, social matching, and fulfillment remain unimplemented. Verify each provider's API and permissions before integration.

## Discovery versus tracking

- The Campaigns landing page presents available campaigns from connected campaign platforms, including available requirements, time frame, and budget/remaining budget where provided by the service.
- Users can select one or multiple campaigns and explicitly add them to a **local tracked list**. Discovery alone never enrolls a campaign in tracking.
- Only tracked campaigns participate in post matching or fulfillment checks. Untracking a campaign changes ReelAnvil's local tracking state only; it does not cancel or delete the external campaign.
- Track external campaign identifiers, requirements, required platforms and post types, start/end/deadline, budgets and remaining budget when available, submission status, and local-to-external associations. Treat unavailable values as unknown rather than inventing them.
- Build 0.9 currently supports manual HTTPS campaign URLs for Content Rewards and Vyro. No public discovery endpoint has been verified, so the Available view reports discovery unavailable and makes no network calls. Manually adding a campaign starts local tracking as Interested; users can edit, untrack, and later re-track it. Untracking preserves the source and prior local notes/status. Amounts and deadlines remain unknown unless entered.
- Source-owned campaign fields and local tracked status/notes are stored in separate SQLite tables. Future discovery refreshes update source fields without replacing local status or notes. Canonical HTTPS URLs and platform/external IDs prevent duplicate source records.

## Publishing and post matching

- When a user publishes through ReelAnvil and associates the publishing job with a tracked campaign, capture the platform's post ID, URL, and available default thumbnail automatically. Do not perform a broad search to find the post.
- When a user publishes manually, do **not** search automatically. The tracked campaign's **Refresh Posts** action explicitly checks connected social accounts/platforms relevant to that campaign for potential matches.
- Display discovered candidates in the tracked campaign listing with platform, thumbnail, URL, and matching requirement. If several posts may match, require user selection.
- A matched post is **not** a submitted or accepted fulfillment. The user explicitly clicks **Update Campaign** to submit the selected URL(s), with review/confirmation. Where no supported API exists, offer a manual submission path rather than pretending a successful automated submission.
- Distinguish discovered, ready for review, submitted, accepted, rejected, failed and unknown status; prevent duplicate submissions where possible. Persist external response IDs when available.

## Architecture and constraints

- Settings > Connectors owns campaign-platform and social-account credentials, permissions, connection tests and synchronization configuration; Campaigns owns discovery, tracking, post associations and fulfillment review.
- Model campaigns, requirements, social posts and submissions separately. A post may satisfy more than one eligible requirement without duplicating the post record.
- No automatic campaign submission. No automatic social search merely because a campaign is tracked. Avoid scanning unrelated social accounts or campaigns.
- Follow the existing human approval before publishing rule. Credential handling, API scopes, pagination, rate limits, consent, retries, deletion semantics and data retention require separate implementation design and verification.
