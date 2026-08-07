# Virtual Channel Builder — Design Doc

**Status:** Draft for implementation
**Audience:** Claude Code (build agent) + maintainer
**Working name:** `loomarr` — weaves your library into TV channels, and follows the *arr / Servarr naming convention since it lives in that stack (alongside Sonarr/Radarr, which it drives). Container image `loomarr`. Rename freely.

> Supersedes the earlier "Channel Content Provisioner" framing. The app's purpose is to **build and maintain virtual TV channels end to end**: from a natural-language intent, through content acquisition, to a live Tunarr channel that stays filled. "Provisioning" is one subsystem of that, not the product.

---

## 1. Purpose

`loomarr` turns *"I want a channel that feels like X"* into an actual, running Tunarr channel — and keeps it populated as content comes and goes. It closes the full loop:

**intent → suggest a lineup → acquire what's missing → build the schedule → push it to Tunarr → backfill and maintain.**

The app has **five cooperating subsystems**:

| Subsystem | Owns | "Decides…" |
| --- | --- | --- |
| **Suggester** (§8) | intent → proposal | *what* content belongs on the channel |
| **Provisioner** (§3–§7) | acquire missing titles, track to available | *whether/when* content exists |
| **Scheduler** (§9) | build lineup, insert pods, push to Tunarr, backfill | *order, timing, and delivery* |
| **Filler** (§10) | filler ingestion, clip catalog, pod assembly | *what plays in the breaks* |
| **Web** (§12) | human control surface | *approval and oversight* |

### In scope
- Natural-language channel intent → grounded proposal (lineup + acquisitions).
- Acquire missing titles via Seerr (or Sonarr/Radarr) and track each to `available`/`unavailable`.
- **Build channel programming** (order, time-slots, shuffle/blocks) and insert **era/audience-matched commercial pods** with their own filler sourcing pipeline (§10).
- **Push channels to Tunarr** via its API and **reconcile** desired vs. actual channel state.
- **Backfill loop:** go live immediately with available content + filler; swap in real titles as acquisitions land; substitute on give-up.
- Persist all state to **Postgres or SQLite**.
- **Multi-user login with Emby/Jellyfin accounts** (Seerr-style import/sync), roles, per-user quotas, and audited approvals (§11).
- Self-documented HTTP API (§7.1) and an embedded web UI (§12).
- Deploy as a single Docker container.
- **Play out its own channels** (§9.1): serve HLS + MPEG-TS segments and publish an M3U tuner + XMLTV guide that the media server registers directly, with Tunarr as a supported alternative backend rather than a requirement.

