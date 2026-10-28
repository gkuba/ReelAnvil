# ReelAnvil security requirements (approved direction; implementation pending)

Security applies even to loopback/internal deployments. The current UI prototype is unauthenticated and loopback-only; do not expose it to LAN, proxy, tunnel, or public networks.

## Secrets and integrations

- Never store API keys, OAuth access/refresh tokens, session cookies, passwords, or encryption keys in plaintext configuration, source, examples, logs, diagnostics, handoff archives, or Git history.
- Ordinary nonsecret settings may remain in JSON. Connector credentials require encrypted storage at rest with keys managed separately; choose and threat-model the key management approach before implementing connectors. File permissions are defense in depth, not encryption.
- Use OAuth 2.0 with PKCE when supported; validate state, redirect URI, scopes and callbacks. Treat ReelAnvil login sessions separately from external provider credentials. Support expiry, rotation, refresh, revocation and reconnection; least-privilege scopes.
- Never place credentials in URLs or browser localStorage. Redact sensitive values in logs, error messages, diagnostics and exported support bundles. Avoid storing platform passwords or importing browser session cookies.
- Human approval remains required for publishing and campaign submissions; a connected account does not authorize automatic submission.

## Web and filesystem boundaries

- Preserve same-origin and CSRF defenses for every state-changing request; do not disable a security check to fix the Duplicate Theme regression. Confirm real browser request headers and add targeted tests before modifying validation.
- Escape all untrusted HTML and attributes, avoid unsafe template rendering, validate input, apply a restrictive CSP, defend against XSS, clickjacking, path traversal, unsafe uploads, SSRF and untrusted connector responses.
- Maintain request limits, secure cookies when sessions are introduced, timeouts and restrictive response headers. Require authentication and authorization before expanding beyond loopback.
- Add automated secret scanning to local pre-commit/pre-push and CI; scan source, fixtures, generated outputs and packaged handoffs. `.gitignore` alone is not a control. Treat any committed secret as compromised: revoke/rotate it and assess Git history and downstream copies.
- Audit dependencies and pinned external integrations. Document threat models, negative tests and recovery checks before production deployment.

## Regression checklist

- [ ] Duplicate Theme succeeds from a real same-origin browser form on supported loopback hosts/ports and opens the editor.
- [ ] Apply Theme succeeds from a real same-origin browser form and persists the active theme.
- [ ] Inventory and verify every settings-changing POST route (including General and custom-theme editing): same-origin mutation succeeds; foreign-origin mutation is blocked without state changes.
- [ ] Cross-site, same-site-but-not-same-origin, mismatched Origin/Referer and mismatched ports are rejected without state changes.
- [ ] All theme-changing POST endpoints remain protected; add CSRF token strategy before authenticated/LAN deployment.
- [ ] XSS tests cover custom theme names and external provider metadata.
- [ ] Credential storage, redaction, session expiry/revocation, and secret scanning have automated tests before connectors ship.
- [ ] Security header and CSP checks run in browser; avoid broad `unsafe-inline` allowances in production.
