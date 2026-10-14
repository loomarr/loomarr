# Deployment and runtime

Formerly `design.md` §9.2 (restarting in place), §16 (the image, Compose, upgrades) and §18.1 (the
job scheduler). Operator instructions live in the install guides; this doc records what the runtime
guarantees and why. Backup is in [`storage.md`](storage.md#backup-and-restore).

## Restarting in place

Loomarr restarts by rebuilding itself in the same process, never by exiting (decision
[0010](decisions/0010-restart-in-process.md)). `main` loops `Build → Run → Shutdown`, and a restart
ends the current iteration so the next constructs a fresh store, handler, scheduler and HTTP server.

- **Two-phase drain.** A generation first closes admission, cancels its work and event streams, and
  synchronously **quiesces** resources whose responses cannot finish by themselves (channel packagers:
  an endless MPEG-TS response ends only when its packager closes). Then HTTP drains. Then schedulers,
  diagnostics, workers and scratch roots are **finalized** in reverse construction order, and only
  then does the store close. Quiescers are a separate, idempotent registry that rejects new admission
  before taking its snapshot.
- **Storage topology is per generation.** Saving `filler.dir` or `filler.watch_dir` records the desired
  layout at once, but the running generation keeps one immutable applied pair; the restart-cost
  endpoint names the pending keys until the next generation applies them.
- **Per-iteration state must be per iteration.** `http.Server`, the mux, the store and the scheduler
  are allocated each pass. Global registries (`prometheus.MustRegister`, `http.HandleFunc` on the
  default mux, `expvar.Publish`, `sql.Register`) are not used; metrics use a generation-scoped
  registry. A package-level `sync.Once` guarding a resource is a bug. Once-only work (logger setup)
  stays above the loop.
- **The gate is a test:** an N-iteration Build/Run/Shutdown test asserts a stable goroutine count
  (`goleak`).
- A restart interrupts internal channels for a few seconds; Tunarr channels keep playing
  ([`playout.md`](playout.md#constraints-that-follow-from-owning-playout)).

## The image

One image, `ghcr.io/loomarr/loomarr`, for `linux/amd64` and `linux/arm64`. There is no native Windows
server; macOS runs the Linux image through Docker Desktop.

- **Contents:** a cgo-free static Go server with the embedded web UI, the required release-matched
  `loomarr-image` Rust worker, and vendored `yt-dlp`, `ffmpeg`, `ffprobe`, `deno` and `whisper-cli`
  with its model. Every non-Go binary runs through `exec`; pixel buffers never cross the process
  boundary. The base is non-root Debian/glibc (uid 65532) because the media tools need glibc.
- **Why one image:** ffmpeg is load-bearing for playout, so a slim image without it would be a Loomarr
  that cannot air a channel. This is the third answer to the packaging question (ingest sidecar →
  opt-in `filler` tag → one image); each change followed a change in what the tooling was for. The
  cost is size, most of it the whisper model, which is a correctness floor.
- **Runtime packages** exist because ffmpeg loads them at run time: the vendor-neutral hardware-encode
  driver set, and `fonts-dejavu-core` for card labels.
- `/data` is pre-created, owned by uid 65532 and declared a `VOLUME`, so a fresh named volume works.
  The Compose init container chowns bind mounts, which the image cannot pre-seed.
- Alternative `filler.dir` or `diagnostics.dir` values are in-container paths; the mount must exist
  first, then every replica is restarted.
- `HEALTHCHECK` uses `/v1/readyz`, which also requires the image worker's self-test.
- `LICENSE` and `THIRD_PARTY_NOTICES.md` ship under `/usr/share/doc/loomarr/`, and release verification
  fails closed if they or their metadata disappear.

**Publication.** Immutable SemVer tags are the contract; prereleases never move `latest`, and no
mutable major/minor alias exists. The release pushes an untagged digest, signs it keylessly with a
pinned cosign, verifies the signature against the exact workflow identity, and only then promotes
the tag. An existing version, an ambiguous registry lookup or a failed signature stops publication.
Every third-party Action is pinned to a commit SHA. The tagged commit needs a green CI run with both
native image builds; a docs-only final commit uses the `release-candidate` manual scope.

## Compose

The supported topology is one Loomarr replica behind a digest-pinned Traefik edge
(`docker/compose.yaml` is the source):

- Traefik owns the host port; Loomarr exposes 8080 only on the private network, and
  `SERVER_PUBLIC_URL` must name the Traefik URL.
- A one-shot preflight rejects any `LOOMARR_VERSION` that is not an exact released SemVer tag.
- The base file does **not** export `DATABASE_URL`, so the in-app migration can pin PostgreSQL in
  `/data/bootstrap.json`; environment would outrank it. PostgreSQL uses the
  `docker/compose.postgres.yaml` override, since a profile cannot replace another service's
  environment.
- The `ai` profile adds a model-less Ollama with a health-gated, optional dependency.
- Filler ingest needs no profile or tag: the tools are in the image.
- The local Prometheus and Grafana topology is a development tool, never a release profile.

## Upgrades

Images are pinned by SemVer tag, and the upgrade ritual is **back up, then pull**. Migrations are
forward-only; migrations on tables with durable rows use `ALTER TABLE … ADD COLUMN` with a default.
If the database schema is newer than the binary, Loomarr **refuses to start** with a message naming
the fix, so a container rollback fails safe instead of corrupting data.

## The job scheduler

All recurring background work runs under one scheduler (`internal/scheduler`): a code registry of
named jobs, each with a user-editable schedule and a **Run now** trigger.

- **Jobs are code; schedules are settings; history is state.** Each schedule is a 6-field,
  seconds-leading cron in `job.<name>.schedule`, edited through `PATCH /v1/settings`. Build the parser
  explicitly with seconds; `cron.ParseStandard` is 5-field and rejects every saved schedule.
- **River is the engine.** Each job is a River periodic job calling the registry's `Run`; River owns
  due selection, leadership, retries and execution records. Its schema is applied at boot through
  `rivermigrate`. `scheduled_jobs` stays the read model for the Tasks page.
- **Queues are derived from `Timeout`.** A job with a ceiling above River's one-minute default runs on
  `long`; others on `default` (1 worker on SQLite, 4 on PostgreSQL). `long` is 1 worker everywhere
  because media work competes with playout. Queue names are never hand-set, and Run now uses the same
  queue as the schedule.
- **Scheduled ticks coalesce; Run now does not.** A manual run is a real request even while an earlier
  tick is pending, and it runs even while the job is paused.
- **Pause is Loomarr state** (`job.<name>.paused`), durable across restarts and leadership changes.
  River's queue pause is the wrong granularity. A `DisabledReason` is different: a fact about the
  build or backend (backup needs SQLite), shown on the Tasks page, never scheduled, and Run now
  returns 409. Jobs that are irrelevant rather than unavailable (the unused requester's queue poller)
  are not registered at all.
- **Every job has a Group, Title and Description.** The Tasks page shows seven outcome groups
  (Acquisitions, Channels, Filler, Artwork, Playout, System, Backup). Progress comes from the `job` SSE
  frame and is indeterminate unless the job knows its denominator; a percentage is never synthesized
  from elapsed time. A job past its next run is shown as `overdue`. `last_error` expands in full.
- **History is lazy:** `GET /v1/jobs/{name}/history` returns 24-hour aggregates and the five latest
  runs from River's finalized rows.
- `GET /v1/jobs` and `POST /v1/jobs/{name}/run` are admin-only. The job list itself is the registry;
  it is not repeated here.
