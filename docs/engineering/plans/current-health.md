# Current Health and startup history

Parent: [#509](https://github.com/loomarr/loomarr/issues/509)

Delivery issue: [#518](https://github.com/loomarr/loomarr/issues/518)

## Outcome

An administrator opening Loomarr sees **Current Health**: one server-owned, continuously refreshed
answer to whether the running application and its configured capabilities are healthy now. The same
diagnostics surface retains immutable **Previous Startups** so an operator or support agent can still
explain how the current and prior application generations booted.

This replaces “current generation” as a user-facing concept. A generation remains useful internal
identity and historical evidence, but it is not the product headline.

## Domain model

- **Current Health** is mutable, in-memory truth for the running generation. It has a stable
  generation id, version, overall state, `updatedAt`, and an ordered set of checks.
- A **Health check** has a stable key, operator label, required/optional classification, mode
  (`startup` or `continuous`), status, concise redacted detail, remediation route, observation time,
  and freshness deadline.
- A **Startup report** is an immutable snapshot of every startup-mode check plus the initial
  observations of continuous checks. Completed reports are retained as Diagnostic events.
- A **Health incident** begins when a check crosses into warning, failed, or stale and ends when it
  recovers or is superseded by a newer generation. Only transitions are persisted; successful
  polling does not create repetitive event rows.

The overall Current Health state is server-owned:

- `starting` while a required startup check is pending;
- `healthy` when every required check is passing and no optional check needs attention;
- `degraded` when required checks pass but an optional check warns, fails, or becomes stale;
- `unhealthy` when a required check fails or becomes stale.

An unconfigured optional capability is `skipped`, not degraded. A continuous check with no completed
observation is `pending`; after its explicit freshness deadline it is `stale`, never silently green.
Periodic probes require two consecutive failures before changing a previously passing required check
to failed; an explicit fatal observation may fail immediately. One success recovers the check. This
prevents one transient timeout from flapping readiness without hiding a sustained outage.

## Module seam

`internal/diagnostics` owns one deep Health module. Callers declare the ordered checks once and report
bounded observations by stable key. The module owns state derivation, freshness, transition
deduplication, redaction, startup snapshotting, readiness projection, persistence, and API-safe
copies. Callers do not derive overall health or write health persistence records themselves.

The existing setup probes remain the adapters for media server, Tunarr, requester, LLM, and TMDB.
The Store supplies the database adapter. Internal lifecycle seams supply configuration, secrets,
image-worker, and HTTP observations. Tests replace those adapters; no generic health-check
dependency or parallel probe implementation is introduced.

`/readyz` is the required-check projection of Current Health. `/healthz` remains process liveness and
does not begin probing dependencies. A database outage can therefore make readiness false while the
process stays alive and continues exposing its in-memory explanation.

## Probe lifecycle

Startup observations populate Current Health immediately. After startup, a named System scheduler
job runs the continuous probes concurrently under bounded per-probe and whole-run deadlines. Its
schedule uses the normal `job.system_health.schedule` setting, defaulting to every 30 seconds. An
explicit refresh invokes the same runner rather than a second implementation.

Configuration changes invalidate affected observations and trigger a prompt run. Internal
components may publish an immediate observation when they already know about a failure or recovery;
the scheduled run remains the backstop. Probe work is bounded, redacted, and cannot block request,
scheduler, or Playout goroutines.

## Persistence and read surfaces

Current Health stays authoritative in memory so a store failure cannot erase the explanation.
Completed startup reports and health incident transitions are synchronously checkpointed through
the Diagnostics redaction seam when persistence is available. A failed checkpoint remains retryable.

The typed admin surface is:

- `GET /v1/diagnostics/health` for Current Health;
- `POST /v1/diagnostics/health/refresh` to enqueue/run the same bounded health check path;
- `GET /v1/diagnostics/startup-reports` for the current startup snapshot and at most 20 retained
  reports.

The existing authenticated Bearer-token path makes the JSON usable by an agent without UI scraping.
Health state changes publish a lossy SSE invalidation; clients refetch the GET source of truth and
also use a slow polling fallback.

## Operator experience

Settings → System → Diagnostics leads with **App Health** and a **Current Health** card. It shows the
overall state, version, last update, next expected refresh, and an accessible check table with
status, freshness, detail, and remediation. Status is always text plus icon, never color alone.

The startup history is subordinate under **Previous Startups**. Selecting a report shows the
immutable boot table and terminal-equivalent evidence. The shell shows calm, short recovery notices
and persistent degraded/unhealthy notices. Acknowledgement is per health incident, so a continuing
incident does not spam and a materially new incident is visible. A newer generation supersedes old
notices.

The interactive terminal table remains a startup projection only. Continuous transitions use the
normal one-line structured JSON log and Diagnostic events; Loomarr never redraws terminal tables in
container logs.

## Delivery slices

1. Amend the design and #518 acceptance contract before changing implementation.
2. Deepen the existing startup state into Current Health while preserving the startup report value
   and its retention format.
3. Register the bounded System health job and reuse the existing probes; add invalidation and
   transition persistence.
4. Add the typed endpoints and regenerate OpenAPI/orval output without changing authorization.
5. Lead the Diagnostics UI with Current Health and move retained startup reports under Previous
   Startups; update notices, responsive stories, and visual baselines.
6. Prove healthy, degraded, unhealthy, recovery, stale, unconfigured, restart, database-outage,
   readiness-agreement, authorization, narrow layout, no-color terminal, and long-detail behavior.
7. Run all touched-area gates, publish the focused PR with auto-merge, then continue #513–#515.

## Acceptance criteria

- Current Health and `/readyz` derive from one required-check state and cannot disagree.
- Current checks distinguish startup-only evidence from continuously observed health and expose
  observation/freshness times.
- Required, optional, unconfigured, pending, failed, recovered, and stale states are distinct.
- Periodic probes are bounded, concurrent, use existing probe implementations, and do not create a
  second scheduler mechanism.
- One transient periodic timeout does not flap a previously passing required check; two consecutive
  failures do, and one success recovers it.
- Store-less and database-failed operation retains an in-memory unhealthy explanation.
- Only incident transitions are persisted, with retryable checkpoints and no credentials or raw
  secret-bearing configuration.
- Completed startup reports remain immutable, survive restart within retention, and preserve prior
  in-process generations.
- Admin JSON supports both the UI and authenticated agents; members cannot read or refresh
  diagnostics.
- The UI says App Health, Current Health, and Previous Startups; it shows status without relying on
  color and remains accessible at narrow widths.
- Notices deduplicate by incident, persist while action is needed, announce recovery calmly, and are
  superseded by a newer generation.
- Interactive startup rendering stays a renderer only; non-interactive output remains one JSON
  object per line with no ANSI or multiline side channel.
- No generic health dependency is added.
