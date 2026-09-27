# 0001. PostgreSQL and SQLite are both first-class stores

- **Date:** 2026-07-13
- **Status:** accepted

## Context

Most households want a single container with a file-backed database and nothing else to run. Some
already operate PostgreSQL and want Loomarr on it, or may later want more than one instance.

## Decision

Support both behind one `Store` interface, with one shared `database/sql` code path:
`modernc.org/sqlite` (pure Go, so the image stays cgo-free) and `pgx` through its stdlib shim. Dialect
differences are confined to per-dialect `goose` migrations and the `ClaimDue*` queries. One conformance
suite runs against both.

## Consequences

- Jobs could not use a Redis- or PostgreSQL-only queue; the job engine had to support SQLite.
- SQLite means exactly one instance. PostgreSQL permits replicas only with row claims
  (`FOR UPDATE SKIP LOCKED`) and the channel revision checks in [storage](../storage.md).
- Timestamps are stored as epoch integers to keep the schema dialect-neutral.
- An install can move from SQLite to PostgreSQL in the app; the reverse is a restore.