### Explicit non-goals
- **Does not manage indexers, download clients, or quality profiles** — that's Sonarr/Radarr's job.
- **Does not replace the media server** — Emby/Jellyfin remain the client, the library, and the thing a viewer actually opens. Loomarr feeds them a tuner; it is not a player.
- **Does not replace Tunarr** — Tunarr stays a first-class playout backend (§9.1), chosen per channel. Hardware that can't transcode, or an install already happy with Tunarr, keeps working unchanged.
- **The provisioner core never chooses titles or auto-acquires** — the suggester proposes and a human (or a quota-gated auto-approve, or a channel's opted-in auto-curate grant — `programming-design.md` §8.2) confirms before anything is acquired or scheduled. Every one of those paths routes through the single `Approve` gate; none writes a `wanted` title directly.

### Design envelope
Sized for a household/homelab, and tests should assume it: **~100k library items, ≤50 channels, ≤20 users, ≤10k filler clips, single media server.** These bounds justify several deliberate simplicities (federated search over indexes, `LIKE` over FTS, client-side list filtering, in-process jobs). Anything beyond the envelope is future work, not a v1 requirement.

---

## 2. Architecture

```mermaid
flowchart LR
  User[Human]
  Web[Web UI]
  SUG[Suggester]
  PROV[Provisioner]
  SCH[Scheduler]
  MS[(Emby / Jellyfin)]
  Seerr[Seerr]
  Arr[Sonarr / Radarr]
  LLM[LLM: Ollama / OpenAI-compatible]
  Tunarr[Tunarr]
  DB[(Postgres / SQLite)]

  User --> Web
  Web -->|intent| SUG
  SUG <-->|grounded tool calls| MS
  SUG <-->|reason| LLM
  SUG -->|proposal| Web
  Web -->|approve: acquisitions| PROV
  Web -->|approve: lineup| SCH
  PROV -->|request| Seerr --> Arr
  PROV -->|poll queue: progress / grabbed| Arr
  PROV -->|library scan + GET /Items lookup| MS
  PROV -->|available / unavailable &#40;internal events&#41;| SCH
  SCH -->|build + push channel, filler, flex| Tunarr
  SCH -->|backfill re-push| Tunarr
  Web -->|monitor via SSE| PROV
  PROV <--> DB
  SCH <--> DB
```

The subsystems are internally decoupled (clean interfaces) but ship in one binary/container by default. The **provisioner's availability events are now an internal feed to the scheduler** — that's what drives backfill. An *optional* outbound webhook/SSE remains for external consumers, but the primary consumer is `loomarr`'s own scheduler.

**Filler flow (not drawn above to keep the diagram legible):** clips land in a drop-folder (manually, via MeTube, or via loomarr's own **ingest job** on a `filler`-variant image) → Tunarr scans that folder as a **`local` media source** → loomarr **syncs its clip catalog from Tunarr** (§10). The media server is not in the filler path at all, and the core never *probes* media — Tunarr assigns duration and program ids.

### Boundaries (ports)
Core logic depends only on interfaces; concrete adapters live at the edges.

| Boundary | Interface | Adapters |
| --- | --- | --- |
| Library | `Library.Lookup(title) → (itemID, present)` | Emby, Jellyfin (shared impl, flavor-specific auth) |
| Requester | `Requester.Request/Cancel(title)` | Seerr (default), Sonarr+Radarr (alt) |
| **Programmer** | `Programmer.Reconcile(channel, lineup)` | **Tunarr** (only impl; abstracted for future ErsatzTV) |
| Suggester | `Suggester.Propose(intent) → Proposal` | LLM: Ollama (local) or an OpenAI-compatible endpoint (hosted — OpenRouter, or a user-supplied Custom base URL; Claude via OpenRouter) |
| Catalog | `Catalog.Search(query) → []Candidate` | Library + TMDB/TVDB — grounds the LLM **and** backs `GET /v1/search` (§7.2) |
| FillerSource | catalog sync + optional ingest | Tunarr `local`-source catalog sync (core); `Ingester`: yt-dlp / Archive.org → drop-folder, in-core on a `filler`-variant image (§10) |
| Store | `Store` (see §5) | Postgres, SQLite |
| Events | `EventBus` | internal (→ scheduler) + optional outbound webhook |

---

## 3. Provisioner domain model

**Title** — a unit of content the app wants. Identity is an external id, never a title string.
- `MediaType`: `movie` | `series`
- `TMDBID` (canonical for movies; accepted by Seerr for series), `TVDBID` (preferred key for series)
- `Name`, `Year` — logs/request payloads only, never identity
- `Seasons []int` — series only; empty = all

**Key** — stable dedup/identity key, identical whether derived from a `Title` or a webhook:
- series with TVDB id → `series:tvdb:<id>`; otherwise → `<mediatype>:tmdb:<id>`

**Record** — persisted provisioning state: key, title blob, state, library item id, `requested_at`, `deadline`, `attempts`, `last_error`, `updated_at`.

> **Time storage:** persist timestamps as Unix epoch `BIGINT`, not native datetime types — keeps the schema dialect-neutral across Postgres/SQLite.

---

## 4. Provisioning state machine

```mermaid
stateDiagram-v2
  [*] --> Available: already in library
  [*] --> Requested: request accepted
  [*] --> Wanted: submit failed
  Wanted --> Requested: reconcile retry
  Requested --> Downloading: webhook Grab
  Requested --> Available: library confirms
  Downloading --> Available: Import + library confirms
  Requested --> Unavailable: deadline exceeded
  Downloading --> Unavailable: deadline exceeded
  Wanted --> Unavailable: deadline exceeded
  Available --> [*]
  Unavailable --> [*]
```

| State | Meaning | Emits event? |
| --- | --- | --- |
| `wanted` | Requested by app; downstream submission not yet accepted | no |
| `requested` | Accepted by Seerr/*arr; awaiting a release | no |
| `downloading` | A release was grabbed; genuinely in flight | no |
| `available` | Present in the library, schedulable | **yes → scheduler** |
| `unavailable` | Gave up (deadline / unfindable) | **yes → scheduler** |

**Availability is discovered by polling, not (only) by a webhook.** The primary path to
`available` is the scheduled **library scan** (§6, §18.1): a `library-scan` job periodically
lists what the media server has recently added and confirms any in-flight title now present —
the same `LibraryConfirmed` transition the reconciler's deadline backstop applies, but
continuous and not deadline-gated. Correlation matches a scanned item to an in-flight record by
**any** provider id the item carries, not just its preferred one: a series is keyed
`series:tvdb:<id>` when a TVDB id is known but `series:tmdb:<id>` when it was added TMDB-only
(the suggester/channel-add path), while the media server exposes *both* ids on the show — so the
scan probes every key an item can produce, or a TMDB-keyed series would never confirm `available`
even with its episodes present. This mirrors how Overseerr/Seerr work (they are entirely
poll-based; verified). The inbound Sonarr/Radarr webhook (§6 Ingest) is a *latency
optimization* on top of the scan, not the source of truth — and is retired once the scan path
is proven (build plan). A `library-full-scan` runs less often as a safety net for anything the
incremental scan's "recently added" window missed.

### Invariants (cover with tests)
1. **Terminal monotonicity — scoped to the acquisition lifecycle.** Once `available`/`unavailable`, no *provisioning* event regresses it; late/duplicate webhooks ignored. But `available` is a statement about a moment, not a promise about forever: media gets deleted, replaced, or re-id'd after acquisition completes. The **scheduler** therefore revalidates slot items against the library at reconcile time (§9) rather than trusting an old `available` — drift is the scheduler's problem to detect, not a reason to mutate terminal provisioning state.
2. **Only `available`/`unavailable` emit events** — those are the only transitions the scheduler acts on.
3. **Idempotent enqueue.** Safe to enqueue the same title repeatedly; dedup keyed on external id via the store; per-key lock prevents concurrent double-requests.
4. **Library is source of truth.** Even on an import webhook, confirm the library reports the item before `available` — Tunarr reads the library, and there's scan lag.
5. **Deadline discipline.** Every in-flight record has a deadline; a `Grab` webhook resets it to the shorter downloading TTL; past deadline → `unavailable` + downstream `Cancel`.

---

## 5. Persistence — Postgres **and** SQLite

One store abstraction; both backends first-class. It holds **both** provisioning records **and** scheduler state (channels, desired lineups).

```
Store interface:
  # provisioning
  GetTitle/UpsertTitle/ListTitlesByState
  ClaimDueTitles(now, limit)               # concurrency-safe reconcile claim
  # scheduling
  GetChannel/UpsertChannel/ListChannels
  GetDesiredLineup/UpsertDesiredLineup
  GetSeriesEpisodes/UpsertSeriesEpisodes  # §9 series expansion, cached (see below)
  ListStaleSeriesEpisodes(before, limit)  # which shows the refresh job should re-enumerate
  # filler catalog (§10)
  GetClip/UpsertClip/DeleteClip
  ListClips(filter: kind/era/audience/category)
  GetSplitProposal/UpsertSplitProposal/DeleteSplitProposal  # §10 V34: persisted until review
  ListSplitProposals(status)              # §10 V35: the Incoming tab's reels
  SetClipsRemoved(paths, at)              # §10 V35: the ONLY writer of the removal tombstone
                                          #   (the scan's upsert must never write it)
                                          # §10 V38: clips carry `confidence` (0-100) + `auto_filed`
                                          #   — the score is grounding-CAPPED, never self-reported,
                                          #   and auto_filed is what makes an unattended file undoable
  ListFillerSources/UpsertFillerSource/DeleteFillerSource   # §10 V35/V37: ONE flat list, all kinds
                                          #   ⚠ V37: `folder`/`library` are SINGLETON rows
                                          #   materialised from config — never operator-inserted,
                                          #   never removable, so no source is listed twice
  GetPull/UpsertPull/ListPulls(status)    # §10 V35: filler acquisition's approval gate
  # users & sessions (§11)
  GetUser/UpsertUser/ListUsers            # keyed by media-server user id
  CreateSession/GetSession/RevokeSessionsForUser
  # jobs & proposals (§8, §10)
  CreateJob/GetJob/UpdateJob
  ClaimDueJobs(now, limit)                # same SKIP LOCKED pattern as titles
  GetProposal/UpsertProposal
  ListProposals(status[, user])           # approval queue; My proposals
  # settings KV (small, typed accessors over one table)
  GetSetting/SetSetting                   # instance id (§11 DeviceId)
```

### Backend selection
`DATABASE_URL` scheme selects the backend: `sqlite:///data/loomarr.db` or `postgres://…`. Fail fast on unknown scheme.

### Drivers
- **SQLite:** `modernc.org/sqlite` — pure Go, **no cgo** → tiny static image (distroless/scratch). Open with `journal_mode=WAL` + `busy_timeout`.
- **Postgres:** `jackc/pgx` **via its `database/sql` stdlib shim** — decided, so both backends share one store code path (§14).

### Schema & migrations
- Keep SQL ANSI where possible; `INSERT … ON CONFLICT(key) DO UPDATE` works on both.
- **`goose`** (decided, §14) with an **embedded** FS and separate `migrations/sqlite` + `migrations/postgres` dirs so dialect DDL never leaks. Auto-run on startup (`AUTO_MIGRATE=true`).

### Cached series episodes (§9 series expansion)

A series lineup entry expands into its episodes, and enumerating them is a media-server call
per SHOW. That call sat on the **request path**: `GET /v1/guide` re-expanded every series on
every load, so a channel with four shows spent **232ms** in enumeration while a 25-film movie
channel spent 1ms. Profiling put ~90% of the guide's latency there (`ComputeDesiredAt` itself
benchmarks at 45ms for *200* channels, so it was never the cost).

`series_episodes` caches one row per show — its episode list plus `fetched_at` — so expansion
becomes a store read. The **library is still the source of truth**; this is a materialized
answer, never a second opinion:

- **Read path:** `GetSeriesEpisodes(libraryID)`. A miss (or a row older than the staleness
  horizon) falls back to the live call and writes the result back, so a cold cache degrades to
  today's behaviour rather than to an empty channel.
- **Refresh:** the **`series-episode-refresh`** job (§18.1) re-enumerates shows whose rows have
  aged out. Bounded to the shows actually referenced by channel lineups — the set that matters
  is small and known, and sweeping the whole library would cost far more than it saves.
- ⚠ **NOT hung off `library-scan`.** That job correlates *in-flight* acquisitions
  (`requested`/`downloading`) and returns early when none exist, so a show that is already
  `available` — precisely the ones the guide expands — is never revisited. Attaching episode
  refresh there would produce a cache that looks invalidated and never is, on exactly the
  settled installs where the guide is used most.
- **New episodes still appear promptly by the other path:** a newly-landed episode arrives as an
  acquisition, and that already triggers a reconcile + a `channel` SSE frame. The refresh job
  covers the case nothing else does — episodes added to the media server directly, with no
  Loomarr acquisition behind them.

### Airing history (§9 recency-aware placement)

The scheduler had no memory of its own output. Separation (`programming-design.md` §3) constrains
what recurs **within one cycle**; once the cycle wraps, the count resets and the deck replays from
position. A viewer sees the same film every couple of days with no pattern behind it — reported
from the dev "1980s Action Heroes": Akira at Tue 21:53, Fri 13:33, Sat 02:10, Mon 01:30.

`airings` records one row per programme aired — `{channel_id, key, library_item_id, aired_at}` —
written from playout at the moment a programme is resolved for streaming. It is the programme
analogue of `RecordClipPlay` (§10 V28), which already does exactly this for commercial clips: the
same write point, the same best-effort posture, and for the same reason (you cannot rotate what
you cannot remember).

- **Write path:** the playout resolver knows what it is about to stream, so the write costs no new
  lookup. **Best-effort:** a failed insert is logged and the programme still airs — telemetry must
  never be able to take a channel off the air.
- **Read path:** `LastAiredByChannel(channelID)` returns the most recent airing per key, which is
  all the scheduler needs. One row per distinct key, not the full history.
- **Loomarr's own output, not viewer behaviour.** This deliberately does not touch the media
  server's per-user watch state (`UserData`): that is a different signal with a per-user "whose
  history counts?" question and privacy implications. This is the system remembering what it
  broadcast — the minimum a human programmer does.
- **Retention:** rows older than the janitor's horizon are purged like every other accumulating
  table (below). History beyond the longest recency horizon has no reader.

`activity` records what Loomarr did, one row per notable event — `{id, at, kind, level, text,
subject_id}` — for the Dashboard's **Recent activity** feed (§12, V32).

- ⚠ **Written at each domain TRANSITION, not at the event bus.** Publishing to the bus is one
  line and would look like a free feed, but the bus is deliberately **in-memory and lossy**
  (`events/bus.go`: *"a dropped event is a latency bug, not a correctness bug"*) — so a feed
  built on it would silently lose rows under load, which is exactly what a persisted feed
  exists to prevent. It is also domain-neutral: it knows `{type: "title"}`, not *"Darkwing Duck
  landed — CH 42 slot 05 backfilled in place"*. The subsystem making the change is the only
  place that knows what actually happened, so that is where the row is written.
- **Best-effort, exactly like `airings`.** A failed insert is logged and the operation
  continues. Recording that a title landed must never be able to stop it landing.
- **`text` is composed at the write point and stored**, not templated at read time. A feed row
  is a historical record: re-rendering it later against current data would let last week's entry
  change its wording when a channel is renamed, which is the opposite of an audit trail.
- **`level`** (`info` | `warn` | `error`) drives the mock's coloured dot. Bounded, not free
  text, so the UI cannot receive a colour it has no rendering for.
- **Read path:** `ListActivity(limit)` — newest first, and nothing else. The feed is a glance,
  not a query surface; a filterable log is a different feature (§20).
- **Retention:** `activity.retention` (§15), purged by the `activity-purge` job (§18.1). ⚠ The
  key is **consumed in the same PR that declares it**. `jobs.retention` and
  `proposals.retention` were declared long ago and are read by **nothing** — no purge exists
  for either table — which is the same dead-setting shape V12 found in `backup.retain`. Adding
  a third would make the Advanced settings page a list of promises. *(Those two remain open;
  they are a pre-existing gap, not this phase's.)*

⚠ **A recency signal cannot make repeats rare on a small library, and must not pretend to.** The
arithmetic is unforgiving: a 24h day consumes ~13 films, so a channel needs ~168h of content to
avoid repeating inside a week. The dev channel has 34 titles ≈ 62h — a 3-day no-repeat is already
*impossible* there, let alone 7. That is why placement consumes this as a **soft ranking signal**
(`programming-design.md` §3.1) rather than a hard constraint with a ladder step: a constraint that
is unsatisfiable on every real run produces a relaxation note on every real run, which teaches
operators to ignore the ladder. The signal spreads airings evenly and stops a title clustering near
its own last showing; only more content fixes the underlying frequency, which is what re-curation
and adjacency candidates (`programming-design.md` §8.2/§8.3) exist to supply.

### Retention & janitor
State accumulates; a **janitor** (piggybacking the reconciler ticker) enforces retention so a year-old install isn't dragging a landfill:
- **Sessions:** sliding TTL, `SESSION_TTL` default 30d; expired rows purged. (Without this, sessions live forever — both a growth and a security problem.)
- **Activity:** feed rows purged after `activity.retention` (default 30d) by the `activity-purge` job (§18.1, V32).
- **Jobs:** finished jobs (`done`/`failed`) purged after `JOBS_RETENTION` (default 30d) by the `retention-purge` job (§18.1). A `queued` or `running` job is never purged regardless of age — age is not evidence that work finished, and deleting a running job's row would strand the worker holding its lease.
- **Proposals:** `denied` purged after `PROPOSALS_RETENTION` (default 90d). ⚠ **`approved` and `submitted` are kept indefinitely**, for different reasons: an approved proposal is the audit trail behind `approved_by` (the record of a decision that spent real resources), and a `submitted` one is a member still waiting for an answer — ageing it out would silently discard a request rather than decline it.
  - ⚠ **Purge order is proposals, then jobs.** `proposals.job_id` has no foreign key, so the constraint is ours to keep: removing a job first would leave a proposal pointing at nothing. Verified that no read path joins the two (`job_id` is diagnostic provenance; the proposal endpoint does not resolve it), so an orphan is cosmetic rather than broken — but a purge that creates one on every run is a purge that makes the data harder to reason about for no gain.
  - *These two keys were declared long before they were read — the same declared-but-unconsumed shape V12 found in `backup.retain` and V32 avoided for `activity.retention`. This section described the purge as shipped for several phases while it did not exist.*
- Filler catalog sync (§10) already removes clips that vanished from the media server.

### Concurrency consequence of supporting Postgres (important)
SQLite ⇒ **single instance**. Postgres enables **replicas**, which changes reconcile correctness:
- `ClaimDueTitles` on Postgres uses `SELECT … FOR UPDATE SKIP LOCKED` (or an advisory-lock leader) so two replicas don't both fire a give-up/retry — the reason it's a distinct method, not a plain list.
- On SQLite it's a straight query.
- **Run exactly one replica with SQLite.** Scale horizontally only with Postgres + row claiming.

### Migrating SQLite → PostgreSQL (V11)

An install that outgrows SQLite can move without an export/import dance: **Settings → System →
Database** runs a six-stage stepper — connect → preflight → backup → migrate → verify → restart.
Only this direction is supported; the reverse is served by the backup file plus reverting one
config line.

**The invariant: the source is only ever READ.** Every failure mode ends with the operator still
running on the database they started on, which is what makes "roll back by reverting one config
line" true rather than aspirational. Nothing in the copy writes to, vacuums, or locks the source
beyond its read transaction, and the destination is never switched to until parity passes.

| Stage | What it does | Why it is its own stage |
| --- | --- | --- |
| Connect | Takes the target DSN | — |
| Preflight | Reachable · version ≥ 13 · **target is empty** · privileges · UTF8 encoding | Fails while failing is free; can send you back to fix the target |
| Backup | Writes a server-side snapshot into `backup.dir` | **A gate, not a step** — see below |
| Migrate | Copies every table, streaming progress as `database` SSE frames | — |
| Verify | Re-counts BOTH sides independently; a mismatch aborts | A copy that reports success is a claim; parity is the evidence |
| Restart | Persists the new `DATABASE_URL` to the bootstrap file | Copying data and changing which database the app answers from are different commitments |

Four rules that are not negotiable, each because the obvious alternative is subtly wrong:

1. **The backup gate is enforced server-side.** The UI disables Migrate until a backup exists, but
   that is a hint: anything a client can satisfy, a client can skip. The server refuses a migrate
   call unless *it* wrote a backup for this migration. This is why `WriteBackup` writes a real file
   rather than streaming one to the browser — a download leaves no evidence.
2. **The table list AND the copy order come from the destination catalog.** goose builds the
   destination from the same embedded migrations, so its live catalog *is* the schema by
   construction. A hardcoded list drifts (this repo's own `TRUNCATE` list had already drifted to 8
   of 10 tables), and the order is load-bearing: `sessions.user_id REFERENCES users(id)` is NOT
   DEFERRABLE, so a topological sort over the catalog's FK graph is what keeps inserts legal.
   `goose_db_version` is never copied — the destination earns its own.
3. **Values are coerced by the DESTINATION's column type.** Everything is scanned as a string,
   which makes the SQLite-INTEGER/Postgres-BOOLEAN divergence a non-event (both drivers parse
   `"0"`/`"1"` correctly). Binary is the real exception: `channel_icons.bytes` is BLOB/BYTEA, and
   routing it through a Go string corrupts every byte that is not valid UTF-8.
4. **Preflight refuses a populated target.** "Wipe it and retry" is safe advice only because this
   check guarantees there was nothing there to lose.

**An env-pinned `DATABASE_URL` makes this copy-only.** Env always wins at boot (§15), so writing
the bootstrap file would produce a switch that silently does not happen. The server refuses the
switchover and the UI says so up front, rather than after a backup and a full copy.

---

## 6. External contracts

### Client resilience defaults (apply to every adapter below)
Every outbound client is built from a shared HTTP factory with **hard timeouts**: media server 10s, Seerr 10s, TMDB 10s, Tunarr 20s (lineup pushes are chunky), LLM 120s per call. **Retry philosophy:** jittered-backoff retries only for idempotent GETs; *writes never client-retry* — write recovery is owned by the idempotent reconcile loops and periodic sweeps, which is why they exist. A down dependency degrades the relevant feature (and lights up the §13 checklist), never wedges the process.

### Library — Emby & Jellyfin
Both share `GET /Items?Recursive=true&AnyProviderIdEquals=<provider>&IncludeItemTypes=<Movie|Series>&Limit=1`; provider `tmdb.<id>` / `tvdb.<id>`; present iff `Items` non-empty → `Items[0].Id`. Use **header** auth (never `api_key` query param — leaks to logs; Jellyfin deprecates legacy auth from 10.11+):
- Emby: `X-Emby-Token: <key>` · Jellyfin: `Authorization: MediaBrowser Token="<key>"`

Flavor via `LIBRARY_FLAVOR`. **Season precision default:** `SEASON_PRECISION=series` — a series counts as in-library if the show exists; `seasons` mode (verify each requested season before `available`) is the stricter opt-in. Caveat to encode as a TODO: provider-name casing in `AnyProviderIdEquals` can differ across versions — if a known-present title returns empty, check casing first.

**Bulk scan (poll-based availability, §4 + §18.1).** Beyond the id-only `Lookup`, the library adapter exposes two bulk reads that drive the `library-scan`/`library-full-scan` jobs — one call returns *many* items with their provider ids, so availability is confirmed without an N-lookup storm:

- `RecentlyAdded(since)` — `GET /Items?Recursive=true&IncludeItemTypes=Movie,Series&SortBy=DateCreated&SortOrder=Descending&MinDateLastSaved=<since>&Fields=ProviderIds,ProductionYear`. The incremental 5-minute path: only what changed since the last scan.
- `AllItems()` — the same query without `MinDateLastSaved`: the periodic full sweep (safety net for anything the incremental window missed).

Both return `[]SearchResult` (the existing shape carrying `LibraryItemID` + `TMDBID`/`TVDBID`), one code path for both flavors (auth-only divergence, as with every `/Items` call). The scan job builds a `provision.Key` from each returned item and confirms any in-flight (`requested`/`downloading`) title whose key matches — applying `LibraryConfirmed` → `available`. Key parity (same key from a `Title`, a webhook, or a scan item) is what makes this correlation exact.

**Collections / BoxSets (for `scope.collections`, programming-design §2.2).** The same `/Items` surface, two reads:

- `Collections()` — `GET /Items?IncludeItemTypes=BoxSet&Recursive=true&Fields=ChildCount&SortBy=SortName`. Lists the operator's curated collections (id + name) so the channel editor can offer a picker. Emby and Jellyfin both model a collection as an item of `Type: "BoxSet"`.
- `CollectionMembers(id)` — `GET /Items?ParentId=<id>&Recursive=true&Fields=ProviderIds`. Returns the members as ordinary `Movie`/`Series` items, so membership maps onto a `provision.Key` with no second lookup — the same `[]SearchResult` shape and the same key-parity property the bulk scan relies on.

**Verified live against Emby 4.10** (2026-07-30), which settled the two open questions: `ParentId=<boxset>` DOES enumerate members (109 returned for a 109-title collection), and members carry full `ProviderIds` (`Tmdb`/`Tvdb`/`Imdb`), so membership maps onto a `provision.Key` with no second lookup. ⚠ **`ChildCount` is NOT returned by Emby even when requested via `Fields=`** — treat it as optional and render around its absence; it is on `LibraryCollection` because Jellyfin may supply it, never as something to rely on.

⚠ **Collections are not the small hand-curated set the name suggests.** A real library returns ~125, roughly 90% of them auto-generated one-per-franchise groupings ("Alien Collection"), because Kometa and similar tools write them in bulk. Any UI over this must assume hundreds and rank the hand-made ones first (§12); a flat list buries the handful that carry a decision.

**User auth & listing (for §11):** `POST /Users/AuthenticateByName` (body `{Username, Pw}`) validates a user's credentials — Jellyfin requires the `Authorization: MediaBrowser Client="…", Device="…", DeviceId="…", Version="…"` header on this request even without a token; Emby accepts the equivalent `X-Emby-Authorization`. `GET /Users` with the admin `LIBRARY_TOKEN` lists users (id, name, `Policy.IsAdministrator`, `Policy.IsDisabled`) for import/sync. Both live in the same flavored adapter as `Lookup`.

### Requester — Seerr (default) or direct Sonarr/Radarr

A `requester.provider` setting (`seerr` default, or `arr`) selects the acquisition backend. The wizard/Settings show the `seerr.*` fields OR the `sonarr.*`/`radarr.*` fields via `ShowWhen` on the provider (mirrors `llm.provider`). Both implement the same `Requester` port (`Request`/`Cancel`) plus a `Reachable` probe for the Test button; the app branches on the provider at construction.

**Seerr (default).** `POST {SEERR_URL}/api/v1/request` (header `X-Api-Key`), body `{mediaType, mediaId=TMDBID, seasons}`. Treat **201** and **409** as success (idempotency). Seerr supports Emby/Jellyfin/Plex natively. **Operational trap:** Seerr has its own approval workflow — if Loomarr's API user lacks auto-approve permission in Seerr, every Loomarr-approved acquisition stalls in a *second* pending queue and deadlines expire. The integrations doc (§13) must instruct: grant the Loomarr service user auto-approve in Seerr; the troubleshooting page covers the "everything stuck in `requested`" symptom.

**Direct Sonarr/Radarr.** Routes movie→Radarr, series→Sonarr (header `X-Api-Key`), each built dynamically like Seerr (conn closures, `httpx.TimeoutArr`). Per title:
- **Request:** `GET /api/v3/{movie,series}/lookup?term={tmdb,tvdb}:<id>` to resolve the arr's payload, then `POST /api/v3/{movie,series}` with `qualityProfileId` + `rootFolderPath` + `monitored:true` + an add-options search trigger. An already-added title (arr returns **400/409**) is treated as success — the same 2xx-or-conflict idempotency rule as Seerr.
- **Quality profile / root folder:** auto-picked (first of `GET /api/v3/qualityprofile` and `/api/v3/rootfolder`) unless overridden by `sonarr.quality_profile`/`sonarr.root_folder` (+ radarr). This keeps the common path zero-config while letting a power user pin them.
- **Cancel:** a *real* withdrawal (unlike Seerr's no-op) — DELETE the matching `GET /api/v3/queue` record so a given-up title stops downloading.
- **Reachable:** `GET /api/v3/system/status` on each configured arr — validates URL + key for the Test button (a dead host is a transport error, a bad key 401).

Availability under the direct requester comes from the same **library scan** (§4, §18.1) as Seerr, plus the arr **queue poller** (`arr-queue-poll`, §18.1): it reads each arr's `/api/v3/queue`, promotes a title with a live download record to `downloading` (`Grabbed`), and **persists progress on the title record** (`progress`/`eta_text`/`download_status`, surfaced by `GET /v1/titles`). Never an inbound webhook.

### Programmer — Tunarr
The scheduler drives Tunarr's REST API (documented OpenAPI at `tunarr.com/api-docs.html`): channels CRUD, programming/lineup, filler lists, flex, custom shows. **Decided: hand-write a thin client** against only the endpoints we use — generating from Tunarr's full pre-1.0 spec would couple us to its schema churn. Pin and record the Tunarr version tested against in the README. Tunarr ships with no authentication (Phase-0 finding: empty `securitySchemes`) and Loomarr reaches it machine-to-machine on the same network, so there is **no Tunarr API-key config** — just the URL. (An operator who fronts Tunarr with their own auth proxy would terminate that at the proxy; Loomarr does not model it.) Tunarr owns transcoding/streaming/EPG/HDHR+M3U output; `loomarr` owns lineup + filler. **Important:** Tunarr must have the same Emby/Jellyfin library configured as *its* media source **with that library enabled and scanned** — Tunarr streams the underlying files and indexes them into its own program table. `loomarr` and Tunarr agree on titles via the library.

**Loomarr wires this for the operator** (`POST /v1/setup/tunarr-connect`, admin — §7): it ensures the Emby/Jellyfin media source exists in Tunarr, then enables the movie + show libraries and triggers a scan — the same **enumerate-first, idempotent** pattern as the Live TV wiring (below) and filler (§10). It reuses Loomarr's **existing admin API key** as the source's access token (Tunarr accepts the admin `X-Emby-Token` directly — verified against Tunarr 1.3.8 + Emby 4.10; no separate Emby *user* login, so Loomarr stores no extra credential). This closes a **silent-failure** gap: without the scan, a channel's slots find no Tunarr program and degrade to flex/dead-air (§9), yet the rest of the wizard reads all-green — so `/v1/setup/status` carries a **`tunarr_library`** check that fails until the source is wired + scanned. Division of labor is unchanged (§6/§1): Tunarr still owns streaming/indexing; Loomarr only performs the one-time setup the operator would otherwise do by hand. It does **not** poll or re-scan on a timer — the one exception is *demand-driven*: a reconcile that finds a program slot Tunarr hasn't indexed yet triggers a library scan so that content becomes playable (§ Content-id resolution below), which is a targeted response to real unresolved content, not a routine background sweep.

**Content-id resolution (Programmer adapter).** A lineup slot carries the *media-server* item id (the Emby/Jellyfin id, from the provisioner). Tunarr's manual-programming API does **not** accept that id directly — a programming entry references Tunarr's *own* program id (a stable uuid Tunarr assigns when it scans the item). So the adapter resolves media-server-item-id → Tunarr-program-id before pushing: it reads Tunarr's persisted library index (`GET /api/media-libraries/{libraryId}/programs`, where each program carries `identifiers[{type:"emby"|"jellyfin"},…]` + its uuid) and builds a cached `{external item id → program uuid}` map (refreshed on a miss / TTL). This covers movies and TV episodes uniformly (episodes are indexed individually; a series pick expands to its episodes' ids). If a slot's item isn't in Tunarr's index yet (library not scanned, or a just-landed acquisition Tunarr hasn't picked up), that slot degrades to flex — never dead air (§9) — and resolves on a later reconcile once Tunarr has scanned it. **To close that gap without waiting on Tunarr's own scan cadence, a reconcile that produced any unresolved program slot triggers a Tunarr media-library scan** (`POST /api/media-sources/{src}/libraries/{lib}/scan`, each non-`local` source's enabled libraries) so the next reconcile finds the now-indexed episodes and promotes those flex gaps to real content. It is **debounced** (one scan pass per reconcile-that-had-misses) and **best-effort** (Tunarr scanning is async + idempotent, the flex fallback is already on-air, and a scan failure never fails the push). NB: Tunarr's browse endpoint (`/api/emby/{src}/libraries/{lib}/items`) returns *ephemeral* handles that change per request; only the persisted `/programs` index yields ids valid for programming.

### Live TV wiring — Tunarr → Emby/Jellyfin (tuner + guide)

For Loomarr's channels to appear in the family's TV guide, the media server must consume Tunarr's **tuner + guide** surface. This is **one-time wiring of Tunarr as a tuner/guide source — never per-channel registration.** Once wired, every channel Loomarr creates/renames/deletes propagates through Tunarr's M3U/XMLTV output; Loomarr then pokes the media server so the change appears in minutes rather than after its nightly refresh. **The poke is operation-specific (§9):** a *new or removed* channel needs a **tuner re-scan** (re-read the M3U channel list — a guide refresh alone won't surface it); an *existing* channel's lineup change needs a **guide refresh** (EPG data).

- **Endpoints (both flavors, Emby lineage):** `POST /LiveTv/TunerHosts` (type `m3u`, `Url` = Tunarr's playlist URL) and `POST /LiveTv/ListingProviders` (type `xmltv`, `Url` = Tunarr's guide URL), using the admin `LIBRARY_TOKEN`. **M3U is preferred over HDHomeRun emulation** — explicit and discovery-free, so registration is deterministic.
- **One-time & never silent.** There is no per-channel media-server call, ever. Wiring is an explicit operator action — the wizard's one-click "Connect Tunarr to Emby/Jellyfin" (`POST /v1/setup/livetv-connect`, admin — §7) or the manual runbook step (§16). Loomarr never reconfigures a media server unasked.
- **Idempotent & self-healing on URL change.** Enumerate first via **`GET /System/Configuration/livetv`** — one read that returns `{TunerHosts, ListingProviders}` — and if Tunarr is already registered the connect is a no-op. Duplicate tuners are a classic Emby mess; tests assert **second-call-no-op** (Phase 10 gate). **Reconcile is by *identity*, not URL string.** Loomarr tags every tuner it registers with `FriendlyName: "loomarr"`, so `Connect` owns exactly the tuners it created: when the Tunarr URL *changes* (the operator repoints `TUNARR_URL`), enumerate-first finds a Loomarr-owned tuner whose `Url` no longer matches the desired M3U, **DELETEs the stale one** (`DELETE /LiveTv/TunerHosts?Id=<id>` → 204, Phase-0 capture), and registers the new — so a URL change *moves* the tuner instead of orphaning a dead one alongside a live one. A tuner the household added by hand (any other `FriendlyName`) is **never touched** (§9 ownership: Loomarr owns only what it created). Listing providers carry no `FriendlyName`, so the stale one is identified as the Loomarr-shaped `xmltv` provider whose `Path` is a Tunarr guide URL that no longer matches; it is likewise DELETEd (`DELETE /LiveTv/ListingProviders?Id=<id>` → 204) and re-added. **A connect that changed anything (added or retired a tuner/listing) then pokes the media server — a tuner re-scan *and* a guide refresh — so the freshly-registered tuner's channels are discovered and their EPG populated immediately, rather than after the media server's nightly scan** (the newly-wired tuner has zero channels in the media server's view until it re-reads the M3U — a guide refresh alone won't surface them; §9 poke semantics). Both pokes are **best-effort**: a poke failure degrades freshness but never fails the wiring. A no-op connect (nothing changed) skips the pokes — there is nothing new to discover. *The Emby-lineage `GET /LiveTv/TunerHosts` / `GET /LiveTv/ListingProviders` are **write-only on Jellyfin** — `POST` works, `GET` returns **405** (verified against Jellyfin 10.10.3). Enumerating through them therefore failed on every Jellyfin install, so the idempotency check could not run and the connect either errored or duplicated the tuner on each attempt. The Phase-10 capture was Emby-only, which is how it survived: §6 claims both flavors, and only Emby was ever exercised. The config endpoint answers 200 on **both**, so this is one code path rather than a flavor branch.*
- **Version fragility → live capture.** The endpoints exist on both flavors, but **payload fields and the guide-refresh task id drift across versions.** A Phase-0-style maintainer-supervised capture (folded into Phase 10, §21) pins the exact accepted request/response payloads + the guide-refresh task id from the real Emby/Jellyfin into `internal/testkit/fixtures/`; the adapter is written against those pins, not memory. Any contract deviation ⇒ update this doc first.
- **Division of labor is unchanged (§1 non-goals):** Loomarr decides *what plays and when*; Tunarr owns playout/transcode/EPG and the HDHR/M3U/XMLTV tuner surface; Emby/Jellyfin consume that tuner + guide like any HDHomeRun. Loomarr never builds streaming; the escape hatch is a second `Programmer` adapter (ErsatzTV).

### Suggester / Catalog — LLM
See §8. Provider-neutral; Ollama (local) or any OpenAI-compatible endpoint (hosted); catalog tool grounds it against the real library + TMDB. In-app provider/model selection with live probe + hot-swap: §8.1.

---

## 7. HTTP API

| Method | Path | Purpose |
| --- | --- | --- |
| POST | `/v1/titles` | Enqueue/ensure a title. Idempotent. |
| GET | `/v1/titles/{key}` | Provisioning state for a title. |
| GET | `/v1/titles?state=…` | List, filter by state. |
| DELETE | `/v1/titles/{key}` | Give up / cancel. |
| POST | `/v1/channels` | Create a channel (admin). From an **approved proposal** (`intentRef` → its grounded lineup + policy), a **hand-made single-series** channel (`series`), or an **empty hand-made** channel (neither given → name/number/strategy only, no lineup — then fill it via the manual lineup editor or Refine-with-AI on its page). The channel **`id` is optional**: the client may pass a stable caller-assigned id, or omit it and the **server assigns one** (`ch_…`) — so the UI's "New channel" action needs no client-side id scheme. Duplicate id or number → 409. |
| GET | `/v1/channels` / `/v1/channels/{id}` | Channel definition + current status. The **single-channel** GET additionally resolves each lineup entry's `state` — `available` (in the library, plays now), `acquiring` (wanted/requested/downloading — on its way), `pending` (added but nothing requested it yet — the manual-add case with no provision Record), or `unavailable` (acquisition gave up) — derived from the `provision.Record` per key, so the editor's "not here yet" badge is durable across reloads rather than known only at add-time. The list endpoint omits per-entry state (its cards show counts, not entries) to avoid an N-query fan-out. |
| PATCH | `/v1/channels/{id}` | Edit a channel (admin). A **partial** update of the identity fields (`name`, `number`, `group`, `logo`), the per-channel programming `policy` (§8, `programming-design.md`), `status` for pause/resume (`paused`↔`building` only), and the **`lineup`** entries. Renumber is unique-checked (409 on a collision). `policy.applied` is reconcile-owned and rejected on write. **Policy ownership is sticky (§8.2 / `programming-design.md` §2):** the `policy` blob mixes proposal-owned fields (scope, audience, ordering, separation, seasonal — the suggester extracts them) with operator-owned ones (filler, window, auto-curate). A PATCH that sets a proposal-owned field **marks it operator-set**, and a later refine/re-curate then **cannot overwrite it** (untouched fields still refresh from the fresh proposal); the audience ceiling is additionally never relaxed. Curation `rules` carry **provenance** (`llm` or `operator`): a refine replaces only the LLM-authored rules and preserves the operator's, so shaping rules by refine and by hand compose instead of clobbering. **Every edit auto-reconciles** (best-effort, like create) — there is **no manual "rebuild" step**; the change reaches Tunarr on its own and the UI reflects it live via the `channel` SSE frame. **Editing the lineup** is a **whole-list replace** (the client sends the full ordered list of entries): add = a new entry, remove = an omitted entry, reorder = the same entries in a new order — one idempotent payload, diffed server-side. Each entry's `key` is validated (`provision.ParseKey`; a malformed key → 422). The handler mutates `ch.Lineup` only and lets the reconcile derive `desired` — it never computes slots itself (§9). Entries that carry richer scheduling metadata already on the channel (a series' season range, `officialRating`, runtime) are **preserved by key**: an incoming entry whose key already exists keeps that metadata; the read DTO is deliberately lossy, so a reorder must not silently drop a season scope. **Safety (prime directive #3):** a manually-added key that is not `available` in the library is inert — it renders as a **pending slot** (flex to Tunarr, never content) and swaps to a program in place only if/when that title independently reaches `available` through the acquisition pipeline. Manual editing therefore cannot make unapproved content play, and does not itself trigger acquisition. |
| POST | `/v1/channels/{id}/refine` | Refine a channel with the LLM (admin). Takes free-text ("add more Schwarzenegger, drop the slow ones"); builds an intent from the channel's **current lineup** + the ask, runs the SAME grounded suggester → proposal (§8), and returns a `jobId`. Approving that proposal patches THIS channel (idempotent on its `intentRef`, same as re-approval). The review shows a **diff** (kept / added / removed); an approve step appears **only if the re-proposal actually differs**. This is the "shape a channel over time by talking to it" path — no create-a-channel-from-scratch detour. |
| GET | `/v1/channels/{id}/pods` | Preview the commercial pool this channel would get from its **saved** filler selection (§10, §12). Assembles WITHOUT touching Tunarr, through the same code path and seed as reconcile, so preview and reality cannot disagree. Returns the ordered entries, total duration, and the `matchLevel` reached on the fallback ladder — the answer to "why are my commercials wrong". Read-only, so any authenticated user may call it. |
| POST | `/v1/channels/{id}/pods/preview` | Preview the pool a **draft** filler selection would produce, WITHOUT saving it (§10, §12) — the live sandbox on the channel's Filler section. Body is a `FillerSelection` (the unsaved draft); the handler runs the SAME assembler as `GET …/pods`/reconcile but with the draft in place of the persisted selection, and returns the SAME shape (entries + `matchLevel` + total). One assembler ⇒ the sandbox shows exactly what will air once applied. Admin-only (it's an authoring tool); applying is a normal `PATCH …/{id}` of `policy.filler`. |
| POST | `/v1/channels/{id}/programming/preview` | Preview a **whole-definition draft** `{lineup?, policy?}` (§8.1) — the cycle slots (which rule wins at `at`, the resolved window) **and** the assembled break pool — WITHOUT saving or touching Tunarr. Generalizes `GET …/cycle` + `POST …/pods/preview` into one call over an unsaved edit; runs the SAME `ComputeDesiredAt` + pod assembler as reconcile, so the preview can't drift from what applying it would ship. Omitted lineup/policy fall back to the saved value. Admin-only (an authoring tool). |
| GET | `/v1/programming/vocabulary` | The closed WHEN/WHAT/HOW curation-rule presets (§6.6): each token + label + the value the BE lowers it to. The rules editor renders its picker from this and lowers identically to the server, so the FE no longer hand-mirrors the lowering table (drift-killed). Read-only; any authenticated user. |
| POST | `/v1/channels/{id}/reconcile` | Force desired→Tunarr reconciliation. Internal/idempotent (§9); NOT a user-facing "rebuild" — edits reconcile automatically, and the periodic sweep is the guarantee. |
| DELETE | `/v1/channels/{id}` | Remove channel; `?purge=true` also deletes the Tunarr channel (default detaches only). |
| POST | `/v1/proposals` | Start a suggestion job from an intent. |
| GET | `/v1/proposals?status=…&mine=true` | List proposals by status (`submitted` = the admin approval queue). **`mine=true` scopes the list to the caller's own proposals** — the "My requests" surface (§12). Scoping is resolved **server-side from the session**, never from a client-supplied user id: a `?user=` parameter would let any member read another's requests by editing a URL. A break-glass `API_TOKEN` caller has no user record, so `mine=true` returns **empty** rather than everyone's — an unscopable caller asking for "mine" must not silently receive all. |
| GET | `/v1/proposals/{id}` | Job status + proposal (the source of truth on SSE reconnect; generation progress streams over `/v1/events` as `suggestion` frames, not a per-job endpoint). |
| POST | `/v1/proposals/{id}/approve` | Approve (admin) → enqueue acquisitions **+ create/patch the channel**, returning its id. This is the primary path from an approved intent to a live channel — §13's flow is describe → review → approve, so **the everyday way to make a channel is to describe one, not fill out a form** (the two other origination seeds — a hand-made single-series or empty channel via `POST /v1/channels` — are express doors into the same object, not a separate "create screen" model; see §12 origination-vs-evolution). **Idempotent on `intentRef`** (the suggestion job id): re-approving the same intent patches that channel rather than minting a second one. The channel is created `building` with the proposal's lineup + grounded policy, then reconciled immediately (§9 "live immediately — never dead air"). **Number** = the lowest free positive integer, so an operator never has to think about numbering to get on air; **name** = the intent description, trimmed to a channel-sized label. Both are ordinary editable fields afterwards (§7 `PATCH /v1/channels/{id}`) — the point is that approving is *sufficient*, not that the derived values are final. A channel is **shaped over time** after creation: direct edits (name/number/rules/lineup) via `PATCH`, or by *refining* it with the LLM (`POST /v1/channels/{id}/refine` → review the diff → approve → the same idempotent patch). |
| POST | `/v1/proposals/{id}/deny` | Deny (admin) with optional reason; proposal → `denied`, member sees it in My proposals. |
| GET | `/v1/filler` | List clip catalog; filter by kind/era/audience/category/untagged, plus `q` for a `name LIKE` search (§7.2 — clip search lives here, not in `/v1/search`). |
| PATCH | `/v1/filler/{id}` | Edit a clip's tags — including **confirming an era suggestion** (§10): setting `era` on a clip that carries one clears the suggestion. The suggestion itself is never settable directly; only the tagger writes it, and only when the year is not in the source text. |
| GET | `/v1/filler/discover` | Browse clips the operator could add, **downloading nothing** (admin, §10 V33/V17d). Two mutually exclusive modes: `q` searches archive.org by keyword; `collection` lists one named collection (a URL, a `/details/<id>` path, or a bare identifier). The `collection` mode is what a **starter pack** is — a curated collection listed for keep/exclude before anything is fetched — so browsing a suggested pack and browsing a search result are one code path, not two. Neither mode requires the ingest tooling: listing is plain `net/http`, so an operator on a degraded install can still see what exists and learn why the fetch is unavailable. Licence availability is stated **once, about the search** — archive.org declares one on ~8% of items, so a per-row chip would imply a check that never happened (build plan §6.3). |
| POST | `/v1/filler/sync` | Sync catalog from the Tunarr `local` filler source (§10). |
| POST | `/v1/filler/ingest` | Download clips into the drop-folder from a playlist/collection/video URL (admin). Runs as a job; progress on `/v1/events`. 409 `feature_not_configured` if the vendored ingest tooling isn't runnable — it ships in the single image (§10, §16), so this is a degraded-install signal, not an opt-in gate. |
| POST | `/v1/filler/tag` | Start an AI-tagging job over untagged clips (§10). |
| POST | `/v1/filler/{id}/split` | Propose splits for a compilation clip (admin, §10 V34). Runs detection — chapters → `blackdetect`/`silencedetect` → transcript rescue for over-long segments — as a **job** (minutes per file; progress on `/v1/events`), producing a persisted **split proposal**: the cut points, per-segment duration/tags (era suggestions marked unconfirmed when the year is not in the text), and dedup flags (a segment whose dHash matches an existing clip). **Nothing enters the catalog here** — review is not optional, because detection quality is a property of the source (§10). |
| GET | `/v1/filler/splits/{proposalId}` | Read a split proposal (admin, §10 V34) — the source of truth on SSE reconnect, the same pattern as `/v1/proposals/{id}`. |
| POST | `/v1/filler/splits/{proposalId}/confirm` | Commit a reviewed split (admin, §10 V34). The body is the operator's confirmed cut list — the proposal as returned, possibly edited (cuts moved, merged, or dropped; era suggestions accepted or rejected; dedup-flagged segments kept or skipped). Only now do segments become catalog clips: cut with ffmpeg stream copy (no re-encode), classified from their transcripts, written into the drop-folder, and the **original compilation row removed** — its identity is a path that now means twenty clips, not one. |
| GET | `/v1/filler/media/{path...}` | Stream a clip's own bytes for in-app preview (§10 V35). Sibling of `/v1/filler/thumb/{path...}` and confined the same way — the path is resolved inside `FILLER_DIR` and anything escaping it is refused before the file is opened. Served with `http.ServeContent`, so Range and conditional requests work and a `<video>` element can seek. ⚠ **Deliberately not named `preview`**, for the reason `/thumb` is not: "preview" already means a pod listing in two places (build plan §6.2). |
| GET | `/v1/filler/pool` | Catalog-wide filler health (§10 V35) — how well the catalog can actually resolve breaks, plus what is thin. ⚠ **Computed over the same pools pod assembly uses** (`internal/filler`), never a second implementation: a meter that agrees today and drifts next quarter is worse than none, which is why the per-channel `/v1/channels/{id}/filler/coverage` was built the same way. |
| GET/POST | `/v1/filler/sources` | List sources, or add one (admin, §10 V35/V37/V38c). **One flat list, one row per source.** A POST carries `{kind, uri, label?}` — ⚠ `kind` is required and validated **per kind** (an archive identifier, a YouTube playlist URL, an absolute folder path and a media-server library name are not interchangeable). ⚠ **V38c: `folder` and `library` are ADDABLE and no longer singletons** — many watched folders and many scanned libraries are supported, so the partial unique index and the 409 that enforced one-of-each are both gone. |
| PATCH/DELETE | `/v1/filler/sources/{id}` | Enable/disable, tune, or remove a source (admin, §10 V35, extended V38c). ⚠ Disabling withdraws a source from future scanning, searching and downloading — **it never removes clips already in the catalog**, and the enforcement lives at those three sites rather than in the UI. The PATCH body also carries the per-source fetch overrides: ⚠ `fetchEverySeconds` is **three-state** — omit/`null` inherits the global, `0` means *never auto-fetch this source*, a positive value is an interval. `fetchMaxPerRun` has a **minimum of 1**, because "fetch nothing per run" is what `fetchEverySeconds: 0` already says and saying it twice invites the two to disagree. |
| GET | `/v1/filler/watch` | **The Filler header's live status (§10 V38c).** Returns `{health, sourcesOn, sourcesTotal, clips, lastScanAt?}` — everything the page header's pill renders, computed on the SERVER. ⚠ **`health` is `healthy` / `attention` / `unconfigured`, and the server owns that judgement.** Deriving it in the client was tried first and rejected for two reasons: the rule ("all sources dark", "nothing has arrived in days") is real domain logic that belongs where it can be tested against the store rather than against a hand-built fixture array, and `/v1/filler/sources` is **admin-only** — so a member's pill would have been permanently grey while their channels played fine. ⚠ **Member-readable**, like `/pool` and the catalog listing, and for the same reason: it explains what the channels are doing. It names no filesystem paths or library targets, which is what keeps it safe to widen — the counts and the verdict, never the infrastructure. |
| GET | `/v1/filler/incoming` | The ingest conveyor (admin, §10 V35): clips whose tags need a human, and compilations mid-split. One read behind the Filler page's Incoming tab, so a restart cannot lose the queue. ⚠ Reports **no confidence score** — nothing measures one; each item carries the reason it is waiting, derived from real state. |
| POST | `/v1/filler/bulk/tag` | Retag a selection (admin, §10 V35). Each tag field is **independent** — omitting one leaves it alone, so setting only the audience never blanks an era. Setting an era confirms an outstanding suggestion through the **same** path the single-clip edit uses. A selected clip that no longer exists is counted, not fatal: a selection races a re-scan. |
| POST | `/v1/filler/bulk/remove` | Remove a selection from the catalog (admin, §10 V35). ⚠ **A tombstone.** The clip leaves the catalog and stops being used in breaks; **the file is untouched**, and the mark survives a re-scan (which a row delete could not). `restore:true` undoes it. |
| POST | `/v1/filler/pulls` | Propose a **pull** — a plan across sources (admin, §10 V35). **Downloads nothing**: it writes a proposal for the approval queue. Refused when every source the plan needs is disabled, with the switch to flip named. |
| GET | `/v1/filler/pulls` | List pulls awaiting a decision (admin, §10 V35). |
| POST | `/v1/filler/pulls/{id}/approve` | Approve a pull (admin, §10 V35) — the commit point. Enqueues through the **existing** ingest path; a pull never downloads by its own route. The body carries the operator's edits: rows dropped, and a note narrowing what to fetch. |
| POST | `/v1/filler/pulls/{id}/dismiss` | Decline a pull (admin, §10 V35). |
| POST | `/v1/auth/login` | Sign in with media-server credentials (§11) → session cookie. |
| POST | `/v1/auth/dev-login` | **Development only, default OFF.** Issues an admin session with no credential (§11). Registered **only** when `LOOMARR_DEV_LOGIN=1` is set on the server; otherwise the route does not exist and any call 404s. Selects the lowest-id existing admin — never creates or promotes one, so the allowlist invariant holds. |
| POST | `/v1/auth/logout` | End session. |
| GET | `/v1/auth/me` | Current user + role + quotas. |
| GET | `/v1/users` | List users (admin). |
| PATCH | `/v1/users/{id}` | Role / quotas / disable (admin). |
| POST | `/v1/users/sync` | Import/sync users from the media server (admin). |
| GET | `/v1/users/{id}/sessions` | List a user's live sessions (admin). Each is `{id, userId, createdAt, expiresAt, current}` where `id` is the stored token **hash** — the revocation handle, never a token. `current` marks the caller's own session so an admin does not sign themselves out by accident. |
| DELETE | `/v1/sessions/{hash}` | Revoke one session (admin). **Idempotent** — revoking an already-dead session succeeds, because the list an admin clicks from can go stale between render and click. |
| GET | `/v1/users/candidates` | List media-server accounts available to import, each flagged `imported` (admin). The read side of §11's explicit-import model — the wizard's step-5 picker and Settings→Users render from this; without it an admin would have to know raw media-server user ids. |
| GET | `/v1/setup/status` | Run the connection checklist; structured pass/fail per integration (admin; powers the wizard + Settings troubleshooting, §13). Each check is `{name, ok, hint, docHref}` where `docHref` deep-links its Troubleshooting section. Covers the connection probes (`media_server`, `requester`/Seerr, `tunarr`, `llm` incl. tool-calling, `tmdb`, and `filler` when configured), the wiring checks `livetv` ("Tunarr wired as tuner + guide in the media server") and `tunarr_library` ("Tunarr's Emby/Jellyfin source is wired + scanned" — §6). |
| GET | `/v1/setup/state` | **Unauthenticated.** `{bootstrapped: bool}` — whether the install has an owning admin yet (§11). Exists so the frontend can route a first-run visitor to the wizard instead of a login they cannot pass: the app is a static bundle, so with no unauthenticated signal every entry point resolves to `/login`, and a brand-new install has no account to log in with — a dead end that only an operator who guesses `/wizard` escapes. Deliberately the *only* fact it exposes, and it leaks nothing `POST /v1/setup/bootstrap` doesn't already reveal by answering 409-vs-created. It carries no counts, no names, and no configuration. |
| GET | `/v1/docs` | The Help table of contents: the embedded pages' slugs + titles (§13). Any authenticated user. |
| GET | `/v1/docs/{slug}` | One help page as **raw markdown** — the frontend renders it and searches it client-side (§7.2), which needs the source, not a rendered blob. Any authenticated user. |
| GET | `/v1/system/version` | Version/commit/build time + the readiness `/readyz` reports, plus what the **About** page (§16, V12) shows an operator writing a bug report: **Go runtime + os/arch**, **`startedAt`** (the process start, from which the UI derives uptime), and the **applied schema version** with the backend. The typed, authenticated twin of the ops probes: `/healthz` and `/readyz` stay OUTSIDE the versioned API and unauthenticated, because their consumers are Docker `HEALTHCHECK` and orchestrators, which hold no session — putting auth in front of a container health probe to satisfy the typed client would be the wrong trade. ⚠ **`startedAt`, never a pre-computed uptime**: a duration is stale the moment it is serialized, so the server sends the instant and the client renders the elapsed time it can keep current. |
| POST | `/v1/setup/test` | Run one named check (powers per-block Test buttons; `config-design.md` §8). |
| GET | `/v1/settings` | Settings registry with per-key provenance; secret values masked (admin, §15). |
| PATCH | `/v1/settings` | Update settings; validates, persists, hot-applies; env-pinned keys rejected (admin). An empty value clears an optional key — except a secret, which is replace-only (`config-design.md` §9). |
| DELETE | `/v1/settings/{key}` | Explicitly clear a key's stored override (reverts to env/default); the only way to unset a secret. 204 · 404 unknown · 409 env-pinned (admin, `config-design.md` §8). |
| GET | `/v1/settings/secrets/{name}` | Reveal a displayable generated secret (admin; API_TOKEN per §4's eye-toggle. SESSION_SECRET returns `displayable:false`, value withheld). Reading never rotates. |
| POST | `/v1/settings/secrets/{name}/regenerate` | Regenerate a generated secret (admin; SESSION_SECRET regen invalidates sessions). |
| POST | `/v1/setup/livetv-connect` | One-time wiring of Tunarr as an M3U tuner + XMLTV guide source in Emby/Jellyfin (admin; idempotent — §6). |
| POST | `/v1/setup/tunarr-connect` | One-time wiring of the Emby/Jellyfin library as *Tunarr's* media source: ensure the source (Loomarr's admin token), enable the movie/show libraries, trigger a scan (admin; idempotent — §6). Distinct from `livetv-connect` (opposite direction: Tunarr→media-server vs media-server→Tunarr). |
| GET | `/v1/system/llm` | Probe the LLM host + machine and recommend a provider/model (admin, §8.1): active provider + model, the **local** model catalog (for `ollama`: detected VRAM/version, per-model fit + recommended default, pulled flags), and the **hosted** catalog (OpenRouter + a Custom template — base URLs, recommended models, `keyConfigured`). API keys are never returned. Read-only. |
| POST | `/v1/system/llm/select` | Set the active provider + model (admin, §8.1). Persists to the settings store and **hot-swaps** the running suggester (no restart); settings override the §15 env defaults. Local model must be pulled (409 else). Hosted: accepts an optional `apiKey`, **validates** it live before committing (401/502 on a bad key), stores it as a secret (never echoed). |
| POST | `/v1/system/llm/test` | Validate a hosted provider + key **without** swapping (admin, §8.1) — the "test my key" check. Returns reachable/authorized + an error hint on failure. |
| POST | `/v1/system/llm/pull` | Start an Ollama `pull` of a model (admin, §8.1; **local-only**, 409 on a hosted provider). Accepts **any** tag — a catalog model, a browse-to-download `pullRef` (`hf.co/…`), or a hand-typed name. Returns a job id; progress streams over `/v1/events` (percent-complete). Idempotent. |
| GET | `/v1/system/llm/discover` | The **downloadable** local models that are **compatible with this machine**, ranked best-first (admin, §8.1). Takes the most-popular GGUF repos from a live source (Hugging Face, §14), sizes each against detected VRAM (the repo's Q4_K_M-class build — what Ollama's `latest` resolves to and what actually downloads), drops repos too big for the machine, and returns each with a bare `pullRef` (`hf.co/<repo>`, implicit `:latest`) to hand to `/pull`. Tool-capability is confirmed only **after** pull + probe. No keyword — it's the compatible set. Best-effort — a source outage returns an empty list (browse on huggingface.co instead), never a 5xx. |
| GET | `/v1/search?q=&scope=` | Federated search (§7.2): library + TMDB. Any authenticated user. Clips are not a scope — see §7.2; use `/v1/filler?q=`. |
| GET | `/v1/backup` | Stream a consistent DB snapshot (admin; SQLite backend — §16). Postgres → 501 + pg_dump docs. Generates a fresh snapshot and keeps nothing; see `/v1/system/backups` for the ones on disk. |
| POST | `/v1/system/restart` | Restart Loomarr in place (admin, §9.2, V13). Drains HTTP, tears down playout sessions by process group, closes the store, and rebuilds every subsystem in the **same process** — no re-exec, no supervisor needed, works identically on Windows. Responds **before** the drain begins, since a client that never gets a reply cannot tell "restarting" from "crashed". |
| GET | `/v1/system/restart` | What a restart would cost right now (admin, §9.2, V13), so the confirm dialog states consequences rather than guessing: the count of channels **Loomarr is currently streaming** (from `/v1/playout/sessions`) which drop for a few seconds, versus Tunarr-backed channels which keep playing (§9.1), plus whether any boot-time setting is pending (`restartRequired`). |
| GET | `/v1/system/services` | The Dashboard's **Services** panel (admin, §12, V31): one row per configured integration with its probe result and the **target** it was probed against, plus a `loomarr` row carrying version/backend/schema. Runs the **same `runConnectionChecks`** the wizard checklist and `/v1/system/reload` use — one probe implementation, asserted by a test, so three surfaces cannot disagree about whether Emby is reachable. |
| GET | `/v1/activity?limit=` | The Dashboard's **Recent activity** feed (admin, §12, V32), newest first. Reads the persisted `activity` table (§5) rather than the SSE bus, so the feed survives a restart and is not subject to the bus's deliberate lossiness. |
| POST | `/v1/system/reload` | Re-probe every configured service without restarting (admin, §9.2, V13) — reuses the **one** `POST /v1/setup/test` probe implementation rather than a second copy, so a reload and the wizard's checklist can never disagree. No downtime: nothing is torn down. |
| GET | `/v1/system/backups` | List the backups on disk in `backup.dir`, newest first (admin, §16, V12): filename, bytes, `writtenAt`. Also reports `dir`, `retain`, `schedule`, and `supported` (false on Postgres, where the listing is empty and the UI explains `pg_dump` rather than showing an empty table). Never 5xxs on a missing/unreadable directory — nothing written yet is an empty list, not an error. |
| GET | `/v1/system/backups/{name}` | Download one **already-written** backup by filename (admin, §16, V12). `name` is validated against the `loomarr-<timestamp>.db` pattern and resolved inside `backup.dir` — it is a client-supplied path segment, so anything else is rejected before it reaches the filesystem. |
| GET | `/v1/events` | SSE stream of state changes. Frame `event:` types: `title` (provisioning) · `channel` (lineup/health) · `suggestion` (generation progress — `searching`→`reasoning`→`scoring`→`done`/`failed`, payload `{jobId, phase}`) · `llm_pull` (model-download percent) · `filler_ingest` (clip-download progress — §10) · `filler_split` (compilation-detection progress: `running` → terminal `success` carrying `{proposalId, segments}` or `error` — §10, V34) · `activity` (a new Dashboard feed row was written — §12, V32). Latency-only: a dropped frame is never a correctness bug — the `GET` endpoints are the source of truth on reconnect (§8). |
| GET | `/openapi.json` / `/openapi.yaml` | OpenAPI 3.1 spec. |
| GET | `/docs` | Interactive API docs (self-hosted assets). |
| GET | `/healthz` / `/readyz` / `/metrics` | Ops. |
| GET | `/debug/pprof/*` | **Development only, default OFF.** Go's standard profiler handlers (CPU, heap, goroutine, mutex), mounted **only** when `LOOMARR_PPROF=1` is set on the server; otherwise the routes do not exist and any call 404s — the same not-registered-is-the-gate posture as `/v1/auth/dev-login` (§11). Unauthenticated by nature (a profiler holds no session), which is exactly why it is boot-time and off by default: it exposes stack traces and memory contents, and a repeated CPU profile can degrade a running server. Boot WARNs while it is on. |

**Authorization model:** every `/v1/*` route requires a session cookie or `Authorization: Bearer ${API_TOKEN}`; approval, destructive-channel, user-management, and filler-ingestion routes additionally require `admin` (§11) — **and so do `POST`/`DELETE /v1/titles`**, since enqueuing an acquisition directly is exactly what the approval gate exists to control (members reach acquisition only via submit→approve). Read visibility is global for all authenticated users — this is a household-scale app, and members see all channels and titles. SSE endpoints authenticate via the same cookie (EventSource sends cookies same-origin). `/healthz`, `/readyz`, `/metrics`, `/openapi.*`, and `/docs` are unauthenticated on the LAN.

### 7.1 Self-documenting API (OpenAPI)
Single source of truth: spec, request validation, and served docs all derive from the same operation definitions — hand-maintained docs are disallowed (they drift).

**Decided — code-first with Huma v2** (see §14): define each operation once (Go input/output types + tags); Huma emits OpenAPI 3.1, validates inputs from the same schema, and serves the docs UI, mounted on stdlib `net/http` via `humago`. Rejected: spec-first `oapi-codegen` (contract-review ceremony we don't need with a committed exported spec) and annotation-first `swaggo` (comments rot — weakest drift guarantee).

**Requirements:** OpenAPI **3.1** at `/openapi.{json,yaml}`; interactive docs at `/docs` with **bundled assets** — note Huma's default docs page loads Stoplight Elements **from a CDN**, which violates the offline rule: override the docs handler to serve self-hosted assets (works air-gapped on LAN); every operation has summary/description/operationId/tags + an example; schemas generated from domain types (`Title`, `Record`, `State` enum, `Channel`, `Proposal`, `Clip`, `Pod`, RFC 7807 error) — the spec `State` enum must equal the code enum; `make openapi` exports and commits `api/openapi.yaml` (diffed in review, published as CI artifact).

### 7.2 Search (federated, no index)
**Decision: Loomarr builds no search index.** Every searchable corpus is already indexed by its owner: the media server exposes `SearchTerm` on the same `/Items` surface as §6 (with `IncludeItemTypes` + `Recursive=true`, flavor auth as usual); TMDB has `/search/multi`; the clip catalog is thousands of rows where a `name LIKE` filter in the store suffices. Dual-dialect full-text (SQLite FTS5 *and* Postgres tsvector, which diverge substantially) to re-index data we don't own is explicitly rejected — revisit only if enormous filler catalogs demand it (§20).

`GET /v1/search?q=&scope=library|tmdb|all` fans out accordingly and returns unified `Candidate` results (external ids, library item id when present, `in_library` flag). **Clips are deliberately NOT a search scope (revised).** `Candidate` models a *provisionable title* — its `MediaType` admits only `movie|series`, and it flows through the same dedupe/identity machinery that grounds the LLM. A clip is not a title (§10: commercials "are not 'titles,' so the provisioning loop does not apply"), so representing one as a `Candidate` would push an unprovisionable row with an invalid media type through the grounding path — the exact filler-into-programming leak §10 is built to prevent. Clip search therefore lives where clips live: `GET /v1/filler?q=` applies the `name LIKE` filter this section prescribes and returns real `ClipDTO`s, so a result carries a Tunarr program id and can be deep-linked. *The `clips` scope was advertised in the enum but never implemented — the catalog was always constructed with a nil clip searcher, so it silently returned nothing. Removing it corrects the contract rather than shipping a leak to satisfy it.* Crucially, **this is the same implementation as the Catalog boundary (§8)** — the LLM's grounding tool and the human's search box share one code path, so humans and the model see identical results, and "why did the suggester pick/miss X" is debuggable by typing the query into the UI. Results feed the lineup editor: adding an `in_library` result places it; adding a missing one creates an acquisition — which flows through the existing approval gate, so search adds **no new privilege surface and no new config**.

Channel/proposal/Board filtering and Help search stay **client-side** — household-scale lists and already-embedded markdown need no backend.

---

## 8. Suggester (AI suggestion engine)

Turns a channel **intent** (NL description + optional constraints: era, runtime target, tone, must-include/exclude) into a **proposal**: a lineup from existing library content + an acquisition list of missing titles. Approved acquisitions feed the provisioner; the approved lineup feeds the scheduler. The intent also carries the **refine** inputs (§7 `POST .../{id}/refine`): a free-text change plus the channel's *current lineup* as context, rendered into the prompt so the model reasons from what's already there. Grounding is unchanged — the current lineup is context only; every new pick is still grounded through the catalog tool (real ids), so a refine can't invent titles any more than a fresh suggestion can.

```mermaid
flowchart LR
  Intent[Channel intent: NL + constraints]
  SUG[Suggester]
  Cat[Catalog tool: library + TMDB]
  LLM[LLM: Ollama / OpenAI-compatible]
  Prop[Proposal: lineup + acquisitions + rationale]
  Intent --> SUG
  SUG <-->|grounded tool calls| Cat
  SUG <-->|reason / structure| LLM
  SUG --> Prop
```

### Grounding — the critical correctness rule
An AI that can trigger real downloads must never act on a hallucinated title.
- The LLM does **not** invent titles; it proposes candidates via a **catalog tool** (function-calling) that searches the real library + TMDB/TVDB and returns **real external ids**. The model selects from tool results. The tool supports both **keyword search** (by title text) and **discovery** (by genre + era) — so an abstract intent ("high-energy 90s action") surfaces themed content instead of an empty title-match, and each returned candidate carries **genre + a short overview** so the model reasons about theme rather than guessing from the title string alone.
- Every proposal item resolves to a real id, tagged `in_library: true|false`; unresolvable items are dropped before display.
- Acquisitions re-validated against TMDB (exists) + library (not present) before actionable.
- Library/TMDB text in prompts is **untrusted**: it must not steer tools, change quotas, or reach secrets; catalog tools are read-only.

### Provider abstraction
One `Suggester` interface; provider by config. **Two adapters, both plain `net/http` (no vendor SDK):**
- **`ollama`** (native `/api/chat` with tools) — the homelab default: local, private, no cost, and its capability/version API gives the §13 wizard + §8.1 model picker a fast pre-check. On a reasoning model (Qwen3-class), thinking mode is disabled on tool turns — with tools present it otherwise returns empty/leaked-marker output that breaks tool-calls (open Ollama bugs).
- **`openai`** (generic OpenAI-compatible: `/v1/chat/completions` with tools) — **one** adapter that covers the converged ecosystem, so the model is a config choice, not a per-vendor fork: OpenAI, Gemini (compat endpoint), Groq, Together, OpenRouter, and local runtimes (llama.cpp/vLLM/LocalAI/LM Studio) — **and Ollama's own `/v1` mode**. **Claude is reached through OpenRouter/an OpenAI-compatible gateway, not a native Anthropic SDK** (a deliberate net dependency *reduction* — the dialect is the interface; do not add named per-vendor adapters). The one shape difference it normalizes: OpenAI returns tool-call `arguments` as a JSON *string* (Ollama gives an object).

`LLM_PROVIDER` selects the client; the model is `LLM_MODEL` (or an in-app selection, §8.1). Both need structured JSON output + tool-use; prompts/tool schemas stay provider-neutral. **JSON mode is off whenever tools are offered** — forcing `format:json`/`response_format` *and* tools makes some models emit the tool call as a JSON object in `content` instead of the native `tool_calls` array, which the grounding loop then mis-reads as a pick-less final answer. Because different models present their *final* JSON differently (some bare, some wrapped in a ```` ```json ```` fence or a sentence of prose even when told "ONLY JSON"), the parser extracts the outermost balanced JSON object from the final turn before validating — presentation never rejects a well-formed, grounded proposal. Grounding is unaffected: extraction only decides whether the picks are *readable*, never which picks survive the surfaced-id chokepoint.

**The probe is the arbiter of capability:** tool-calling support varies by runtime and model, and generic endpoints expose no uniform capability API — so the §13 wizard check is *behavioral* (send a trivial tool-call request, assert a real tool call returns). Ollama's declared capabilities are a pre-check only. Keep the tool loop to **sequential single tool calls** (no parallel-call dependence — the least-supported corner of the dialect); the grounding pipeline already ensures a weak model degrades to "no valid proposal," never to corruption.

**Honest quality guidance:** ~7–8B-class models are the practical floor for reliable grounded tool use; local inference yields private, free, serviceable proposals, hosted frontier models yield noticeably better curation — the deterministic scoring below exists partly to narrow that gap.

### 8.1 Model selection & system probe (local + hosted)

Picking a local model is a real onboarding hurdle: a household admin shouldn't have to know which Ollama tag fits their GPU or supports tool-calling. Loomarr does that for them.

The same guided flow covers **both** a local Ollama host and a **hosted** endpoint — the user picks instead of hand-editing four env vars, and the choice hot-swaps the running suggester with no restart. The hosted surface is deliberately narrow: **OpenRouter** (the one blessed aggregator — a single key reaches every frontier family: OpenAI, Anthropic, Gemini, Llama, Qwen, …) plus **Custom** (any OpenAI-compatible base URL the user supplies — a direct vendor endpoint, or a self-hosted runtime like vLLM/LM Studio/LocalAI/a corporate gateway). We do **not** curate per-vendor entries (OpenAI/Anthropic/Groq/Gemini as separate providers): OpenRouter already fronts them with the richest live metadata, and Custom reaches any that OpenRouter doesn't — so two hosted choices cover the ecosystem without per-vendor curation to maintain (consistent with §8: "the dialect is the interface; do not add named per-vendor adapters").

- **System probe** (`GET /v1/system/llm`, admin) returns the active provider + model and **two curated catalogs**:
  - **Local models** — the catalog is **discovered live from the host**, not curated in-code (there is no maintained "good models" list to go stale). Detected inputs: **GPU VRAM** (best-effort via `nvidia-smi`; absent → CPU/unknown, which just widens the "tight" band) and the models that are **already pulled** (`/api/tags`, which also gives each model's on-disk size — a VRAM proxy — and its family/params/quant). Each pulled model is then checked for **tool-calling** via `/api/show` `capabilities`: only tool-capable models enter the catalog (grounding is impossible without tools), so a model that can't tool-call — e.g. a DeepSeek-R1 build — simply never appears, with no hand-maintained exclusion list. Each carries its VRAM footprint and a **fit verdict** ("fits" / "tight" / "won't fit") against detected VRAM, plus a single **recommended** best-fit default (the largest pulled tool-capable model that comfortably fits). To pull something new, the user **browses downloadable models live** (`GET /v1/system/llm/discover`, below) rather than choosing from a frozen list.
  - **Hosted** — the probe returns **OpenRouter** (curated in-code: label, base URL, keys-URL, note — stable) and a **Custom** template (the user fills base URL + key). For either, the **model list is live** (`{baseURL}/models`), and recommendations rank it for **Loomarr's use case — best grounded tool-caller, not merely cheapest**. Ranking, over the live list:
    1. **Hard filter (capability):** keep only models the provider advertises as tool-calling (`supported_parameters` has `tools`) — grounding is impossible without it. This is pure live metadata.
    2. **Quality tier (curated by FAMILY, not exact id):** a small durable table maps model **families** (`gpt-4o`, `claude-sonnet`/`claude-haiku`, `gemini-*-flash`/`-pro`, `llama-3.3`, `qwen*`) to a quality tier for grounded reasoning. Families change far slower than exact ids, so this barely rots — and it encodes the one thing metadata can't: *which models actually reason well enough to pick themed titles*. A live model matches by family **prefix**.
    3. **Cost tie-break:** within a tier, cheaper wins (from live `pricing`); context length breaks a further tie.

    So the recommended few are **high-tier families that are live + tool-capable**, cheapest within tier — with a rationale naming the family + why ("GPT-4o family — strong grounded tool-caller, ~$X/1M tokens"). A tool-capable model **not in any tier still appears** (selectable), just **unranked** — we never hide a new model we haven't tiered yet. A **thin** provider whose `/models` returns bare ids degrades to the live id list, unranked. Each provider carries a **`keyConfigured`** flag; the API key is **never echoed back**. The one per-model hardcode is a tiny pre-key **fallback**. The user may always select any id they type.

  The **local** catalog is **live-discovered** — installed models from `/api/tags` + `/api/show`, tool-capability from Ollama itself, fit from real on-disk size — with **no** curated-in-code model list. The **hosted** ranking likewise curates only *quality judgment by family* (durable) and derives *availability, capability, and cost* from live metadata. Neither side hardcodes an exact model id that can silently go stale.
- **Select** (`POST /v1/system/llm/select`, admin) sets the active provider **and** model. For a **local** model it verifies the tag is pulled (409 if not — pull it first). For **OpenRouter** it accepts an optional `apiKey`; for **Custom** it accepts a `baseUrl` (any OpenAI-compatible `/v1`) plus its `apiKey`. Before committing it **validates** the endpoint + key with a cheap live call (a tiny models-list / completion) and rejects a bad base or key (**401/502**) rather than letting the next suggestion job fail opaquely — so only a reachable, authorized endpoint is ever committed (this validation, not a hardcoded allowlist, is what gates a user-supplied base URL). It then persists to the **settings store** (§5: `llm.provider`, `llm.model`, `llm.url`, and the **secret** `llm.api_key`) and **hot-swaps** the live suggester via an atomic pointer — effective on the next job, **no restart**. These persisted settings **override** their §15 env defaults when present, so an in-app choice survives reboots. The stored key rides in the settings table (plaintext, same trust boundary as the `LLM_API_KEY` env var it replaces — a household LAN app; §11) and is **never returned** by any GET.
- **Test** (`POST /v1/system/llm/test`, admin) validates a provider+key **without** committing a swap — the "test my key" button. Returns reachable/authorized + the error on failure. Uses the same cheap validation call as select.
- **Discover** (`GET /v1/system/llm/discover`, admin) returns the downloadable models that are **compatible with this machine**, ranked best-first — since **Ollama has no first-party "what can I download" API** (`/api/search` is unshipped; ollama.com serves HTML only). Loomarr takes the most-popular GGUF repos from **Hugging Face's model API** (§14) and, crucially, reads **each repo's real per-file GGUF sizes** (which HF exposes *before* download) to size it against the detected VRAM — using the **Q4_K_M-class build**, which is what Ollama's `latest` tag resolves to and therefore what actually downloads. Repos too big for the machine are dropped; the rest are **curated, dedup'd, and ranked for Loomarr's use case** (below) — *not* by raw HF popularity — with one **recommended** pick surfaced for this machine. Each carries a **bare `pullRef`** of the form `hf.co/<repo>` (Ollama's implicit `:latest`). *(A synthesized `:quant` tag would NOT be reliable: Ollama resolves `hf.co/<repo>:<tag>` against the repo's own manifest, and many repos expose only `latest`, so a guessed `hf.co/<repo>:Q8_0` returns 400 "tag not available". `latest` always resolves and is the build we size against, so what we show equals what pulls.)* **The list is filtered to a curated allowlist of tool-capable model *families*** (Qwen, Llama, Mistral, Gemma, Phi, …) — because grounding needs tool-calling and **HF exposes no reliable pre-download signal for it** (verified live: its `conversational`/`pipeline_tag` tags don't separate tool-callers from completion-only models, and a model like DeepSeek-V4 reports no `tools` to Ollama only *after* pull). This is a deliberate, documented tradeoff: a maintained *family* allowlist (broad names that change slowly, one table in `internal/llm`) is the price of "only offer models that actually work locally", chosen over an inaccurate live heuristic that would both hide good models and still let dead ends through. So the operator sees "these download and run on your GPU, best first" — every one usable — not a search box. There is **no keyword parameter**: it's the compatible set for this hardware. Tool-capability is confirmed only after the model is pulled and re-probed (HF has no reliable tool-calling signal). It's **best-effort**: if the source is unreachable the list is empty and the UI falls back to a "browse on huggingface.co" link — a discovery outage never wedges the AI settings page. This machine-ranked browse, plus `/api/tags` for what's installed, is what replaces any curated-in-code local catalog.

  **Curation (the family gate is necessary but not sufficient).** Raw popularity within the family allowlist still surfaces a list that's hostile to a non-expert: the *same* weights re-uploaded by several packagers appear as separate rows (three "Qwen3.5 9B"s), and community remixes leak through the substring family match (`gemma-…-Heretic-Uncensored-Aggressive`, `…agentic-composer2.5-v2-3.5x-tau2` — they *contain* "gemma"/"qwen"). So after family-gating + sizing, Discover applies a curation pass, entirely over HF metadata already fetched (no extra calls):
    1. **Reject fine-tune soup.** Drop repos whose id carries remix markers that signal a community fine-tune rather than a base/instruct model — `uncensored`, `abliterated`, `heretic`, `roleplay`/`rp`, `-nsfw`, `aggressive`, and multi-token "recipe" names (several `-vN`/`x`/exotic-word segments). These are broad, lowercase, delimiter-bounded id substrings (one table in `internal/llm`, sibling to the family allowlist), kept narrow so a clean `-instruct`/`-it` build never trips them. A base *family* stays; its remixes go.
    1a. **Reject vision/multimodal variants that don't tool-call.** The family gate matches by substring, so a family's *multimodal* line rides in — `Qwen3-VL-*` (vision-language), Gemma's `-E2B`/`-E4B` efficiency-multimodal builds — and Ollama confirms only *after* pull that these advertise `vision`/`completion` but **not** `tools`. Grounding needs tool-calling, so a pulled vision model downloads fine yet never enters the installed catalog and can't be used — exactly the "I downloaded it but nothing happened" trap. Their repo ids **do** carry the marker before download (`vl`, `e2b`, `e4b`), so Discover drops them the same way it drops soup: a small, delimiter-bounded marker table (`internal/llm`). This is a *usability* gate on top of the *family* gate — the family says "right lineage", this says "the tool-calling member of it". A genuinely-usable model that slips through is still caught post-pull by the `tools` capability check; this table stops the common, popular offenders from ever being *recommended*.
    2. **Collapse to one row per real model.** Group the survivors by a **canonical model key** — family + parameter size + base variant (e.g. `qwen3.5/9b`, `gemma/12b-it`), ignoring uploader, quant, and packaging — and keep a **single representative** per key. The representative is chosen by **uploader trust then popularity**: a small allowlist of known-good packagers (`unsloth`, `bartowski`, `ggml-org`, `Qwen`, `google`, `meta-llama`, `microsoft`, `mistralai`, …) wins over an unknown re-uploader; ties break on downloads. The representative's own download count (not the group's) carries into ranking, so a widely-pulled canonical model still ranks high.
    3. **Rank for Loomarr, not for Hugging Face.** Raw download count is HF's popularity signal, not "best model for this app on this machine" — so it is **not** the sort key (and is **not** shown to the user). Discover instead ranks each survivor by a **fitness score** that mirrors the hosted ranking (above): (a) **fit** — a model that fits comfortably outranks a tight one (a tight model runs slowly or spills to CPU); (b) **family+size reliability** — a small durable table scores the *known-good grounded tool-callers* at the sizes that reason well enough to pick themed titles (a well-supported 7–14B instruct model outranks a 1B toy and an exotic 30B that barely fits), the same "quality by family, not exact id" judgment §8.1 already applies to hosted; (c) **popularity as a last tiebreak only** — within an equal fit+reliability tier, a more-pulled repo is the better-supported one. So the top of the list is "the model Loomarr would pick for your GPU", not "what's trending on HF".
    4. **Assign a role + recommend one.** Over the ranked survivors, Discover assigns each a plain-English **`role`** from the choice the user is actually making — **balanced** (the all-rounder), **faster** (smaller/lighter, quicker but lower quality), **higher-quality** (larger, best output that still fits) — and flags exactly one **`recommended`** pick: the top-ranked *balanced* model that fits comfortably (the safe default for a household admin). Each model also carries a deterministic **`note`** — its role phrased for a human plus the fit, e.g. "Best all-round pick — fits comfortably", "Faster and lighter — fits comfortably", "Higher quality — fits, but tight on VRAM". No jargon (no quant tag, no download count) reaches the row; `role`/`recommended`/`note` are all derived from params + fit + the reliability table, no new fetch.
    5. **Show a recommendation, not a firehose.** The API returns the full ranked+curated list (capped at `hfKeepLimit`), but the **UI leads with the one `recommended` pick as a hero card** ("Recommended for your RTX 3080 Ti — Qwen3 8B, best all-round pick, fits comfortably, ~5 GB"), then a **short list of a few strong alternatives** (the next best across roles), then a **"show more"** revealing the rest for anyone who wants the full compatible set. A beginner sees one confident choice; an expert can still expand.

    The result is "the model Loomarr recommends for your GPU, in plain language — with a few good alternatives" — the family gate decides *usable*, the ranking decides *best-for-Loomarr*, the roles + hero make it *legible*.
- **Pull** (`POST /v1/system/llm/pull`, admin) is **local-only**: it triggers an Ollama `pull` as a background job and streams **percent-complete** over the `/v1/events` SSE bus (§7) so the UI shows a live download bar. It accepts **any** tag — a bare `pullRef` from discover (`hf.co/<repo>`), or a plain tag — Ollama errors cleanly on an unresolvable one. Idempotent. A hosted provider has nothing to download (409 if called on one).

The grounding rules (above) are untouched — provider/model selection changes *which* grounded model runs, never *whether* grounding is enforced.

### Output contract (schema-validated, in the OpenAPI spec)
- `lineup[]` — library items: external id, library item id, order hint, why-it-fits, and — for a **series** whose intent implies an era — an optional **airing season range** (`seasonMin`/`seasonMax`).
- `acquisitions[]` — media type, resolved id, seasons, rationale, confidence.
- **Per-pick airing season range (`seasonMin`/`seasonMax`)** — a series `lineup`/`alternates` pick may carry a season window so an era-scoped intent airs only those seasons. **"Simpsons Classics" → seasons 1–10**; "early Seinfeld", "golden-age X", "first N seasons" are the trigger phrases. This is the LLM translating the intent's *era words* into a concrete, per-series constraint the scheduler already enforces (`LineupEntry.SeasonMin/Max` → `inSeasonRange`, §9 series expansion). **It is airing scope, DISTINCT from acquisition `seasons`** (what to *download*): a show can be fully acquired yet air only its classic seasons, so a range never suppresses acquiring later seasons. **Grounded + clamped like all pick metadata (§8):** the model proposes a range but the suggester validates it — a non-positive or inverted (`min>max`) range is dropped (→ all seasons, never an empty channel), and the value is a *hint* the scheduler applies against the series' real in-library seasons (an out-of-range window simply matches nothing extra). The model can never invent airing content this way — every episode still comes from the grounded, in-library series; the range only *narrows* an already-grounded expansion, upholding the grounding chokepoint. Movies ignore it.
- `alternates[]` — ranked backup candidates (same shape as acquisitions/lineup items), consumed by the scheduler when a title goes `unavailable` (§9). Same grounding rules — real ids only.
- `policy` — the **ChannelPolicy**: scope (series/seasons/era/genres), audience ceiling, separation, ordering, seasonal mode — extracted by the LLM, schema-validated, grounded (season ranges clamped to the real series, ratings from a closed enum), and enforced deterministically by the scheduler. Full design: `programming-design.md`. **Audience enforcement needs a content rating**, so `Candidate`/`ProposalItem` carry `officialRating` (from the media server's `OfficialRating`, alongside `genres`/`overview`); it is display/enforcement metadata only — never identity (`Candidate.Key()`/dedupe ignore it). **v1 limitation:** the rating enforced on a series is the *series'* rating (episodes carry no per-episode rating in the current library adapter), so a mixed-rating series is gated at its series ceiling; per-episode ceilings are future work. The stamped rating/genres/year travel onto the channel's approved lineup entries at create time, so enforcement is a pure entry-set filter (no per-reconcile library I/O) — **once an entry is rated**. Two paths keep an entry from reaching the scheduler *unrated*, which would be dropped by a fail-closed audience gate and take the channel to dead air (§9): (1) at proposal time an acquisition not yet in the library — so with no library rating — is enriched from **TMDB** (`/tv/{id}/content_ratings`, `/movie/{id}/release_dates`, US); TMDB coverage is sparse, so this is best-effort. (2) at reconcile, an entry that is *still* unrated but whose title is now present in the library has its rating **healed from the library and persisted back onto the entry** — a one-time, self-healing repair bounded to unrated entries (a normally-stamped entry is never re-looked-up, preserving the no-per-reconcile-I/O property for the steady state). This is what makes an acquisition rateable once it lands, and repairs a pre-fix cached proposal (§8 24h TTL) whose entries were stamped empty.
- `scores` — **deterministic** post-scoring (theme fit, runtime/era balance, availability ratio) layered on the LLM output so ranking isn't pure vibes (à la SmarTunarr's multi-criterion scoring; keep criteria configurable). **Theme fit measures the intent's terms against each item's genres + overview** (not the title string — a "90s action" intent rarely appears in a *title*, so title-substring scoring is near-useless); it stays deterministic (same inputs → same score). `Candidate`/`ProposalItem` therefore carry `genres` + `overview` (populated from TMDB and the media server; §7.2 output contract, in the OpenAPI spec).

### Human-in-the-loop (non-negotiable default)
Proposals are never auto-executed. Members submit; a user with the **approve** permission (`admin`, §11) confirms before anything acquires or schedules, and `approved_by` is recorded. Optional `auto_approve` is a per-user grant **hard-gated by the pending-acquisition cap** (§11): a proposal auto-approves only while its requester stays within quota, and otherwise falls back to the admin queue rather than being denied. `approved_by` records `auto` for that path, so the audit distinguishes a machine decision from a human one.

### Execution model
Generation is a **job**, persisted in the store (§5) and executed by the in-process worker pool (§14; `JOB_WORKERS` default 2, per-job `JOB_TIMEOUT` default 10m — so one hung LLM call can never starve the queue) — on Postgres replicas, jobs are claimed via the same `SKIP LOCKED` pattern as titles. Proposals are persisted too, each recording `created_by` (powers My proposals, §12): the approval queue (`GET /v1/proposals?status=submitted`) and pending approvals must survive restarts. Generation progress streams over the shared `/v1/events` SSE bus (§7) as `suggestion` frames — payload `{jobId, phase, round}` where phase advances `searching` (the model is calling the catalog tool) → `reasoning` (the model is composing the grounded lineup) → `scoring` (deterministic post-scoring) → `done`/`failed`; on reconnect, `GET /v1/proposals/{id}` is the source of truth (dropped progress events are a latency bug, never a correctness bug).

**A phase must name what is happening NOW, not what is about to.** The tool loop alternates — the model thinks, calls the catalog, reads the results, thinks again — for up to `maxToolRounds` iterations, so `searching` and `reasoning` are *repeating* states, not a one-way sequence. Emitting `searching` once before the loop and `reasoning` only after it exits (the original shape) meant the label read "Searching the library" for the entire run including every model turn, which is where the operator's time actually goes: it named the fastest step in the job as the explanation for the slowest. Each phase is therefore emitted **inside** the loop at the transition it describes, and may repeat. `round` (1-based, `0` = not in the tool loop) carries the iteration so a long run is legibly *progressing* rather than hung — the UI pairs it with elapsed time. Grounding is untouched: progress is display-only, and `buildProposal`'s surfaced-id chokepoint is the only thing that decides which picks survive. Cache proposals by hash(normalized intent + constraints) with a short TTL (default 24h) — **but only a *successful* job is a cache hit**: a run that grounds no titles (or fails/times out) must NOT be cached, or an operator retrying the same intent would be wedged to the empty result for the TTL. A zero-grounded-title result **fails the job** (with a clear "no grounded titles found" reason surfaced via `last_error`), rather than persisting an empty `submitted` proposal. The grounded turns are generated at a **low sampling temperature** (JSON/tool-call adherence over creativity), with a small **bounded repair loop** that re-asks when the model's final turn isn't valid schema JSON. The suggester is an internal subsystem using the Store like the others; the *external* thing it talks to is the LLM, and that boundary is what the grounding rules police.

### 8.2 Model residency — keeping the local model warm

A local model that is not resident must be loaded from disk into VRAM before it can answer, and that load dominates everything else the suggester does. Measured on the maintainer's Apple-Silicon machine with an 8B model: **~9.1s cold vs ~0.5s warm**, an 18× difference for the identical request. Ollama unloads an idle model after **5 minutes** by default, so a household operator who describes a channel, reads the proposal, and then refines it reliably pays that load *again* — the app is slowest exactly when someone is using it, and the cost is invisible because nothing distinguishes "loading a model" from "thinking".

Two mechanisms, both local-only (a hosted endpoint has no residency to manage, and sending either to one would be meaningless):

- **`keep_alive` on every Ollama call.** Each `/api/chat` request carries a residency hint (`llm.keep_alive`, §15, default `30m`, `0` disables) so the model stays loaded between the turns of one run *and* across the pause between a suggestion and its refine. Without it the multi-round tool loop can re-pay the load mid-run. This is a hint to Ollama, not a reservation: it holds VRAM the operator may want elsewhere, so it is a settable knob rather than a constant, and `0` restores stock behavior for a memory-tight host.
- **Warm-up on selection.** Boot and every §8.1 model pick call the same code path (`Swappable.Set`), so that is where a warm-up fires: a minimal generation request that loads the model and returns immediately. It runs **in the background, best-effort** — a warm-up that fails or is slow must never delay startup, block the admin's Select response, or fail a job. The first *real* suggestion after an app start is therefore usually warm rather than paying the 9s on the operator's first impression.

  **Nothing is warmed until a model is actually chosen.** `LLM_MODEL` is deliberately blank by default (§8.1: the wizard's ranked picker owns the choice, because it depends on the user's GPU), and the Ollama client substitutes a built-in fallback tag when it has no model. Warming *that* would fire a tag the host has almost certainly never pulled, so a fresh install logged a 404 "warm-up failure" on every boot for a model nobody selected. A warm-up with no configured model is therefore **declined**, and declining is a **third outcome** distinct from success and failure (`ErrNothingToWarm`): reporting it as either would describe something that did not happen — first a failure that was never a real attempt, then, once the request was suppressed, a `model warmed took=0` announcing a preload of nothing. The picker's own `Set` warms the model the moment it is chosen, so declining costs no real latency.

Neither mechanism changes what the model is asked or what it returns; they only decide whether the weights are already in memory when it is asked. Grounding, the repair loop, and the approval gate are untouched.

### Bonus: filler suggestions
The suggester can also propose **era/genre-matched filler** (90s ads → 90s sitcom block) for the scheduler's flex — same grounding rules apply.

---

## 9. Scheduler / lineup builder — *the point of the app*

Turns an approved proposal + live availability into an actual, filled Tunarr channel, and keeps it that way. Everything upstream exists to feed this.

### Responsibilities
- Own a **Channel**: intent ref, target Tunarr channel (number, name, logo, group), scheduling strategy, filler policy.
- Compute **desired programming** from the approved lineup per strategy.
- **Reconcile desired → actual** in Tunarr (create/update channel, set lineup, filler lists, flex) — idempotent, minimal-diff.
- Run the **backfill loop** so a channel is live immediately and improves as content lands.
- Keep channels from running dry; refresh on library changes.

### Scheduler domain (persisted in the same store, §5)
- `Channel`: id, intent ref, Tunarr channel id/number, strategy, filler-list ref, status.
- `DesiredLineup`: ordered `Slot`s referencing external ids (some not-yet-available).
- `Slot`: `program` (library item, once available) | `pending` (awaiting provisioner) | `filler`/`flex`.
- **Availability resolution** turns an approved lineup entry into a `program` slot: it resolves the entry's key to `(library item id, duration, available)`. Duration comes from the media server (the same `RunTimeTicks` source filler uses, §10) — the approved lineup carries only *what* should play, not its runtime, so the scheduler learns duration at resolution time. A program slot always carries a real `duration > 0`; the downstream Tunarr programming push requires it.
- **Series expansion.** A movie lineup entry is one playable item → one program slot. A **series** entry is *not* directly playable: a show has no single library item and no single runtime — its **episodes** are the programs. So a `series` entry **expands** at resolution time into one program slot **per episode**, each carrying that episode's own media-server item id and duration (from `RunTimeTicks`). Expansion is the scheduler's job, not the suggester's: the approved lineup stays at the intent level ("this channel plays Seinfeld"), and the scheduler resolves the concrete episodes that exist *now* (so newly-imported episodes join on a later reconcile, consistent with backfill). **Ordering follows the channel strategy** (the same rule as movies): `sequential` → episodes in season/episode order; `shuffle` → episodes shuffled with the channel seed. Episode enumeration comes from the library adapter (`ListEpisodes(showItemID)` → `[]{itemID, durationMs, season, episode}`); a series whose episodes aren't in the library yet resolves to a `pending` slot until they land.
  - **Season range (intent-level constraint).** A series entry may carry an optional `SeasonMin`/`SeasonMax` (inclusive; 0 = unbounded on that end) — an intent-level filter for channels like "old-school Simpsons" (seasons 1–10) or "just the classic run." Expansion filters the enumerated episodes to that range (by each episode's season number) before producing slots. It's a property of the *approved lineup entry* (the human's intent), not of availability, so it survives re-syncs and applies uniformly under any strategy. A range that matches no in-library episodes yet → a `pending` slot (same as an unavailable series).

### Scheduling strategies (map onto Tunarr)
- **Ordered/sequential** (e.g., a series in episode order).
- **Shuffle** (random rotation).
- **Time-slot / block** (fixed start times, themed blocks — cartoons AM, movies PM).

### Filler & commercials
Ad pods, bumpers, and station IDs between programs are what make a channel read as broadcast rather than a playlist. This is a first-class capability with its own sourcing pipeline and matching logic — see **§10**. The scheduler is the component that inserts the pods (via Tunarr Flex + filler lists) as it builds each channel.

### Backfill loop (async correctness)
- On approval: build the channel from currently-available items; fill the remaining timeline with filler/fallback so it's **live immediately — never dead air**. **Default pending-slot policy: pod-fill** (fill the gap with matched filler); a "coming soon" interstitial card is a config alternative.
- Subscribe to provisioner availability events (internal). On `available` → place the real title, re-push affected programming. On `unavailable` → substitute permanently: next-ranked candidate from the proposal's `alternates[]` (§8), else the fallback pool. **The fallback pool is defined as** the channel's already-available lineup items (loopable) plus its filler catalog — i.e., "never dead air" concretely means: loop what the channel has, padded with pods.
- **Default backfill placement is stable:** landed titles fill their pending slots in place; no global reshuffle of a live channel on backfill (`SCHED_BACKFILL=stable|reshuffle`, default `stable`) — viewers shouldn't see the guide scramble every time a download lands.
- The desired lineup is built under the channel's **ChannelPolicy** — hard filters (scope, audience fail-closed, seasonal bench) → seeded constraint-aware slotting (separation, ordering) → **relaxation ladder** on shortfall (recorded + surfaced; audience and scope are never relaxed) → pods. Separation is enforced **across the cycle seam** (Tunarr lineups loop, so the last→first adjacency honors the gaps too). The audience filter emits an **exclusion report** (`{overCeiling, unrated, items}`) surfaced at proposal review *and* reconcile, so gaps are visible before approval ("14 excluded: 11 over ceiling, 3 unrated") — the fix (rate the media, or relax the policy) is a human decision. The `programming-design.md` doc is authoritative for the policy schema, the enforce-not-extract split, cycle-wrap separation, seasonality, and the ladder; the `GET /v1/channels/{id}/cycle` cycle preview (§8.1) shows the first N slots with the active-rule attribution for proposal review and the channel's Programming surface.
- **Policy defaults (v1):** omitted policy fields resolve to **built-in Go constants** (§15 lists the values). The full `channel-policy > registry-default > built-in` precedence in `config-design.md` awaits the settings registry (not yet built); the registry-default middle tier is a documented no-op for v1 and slots in later without touching enforcement.
- **Ordering has one operator-facing knob (`policy.ordering`), not two.** The canonical 3-tier precedence is **per-rule `How.Ordering` (within that rule's window) > `policy.ordering` > `Channel.Strategy` (the stored default)**. `Channel.Strategy` is the create-time default the suggester/binder seed and is consulted only when `policy.ordering` is unset (inherit); it is **not** a separately-editable field on the channel page — the operator edits `policy.ordering`. (`programming-design.md` §5 is authoritative for the ladder.)
- Reconciliation is **desired-vs-actual and idempotent**: recompute desired lineup, diff against Tunarr's current channel state, apply the minimal API calls. Safe to re-run any time (`/v1/channels/{id}/reconcile`).
- **Periodic sweep (correctness):** a channel-reconcile ticker (`CHANNEL_RECONCILE_EVERY`, default `10m`) re-derives every channel's desired lineup from the store and reconciles — so availability **events are a latency optimization, never load-bearing**. This is what makes backfill survive a crash between event and re-push, and what makes Postgres multi-replica correct without cross-instance event delivery (an in-memory event can't reach another replica; the sweep can). The sweep also **revalidates every program slot against the library** (§4 invariant 1): if a scheduled item has vanished (deleted, replaced, re-id'd), the slot is substituted via `alternates[]`/fallback pool and the channel is flagged on the Channels view — an old `available` is never trusted forever. Postgres `LISTEN/NOTIFY` as a faster cross-replica signal is future work (§20).
- **Ownership semantics:** Loomarr-managed Tunarr channels are **desired-state authoritative** — manual edits made in Tunarr's own UI will be overwritten by the next sweep, by design (that's what idempotent reconcile means). The UI labels these channels "Managed by Loomarr" (§12) so nobody loses an hour of hand-tweaking to the robot. Channels Loomarr didn't create are never touched.
- **Time zones:** time-slot schedules are computed in the container's `TZ` (standard env; set it in compose). Slots are **wall-clock** — "cartoons at 8 AM" stays 8 AM across DST transitions, accepting the one skipped/doubled hour a year. Per-channel time zones are future work (§20).

### Tunarr integration
Only implementation of the `Programmer` boundary, but abstracted so a future ErsatzTV/dizqueTV target is possible. Tunarr must point at the same Emby/Jellyfin library as its media source (§6).

### Guide freshness
Emby/Jellyfin refresh guide data on a schedule (nightly by default). After any channel reconcile that **creates, renames, or deletes** channels, the scheduler pokes the media server so the change appears in minutes rather than after the nightly refresh (best-effort — a failure degrades freshness, never the reconcile). **Two distinct media-server operations, and the difference matters (learned in the first live smoke):**
- **Guide refresh** (the `RefreshGuide` scheduled task) updates **EPG/program data for channels the media server already knows about**. It does **not** discover new channels. Use it when an *existing* channel's lineup changed.
- **Tuner re-scan** re-reads the tuner's **M3U playlist to discover the channel _list_** — this is what surfaces a **newly-created** (or removed) channel. On Emby/Jellyfin, re-saving the M3U tuner host (`POST /LiveTv/TunerHosts` with the existing host config) forces this re-read. A guide refresh alone leaves a brand-new channel invisible in the family's guide until the media server's own periodic tuner scan (hence "I had to refresh the playlist manually").

So the poke is **operation-specific**: a reconcile that **added or removed a channel** triggers a **tuner re-scan** (channel-list changed); a reconcile that only changed an existing channel's lineup triggers a **guide refresh** (EPG changed). Both are best-effort and idempotent (safe to re-request). The tuner-host payload + guide-refresh task id are version-fragile and pinned via the §6 Live TV wiring capture. Wiring itself is one-time (§6); these pokes are the only per-reconcile media-server touch.

---

## 9.1 Playout backends — *Loomarr serves its own streams*

> **This section reverses a founding non-goal, deliberately.** §1 previously read *"Not a
> transcoder/streamer. Tunarr (and Emby/Jellyfin) do playback, transcoding, EPG, and HDHomeRun/M3U
> output."* That is no longer true: Loomarr plays out its own channels, with Tunarr retained as an
> alternative backend.
>
> **Why the reversal.** Every capability that distinguishes real TV from a playlist lives at the
> encoder: mid-roll breaks (§10 — impossible at Tunarr's program boundaries), honest transcode
> telemetry (§12's dashboard can only report on an encoder it owns), and per-channel control of cut
> points. Handing playout to another service meant those were permanently out of reach, and the
> workarounds were accumulating faster than the feature they avoided. The cost is real and stated
> below — this is not a free win, and it is not reversible cheaply.

**A channel names its backend.** `playout.backend` is a registry setting (§15) with a **per-channel
override** — switching the global default affects *new* channels only; channels already on the other
backend keep playing exactly as they are, and one can be moved from its own page. There is never a
fleet-wide flip.

| | **Loomarr (internal)** | **Tunarr** |
| --- | --- | --- |
| Streams | Loomarr, via bundled `ffmpeg` | Tunarr |
| Break placement | between programs **and mid-roll** (§10) | between programs only |
| Transcode telemetry | real, per-session (§12) | none — Loomarr can't see inside Tunarr |
| Extra service to run | no | yes |
| Right when | you want mid-roll, fewer moving parts, or visibility into playback | your hardware can't transcode, or your install already works |

**Tunarr is not deprecated.** It remains first-class and supported: the honest answer for hardware
that can't transcode is "let Tunarr do it", and an install that already works should never be forced
to migrate.

### What internal playout serves

- **Segments** over **both HLS and MPEG-TS**. Both, because media servers differ in what they accept
  and the compatibility matrix is not ours to police — MPEG-TS matches Tunarr's existing shape and
  keeps latency low; HLS survives proxies.
- **An M3U tuner** (`/playout/tuner.m3u`) — the channel list the media server registers.
- **An XMLTV guide** (`/playout/guide.xml`) — the listings provider.

**Audio track selection is ours to make, because nobody else is left to make it.** With no explicit
`-map`, ffmpeg picks one stream per type by "best" — for audio that means **the most channels**,
ties broken by lowest index. It does not read language tags and it does not honour the `default`
disposition. So a release whose Russian dub is 5.1 and whose English track is 2.0 plays **in
Russian, every time, deterministically** — which is exactly what a dev-install channel did. Direct
playback never showed it because the media server applies the viewer's language preference; internal
playout calls `ffmpeg` itself and bypasses that entirely.

`playout.audio_language` (§15, default `eng`) names the preferred track. The selection is
**preference, not requirement**: `-map 0:a:m:language:<pref>?` with the trailing `?` making the
match optional, plus a `-map 0:a:0` fallback so a file with no tagged track of that language still
gets audio. A hard map without the `?` fails the whole encode on an untagged file — a channel that
goes black rather than one that speaks the wrong language, which is strictly worse. Empty means
"whatever ffmpeg would have picked", preserving today's behaviour for anyone who wants it.

*A per-channel override is deliberately NOT offered yet.* The obvious next request is a foreign-language
channel, but the honest scope is a per-**title** decision (a subtitled original vs a dub), which is a
different feature than a channel-wide default and should not be pre-empted by a knob that half-solves it.

Both files carry a **`playout_token`** (§15, a generated secret): every segment request is signed, so
only the operator's media server can pull the stream. Regenerating it invalidates the media server's
wiring — guide entries survive, playback stops until Live TV is re-connected — so the UI gates it
behind a typed confirmation.

**Segment auth is a second authorization path, and §11 says so explicitly.** A television cannot hold
a session cookie, so segment routes authenticate a **device** by token, not a **person** by session.
This is the only route family that bypasses the allowlist model, it is read-only, and it is scoped to
playout. It is described in §11 alongside the credential paths rather than left implicit here.

### Consequences recorded honestly

1. **`ffmpeg` becomes a core runtime dependency** (§14) and ships in the single image (§16). The
   previous "two tags, one binary" split — a 31 MB default plus a 549 MB `loomarr:filler` (retired-ok) — collapses
   into one 549 MB image. An 18× increase in the default download, accepted because a playout-capable
   Loomarr without an encoder is not a coherent artifact.

   ⚠ **"ffmpeg is present" is not one fact — capability is per BUILD, and the build is the only
   honest source.** The image controls its own ffmpeg; a developer running the binary on a host
   does not, and distro/Homebrew bottles differ in which *optional* pieces they carry. The
   detector already takes this seriously for encoders (`listEncoders` asks the binary rather than
   inferring from hardware). **The same rule binds every optional filter, and `drawtext` is one:**
   it needs libfreetype at compile time (plus libharfbuzz on ffmpeg 8), and a build without it
   rejects the filter at graph-init with *"Filter not found"* — the encode exits 8 and the channel
   is dead. Font *discovery* cannot answer this: the card code asked only "is there a font file?",
   which on macOS is yes (Arial ships with the OS) while the Homebrew bottle carries no freetype
   — so every offline card died on a machine that looked correctly provisioned. **The contract is
   that a card degrades to an unlabelled colour field, never to a dead channel**, so text is
   probed like an encoder and an unprobeable ffmpeg resolves to *unlabelled*, never to an assumed
   yes. This is a Linux/macOS parity requirement, not a macOS workaround — a minimal Linux ffmpeg
   fails identically.
2. **`ffprobe` returns.** §16 excluded it on the grounds that *"Loomarr never probes media — Tunarr
   assigns duration during its `local`-source scan."* Once Loomarr owns playout it owns duration, so
   the premise no longer holds. This is the second reversal in this section; both follow from the
   same root cause.
3. **Loomarr publishes its own M3U/XMLTV**, so the §6 Live TV wiring points at Loomarr rather than
   Tunarr for internal channels. `StaleLoomarrListings` currently identifies Loomarr's provider **by
   its Tunarr-shaped path** — retargeting silently breaks stale cleanup unless that identification
   changes with it. *(Both halves are now done: `isLoomarrManagedGuidePath` matches Tunarr's shape
   AND internal playout's, and `LiveTVURLsFor` selects the URL pair from `playout.backend`. Until
   the latter landed the wiring built Tunarr's URLs unconditionally while the backend defaulted to
   `internal`, so the media server was registered against a backend that was not serving those
   channels — the channels appeared in Emby's guide and refused to play, and a `livetv-reconnect`
   "repaired" it by re-registering the same wrong URLs. The URLs resolve **per call**, not at
   construction, so switching backends applies without a restart; and an internal backend with no
   `server.public_url` yields NO urls rather than a relative path, because the media server
   resolves the URL from its own host and would silently point at itself.)*

   ⚠ **Every "what is on now" reader must select on the backend too — the M3U/XMLTV pair was
   not the only place this was wrong.** `GET /v1/channels/now-next` and `…/{id}/upcoming` read
   **Tunarr's** guide, keyed by `TunarrID`, and were wired on `tunarr.url != ""` alone. A channel
   that has been reconciled to Tunarr in the past keeps its `tunarr_id` and Tunarr keeps
   generating listings for it, so after a switch to internal playout the endpoints kept answering
   — from a schedule with its own independent epoch. Observed on the dev install: the guide and
   XMLTV said *The Last Jedi*, `now-next` said *The Rise of Skywalker*, ~30 minutes apart, at the
   same instant. Neither was stale in the caching sense; they were two different schedules.

   **The rule: a reader answers for the backend that is actually streaming that channel, or it
   does not answer for that channel at all.** For internal channels now/next comes from
   `BroadcastsBetween` — the same resolver the encoder and XMLTV already share — so §9.1's
   one-source guarantee covers the card too. Mixed installs resolve **per channel**, via the same
   `policy.playout.backend` precedence `playoutChannels` uses; there is no global switch here
   either. A Tunarr-backed channel with no `tunarr_id` still yields no entry, exactly as before.
4. **Restart is no longer free.** Prior copy promised *"Channels keep playing — Tunarr streams them,
   not Loomarr."* For internal-playout channels a restart **does** interrupt playback, and any
   restart UI must say so rather than inherit the old reassurance.

   ⚠ **The honest copy is PER-BACKEND, not a flat reversal** (recorded during V16, when the
   telemetry made the mechanism concrete). ffmpeg is spawned as a child of Loomarr with
   `Setpgid` and torn down by process group, so a restart kills every stream Loomarr is
   encoding — while Tunarr-backed channels genuinely do keep playing, exactly as the old copy
   said. Since `policy.playout.backend` is per channel, an install can have both at once. The
   restart dialog (V13) therefore needs the live session count, which `GET /v1/playout/sessions`
   now provides: *"3 channels Loomarr is streaming will drop for a few seconds; Tunarr-backed
   channels keep playing."*

### 9.2 Restarting in place (V13)

**Loomarr restarts by rebuilding itself in the same process, never by exiting.** `main` runs a
loop — `for { app := Build(); app.Run(); app.Shutdown() }` — and a restart request ends the
current iteration so the next one constructs a fresh store, handler, scheduler and HTTP server.
Same PID, no re-exec, **no supervisor required**.

The three mechanisms were weighed against the constraint that an operator must never be left
with a dead service and no way back:

| | Unix | Windows | Needs a supervisor |
| --- | --- | --- | --- |
| `syscall.Exec` (execve) | ideal — same PID, bounded failure | **unsupported** | no |
| **In-process rebuild (chosen)** | works | **works identically** | no |
| Exit and let a supervisor restart | works | works | **yes** |

- ⚠ **`syscall.Exec` is a stub on Windows, not an absence.** `syscall/exec_windows.go` returns
  `EWINDOWS`, so cross-platform code **compiles cleanly and fails only at runtime** — the button
  would ship broken rather than refuse to build. Windows has no execve at all (`CreateProcess`
  always makes a new process), so any exec-based design needs a permanent platform branch.
- ⚠ **Exit-and-be-restarted is the option that can strand an operator.** It assumes a supervisor
  exists — false for `make dev-be` and any bare binary — and the exit-code contract is
  supervisor-specific (Docker `unless-stopped` restarts on 0; systemd `Restart=on-failure` does
  not). Docker's restart backoff is also exponential, so a wrong guess costs minutes of downtime.
- **Prior art:** this is what **Jellyfin** does (`Jellyfin.Server/Program.cs`: `do { await
  StartServer(...); } while (_restartOnShutdown);`) — a full host rebuild, same PID, and the
  reason Windows needs no special case there. Sonarr, by contrast, spawns a detached child and
  branches on `IsWindowsService`, which is exactly the platform-specific complexity this avoids.

**What the loop costs, stated plainly.** Rebuilding in-process makes package-level mutable state
a correctness constraint the compiler cannot enforce:

- **Anything constructed per iteration must be per iteration.** `http.Server` is single-use by
  design (`inShutdown` is set on `Shutdown` and never cleared), so each pass allocates a fresh
  one. The same applies to the mux, the store handle, and the scheduler.
- **Global registries are the loud failure.** `prometheus.MustRegister`, `http.HandleFunc` on
  `DefaultServeMux`, `expvar.Publish` and `sql.Register` all panic on a second registration.
  Loomarr uses **none** of them except Prometheus, and `metrics.RegisterStoreCollector` already
  tolerates `AlreadyRegisteredError` — written for "a second boot in one test process", which is
  precisely a restart iteration.
- ⚠ **`sync.Once` is the quiet failure**, and the one to watch: a package-level `Once` guarding a
  resource makes iteration 2 inherit iteration 1's closed handle, with no panic and no log line.
  Every `sync.Once` in this repo is closure-local or a struct field, so it rebuilds with its
  owner; a package-level one would be a bug.
- **Once-only work stays ABOVE the loop** — logger setup and anything that genuinely must not
  re-run. Jellyfin hoists migrations and path setup for the same reason.

**The gate is a test, not a rule.** A restart is only correct if the loop can run repeatedly
without accumulating goroutines or stale state, so the phase ships an N-iteration
Build/Run/Shutdown test asserting a stable goroutine count (`go.uber.org/goleak`, §14). A prose
rule would not have caught it.

### What does not change

The scheduler, the lineup, pod assembly, the relaxation ladder, determinism and the approval gate are
**backend-agnostic**. A backend decides *how bytes reach the television*; it never decides what plays,
in what order, or whether a title was allowed to be acquired. The same lineup produces the same
schedule on either backend — that is the invariant that makes the choice safe to change per channel.

---

## 10. Commercials & filler

Commercials are core to the "feels like real TV" goal, not a garnish — this is a first-class capability with its own **sourcing pipeline** (deliberately *not* the *arr acquisition path) and its own **matching logic**. The scheduler (§9) inserts the results; this section defines where filler comes from, how it's described, and how pods are built.

### Why filler is a separate pipeline
Titles come from TMDB via Seerr/Sonarr/Radarr. Commercials, bumpers, and station IDs are **not** in TMDB and aren't "titles," so the provisioning loop (§3–§7) does not apply to them. Filler gets its own ingestion — designed so the **core stays a static binary** (no Python, no ffprobe; see §16):
- **Sources:** Internet Archive collections; curated YouTube playlists (the dizqueTV-wiki-style filler repos); user-created bumpers / station IDs / "we'll be right back" cards.
- **Ingestion path (v1):** clips land in a **drop-folder** — placed manually, via an existing tool like MeTube, or via loomarr's own **ingest job** (yt-dlp for YouTube/playlists, plain `net/http` for Archive.org), which writes files + info-JSON sidecars into that folder.
- **Ingest runs in the core, in the single image (revised twice — see the history below).** Ingest is a normal job on the same job bus as every other long operation: it reports progress over SSE, is cancellable, and needs no service discovery, no compose profile, and no proxy hop from the API. The tooling it shells out to (`yt-dlp` + `ffmpeg`) **ships in the one published image** (§16), so ingest is always available. The `FeatureIngest` gate (config-design §7) remains — it now resolves from the binaries being *runnable* rather than from the image variant, and still drives the 409 and the Filler tab's empty state, so a broken vendored binary degrades honestly instead of erroring at the point of use.
  - *History, because this question keeps being re-decided:* **(1)** a `loomarr-ingest` sidecar, removed because its only justification was keeping media tooling out of the core image, bought at the price of a second image, a compose profile, a distributed seam on the Filler page's primary action, and progress that couldn't ride the SSE job bus. **(2)** an opt-in `loomarr:filler` tag, which bought the same slimness without the seam. **(3)** the single image (§9.1, §16) — because `ffmpeg` became load-bearing for *playout*, so a variant without it is not a slimmer Loomarr but a broken one. Each step followed a change in what the tooling was **for**.
- **Filler is Loomarr-owned. A media-server library may be one SOURCE it pulls from (revised twice).** The clip folder is registered in **Tunarr as a `local` media source** (Tunarr scans a plain folder directly) that Loomarr sets up idempotently at first filler sync (same enumerate-first pattern as the Live TV wiring, §6). Loomarr owns the clips: they live in its clip folder, identified by content hash, and program content stays cleanly separated from filler.

  ⚠ **What changed (V38c).** This bullet previously said the media server was out of the filler path *entirely* and that the operator "never creates or manages a commercials library in their media server". That is now too strong: an operator who ALREADY keeps commercials in an Emby/Jellyfin library can register it as a filler source, and Loomarr scans it (§10, "the media-server library row is scanned again"). What has not changed is the ownership model — a library scan is an **acquisition** path feeding the same intake as every other source, so clips are copied into the clip folder rather than played out of the library in place, and Loomarr never modifies the library. The original rationale still holds for the DEFAULT: commercials aren't "library titles", so nothing requires an operator to curate one. It is now an option rather than a prohibition.

  ⚠ **The dependency §9.1 removed stays removed.** A library is never the catalog's only route: an install with no media server, or one whose media server is down, still gets a full catalog from its folders and remotes. "No media server ⇒ no commercials" must not come back.
- **Catalog sync (core) — revised by §9.1.** Loomarr scans **`FILLER_DIR` itself** and probes each clip's duration with `ffprobe`. **Clip identity = the clip's path relative to `FILLER_DIR`** (e.g. `1994/toys-transformers.mp4`).

  *This reverses the previous design, in which Tunarr scanned the folder, assigned each clip a program id, probed its duration, and Loomarr synced that back — clip identity being the Tunarr program id.* Two things forced the change, both traceable to §9.1:

  1. **Internal playout needs a playable input, and a Tunarr program uuid is not one.** Loomarr's own encoder takes a file path or a URL. A channel on the internal backend could assemble a pod and then have nothing to hand ffmpeg.
  2. **The dependency ran the wrong way.** Discovering clips *by asking Tunarr* meant an install running internal playout with **no Tunarr at all** had an empty catalog and therefore no commercials — a hard requirement on a service §9.1 makes optional. The files are on Loomarr's own disk the whole time; routing their discovery through Tunarr was a detour.

  The premise for the old arrangement is also simply gone: it existed so *"probing stays out of loomarr entirely… the core never needs ffprobe"*, and §14 now bundles both `ffmpeg` and `ffprobe` as core runtime dependencies **because internal playout owns duration and cut points**. Scanning locally spends a dependency we already have.

  **Tunarr-backed channels are unaffected.** A clip row keeps a nullable `tunarr_program_id`, populated by the same local-source sync as before, so `attachFillerList` still builds filler-lists from real Tunarr program ids. Internal playout reads `path`; Tunarr reads `tunarr_program_id`; one catalog, one assembler, one seed (below). An install with no Tunarr simply leaves that column empty.

  `/v1/filler/sync` triggers the scan; a periodic sync runs alongside the reconciler. **Identity change ⇒ forward-only migration that drops and recreates the catalog empty**, exactly as `00006` did for the same reason: filler is a synced cache, not source-of-truth data, so the next sync repopulates it.

### Filler catalog (metadata is what enables matching)
Each clip carries metadata so the scheduler can place it well, persisted in the store (§5):
- `kind`: commercial | bumper | station_id | psa | trailer | interstitial
- `era`: decade / year (e.g., 1994)
- `audience`: kids | family | general | late_night
- `category`: toys | cereal | cars | tech | fast_food | movie_trailer | …
- `duration` (from the media server), `rating`, `source`

Tagging options, in increasing order of leverage: filename/folder convention → sidecar metadata → **AI-assisted classification**. AI tagging classifies a clip's **text signals** — filename, the source title/description that yt-dlp/Archive provide (the ingest job preserves these as info-JSON sidecar files next to each clip), and, for clips produced by compilation splitting, the segment's **transcript** (V34; whisper.cpp, §14) — into era/audience/category via the configured LLM. Vision-based tagging remains future work (§20). Even text-only tagging is what makes thousands of clips practical, and is where Loomarr beats hand-curated filler lists.

**Era must be grounded in the source text — a measured §8 hole, closed by V34 (maintainer's call: both halves, not one).** Running the real tagging prompt over real transcripts invented an era on 2 of 10 clips — `1980` and `1970` with no year anywhere in the text, inferred from tone — and the validator had no way to tell an inferred year from a read one (plan §6.4). So: an `era` tag is accepted **only when that year appears literally in the clip's text signals** (filename, sidecar text, or transcript); otherwise it is **not persisted as fact** and is instead recorded as a **suggestion** the operator confirms (`PATCH /v1/filler/{id}` setting `era` confirms and clears it). This applies to **every** tagging path, not just transcripts — the sidecar path has always been able to hit it; transcripts merely made it frequent enough to measure.

### Sources fetch on their own (V38b)

A registered, enabled source is **polled on a schedule** and new items download without anyone
asking. This supersedes §15's "there is no unattended crawler": clips arrive because you added a
source, not because you pasted a URL each time.

⚠ **The superseded rule's concern was legitimate — unattended fetching can fill a stranger's disk
— so it survives as LIMITS rather than as a prohibition.** All are settings, all have defaults, and
all fail toward doing less:

| Bound | Default | Why |
| --- | --- | --- |
| `filler.fetch.every` | `6h` | How often a source is polled. Off (`0`) disables auto-fetch entirely |
| `filler.fetch.max_per_run` | `10` | Items one source may pull per poll — a collection of thousands trickles in rather than arriving at once |
| `filler.fetch.max_catalog_clips` | `2000` | A **ceiling on the whole catalog**. At the limit, auto-fetch stops; manual queueing and approved pulls still work |
| `filler.fetch.max_disk_gb` | `20` | A ceiling on what the drop-folder may consume. Same behaviour at the limit |

**Three properties that are not negotiable:**

1. **Only registered, ENABLED sources are polled.** The Sources switch already claims Loomarr
   "stops scanning, searching and downloading" from a source that is off; auto-fetch is bound by
   the same switch or that copy becomes false.
2. **Everything fetched arrives HELD.** Auto-fetch does not bypass the lifecycle — a downloaded
   clip is still tagged, still scored, and still gated by the confidence cap before it can play.
   The unattended step is *acquisition*, never *admission*.
3. **A limit that is reached is REPORTED, never silent.** An operator whose catalog stopped
   growing must be able to see which ceiling stopped it. A crawler that quietly does nothing is
   indistinguishable from one that is broken.

⚠ **Archive.org collections are the case the limits exist for.** A collection is thousands of
items; `max_per_run` is what stops "add a source" from meaning "download 8,000 files tonight".
A bulk backfill remains the **pull**'s job, where a human sees the plan and approves it.

### Two downloaders, two gates (V38b)

Filler arrives by two mechanisms with **different requirements**, and they are gated separately:

| Source | Fetched by | Needs |
| --- | --- | --- |
| **archive.org** | plain HTTP | **ffmpeg only** (to probe and thumbnail what it fetched) |
| **YouTube** | yt-dlp | yt-dlp **and** ffmpeg |

⚠ **This corrects a real defect, not a preference.** One `ingest` feature flag required *both*
binaries, so a missing yt-dlp switched off archive.org downloads too — despite that path never
invoking it. On a source build with ffmpeg installed and no yt-dlp, downloads were reported
unavailable while being perfectly runnable, and **the starter pull is an archive.org collection**,
so first-run acquisition was blocked by a binary it does not use.

The invariant "every ingest needs yt-dlp" was true when written and became false when the archive
downloader landed beside it; the gate never split. Same shape as V37's `Fetchable()` — an implicit
rule that quietly stops holding when a second case appears.

**So the surface reports per source, never one blanket verdict.** "Downloading isn't available"
was a claim about the whole subsystem made from one binary's absence; a source that can fetch says
so, and one that cannot says which tool it wants.

### The clip lifecycle: held, then filed (V38)

Until V38 a clip had no lifecycle. The folder scan catalogued it and the tagger tagged it **in
place**, which meant everything Loomarr downloaded was playable the moment it landed — tagged or
not, right or wrong. V38 gives an arriving clip a **state**:

- **held** — in Loomarr's records, **not in the playable catalog**. It is not matched into a pod,
  not attached to a filler-list, and not counted as coverage. It is waiting to be tagged, and
  then either filed or rejected.
- **filed** — the catalog proper. Everything that plays is filed; everything filed was either
  filed by a human or cleared the confidence bar below.

⚠ **Two properties this must not break, both learned the hard way:**

1. **Every existing clip migrates to `filed`.** They were catalogued under the old model and are
   playing right now; migrating them to `held` would silently empty every channel's filler pool
   on upgrade. This is the same class as `00026`'s upsert default, which would have switched off
   every existing source — a default chosen for new rows, applied to old ones.
2. **The drop-folder stays DIRECT.** A file an operator hand-copies into `FILLER_DIR` is a
   deliberate human act, so it is filed on sight. Holding it would mean a clip you placed
   yourself sits invisible until you approve it — the ceremony §7 warns about, which teaches
   people to click through gates rather than read them. **Only ingested clips are held**: pulls,
   queued downloads, and split segments, i.e. everything that arrives because *Loomarr* fetched
   it rather than because a person put it there.

### The quality gate: reject the broken, normalise the quiet (V40)

A clip that arrives is not automatically a clip worth playing. Real downloads from the archive
sources include truncated fragments, audio-only files, and spots recorded a decade apart at wildly
different levels. **All of this is handled automatically — there is no badge, no review step, and
no operator decision.** A gate an operator has to read is a gate they learn to click through (§7).

**Rejected at the scan boundary.** `ScanDir` already skips unprobeable files; these join it, and
they never become clips at all:

- **Shorter than 10 seconds.** Nothing usable as a break body is shorter. The existing guard was
  `DurationMs <= 0`, which a **2.9KB, 33-millisecond** truncated download passed — it sat `filed`
  and airable in the dev catalog, i.e. a third of a second of nothing in an ad break.
- **No audio stream.** Silence mid-break.
- **No video stream.** An audio-only file in a video catalog.

**Loudness is measured at ingest and applied at playout.** Measured across real fetched clips the
spread was **−21.8 to −32.6 LUFS** — about 11 dB, which is the clip-to-clip volume jump an operator
hears as "some of these are too quiet". The target is **−23 LUFS**, what broadcast uses. The
measurement rides the decode that already happens for artwork (§10 V39), so it costs no extra pass.

⚠ **Normalisation happens in the PLAYOUT chain by default — and that remains the default.** At
playout it is one filter on a stream already being encoded, it is reversible, and changing the
target later simply works. `FILLER_DIR` and the watch folder hold files a person put there, so
rewriting them is never something Loomarr does unasked.

**On-file normalisation is available as an explicit opt-in** (`filler.autofile.normalize_loudness`,
default **off**, V42 — maintainer decision, surfacing the mock's Tune-panel toggle). When enabled,
the auto-file step rewrites the clip in `FILLER_DIR` with ffmpeg `loudnorm` before it enters the
catalog.

⚠ **It uses the SAME `filler.target_lufs` (−23) as the playout pass, not a second target.** Two
targets in one system means a clip normalised on file is then corrected again downstream toward a
different number — double processing, and a quieter result than either setting asks for. One
target, whichever stage applies it.

⚠ **It is DESTRUCTIVE and the operator is told so.** The original cannot be recovered; that is
inherent to rewriting in place and is why this is opt-in rather than the default. What it must not
also be is *repeating*: a re-scan cannot tell by looking that a file has already been normalised,
so without a marker every pass would normalise an already-normalised file, walking the loudness
down on each run. The sidecar records `normalizedLufs` beside the clip, and the pass **skips any
file already carrying that marker at the current target**. The marker travels with the clip the
same way `originalName` does, so a restored catalog does not re-do the work either.

⚠ **Playout still normalises, and that is deliberate rather than redundant.** A file normalised on
disk measures at target already, so the playout filter is a no-op for it — while clips that arrived
before the toggle was turned on, or from a source that bypassed auto-file, are still corrected. The
guarantee "every break plays at a consistent level" cannot depend on which clips happened to pass
through one optional step.

**Language is a job, not an inline check**, and it REJECTS rather than holding (maintainer,
2026-08-03) — consistent with the other two gates, which drop a file at the boundary and leave a
log line rather than a queue entry.

⚠ **It runs in the BACKGROUND, after the clip is already catalogued.** A clip enters the catalog on
the scan and the job fills in its language afterwards, acting on the answer then. Inline was the
simpler code path and is not viable: on the local backend a 100-clip folder becomes a ~9.5-hour
scan on arm64 (see the timings below), and the sync timer would overlap itself. This is the same
shape the AI tagger already has, for the same reason.

⚠ **Only where there is audible speech to judge, and SILENCE NEVER REJECTS.** A wordless visual
spot has no language, and those are often the best filler — so "no speech detected" means keep,
never drop. Only confident non-target *speech* rejects.

⚠ **A model handed silence does not decline — it GUESSES, and arbitrarily.** This is the failure
the first live run produced, and it deleted two real clips. Two recorded ad breaks (867s and 978s)
were sampled at their first ten seconds, which on a long recording is leader: tape run-up and dead
air measuring **−70 LUFS**. Asked what language that was, the model answered `ar` for one and `es`
for the other, and the gate tombstoned both. Re-asked about the identical span later it said `en` —
so the answer is not reliably *wrong*, it is reliably *unpredictable*, which no amount of prompt
tuning makes safe to act on.

Two defences, and they are deliberately independent:

- **Long recordings are sampled from the MIDDLE.** Past two minutes a clip has stopped being one
  advert and become a recording of several, which always opens with leader. This fixes *where* we
  look — the same clips measured −25 and −28 LUFS mid-recording, squarely in speech range.
- **A span below a loudness floor is never asked about at all.** `−50 LUFS`, measured with the
  same `ebur128` the loudness half of V40 uses. This holds *wherever* we land, including on a clip
  that is genuinely silent throughout. The floor leaves wide room above the quietest real clip
  measured in this catalog (−32.6 LUFS), because treating a quiet advert as silent would be the
  same bug in the other direction.

⚠ The local backend has a third defence the hosted one structurally cannot: it checks whether
whisper transcribed anything, so silence yields no utterances and returns `none` naturally. Only
the hosted path can guess, which is why the floor exists.

**Two backends behind one seam**, mirroring `llm.provider`'s local-vs-hosted split (§8.1):

| | `filler.language_provider = whisper` (default) | `= hosted` |
| --- | --- | --- |
| Engine | vendored `whisper-cli` + `ggml-small.en.bin` | an audio-input model via the §8.1 hosted provider |
| Per clip | ~3s natively, **~341s under QEMU** | ~1s — it is a network call, so architecture stops mattering |
| Cost | free | fractions of a cent for a 10s span |
| Offline | yes | no |

⚠ **NOT Ollama, and this is the trap worth naming.** "We already run a local LLM, so we do not need
whisper" is the reasonable inference and it is wrong: Ollama has no audio input path at all. Probed
against the live dev instance (2026-08-03), its models report `completion`, `vision`, `tools`,
`thinking` — there is no `audio` capability, and `vision` is images only. Local audio means
whisper; the hosted option is what Ollama cannot be.

⚠ **The consequence for arm64, stated so nobody has to discover it:** 341s per clip is not usable,
so an arm64 install effectively has only the hosted path. The feature is off by default rather than
degrading silently there.

⚠ **Local is the default, and hosted is an opt-in that costs money and leaves the house.** Sending
clip audio to a third party is a change in posture, not a performance tweak — the same reason §8.1
defaults to local Ollama and makes hosted a deliberate choice with a key. It is also the first
feature that spends money per clip.

The seam is `MediaTools.Transcribe(ctx, file, startMs, endMs)`, which already exists for
compilation splitting. A hosted detector is a second implementation behind it, so the job, the
reject rule and every test are identical whichever backend answered.

⚠ **Brightness is deliberately measured by nothing and fixed by nothing.** Sample clips ranged
YAVG 64–127 against a mid-grey of 128, and the dim end is what an eighties VHS transfer genuinely
looks like. Auto-brightening would invent a picture the source never had — and unlike loudness it
is not recoverable at playout, because the correction would be baked into what the viewer sees with
no original to fall back to.

### Tagging confidence, and auto-filing (V38)

The tagger records a **confidence score** (0–100) alongside the tags. It exists for one job: to
decide whether a held clip is **filed automatically** or **surfaced to a human** in Incoming.
Anything scoring at or above `filler.autofile.min_confidence` is filed; anything below stays held
and waits for a person.

⚠ **The score is grounding-gated, and that is the whole safety property.** It is NOT the model's
own self-assessment, because this tagger has a measured history of confident fabrication — the
paragraph above records it inventing an era on 2 of 10 real clips, inferred from tone. A
self-reported number would be the same failure one level up: the model that fabricated the era
also grades how sure it is about the era.

So the score is built in two layers, and only the first can *raise* it:

1. **Grounding facts CAP it.** Everything `validateTags` can verify sets a ceiling — was the era
   found **literally** in the text or merely inferred; did audience and category match the known
   enums; was there any source text to check at all. ⚠ **An ungrounded era can never reach the
   auto-file threshold**, no matter what the model claims. That is a hard ceiling, not a
   subtraction, and it is the property to sabotage-test.
2. **The model refines within the cap.** The model reports its own confidence and it may only
   *lower* the grounded ceiling, never lift it. A model that is unsure about a clip whose tags all
   verify is still worth surfacing; a model that is certain about an era it invented is not.

**The consequence, stated plainly: auto-filed clips enter the catalog and can play on a channel
with no human having looked at them.** That is a real change in what Loomarr does unattended, and
it is why the ceiling above is not negotiable.

⚠ **It is also strictly safer than what it replaces**, which is worth recording because the
opposite is the intuitive reading. Before V38 an ingested clip was catalogued and playable
*immediately*, with no score and no gate — auto-filing at 85 is the first time anything has stood
between a downloaded file and a channel. The risk this section guards is not "clips reach
channels unreviewed" (they already did); it is that a *fabricated* tag reaches matching and
corrupts it silently.

**Every auto-filed clip is attributed and reversible.** `auto_filed` records that no human looked
at it, Incoming lists what was filed without asking, and sending one back to **held** is a single
action. An unattended decision that cannot be found and undone is not a decision an appliance
gets to make.

**Defaults (maintainer, 2026-08-02):** auto-filing is **ON at 85**, matching the mock. ⚠ This
means an existing install begins auto-filing on its first tagging run after upgrade without
opting in — a deliberate product call, made with the grounding cap in mind: the fabrication class
this section exists to guard against stays in Incoming regardless of the threshold, because an
ungrounded era cannot clear it.

**Loudness normalisation — SPECIFIED, NOT YET BUILT.** Clips filed automatically should be
normalised to **−16 LUFS** on the ingest path: filler is cut together from sources recorded
decades apart, and without it a break swings between a whisper and a shout, which is the single
most audible defect in a channel that otherwise works.

⚠ **`filler.autofile.normalize_loudness` is deliberately NOT in the registry yet**, and the reason
is a rule this very phase re-learned. The key was declared alongside the other two, `make
config-docs` published it, and a grep for its consumer found **nothing** — the exact
declared-but-unconsumed defect that got `filler.autofile.*` removed in V35's review, committed
inside the phase that documents the lesson. §15's rule is that a setting nothing READS does not
exist. The key lands with its ffmpeg `loudnorm` pass, in the same PR, or not at all.

**A new install has an empty drop-folder, so the first channel has nothing to break to.** The fix is a **starter pack**: `GET /v1/filler/discover?collection=<id>` lists a curated archive.org collection, the operator keeps or excludes rows, and only what survives is fetched through the ordinary ingest path. Three properties are load-bearing:

- **It is a listing, not an acquisition.** Nothing downloads until the operator chooses, so a suggested pack cannot fill a stranger's disk with clips they never asked for. This is the same rule the approval gate states for titles (§7): the machine proposes, a human commits.
- **It is the discovery path with a different argument**, not a parallel one. A starter pack that acquired through its own route would be a second implementation of ingest — the shape §10 already rejected for filler search, and the shape that let `filler_sources` ship with no reader.
- **The pack is a default, never a requirement.** "Start from scratch" is always offered, and a collection that has gone away degrades to an empty list with the reason shown — not a blocked channel. An operator with their own clips must never have to walk through someone else's taste to reach their own.

⚠ **Which collection is a curated default, not a hardcoded truth.** It is a **setting** (§15 `FILLER_STARTER_COLLECTION`) so an operator can point it at their own collection or empty it to turn the pack off, and so a collection going dead is a config fix rather than a release.

### Pulls — the approval gate arrives for filler (V35)

The three properties above are right and had **no object to hang them on**: "the machine proposes, a human commits" described an intention, while the only thing that existed was a listing endpoint and a download button. A **pull** is that object.

A pull is a **plan Loomarr composed across sources** — *"fill the 1990s kids gap"* resolving to several collections, each with a reason and an estimate. It is persisted, it appears in the approval queue beside title proposals, and **nothing downloads until it is approved**. The starter pack becomes its first seed rather than its own flow: `FILLER_STARTER_COLLECTION` seeds a pull on a fresh install instead of driving a parallel path.

**What a pull carries:** a title, who or what proposed it, a rationale, plan rows (each a source with a reason, an estimate, and the ability to drop it before approving), an aggregate estimate, and an operator note that narrows it (*"no local dealers, no PSAs"*).

Three rules, each of which is a safety property rather than a feature:

- **Approval enqueues through the existing ingest path.** A pull that downloaded through its own route would be a second implementation of ingest — the shape this section already rejects for filler search, and the shape that let `filler_sources` ship with no reader. Approval writes work items; the ordinary job does the fetching.
- **A pull whose sources are all switched off is refused, not silently empty.** Disabled sources are a real precondition; the operator is told which switch to flip rather than watching an approved pull do nothing.
- **Dropping a plan row before approval is part of the gate, not an edit afterwards.** The committed set is what the human agreed to.

⚠ **The gate binds bulk composition, not an admin's own hands.** An admin searching one source and queueing one clip stays direct — the §7 shape, where an admin may `POST /v1/titles` because the admin *is* the gate. Requiring a proposal for a single deliberate click would make the gate ceremony, and ceremony is what teaches people to click through it. What the gate exists for is what happens when *nobody is looking*: a composed multi-source plan, which is exactly what a pull is.

### Removing a clip from the catalog is a tombstone (V35)

The catalog's bulk actions include **Remove from catalog**. It marks the clip removed; it does **not** delete the row, and it does **not** touch the file.

⚠ **A row delete cannot work here.** The catalog is a synced *cache* of `FILLER_DIR`, so the next scan finds the file still on disk and re-creates the row: the operator removes a clip and watches it come back minutes later. The tombstone is what survives a re-scan, and it survives because the scan's upsert is not allowed to write it — the same protection the play counters have.

⚠ **Deleting the file is not the alternative.** Nothing in Loomarr deletes an operator's media: disabling a source keeps its clips, and deleting a source keeps its clips because they are real files that may already be tagged and pinned into a channel. The action names the catalog, and stops at the catalog.

A removed clip is excluded from the catalog listing and from pod assembly **by default**. That polarity is load-bearing rather than cosmetic — assembly loads the catalog with an unfiltered read, so an opt-in exclusion would leave a removed clip airing until some caller remembered a flag. Restoring is the same write with the timestamp cleared.

### What is waiting on a human: the Incoming queue (V35)

Between "a file arrived" and "a clip the scheduler can place" there is work only a person can finish. `GET /v1/filler/incoming` is that queue, in one read:

- **Clips whose tags need a human** — an era the tagger proposed but could not ground in the clip's text (the rule above), or a commercial with no match tags at all. These are **two different questions** and stay separate: the first has a proposed answer to confirm, the second has nothing to confirm. Bumpers and station IDs never appear — they do their bookend job untagged, so queueing them would be work that changes nothing.
- **Compilations mid-split** — the persisted split proposals, with a count of the segments an operator cannot simply accept (unsplittable, or flagged as a duplicate).

⚠ **No confidence score is reported, because nothing measures one.** The mock draws a per-item confidence bar; the tagger records neither a score nor a rationale. The queue therefore reports *why* an item is waiting, derived from its real state. An auto-file threshold (`filler.autofile.*`) is the feature that would need a real score, and it is not built — inventing one to fill a bar would put a number in front of an operator that no code produced.

### Sources can be switched off

The **drop-folder** and each **remote collection** can be **disabled**. A disabled source is **not scanned**, and **cannot enter a pull** — neither when one is composed nor when one is approved, which is re-checked at the commit point because a source can be switched off while a pull sits in the queue.

⚠ **What the switch does NOT bind, deliberately: an admin's own hands.** Keyword discovery searches the Archive globally rather than per-source, and `POST /v1/filler/ingest` takes a URL the operator typed. Both are the *single-item direct* path this section already carves out — an admin acting deliberately, one item at a time, is the person the gate exists to defer to (§7). *(An earlier draft of this paragraph claimed a disabled source was "not searched, and not downloaded from" full stop. That was never true of those two routes and was not going to become true, because neither is scoped to a source; the claim is narrowed rather than the routes gated.)*

⚠ **Clips already in the catalog stay.** Disabling a source withdraws it from *future* work; it is not a delete, and it must never look like one. This is enforced at the scan and fetch sites rather than by hiding the source in the UI — a toggle that only dims a row is a claim the system does not honour.

⚠ **The media-server library row is SCANNED again (V38c — this reverses V35).** A `library` source names a media-server library that Loomarr scans for clips, exactly as it scans a watched folder, and it carries a working on/off switch like any other source.

*What this reverses, and why it is not a quiet flip.* V35 recorded, at length, that the row had no switch **because nothing scanned a library** — the media server had been taken out of the filler path by §9.1 and a toggle would have been "a control that dims a row and changes nothing". That reasoning was sound about the code as it stood. The maintainer's decision (2026-08-02) is to give the kind real work instead of removing it from the UI: the mock's "Add a source" dialog offers Media server library, Watched folder, and Playlist/collection URL, and an operator who already keeps their commercials in an Emby library should be able to point Loomarr at it rather than being told to copy files.

⚠ **What §9.1 forbade is still forbidden.** The dependency this restores is NARROW and must stay that way:

- A library source is **one source among several**, never the catalog's only route. An install with no media server, or one whose media server is down, still gets a full catalog from its folders and remotes — the failure §9.1 removed was *"no media server ⇒ no commercials"*, and that must not come back. A library scan that fails is logged and skipped, exactly like an unreachable archive collection.
- **Clip identity is still the content hash**, never a media-server item id. This is the third identity change's whole point (below): identity comes from the bytes Loomarr can see, so a library that is re-indexed, moved, or removed cannot orphan a catalogued clip.
- **Clips still live in the clip folder.** A library scan is an *acquisition* path, not a second storage model: what it finds goes through the same intake as everything else (watch → hash → file → sidecar), so there are still no divergent paths. Loomarr does not play clips out of the operator's library in place, and never modifies it.
- **Program content stays separate.** The reason commercials were moved out of the media server was that filler could otherwise leak into a programming lineup. A library registered as a *filler source* is read for clips only; it is never offered to the suggester as programme material.

*(An earlier draft specified `FILLER_SOURCE_LIBRARY_ENABLED` as a setting. It stays deleted — V37 made sources one flat list, so a library's switch is a column on its own row like every other source, not a key in §15.)*

### Sources are one flat list (V37 — supersedes the derived/registered split)

**Every source is one row in one list**, whatever backs it: the drop-folder, the media-server
library, each Internet Archive collection, each YouTube playlist. An operator adding a source
picks a **kind** and gives a target; the list is the whole answer to *"where does filler come
from?"*

#### A fresh install ships with sources (V38c.8, maintainer)

`folder` and `library` were the only seeded rows; **Internet Archive and YouTube now seed too**,
so a new install can fetch without the operator first having to know what to add.

⚠ **The two seed differently, and the difference is a rule rather than an accident.** Three
archive collections seed with real targets; the YouTube row seeds **empty**. §10 says Loomarr
*"never recommends YouTube content itself"* — the operator brings their own playlist — so a
seeded YouTube target would be Loomarr making exactly the recommendation that sentence forbids.
The mock draws the same split: a YouTube row present but reading *"Bring your own playlist"* with
the stat *"no playlist yet"*. An empty row is an invitation; a filled one is an endorsement.

⚠ **Seeding a source downloads NOTHING.** A row records that a source exists and is allowed;
fetching is the pull's approval gate or a deliberate per-result queue. This is the same promise
the Add-a-source copy makes, and it is what makes seeding safe to do on the operator's behalf.

**The three collections, each VERIFIED against the live archive.org API rather than guessed**
(2026-08-03) — identifier, human-readable label, and item count at capture:

| identifier | label | movies |
| --- | --- | --- |
| `classic_tv_commercials` | Classic TV Commercials | 7,985 |
| `vhscommercials` | Commercials From The Vault | 17,953 |
| `tv_ads` | TV Ads | 2,951 |

⚠ **Every row carries a human-readable `label`**, not the bare identifier. `vhscommercials` is not
a name an operator recognises, and the Sources row renders the label with the target beneath it.

⚠ **The count is THREE, not the mock's "11 curated collections".** That number is fixture prose in
the prototype — there is no list behind it in code or in this doc. Five plausible identifiers were
checked and returned **zero items** (`classic_tv_ads`, `televisionads`, `vintage_tv_commercials`,
`tvcommercials`, `commercialsandbumpers`); the large collections that DO exist — `mirrortube`
(1.2M mirrored YouTube videos), `television` (735K), `vhsvault` (116K) — are general video, and
seeding them would point auto-fetch at arbitrary long-form content for a catalog meant for
30-second breaks. Three verified beats eleven invented. Expanding the list means verifying more,
not padding to a number.

⚠ **All three declare NO licence, and the seed records that honestly** — `license` stays empty,
which this section already defines as UNKNOWN and never "public domain". ~92% of archive items
declare nothing (667 of 8362 measured in `classic_tv_commercials`), so absence is the norm and
carries no permission. The row renders no licence chip rather than a reassuring one.

This **supersedes the read-model/registry asymmetry** V28 and V33 established — recorded here
rather than quietly replaced, because the superseded rule was load-bearing and its reasoning
still applies to the thing that replaces it. The old rule said: the folder is *derived from
configuration* so its switch is a setting, remote collections are *rows* so theirs is a column,
"and that asymmetry is deliberate and is why one source never appears twice."

⚠ **Two properties that asymmetry protected are NOT optional, and the flat list must carry them
itself:**

1. **"Not configured" stays expressible.** The derived rows could say *"you could set up a
   drop-folder but have not"* — a table of things-that-exist cannot say that, and it is §10's
   own answer to *"why is my catalog empty?"*, the most important question this tab answers. So
   the flat list keeps **`configured`** as a per-row fact, and the config-backed kinds
   (`folder`, `library`) are **always present as rows even when unset**. They are not created by
   an operator and cannot be removed; their target is the setting's value, blank when unset.
2. ~~**No source appears twice.** The two config-backed kinds are **singletons**.~~
   ⚠ **REVERSED in V38c — see "Many folders, many libraries" below.** The concern was real and
   survives; the singleton was the wrong instrument for it.

**Where the switch lives, now that both are rows.** A source's `enabled` is a column on every
row. For `folder` the column is the projection of `filler.source.folder.enabled` — the setting
remains the source of truth, so an operator flipping it in Settings and flipping it here are the
same act, and there is still no precedence rule to write. ⚠ **`library` remains switchless** for
the reason above: nothing scans a library, so its toggle would change nothing. Flattening the
list does not create work for a switch to do.

**Kinds.** `folder` · `library` · `archive` · `youtube`. Each declares whether it is
*searchable* (archive today), *fetchable*, and whether an operator may add or remove it (`folder`
and `library` are neither). ⚠ **`packs` — the dizqueTV/Tunarr-wiki bumper packs — is deliberately
NOT a kind yet.** There is no pack index to read: no URL, no manifest, no fetcher. A row for it
would be a control that dims and changes nothing, which this section forbids two paragraphs up.
It returns when there is something real behind it.

⚠ **A source's licence stays OFF the wire** (V37 decision). It is stored per source, but §6.3
measured ~92% of archive.org items declaring none, and a badge that is absent for nine rows in
ten teaches an operator to read absence as "fine" rather than "unknown". The stored value is
audit, not UI.

### Two folders, one pipeline (V38c)

**`FILLER_WATCH_DIR` (`filler.watch_dir`) — the watch folder.** Where clips arrive: downloads land
here, and an operator drops files in here. Loomarr **drains** it. Defaults to `<clip folder>/_watch`
so a zero-config install has one without the operator mounting a second volume.

**`FILLER_DIR` (`filler.dir`) — the clip folder.** Loomarr's own store. Every clip lives as
`<hash>.<ext>` with `<hash>.info.json` beside it, sharded two levels (`a3/f9/<hash>.mp4`).

⚠ **The clip folder keeps the EXISTING key rather than gaining a `filler.clip_dir` twin.**
`filler.dir` has always meant "where the clips are", which is exactly what the clip folder is —
its meaning did not change, only its layout did. Minting a new key would mean migrating every
reader, retiring an identifier, and leaving two settings whose difference an operator would have
to learn. The new concept is the watch folder, so the watch folder is what gets a new key.

**Every source uses the same plumbing — there are no divergent paths.** YouTube, Internet Archive
and a hand-dropped file all take one route:

1. **Arrive** in the watch folder (downloader writes there; operator copies there).
2. **Hash** the file — the sparse content hash below.
3. **Move** it into the clip folder as `<hash>.<ext>`. Already present ⇒ it is a duplicate; the
   arriving copy is discarded rather than catalogued twice.
4. **Write the sidecar** beside it, carrying tags and provenance.
5. **Catalogue** the clip.

⚠ **The original filename is preserved IN THE SIDECAR before the rename**, and this is
load-bearing rather than sentimental. §10's grounding rule accepts an era only when the year
appears literally in the clip's text signals — and the filename is one of them
(`Frosted Flakes 1993.mp4`). Renaming to a hash without capturing the name first would destroy
that signal permanently, so every clip whose era came from its filename would become ungrounded.
The tagger reads `originalName` from the sidecar instead of from the path.

⚠ **Loomarr rearranges only its OWN clip folder.** The watch folder is drained, never
reorganised; nothing outside those two directories is touched. §10's promise — Loomarr never
deletes an operator's media — is unchanged: a file moves from watch to clip, it is not destroyed.

⚠ **Why moved rather than copied.** A watch folder that never empties is a second copy of the
whole library, and the operator would have to tidy it by hand forever. Draining is what makes it
a *watch* folder.

### Clip identity is a content hash (V38c — the third identity change)

**A clip is identified by a hash of its bytes; its path is data, not identity.** Both are stored,
each doing what it is good at:

- **`id` = sparse content hash** — the first 64 KB, the last 64 KB, and the file size. Two seeks
  rather than a full read, so hashing a 200 MB compilation costs about what a 2 MB clip does.
- **`path` (+ its folder) = where the file lives** — human-readable in logs and the UI, the thing
  playout opens, and the **filename the tagger reads for era and brand**. Identity moving off it
  changes nothing about that signal.

**Why identity had to move.** V38c allows many watched folders (below), and a path is only unique
*within* its folder — two folders each holding `ads/coke.mp4` produced the same identity, and one
silently overwrote the other. Prefixing with a folder id would have fixed the collision; hashing
fixes it **and** answers the question a prefix cannot: *is this the same advert?*

**Duplicates are not saved.** The same file found in two folders is catalogued **once**, first
scan wins; the second is skipped. ⚠ **This is only safe because identity is the hash.** If the
operator deletes the winning copy, the next scan finds the survivor, computes the *same* id, and
re-catalogues it — the clip returns **with its tags intact**. Under path identity that would have
been a different clip and the tags would be gone.

⚠ **Three caveats, written down rather than discovered:**

1. **A sparse hash is a heuristic, not a cryptographic guarantee.** Two files could share a size,
   a head and a tail while differing in the middle — in practice a truncated duplicate or a
   deliberately constructed file. The consequence is one clip shadowing another in the catalog,
   which a re-scan does not fix by itself. Tolerable for filler (clips are a synced cache, and
   the operator can delete the shadowing file), and stated here because a silent shadow is worse
   than a documented one.
2. **A re-encoded or trimmed file is a DIFFERENT clip, and loses its tags.** That is correct — it
   is a different file — but an operator who re-encodes their library re-tags it. Worth saying
   because "I only changed the bitrate" does not feel like "I replaced the clip".
3. **The migration DROPS the catalog** (`00033`), because ids cannot be recomputed without
   reading every file, and a migration that does I/O over an operator's whole media library is
   not a migration. Clips are a synced cache and the next scan repopulates paths.

   ⚠ **Tags are recoverable, play counts and pins are not.** Sidecars (below) carry era, audience,
   category and confidence back into the catalog on the next scan — which is most of what an
   operator typed. Play counts and channel pins live only in the database and do not survive.
   ⚠ **The loss is surfaced in the app**, not left to be noticed: an operator whose pins vanished
   must be told why, for the same reason §10 requires an auto-fetch limit to report itself rather
   than silently doing nothing.

   ⚠ **Sidecar recovery only helps installs that HAVE sidecars** — i.e. clips Loomarr downloaded,
   plus anything tagged after V38c ships. A pre-V38c install that hand-dropped and hand-tagged its
   whole library has none, and loses those tags outright. That is the case the warning is for.

### Sidecars: metadata travels with the clip (V38c)

A **sidecar** is a small JSON file beside a clip — `ads/coke-1985.mp4` and
`ads/coke-1985.info.json`. `clipfetch` has always written one at download (yt-dlp's
`--write-info-json` shape: title, description, upload date, source URL, licence) and Loomarr has
only ever READ it.

**Loomarr now writes to it too**, recording what it worked out: era, audience, category, kind and
the confidence score. So the metadata **travels with the file**. Reset the database, move the
folder to another install, or take migration `00033` above, and the tagging comes back on the next
scan instead of being retyped.

⚠ **JSON, not `.nfo`** (maintainer, 2026-08-02) — considered, because `.nfo` is the convention in
exactly this ecosystem (Kodi/Emby/Jellyfin/*arr) and an operator would recognise it. Three things
decided it: we would be *extending a file that already exists* rather than adding a format;
`.nfo`'s schema is `<movie>`/`<episode>` and has no element for era-as-decade, audience or a
confidence score, so we would be inventing custom tags inside someone else's schema — compliance
in appearance only; and **nothing consumes it**, because §10 took the media server out of the
filler path, so the interoperability argument that justifies `.nfo` elsewhere does not apply here.
If filler ever lives in a scraped media library, that changes and `.nfo` becomes the better bet.

⚠ **This is the first time Loomarr writes into the operator's media folder, and the promise it
must not break is narrower than "never write".** §10's existing rule is that Loomarr never
*deletes* an operator's media. A sidecar honours that — it adds a file, never removes or rewrites
one the operator authored. **Loomarr may only ever create or update `*.info.json`**; the media
files themselves stay byte-for-byte untouched.

⚠ **The held/filed fork moves from the file's EXISTENCE to a FIELD inside it.** V38b decided
"downloaded vs hand-dropped" by asking whether a sidecar existed — which stops working the moment
Loomarr writes sidecars for hand-dropped clips too, because then everything has one. The download
path now stamps **`"fetchedBy": "loomarr"`** and the fork reads that.

That is a better signal than the one it replaces, not merely a repair: **existence was inferred,
a field is explicit**. An operator who copies a clip together with its sidecar gets the honest
answer either way, and one who tidies sidecars away no longer flips a clip's lifecycle by
accident.

### Many folders, many libraries (V38c — reverses V37's singleton rule)

An operator may add **any number** of watched folders and media-server libraries, not one of each.
Commercials living in two places is an ordinary situation, and V37 gave it no expression at all.

⚠ **This reverses a rule V37 added one phase earlier, so the reasoning is recorded rather than
edited away.** V37 made `folder`/`library` singletons because V28/V33's superseded asymmetry had
protected "no source appears twice", and a flat list that let someone add a second folder beside
the derived one looked like exactly that double-listing.

**The concern was right; the instrument was wrong.** What must not happen is ONE folder appearing
as TWO rows — a stale row disagreeing with the setting about the same directory. Forbidding a
second *distinct* folder does not prevent that, and it forbids something legitimate. So:

- **Uniqueness is on the TARGET, not the kind.** One row per distinct path or library id. Adding
  a folder already listed is refused as a duplicate, which is the actual invariant.
- **`filler.dir` is the FIRST folder, not the folder.** It seeds a row on a fresh install so a
  zero-config install still has somewhere to drop files, and it remains the default the ingest
  path downloads into. It stops being the only one anything scans.
- **The scan walks every enabled folder row**, not just `filler.dir`. A folder that is switched
  off is not scanned — the same promise the switch already makes.

⚠ **"Not configured" still has to be expressible** (property 1 above, unchanged). A fresh install
shows its seeded folder row with a blank target and a `not configured` badge, because "you could
set up a drop-folder but have not" is §10's own answer to *"why is my catalog empty?"*.

### Per-source fetch overrides (V38c)

`filler.fetch.every` and `filler.fetch.max_per_run` gain **per-source overrides**. A busy archive
collection and a small playlist genuinely want different numbers, and one global figure serves
neither well.

⚠ **Unset must be NULL, never 0.** `0` already means something for `fetch.every` — *never
auto-fetch this source* — so "inherit the global" cannot share that encoding. A column defaulting
to `0` would read as "every existing source is switched off", silently, on upgrade. That is the
`00026` mistake (a default chosen for new rows applied to old ones) and it is the thing to
sabotage-test here.

⚠ **The catalog and disk ceilings stay GLOBAL.** They bound the whole install — what the operator
is protecting is one disk, not one source — and a per-source disk cap would let four sources each
stay under their limit while together filling the volume.

### What the Sources tab shows (V38c — the mock, read properly)

⚠ **V37 built this tab from the delta doc's SUMMARY rather than the mock's markup and JS**, which
is how it ended up structurally right and detailed wrong. The summary described the shape (flat
list, toggles, a search expander) and that shipped faithfully; the things only the source shows —
the kind badge, the greying, the combined stat line, the three-kind picker, the per-kind copy —
were never read. **A summary is not the source**, which is the same lesson the truncated-fetch
correction records one section up.

Per row: an on/off switch · a **kind badge** (fixed-width, colour-coded) · name + description ·
a **stat** reading *"6 clips · scanned 2m ago"* · an optional Search expander · an optional
remove. ⚠ **A disabled row is GREYED** (the mock's `sv.opacity`), not merely badged — the switch's
effect has to be visible at a glance down a list.

**A config disclosure per row** (V38c), on the same shelf as the search and URL expanders the mock
already draws. It shows the source's target **read-only** and makes its *behaviour* editable —
enabled, and the fetch overrides above. ⚠ Re-pointing a source is deliberately NOT offered:
changing a folder's path orphans every clip it brought in, which stay attributed to a source that
no longer means the same thing. Remove and re-add makes that explicit rather than silent.

**Two status lines, and they are different.** `svcOnLine` on the tab header reads *"4 of 5 on"*.
The page header's pill — on **every** tab, with a live pulse — reads *"4 of 5 sources on · 9 clips
· last scan 2m ago"*. ⚠ The scan time is the load-bearing part: with auto-fetch running
unattended, *"is this thing actually running?"* is the question the header exists to answer, and
counting rows does not answer it.

⚠ **No Sync or AI-tag buttons in the header** (V38c). Both jobs run on schedules, so a manual
trigger asks the operator to do work the appliance is already doing; the mock draws the status
pill in that space instead. The capability is not lost — the Tasks page has Run-now for every
scheduled job — and the pill's "last scan" is what those buttons were really being used to check.

### Compilation splitting (V34)
Discovery (V33) surfaces a source; ingest downloads it. But a large share of what discovery finds is a **compilation** — one file holding twenty or more commercials back to back. Ingested whole it is a single 15-minute "clip" the pod assembler can never place (`durationEligible` rejects anything far longer than a break); split blindly it is twenty files named `compilation_seg07` with no era, audience or category, which the ladder cannot place either. **Splitting and metadata are one phase because either alone produces unplaceable clips.** The pipeline, designed from measurement on six real compilations rather than reasoning (plan §6.4 — every number below names its method there):

1. **Triage.** A source with chapters splits for free (chapters are exposed without downloading the file). Rare in practice — 6 of 8 sampled sources had none — so this is an optimisation, not the mechanism.
2. **Coarse split.** ffmpeg's `blackdetect` + `silencedetect`, parsed in Go; segments under ~3s are dropped. Scene-cut detectors (`scdet`, PySceneDetect) were measured and rejected: they fire on camera cuts *inside* an advert — the wrong granularity, not a tuning problem. Detection quality is a property of the **source**, not of any threshold (69–100% across the six compilations; two had genuinely absent boundaries no setting fixes).
3. **Rescue.** A segment far longer than a plausible advert means boundaries the A/V pass could not see; it goes to **transcript (whisper) + LLM** for cut points. ⚠ The LLM must return **exactly one entry when the transcript is a single advert** — without that instruction it invented cuts at suspiciously round 30/61/92s marks inside one 121s infomercial. With no runnable whisper (`INGEST_WHISPER_PATH`, §15) an over-long segment is not guessed at: it surfaces in the review as **unsplittable**.
4. **Metadata.** Each segment's transcript feeds the **existing** text-signal classifier unchanged (above) — it already knows `cereal`, `toys`, `cars`. Era follows the grounding rule above: persisted only when the year appears in the text, else carried on the proposal as an unconfirmed suggestion.
5. **Dedup.** The same advert recurs across compilations. A dHash over frames sampled at 1/3fps — ~30 lines of pure Go over `ffmpeg -pix_fmt gray` output, no library, no cgo — separates a re-encoded duplicate from a different advert by a measured 25× margin (mean per-frame Hamming 1.1 vs 27.6–32.2), so any threshold in the teens works. Matches are **flagged on the proposal**, never silently dropped — the operator sees "already in the catalog" and decides.
6. **Review — required unless the result is unambiguous (V43).** Because detection quality is a property of the source, an uncertain result is confirmed by a human before anything enters the catalog; auto-accepting a 69% result puts 3-minute "commercials" into 30-second breaks. Detection runs as a **job** (minutes per file) producing a **persisted split proposal** (§5) — review can happen long after detection, and a restart must not lose it — and an unconfirmed proposal writes nothing until `POST /v1/filler/splits/{id}/confirm` (§7) commits a cut list.

   ⚠ **This was "not optional, ever" until V43, and the blanket rule was over-applied.** The same argument — quality varies by source — produced a *threshold* everywhere else in this pipeline: the tagger files a clip unattended above `filler.autofile.min_confidence` and asks below it. Splitting alone demanded a human for every reel, including the ones where every segment is plainly an advert. That asymmetry is what made compilations the most manual part of a system whose whole claim is that it maintains itself: Loomarr would decide unsupervised that a clip is a 1993 cereal advert and air it, but not accept a cut point it was certain about.

   **`filler-split` is a scheduled job** (on by default). It proposes splits for over-long catalog clips rather than waiting for a click, so proposals are ready when the operator looks instead of costing minutes of waiting once they do.

   **Auto-confirm is a separate opt-in** (`filler.autosplit.enabled`, default **off**), gated on `filler.autosplit.min_confidence` — deliberately NOT reusing the auto-file threshold. The failure modes differ in kind: a mis-*tagged* clip plays in the wrong break, a mis-*cut* clip plays half an advert. One dial would force the stricter case to govern both.

   ⚠ **The gate is ALL-OR-NOTHING over the reel, not per segment.** A proposal auto-confirms only when *every* segment is advert-shaped (within `filler.autosplit.max_duration`, and above the existing `filler.min_duration` floor), *none* is flagged `unsplittable`, *none* is flagged a duplicate, and *every* one classified above the threshold. One doubtful segment sends the whole reel to review.

   That shape follows how the 69% case actually fails. A badly-split reel is not uniformly slightly-wrong; it has obvious tells — one 6-minute block where the detector saw no boundary, sitting beside perfectly good 30-second cuts. Confirming the good segments and surfacing the rest would split one reel's decision across two places and hand the operator fragments to judge without the picture. The filmstrip exists to show that whole picture, and it is what the operator sees for the reels that genuinely need them.

### Break & pod policy (per channel)
The scheduler assembles realistic **ad pods**, not single random clips:
- **Pod structure:** intro bumper → 2–4 matched commercials → return bumper, sized to the flex gap.
- **Matching rules:** `era` to the block (90s sitcom block → 90s ads), `audience` to the channel (Saturday-morning cartoons → toy/cereal ads, not car insurance), `category` variety within a pod so it doesn't play three car ads back to back.
- **Per-channel filler selection (`policy.filler`, the `FillerSelection`).** A channel narrows its own break content — the era/audience/category/kinds it draws from, plus specific clips to always include or never use — rather than every channel drawing the same global pool. It lives on `ChannelPolicy` (persisted in `policy_json`, no new column; edited on the channel page like the other programming rules). The shape: `era` (a year range; unset = any, **seeded from `policy.scope.era` at channel creation** so a 90s channel gets 90s ads out of the box), `audience` (unset = any), `categories` (empty = any; a subset of the closed category set), `kinds` (empty = the default commercial+bumper+station_id; else the chosen subset), `pinned` (clip ids always included), `excluded` (clip ids never used). Every field is optional and an empty selection == the whole catalog (the prior behavior), so this is additive.
- **How the selection reaches assembly.** The theme filter is applied as a **catalog pre-filter** (`[]Clip → []Clip` by category + kinds) plus `Window.Era`/`Window.Audience` from the selection — replacing the previously **hardcoded** `PodEra→0` and empty audience. `excluded` ids are pre-seeded into the assembler's no-repeat set (`used`), which already excludes at every pick site, so exclusion needs no ladder change. `pinned` ids are placed as a **top-priority pool** at the front of the commercial fill before the ladder takes the rest (the one genuinely new assembly step, since the ladder ranks pools and has no force-include). If a clip is both pinned and excluded, **exclude wins** (the safe default). *(Historical note: the assembler once passed `general` as the channel audience under a comment claiming it "matches broadly" — the opposite of the filter's actual behavior — so every channel's filler-list held only bumpers + the fallback card, §10's central feature silently doing nothing; found by building the §12 pod preview. The per-channel selection above is what finally wires real era/audience through.)*
- **Density:** target break length and breaks-per-hour; min/max filler duration. **Break placement (the scheduler's job, §9):** the scheduler interleaves break slots between program slots at `FILLER_BREAKS_PER_HOUR` — a break roughly every `60 / breaks-per-hour` minutes of accumulated program runtime (default 4/hr ⇒ ~every 15 min). Because Tunarr only inserts filler at **program boundaries** (below), breaks snap to the nearest boundary: walk the ordered program slots summing durations, and when the running total crosses the next break threshold, emit a `SlotFiller` break *after* the current program and reset the accumulator. This is duration-aware — a 90-min movie gets several breaks, a 22-min sitcom about one — and it inserts `SlotFiller` gaps that the reconcile's pod assembler (`fillPods` → `Assemble`) fills with matched pods. **Breaks are only interleaved when a filler pool actually exists** (the reconcile builds the pool up front and passes `BreaksPerHour 0` when it's empty / no `FILLER_DIR` / no `PodFiller`): inserting break gaps with no clips to fill them leaves empty flex that Tunarr renders as large **channel-named blocks** in the guide — a promise of commercials it can't keep. No pool ⇒ programs play **back-to-back** (still "never dead air"). Self-healing: once clips land, the next reconcile sees a pool and re-inserts breaks. Deterministic: the same lineup + seed yields the same break positions.
- **Repeat avoidance:** don't repeat a clip within a session/window.
- **Fallback ladder:** exact-era match → widen era → any appropriate-audience clip → channel bumper card (Tunarr's flex fallback). Never dead air. Loomarr **ships a default bumper-card asset** (embedded) and sets it as each channel's Tunarr fallback at creation, so the bottom of the ladder exists on day one; operators can replace it per channel.

### Break placement: a per-backend capability
Loomarr drives Tunarr's **Flex** (time between programs) + **Filler lists**. A channel's commercials live in a Tunarr **filler-list** (`/api/filler-lists`, referencing the Tunarr-`local`-source clip program ids) that Loomarr builds from its matched catalog and attaches to the channel; Tunarr then plays clips from that list into the flex gaps the scheduler leaves between programs (§9 break placement). Both the filler-list programs and the channel's flex gaps are Loomarr-managed, so pods reproduce deterministically. Tunarr inserts filler at **program boundaries** — breaks *between* episodes/movies — not true mid-episode cut-ins. Real TV cuts mid-show; Tunarr generally doesn't unless the content itself is pre-segmented into parts.

**Where breaks can go depends on the channel's playout backend (§9.1) — this is the clearest functional difference between the two:**

| Backend | Break placement |
| --- | --- |
| **Tunarr** | **Between programs only.** The limitation above is Tunarr's, and it is not going away — design for between-program pods on these channels and be upfront about it in the UI. |
| **Loomarr (internal)** | **Between programs *and* mid-roll.** Owning the encoder means owning the cut points, so a break can land inside a program. |

**Mid-roll is therefore in scope for internal-playout channels** (it was previously out of scope everywhere, because Tunarr was the only backend — see the §20 note struck alongside this change). It carries its own costs, decided deliberately:

- **Detection is opt-in per channel, not library-wide.** Finding cut points means decoding the file, which is minutes per title; running it across a whole library to serve a handful of channels is waste. Detect only for titles on channels with mid-roll enabled.
- **The guide does not advertise mid-roll breaks.** Breaks stay an internal scheduling detail; a break rendering as its own EPG entry is confusing in the family's TV guide, and empty breaks have already caused exactly that (a bare channel name between episodes).
- **Everything else is unchanged.** Pod assembly, the relaxation ladder, determinism and the shared assembler (below) are backend-agnostic — a mid-roll pod is assembled by the same code, from the same catalog, with the same seed, as a between-program one.

### AI assist (optional, opt-in)
Two jobs the suggester (§8) can do here, both under the same grounding rule (can only reference clips that actually exist in the filler catalog):
1. **Classify/tag** ingested filler so matching works without manual tagging.
2. **Assemble pods** matched to a block's vibe, and flag gaps — "the Saturday-morning channel has no 80s toy ads" — so you can point the `FillerSource` at a playlist to fill them.

### Config
Core: `FILLER_DIR` (the drop-folder path Loomarr registers as a Tunarr `local` media source — replaces the old `FILLER_LIBRARY` media-server-library id, which is removed since the media server is no longer in the filler path), `FILLER_SYNC_EVERY`, `FILLER_AI_TAGGING`, and pod/density knobs (see §15). **Ingest config now lives in the core** (revised — it previously belonged to the sidecar, which no longer exists): `INGEST_YTDLP_PATH` and `INGEST_FFMPEG_PATH` (defaulted to the vendored binaries on the `filler` variant; overridable so an operator can point at a newer yt-dlp without waiting on a loomarr release — the tool ships fixes far faster than we cut images), plus `INGEST_MAX_CONCURRENT` and `INGEST_TIMEOUT`. ⚠ **"Ingestion targets are supplied per-request by an admin — there is no unattended crawler" is SUPERSEDED (V38b).** A registered source now fetches on a schedule; see "Sources fetch on their own" below for what bounds it. The superseded rule's concern was right and is preserved as the limits there, not discarded. **Migration note (THRICE revised):** the `FILLER_LIBRARY` env var and the media-server-item-id clip identity were superseded by the Tunarr `local`-source program id — itself superseded by the clip's path relative to `FILLER_DIR` (§9.1: internal playout needs a playable input, and it must not require Tunarr to discover its own files) — and **that is now superseded by a content hash (V38c, see "Clip identity is a content hash" below)**. Each step moved identity closer to the thing Loomarr actually owns: from a foreign id, to a path we control, to the file's own bytes.

---

## 11. Users, authentication & permissions

Multi-user, **Loomarr-owned identity**: the local `users` table is the source of truth (the allowlist — you can sign in iff you have a row), and roles gate the actions that spend real resources. **Three** credential paths land on that one identity: **local** users authenticate against a Loomarr-stored bcrypt hash; **imported** media-server users authenticate against Emby/Jellyfin (Loomarr never stores their password); and **SSO** users authenticate against an OIDC provider (V8, below). All three resolve to the same allowlist — a credential proves who you are, the `users` row decides whether you may enter. A media-server account grants no access until an admin has **explicitly imported** it — signing in is not self-provisioning.

*This replaces the earlier "first media-server admin to sign in claims the instance / users created lazily on first login" model. The reason: identity should be Loomarr's to own, so an install works with zero media-server config, and access is an admin decision, not a side effect of who happens to hold a media-server account.*

### Identity is the DB; credentials are per-user
- The `users` table (keyed by user id) is the **allowlist and source of truth**. Each row carries an optional `password_hash` (bcrypt): set ⇒ a **local** user (verified in-app); null ⇒ an **imported** media-server user (verified against the media server). Role/quota/disabled are Loomarr-owned regardless of credential path. The credential path itself is exposed as `local` on the user body (hash set ⇒ true) so the UI can label rows and explain why password actions apply to some users and not others — the hash is never exposed, only whether one exists.
- A login attempt for a username with **no matching row is rejected** (`invalid credentials`, indistinguishable from a wrong password) — an un-imported media-server user is denied *even with valid Emby credentials*. There is no lazy self-provision.

### Authentication
- **Local users:** `POST /v1/auth/login` verifies the password against the row's bcrypt `password_hash` (constant-time; a missing hash never verifies).
- **Dev login (development only, default OFF):** `POST /v1/auth/dev-login` issues an ordinary admin session with no credential, so a maintainer working on the UI is not locked out by a wedged backend or a forgotten password. It is **gated by a server-side environment variable, `LOOMARR_DEV_LOGIN=1`** — deliberately *not* a build-time flag, because a bundler constant travels inside the artifact and the same `dist/` could ship to production carrying the bypass. An operator must set the variable on the server; the default is closed. When unset the route is **not registered at all** and returns 404 — indistinguishable from a build that never had it (§19 pins this as a negative test). It selects the **lowest-id existing admin** and never creates, promotes, or enables a user: it is a shortcut past the *credential check*, never past the allowlist (§11's invariant holds — you can sign in iff you have a row). It refuses when no admin row exists, rather than bootstrapping one. Boot **WARNs on every startup** while the flag is on, because a bypass nobody remembers enabling is the failure mode worth shouting about. It is not a credential path in the sense the other three are — it is a sanctioned bypass of the *credential check*, for the maintainer's dev loop, and must never be reachable in a shipped install. (The phrase "the only sanctioned third credential path" here predated SSO, V8; the distinction that matters is bypass-vs-credential, not the count.)
- **Imported media-server users:** verification delegates to `POST {LIBRARY_URL}/Users/AuthenticateByName` (shared Emby/Jellyfin endpoint). On success the server returns an `AccessToken` + `User`; Loomarr verifies, **discards** the media-server token (best-effort `POST /Sessions/Logout`), and — critically — only proceeds if the user id is already an allowlisted row. Media-server passwords are never persisted or logged.
- **Flavor quirk (encode in the adapter):** Jellyfin requires a client-identification authorization header **on the login request itself** — `Authorization: MediaBrowser Client="Loomarr", Device="…", DeviceId="…", Version="…"` — even before any token exists. Emby accepts the equivalent `X-Emby-Authorization` header. Extend the existing flavor-specific auth handling (§6) rather than special-casing.
- Either path issues Loomarr's **own session**: HTTP-only, `SameSite=Strict` cookie signed with the generated `SESSION_SECRET` (config-design §4). Sessions are rows in the store (revocable **and reviewable** — `GET /v1/users/{id}/sessions` lists a user's live sessions for an admin, `DELETE /v1/sessions/{hash}` ends one), not stateless JWTs — disabling a user kills their sessions immediately; sliding `SESSION_TTL` (§5) expires idle ones. Cookies set `Secure` per `cookie.secure=auto|always|never` (`auto` honors direct TLS or `X-Forwarded-Proto: https` from a reverse proxy — plain-HTTP LAN installs still work). Session tokens are random 256-bit values, **SHA-256-hashed at rest** (a DB read never yields a usable cookie). Mutating routes additionally require a static `X-Loomarr-Csrf: 1` header — combined with `SameSite=Strict`, that closes form-based CSRF cheaply. Rate-limit login attempts.
- The `DeviceId` in the media-server login header is stable per install (derived from an instance id generated at first migration), so Loomarr appears as one device in the media server's dashboard.
- **Machine access:** the generated `API_TOKEN` (config-design §4) authenticates non-human clients (scripts, an external scheduler) via `Authorization: Bearer` and doubles as break-glass admin — it is the escape hatch if the media server is down *and* before any user exists.

### SSO is a credential path, not a provisioning path (D-F, V8)

An **OIDC** provider (Authelia, Authentik, Keycloak, any compliant issuer) becomes a **third
credential path onto the one identity §11 already owns** — exactly parallel to imported
media-server accounts. The provider proves *who you are*; the `users` table decides *whether
you may enter and what you may do*.

Every existing invariant holds verbatim, which is why this is an addition rather than a
change:

- ⚠ **An SSO identity with no allowlist row is REJECTED**, even with a perfectly valid token
  from a correctly configured provider. This is the direct analogue of *"an un-imported
  media-server user is denied even with valid Emby credentials"*, and it is pinned by a §19
  negative test.
- ⚠ **SSO does not provision.** There is no `auth.sso.auto_create` key and no code path that
  creates a row on first sign-in. Lazy self-provision is what §11 exists to prevent: it makes
  access a side effect of who holds an account somewhere else. **Explicit import (or an
  admin creating a local user) remains the only way in.**
- ⚠ **Roles stay Loomarr-owned.** There is no `auth.sso.admin_group` and no group-to-role
  mapping. A provider that says you are in `loomarr-admins` is telling us about *its* world;
  role is a decision an admin makes here — consistent with the People tab's existing stance
  that Loomarr does not infer role from the media server either.
- **Loomarr's own sign-in always works alongside** SSO, never instead of it. An install whose
  provider is down or misconfigured must not be an install nobody can enter; the break-glass
  `API_TOKEN` remains the final escape hatch.

**Mechanism: OIDC only — deliberately not forward-auth.** The mock also draws a proxy header
mode (`Remote-User` + a trusted-CIDR allowlist). It is not built, and the reason is a safety
asymmetry rather than effort: header trust is only as strong as the operator's network wiring,
and a Loomarr reachable on the LAN *beside* its proxy would accept `Remote-User: anyone` as
proof of identity — a total authentication bypass with no signal that anything is wrong. OIDC
carries its own proof (a signed token verified against the issuer's published keys), so there
is no network-topology assumption to get wrong. Forward-auth stays open work (§20); if it is
ever added, the trusted-CIDR check is load-bearing in the way the allowlist is.

**Verified against two real providers, and each found a bug the other could not.** `make
test-sso` stands up **Authelia** and **Authentik** in containers and drives the whole flow.
This is not belt-and-braces: a hand-written stub IdP is one reading of the spec on *both*
sides of the wire, so a misunderstanding agrees with itself and stays invisible.

- **Authelia** found that profile claims live at the **userinfo endpoint**, not in the
  id_token — it follows the spec strictly. Loomarr read only the id_token, so `MatchName()`
  fell through to an opaque `sub` UUID and **every login against a default Authelia was
  refused**. Claims are now fetched from userinfo, with the response's `sub` cross-checked
  against the token's (a substituted response could otherwise name a different person).
- **Authentik** found that its issuer is path-based **with a trailing slash**
  (`…/application/o/loomarr/`). OIDC requires an exact issuer match, and Loomarr was
  trimming it — discovery failed outright, so an operator pasting the value Authentik
  *displays* could never connect. Only whitespace is trimmed now.
- They also **disagree** about something easy to mistake for a rule: Authelia refuses plain
  HTTP (it derives its issuer from the request); Authentik serves discovery over it happily.
  So "a provider requires HTTPS" is per-provider, and Loomarr assumes neither.
- Two operator-facing setup requirements worth knowing, both harness findings rather than
  Loomarr bugs: **Authentik needs a signing key assigned to the provider**, or it signs with
  HS256 and a compliant client rejects the token; and **Authelia's session cookie domain must
  match the URL Loomarr is reached at**, or discovery 400s.

**Matching an identity to a row.** The provider's subject (`sub`) is stable but opaque and
means nothing to an operator reading the People list, so an SSO login matches on the
**preferred username** claim (falling back to `email`) against the same `users.name` the other
paths use. That keeps one allowlist rather than a parallel SSO-identity table, and it is why
the *"what the provider told us"* claims dump is worth building: when a login is refused, the
operator needs to see which claim arrived so they can tell a misconfigured provider from a
missing row.

**The callback is bound to the browser that started the login.** State and nonce are minted
per login and held server-side, but a server-side map answers *"did some login start here?"*,
not *"did **this** browser start it?"* — those are different questions, and only the second
one is CSRF protection. So `start` also writes the state to a **short-lived cookie**, and
`callback` refuses unless that cookie matches the `state` it was handed.

- ⚠ **`SameSite=Lax`, not `Strict`, and this one is load-bearing.** The callback arrives as a
  cross-site top-level navigation *from the provider*, and a `Strict` cookie is not sent on
  it — so `Strict` here does not harden the flow, it breaks every SSO login. `Secure` follows
  the same `cookie.secure=auto|always|never` the session cookie uses (§11), because a homelab
  install on plain HTTP must still be able to sign in.
- ⚠ **`SameSite` on the *session* cookie does not cover this.** SameSite governs when a cookie
  is **sent**, not whether a `Set-Cookie` is **stored** — and the callback's job is to store
  one. Nor does the `X-Loomarr-Csrf` header: that guards mutating Huma routes, and this is a
  plain-mux `GET` mounted outside that middleware.
- Without the cookie, someone the provider authenticates could capture their own
  `?state=&code=` redirect and get another person's browser to follow it, landing that person
  in the app signed in **as them** — every subsequent action attributed to the wrong account,
  including anything the approval gate (§7) records.
- **PKCE** (S256) is sent alongside. It does not close the above on its own — an attacker
  holds their own verifier — but it binds the authorization code to the flow that requested
  it, and OAuth 2.1 requires it.

**The post-login redirect is validated where it is emitted, not only where it is accepted.**
`next` is a same-app path, and *path* means parsed rather than prefix-matched: a backslash is
the case that prefix-matching misses, because browsers resolving a special-scheme URL treat
`\` as `/`, so a `Location: /\evil.test` navigates off-site while looking like a path. The
value crosses a package boundary and a persisted map between the two routes, so `callback`
re-validates rather than trusting a gate three hops upstream.

### Device authentication for playout (§9.1) — the one path that isn't a person

Internal playout (§9.1) serves segments to a **television**, which cannot hold a session cookie. Those routes therefore authenticate a **device** by token rather than a **person** by session — the only route family that does not resolve to a `users` row. Stated explicitly rather than left implicit, because §11's whole model is "identity is the DB":

- **Scope is playout and nothing else:** the tuner M3U, the XMLTV guide, and segment reads. A valid `playout_token` grants **no** API access, no user identity, and no write of any kind.
- **Read-only by construction.** There is no playout route that mutates state, so a leaked token exposes the streams — the same content the media server already serves to the household — and never the approval gate, settings, or user data.
- **It does not touch the allowlist.** A device is not a user, is never provisioned as one, and cannot become one. The invariant that *"a login attempt for a username with no matching row is rejected"* is untouched, because this path has no username.
- **Rotation is a deliberate, gated action.** Regenerating `playout_token` invalidates the media server's wiring: guide entries survive, playback stops until Live TV is re-connected. The UI requires a typed confirmation and says exactly that.
- **Redaction applies** (config-design §4): the token never appears in logs, error bodies, or `setup/status`, and it is covered by the log-grep redaction test like every other secret.

*Comparison:* `API_TOKEN` is break-glass **admin** — full authority, one secret, for a human or a script acting as one. `playout_token` is the opposite: no authority beyond reading streams, held by an appliance. They are separate secrets and must not be conflated.

### Explicit import & sync (admin-only, never implicit)
- `GET {LIBRARY_URL}/Users` with Loomarr's admin `library.token` lists server users; `GET /v1/users/candidates` (admin-only) surfaces that list to the UI with an `imported` flag per account, so picking is a choice from real names rather than pasted ids. `POST /v1/users/import` (admin-only) takes an explicit set of media-server user ids and upserts them as allowlisted rows (`password_hash` null; media-server admins may map to `admin` at the importing admin's choice, default `member`). This is the **only** way a media-server user gains access.
- `POST /v1/users/sync` (and a periodic sync, same pattern as the filler catalog) refreshes **already-imported** users: name + disabled state from the source. It **never adds** new users — sync reconciles the allowlist, import defines it. **This route is registered only when a media server is configured**, yet it is unconditionally present in the committed spec (which is generated in schema-only mode), so the generated client would otherwise offer a call that 404s. The computed `user_sync` feature (config-design §7) gates it, mirroring the same `library.flavor` condition the wiring uses. Users disabled/deleted server-side are disabled in Loomarr on next sync, and their sessions revoked. Local users are untouched by sync.

### Roles & quotas (v1: deliberately simple)
- **`admin`** — approve proposals/acquisitions, manage channels destructively, manage users (import/disable/role), settings, filler ingestion.
- **`member`** — browse everything, run suggestion jobs, **submit** proposals; approval routes to an admin. This gives §8's human-in-the-loop a concrete owner: *approve* is a permission, not a vibe.
- **Per-user quotas:** a **pending-acquisition cap** (0 ⇒ the `suggest.max_acquisitions` default) and an optional per-user `auto_approve` grant, bounded by that cap. Approvals are audited: `approved_by` is recorded on every approval and channel creation.

**What "pending" counts.** A title is keyed by identity (`movie:tmdb:603`), not by requester — two people wanting the same film is one row — so pending acquisitions are attributed through **proposals**, which carry `created_by`. A user's pending count is the deduplicated set of acquisition keys across their **approved** proposals whose titles have not yet reached a terminal state (`available`/`unavailable`). A title two users both asked for counts against both, which is the honest reading: each of them caused it.

**Where the cap binds.** On **auto-approve only**. An admin's manual approval is never blocked, because the admin *is* the approval gate (§7) — the quota exists to bound what happens when nobody is looking, and a human deciding is the thing it substitutes for. The UI still shows the count against the cap on the Users page, so an admin sees the cost before clicking.

**Auto-approve.** When a proposal is produced for a user holding the `auto_approve` grant, it is approved automatically **iff** its new acquisitions keep that user at or under their cap; otherwise it stays `submitted` for an admin, with the reason recorded. Approval — auto or manual — runs the identical code path, so the two can never diverge on what approving means (§19: "`auto_approve` respects quota").

### The required role is part of registering a route

**Every route declares the role it needs, and the middleware enforces it.** The role is an
argument at the registration site — not a sentence in a `Description`, and not a call a
handler body may forget to make.

⚠ **Anonymous is denied by DEFAULT.** A route with no declared role is refused, so the
failure mode of forgetting is a closed door rather than an open one. The genuinely
pre-authentication routes — login, logout, bootstrap, setup-state, version — say
`RolePublic` **explicitly**, which makes the public surface a list you can read in one
grep instead of an absence you have to infer.

- **`RoleAdmin`** — the destructive and resource-spending surface (approve, channels,
  users, settings, jobs, filler).
- **`RoleMember`** — reads, and the member actions §11 already grants above: run suggestion
  jobs, submit proposals. Being signed in is the whole requirement.
- **`RolePublic`** — the five routes that must work before a session can exist.

**This was found by probing, not by reading, and it had already gone wrong.** Enforcement
lived in `requireAdmin` at the top of a handler body — 56 call sites against 83 registered
operations. The other 27 consulted no role at all, so they were reachable with **no
credential of any kind**. Two consequences that were live in shipped code:

- ⚠ **`POST /v1/proposals` returned 200 to an anonymous caller and invoked the LLM.**
  Unauthenticated spend, with `created_by` stamped `""` because there was no user to
  attribute it to. §11's own text above says a member "runs suggestion jobs" — the model
  was right; nothing enforced it.
- ⚠ **`RoleMember` was declared, returned by the authorizer, and never checked anywhere.**
  The role existed as vocabulary only, which is why the gap survived: the tests could not
  express it. The default harness's token authorizer resolves to admin-or-anonymous, so
  member-vs-anonymous was **structurally untestable**, and a test asserting an anonymous
  read passed under the name `…VisibleToAnyAuthenticatedUser`.

**A test enumerates the registry** and asserts every operation declares a role, so a route
added without one fails the gate rather than shipping open (§19).

### Plain-mux routes carry the same guarantee

The routes that are not typed JSON — SSE, backup download, icons, playout, SSO redirects —
are mounted directly on the mux and **do not pass through the Huma middleware**. They
therefore authorize with a shared guard that fails closed, rather than each re-deriving the
rule inline.

⚠ **Two handwritten guards had already diverged on what `nil` means:** the backup handler
denied when the authorizer was absent, the events handler allowed. Same package, same
concern, opposite fail-safe defaults — which is what an unshared rule decays into. The
shared guard has one answer: no authorizer ⇒ denied.

### Bootstrap — first-run local admin
First run creates a **Loomarr-native admin** with a local username + bcrypt password, working with **zero media-server config**: `POST /v1/setup/bootstrap` (username, password) succeeds **exactly once** — only while `CountAdmins() == 0` — and creates the owning admin (`password_hash` set). It is unauthenticated *because* it is gated on "no admin exists yet"; the first success closes the door (a second call 409s), and the wizard (§13) drives it as its first step. `API_TOKEN` works throughout as break-glass, including to run bootstrap in automation. Media-server users are added afterward via explicit import — the bootstrap admin does the importing.

---

## 12. Web UI

Human control surface for the whole loop: browse/search, drive suggestions, approve, monitor channels and provisioning live.

### Stack & delivery
- **React 18 + TypeScript** (Vite SPA); **TanStack Router** (file-based, typed); **TanStack Query** for server state; **Tailwind CSS + shadcn/ui** (full stack rationale in §14).
- **Typed hooks generated by `orval` from committed `api/openapi.yaml`** — the payoff of §7.1: no hand-written types or fetch glue; contract changes become TypeScript compile errors.
- **Decided: embed** built assets in the Go binary (`embed.FS`), served at `/` → single self-contained container (§16), same-origin (no CORS). A separate SSR container is future work if ever needed.
- **Live updates** via SSE `/v1/events` (native `EventSource` hook).

### Views
- **Login** — local or imported-media-server credentials (§11); first-run flows into the **setup wizard** (§13): create the owning admin (bootstrap) → **choose who plays your channels** (`playout.backend`, internal by default — §9.1) → connection checklist → **connect Tunarr to your library** *(Tunarr path only:* `tunarr-connect` *wires + scans Tunarr's media source so channels get real programs not dead-air — §6; internal playout reads the library directly)* → import media-server users → guided first channel.
- **Guide** (route `/guide`) — headed **"Channels"**, this is the single channels surface: a cross-channel time grid answering both *"what do I have"* and *"what is on"*. **Origination** (how a channel is born) is a header action on this surface: the everyday door is **"✦ Add a channel" → describe it** (the §13 describe→review→approve flow, inlined below the header), and an empty install shows the **"Dead air"** state whose one action opens the same panel. **Evolution** (shaping a live channel) happens on the detail page (`/channels/{id}`) and never re-originates it.

  The **hand-made seeds** (single-series / empty via `POST /v1/channels`) have **no UI door and that is deliberate** — see the §12 surface-map row. They remain an API-only express door into the same object for scripted and restore use; the everyday way to make a channel is to describe one.

  ✅ **The Channels/Guide fold is DONE** (2026-07-26). It was blocked for several phases on one thing — the grid had no origination affordance, so removing the card list would have stranded the everyday way a channel gets made. The v2 mock settles it: its **Guide screen is headed "Channels"** and carries the `✦ Add a channel` button in its header, with the inline describe panel and the "Dead air" empty state beneath. The affordance moved to the grid; `/channels` and `/suggest` are now **redirects** to `/guide`, kept so existing bookmarks and deep links do not 404. `/channels/{id}` is untouched — evolution still lives there.

  The **channel detail page is four surfaces, organized by intent, with two audiences** — the everyday **Overview is the viewer surface (read-only); the other three are admin.** Every surface answers one question, so the page stops being a flat pile of tabs:
  - **Overview** — *"Is it on? What's playing? What's on later?"* Status (`OnAirIndicator`) + an **Upcoming guide strip** — the program airing now (highlighted) then the next few with their real Tunarr airtimes (`GET …/{id}/upcoming`, §6: Tunarr owns airtimes; commercial gaps filtered out). This is the schedule on the product's face, shown to every user. An admin-only **diagnostics** disclosure carries the relaxation-ladder report (§9), drift, and the Tunarr link — status, with one deliberate exception: the per-channel **playout backend** (§9.1) sits in its Broadcast section, because *who streams this channel* is the same subject as the Tunarr link below it and changing it is an infrastructure decision, not a content one. (The channel-icon editor lives in the **page header** beside the channel's name, not here — it is a setting, not read-only status.)
  - **Programming** (admin) — *"What plays, and when?"* One surface with a visible hierarchy: **what plays** (the lineup + scope: era, genres, audience ceiling + unrated, runtimeMax, and the *only these shows* series picker) → **how it's ordered** (ordering, separation) → **when it changes** (the wall-clock curation rules, `programming-design.md` §6.5, plus **seasonal**/holiday behaviour on the calendar clock). The `GET …/cycle` **cycle preview docks here** as the shared verification pane ("what airs Saturday 9am, and which rule wins"). **Refine-with-AI is a verb on this surface, not a separate place** — a header action opens the describe→review→apply loop (§8) acting on the *same* object the manual controls edit; the review shows a diff including **policy deltas** (so a refine can't silently change era/ceiling — §8.2 ownership).
  - **Filler** (admin) — *"What plays between shows?"* The per-channel selection (era/audience/category/kinds + pin/exclude) with a **live sandbox** — every change re-assembles the actual break against an unsaved draft (`POST …/pods/preview`, §7/§10) so you see exactly what airs before you **Apply**.
  - **Danger zone** (admin) — *"Stop or remove this channel."* Pause/resume and a typed-confirm **delete**. Deliberately narrow: a tab headed by an irreversible action is the wrong home for anything an operator edits routinely, so identity and growth settings live where they are used rather than being grouped here as "lifecycle".

  ⚠ **There is no Settings tab, and the auto-curate opt-in is not on one.** This bullet previously described a fifth *Settings* surface holding identity + lifecycle + auto-curate. It was never built — the tabs are `info | programming | filler | danger` (`SECTION_IDS`, `channels/$id.tsx`) — and describing it cost real work: the 2026-07-26 surface audit found `autoCurate`'s map row asserting a home ("Settings → lifecycle") that did not exist, so anyone checking "is auto-curate reachable?" read a row saying yes. Identity (name/number/group/**logo**) lives in the **page header**, next to the name it edits; auto-curate lives in **Programming → when it changes** (below), beside the curation rules it shares a clock with. Both are deliberate, not drift.

  **Two commit models, each cued:** most edits are **seamless** — no "rebuild now" button; an edit auto-reconciles in the background and the page updates live (the `channel` SSE frame). The **review-before-apply** surfaces say so explicitly: AI refine's diff, the filler sandbox, and **the Programming surface's scheduling rules** (below). A **surface map** (below) is the contract that every channel capability has exactly one home and audience.

  ⚠ **Why scheduling rules are the third review-before-apply surface, when everything else on the channel page saves inline.** Seamless is right when an edit is *self-contained* — renaming a channel, changing its era ceiling, adding a title. Scheduling rules are not: they resolve **first-match-by-priority**, so a rule's effect depends on every rule above it. Authoring one is therefore a multi-step edit — add the rule, set its WHEN/WHAT/HOW tokens, drag it to the right priority — and each intermediate state is a *different schedule*. Saving each keystroke would reconcile half-finished rule sets to Tunarr, so a viewer could tune in to a marathon that existed for four seconds because the operator had not yet dragged it below the kids block. The preview pane is what makes the draft usable rather than merely safe: `POST /v1/channels/{id}/programming/preview` (programming-design §8.1) resolves an unsaved `{lineup?, policy?}` through the **same** `ComputeDesiredAt` the reconciler runs, so "which rule wins at Saturday 9am" is answerable *before* anything ships.

  ⚠ **The scope of the draft is the rules, not the page.** Everything else on Programming — the lineup, era/ceiling, ordering, auto-curate — stays seamless, because each of those *is* self-contained. A whole-page draft would make renaming a channel require an Apply click, which is the ceremony §12 spent its first version removing. The blast radius of this exception is exactly the surface whose edits are interdependent.

  **Channel surface map** (the guardrail against capability-without-UI — a PR adding a channel capability updates this table):

  ⚠ **A row may say a capability has no door, and several do.** The table was previously written as though every row had a home, which made it worse than absent: `autoCurate`'s row asserted "Settings → lifecycle", a surface that was never built, so anyone checking "is auto-curate reachable?" found a row saying yes. A surface audit (2026-07-26) found **nine orphaned capabilities and three factually wrong rows**. The map is now allowed to record `none — ORPHANED`, because a guardrail that can only describe successes cannot report a failure.

  `ORPHANED` means: no UI path, and no decision that there should not be one. It is a defect awaiting a phase, not a documented API-only choice — those say **API-only by design** and give the reason.

  | Capability | API field / route | Home (surface) | Audience |
  | --- | --- | --- | --- |
  | name / number / group | `PATCH` `name`/`number`/`group` | Overview → header (inline rename/renumber); `group` API-only v1 | admin |
  | channel icon (`logo`) | `PATCH` `logo`; `GET …/{id}/icon-suggestions`, `POST …/{id}/icon` | Overview → Channel icon | viewer sees it; admin edits |
  | on-air status, now/next, upcoming guide | `GET …/now-next` (card), `GET …/{id}/upcoming` (Overview strip) | Overview | viewer |
  | cross-channel schedule (time grid) | `GET /v1/guide?from=&to=` | **Guide** (top-level, not a channel surface) | viewer |
  | relaxation ladder, drift, Tunarr link | `policy.applied`, status | Overview → diagnostics | admin |
  | lineup (add/remove/reorder, season windows) | `PATCH` `lineup` | Programming → What plays | admin |
  | scope: era, runtime cap | `policy.scope.era`, `.runtimeMax` | Programming → What plays | admin |
  | scope: series | `policy.scope.series` | Programming → What plays ("Only these shows") — a **search-backed picker**, because the field holds resolved `provision.Key`s, never names. Reuses the lineup editor's `keyOf` derivation and the shared `SearchCommand`; a movie or an id-less series is filtered out of the results rather than offered and then 422'd. | admin |
  | scope: genres, collections | `policy.scope.genres`, `.collections` | The two are in *different* states and an earlier row conflated them. `genres` **binds** but has no dedicated control: the scheduler's scope pass filters on it (`schedule/slotting.go`), it is simply only settable inside a curation rule. **`collections` is now reachable end to end** — Programming → What plays → *Only these collections*, a checkbox list over `GET /v1/library/collections`, enforced by the membership stamped at reconcile (programming-design §2.2). ⚠ **It was described as ORPHANED and "blocked on a missing endpoint" through three planning passes, and that framing was wrong**: the endpoint was the easy half. `filterEntries` is a pure no-I/O function over `[]LineupEntry`, so the load-bearing work was stamping membership onto the entry — a lookup inside the filter would have been an N+1 on the scheduling path. Recorded because the mis-framing survived being written down three times. | admin |
  | audience ceiling (safety) | `policy.audience.ceiling` | Programming → What plays | admin |
  | unrated allowance | `policy.audience.unrated` | Programming → What plays, beside the ceiling its default is derived from ("Automatic" names which way it currently resolves) | admin |
  | playback strategy | `PATCH` `strategy` | Programming → How it's ordered | admin |
  | ordering; separation (no-repeat, series gap, block cap) | `policy.ordering`, `policy.separation.*` | Programming → How it's ordered | admin |
  | wall-clock curation rules | `policy.rules` | Programming → When it changes | admin |
  | rolling-window horizon | `policy.window` | **none — ORPHANED** at channel level; a curation rule sets its own `window` | admin |
  | AI refine (diff → apply) | `POST …/refine` | Programming → header action | admin |
  | cycle preview (`?at=`) | `GET …/cycle` | Programming → verification pane | any authenticated |
  | per-channel filler + sandbox | `policy.filler`, `POST …/pods/preview` | Filler | admin |
  | auto-curate opt-in (+ overrides) | `policy.autoCurate` | Programming → When it changes (below the curation rules — the same question on a slower clock). The checkbox **constructs/deletes the object**, since the opt-in IS its presence; the two thresholds show blank for `0` (inherit), never a literal 0. Disabled with a reason on a hand-made channel — §8.2 skips a channel with no `IntentRef`. | admin |
  | pause / resume / delete | `PATCH` `status`, `DELETE` | Danger zone | admin |
  | seasonal mode / holidays / off-season | `policy.seasonal.*` | Programming → When it changes (below the curation rules — the calendar is the longest of the three clocks on that surface). The holiday list is read from the **rule vocabulary** (`when` tokens `holiday:<id>`), not hand-mirrored, so it cannot offer a holiday `builtinCalendar` does not know. Off-season fallback renders **only** in `exclusive` mode, the only mode that consults it. | admin |
  | playout backend (per channel) | `policy.playout.backend` | Overview → Advanced (admin) → Broadcast, beside the Tunarr link — the same subject, *who streams this channel*. §9.1's "one can be moved from its own page", now true. "Follow the default" lowers to `""` so the inherit shape survives. | admin |
  | draft programming preview | `POST …/{id}/programming/preview` | **none — ORPHANED**; the UI previews only saved state | admin |
  | forced reconcile | `POST …/{id}/reconcile` | **API-only by design** — §9: every edit auto-reconciles, there is no manual rebuild | admin |
  | origination (describe → approve) | approve (`POST /v1/proposals/{id}/approve`) | **Guide header** → `✦ Add a channel` (inline describe panel; the "Dead air" empty state opens the same one); the **approval queue** for the edit-before-approve path (drop/add/note ride the same call) | admin |
  | hand-made channel | `POST /v1/channels` | **none — API-ONLY BY DECISION** (C8, 2026-07-26). Not an orphan: §12 gives the UI exactly one origination door (describe → approve) and says so. The single-series/empty seeds stay for scripted and restore use, where a caller supplies every field deliberately. `strategy` is REQUIRED by this body, so it is unsettable at creation from a form — which is consistent with there being no form, and would need a §7 default before any UI could offer one. | admin |

- **Dashboard** (admin, route `/dashboard`, V16) — *"is everything alright?"* Four stat cards (on air · needs you · acquiring · filler), each a link to the surface it summarizes, over a **Transcoding** panel showing live internal-playout telemetry: one row per encoding channel with its viewer count, resolved encoder (hardware vs software — the difference between four concurrent streams and one), realtime speed and buffer-ahead, plus an `active / capacity` load line against `playout.max_channels`.

  Data comes from `GET /v1/playout/sessions` (admin-only), with the **`playout` SSE frame as the latency path**: the frame fires when a channel starts or stops, and the dashboard re-reads the endpoint. Deliberately not per progress sample — those arrive about once a second per stream, and republishing each would push several frames a second at every open browser for numbers that move by fractions. This is §8's standing rule applied: SSE is a latency optimization, the GET is truth on reconnect.

  Below it sit two more panels. **Services** (V31) lists every configured integration with the target it was probed against and a pass/fail state, **polled every 30s**, with a *"Fix →"* on a failing row that routes to the settings block owning it — a red dot that does not say where to go is a puzzle, not a diagnosis. It runs the same `runConnectionChecks` as the wizard checklist and `/v1/system/reload`; a test asserts the single implementation, because three surfaces disagreeing about whether Emby is reachable is worse than any one of them being wrong.

  ⚠ **Services POLLS and the feed does not, and that asymmetry is deliberate.** A probe result is not a state change anyone observes — the server only learns it by making six outbound calls (~730ms measured against a real stack). Pushing it over SSE would still need a server-side timer doing exactly that polling, but *continuously, whether or not any browser is open*; client polling stops when nobody is looking. It would also invert §8's rule: a pushed frame is safe to drop precisely because a GET can re-derive the truth, and here the probe **is** the truth — there is nothing to fall back to. The activity feed is the opposite shape (the server knows at write time, and `GET /v1/activity` is authoritative on reconnect), so it takes an `activity` frame and polls not at all. **Recent activity** (V32) is the persisted `activity` feed (§5) — *what Loomarr did*, newest first. ⚠ It reads the table, **not** the SSE bus: the bus is deliberately lossy, so a feed built on it would drop rows precisely when the system is busiest and most worth watching.

  **`running: false` is not the same as "nothing playing".** On a Tunarr-backed install the session list is legitimately empty, and a panel that cannot tell those apart renders a blank table that reads as every channel having just died — so the flag is explicit on the wire. A **member** who reaches the route sees a short explanation that this is machine state kept to admins, never a 403 wall.

- **Guide (time grid)** — the cross-channel schedule: every channel a row, time the shared horizontal axis, each programme a block whose width IS its duration. Answers the question the per-channel Upcoming strip cannot — *"what is on across all my channels right now?"* — and is the product's most recognisable surface, because it is what a TV guide has always looked like.

  Reads `GET /v1/guide?from=&to=` (§7), which returns per-channel timelines over a window with an explicit `kind` per block. **The `kind` discriminator is what makes the grid honest**: a commercial pod, a programme, and a still-acquiring slot are three different things, and the earlier `gap bool` could represent only "not a programme" — so a break and a pending acquisition rendered identically. Blocks are drawn:
  - **program** — the normal case; title, and for an episode the series name plus SxxExx.
  - **filler** — a commercial pod, visually quieter than a programme (it is not what the viewer tuned in for) but never invisible: an unexplained gap in a schedule reads as a bug.
  - **pending** — an acquisition still in flight, drawn at a **nominal width** and cued as an estimate (hatched/dimmed, not a solid block). Its times are a placeholder so the slot holds its position in the timeline; it is *not* a promise that something airs then, and the API marks it `nominal` for exactly this reason.
  - **flex** — dead-time padding, the quietest of all.

  **Gaps are preserved, never filtered.** Dropping a block would leave a hole that every later block slides into, so the timeline would stop matching the clock — the one thing a guide must never do. (`GET …/{id}/upcoming` *does* filter gaps; that is right for a "what's on next" strip and wrong here.)

  A **now-line** marks the current instant and advances client-side from the block timestamps rather than by polling — the browser already knows the wall clock, so pushing "time passed" over the wire would be pure waste. **Lineup changes** are the genuine invalidation, and those arrive on the existing `channel` SSE frame (§7 live updates): the grid refetches its window when a channel reconciles. **Zoom and span are two separate controls** (revised after V14a): zoom scales the *chrome* — rail width, row height, type — while the window always fits the viewport, which is the TV-guide convention (you change how much detail each row shows, not how much time is on screen). The **time span** is chosen independently (2H/4H/6H/day) by asking the API for a different window. Scrolling backwards is bounded (the past is recomputed from the *current* lineup, so a distant past would be fiction rather than history).

- **Queue** (route `/queue`) — **three tabs, per the v2 mock: `Needs approval · In flight · History`** (V27). *Needs approval* is the admin approval gate (§7) with per-row review/edit and **bulk approve**; *In flight* is the tracked titles below; *History* is the decided proposals — each carrying **when** it was approved (`approvedAt`), who by, what they changed, and any deny reason. This is what "the approvals queue as its own surface" means: **Queue is that surface**, which is why the mock hangs a pending-proposal count badge off this nav entry and not a separate one. Tab counts come from the same queries that fill the tabs, so a count cannot disagree with its list. Tracked titles by provisioning state keep their **retry** on a stalled acquisition, and each title's journey (*pending approval → acquiring (3/7) → live on channel N*). Named for what it holds — work waiting on someone — rather than "Board", which named a layout. Per §342's global-read model this list is **not per-member**: every authenticated user sees every tracked title, because `TitleDTO` carries no requester and the store has no per-user index to scope by. "My requests" as a heading would therefore be a promise the data cannot keep — scoping it is a schema change (a requester column + a filtered list route), not a UI filter.

  **The member-facing "My requests" surface therefore lists PROPOSALS, not titles** (§12, V26). The two halves of a request scope differently and it is worth being precise about why: a `Proposal` carries `created_by`, so *"the requests I submitted"* is answerable; a `Title` does not, so *"the titles acquired for me"* is not. The surface shows each proposal's journey (submitted → approved/denied), the approver's **note** and server-generated **modification summary** when it came back altered, and the **deny reason** when it did not — the provenance V25/V25b persist. Tracked titles remain the global list above. **Cancel** is likewise unbuilt: giving up on a title is `DELETE /v1/titles/{key}`, which is admin-only by the same §342 rule that makes enqueuing admin-only, so it is not a member-facing control on this route.
- **Suggestion workspace** — enter intent (or start from a **template**, §13) → watch generation → review lineup + acquisitions w/ rationale + scores → **edit via search** (§7.2: add/replace titles; missing ones become acquisitions) → **submit**; admins get an **approval queue** and approve/deny with `approved_by` recorded. **The queue is also where a proposal is edited before approval** (§7.2 / D-K): "Review & edit picks" opens the same pick list with a drop control per title, an add-via-search box, and a note to the requester. The edit is a **delta** (`drop` by provisioning key, `add`, `note`) sent **on the approve call itself** — never a separate save — because `suggest.Approve` takes it as a parameter, keeping "what gets acquired" inside the one gate. Approving unmodified sends an empty body and behaves exactly as it did before the feature existed. Inline intent-writing hints. The same describe→review→approve machinery is reused **in a refine mode on an existing channel** (§7 `refine`): the intent is seeded from the channel's current lineup + a free-text change, and review shows a diff instead of a fresh lineup.
- **Filler library** — browse/tag commercial clips (era/audience/category), trigger sync, review AI tags, preview a channel's pods (§10). This is the **catalog**; each channel *chooses* from it on its own Filler section (§10 per-channel selection). The two surfaces cross-link both ways — the catalog heading points to per-channel selection, a channel's Filler section links back to the catalog, and a clip offers a **"Use in a channel"** action that pins it into a channel's filler directly (a normal `PATCH …/{id}` of `policy.filler.pinned`, merged onto the channel's live policy).

  **Three tabs — `Catalog · Incoming · Sources` — under a persistent pool-health strip** (V35). The strip is *not* a tab: catalog health is the context you read the other three in, and burying it behind one made it the thing nobody clicked. It carries the honest headline numbers plus the page's primary action, **Propose a pull**.

  - **Catalog** — the clips themselves, in a grid or a dense list. Multi-select drives bulk retagging and removal; a card carries its thumbnail, duration, quality, tags, and **how often it has actually aired**.
  - **Incoming** — the conveyor: what has been downloaded but is not yet filed. Clips whose tags need a human (with the guess, its confidence, and *why*), and compilations mid-split. The auto-file policy lives here too, beside the queue it governs, rather than in Settings — the number that decides how much lands in this queue belongs next to the queue.
  - **Sources** — where filler comes from, each switchable off, each searchable in place.

  ⚠ **There is no Discover tab.** Finding clips used to be its own destination; it is now something you do *to a source*, which is the only place the answer differs. ⚠ **Incoming does not replace the split-review route** — `/filler/splits/{proposalId}` remains a **sibling** of `/filler`, because the catalog page renders no `<Outlet/>` and nesting it would make the whole surface unreachable while every unit test stayed green (PROGRESS.md records the near-miss). The tab is an additional door.

  ⚠ **The card's per-channel control is an include-set override, not two flags.** It replaces the pin/block pair: channels are checkboxes with a fit note, and **Back to automatic** returns the clip to being placed by the ladder. Pin-and-block let an operator build a state that reads as contradictory ("pinned *and* blocked") which the assembler had to resolve by rule; one set has no such state.
- **People** (admin, route `/people`) — imported users, roles, quotas, disable, sync-now (§11). "People" rather than "Users" because the list is households and family members, not system accounts.
- **Settings** (admin, route `/settings`) — **six tabs** (V9, restructured to the v2 mock): *Connections* (media server, requester, Tunarr, TMDB, plus `/readyz` and the re-runnable **connection checklist** of §13 as the troubleshooting console) · *AI* (provider/model, including the in-app **model manager** of §8.1 — probe, catalog, hot-swap, streaming pull) · *Defaults* (what a new channel inherits, plus filler behaviour) · *System* — itself sub-tabbed into **Tasks** (the §18.1 job console: cron, last/next run, Run-now) · **Playout** · **Database** · **Backup** · **About** — · *Security* (incl. **secret regeneration**) · *All settings* (every key, searchable). The typed registry, `env > database > default` resolution, hot-apply, the cross-tab save bar, and the secrets lifecycle are `config-design.md`'s domain — **it wins on those mechanics** (§5 carries the page table and the four inline-commit exceptions); this row records only *where the surfaces are*.
- **Account** (route `/account`, any authenticated user) — the signed-in user's own credentials: change password and view/revoke active sessions (§11). Distinct from **People**, which is an admin managing *other* accounts; this is the one settings-shaped surface a member can reach, which is why it sits outside the admin-only `/settings` IA above.
- **Global search (⌘K)** — command palette over `/v1/search` + channels + help; the single fast entry point. **Hand-rolled, not cmdk** (revised — §12 described shadcn `Command`/cmdk, which was never built and is not a dependency; adding one is a §14 conversation, and the ARIA pattern is small enough not to need it). It implements the combobox/listbox pattern directly: `role="combobox"` on the input, `role="listbox"` over the results, one `role="group"` per scope, and `aria-activedescendant` — so focus stays in the input while ↑/↓/Home/End move the active option and Enter selects it. `Escape` is bound once at the window level (`useCommandShortcut`), never inside the component, so it cannot close twice. The search call deliberately omits `scope`, which the API defaults to `all` — the right corpus for a palette; channels, clips, and help are merged in from their own sources (clips are not a `/v1/search` scope, §7.2).

### Navigation

**Two distinct navs, not one list filtered by role.** A member is not an admin with items missing — they arrive to watch and to ask for things, and a rail of greyed-out entries advertises a product they cannot use. So the two lists are authored separately:

| | Items |
| --- | --- |
| **admin** | **Dashboard** · Channels · Guide · Queue · Suggest · Filler · People · Settings · Help |
| **member** | Guide · Request a channel · My requests · Help |

The member's `Request a channel` and `My requests` are the same suggestion and queue surfaces the admin sees, named for what a member is actually doing with them.

**`Dashboard` leads the admin nav (V16),** matching the v2 mock. It was deferred on the grounds that "a nav entry to a placeholder is worse than no entry" — V16 removes that objection by building the surface, so the entry is now appropriate rather than aspirational. **Members do not get it**: its content is machine state (encoder families, realtime speed, how close the box is to its channel ceiling), which §11 keeps to admins.

✅ **The admin nav now matches the v2 mock** (2026-07-26): **`Dashboard · Guide · Queue · Filler ·
People · Settings · Help`** — seven entries, exactly `navDefs` in
`design/loomarr-prototype-desktop-v2.dc.html`. Both `Channels` and `Suggest` are gone from the nav;
their routes redirect to `/guide`. Read the mock before touching the nav — it remains the authority
on this surface. Three consequences worth recording rather than rediscovering:

1. **What unblocked it was reading the mock's Guide screen, not a new decision.** Both deferrals
   cited the same blocker — "the grid has no origination affordance yet" — and the mock has always
   carried one: its Guide screen is headed **"Channels"** with `✦ Add a channel` in the header. The
   affordance was specified all along; nothing had ported it. `Dashboard` was the other half and
   shipped as V16.
2. **`Suggest` disappearing was the harder one**, and it is NOT merely a rename: it is §12's
   origination path (`describe → review → approve`). It resolves because the Guide header now hosts
   that exact path inline — admin origination has a decided home, so the standalone entry is
   redundant rather than lost. **The member nav is unaffected** and still lists it as
   *Request a channel* (members have no Guide-header affordance; four entries, unchanged).
3. The mock hangs a **pending-proposal count badge** off `Queue`, which is why V27 built the
   approvals surface into Queue's tabs rather than as a ninth nav entry.

- **Help** — embedded docs (§13), rendered offline; searched client-side.

### Auth
Sessions and roles per §11; the UI hides admin-only actions from members and surfaces the approval queue to admins. Do not expose the app publicly.

---

## 13. Onboarding & documentation

Two personas with different problems. The **operator** faces an integration problem — five external services must be wired correctly before anything works. The **member** faces a blank-page problem — an NL intent box with no guidance produces bad intents and confusion about what happens after submit. Both get first-class treatment.

### Design decision: settings live in the app; the environment pins
**(Supersedes the earlier "wizard validates, does not store" rule.)** Loomarr follows the *arr-ecosystem convention — connections and integration settings (media server URL + token, Seerr, Tunarr, TMDB, LLM) are configured **in the app**, like Sonarr and Seerr — because fix-in-place beats "edit compose, restart, return." The dual-source-of-truth wound is prevented not by banning a source but by **deterministic precedence with visible provenance**: every setting resolves `env > database > default`, per key, through one typed settings registry (§15). An env var that is set **wins and locks its UI field** with a "set via environment" chip; unset, the field is editable and persisted. GitOps/compose-template users pin what they want and lose nothing. Full mechanics: `config-design.md`.

Consequences, embraced:
- **Generated secrets:** `SESSION_SECRET` and `API_TOKEN` are auto-generated at first migration (like the instance id), viewable/regenerable in Settings — the Sonarr API-key model. Env override remains possible, never required.
- **Zero-required-env first run:** with `DATABASE_URL` defaulting to the SQLite volume path, `docker run -v loomarr-data:/data loomarr` boots straight into the wizard.
- **The wizard becomes configure → validate → save:** each step is a real settings form with a live test; the checklist still re-runs from Settings for the life of the install.
- **The LLM provider/model is configured in-app** (§8.1): the AI settings step probes the host, lists a fit-ranked model catalog, tests a hosted key, and hot-swaps — the persisted `llm.*` settings override their env defaults like any other key.
- **Secrets in the DB** follow ecosystem practice (Sonarr's config, Seerr's settings) with hard redaction rules: masked after save (replace-only), never logged, excluded from `/v1/setup/status` responses — and **backups are secret material** (§16).

### Operator first-run (wizard)
**Entry:** every route the operator can land on — `/` above all — sends an **unbootstrapped** install to the wizard, decided from the unauthenticated `GET /v1/setup/state` (§7). Without that signal the guard has only the session to go on, so a fresh install bounces the owner to `/login` and strands them there: no account exists, and nothing on the page says the install is unclaimed. §16's install steps say "open the UI at `/` and create the owning admin", and this is what makes that true.

On a fresh instance the UI then walks the owner through, in order:
1. **Bootstrap** — create the owning admin (local username + password, `POST /v1/setup/bootstrap`, §11). Works with zero media-server config; succeeds once (while no admin exists), then this step is done forever. **Once done it renders read-only**, naming the owner: the step cannot run twice, so offering the form again is a dead end an operator can only discover by submitting it.
2. **Playout** — *"How should Loomarr play your channels?"* Two choices, writing `playout.backend` (§9.1): **Loomarr** (default) or **Tunarr**. See "The playout choice shapes the wizard" below — this answer decides which of the remaining steps exist at all. ⚠ **Choosing Tunarr reveals Tunarr's own connection form on this same step**, rather than sending the operator to Connections for it: "which one plays my channels" and "where is it" are one decision, and splitting them across two screens made it feel like two. The form is the ordinary `ConnectionBlock` + settings-group fields with its live Test, so it writes through the same PATCH path as everything else (config-design §6).
3. **Connection checklist** — live-tests each dependency and shows pass/fail with a fix hint and a deep link into the relevant docs page: media server reachable + `library.token` valid; filler library found (if configured); Seerr reachable + key valid; LLM reachable + model present **and supports tool-calling** (Ollama: query the model's capabilities — a non-tools model fails grounding silently otherwise); TMDB key valid. **Tunarr is never a block on this step** — it is configured on the Playout step above — but on the Tunarr path its check still **gates** here: being configured elsewhere does not stop it being required (**Tunarr reachable *and* with a media source matching `library.url`**, queryable via Tunarr's API, which verifies §6's "Important" invariant instead of just documenting it).
4. **Give Tunarr your library** (**Tunarr path only**) — one-click wiring + scan of Tunarr's Emby/Jellyfin source (`POST /v1/setup/tunarr-connect`, §6/§7), so channels get real programs rather than dead air. Internal playout reads the library directly and needs no equivalent, so the step does not exist there.
5. **Import media-server users** (optional) — the admin picks which Emby/Jellyfin accounts get in (`POST /v1/users/import`, §11); only imported users can sign in. Skippable for a solo install (the bootstrap admin is enough).
6. **Guided first channel** — offer a template intent (below); since the owner is an admin, they can self-approve and watch the full pipeline run end to end.

#### The playout choice shapes the wizard

**§9.1 made internal playout the default, and the wizard had not caught up: it hardcoded `tunarr` as a required check and gave Tunarr a dedicated wiring step.** An operator on the default path was therefore asked to install, connect and wire a second server they would never use — and *blocked* from continuing until they did. That is not a copy problem; the shortest honest path to a live channel no longer runs through Tunarr, so a fixed step list cannot describe it.

**The step list and the blocking set are therefore DERIVED from `playout.backend`, not constant:**

| | Internal (default) | Tunarr |
| --- | --- | --- |
| Blocking checks | `media_server` | `media_server` + `tunarr` |
| Tunarr connection form | absent | **on the Playout step**, never in Connections |
| "Give Tunarr your library" step | absent | present |

**Steps the choice removes are hidden outright, not shown as satisfied.** A rail entry that reads "not needed" still advertises work; on the internal path the Tunarr wiring is not deferred or optional, it is *not part of this install*. The rail shows only real remaining work, which is the same reasoning that keeps `livetv` from having a step of its own.

⚠ **Naming the choice is what makes it a choice.** "Tunarr" appears nowhere in the wizard before this step. Listing it earlier as a peer of Media server / Requester / TMDB presented a decision as a dependency, which is precisely how the old flow read.

**Tunarr stays first-class, not legacy** (§9.1): the choice is a fork, not a migration path, and it is re-decidable per channel afterwards. The wizard writes the *instance default*.

The checklist is backed by `GET /v1/setup/status` (runs all checks, returns structured results) and is **re-runnable from Settings** — the same panel doubles as the troubleshooting console for the life of the install.

**No wiring step may hold the wizard hostage.** The blocking set is the shortest honest path to a live channel (config-design §6), which since §9.1 **depends on the playout backend**: `media_server` alone on the default internal path, `media_server` + `tunarr` when Tunarr is doing the streaming. (It was hardcoded to the latter, which blocked every internal install behind a service it would never use — see "The playout choice shapes the wizard" above.) The wiring steps that exist on a given path (guide, webhooks, library) are all **skippable**, and skipping records a deliberate `skipped`, not an unfinished step. This is not a convenience: an install that doesn't use its media server's Live TV, or has no *arr apps, can never turn those checks green, and a step that blocks on a check the operator cannot satisfy is a dead end with no way around it — the wizard offers only Back/Continue, so a hard gate strands them on that screen forever. It bit hardest on the *library* step, whose whole purpose (§6) is to prevent channels scheduling slots with no program: gating it behind Live TV meant an operator whose guide wiring failed could never reach the guard against dead air.

**Both one-click wiring actions also live in Settings → Connections** (config-design §5), so they are available for the life of the install rather than only during first-run — the same principle that already makes the checklist re-runnable there.

### Member first-run
- **First login intro** — one screen with the mental model: *intent → proposal → submit → admin approves → titles are acquired → your channel appears in the TV guide.* Sets the expectation that channels may start filler-heavy and improve as content lands (§9's backfill).
- **Channel templates** — the blank-page killer: a set of one-click starter intents ("90s Saturday Morning Cartoons," "Cozy Mystery Nights," "Late-Night Sci-Fi," "Action Movie Marathon") that prefill the suggestion workspace with a good intent + sensible constraints. Templates ship as embedded JSON in the FE bundle; users edit before running.
- **Intent-writing hints** — inline examples in the workspace of constraints that work well (era, tone, runtime target, must-include/exclude).
- **"My proposals" status** — members always see where their submission is: *pending approval → approved → acquiring (3/7 titles) → live on channel 42.* This is the member-facing framing of the Board + channel status.
- In-app status only for v1; notification agents (email/Discord on approval or channel-live) are future work (§20).

### Documentation set
Docs live as markdown in `docs/` in the repo and are **embedded and rendered as an in-app Help section** (same `embed.FS` mechanism as the SPA and `/docs` — works air-gapped, consistent with §7.1's offline rule). A public MkDocs site can be generated from the same files later.

**Mechanics.** The user-facing pages live in **`docs/help/`** and are embedded by `docs/embed.go`. Only that subdirectory ships: the design docs sit beside it and are internal, so embedding `docs/` wholesale would put the project's own architecture notes and open questions in front of every operator. (A Go file lives in `docs/` because `//go:embed` cannot reference paths outside its own package directory — moving the pages under `internal/` would contradict this section's "docs live in `docs/`".) `GET /v1/docs` lists them; `GET /v1/docs/{slug}` returns raw markdown, since the frontend both renders and *searches* it client-side (§7.2).

**The `docHref` anchors are a contract.** Every setup check carries a deep-link like `troubleshooting#tunarr-library`, and §13's promise that "every red check deep-links to its section" is only true if the target exists. A dangling deep-link is worse than none — it promises help and delivers a blank page at the exact moment the operator is already stuck. A test asserts every anchor the API emits resolves to a real heading in an embedded page, so renaming a heading fails the build rather than silently breaking a link.
- **Quickstart** — compose up → wizard → first channel (the 10-minute path).
- **Integrations** — one page per dependency (media server, Tunarr, Seerr, Sonarr/Radarr webhooks, LLM: Ollama or a hosted OpenAI-compatible provider, TMDB) with exact setup steps.
- **Concepts** — the mental model: proposals, approval, provisioning states, backfill, pods, and the **programming heuristics** extract/enforce principle (`programming-design.md` §1). (Aimed at both personas.)
- **Member guide** — writing good intents; what happens after submit; reading channel status.
- **Programming guide** — the ChannelPolicy: scope/audience/separation/ordering/seasonal, and how the relaxation ladder keeps a channel filled (`programming-design.md`).
- **Filler guide** — drop-folder, MeTube, the in-core ingest job, tagging, pod policy.
- **Troubleshooting** — organized by checklist item: every red check in the wizard deep-links to its section here. The checklist is executable documentation; this page is its narrative twin.

**Companion design docs** (incorporated in Phase 14; authoritative for their own domains): `programming-design.md` (ChannelPolicy heuristics — §8/§9), `config-design.md` (settings registry mechanics — §13/§15), and `frontend-design.md` (the "Test Card" design system — §12/§14).

---

## 14. Technology stack (decided)

Every "pick one" in this doc is now picked. The agent builds with this stack; deviations require a doc update first.

### Backend (Go 1.22+)
| Concern | Decision | Why |
| --- | --- | --- |
| HTTP router | **stdlib `net/http` ServeMux** (Go 1.22 method+path patterns) via Huma's `humago` adapter | No third-party router; the embedded same-origin SPA also means **no CORS layer at all** |
| API framework | **Huma v2** (code-first OpenAPI 3.1 + validation + docs UI) | §7.1's single-source-of-truth requirement; `oapi-codegen`/`swaggo` rejected (spec-first ceremony / weakest drift guarantee) |
| Config | `caarlos0/env` (struct tags) for the bootstrap/env layer, feeding one **typed settings registry** (`env > database > default`, hot-apply, `config-design.md`) | Boring, maintained; the registry is the single source of truth (§15) |
| DB access | **`database/sql` for both backends** — `modernc.org/sqlite` + `pgx` via its stdlib shim | One store code path; dialect differences live only in migrations + `ClaimDue*` |
| Migrations | **`goose`** with `embed.FS`, per-dialect dirs | Simple embedded-FS story; golang-migrate rejected as heavier for no gain here |
| Jobs | **hand-rolled jobs table in the Store** + in-process worker | Forced, not preferred: River is Postgres-only, Asynq needs Redis — both break the SQLite promise. Claiming reuses the `SKIP LOCKED` pattern |
| Scheduled-job cron | **`github.com/adhocore/gronx`** (parse + next-tick) | The job scheduler (§18.1) exposes Sonarr/Overseerr-style **cron** schedules (6-field, seconds-leading). Correct cron next-time (DST, ranges, `*/n`, day-of-week vs day-of-month) is fiddly to hand-roll; gronx is a **pure-Go, zero-transitive-dependency** parser/next-tick lib — the minimal add for correctness. Used only to validate a job's cron setting and compute its next run. |
| Background job engine | **`github.com/riverqueue/river`** + **`riverdriver/riversqlite`** (v0.41.x) | Replaces the hand-rolled leased scheduler (§18.1) with a real queue: durable job records, retries with backoff, and a run history the Tasks page can show instead of one `last_error` string. ⚠ **SQLite support is officially EXPERIMENTAL** and its schema "may still have a few tweaks" — accepted deliberately (maintainer's call, 2026-07-30) with that risk stated rather than discovered. Uses `modernc.org/sqlite`, already this repo's driver and exactly what River tests against, so no CGO and no driver change. ⚠ **Its schema is applied programmatically via `rivermigrate`, NEVER the `river migrate-up` CLI** — a second migration *system* an operator must run alongside goose is the thing that would make this unshippable. **The honest dependency cost:** 5 direct modules (`river`, `riverdriver`, `riverdriver/riversqlite`, `riverdriver/riverdatabasesql`, `robfig/cron/v3`) plus 5 indirect (`rivershared`, `rivertype`, `lib/pq`, `tidwall/{gjson,sjson}` and their two helpers) — against a hand-rolled scheduler of ~350 lines that worked. Recorded rather than glossed: this is the largest single dependency addition in the project, and §18.1's cron trap is a direct consequence of `robfig/cron` arriving with it. |
| Sessions | hand-rolled in the Store (random 256-bit token, **SHA-256-hashed at rest**, HttpOnly cookie) | We need revocation-by-user + dual-backend anyway; `scs`/`gorilla` add a dependency for no gain |
| Local passwords | `golang.org/x/crypto/bcrypt` (DefaultCost) | Local-admin bootstrap + local users (§11 identity rework) need a password hash at rest. bcrypt is the boring, correct choice; already in the module tree transitively — this promotes it to a direct dependency. Session *tokens* stay SHA-256 (fast, high-entropy); only human passwords use bcrypt. |
| Rate limiting | `golang.org/x/time/rate`, per-IP+username, in-memory | Login only; per-instance is acceptable v1 |
| Metrics / logs | `prometheus/client_golang` / `slog` | Standard |
| OIDC (SSO) | **`github.com/coreos/go-oidc/v3`** (+ `golang.org/x/oauth2`, `github.com/go-jose/go-jose/v4`) | SSO is a third credential path (§11, V8), and OIDC means verifying a signed token against the issuer's published JWKS — discovery, key rotation, `nonce`/`aud`/`exp` validation. Hand-rolling JWT verification is the kind of security code that looks right and is not. **Three modules total**, all current and maintained; `go-jose` does the crypto and `x/oauth2` the code exchange. Deliberately chosen over building forward-auth instead, which needs no dependency but trusts network topology (§11). |
| Goroutine-leak gate | **`go.uber.org/goleak`** (test-only) | The in-process restart loop (§9.2) is only correct if Build/Run/Shutdown can repeat without accumulating goroutines or stale state, and a leak there is **silent** — it degrades an install over successive restarts rather than failing anything. goleak is the standard detector, test-only (never in a shipped binary), zero runtime cost. Added by V13 alongside the N-iteration restart test, because a prose rule would not have caught it. |
| LLM clients | **Ollama via plain HTTP** (`/api/chat` with tools) + a hand-written **OpenAI-compatible** client (`/v1/chat/completions` with tools) — both plain `net/http`, no SDK | One OpenAI-compat client covers OpenAI, Gemini (compat endpoint), Groq, Together, OpenRouter, **and** local Ollama's own `/v1` mode — so the model is a config choice, not a per-vendor code fork. Replaces the earlier `anthropics/anthropic-sdk-go` intent (a net dependency *reduction*); Claude is still reachable via OpenRouter. Ollama stays first-class as the local default. |
| TMDB / Seerr / media server / Tunarr | **plain HTTP, hand-written thin clients** | Each uses a handful of endpoints; generating from Tunarr's full pre-1.0 spec couples us to its churn. Pin + record versions tested against |
| Model discovery source | **Hugging Face model API** (`huggingface.co/api/models`), plain HTTP via the existing factory | The **only** live source of *downloadable* Ollama models — Ollama ships no such API (`/api/search` unshipped; ollama.com is HTML-only). Anonymous GET, **no new Go dependency** (one `net/http` call), and `ollama pull hf.co/<repo>` consumes its ids directly (§8.1). Best-effort: an outage degrades to a "browse on huggingface.co" link, never a page failure. A single read-only outbound endpoint, pinned via a captured fixture like the others |
| Backend tests | stdlib `testing` + `testcontainers-go` (Postgres) | Already specified |

### Frontend (Node 20+, Vite + React 18 + TypeScript)
| Concern | Decision | Why |
| --- | --- | --- |
| Server state + API client | **TanStack Query** with hooks **generated by `orval`** from `api/openapi.yaml` | One generator yields both types and query/mutation hooks; `openapi-typescript`+`openapi-fetch` rejected only because orval removes more hand-written glue |
| Routing | **`@tanstack/react-router`** (file-based; `@tanstack/router-plugin` + `-cli` generate `routeTree.gen.ts`) | End-to-end type-safe routing (typed params/search/links) matching the orval-contract ethos; shares the TanStack Query client via router `context` + loader-based auth guards (`beforeLoad` → `redirect`, no guard-flash). Web-only — routing was always the per-platform seam (frontend-build-plan §), mobile keeps Expo Router; `react-router` v6 replaced 13.3a |
| Styling / components | **Tailwind CSS + shadcn/ui** (Radix primitives: `@radix-ui/react-slot`, `@radix-ui/react-select`, `@radix-ui/react-tooltip`) | Fast, decent defaults, copy-in components. Radix primitives are the runtime piece shadcn wraps. **`@radix-ui/react-select`** added for the enum control: the native `<select>` first shipped (accessible, mobile-correct, zero-dep) renders an **unstyleable OS option list** (light popup on some platforms, off-theme), and richer selects (search, groups, icons) are planned that native can't do — so standardize on Radix Select now. Supersedes the earlier native-only choice recorded in `select.tsx`. **`@radix-ui/react-tooltip`** added for icon-only-button labels: the app has many icon-only affordances (sidebar search/sign-out, the channel-detail back arrow, row actions) whose meaning needs a hover/focus label; the native `title=` attribute is unstyled, ~1s-delayed, and keyboard/touch-hostile, so a themed Radix tooltip (matching the Select precedent) is the accessible, on-brand choice. **`@radix-ui/react-slider`** added for the video scrubber (V39): a seek bar is a slider, and the hand-rolled `role="slider"` it replaced had to re-implement the WAI-ARIA keyboard contract (arrow steps, Home/End, `aria-valuetext`) by hand — semantics that rot silently, because nothing fails when they drift. Radix owns them, plus drag, pointer capture and RTL. Same family as Select/Dialog/Tooltip, so no new vendor. |
| Drag-and-drop (lineup reorder) | **`@dnd-kit`** (`@dnd-kit/core` + `@dnd-kit/sortable` + `@dnd-kit/utilities`) | Reordering a channel's lineup (§7 PATCH, §12) is a sortable list. `@dnd-kit` is the current-gen, React-18/StrictMode-safe choice (`react-beautiful-dnd` is archived); it is headless (~10kb core, no runtime deps of its own, CSP-safe for the embedded assets) and ships **keyboard + screen-reader reordering** built in (arrow-key sort + live-region announcements), which is the accessibility cost that would otherwise make drag worse than up/down buttons. The reorder still commits through the same `PATCH /v1/channels/{id}` whole-list replace — DnD is presentation only. |
| Guide row virtualization | **`@tanstack/react-virtual`** | The Guide (§12) renders one row per channel and one absolutely-positioned block per airing, so its DOM grows with **channels × airings-in-window** — a 12-hour window over 50 channels is thousands of nodes, and every zoom change re-lays-out all of them. `react-virtual` is headless (no DOM of its own, no runtime deps), ~4kb, and already the TanStack family this app standardizes on. It windows **rows only**: blocks are already clipped to the visible time span in the render loop, which is the horizontal half of the same idea. **Honest scope:** at the maintainer's current 4 channels this changes nothing measurable (147 grid nodes, no vertical scroll) — it is added for the shape of the surface, not for a measured problem today, and the row-count threshold where it starts paying is ~100+. |
| Live updates | native `EventSource` wrapped in a small hook | SSE, cookie-authed same-origin |
| Forms | **`@tanstack/react-form`** | Consumes the `packages/core` zod schemas **directly via Standard Schema** (zod ≥3.24) — no resolver adapter, so `react-hook-form` + `@hookform/resolvers` collapse to one dep; field names/values infer from `defaultValues` (the same end-to-end typing as orval DTOs + typed router links); `@tanstack/form-core` is framework-agnostic so mobile shares form logic like it already shares the schemas; consistent with TanStack Query + Router. Replaced `react-hook-form` — its stated justification was shadcn's RHF `<Form>` wrapper, which Loomarr never adopted (forms hand-compose `Label`+`Input`). Used by Login and the wizard's Bootstrap step. *(`SettingsGroupForm` was removed — Settings writes through `SettingsPage`'s one-save-bar-per-page path instead, config-design §5.)* |
| Help rendering | `react-markdown` + `remark-gfm` over the embedded `docs/` markdown | Offline, consistent with §7.1 |
| Component workshop + gallery | **Storybook 10** (`@storybook/react-vite`) + `@storybook/addon-a11y` (axe, in the workshop) | The component gallery/contract *and* dev workshop (frontend-design §5); carries to the future mobile app via `@storybook/react-native` (Expo, on-device). Replaces the hand-rolled `/__gallery` registry. The CI gate (visual + a11y) is **one Playwright pass** over the offline `storybook-static` build. **Chromatic rejected** — hosted SaaS visual-diff, breaks the offline/self-hosted rule (§16) |
| FE tests | Vitest + Testing Library (jsdom units) + a story-coverage test; **Playwright** over `storybook-static` for the visual suite (`toHaveScreenshot`) **and** a11y (`@axe-core/playwright`), plus the e2e approve-flow smoke | Matches §19 |

### Ingest tooling & CI
- **Ingest is core Go code** (`internal/clipfetch` — named so it is never confused with `internal/ingest`, the Sonarr/Radarr *webhook* handler of §6), shelling out to **`yt-dlp`** + **`ffmpeg`** (CLI) for YouTube/playlists and plain `net/http` for Archive.org; it writes files + info-JSON sidecars into the drop-folder. Deliberately dumb. Those binaries — plus **`deno`** (modern yt-dlp requires it for YouTube extraction), **`ffprobe`**, and **`whisper-cli`** (below) — are the **only** vendored non-Go executables the project allows, and they are invoked via `exec`, never linked. **They ship in the single image** (§16); there is no variant that omits them, so the `ingest` feature is always available.
  - **`ffmpeg` is a core runtime dependency, not an ingest-only tool** (revised — §9.1). It serves two callers now: yt-dlp's stream merging, and **internal playout's encoder**. A Loomarr that can't encode can't play out, so the previous opt-in-variant model (below) no longer describes a coherent artifact.
  - **`ffprobe` is bundled** (revised — it was previously excluded to save ~99MB, on the grounds that *"Loomarr never probes media — Tunarr assigns duration during its `local`-source scan"*). Internal playout owns duration and cut points, so the premise is gone. Both reversals trace to the same root cause: §9.1.
  - **`whisper-cli` (whisper.cpp) transcribes filler audio for compilation splitting** (§10, V34 — a maintainer-approved §14 addition, 2026-07-31). The transcript is the only signal that sees an ad boundary with no black frame and no silence: measured, one 149s block defeated every A/V detector while holding three complete adverts whose cuts exist only in language (plan §6.4). It matches the vendored-binary pattern in the ways that matter — exec'd, no cgo, no service — and ships in the single image with its model file like the rest of the tooling. ⚠ **It is NOT self-contained the way `yt-dlp` is** (this line used to say it was): whisper-cli links `libwhisper` + `libggml`, so those ship beside it, and **ggml `dlopen()`s its compute backend from the executable's own directory** — hence the binary lives in `/usr/local/lib/whisper` with a symlink on `PATH`. Getting that layout wrong produces a binary where `--help` succeeds and the first real transcription aborts (`GGML_ASSERT(device) failed`), so the image proves whisper by **transcribing at build time**, never by `--help`. On amd64 upstream ships 15 `libggml-cpu-*` microarchitecture variants selected at run time; copy the whole set or it fails only on untested host CPUs. ⚠ **Model size is a correctness property, not a tuning preference:** verified against the vendored **v1.9.1** binary on a real 244s commercial break — `tiny.en` dropped a complete 20s advert at the file's average loudness and `base.en` dropped 7s of equally audible speech, while **`small.en`** had no gap over audible content (its only gap is true near-silence). `small.en` therefore ships, at **466MB** — the single largest item in the image. Full method and table: plan §6.4.
- **ffmpeg is bundled** (not skipped) so yt-dlp can merge separate video/audio streams — without it, high-resolution YouTube sources either fail or silently downgrade to a muxed low-quality rendition, which is a poor default for content that will be shown between programs. The cost is a second fast-moving vendored binary; both are version-pinned in the image and overridable by path (§10 config).
- CI (GitHub Actions): `golangci-lint`; `make openapi` then **`git diff --exit-code api/openapi.yaml`** (spec drift = red); **`vacuum`** lints the spec as valid 3.1; FE Biome + typegen + `tsc` + Vitest (jsdom units) + story-coverage; Storybook build + Playwright visual/a11y over `storybook-static` (Docker); Playwright e2e smoke.

### 14.1 Backend structure — the rules, and what they are not

Recorded after a full sweep of `internal/`, because two of the rules below exist to stop a *plausible-sounding* refactor rather than to prescribe one.

**What holds, and is worth keeping:**

- **The dependency direction is one-way.** No domain package imports `internal/api`; `internal/app` is the only composition root. Verified, not assumed — a domain package that needs an API type is a sign the type belongs in the domain.
- **`internal/testkit` never reaches production.** `go list -deps ./cmd/loomarr` must not contain it. Test doubles compiled into the shipped binary is a seam that only ever gets wider.
- **Every package carries a package doc.** They are the orientation for a subsystem whose invariants are not obvious from its types — `internal/playout` (added in this sweep) is the clearest case: the ffconcat mechanism, the wall-clock rule, and the drop-the-viewer-not-the-message inversion are all invisible from the function signatures.
- **`panic` is for boot-time programmer error only** — a duplicate settings key, an undeclared job name. Never for a runtime condition an operator could cause.
- **A file that has accreted past ~600 lines gets split along its seams, not arbitrarily.** `api/channels.go` was 1082 lines / 15 handlers / 25 DTOs and became four files: CRUD, wire shape + mapping, the now/next strip, and the preview surfaces. The tell that the split was real: `podToPoolDTO` and friends were already shared with `programming.go` and `guide.go`, so they had never been channel-lifecycle code — they were just living in the channel-lifecycle file.

**Two things that look like problems and are not.** Both were flagged from metrics during the sweep and both survived contact with the code:

- **`BuildHandler` is ~630 lines and stays that way.** Decomposing it into methods on a shared builder would convert ~70 locals into fields on a mutable carrier — *widening* their scope, and trading compile-time use-before-assignment errors for runtime nils. The sections are sequential and genuinely interdependent (three deliberate back-patches). A composition root may be long; it may not be unnavigable, so it carries a section map instead. Its heavy `if st != nil` nesting is likewise deliberate: a container started without `DATABASE_URL` must answer `/readyz` with the reason rather than crash-loop past the probe that would explain it.
- **`api.Server`'s 33 fields are not a service locator.** Every field is a narrow, purpose-named interface (`LoginService`, `PodPreviewer`, `ChannelBinder`) with a doc comment stating what it wires and what `nil` means, and the nil-means-501 convention is uniform — 33 optional capabilities, 33 `errNotImplemented` guards. That is what lets an unconfigured install boot and explain itself. Grouping them into sub-structs would add indirection at every call site and bury the one thing the comments make plain.

**The general rule the two exceptions illustrate:** a line count or a field count is a prompt to go and read something, never a finding on its own. Both of the above were "obvious" refactors until the code was read, and both would have made the system worse.

### 14.2 The package map

`internal/` is **32 flat packages, deliberately** — the grouping below is prose, not directories.

Nesting them under `internal/{domain,adapters,platform}/` was considered and rejected on evidence: four of the six would-be "adapters" import domain packages (`tmdb`→`provision`, `requester`→`provision`, `programmer`→`schedule`, `library`→`filler`), so the folder would announce a layering the code correctly violates. And it violates it correctly — a requester must speak `provision.Key`, because requesting a title *is* a provisioning operation. The domain half has no clusters either: it is a core (`provision`, `schedule`, `store` — imported by 7, 5 and 5 of 9) with satellites.

Go packages already carry a name, a compiler-enforced import list, and a doc. A directory above them would lengthen every import path and enforce nothing. (Contrast the frontend, where folders *were* worth adding: 46 React components in one directory have no enforced boundary at all, which is why they had grown `channel-*`/`guide-*` filename prefixes — a naming convention doing a filesystem's job. There, folders replaced a convention with structure. Here the structure is already there.)

**The loop** — what turns an intent into a channel that plays:

| Package | Job |
| --- | --- |
| `suggest` | Turns an intent into a grounded proposal (§8) |
| `catalog` | The federated search boundary the suggester is grounded against (§7.2, §8) |
| `binder` | Materialises an APPROVED proposal onto a channel — the one path (§7) |
| `schedule` | The scheduler domain: Channel identity, DesiredLineup, policy (§9) |
| `channels` | The reconcile engine — the conductor that drives a channel to its desired state (§9, §18) |
| `recurate` | Scheduled re-curation: a channel that keeps itself current (§8.2) |
| `provision` | The Title/Key identity model and the acquisition state machine (§3–§4) |
| `reconcile` | The provisioning backstop when a webhook never arrives (§4, §7, §18) |
| `retention` | What may be purged, in what order, after how long (§5, §18.1) — the policy; `store` owns the SQL |
| `filler` | Commercials: the clip catalog and seeded pod assembly (§10) |
| `playout` | Loomarr's own streaming engine — lineup to MPEG-TS (§9.1) |

**The ports** — everything that talks to something outside the process. Each is a boundary with one implementation today and a second one plausible tomorrow:

| Package | Speaks to |
| --- | --- |
| `library` | Emby/Jellyfin, one adapter for both flavours (§6) |
| `requester` | Seerr, or Sonarr/Radarr directly (§2, §6) |
| `programmer` | Tunarr — the port the scheduler pushes channels through (§6, §9) |
| `llm` | Any provider, behind one Chat primitive (§8) |
| `tmdb` | TMDB, the grounding corpus (§8) |
| `clipfetch` | yt-dlp / Archive.org, into the filler drop-folder (§10, §16) |

**The platform** — the machinery every feature sits on:

| Package | Job |
| --- | --- |
| `store` | One Store interface, two backends, one conformance suite (§5) |
| `settings` | The typed registry; `env > database > default` (config-design) |
| `config` | ENV-ONLY bootstrap — the handful of values needed before the store opens |
| `scheduler` | Recurring work as named, tunable, on-demand jobs (§18.1) |
| `activity` | Records what Loomarr did, for the Dashboard feed (§5, §12) — written at each domain transition, never off the lossy event bus |
| `auth` | Sessions and their validation (§11) |
| `events` | The in-memory bus behind SSE (§7) |
| `httpx` | The shared outbound HTTP client factory (§6) |
| `metrics` | The Prometheus surface (§7, §18) |
| `buildinfo` | The version stamped in at build time |

**The edges** — inbound HTTP, wiring, and the things that only exist for tests:

| Package | Job |
| --- | --- |
| `api` | The inbound HTTP surface: handlers, DTOs, the OpenAPI definitions (§7) |
| `app` | The composition root — the only place subsystems are wired together |
| `web` | Embeds the built SPA, served same-origin (§12) |
| `setup` | The operator connection flows: Live TV wiring, connection tests (§7, §13) |
| `testkit` | Shared test doubles and pinned fixtures — never linked into the binary |
| `integration` | Journey tests that drive the REAL composition root |
| `eval` | Semantic evaluation of suggester output, run by hand not by CI |

---

## 15. Configuration — layered settings

**Full subsystem design — registry schema, resolution semantics, secrets lifecycle, Settings IA, wizard integration — lives in `config-design.md`.** Every setting resolves **`env > database > default`**, per key, through one **typed settings registry** (backed by the §5 settings store; all subsystems read via the settings service, never `os.Getenv` directly). An env var that is set wins and **locks its UI field** ("set via environment"); unset, the setting is managed in the app (§13). Connection settings **hot-apply** on save (adapters read through the service; intervals re-read per tick; the LLM provider hot-swaps per §8.1) — the rare restart-required keys are exactly the bootstrap set below.

### Bootstrap (needed before/independent of the DB) — `env > file > default`

*Revised by V5: these were env-**only**. The wizard's Database step must persist which
database to use and cannot write into the database it is choosing, so a narrow file tier
sits beneath env — `bootstrap.json` in the data directory, bootstrap keys only, env still
wins. See `config-design.md` §1. Every app-managed setting is unchanged at
`env > database > default`.*

| Env var | Required | Example / default |
| --- | --- | --- |
| `DATABASE_URL` | no | **default `sqlite:///data/loomarr.db`** / `postgres://…` |
| `AUTO_MIGRATE` | no | `true` |
| `LISTEN_ADDR` / `LOG_LEVEL` | no | `:8080` / `info` |
| `TZ` | no | container time zone; time-slot schedules computed here (§9) |

**Zero required env** for a SQLite first run: `docker run -v loomarr-data:/data loomarr` → wizard.

### Generated secrets (created at first migration; view/regenerate in Settings; env override optional)

| Setting (env override) | Notes |
| --- | --- |
| `SESSION_SECRET` | Signs session cookies (§11). Regenerating invalidates all sessions. |
| `API_TOKEN` | Machine access + break-glass admin (§11) — the Sonarr API-key model. |

### Application settings registry (UI-managed; each key's env name pins it)

| Setting (env name) | Default / example |
| --- | --- |
| `LIBRARY_FLAVOR` / `LIBRARY_URL` / `LIBRARY_TOKEN` | `emby` \| `jellyfin` / `http://emby:8096` / *(secret)* |
| `REQUESTER_PROVIDER` | `seerr` \| `arr` — selects the acquisition backend (load-bearing, like `LLM_PROVIDER`); gates which fields show. |
| `SEERR_URL` / `SEERR_API_KEY` | `http://seerr:5055` / *(secret)* — when `REQUESTER_PROVIDER=seerr` |
| `SONARR_URL` / `SONARR_API_KEY` / `RADARR_URL` / `RADARR_API_KEY` | direct requester (`REQUESTER_PROVIDER=arr`): TV→Sonarr, movies→Radarr / *(secrets)*. Optional `SONARR_QUALITY_PROFILE`/`SONARR_ROOT_FOLDER` (+ `RADARR_*`) pin the profile/root; blank = the arr's first. |
| `TUNARR_URL` | `http://tunarr:8000` (Tunarr has no auth; no key config) |
| `TUNARR_TRANSCODE_CONFIG_ID` | Tunarr transcode-config uuid created channels reference (Phase-0: channel create requires a valid `transcodeConfigId`; empty → resolve the instance `Default` via `GET /api/transcode_configs`, §9) |
| `SERVER_PUBLIC_URL` | **Re-scoped by §9.1 — no longer icon-only, and no longer Advanced.** Loomarr's own address as your media server *and* Tunarr reach it (e.g. `http://loomarr:8080`). Internal playout serves **every stream segment** from this base, so a wrong value means channels appear in the guide and never play. Still also used for **uploaded** channel icons — the stored icon URL is built from this, never from request headers (Host-injection-safe). Deliberately ONE key rather than a second `playout.public_url`: it is genuinely the server's own address, both callers need the same value, and two keys could drift. Empty → a relative `/v1/channels/{id}/icon` URL for icons (works when Tunarr shares Loomarr's origin); internal playout requires it set. |

**Playout (§9.1 — added with internal playout).**

| Env | Meaning / default |
| --- | --- |
| `PLAYOUT_BACKEND` | `internal` (default) or `tunarr` — who streams a channel. **Overridable per channel** via `policy.playout.backend`, which rides `policy_json` (no schema change, like `rules`/`filler`/`window`/`autoCurate`). Nil per-channel = inherit this global, which is what makes "changing the default affects new channels only" true rather than aspirational. |
| `PLAYOUT_TRANSPORT` | `both` (default) / `hls` / `mpegts` — which formats internal playout offers. Both by default: MPEG-TS matches Tunarr's shape and keeps latency low, HLS survives proxies. |
| `PLAYOUT_ENCODER` | ffmpeg encoder (e.g. `libx264`, `h264_vaapi`, `h264_nvenc`). Empty ⇒ the best one the transcode check found. |
| `PLAYOUT_AUDIO_LANGUAGE` | `eng` (default) — ISO 639-2 code for the preferred audio track. A **preference**: an optional map plus a first-track fallback, so a file with no track in that language still gets audio rather than failing to encode. Empty ⇒ ffmpeg's own choice, which picks the track with the **most channels** and ignores language entirely — that is how a 5.1 Russian dub beats a 2.0 English track (§9.1). |
| `PLAYOUT_FFMPEG_PATH` | `ffmpeg` — the binary playout executes. Deliberately **separate from `INGEST_FFMPEG_PATH`**, though ⚠ **not for the reason this row used to give** (it cited the filler sidecar bundling its own ffmpeg in a different image — there is one image now, §16, so that rationale died with the sidecar). The live reason is that the two fail differently: playout's ffmpeg is a runtime dependency of a channel that is **on air**, ingest's is a dependency of a download nobody is watching, so repointing one must not be able to break the other. Advanced; the default is right whenever ffmpeg is on `PATH`. |
| `PLAYOUT_QUALITY_TIER` | `balanced` (default) / `efficient` / `quality` — the picture-vs-channel-count target. Resolved at each program boundary against measured capacity and current load, so quality adapts as channels come and go rather than being fixed per channel (§9.1). |
| `PLAYOUT_MAX_CHANNELS` | `4` — concurrent encodes. The wizard's transcode check measures a realistic figure; a test pattern encodes cheaper than film grain, so treat any measurement as a starting estimate. |
| `PLAYOUT_TOKEN` | **Generated secret** (§11 device auth), viewable because it must be pasted into a tuner/listings URL by hand. Signs every segment request so only your media server can pull a stream. Distinct from `API_TOKEN`: that is break-glass **admin** with full authority; this grants nothing beyond reading streams. |

**Backup (§16 — added with the Backup UI).**

| Env | Meaning / default |
| --- | --- |
| `BACKUP_SCHEDULE` | `0 30 3 * * *` — nightly instance backup. A backup is the whole instance (settings, channels, people, generated secrets), so treat the file as a credential. |
| `BACKUP_RETAIN` | `7` — how many to keep before pruning the oldest. |
| `BACKUP_DIR` | `/data/backups` — inside the documented volume by default; point elsewhere to keep backups off the database's disk. |
| `LLM_PROVIDER` / `LLM_URL` / `LLM_MODEL` | `ollama` \| `openai` / base URL / model id. **`LLM_PROVIDER` is load-bearing** (selects the client). For `openai`, `LLM_URL` is the OpenAI-compatible **base URL** (a hosted `…/v1`, or Ollama's own `http://ollama:11434/v1`). Local default: `ollama` + `qwen3:8b` (or `qwen3:14b` at **Q6_K** — stock Q4 degrades tool-calling/JSON). **Initial defaults only:** an in-app selection (§8.1) persisted to the settings store (`llm.provider`/`llm.url`/`llm.model` + per-provider secret `llm.api_key.<provider>`) **overrides** them and hot-swaps the running suggester, so a UI choice survives a reboot without editing env. |
| `LLM_API_KEY` | *(secret; read for `LLM_PROVIDER=openai`. An in-app hosted selection stores its own per-provider key in the settings store, overriding this — §8.1; **never echoed** by any API.)* |
| `LLM_KEEP_ALIVE` | `30m` — how long a **local** Ollama model stays resident between calls (§8.2). Loading an 8B model costs ~9s vs ~0.5s warm, and Ollama unloads after 5m idle, so the stock behavior makes a describe→read→refine cycle re-pay the load every time. `0` disables (stock unload) for a memory-tight host. Ignored by hosted providers, which have no residency to manage. |
| `TMDB_API_KEY` | *(secret; grounds suggestions — required if the suggester is enabled)* |
| `REQUEST_TTL` / `DOWNLOADING_TTL` / `RECONCILE_EVERY` | `48h` / `12h` / `5m` |
| `CHANNEL_RECONCILE_EVERY` | `10m` (periodic channel sweep, §9) |
| `SESSION_TTL` / `COOKIE_SECURE` | `720h` / `auto` (§11) |
| `LOOMARR_PPROF` | *(unset)* — **development only.** `1` mounts `/debug/pprof/*` (§7). Unset ⇒ the routes do not exist. Bootstrap-tier for the same reason as `LOOMARR_DEV_LOGIN`: it decides which routes are mounted, and a profiling surface an admin session could switch on at runtime would be a worse hole than the one it opens. Boot WARNs while it is on. |
| `LOOMARR_DEV_LOGIN` | *(unset)* — **development only.** `1` registers `POST /v1/auth/dev-login`, a credential-free admin sign-in (§11), and makes the login screen offer it. Unset ⇒ the route does not exist. Bootstrap-tier (read at boot, not hot-appliable): it decides which routes are mounted, and a bypass that could be switched on at runtime through the settings API would be a worse hole than the one it opens. Boot WARNs on every startup while it is on. |
| `JOB_WORKERS` / `JOB_TIMEOUT` | `2` / `10m` (§8) |
| `JOBS_RETENTION` / `PROPOSALS_RETENTION` | `720h` / `2160h` (§5 janitor) — ⚠ **declared but not consumed**; no purge exists for either table yet (§5). |
| `ACTIVITY_RETENTION` | `720h` — how long Dashboard activity rows are kept before `activity-purge` removes them (§5, §18.1, V32). |
| `episodes.max_age` | `24h` — how stale a cached series episode list may be before `series-episode-refresh` re-enumerates it (§5). A miss or an aged-out row still falls back to the live library call, so this bounds staleness, never correctness. |
| `EVENT_WEBHOOK_URL` | optional external event target |
| `SUGGEST_AUTO_APPROVE` / `SUGGEST_MAX_ACQUISITIONS` | `false` / `10` |
| `SEASON_PRECISION` | `series` (default) \| `seasons` (§6) |
| `SCHED_DEFAULT_STRATEGY` / `SCHED_BACKFILL` | `shuffle` / `stable` (§9) |
| Policy defaults: `SCHED_EPISODE_NOREPEAT` / `SCHED_MOVIE_NOREPEAT` / `SCHED_SERIES_MIN_GAP` / `SCHED_BLOCK_MAX` / `SCHED_ORDERING` / `SEASONAL_MODE` | `168h` / `720h` / `2h` / `2` / `syndication` / `auto` (per-channel overridable — `programming-design.md`) |
| `SCHED_WINDOW_HOURS` | `24h` (rolling-window horizon a channel materializes; per-channel/-rule overridable, `0` = the whole run — `programming-design.md` §6.5) |
| `FILLER_DIR` / `FILLER_SYNC_EVERY` / `FILLER_AI_TAGGING` | **`/data/filler`** / `15m` / `false` (§10). ⚠ **V38c: this is the CLIP FOLDER** — Loomarr's own store, holding `a3/f9/<hash>.mp4` plus sidecars, and the only directory Loomarr rearranges. *(It briefly meant "the first watched folder" in V38c's intermediate model, before "Two folders, one pipeline" split arrival from storage. The key kept its name because its meaning — where the clips are — did not change; only the layout did.)* The folder Loomarr registers as a Tunarr `local` source. ⚠ **Defaults inside `/data`, like `DATABASE_URL` and `BACKUP_DIR`** — it was previously empty for no recorded reason, which made filler opt-in by accident: a zero-env install opened the Filler page on a single "no folder configured" empty state, hiding every shipped filler capability behind a config step. Created at boot if missing (the scanner treats a missing root as fatal by design, so a default that did not exist would swap an honest empty state for a scan error) |
| `FILLER_WATCH_DIR` | **`""` ⇒ `<FILLER_DIR>/_watch`** (§10 V38c, "Two folders, one pipeline"). Where clips ARRIVE — downloads land here, operators drop files here — and Loomarr drains it into the clip folder on every sync. ⚠ **The default is derived rather than a literal**, so pointing `FILLER_DIR` at an existing library moves the watch folder with it instead of leaving it orphaned under `/data`. ⚠ **Underscore-prefixed and INSIDE the clip folder on purpose**: a sibling default would need a second mounted volume to survive a restart, and a watch folder that vanishes silently loses whatever had not been filed yet. The scan skips it by name, so a file waiting there is never catalogued from its arrival path |
| `FILLER_BREAKS_PER_HOUR` / `FILLER_POD_MAX` | `4` / `4` (density + pod size) |
| `FILLER_COOLDOWN_SECONDS` / `FILLER_WEIGHT` | `30` / `1` (Tunarr filler-list attach: min seconds before a clip repeats; relative draw weight across multiple filler-lists) |
| `FILLER_MIN_QUALITY` | `0` — minimum clip height in px for a commercial to be eligible (`480` excludes 240p rips). **`0` disables the floor, and that is the default**: quality is display-only unless an operator opts in, because a blanket "prefer HD" starves the era-accurate 4:3 commercials §10 exists to play (V17c) |
| `FILLER_MIN_DURATION` | `10s` — the quality gate's floor (§10 V40). A clip shorter than this is **rejected at the scan boundary** and never becomes a catalog row at all. ⚠ Distinct from `FILLER_MIN_QUALITY`, which is an opt-in *eligibility* filter over clips that already exist: this one rejects, and its default is ON. It exists because `DurationMs <= 0` was the only guard, and a 2.9KB / 33ms truncated download passed it and sat airable in the catalog |
| `FILLER_TARGET_LUFS` | `-23` — the broadcast loudness target filler is normalised to (§10 V40, §9.1). Measured spread across real fetched clips was −21.8 to −32.6 LUFS, about 11 dB of clip-to-clip jump. ⚠ **Applied at PLAYOUT by default** — the drop-folder holds the operator's own files, so Loomarr does not rewrite them unasked. ⚠ **ONE target for both stages**: `FILLER_AUTOFILE_NORMALIZE_LOUDNESS` reuses this value rather than declaring its own, or a clip normalised on file would be corrected again at playout toward a different number. Set empty to disable |
| `FILLER_AUTOFILE_NORMALIZE_LOUDNESS` | `false` (V42) — when on, auto-file rewrites the clip in `FILLER_DIR` with ffmpeg `loudnorm` at `FILLER_TARGET_LUFS` before it enters the catalog. ⚠ **DESTRUCTIVE and opt-in for that reason**: the original is unrecoverable. The sidecar records `normalizedLufs` so a re-scan skips a file already normalised at the current target — without that marker every pass would re-normalise, walking the loudness down each run |
| `FILLER_LANGUAGE` | `en` — the language filler is expected to be in (§10 V40). A clip whose SPEECH is confidently something else is rejected; a clip with no speech at all is always kept, because a wordless visual spot has no language and those are often the best filler. Empty disables the language gate entirely |
| `FILLER_LANGUAGE_PROVIDER` | `whisper` \| `hosted` — which engine answers "what language is this?" (§10 V40), mirroring `LLM_PROVIDER`'s local-vs-hosted split. **`whisper`** uses the vendored `whisper-cli` + model already configured by `INGEST_WHISPER_*`: free and offline, but ~3s per clip natively and **~341s under QEMU**, which is why the job runs in the BACKGROUND and why an arm64 install effectively needs the hosted path. **`hosted`** sends a ~10s audio span to an audio-input model through the §8.1 hosted provider: ~1s regardless of architecture, fractions of a cent per clip. ⚠ **NOT Ollama** — it has no audio input path at all (probed 2026-08-03: `completion`/`vision`/`tools`/`thinking`, no `audio`), so "we already run a local LLM" does not remove the need for whisper. ⚠ Hosted sends clip audio off the box and spends money per clip, so local is the default and hosted is a deliberate choice |
| `INGEST_YTDLP_PATH` / `INGEST_FFMPEG_PATH` | vendored paths in the image; **unset ⇒ looked up on `PATH`** (V38b), so a source build with the tools installed works without configuring anything. Overridable so an operator can run a newer yt-dlp than the image ships. `ffmpeg` is also the internal-playout encoder (§9.1), so pointing this at a broken binary degrades playout too. ⚠ **They gate DIFFERENT things** — see §10's "Two downloaders, two gates": ffmpeg alone enables archive.org; yt-dlp adds YouTube |
| `INGEST_WHISPER_PATH` / `INGEST_WHISPER_MODEL` | vendored paths in the image — the whisper.cpp binary and its model file (§10, §14, V34). Unset/unrunnable ⇒ compilation splitting's transcript-rescue step is unavailable: over-long segments surface to the operator as **unsplittable** in the review UI rather than being guessed at (coarse splitting still works — it needs only ffmpeg). Overridable like the other tool paths |
| `INGEST_MAX_CONCURRENT` / `INGEST_TIMEOUT` | `2` / `30m` (bounded parallel downloads; per-item wall-clock ceiling so one wedged fetch can't hold a worker forever) |
| `FILLER_STARTER_COLLECTION` | the archive.org collection the **starter pack** lists on a fresh install (§10). A curated default, not a hardcoded truth: point it at your own collection, or **set it empty to turn the pack off entirely**. Listing only — nothing downloads without the operator keeping a row. ⚠ **From V35 this seeds a pull** rather than driving its own flow, so the "nothing downloads" property is now enforced by the approval gate instead of by a UI convention |
| `FILLER_AUTOFILE_ENABLED` / `FILLER_AUTOFILE_MIN_CONFIDENCE` | **`true` / `85`** (§10 V38). Whether a tagged clip is filed automatically, and the score it must reach. ⚠ **These keys were REMOVED from this table in V35's review** as declared-but-unconsumed — §15's own rule is that a setting not in the registry does not exist. They return **with their consumer, in the same PR**: the filing path reads them, and a test proves a clip below the threshold reaches Incoming instead of the catalog. ⚠ **ON by default means an existing install starts auto-filing on its first tagging run after upgrade** (maintainer, 2026-08-02) — a deliberate product call. What makes it safe is not the number but §10's grounding **cap**: an ungrounded era cannot reach any threshold, so the fabrication class stays with a human regardless |
| `FILLER_SPLIT_EVERY` | `6h` (§10 V43). How often the `filler-split` job looks for over-long catalog clips and proposes cuts. ⚠ **On by default, because PROPOSING costs nothing an operator must undo** — an unconfirmed proposal writes no clips. What it buys is that review is waiting when they look, instead of a click and minutes of ffmpeg. `0` disables the job. ⚠ Only clips longer than `FILLER_AUTOSPLIT_MAX_DURATION` are candidates: a 30-second advert is not a compilation, and running detection over the whole catalog every cycle would spend hours proving that |
| `FILLER_AUTOSPLIT_ENABLED` / `FILLER_AUTOSPLIT_MIN_CONFIDENCE` | **`false` / `85`** (§10 V43). Whether an unambiguous split is confirmed without a human, and the score every segment must reach. ⚠ **OFF by default, unlike auto-file** — the one it resembles is ON. Cutting is destructive in a way tagging is not: a mis-tagged clip plays in the wrong break, a mis-cut clip plays half an advert, and the source file is consumed either way. An operator opts in once they have seen the judgement on their own reels. ⚠ **A SEPARATE threshold from `FILLER_AUTOFILE_MIN_CONFIDENCE`, deliberately.** One dial would force the stricter of two different failure modes to govern both |
| `FILLER_AUTOSPLIT_MAX_DURATION` | `120s` (§10 V43). The longest a segment may be and still count as advert-shaped. ⚠ Serves TWO jobs and that is why it is one key: it selects which catalog clips the split job even looks at (longer than this ⇒ a compilation worth detecting), and it is the ceiling every segment must clear for auto-confirm. A single number keeps those two answers from disagreeing — a clip the job considers too long to be an advert must not then auto-confirm as one |
| `FILLER_FETCH_EVERY` | `6h` (§10 V38b). How often each registered source is polled for new items. ⚠ **`0` disables auto-fetch entirely** — the escape hatch for an operator who wants acquisition to stay manual, and the value to reach for before disabling sources one by one. ⚠ **V38c: this is now the DEFAULT, not the only value** — a source may override it, and `0` on one row means *that* source never auto-fetches. Inherit is NULL, never 0 |
| `FILLER_FETCH_MAX_PER_RUN` | `10` (§10 V38b). Items ONE source may pull per poll. ⚠ The bound that stops "add a source" meaning "download 8,000 files tonight" — an archive.org collection is thousands of items, and this is what makes it trickle rather than flood |
| `FILLER_FETCH_MAX_CATALOG_CLIPS` | `2000` (§10 V38b). Auto-fetch stops when the catalog reaches this. ⚠ Manual queueing and approved pulls still work at the limit: a ceiling on what happens UNATTENDED is not a ceiling on what an operator may deliberately do |
| `FILLER_FETCH_MAX_DISK_GB` | `20` (§10 V38b). Same, for drop-folder size. ⚠ Measured against the folder, not a running total, so files an operator deletes by hand are noticed |
| `FILLER_SOURCE_FOLDER_ENABLED` | `true` (§10 V35). The drop-folder's on/off switch. It is a setting rather than a row because the folder is **derived from configuration** — a remote collection's switch is a column on its own row. Disabling stops the catalog scan; ⚠ **it never removes clips already in the catalog**, and the enforcement lives in the syncer, not in the UI. ⚠ There is deliberately **no library equivalent**: nothing scans a media-server library for filler (§10), so the key would gate nothing |
| `USER_SYNC_EVERY` | `1h` (user import/sync from the media server) |

**Secrets handling:** stored in the DB following ecosystem practice (Sonarr, Seerr); masked after save (replace-only in the UI), never logged, excluded from `/v1/setup/status`; env-supplied secrets may come from env or mounted files (`<VAR>_FILE`), never baked into the image. This table mirrors the code registry — a setting that isn't here doesn't exist (CLAUDE.md do-nots). Full mechanics: `config-design.md`.

---

## 16. Deployment (Docker)

Multi-stage build → **distroless static** or `scratch` (pure-Go SQLite driver ⇒ no cgo). Toolchain pins: **Go 1.22+** for the binary, **Node 20+** in the FE build stage. Non-root. `HEALTHCHECK` → `/healthz`. The web UI is embedded and served at `/`.

**One image (revised — supersedes "two tags, one binary").** `loomarr:latest` is the only published tag. It vendors pinned **`yt-dlp`** + **`ffmpeg`** + **`ffprobe`** + **`deno`** + **`whisper-cli`** (with its model file, §14 — added by V34) on a non-distroless base (those binaries are glibc-linked), at **~1.3GB measured** (amd64 uncompressed rootfs; **~821MB before whisper-cli**). ⚠ The **+486MB is almost entirely the whisper MODEL** (466MB for `ggml-small.en.bin`; the binary and its libggml/libwhisper set are only ~20MB) — and that model size is a correctness floor, not a preference (§14). Re-measured when V34 landed, as this line previously promised. ⚠ **Do not compare this number to the "549MB" below or the "510MB" this line used to claim**: those were *compressed registry* sizes for a tree that no longer exists, and mixing the two units is how the earlier figure drifted. What an operator downloads is the compressed pull, which is smaller.

It pre-creates `/data` owned by uid 65532 and declares it a `VOLUME`, so a fresh named volume inherits nonroot ownership and the documented `docker run -v loomarr-data:/data loomarr` boots. Without that the volume arrives root-owned and boot dies with *"unable to open database file (14)"* — a failure that was **masked** while `DATABASE_URL` had no default (§15), because the app never tried to open a file. Compose's one-shot chown init container stays for **bind mounts**, which the image cannot pre-seed.

*Superseded model, recorded because the reversal matters:* the project previously published a 31MB `loomarr:latest` with no media tooling plus a 549MB `loomarr:filler` variant that added it, so an operator opted in with a tag change and a restart. That split existed to keep media tooling out of the default image — the same goal that had earlier motivated a separate ingest sidecar, itself already reversed in favour of the opt-in tag.

**Why one image now.** §9.1 makes `ffmpeg` load-bearing for *playout*, not just ingest. A tag without an encoder can't serve a channel, which means the "slim" variant would no longer be a smaller Loomarr — it would be a Loomarr that can't do the main thing. Two tags where one is functionally incomplete is a support burden, not a choice, so the split collapses.

**The cost, stated plainly:** the default download grows by more than an order of magnitude (31MB → a rootfs of ~1.3GB uncompressed, ~821MB of it before V34's whisper model) and every install carries an encoder whether or not it uses internal playout, plus a speech model whether or not it ever splits a compilation. That is the price of the capability, and it is the third time this packaging question has been decided — sidecar → opt-in tag → single image. Each reversal followed a change in what the tooling was *for*; if a future change makes the encoder optional again, revisit it with that history in view rather than as a fresh question.

Every binary is invoked via `exec` — `yt-dlp`, `ffmpeg`, `ffprobe`, `deno`, and `whisper-cli` (plus its model file, added by V34, §14) are the only vendored non-Go artifacts the project permits (§14).

### Compose (profiles: sqlite · postgres · ai)
- **sqlite:** just `loomarr` + a `/data` volume for the DB file.
- **postgres:** `loomarr` + `postgres:16` (or external). No SQLite volume.
- **ai:** adds a local **Ollama** service (skip if using a hosted OpenAI-compatible provider or an external Ollama). The service ships **ready-to-use but model-less** — model choice is the wizard's job (§8.1: it depends on the user's GPU), so no model is baked in. Three deploy affordances, all optional and design-aligned: (1) a **healthcheck + `depends_on` gate** so `loomarr` waits for Ollama before its first probe (no transient "AI host unreachable" on first load) — the `depends_on` is `required:false`, so a hosted/external-LLM deploy that omits the `ai` profile skips it; (2) **opt-in GPU passthrough** via a separate overlay (`docker/compose.gpu.yaml`, NVIDIA + nvidia-container-toolkit; mirrors the dev Tunarr overlay) — without it Ollama runs on CPU (works, but slow); (3) **opt-in model preload** — set `LLM_MODEL` and a one-shot `ollama-pull` fetches it on first boot for a zero-wizard-step install; left empty (the default), the wizard picks the model, preserving the §8.1 "the user picks" default.
**Filler ingest needs no profile, no tag, and no service.** The vendored yt-dlp + ffmpeg + deno ship in the single image (§16), so in-app clip downloads work out of the box — mount a drop-folder and go. *Revised: this supersedes both the `filler` compose profile and the opt-in `loomarr:filler` tag that replaced it; see §10's history note for why the question moved three times (retired-ok).*

The image is **non-root** (distroless `nonroot`, uid 65532). A freshly-created named
volume is owned by `root:root`, so under the sqlite backend the container cannot create
`/data/loomarr.db` (`SQLITE_CANTOPEN`) on first run. Fix it **in compose**, not by running
the app as root: a one-shot `loomarr-init` sidecar chowns the volume to uid 65532 before
`loomarr` starts (`depends_on … service_completed_successfully`). This is sqlite-only — the
postgres backend has no `/data` volume — so the init runs under the `sqlite` profile.

```yaml
services:
  # sqlite-only: chown the fresh /data volume to the nonroot uid before loomarr starts.
  loomarr-init:
    image: busybox:1.36
    profiles: ["sqlite"]
    command: ["sh", "-c", "chown -R 65532:65532 /data"]
    volumes: ["loomarr-data:/data"]

  loomarr:
    image: loomarr:latest
    depends_on:
      loomarr-init:
        condition: service_completed_successfully   # sqlite profile only
        required: false                             # postgres profile skips it
    environment:
      DATABASE_URL: ${DATABASE_URL}
      LIBRARY_FLAVOR: ${LIBRARY_FLAVOR}
      LIBRARY_URL: ${LIBRARY_URL}
      LIBRARY_TOKEN: ${LIBRARY_TOKEN}
      SEERR_URL: ${SEERR_URL}
      SEERR_API_KEY: ${SEERR_API_KEY}
      TUNARR_URL: ${TUNARR_URL}
      SESSION_SECRET: ${SESSION_SECRET}
      API_TOKEN: ${API_TOKEN:-}
      LLM_PROVIDER: ${LLM_PROVIDER:-ollama}
      LLM_URL: ${LLM_URL:-http://ollama:11434}
      LLM_MODEL: ${LLM_MODEL}
      LLM_API_KEY: ${LLM_API_KEY:-}
      TMDB_API_KEY: ${TMDB_API_KEY}
      AUTO_MIGRATE: "true"
    ports: ["8080:8080"]
    volumes: ["loomarr-data:/data"]   # sqlite backend only
    restart: unless-stopped
    # depends_on: [postgres]       # postgres profile

  # postgres:                      # postgres profile
  #   image: postgres:16
  #   environment: { POSTGRES_DB: loomarr, POSTGRES_PASSWORD: ... }
  #   volumes: [pg-data:/var/lib/postgresql/data]
  # ollama:                        # ai profile
  #   image: ollama/ollama:latest
  #   volumes: [ollama:/root/.ollama]
  # For in-app clip downloads, mount the drop-folder (the tooling is already
  # in the image — no tag change, no extra service):
  #   volumes: [/mnt/media/filler:/filler]      # the folder Tunarr scans as a `local` source

volumes:
  loomarr-data:
  # pg-data:
  # ollama:
```

### Backup & restore
The database **is** the product — channels, tags, proposals, audit trail — so data safety is not optional (every mature *arr app ships backups):
- **SQLite:** `GET /v1/backup` (admin) streams a **consistent snapshot** produced via `VACUUM INTO` a temp file — pure SQL, so it works with the cgo-free driver and is safe while WAL is active (never `cp` a live SQLite file). Restore = stop container, replace `/data/loomarr.db`, start.
- **Postgres:** `/v1/backup` returns **501 + a docs pointer** — the container has no `pg_dump` (scratch image, by design); back up with `pg_dump` against the DB directly, restore with `pg_restore`. The docs Quickstart shows a one-line cron example for each backend.
- **Server-written backups** (`backup.dir`, default `/data/backups`) exist as of V11, because the
  migration's backup gate has to be enforceable by the server: a streamed download leaves no
  evidence, so "a backup is required, not suggested" would be unenforceable. Same `VACUUM INTO`
  snapshot; the difference is that the file is kept, at `0600` (a backup carries every secret the
  instance holds).
- **Scheduled backups with rotation** (V12). A `backup` scheduler job (§18.1) writes one snapshot
  into `backup.dir` on `backup.schedule` (default `0 30 3 * * *`) and then prunes the directory to
  the newest `backup.retain` files (default 7). Both keys were declared by V4 and consumed by
  nothing until here; the migration stepper's `WriteBackup` is the same writer, so a scheduled
  backup and a pre-migration backup are the same artifact and one is a valid restore point for
  the other.
  - **Pruning is scoped by filename**, not "everything in the directory": only files matching the
    `loomarr-<timestamp>.db` pattern this code writes are eligible. `backup.dir` is operator-set
    and may point at a directory holding other things — a retention policy that deletes files it
    did not create is a data-loss bug wearing a feature's clothes.
  - **Prune runs after a successful write**, never before. Pruning first would satisfy retention by
    destroying the oldest backup and then, if the snapshot fails, leave the instance with fewer
    backups than it started with — worst behaviour exactly when the database is unhealthy.
  - `backup.retain` of `0` disables pruning (keep everything), so an operator who wants unbounded
    history has a way to say so that isn't "delete them all".
  - **On Postgres the job does not register.** `WriteBackup` is SQLite-only by design (§16 above),
    so a registered job would fail on every fire and the Tasks page would show a permanently red
    row for a backup strategy the operator is correctly running with `pg_dump`.
- **Restore stays a CLI operation, deliberately.** Stop the container, replace the database file,
  start. There is no in-app restore button: the operation replaces the store the app is running
  on, including the accounts and sessions authorizing the click.

### Upgrades & downgrades
- **Images are semver-tagged**; docs steer production installs to pinned tags, not `:latest`.
- **Migrations are forward-only** (goose up; no down migrations to half-trust). The upgrade ritual, stated in the runbook and Quickstart: **back up, then pull.** Migrations that alter a table holding real data use `ALTER TABLE … ADD COLUMN` with a DDL default (the ChannelPolicy migration `00007` establishes this convention — earlier migrations only ever DROP+CREATE disposable caches like `clips`, which is unsafe for tables with durable rows).
- **Downgrade guard:** on startup, if the DB's schema version is **newer than the binary knows**, Loomarr **refuses to start** with a clear message ("DB is from a newer version — restore your pre-upgrade backup or return to tag ≥ X") instead of limping into corruption. Rolling back a container is a homelab reflex; this makes it safe-by-failure rather than silently destructive.

### Operator runbook
The first-run wizard (§13) walks these checks interactively — the list below is the manual path and the reference for what the wizard verifies.
1. `loomarr` must reach the library, Seerr, **Tunarr**, the LLM (if AI), and any external event target.
2. **Tunarr must have the same Emby/Jellyfin library as a media source** — that's how `loomarr` and Tunarr agree on titles.
3. Open the UI at `/` and **create the owning admin** (bootstrap: a local username + password, §11) — no media-server config needed to get in. Then connect the media server and import the accounts you want to admit. If local AI, pull the model into Ollama first.
4. Keep everything on an internal Docker network / LAN; do not expose publicly.

---

## 17. Observability
- **Logging:** structured (`slog`); one line per provisioning transition and per channel reconcile (diff summary).
- **Metrics (Prometheus):** records by state; requests submitted / give-ups; webhook events by type; library-lookup + reconcile-loop latency; **channel reconciles, Tunarr API latency/errors, slots pending-vs-filled per channel**; LLM latency + (hosted) token/cost, proposals generated, acquisitions proposed/approved/rejected, grounding-dropped candidates; filler clips synced/tagged/untagged and pod fallback-ladder depth (how often matching degrades); logins (success/failure) and active sessions; job queue depth + janitor purge counts; slot-drift substitutions. Log lines carry the relevant job/proposal/channel id as a correlation field.
- **Readiness** true only after DB connectivity + migrations, and (soft) Tunarr reachability.

---

## 18. Concurrency & correctness
- Per-key mutex serializes provisioning work per title; per-channel mutex serializes reconciles per channel.
- Upsert + external-id keying ⇒ idempotent writes; terminal provisioning states monotonic.
- Reconciler is the deadline enforcer + a library-presence backstop; availability itself is confirmed by the poll-based scheduler jobs (§18.1), not by an inbound webhook.
- **Channel reconciliation is desired-vs-actual and idempotent** — recompute + diff + minimal Tunarr calls; safe to re-run.
- Multi-instance (Postgres) needs `ClaimDue*` (titles, jobs, **and scheduled jobs**) via `FOR UPDATE SKIP LOCKED` / advisory-lock leader; **the channel reconcile loop likewise needs single-leader or per-channel row claiming** so two replicas don't fight over one Tunarr channel. In-memory availability events do not cross replicas — the periodic channel sweep (§9) is what makes multi-replica correct; events only reduce latency on the instance that emitted them. SQLite = single instance.

### 18.1 The job scheduler — named, tunable, on-demand background work

All recurring background work runs under **one scheduler** (`internal/scheduler`), modeled on how Sonarr/Radarr/Overseerr expose *System → Tasks*: a registry of **named jobs**, each with a **default interval** that is **user-configurable**, all **triggerable on demand** ("Run now"). This replaces the previous model of four independent `time.NewTicker` goroutines — there is now **one scheduling mechanism**, one place to see what runs when, and one way to force a run.

- **Jobs are code-defined; schedules are cron settings; run-history is state.** The set of jobs and their `Run` funcs live in a code registry (a runner can't live in a DB row). Each job's schedule is a **cron expression** (6-field, seconds-leading, Overseerr-style, e.g. `0 */5 * * * *`) in an ordinary **settings key** (`job.<name>.schedule`, a new `KindCron` validated via the cron lib, `env > db > default`, hot-read per tick), so a schedule is edited through the normal settings path (`PATCH /v1/settings`), not a bespoke one. Next-run is computed from the cron, not `now + interval`. *Last-run / next-run / last-result* is **runtime state** in a small `scheduled_jobs` table (keyed by job name), upserted after each run — this powers the Tasks UI.

- **Every job carries a Title AND a Description, both required.** The title is the human label ("Reconcile acquisitions"); the description is one plain sentence saying what running it actually does ("Checks in-flight downloads and moves finished ones into your library"). ⚠ **Required, not optional** — an operator deciding whether to run or pause a task needs to know what it does, and a nullable field is one where every later job ships without one. Enforced by the registry seal test, alongside the existing uniqueness checks.

- **A run reports progress, and the button reflects it.** Triggering a job moves its control through `Run now → Running… → done`, driven by the existing `job` SSE frame rather than client-side guessing. Jobs report **indeterminate** progress by default (a spinner, not a fake percentage); a job that genuinely knows its denominator may report a fraction. ⚠ **Never synthesize a percentage from elapsed time** — a bar that reaches 90% and stops is a worse claim than a spinner that says only "running".

- **Errors open up.** `last_error` is rendered as an expandable panel with the full message, not a truncated single line. The Tasks page is where an operator diagnoses a failing integration, and a clipped error is the one piece of information they came for.
- **River is the engine.** Each registry job becomes a River **periodic job** whose worker calls the same `Run` func; River owns due-selection, leadership (so only one replica runs a tick), retries with backoff, and the durable job records behind run history. **Run now** = inserting the job's args immediately — the same worker, no separate code path. The `scheduled_jobs` table remains the read model the Tasks page renders (last run, last result, paused), because River's own tables are keyed by *job execution*, not by "the operator's view of this named task".

  ⚠ **Build the cron parser explicitly; never use `cron.ParseStandard`.** River's docs point at it, and it is **5-field** — it rejects *every* schedule Loomarr has, all of which are 6-field seconds-leading (`0 */5 * * * *`). Since `job.*.schedule` values are operator-editable settings already persisted in the database, the documented example would have failed every saved schedule at boot on installs that were working fine. `cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)` accepts them all (verified in `docs/engineering/FINDINGS-river-spike-2026-07-30.md`). `gronx` stays as the settings **validator**, so `KindCron` keeps one definition of a valid expression.

  ⚠ **River's schema is applied at boot via `rivermigrate`, never the `river migrate-up` CLI.** goose owns the application catalog; River owns its own. Two migration *libraries* is a stated cost — two migration *systems an operator must run* would not be shippable, and is why the programmatic path is load-bearing rather than a convenience.

- **Pause is Loomarr's state, not River's.** A paused job is skipped when its schedule comes due and its row greys out on the Tasks page, showing "Paused" where the next-run time would be — a time that will never fire reads as a bug. Persisted per job (`job.<name>.paused`), so it survives restarts and leadership changes.

  ⚠ **River's own pause is the wrong granularity and the wrong durability.** `Client.QueuePause` pauses a whole *queue*, not a job; and `PeriodicJobs().Remove` only takes effect on the client holding leadership, so a restart or a leadership change would silently resume a job the operator deliberately paused. Pause is a record of operator intent and must be as durable as any other setting.

  **Distinct from `DisabledReason`**, which states a fact about the environment (backup needs SQLite) that no clicking changes: paused is an operator choice with a Resume control, disabled is not.
- **The existing loops are jobs.** The reconciler tick, the channel sweep, filler sync, and the session sweep are registered jobs (reading their existing interval keys `reconcile.every` / `channel.reconcile_every` / `filler.sync_every`, plus a new `job.session_sweep.interval`); their standalone ticker/retune plumbing is gone. Their *logic* (`Tick`/`Sweep`/`Sync`) is unchanged — only the loop driver moved. So they appear on the Tasks page and are Run-now-triggerable like any other job.
- **Availability jobs (§4, §6).** Poll-based availability runs as scheduler jobs: **`library-scan`** (incremental, default every 5m — `RecentlyAdded(since)` within `job.library_scan.lookback`) and **`library-full-scan`** (daily safety net — `AllItems()`). The scan confirms any in-flight (`requested`/`downloading`) title now present in the media server → `available`, correlating by `provision.Key`. This is the mechanism that replaces the retired inbound `/hooks/arr` webhook.
- **Arr queue poller (§6, arr provider only).** When `requester.provider=arr`, a **`arr-queue-poll`** job (default every 1m) reads each configured arr's `/api/v3/queue` and correlates records to in-flight titles by `provision.Key` (via the arr's lookup id). A title with a live download record is **`Grabbed`** → `downloading` (resetting the deadline), and its **download progress is persisted on the title record** — `progress` (0..1), `eta_text`, and `download_status` (the arr's own status string, passed through so a `warning`/`stalled` download reads as such rather than fake healthy progress). Persisting (rather than an in-memory cache) means `GET /v1/titles` reads progress straight from the store and it survives a restart. A grabbed-but-stalled title still ages out under the reconciler's deadline discipline (§4). Availability itself still comes from the library scan; the poller adds the grabbed transition + progress, never the `available` flip.
- **Seerr queue poller (§6, seerr provider only).** When the provider is Seerr, a **`seerr-queue-poll`** job (default every 1m) shares the same poller but a different source: Seerr exposes no download *queue*, so it cannot report a byte percentage. Instead one `GET /api/v1/media?filter=processing` returns Seerr's coarse per-title lifecycle enum, correlated to in-flight titles by TMDB id. `PROCESSING`/`PARTIALLY_AVAILABLE` are **`Grabbed`** → `downloading` and persist a **coarse `download_status` label** ("Downloading" / "Partly available") with `progress` left **0** (indeterminate — never a fabricated percentage); `PENDING` and `AVAILABLE` are not grabbed (`AVAILABLE` is the library scan's flip). Observed caveat: Jellyseerr's `downloadStatus` array *can* carry the arr's size/sizeleft, but it is empty on the deployments seen, so this path deliberately reads only the enum. The UI shows the label as the acquiring entry's chip text (no progress bar, since there's no percentage). Both pollers register mutually exclusively (a provider is arr XOR seerr).
- **Series episode refresh (§5, §9).** A **`series-episode-refresh`** job (default hourly) re-enumerates the shows whose cached episode lists have aged past `episodes.max_age`, so `series_episodes` stays current without any expansion happening on the request path. **Bounded to the shows referenced by channel lineups** — that set is small and known, and sweeping the whole library would cost more than the cache saves. Deliberately its own job rather than a hook on `library-scan`: that job only correlates *in-flight* acquisitions and returns early when there are none, so it would never revisit an already-`available` show (see §5).
- **Backup (§16, SQLite only).** A **`backup`** job (default `0 30 3 * * *`) writes one `VACUUM INTO` snapshot into `backup.dir` and then prunes that directory to the newest `backup.retain` files, matching only the `loomarr-<timestamp>.db` names it writes. Prune runs **after** a successful write, so a failed snapshot never costs the operator a backup they already had. On Postgres it registers as a **disabled job** (below): `WriteBackup` is SQLite-only, so it cannot run there — but an operator whose backup strategy is `pg_dump` should read that as a stated fact, not infer it from an absent row.
- **Retention purge (§5).** A **`retention-purge`** job (default daily) removes finished jobs past `JOBS_RETENTION` and denied proposals past `PROPOSALS_RETENTION`. **Proposals first, then jobs**, so the purge never creates an orphaned `proposals.job_id` (there is no foreign key to enforce it). In-flight work and the audit trail are both exempt: a `queued`/`running` job and an `approved`/`submitted` proposal are never removed by age.
- **Activity purge (§5, §12, V32).** An **`activity-purge`** job (default daily) deletes feed rows older than `activity.retention`. The feed is append-only on a busy install, so it needs a reader for its own retention key — declared and consumed in one PR, unlike `JOBS_RETENTION`/`PROPOSALS_RETENTION`, which are still read by nothing.
- **Disabled jobs.** A job may register with a `DisabledReason`. It appears on the Tasks page with that reason and is **never scheduled, never claimed, and refuses "Run now"** (`409`), so "cannot run here" is a property of the job rather than a UI convention a client could ignore.
  - **The alternative was silence, and silence is a claim too.** A conditionally-registered job simply vanishes, which is indistinguishable — from the Tasks page alone — from a job that runs fine and has never failed. For backup specifically, the failure mode of that ambiguity is an operator believing they are covered when they are not.
  - **Disabled is not "off".** It is not operator-settable and carries no enable control: it means *this build/backend cannot run this job*, which no amount of clicking changes. A per-job on/off switch would be a different feature.
  - **Not every absent job becomes a disabled one.** The queue pollers (`arr-queue-poll`/`seerr-queue-poll`) stay conditionally registered: exactly one is correct at a time and the other is *irrelevant*, not unavailable — listing it would advertise a job for a requester the operator is not using.
- **API + UI.** `GET /v1/jobs` (status list — each job's cron `schedule`, its settings `scheduleKey`, a human label, last/next-run, result) + `POST /v1/jobs/{name}/run` (trigger), **admin-only** (they expose acquisition internals — §7 authorization model). Schedule editing reuses `PATCH /v1/settings` (the cron key). The Settings → **Tasks** page renders the list with relative last/next-run, a status dot, and a Run-now button; a **"Modify Job" modal** edits the schedule — **human-readable presets by default** ("Every 5 minutes", "Daily at 3 am", …) each mapping to a canonical cron, with an **"Advanced" toggle revealing the raw cron field** for power users (validated live). A schedule not matching a preset shows as "Custom". A **disabled** job renders its reason in place of the schedule and offers neither Run-now nor Modify; `POST /v1/jobs/{name}/run` returns **409** for one, because a disabled control is a hint and the refusal has to live on the server.

---

## 19. Testing strategy
- **State machine:** every transition + the five invariants.
- **Store conformance:** one suite vs **both** SQLite (temp file) and Postgres (**testcontainers**), incl. `ClaimDue` concurrency (no record claimed twice).
- **Library conformance:** Emby vs Jellyfin flavors w/ mock transport; correct auth header each.
- **Webhook idempotency/replay:** duplicate/out-of-order events converge.
- **Scheduler reconcile:** desired-vs-actual against a **mock Tunarr** — idempotent (second reconcile = no-op), minimal-diff, and **backfill** (pending slot filled with filler → real title on `available` → re-push; `unavailable` → substitute). **Event-loss recovery:** drop the availability event entirely and assert the periodic sweep still backfills. Per-channel single-leader claim under concurrency.
- **Jobs:** `ClaimDueJobs` concurrency (no job claimed twice); a hung mock LLM hits `JOB_TIMEOUT` and the worker pool keeps draining other jobs; proposals and the approval queue survive a restart.
- **Lifecycle:** the downgrade guard refuses to start on a newer-schema DB; the janitor purges expired sessions/old jobs on schedule; `GET /v1/backup` (SQLite) yields a snapshot that restores to a working instance; deleting a scheduled item from the mock library → the sweep flags drift and substitutes.
- **Search:** `/v1/search` fans out to mock media server + mock TMDB + clip store; `in_library` flags correct; a member can search (read-only) but adding a missing title still routes through submit→approve; scope filters honored.
- **Suggestion grounding (critical):** mock LLM returns fabricated titles → **zero** unresolvable items reach a proposal, **nothing** unapproved reaches `/v1/titles`; already-present acquisitions filtered; `auto_approve` respects quota; output validates against schema.
- **Filler & pods:** catalog sync from a **mock media server's** filler library lands clips with duration + metadata; pod assembly is **seeded-deterministic** (seed = channel + window, so tests reproduce exactly) and respects era/audience matching, category variety, density, and no-repeat-in-window; the fallback ladder degrades gracefully to a bumper card; filler never appears as a lineup "program". Grounding applies to AI tagging and pod assembly (only real catalog clips).
- **Auth & roles:** bootstrap creates the first local admin and succeeds **exactly once** (a second call 409s while an admin exists); a **local** user logs in against its bcrypt hash; an **imported** media-server user logs in against a mock media server with the correct flavor header (MediaBrowser vs X-Emby-Authorization); an **un-imported** media-server user is **rejected even with valid credentials** (the allowlist — no lazy self-provision); passwords/media-server tokens never persisted; import is admin-only and creates rows, sync refreshes but **never adds**; `member` cannot hit approve/admin routes **or `POST /v1/titles`** (403 — the approval bypass is closed); disabling a user (directly or via sync of a server-disabled user) revokes their sessions immediately; `API_TOKEN` grants break-glass admin; ⚠ **an SSO identity with no allowlist row is rejected even with a valid provider token** (§11 V8 — the direct analogue of the un-imported media-server case), and no SSO login path creates a row.
- **Onboarding:** `GET /v1/setup/status` reports each integration pass/fail correctly against mocks (including the Tunarr media-source-matches-library check); a Sonarr/Radarr `Test` webhook with minimal payload is acked and flips the handshake check; a failing check carries an actionable hint + doc link.
- **API contract:** `/openapi.json` valid 3.1; served spec == committed `api/openapi.yaml` (fail CI on drift); spec `State` enum == code enum; `/docs` renders offline.
- **Frontend:** typed-client generation compiles; e2e smoke of approve flow vs mocked backend; SSE board updates on simulated `available`.

---

## 20. Open questions & follow-ons
Decisions formerly listed here now have v1 defaults baked into the doc (season precision → `series`, §6; pending-slot policy → pod-fill, §9; backfill placement → `stable`, §9) — all config-overridable. **Pre-publish decisions for the maintainer** (none block the build; Claude Code should use placeholders):
- **License** — pick before publishing (MIT/Apache-2.0 for permissive, GPL-3.0 if you want Jellyfin-style copyleft; Tunarr itself is Zlib).
- **Name availability** — verify `loomarr` is free on GitHub, Docker Hub, and isn't squatted in the Servarr ecosystem before announcing.
- **Go module path** — `github.com/<you>/loomarr`; agent builds against a placeholder until set.

Genuinely future work:
- **Direct *arr requester** as a Seerr alternative (adds real `Cancel` via un-monitor).
- **Local (non-media-server) accounts** and finer-grained permissions beyond admin/member (§11's v1 keeps two roles).
- **Notification agents** (email/Discord/webhook on approval, channel-live, give-ups) — Seerr users will expect these; v1 is in-app status only (§13).
- ~~**DB-backed settings UI** as an alternative to env-only config, if demand warrants the dual-source complexity (§13's wizard deliberately validates rather than stores).~~ **Resolved and superseded** by `config-design.md`: settings are DB-backed with `env > database > default` resolution, and the wizard **writes** through the same `PATCH` path as Settings (configure → validate → save → advance). The parenthetical above described the opposite rule and was dead text.
- **Vision-based filler tagging** (video models) beyond text-signal + transcript classification (§10). Transcript-based tagging (whisper.cpp) landed with compilation splitting (V34).
- ~~**Mid-roll ad insertion** via content segmentation, if Tunarr ever supports it (§10's honest limitation).~~ **Resolved:** internal playout (§9.1) owns the encoder and therefore the cut points, so mid-roll is in scope for internal-playout channels without waiting on Tunarr. See §10 "Break placement: a per-backend capability".
- **Second Programmer target** (ErsatzTV) once the Tunarr adapter is proven.
- **Leader election** if Postgres scale-out is needed beyond `SKIP LOCKED`.
- **Postgres `LISTEN/NOTIFY`** as a faster cross-replica availability signal (the periodic sweep already makes replicas correct; this would only cut backfill latency).
- **Full-text search index** (SQLite FTS5 / Postgres tsvector) only if enormous filler catalogs outgrow `LIKE` — §7.2 deliberately federates instead of indexing.
- ~~**Scheduled in-app backups with rotation** (§16 ships on-demand + documented cron for v1).~~ **Resolved** by V12: a `backup` scheduler job writes on `backup.schedule` and prunes to `backup.retain`, consuming two keys V4 had declared with no reader. See §16 "Backup & restore".
- **Per-channel time zones** (§9 uses the container `TZ` for all channels in v1).
- **Multiple media servers** (v1 assumes one `LIBRARY_URL`, per the design envelope).

---

## 21. Build plan for Claude Code (phased, verifiable)

Each phase ends green (compiles + its tests pass) before the next.

0. **Contract spikes (with the maintainer, against real services).** Before any product code: verify the risky external contracts against the maintainer's live homelab and **pin the evidence into the repo**. (a) Tunarr: exercise channel CRUD + lineup + filler-list calls against a throwaway test channel; vendor the spec to `api/vendor/tunarr-openapi.json` with the tested version recorded; settle the API-key question (§6). (b) Sonarr/Radarr: trigger real `Test`, `Grab`, and `Download/Import` webhooks; capture the JSON verbatim to `internal/testkit/fixtures/` with source-version comments — the phase-6 handler is written against these, not against memory. (c) Media server: one authenticated `AuthenticateByName` + `SearchTerm` round-trip per flavor available. **If any contract deviates from §6/§9, stop and update this doc before proceeding.** Deliverables: pinned spec, fixtures, and a short findings note in PROGRESS.md.
1. **Scaffold + build harness.** Module, `cmd/loomarr`, env config, `slog`, `/healthz`, **shared outbound HTTP client factory with per-service timeouts (§6)**, Dockerfile (distroless, non-root), compose skeleton (all profiles). **Harness:** `Makefile` target contract (`check`, `test`, `test-pg`, `openapi`, `openapi-verify`, `fe`, `e2e`, `dev`, `seed`), `.env.example` covering every §15 var, `internal/testkit/` skeleton (shared mocks for media server ×2 flavors, Tunarr, Seerr, TMDB, LLM + the Phase-0 fixtures), `PROGRESS.md`, and `CLAUDE.md` at the repo root. Repo layout:
   ```
   cmd/loomarr/            # main
   internal/provision/     # §3–§4 domain + reconciler
   internal/schedule/      # §9 channels, lineups, backfill, Programmer
   internal/suggest/       # §8 suggester + catalog tool
   internal/filler/        # §10 catalog sync + pod assembly
   internal/library/       # §6 Emby/Jellyfin adapter
   internal/store/         # §5 + migrations/{sqlite,postgres}/
   internal/testkit/       # shared mocks + Phase-0 fixtures (all tests use these)
   internal/api/           # §7 HTTP + OpenAPI wiring
   web/                    # §12 Vite SPA (built assets embedded)
   docs/                   # §13 markdown docs (embedded → in-app Help)
   api/openapi.yaml        # committed exported spec
   ```
2. **Provisioner domain + state machine.** Types, keying (+ webhook-key parity test), pure transitions + invariant tests. No I/O.
3. **Store + SQLite.** `Store` (titles, channels, **clips**, **users, sessions**, **jobs, proposals**), `modernc.org/sqlite`, WAL, embedded migrations + **startup downgrade guard**, `ClaimDue`. Conformance suite (SQLite).
4. **Postgres backend.** `pgx`, `postgres` migrations, `ClaimDue` via `FOR UPDATE SKIP LOCKED`; same conformance suite via testcontainers incl. concurrent claim. DSN-scheme selection.
5. **Library adapter.** Shared Emby/Jellyfin `Lookup` + `AuthenticateByName` + `ListUsers`, flavor header auth (incl. Jellyfin's MediaBrowser login header), `SEASON_PRECISION` (default `series`), flavor tests.
6. **Requester + availability.** Seerr requester (201/409 ok) or direct Sonarr/Radarr (§6); availability by **library scan** (poll, not webhook — §4/§18.1) + the arr queue poller (progress + grabbed); library-confirm-before-available, idempotency tests. *(The inbound `/hooks/arr` webhook this phase originally shipped has been retired in favor of polling — see the webhook-retirement arc.)*
7. **Provisioning reconciler + janitor.** Ticker → `ClaimDue` → retry `wanted`, library re-check (deadline backstop), deadline give-up + `Cancel`; retention sweeps (§5: sessions, jobs, proposals).
8. **Self-documenting API.** Huma v2 on `humago` (§7.1, §14); `/v1/titles*`, `/v1/events`, `/openapi.*`, `/docs`, ops; `GET /v1/backup` (SQLite `VACUUM INTO`); `make openapi` + committed `api/openapi.yaml`; contract tests.
9. **Users & auth (§11).** Session issuance/middleware, `/v1/auth/*` + `/v1/users*`, **local-admin bootstrap (once) + explicit media-server import (allowlist; un-imported → 403) + local bcrypt credential path**, user sync (periodic + on-demand; refreshes imported users, never adds), role enforcement on all mutating routes, `API_TOKEN` break-glass, login rate-limit. **Auth & roles tests are the gate.** *(The identity model was reworked from the earlier claim-on-login/lazy-provision design — see §11.)*
10. **Scheduler + Tunarr (the point).** `Channel`/`DesiredLineup`/`Slot`; Tunarr `Programmer` adapter; desired-vs-actual reconcile + **periodic sweep with slot revalidation** (`CHANNEL_RECONCILE_EVERY`, §9 drift + ownership + TZ); **backfill** consuming provisioning events (sweep-backed); basic Flex/filler-list plumbing; `/v1/channels*`. **Live TV wiring (§6):** `POST /v1/setup/livetv-connect` wires Tunarr as an M3U tuner + XMLTV guide source in the media server (idempotent enumerate-first), a `/v1/setup/status` "wired?" check, and a best-effort guide-refresh poke after channel-affecting reconciles (§9). **Maintainer-supervised live capture (Phase-0 style, folded here):** pin the accepted `/LiveTv/TunerHosts` + `/LiveTv/ListingProviders` request/response payloads and the guide-refresh task id from the real Emby/Jellyfin into `internal/testkit/fixtures/`; adapter written against the pins, not memory. Reconcile-against-mock-Tunarr tests **and the idempotent-connect second-call-no-op test** are the gate.
11. **Suggester (§8).** `Suggester` + Ollama and the OpenAI-compatible client (hosted OR Ollama's own `/v1`); in-app provider/model selection (§8.1: probe, catalog, hot-swap); catalog tool (library+TMDB) w/ tool-calling; grounding + validation; deterministic scoring; persisted jobs (store worker + `ClaimDueJobs`) + proposals + SSE; `/v1/proposals*` + `/v1/system/llm*`; expose Catalog as `GET /v1/search` (§7.2). **Grounding tests are the gate.**
12. **Commercials & filler (§10).** Catalog sync from the Tunarr `local` filler source (`/v1/filler/sync` + periodic); clip metadata + tag editing; pod assembly with era/audience matching, category variety, density, no-repeat, and the fallback ladder; optional AI text-signal tagging job; the in-core ingest job (yt-dlp/Archive → drop-folder), whose tooling ships in the single image (§16 — the `loomarr:filler` variant this line used to name no longer exists). **Filler-never-a-program + pod-matching tests are the gate.**
12.5. **End-to-end integration (the seams).** *Added after the first live smoke (2026-07-13/14) revealed that phases 0–12, each gate-green in isolation, had unwired seams between them — the per-phase unit gates never exercised the composition.* This phase makes "the whole thing works, driven only through Loomarr's own endpoints" an explicit gate, not an emergent hope. Scope = close every seam between an approved intent and a playing channel **with pods**, each proven against the live stack AND covered by an integration test:
    - **Approve → lineup carries acquisitions** (`#9`): a not-yet-available acquisition must enter the channel's lineup as a *pending* entry (key preserved) so backfill/sweep can place it — today `lineupEntries` drops non-in-library items, so acquired titles never appear.
    - **Provisioner availability events → scheduler** (`#10`) and **→ SSE `/v1/events`** (`#11`): wire the emitter adapters in `cmd/loomarr/main.go` (a `reconcile.Emitter` + ingest-handler emitter) that fan `DomainEvent`s to `engine.OnAvailability` **and** `eventBus.Publish`. Both are built on the consuming side and never called.
    - **Commercial breaks between programs** (§10 density): `ComputeDesired` must interleave pod-fill break slots at `FILLER_BREAKS_PER_HOUR` between *available* programs (today `SlotFiller` gaps are only created for *un*available titles, so an all-available channel gets zero commercials even with a full catalog). The pod assembler + `fillPods` already consume such gaps.
    - **Filler catalog present**: the manual smoke syncs a real (small) filler library so pods have content; the automated gate uses testkit clips.
    - **Playback sanity**: the channel plays through Tunarr AND is watchable from the media server's Live TV (rule out tuner-stream/format issues end-to-end, not just a raw Tunarr pull).
    Already fixed en route to this phase (live-smoke commits): `#6` channel-create binds the approved lineup, `#7` program duration resolution, `#8` in-library picks become `available` records, `#12` Tunarr content-id resolution (media-server id → program uuid), `#13` series expansion (a series entry → episode slots). **Gate:** an integration test (`make e2e` or a Go integration suite) that drives intent → suggest → approve → create channel → reconcile → asserts the pushed Tunarr lineup has real programs **with pod breaks**; plus the live manual smoke below. No phase-13 UI work starts until this is green — the UI must be built on a system that actually works end-to-end.
13. **Web UI + onboarding (§12, §13).** Vite React+TS per §14 (TanStack Query, orval hooks, Tailwind+shadcn/ui); typed client from `api/openapi.yaml`; Login + first-run wizard (bootstrap admin → `GET /v1/setup/status` checklist → import users → guided first channel), embedded channel templates, Channels, Board/My-proposals, Suggestion workspace (search-driven lineup editing) + admin approval queue, ⌘K command palette, Filler library, Users, Settings w/ re-runnable checklist, Help (embedded docs rendering); SSE; embed in binary at `/`. **Onboarding tests are part of this gate.**
14. **Docs, harden & ship.** Write the `docs/` set (§13: Quickstart, Integrations, Concepts, Member guide, Filler guide, Troubleshooting keyed to checklist items); finalize profiles (incl. ai/Ollama), runbook (§16), metrics/dashboards, README (record OpenAPI approach, LLM provider, FE delivery).

**Definition of done — automated (CI, against the testkit):** compose `sqlite` and `postgres` both start clean; store conformance green on both. A **member signs in** (mock media server), starts from a **template**, gets a **grounded** proposal (no unresolvable titles) → submits; an **admin approves** (`approved_by` recorded) → acquisitions enqueue and the reconcile issues the expected Tunarr API calls (mock) creating the channel **with matched commercial pods and filler**; a fixture Sonarr `Import` webhook flips titles to `available` and the scheduler **backfills** and re-pushes; `/docs` and in-app **Help** render offline; `/openapi.json` is valid 3.1 and matches committed `api/openapi.yaml`; the lifecycle suite (downgrade guard, janitor, backup snapshot, drift substitution) is green.

**Definition of done — manual smoke (real stack, maintainer-run):** against live Jellyfin/Emby + Tunarr + Seerr + Sonarr/Radarr (the Phase-0 environment): complete the first-run wizard with **all checklist items green** including the Tunarr media-source check and a real Sonarr **Test** webhook; run one intent → approve → **watch the channel actually play in Tunarr** with pods between programs. The end state is a **populated, self-maintaining Tunarr channel — with era-appropriate ad breaks — built from a sentence.**

> **Enforcement (learned the hard way).** This manual smoke is exactly the flow **phase 12.5** gates. The first live run (2026-07-13/14) proved that phases can be individually gate-green while the *integration* is broken — because the per-phase gates test components in isolation, never the composition. The lesson: **"gate-green" ≠ "works end-to-end"**; the DoD is only real once phase 12.5's integration gate passes. Prerequisite for Tunarr playback: Tunarr must have the media-server libraries **enabled + scanned** (§6) — an empty Tunarr program index fails all programming pushes.
