# ReelAnvil security requirements

Security applies even to loopback and internal deployments. The current prototype is unauthenticated and loopback-only; do not expose it to a LAN, proxy, tunnel, or public network.

## Secrets and integrations

- Never store API keys, OAuth access/refresh tokens, session cookies, passwords, or encryption keys in plaintext configuration, source, examples, logs, diagnostics, release archives, or Git history.
- Ordinary nonsecret settings may remain in JSON. Connector credentials require encrypted storage at rest with keys managed separately; choose and threat-model the key management approach before implementing connectors. File permissions are defense in depth, not encryption.
- Use OAuth 2.0 with PKCE when supported; validate state, redirect URI, scopes and callbacks. Treat ReelAnvil login sessions separately from external provider credentials. Support expiry, rotation, refresh, revocation and reconnection; least-privilege scopes.
- Never place credentials in URLs or browser localStorage. Redact sensitive values in logs, error messages, diagnostics and exported support bundles. Avoid storing platform passwords or importing browser session cookies.
- Human approval remains required for publishing and campaign submissions; a connected account does not authorize automatic submission.

## Web and filesystem boundaries

- Preserve same-origin and CSRF defenses for every state-changing request; never disable a security check to fix a form bug. Confirm real browser request headers and add targeted tests before changing validation.
- Escape all untrusted HTML and attributes, avoid unsafe template rendering, validate input, keep the CSP restrictive, and defend against XSS, clickjacking, path traversal, unsafe uploads, SSRF and untrusted connector responses.
- Show users validation messages only; log internal database and I/O errors instead of displaying them.
- Maintain request limits, secure cookies when sessions are introduced, timeouts and restrictive response headers. Require authentication and authorization before expanding beyond loopback.
- Add automated secret scanning to local pre-commit/pre-push and CI; scan source, fixtures, generated outputs and release archives. `.gitignore` alone is not a control. Treat any committed secret as compromised: revoke or rotate it, and assess Git history and downstream copies.
- Audit dependencies and pinned external integrations. Document threat models, negative tests and recovery checks before production deployment.

## Current controls

- Listener refuses non-loopback addresses; requests with a non-local `Host` are rejected (DNS-rebinding defense).
- POSTs must be same-origin by Fetch Metadata and by Origin/Referer; `Origin: null` is accepted only with `Sec-Fetch-Site: same-origin`. Bodies are capped at 1 MiB.
- Headers: `Content-Security-Policy: default-src 'none'; img-src 'self'; style-src 'self'; script-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'`, plus `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`.
- Templates, CSS, JS and branding are embedded in the binary; nothing is fetched from other hosts. A test fails if a template contains inline scripts or styles.
- One-time status messages use a short-lived `HttpOnly`, `SameSite=Strict` cookie; links cannot inject page text.

## Regression checklist

- [x] Duplicate Theme succeeds from a real same-origin browser form on supported loopback hosts/ports and opens the editor.
- [x] Apply Theme succeeds from a real same-origin browser form and persists the active theme.
- [x] Every settings-changing POST route (including General and custom-theme editing): same-origin requests succeed; foreign-origin requests are blocked without state changes.
- [x] Cross-site, same-site-but-not-same-origin, mismatched Origin/Referer and mismatched ports are rejected without state changes.
- [x] Security headers and CSP checked in the browser; no `unsafe-inline`.
- [ ] Add a CSRF token strategy before any authenticated or LAN deployment.
- [ ] XSS tests cover custom theme names and external provider metadata (campaign titles are covered).
- [ ] Credential storage, redaction, session expiry/revocation, and secret scanning have automated tests before connectors ship.

## SQLite and connector secret boundary

- SQLite holds nonsecret application records only. Never persist OAuth tokens, API keys, session cookies, or encryption keys in plaintext SQLite, JSON, logs, diagnostics, release archives, or Git.
- Before implementing connectors, require encrypted credential storage with separate key management and a scoped token lifecycle.

## Campaign boundaries

- Campaign URLs and optional View / Apply / Submit targets accept HTTPS only, reject URL user information and nonstandard ports, and are never fetched by the server. External links open with `noopener noreferrer`.
- Campaign text is rendered through `html/template`; values and notes are validated and length-bounded.
- Discovery adapters have an interface boundary, but no Content Rewards or Vyro endpoint is called until its public API and permitted use are confirmed. ReelAnvil performs no scraping, login, OAuth, automatic application, post search or content submission.
- Automated checks cover unsafe URL schemes and user info, cross-origin campaign requests, method rejection, HTML escaping, schema upgrades, deduplication, untracking, and preservation of local notes and status during discovery updates.
