# Multi-replica readiness

- Status: investigation required; **one Loomarr replica is the beta support boundary**
- Edge: Traefik 3.7.1
- Database under test: Postgres only

Traefik gives the Docker deployment a health-aware load-balancer seam. It does not make Loomarr's
process-local state distributed. `docker compose up --scale loomarr=2` is unsupported until every
row below has automated two-process evidence and the playout ownership decision is implemented.
Sticky sessions are not a substitute: media-server and ffmpeg requests are independent, and
stickiness cannot serialize jobs, settings, bootstrap, quotas, or capacity.

## Required test topology

- Two independently-addressable Loomarr processes using one Postgres database.
- One Traefik edge with both replicas discovered and active health checks enabled.
- Shared durable storage only where the supported production topology will actually provide it.
- Barriers that force the named operations to overlap rather than relying on probabilistic races.
- Per-replica identity in test responses and logs so distribution and ownership are observable.
- Graceful termination and hard-kill cases for each owned background or child process.

## Matrix

| Seam | Required proof | Current finding |
| --- | --- | --- |
| Routing | Requests reach both replicas; an unhealthy or killed replica is removed; recovery needs no Traefik restart. | Traefik runtime routing is proven for one replica. Two-backend distribution remains to be automated. |
| Database | Concurrent title, channel, approval, and job claims commit exactly once. | Postgres job claims use `FOR UPDATE SKIP LOCKED`; SQLite remains single-replica only. |
| Sessions | Login on A, authenticate/revoke/disable on B, and observe the change immediately on A. | Expected to pass because sessions and users are resolved from shared rows per request; not yet proven as a cluster test. |
| OIDC | Begin on A; callback on B succeeds once; replay fails. | **Blocked:** state, nonce, and PKCE verifier are process-local in `internal/auth/sso.go`. |
| Login limits | Attempts split across A/B enforce one aggregate limit. | **Blocked:** the limiter is process-local, so capacity multiplies with replica count. |
| Bootstrap | Two synchronized first-admin requests yield exactly one success and one admin. | **Blocked:** `internal/auth/bootstrap.go` uses a check-then-create sequence that permits two winners. |
| Migrations | Two replicas start from the prior schema; killing the migrator still converges to one exact history. | Unproven; `internal/store/migrate.go` has no explicit application migration leader. |
| Settings | A write on one replica immediately changes runtime settings, cron, and pause state on the other. | **Blocked:** `internal/settings/service.go` keeps process snapshots refreshed by local writes. |
| Suggestion jobs | A claims then dies; B reclaims and produces one terminal result. | **Blocked on current main:** running jobs can strand. Active proposal-job work owns recovery. |
| Recurring work | One tick and one Run-now execute once; leader death retries; schedule edits propagate. | River's shared database is promising, but stale settings already block the contract and no two-client proof exists. |
| Approval quota | One proposal commits once; two proposals competing for one remaining unattended slot approve at most one. | Proposal CAS is strong; **quota is blocked** because usage check and approval are separate operations. |
| Playout | One global capacity charge and one ffmpeg tree per channel; alternating manifest/segment requests never miss; owner death fails over. | **Blocked:** sessions, HLS lookup, scratch state, and admission are process-local in `internal/playout`. |
| Files | Upload/build on A is readable on B; generation, GC, and serving do not race. | Unproven. `/data` can be host-shared, while HLS scratch defaults to process-local temporary storage. |
| Events | Work on B reaches an SSE client on A, or every consumer recovers by bounded authoritative polling. | **Blocked for realtime delivery:** `internal/events` is an in-process bus. |
| Shutdown | SIGTERM/hard-kill under jobs, SSE, MPEG-TS, and HLS leaves no child process and work completes once or is reclaimed. | Single-process shutdown is strong; cluster drain and suggestion recovery remain unproven. |

## Promotion gate

Supporting more than one Loomarr replica requires:

1. an automated Postgres two-process suite covering every matrix row;
2. distributed or database-backed OIDC state, throttling, bootstrap exclusion, settings invalidation,
   event recovery, and unattended-quota serialization;
3. either durable globally-claimed playout ownership with cross-replica segment routing/shared
   assets, or a dedicated single playout owner outside the general HTTP replica pool;
4. Linux amd64/arm64 and Docker Desktop macOS evidence for routing, drain, restart, and long-lived
   SSE/HLS/tuner streams; and
5. removal of the single-replica warning from install, design, and release notes in the same PR.

Until then, scale **up** CPU/GPU/memory and measured playout capacity on one Loomarr replica; do not
scale the application **out** behind Traefik.
