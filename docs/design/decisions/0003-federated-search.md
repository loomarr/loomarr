# 0003. Search is federated; Loomarr builds no index

- **Date:** 2026-07-13
- **Status:** accepted

## Context

Loomarr searches three corpora: the household library, TMDB and its own clip catalog. The first two
are already indexed by their owners, and the clip catalog is thousands of rows at household scale.

## Decision

Build no search index. `GET /v1/search` fans out to the media server's `SearchTerm` and TMDB's search
and discovery endpoints and merges the results; clips use `LIKE`.

## Consequences

- No index to build, migrate, keep consistent or back up, on either database.
- Results are only as good as each owner's search, and a missing connection removes that corpus.
- A full-text index stays out unless very large filler catalogs outgrow `LIKE` (#1576).
