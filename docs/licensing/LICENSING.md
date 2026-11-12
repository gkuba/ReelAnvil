# ReelAnvil licensing and distribution policy

**Status:** Release policy for build 0.9.2. Copyright holder is "ReelAnvil contributors" (see `LICENSE`); the dependency audit is in `THIRD_PARTY_NOTICES.md`.

## ReelAnvil application

ReelAnvil-owned application code is released under the MIT License; see `LICENSE`. The MIT license permits personal and commercial use, modifications, forks, and sale of copies, provided required copyright and license notices are retained. This includes earning money *using* ReelAnvil.

No federal copyright registration or trademark registration is required merely to publish the application. Original eligible work generally receives copyright protection automatically under U.S. law. Any paid registrations are deferred unless there is a later business reason.

## External software

ReelAnvil does not distribute third-party container images, model weights, or standalone executables. Our Compose manifests reference upstream images, and users retrieve third-party software directly from its providers. Each component remains subject to its own license and terms. See `THIRD_PARTY_NOTICES.md`.

ReelAnvil binaries may include compiled third-party Go libraries, and packaged frontend assets may also carry third-party obligations. Their notices are reproduced in `THIRD_PARTY_NOTICES.md`, which ships in every release archive.

## Branding

The ReelAnvil name and visual identity are distinct from the MIT-licensed code. Forks may say where they came from but must not claim to be the official project. See `TRADEMARKS.md`. No trademark search or registration is planned.

## Hosting and installation

GitHub is the planned authoritative source for repository code and versioned releases. A future custom domain may serve a static `install.sh` through GitHub Pages; the script will retrieve verified versioned ReelAnvil releases, not redistribute third-party images. A domain is optional.

## Release checks

`scripts/package-release.sh` builds the archive from an allowlist (binary, `README.md`, `licensing/`) and `scripts/check-release.sh` verifies it: license files present, no status or history docs, databases, logs, tokens or media, no secret-like text, and a binary that runs. `scripts/scan-secrets.sh` scans the tracked files. CI runs both. Installer images: Ollama and Whisper are pinned by digest; the media image tracks `:latest` so yt-dlp stays current.
