# ReelAnvil licensing and distribution policy

**Status:** Pre-release policy. Copyright holder attribution and exact dependency notices must be confirmed before publishing.

## ReelAnvil application

ReelAnvil-owned application code is planned to be released under the MIT License; see `LICENSE`. The MIT license permits personal and commercial use, modifications, forks, and sale of copies, provided required copyright and license notices are retained. This includes earning money *using* ReelAnvil.

No federal copyright registration or trademark registration is required merely to publish the application. Original eligible work generally receives copyright protection automatically under U.S. law. Any paid registrations are deferred unless there is a later business reason.

## External software

ReelAnvil does not distribute third-party container images, model weights, or standalone executables. Our Compose manifests reference upstream images, and users retrieve third-party software directly from its providers. Each component remains subject to its own license and terms. See `THIRD_PARTY_NOTICES.md`.

ReelAnvil binaries may include compiled third-party Go libraries, and packaged frontend assets may also carry third-party obligations. Review and comply with those terms before release.

## Branding

The ReelAnvil name and official visual identity are distinct from the MIT-licensed code. Forks may identify their origins but must not falsely imply they are official or endorsed. See `TRADEMARKS.md`. This does not assert registered trademark status or guarantee exclusivity.

## Hosting and installation

GitHub is the planned authoritative source for repository code and versioned releases. A future custom domain may serve a static `install.sh` through GitHub Pages; the script will retrieve verified versioned ReelAnvil releases, not redistribute third-party images. A domain is optional.

## Before public release

- Confirm copyright-holder wording and contributor policy.
- Inventory linked Go/frontend dependencies and required notices.
- Verify upstream image sources, versions, and license obligations.
- Perform final name/conflict check; do not claim legal clearance from a preliminary search.
- Review installer security, version pinning, and checksums.
