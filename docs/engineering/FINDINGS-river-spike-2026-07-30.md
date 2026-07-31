# FINDINGS — River + SQLite spike (2026-07-30)

Run before adopting River, against **river v0.41.1** + **riverdriver/riversqlite v0.41.1**, using
this repo's exact SQLite DSN (`journal_mode(WAL)`, `busy_timeout(5000)`, `foreign_keys(on)`) and
`SetMaxOpenConns(1)`. Everything below was executed, not read.

## It works

- `rivermigrate.New(riversqlite.New(db), nil).Migrate(ctx, DirectionUp, nil)` applied **7
  migrations** programmatically. ⚠ **This is the finding that makes adoption viable**: the docs
  only show `river migrate-up` (a CLI + a `sqlite://` DSN), which would have meant a second
  migration *system* operators must run alongside goose. The programmatic API means River's
  schema is applied at boot like everything else.
- A `PeriodicJob` with `RunOnStart: true` fired and a worker executed it, on SQLite, on the
  single-connection pool. No deadlock against `MaxOpenConns(1)`.
- `modernc.org/sqlite` is already this repo's driver and is exactly what River tests against, so
  no driver change and no CGO.

## ⚠ Cron: `ParseStandard` REJECTS every schedule Loomarr has

River's docs point at `cron.ParseStandard`, which is **5-field**. Loomarr's schedules are
**6-field seconds-leading** (`gronx`, matching Overseerr) and every single one is rejected:

```
ParseStandard("0 */5 * * * *") REJECTS: expected exactly 5 fields, found 6
ParseStandard("0 30 3 * * *")  REJECTS: expected exactly 5 fields, found 6
```

The fix is to build the parser explicitly — `cron.NewParser(cron.Second | cron.Minute |
cron.Hour | cron.Dom | cron.Month | cron.Dow)` — which accepts all of them. Verified.

**Why this matters beyond a compile error:** `job.*.schedule` values are operator-editable
settings already persisted in the database. Following the documented example would have rejected
every saved schedule at boot, on installs that were working fine.

## ⚠ Pause is QUEUE-level, not per-job

`Client.QueuePause(ctx, name, opts)` pauses a whole queue. The per-job levers are
`PeriodicJobs().Add/Remove/RemoveByID`, which only take effect **on the client holding cluster
leadership**.

So the requested per-job pause cannot be delegated to River: the paused set must live in
Loomarr's own state (settings), with the scheduler consulting it. River's queue pause is the
wrong granularity, and a leadership-dependent `Remove` is not a durable record of operator
intent — a restart or a leadership change would silently resume a job someone deliberately
paused.

## Limitations carried from the docs (not re-verified here)

- SQLite support is **officially experimental** as of 0.23; the schema "may still have a few
  tweaks". Against this repo's forward-only migration rule, that is the standing risk of this
  decision — accepted by the maintainer with the tradeoff stated.
- ~10k jobs/sec on SQLite vs ~4× that on Postgres. Irrelevant here: this is 9 cron jobs, not a
  throughput workload.
- Single concurrent writer; batch ops go one row at a time (sqlc limitation on SQLite).

## Bottom line

Adoption is mechanically sound. The two things that would have broken silently — the cron field
count and the pause granularity — are both handled in the design rather than discovered later.
