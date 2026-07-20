# Changelog

All notable changes to Loomarr are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html) once it reaches
its first tagged release.

A release is cut by pushing a `vX.Y.Z` tag; the release workflow then publishes
multi-arch `ghcr.io/mantonx/loomarr` images (`:X.Y.Z` and `:X.Y.Z-filler`).

## [Unreleased]

Pre-1.0. Loomarr is built in verifiable phases (0–14, see `PROGRESS.md`); the
core is feature-complete and the automated Definition of Done is green. Highlights
of what exists today:

### Added
- Natural-language channel intent → grounded LLM lineup (Ollama or any
  OpenAI-compatible provider) → acquisition via Seerr → scheduling with
  commercial pods → push to Tunarr, with event- and sweep-driven backfill.
- SQLite and Postgres backends behind one store conformance suite.
- Embedded Vite/React SPA + in-app Help, served offline from a single binary.
- Prometheus `/metrics` covering the design's §17 observability set.
- First-class Docker images (distroless `loomarr:latest`; `loomarr:filler` with
  vendored yt-dlp/ffmpeg/deno for in-app clip ingest).

_The first tagged release will snapshot this section under its version._
