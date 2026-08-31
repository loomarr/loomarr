# Database lifecycle certification — 2026-08-31

Issue: [#743](https://github.com/loomarr/loomarr/issues/743)

## Boundary and command

`make test-db-lifecycle` is the repeatable local certification command. It first runs the shared
SQLite/PostgreSQL store and backend-transition suites through `make test-pg`, then builds the real
Loomarr image and starts isolated Compose projects. Deployment behavior is driven through Traefik
and Loomarr's HTTP API. Tests use direct database access only to create a large source fixture,
observe that an interrupt occurred during a real copy, clear the disposable failed target before a
retry, and independently verify target fidelity.

The suite never drives the maintainer's live stack. Each scenario receives a unique Compose project,
host port, and volumes, then removes its containers and volumes during cleanup.

## Scenarios certified

| Scenario | Evidence |
| --- | --- |
| Fresh PostgreSQL deployment | The documented base Compose file plus PostgreSQL overlay boots on PostgreSQL, accepts an application write, survives a container restart, and retains the write. |
| SQLite to PostgreSQL | A SQLite deployment creates an admin, session, and title through HTTP; preflight and backup pass; atomic migration drains, copies, verifies, persists the target, and restarts on PostgreSQL; the session and title survive; a PostgreSQL write survives another restart. |
| Operator rollback | After successful migration, restoring the SQLite bootstrap selection restarts the same deployment on the intact source. The pre-migration title and session remain, while the PostgreSQL-only title is absent. |
| Interrupted copy and retry | A process is killed after the target schema appears during a 100,000-row copy. Restart returns to readable and writable SQLite. After the disposable partial target is cleared, the complete HTTP workflow retries successfully. Direct target checks find 100,001 titles, the expected schema version, one valid session-to-user relation, and no orphan sessions. |
| Target changes after preflight | A table added directly to the target after preflight makes the process-owned recheck fail. Loomarr reports the failure on SQLite and continues to read and write the source. |
| Concurrent write drain | A write admitted before migration is held in flight while migration begins. New admission closes, the in-flight request completes successfully, and its row is present after PostgreSQL starts. This certifies a bounded maintenance window rather than live dual-write behavior. |

The prerequisite `make test-pg` coverage also checks all migrated tables, logical values, foreign-key
topology, schema versions, source immutability, snapshot/locking behavior, and a test-only
`BLOB`/`BYTEA` probe. Loomarr's production schema currently has no binary column, so the probe keeps
that cross-dialect contract executable without inventing a production field.

## Defect found

The first real migration failed with HTTP 409 because the base Compose file exported
`DATABASE_URL=sqlite:///data/loomarr.db`. Environment configuration has deliberate precedence over
the persisted bootstrap selection, so that apparently harmless default pinned every SQLite Compose
deployment and disabled the supported in-app switchover.

The base Compose file now relies on Loomarr's identical built-in SQLite default without exporting the
key. The PostgreSQL overlay remains explicitly pinned for fresh PostgreSQL deployments, and the
Compose contract check fails if the base deployment regresses.

## Passing run

On 2026-08-31, `make test-db-lifecycle` passed locally with Docker 29.7.2:

- `internal/store`: 205.706s
- `internal/backendtransition`: 33.383s
- `internal/app`: 36.922s
- `internal/integration` real-image lifecycle suite: 98.070s

The full run left no lifecycle containers or volumes behind.
