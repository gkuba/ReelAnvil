# Third-party software and notices

**Status:** Pre-release template; complete dependency audit pending.

ReelAnvil intends to distribute only its own application, source, configuration, documentation, and installation tooling. Docker Compose references third-party container images obtained directly from their upstream registries. Third-party model weights, services, and executable tools are not bundled, mirrored, or repackaged by ReelAnvil.

External software is owned and licensed by its respective authors. Referencing it in Compose or downloading it from upstream does not make it part of the ReelAnvil MIT license.

## Planned external integrations (verify before release)

| Component | Integration/distribution approach | License verification |
| --- | --- | --- |
| modernc.org/sqlite v1.36.3 and runtime Go modules | Pure-Go SQLite driver linked into the ReelAnvil binary; versions are pinned in `go.mod` and `go.sum` | Initial source review completed for B0.8; compiled modules include BSD-style and MIT notices. Preserve applicable copyright/license text in distributed binary materials; complete automated license inventory before release. |
| Ollama | Pull official upstream container or connect to an existing endpoint | Pending version-specific audit |
| faster-whisper / Wyoming service | Pull a validated upstream container | Pending image and dependency audit |
| FFmpeg | Obtain from an upstream package/image | Pending build/license audit |
| yt-dlp | Obtain from upstream package/image | Pending version-specific audit |
| AI models | User obtains from model provider | Pending model-specific terms |

## Bundled dependencies

Even when no external containers or executables are redistributed, compiled Go modules, frontend libraries, fonts, icons, and other assets included in ReelAnvil releases may require attribution or license notices. Inventory exact versions and their terms before the public release. Do not claim this template is a completed license audit.
