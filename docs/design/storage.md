# Storage

Formerly `design.md` §5 (store, retention, Postgres concurrency, SQLite → PostgreSQL) and §16's backup
section. The database is the product: channels, tags, proposals and the audit trail live there.

## One store, two backends

One `Store` holds provisioning, scheduling, filler, users, jobs, proposals and settings. SQLite and
PostgreSQL are both first-class (decision [0001](decisions/0001-postgres-and-sqlite.md)); the store
code path is shared through `database/sql`.

- **Selection:** the `DATABASE_URL` scheme, `sqlite:///data/loomarr.db` or `postgres://…`. An unknown
  scheme fails at boot.
- **SQLite** uses `modernc.org/sqlite` (pure Go, no cgo), opened with WAL and `busy_timeout`.
  **PostgreSQL** uses `pgx` through its `database/sql` shim.
- **Schema:** `goose` migrations embedded in the binary, one directory per dialect so dialect DDL never
  leaks; they run at startup. SQL stays ANSI where it can (`INSERT … ON CONFLICT … DO UPDATE` works on
  both). Migrations are forward-only.
- The shared SQLite/PostgreSQL conformance suite is the contract for both backends.

## Concurrency on PostgreSQL

SQLite means exactly one instance. PostgreSQL permits replicas, which changes what correctness needs:

- **Claims use `SELECT … FOR UPDATE SKIP LOCKED`** (`ClaimDueTitles` and the other `ClaimDue*`), so
  two replicas never both fire a retry or give-up. On SQLite the same method is a plain query.
- **Every channel-row mutation advances a monotonic `revision`.** Full saves are compare-and-swap on
  the revision they read. An operator's PATCH carries the revision the editor showed and gets 409 when
  stale, because silently replaying a whole-lineup replacement could erase a title they never saw.
  Timestamps cannot do this job: they are second-resolution.
- **Changing and publishing the global playout backend holds one workflow lock** (an in-process mutex
  on SQLite, a database advisory lock on a dedicated PostgreSQL connection). A waiting replica refreshes
  its settings snapshot only after taking the lock, so two controllers cannot overlap a publication.
- **Publication reads use the durable checkpoint row, not a process cache.** Each reconcile attempt or
  routing request reads it once without taking the workflow lock; a missing or unreadable checkpoint
  fails closed.
- **Runtime settings converge across replicas within 30 seconds**: each generation re-reads all
  settings as one snapshot on a timer. SQLite starts no reader.
- **Host filesystem layout is per replica.** Changing `filler.dir` means presenting the same paths to
  every replica and restarting each one; a database lock cannot orchestrate mounts.

One replica is the supported configuration until a multi-replica test passes.

## Decisions are atomic

Approving a Proposal is a compare-and-swap from `submitted`. The final audited Proposal, every newly
tracked title and the intent's `building` channel are written in one transaction, so a concurrent
approve, deny or retry has exactly one winner. A failed approval changes nothing. A channel's non-empty
`intent_ref` is unique in the database, so concurrent materialization cannot create two channels for one
intent; uniqueness races roll back and replan. Codec derivation and the first Tunarr reconcile run after
commit and are best-effort: the periodic channel sweep is the durable retry. A denial can never
overwrite an approval.

## Retention

The daily `housekeeping` job enforces retention:

| Data | Rule |
| --- | --- |
| Sessions | sliding TTL `SESSION_TTL` (30 days); expired rows purged |
| Activity feed | purged after `activity.retention` (30 days) |
| Notifications | terminal intents and their attempts purged after 30 days; queued and sending work never |
| Diagnostics | purged after `diagnostics.retention` (7 days), then trimmed oldest-first to `diagnostics.max_storage_mb` (512 MiB); an active process run is never removed, and each run has a fixed output cap that keeps its start and failure tail |
| Jobs | `done` and `failed` purged after `JOBS_RETENTION` (30 days); `queued` and `running` never, because age is not evidence that work finished |
| Proposals | `denied` purged after `PROPOSALS_RETENTION` (90 days); `approved` (the audit trail) and `submitted` (a member still waiting) are kept |

