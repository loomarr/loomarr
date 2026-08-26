# Changelog

Notable Loomarr changes are summarized here. The authoritative, generated history for every
published beta is [GitHub Releases](https://github.com/loomarr/loomarr/releases); it includes the
exact commit, artifacts, signatures, SBOM, and provenance for each tag. The release workflow cuts
signed multi-architecture `ghcr.io/loomarr/loomarr` images from successful `main` builds.

## [Unreleased]

Changes on `main` that have not yet shipped belong here. `PROGRESS.md` carries current initiative
status; pull requests and GitHub Releases carry the durable delivery record.

## [v0.1.0-beta.1] — 2026-08-18

The first public beta established the installable appliance described below. Later beta fixes and
incremental capabilities are recorded in [GitHub Releases](https://github.com/loomarr/loomarr/releases).

### Added

- Natural-language channel intent → grounded LLM lineup (Ollama or any
  OpenAI-compatible provider) → acquisition via Seerr or Sonarr/Radarr → scheduling
  with commercial pods, with polling-driven backfill as titles land.
- **Loomarr plays out its own channels** — it encodes and serves a tuner and guide
  the media server picks up as Live TV. Tunarr remains fully supported as an
  alternative backend, chosen in the wizard and overridable per channel.
- ChannelPolicy: audience ceilings that fail closed, separation and no-repeat
  windows, ordering modes, seasonality, and a relaxation ladder that degrades
  quality rather than safety.
- SQLite and Postgres backends behind one store conformance suite.
- Embedded Vite/React SPA and in-app Help, served offline from a single binary.
- Prometheus `/metrics` covering the design's §17 observability set.
- A single multi-arch Docker image carrying ffmpeg, yt-dlp, deno and whisper, with
  hardware encoding auto-detected by trial rather than assumed.

### Fixed

- `docker/compose.yaml` never mounted the data volume onto the app service, so a
  SQLite install's database lived in the container's writable layer and was
  destroyed by any `pull` or `--force-recreate`.
- The compose healthcheck invoked a `healthcheck` subcommand that did not exist, so
  every container reported `unhealthy` for its entire life and anything gating on
  `service_healthy` waited forever.
- The release workflow built an image variant against a Dockerfile stage that does
  not exist, which would have failed the first tagged release.

[Unreleased]: https://github.com/loomarr/loomarr/commits/main
[v0.1.0-beta.1]: https://github.com/loomarr/loomarr/releases/tag/v0.1.0-beta.1
