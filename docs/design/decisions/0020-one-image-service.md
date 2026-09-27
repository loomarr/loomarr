# 0020. One content-addressed image service

- **Date:** 2026-08-09
- **Status:** accepted
- **Supersedes:** per-source image handling

## Context

Loomarr showed images from four sources with four storage models: channel icons as database blobs,
clip stills and hover loops on disk, and TMDB posters hot-linked from the operator's browser. One
hard-coded 500 px poster served every surface, third-party origins loaded in the operator's browser
(the beacon the clip-thumbnail builder already refused), and no still had a modern format.

## Decision

One service, `internal/images`, owns ingest, storage, derivatives and serving for every image, keyed
by content hash. Fetching a remote URL re-keys it to the hash of its bytes. Go owns identity, policy
and serving; a required Rust worker owns every pixel operation, with no Go codec fallback.

## Consequences

- Every surface gets responsive derivatives and modern formats from one pipeline.
- The operator's browser never loads a third-party image origin.
- Visibility, security, caching and restore behaviour are properties of the image, decided once.
- The image worker is a required runtime component with its own certification.
