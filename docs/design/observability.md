# Observability

Formerly `design.md` §17 and §5's activity and diagnostic evidence tables. Loomarr keeps three
different kinds of record, each answering one question:

| Record | Answers | Written by |
| --- | --- | --- |
| `activity` | What did Loomarr do? (the Dashboard's Recent activity) | the subsystem making the change, at the domain transition |
| Diagnostic events and Process runs | What did this part of Loomarr observe? | `internal/diagnostics` |
| Metrics | Is Loomarr operating? | the generation's `metrics.Recorder` |

A fourth, the discovery-quality ledger, answers how far requested Proposals progressed.

## Activity

`activity` holds one row per notable event (`{id, at, kind, level, text, subject_id}`).

- Rows are written at each domain transition, never from the event bus: the bus is in-memory and
  lossy, and only the subsystem making the change knows what happened.
- Best-effort: a failed insert is logged and the operation continues.
- `text` is composed at the write point and stored, so a historical row never changes wording when a
  channel is renamed. `level` is `info | warn | error`.
- `ListActivity(limit)` reads newest first and nothing else. `activity.retention` is enforced by the
  housekeeping job.

## Diagnostics

`internal/diagnostics` owns retained technical evidence behind a small interface: record one
structured Diagnostic event, or begin one Process run and report its progress, output and result.
Callers never choose columns, paths, batching, redaction, retention or export formats. It has SQLite
and PostgreSQL adapters through the shared store and an in-memory test adapter, and receives a narrow
sink role rather than the whole `store.Store`.

- **Logging fans out.** Every accepted `slog` record goes to JSON stdout (available even when the
  store is down) and to a bounded recorder that redacts a copy and enqueues it without blocking.
  Saturation drops lower-severity evidence and counts it. The durable threshold is `info`; an admin
  may capture `debug` for 1–15 minutes, optionally scoped, and it expires on its own.
- **Events are structured at ingestion:** time fields in Unix milliseconds (`occurred_at` from the
  producer, `received_at` from the server, so client skew stays visible), level, source, subsystem, a
  stable `event` name, a human `message`, the correlation ids, bounded JSON attributes and a
  normalised `size_bytes`.
- **Correlation.** Every request has one `request_id` (echoed in `X-Request-Id` and RFC 7807
  `instance`). Producers add only the ids they already know: `playback_session_id`, `channel_id`,
  `schedule_block_id`, `job_id`, `process_run_id`, `actor_id`, `instance_id`. Empty means unknown; no
  lookups are made to fill a log. `schedule_block_id` is derived by the server from Channel, start,
  kind and content, so client transitions, guide rows and ffmpeg runs join without exposing ids or
  paths.
- **Best-effort by contract.** An event is never part of the operation it describes and can never
  fail a request, job, reconcile or playout process. Shutdown offers a bounded flush.
- **Redacted before persistence.** Credentials, authorization, cookies, sessions, secrets, signed
  query parameters, form or DOM content and unnecessary full paths are removed before any row or
  output byte is written; read-time masking is only defence in depth.

**Process runs.** One row per external media process: purpose, parent, instance, related channel,
block and job, executable and version, a redacted command summary, times, status, exit and
termination reason, first and last error, and an output reference. ffmpeg has three streams with
three owners: stdout is media and never logged, structured progress is downsampled into the run, and
stderr drains continuously into a bounded file (the first 256 KiB plus a rolling 768 KiB tail) with
per-line timestamps. Writes never apply backpressure; dropped lines are counted and marked. Process
exit and output draining have separate lifetimes, so reaping a child never loses buffered media.
Active runs are never aged out.

**Retention.** `diagnostic_retained_bytes` keeps one running total per evidence table, adjusted in
the same transaction as each row change, so the budget check reads two rows instead of scanning
(#1398). `diagnostics.max_storage_mb` bounds logical payload, not the file: indexes and pages add to
it, and SQLite's file only shrinks on `VACUUM`. Time is the workhorse index; the six correlation-id
indexes are partial (`WHERE col <> ''`), and low-selectivity columns are not indexed.

## Client diagnostics

`POST /v1/diagnostics/client-events` takes 1–20 observations from `web` or `android_tv`, from a closed
vocabulary: `client.error_boundary`, `client.unhandled_error`, `client.api_failed`,
`player.attached`, `player.detached`, `player.ready`, `player.source_replaced`,
`player.buffering_started`, `player.buffering_ended`, `player.seeking`, `player.seeked`,
`player.media_error`, `player.schedule_block_changed`, `player.playhead_drift`. Each carries only
its allowed fields; messages, stacks, URLs, bodies and console output are never accepted. The server
derives `actor_id` and owns severity. Any invalid field rejects the whole batch. Accepted batches
return `202` after enqueue. The limit is 120 observations a minute per actor with a burst of 30.
Clients keep a 100-observation memory queue, send at most 20 every two seconds, drop routine events
before errors, and never block playback or keep a second log.

## Current Health

Each generation owns one **Current Health** value. Checks have a key, label, required flag and
`startup | continuous` mode; observations are `pending | passed | warning | failed | skipped | stale`
with bounded detail, remediation and a freshness deadline. Unconfigured optional integrations are
`skipped`. A continuous pass goes `stale` after its deadline.

- The overall state is `starting`, `healthy`, `degraded` (only optional checks need attention) or
  `unhealthy` (a required check failed or went stale). A previously passing required check needs two
  consecutive periodic failures to fail; one success recovers it.
- `/readyz` is the required-check projection of the same value; `/healthz` is liveness only.
- Startup is the first pass. Configured integrations reuse the setup probes concurrently in one
  bounded window; the `server.public_url` check runs only after the listener exists. The
  `system-health` job refreshes continuous checks (default every 30 s), and settings changes prompt a
  run. No request or playout path waits on a probe.
- The value survives a database failure. Only transitions into or out of an incident are checkpointed
  as events. Each generation freezes one **Startup report**; the last 20 are kept.

Settings → System → Diagnostics has three views: **Logs** (the default), **Current Health** and
**Media processes**. Logs refreshes automatically, newest first by default, with an explicit oldest
first that starts at the earliest retained event. Severity filters are All, Errors, Warnings and
Info; exact correlation filters sit behind a disclosure. Pages are 50 records, virtualised.

## Reading and exporting

The read surfaces are admin-only, typed and cursor-paged, for the UI and Bearer-token agents alike.
The event read defaults to the last hour, newest first, 100 records (maximum 200); the cursor is the
`(occurred_at, id)` position bound to its order. Filters are capped at 128 bytes and the text search
at 256. JSON and NDJSON are projections of the same page, and one Process run downloads as text.

A **troubleshooting report** is a redacted ZIP assembled on demand from one selection: a window of at
most 24 hours; events, process metadata and process output; the same correlation filters. Caps: 2,000
events, 50 runs, eight outputs, 16 MiB, 30 s; hitting one is a declared truncation. Preview and
download share the selection. Entry names are fixed (`manifest.json`, `system.json`, `events.ndjson`,
`processes/…`) with fixed timestamps; caller identifiers never become paths. `manifest.json` declares
windows, counts, drops, truncation and redaction; `system.json` contains no host names, addresses,
targets, paths or user data. Assembly redacts again. Nothing is ever sent anywhere automatically.

## Metrics

Prometheus metrics are an operator contract, not a mirror of domain events. One generation-scoped
`internal/metrics.Recorder` owns a private registry, the collectors, HTTP middleware, store collection
and every label classifier; callers use semantic callbacks and never pass label values.

- `/v1/metrics` (alias `/metrics`) is unauthenticated on the trusted-LAN listener; deployments keep it
  on a private scrape network.
- **No identifying labels:** no usernames, emails, titles, channel or media ids, request ids, URLs,
  paths, prompts, raw errors or secrets. `route` is the matched template. `job` is the registry's
  code-defined name; every other label is a closed enum, and unexpected values collapse to `other`.
- Families cover build info, inbound HTTP (including outbound fan-out per route), outbound
  dependencies and retries, the database pool, retained-object gauges, scheduler jobs, channel
  reconciles, playout sessions and failures, auth, LLM tokens, filler pods and rotation, the image
  worker, and scrape errors. The exposition endpoint is the list.
- Name, type, unit, label names and value domains are compatibility commitments. A rename dual-emits
  for one release line before removal.
- One provisioned Grafana **Overview** dashboard (stable UID, read-only) and example rules ship as
  source; Prometheus and Grafana are never default services. `make observability-verify` checks the
  manifest, hostile-label exclusion, dashboard queries and rules. `make observability-dev` runs a
  local stack against `make dev-be`.

## Discovery quality

A separate local ledger, never Prometheus labels, records how far requested Proposals progressed.
Stages are `retrieval`, `generation`, `grounding`, `approval`, `acquisition` and `scheduling`, each
with a closed outcome set.

- Only the authoritative transition writes, after it commits: the Proposal Job for the first three,
  the approval transaction for `approved`/`declined`, the provisioning transition for
  `playable`/`failed` (timed from `requested_at`), and the first successful live reconcile for
  `scheduled`. A stage never reached is absent. Writes are best-effort and idempotent by an opaque key.
- **A declined Proposal is not dislike.** Only explicit `keep`, `less`, `never` and `surprise` actions
  are taste signals; denial, detach, stopping playback, inactivity and non-selection are never
  recorded as preference.
- Receipts hold stage, outcome, time and bounded quantities only: no titles, intents, errors, URLs,
  credentials or ids, hashed or not. Housekeeping folds them into daily aggregates and purges them
  after 30 days; aggregates last 24 months, with at most 256 unreferenced evaluation-run snapshots.
- One admin-only JSON export returns aggregates and referenced snapshots, never receipts or keys.
  Nothing is uploaded. SQLite and PostgreSQL share one conformance suite.
