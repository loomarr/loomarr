# Changelog

All notable changes to Loomarr are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html) once it reaches
its first tagged release.

A release is cut by pushing a `vX.Y.Z` tag; the release workflow then publishes a
multi-arch `ghcr.io/mantonx/loomarr` image. **No tag has been cut yet**, so nothing
is published and the compose file builds from source.

## [Unreleased]

Pre-1.0. The core is feature-complete and the automated Definition of Done is green;
`PROGRESS.md` carries phase status. Highlights of what exists today:

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

_The first tagged release will snapshot this section under its version._