Proposals are purged before jobs, because `proposals.job_id` has no foreign key. Filler catalog sync
removes clips that vanished from the media server. Diagnostic files and their rows are deleted together
by the diagnostics module, not by the store.

## Moving from SQLite to PostgreSQL

Settings → System → Database runs connect → preflight → backup → migrate and restart. Only this direction
is supported; going back is the backup file plus reverting one setting.

- **The source is only ever read.** After the live store closes, SQLite is reopened with `mode=ro` and
  `query_only`. Every failure ends with the operator still on the database they started on.
- **The copy runs outside the request and outside a writable generation.** `POST
  /v1/system/database/migrate` queues it and returns; the generation drains, copies, verifies parity
  independently, writes the target into `/data/bootstrap.json` only after parity, and starts on
  PostgreSQL. If that first PostgreSQL generation cannot start, bootstrap reverts to SQLite.
- **Preflight** requires a reachable, empty PostgreSQL 13+ target with UTF8 encoding and the needed
  privileges. An advisory lock serializes competing migrations; destination tables stay locked through
  parity and commit.
- **The backup gate is enforced by the server**, which refuses to migrate unless it wrote a backup for
  this migration. That is why the backup is a server-written file, not a download.
- **Tables and copy order come from the destination's catalog** (a topological sort over its foreign
  keys), never a hand-kept list. `goose_db_version` is not copied.
- **Values are coerced by the destination column type**; binary columns bypass string scanning.
- An environment-pinned `DATABASE_URL` disables in-app migration, because environment always wins at boot.

## Backup and restore

- **SQLite:** `GET /v1/backup` (admin) streams a consistent `VACUUM INTO` snapshot, safe under WAL.
  Never copy a live SQLite file.
- **Server-written backups** go to `backup.dir` (`/data/backups`) with mode `0600`, because a backup
  holds every stored secret. The `backup` job writes one on `backup.schedule` (nightly) and then prunes to
  the newest `backup.retain` (7; `0` keeps all). Pruning only touches `loomarr-<timestamp>.db` files it
  wrote, and only after a successful write. The pre-migration backup is the same artifact.
- **PostgreSQL:** `/v1/backup` returns 501 with a pointer, because the image ships no PostgreSQL client.
  The Compose path runs `pg_dump --format=custom` and `pg_restore` inside the Postgres service. The
  `backup` job does not register on PostgreSQL.
- **Restore is a CLI operation**, with Loomarr stopped: it replaces the store the app runs on, including
  the sessions that would authorize an in-app button. `make backup-restore-verify` (SQLite) and
  `make backup-restore-drill` (PostgreSQL) prove the procedure in isolation.

## Tests that pin this

Formerly `design.md` §19.

- **Store conformance:** one suite runs against SQLite (private temp-file clones of a migrated,
  boot-seeded template) and Postgres (testcontainers, private database clones), including
  `ClaimDue` concurrency (no record claimed twice). Each clone opens through its production adapter
  without replaying migrations; dedicated migration and lifecycle tests use fresh databases. The
  race-enabled integration target carries an explicit 20-minute package timeout.
- **Database lifecycle certification:** `make test-db-lifecycle` runs the Postgres gate, then drives
  the shipped image through Compose and Traefik: a fresh PostgreSQL install writes and restarts; a
  populated SQLite install preflights, backs up, drains, copies, verifies, switches, restarts,
  keeps auth and reads, accepts PostgreSQL writes and rolls back; target failures and a killed
  mid-copy recover on SQLite; an in-flight write drains into the snapshot. Direct SQL is limited to
  fixture setup and independent fidelity checks.
- **Lifecycle:** the downgrade guard refuses a newer-schema database; the janitor purges expired
  sessions and old jobs; `GET /v1/backup` on SQLite yields a snapshot that restores to a working
  instance.
