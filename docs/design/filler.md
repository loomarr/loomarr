# Filler

Formerly `design.md` §10: the catalog, sources, automatic fetching, storage, pulls, the clip
lifecycle, the media gates, media roles, conditioning evidence and acquisition runs. The per-clip
pipeline is in [filler-pipeline](filler-pipeline.md), compilation structure is in
[filler-structure](filler-structure.md), and tagging, screening and readiness evidence are in
[filler-classification](filler-classification.md). Break and pod policy is in
[scheduling](scheduling.md#breaks-and-pods).

Commercials, bumpers and station IDs are core to "feels like real TV". They are not titles and are
not in TMDB, so filler has its own sourcing pipeline and its own matching, separate from the
provisioning loop (decision [0006](decisions/0006-filler-is-its-own-pipeline.md)).

## Ownership and folders

- **Filler is Loomarr-owned.** Clips live in Loomarr's clip folder, identified by content hash, and
  programme content stays separate. A media-server library may be one **source** Loomarr copies
  from; it is never the catalog's only route, never played in place, never modified, and never
  offered to the suggester as programme material. An install with no media server, or with one
  that is down, still gets a full catalog from folders and remotes. "No media server, no
  commercials" must not come back.
- **Ingest runs in the core.** It is an ordinary job on the job bus (SSE progress, cancellable).
  `yt-dlp` and `ffmpeg` ship in the one image. The `FeatureIngest` gate resolves from the binaries
  being runnable, so a broken vendored binary degrades honestly (409 and the Filler empty state).
- **Tunarr-backed channels** read a nullable `tunarr_program_id` filled by the idempotent Tunarr
  `local` media-source sync; internal playout reads `path`. One catalog serves both, and an install
  with no Tunarr leaves the column empty.

**Two folders, one intake.** `filler.watch_dir` (default `<clip folder>/_watch`) is where clips
arrive, from downloads or by hand, and Loomarr drains it. `filler.dir` is the clip folder: every
clip lives at `a3/f9/<hash>.<ext>` with `<hash>.info.json` beside it. Every source takes the same
route:

1. arrive in the watch folder;
2. hash the file (the sparse content hash below);
3. move it into the clip folder as `<hash>.<ext>`; if it is already present, discard the duplicate;
4. write the sidecar;
5. catalogue the clip.

- The original filename is saved in the sidecar as `originalName` **before** the rename, because
  the filename is a grounding signal for era and brand. The tagger reads it from the sidecar.
- Loomarr rearranges only its own clip folder. The watch folder is drained (moved, not copied, so
  it never becomes a second copy of the library) and nothing outside the two folders is touched.
- A completed download runs the ordinary catalog sync and nudges the pipeline before its terminal
  success event, so new held clips are already in Incoming. A sync failure is an ingest failure.
- **The two paths are one layout, applied per application generation.** Changing either persists
  the desired layout and is restart-required. Changing it moves no data. A missing root may be
  created; a scan failure never becomes an empty scan that prunes the catalog; an unverifiable
  watch/clip aliasing check fails the generation closed. Tunarr registration uses the operator's
  shared-volume spelling of the root; local traversal uses the resolved one, and folder sources
  that alias, contain or sit inside the clip root are skipped.

## Clip identity

A clip is identified by a hash of its bytes; its path is where it lives (decision
[0016](decisions/0016-clip-identity-is-a-content-hash.md)).

- **`id` is a sparse content hash:** the first 64 KB, the last 64 KB and the file size. The API's
  wire identity is the hash too (`ClipDTO.hash`; byte routes take `{hash}` and resolve the path
  server-side), so a path never crosses the wire.
- **Duplicates are catalogued once**, first scan wins. Deleting the winning copy lets the survivor
  return with the same id and its tags intact.
- **A sparse hash is a heuristic.** Two files that share size, head and tail but differ in the
  middle shadow each other. That is tolerable for a synced cache, and it is documented rather than
  silent. Conditioning and reuse boundaries compare full byte streams (below).
- **A file changed outside Loomarr is a new clip.** A transform Loomarr performs inside the
  pipeline files the new bytes at their own hash and atomically re-keys the row, tags, pipeline
  state and lineage. The old hash never names bytes it no longer identifies.
- **Legacy hash-named files repair themselves at sync.** A file whose stem is an old
  machine-generated hash but whose bytes hash differently is refiled at its canonical path without
  overwriting, and a hash-shaped display name is replaced from the durable catalog name, then from
  grounded facts, then with a neutral "Untitled commercial". Operator filenames are never
  "repaired".
- **An identity change drops the catalog** with a forward-only migration (`00033` did this), since
  filler is a synced cache. Sidecars restore tags; play counts and pins do not survive, and the app
  says so.

**Sidecars carry metadata with the clip.** Loomarr writes era, audience, category, kind, the
diagnostic score and `originalName` into the existing yt-dlp-style `*.info.json`, so tagging
survives a database reset or a move. Loomarr may create or update only `*.info.json` in the media
folders; media files stay byte-for-byte untouched. JSON rather than `.nfo`: nothing consumes `.nfo`
here, and its schema has no place for these fields. The sidecar's `"fetchedBy": "loomarr"` is
portable provenance, never admission authority.

**Download manifests are the quarantine authority (V65).** Archive and yt-dlp publish through one
manifest boundary. Per output it records acquisition and source identity, provider, remote item
id, source URL, media and sidecar paths, byte length, full SHA-256, completion time and state, and
the application persists it before the file is eligible for intake.

- Bytes stay in run-owned hidden staging until the manifest is durable. Publication records the
  intended watch path before the rename, so recovery can tell "not published", "published but not
  acknowledged" and "consumed".
- A row matches by exact path and digest; a symlink, escape, size or digest substitution is a held
  repair, never a match. After intake the row binds the clip hash.
- If durable acquisition state claims the bytes, the clip is held. A missing or damaged sidecar
  cannot weaken that; unprovable bytes stay quarantined with a repair reason and are never deleted
  for it.
- Operator drops keep their own policy: intake never claims unmanifested files, and a copied
  third-party sidecar cannot create a manifest.
- Provider deduplication advances only after the manifests are durable. Startup recovery is
  idempotent, counts each manifest once, and keeps repair counts visible after newer runs.
- A consumed manifest is the acquisition planner's `already_catalogued` mark; staged, published
  and repair manifests are `already_queued`.

## The catalog

Each clip carries the metadata that lets the scheduler place it:

- `kind`: `unclassified`, `commercial`, `bumper`, `station_id`, `psa`, `trailer` or
  `interstitial`. `unclassified` means no exact role is established; it is descriptive, not a
  lifecycle state, and an unknown filename defaults to it, never to `commercial`.
- `era` (decade or year), `audience` (`kids`, `family`, `general`, `late_night`), `category`,
  and `brand` (free text, grounded).
- `duration` from Loomarr's own `ffprobe` scan, `rating`, `source`, and a persisted `transcript`.

**Tags come from a grounding ladder**, cheapest and most trusted first, each tier running only
where the ones above left a gap. A tag is a fact only when its signal literally contains it.

| Tier | Signal | Cost | Catches |
| --- | --- | --- | --- |
| 0 | filename or folder convention | free | eras and kinds encoded in names |
| 1 | source sidecar text (title, description, uploader) | free | clips that describe themselves |
| 2 | LLM over the text signals, giving era, audience, category and brand | cheap | text-described adverts |
| 3 | Whisper transcript, fed into tier 2 | moderate | adverts that speak their brand |
| 4 | frame heuristics (black-and-white, aspect ratio) | free | era hints without usable text |
| 5 | vision LLM over keyframes | expensive | silent adverts and on-screen logos |

- **Era must appear literally** in the filename, sidecar text or transcript. Otherwise it is
  stored as an unconfirmed era the operator confirms (`PATCH /v1/filler/tags`), on every tagging path.
- **Transcription is selective:** only when the source description is thin or a text-only pass
  left the clip unidentified. It is local by default; a connected service is an explicit opt-in.
  Transcripts persist to the store and the sidecar.
- **Vision** uses a separate `AskAboutImages` call (hosted `image_url` parts) and Ollama's
  per-message `images` locally. Keyframes come from `ffmpeg` stills. Vision JSON is field-tolerant
  and evidence-strict: a wrong-typed optional field is dropped without discarding the rest, a
  numeric-string era is accepted, and invalid JSON is a retryable provider failure. Ollama vision
  requests use native JSON mode.

**Removing a clip is a tombstone.** **Remove from catalog** marks the row removed; it deletes
neither the row (the next scan would re-create it) nor the file. The scan's upsert may not clear
the mark. Removed clips are excluded from the listing and pod assembly by default, so no caller
has to remember a flag. Restoring clears the timestamp.

## Sources

Every source is one row in one list (decision [0014](decisions/0014-sources-are-one-flat-list.md)):
kinds `folder`, `library`, `archive` and `youtube`. Each kind declares whether it is searchable,
fetchable and operator-removable.

- **Uniqueness is on the target**: one row per distinct path or library id. Any number of folders
  and libraries may be added. `filler.dir` is the first folder and the default download target, not
  the only one scanned.
- **"Not configured" stays expressible.** The config-backed seed rows are present even when unset,
  with a blank target and a `not configured` badge, because that answers "why is my catalog empty?".
- **A fresh install seeds sources but downloads nothing.** Three Archive.org collections seed with
  targets verified against the live API (`classic_tv_commercials`, `vhscommercials`, `tv_ads`),
  each with a human-readable label. The YouTube row seeds empty: Loomarr never recommends YouTube
  content itself. `packs` is not a kind until a real pack index exists.
- **A disabled source is not scanned, fetched or searched, and cannot enter a pull**, re-checked at
  approval. Its clips stay. The switch is enforced at the scan and fetch sites, not by dimming a
  row. It does not bind an admin's deliberate one-item actions (global discover search, a typed
  `POST /v1/filler/ingest` URL).
- **Providers have master switches** (`filler_providers`, keyed by `kind`). The source projection
  owns `effectiveEnabled = provider.enabled && source.enabled`; pausing a provider keeps each
  child's saved switch and blocks enumeration, search, fetch, pull proposal, approval and
  registration of new targets. The read model shows both values ("Paused with Archive.org").
- **Grouping is derived from `kind`**; there is no `parent_id`. The wire is a flat pre-ordered
  array with `group` and `parentId`. Fetch overrides apply only to leaves, and a provider's
  `lastCheckedAt` is a read-only maximum over its children. `folder` and `library` do not group.
- **Licence metadata is passive provenance.** It is recorded when the provider supplies it; absence
  means "not provided", never public domain. It never ranks, filters, holds, rejects or admits, and
  it stays off the Sources UI.
- **Geography is inherited.** A source without explicit coverage uses the Installation geography,
  live. An explicit country and market is an Advanced override. The read model reports the
  effective geography, whether it is inherited, and one readiness state (`ready`, `off`,
  `needs_location`, `not_configured`, `out_of_area`). Candidate geography stays a hard acquisition
  constraint, and Installation or inherited geography is never copied into a clip's location.
- **Attribution gap.** The folder scan writes `Source = "filler-dir"` for every clip and the
  sidecar does not record which remote source fetched it, so per-source clip counts are sums of
  what children claim. Per-source attribution is an intake change, tracked separately.

**Finding a source is not adding it (V67).** A search hit is a transient provider result;
resolution verifies typed input and returns one canonical target; only `POST /v1/filler/sources`
registers. Search hits and resolutions create no row, download nothing and grant no authority.

- One provider-neutral finder owns the contract. Archive.org searches `mediatype:collection` and
  resolves through `/metadata/{id}`, refusing non-collections. YouTube uses yt-dlp in flat,
  listing-only mode, collapses hits by channel and resolves to `/channel/{id}/videos`; a video-only
  URL is refused as a source. Both fail closed when the provider is paused.
- Resolution returns at most three example items. They stay text-only until the operator asks for
  the provider's own player; **Open original** is always available. Archive examples are video
  items ordered by download count, which does not change what acquisition may inspect.

**The Sources page.** *Your files* comes first, then one section per provider with its master
switch and one search-or-paste finder. Registration is an explicit action after the exact target
is shown; **Cancel** discards the selection and a late response cannot restore it. Turning a
provider off folds its body closed without changing child switches. A registered row shows its
switch, name, one status and one way into the source workspace.

- The workspace is the application Sheet at `/filler/sources/{sourceId}`; direct links, reload and
  Back/Forward restore it, and a stale id returns to the index. It holds the check outcome,
  provider link, the three-item preview, the clip browser, the location exception, cadence and
  limits, and removal. A target is shown read-only; re-pointing is remove and re-add.
- **Look for new clips** is the manual action (see fetching below). Archive's **Find a specific
  clip** starts collapsed; its **Queue download** keeps the parent source and item identity and
  moves through Queueing, Downloading, then **Added — being checked** or **Couldn't add**, with the
  acquisition GET authoritative after reconnect.
- Row counts separate ready clips from the Incoming conveyor ("0 ready · 12 being checked"), using
  Incoming's definition. Disabled rows are greyed. At ten sources a section adds a filter; without
  one it shows every problem row first and five healthy rows behind **Show N more**.
- The page header pill reads sources on, clip count and last scan time. There are no Sync or AI-tag
  header buttons; the Tasks page runs any scheduled job now.

## Automatic fetching

An enabled source is checked on its effective policy and new items download without anyone asking
(decision [0015](decisions/0015-sources-fetch-on-their-own.md)).

- `filler.fetch.every` is the only user-facing cadence. A per-minute internal planner makes no
  provider request unless a source is due by its durable `last_checked_at`. A successful listing
  records the check even when it finds nothing. Failed listings back off 1, 5 and 15 minutes, then
  hourly, and success clears the backoff. The row projects its next eligible check.
- The planner leases a source for at most 30 minutes before a provider request, so a scheduled
  pass and **Look for new clips** cannot enumerate it twice, and a stale worker cannot complete a
  newer claim.
- **Per-source overrides** of interval and count are nullable, and NULL means inherit (`0` already
  means "never"). Advanced source settings offer: use the defaults, use a different schedule, or
  never download automatically. Reset clears both overrides. Filler → Manage presents the global
  policy as **Automatic downloads** (Never, 6 h, 12 h, daily, weekly, custom) and states the
  consequence ("3 sources means at most 30 new clips per check").

Unattended fetching can fill a disk, so it is bounded by limits that fail toward doing less:

| Bound | Default | Why |
| --- | --- | --- |
| `filler.fetch.every` | `6h` | default interval; `0` stops inheriting sources; custom values run from 1 minute to 7 days |
| `filler.fetch.max_per_run` | `10` | items one source may pull per check, so a huge collection trickles in |
| `filler.fetch.max_catalog_clips` | `2000` | whole-catalog ceiling; at the limit auto-fetch stops, while manual queueing and approved pulls still work |
| `filler.storage.library_budget_gb` | `0` (automatic) | soft allowance for managed filler media (below) |

- One pass queues at most **50 items per provider**, a fixed internal protection. Sources left due
  are taken next pass, and a provider cannot starve others by restarting from its newest items.
  The catalog and disk ceilings stay global: the operator is protecting one disk.
- **YouTube enumeration is newest-first and checkpointed.** A new source examines at most the newest
  100 entries. Each check records the last stable item examined and the newest item seen at sweep
  start, resumes after the former, and commits the watermark on reaching it, exhaustion or the
  lookback. Exact identity deduplication handles inserted entries; a missing cursor restarts the
  bounded sweep; failures do not advance the checkpoint.
- Before download, YouTube entries are rejected when duration is unknown, shorter than
  `filler.min_duration`, longer than `filler.autosplit.max_duration`, or live, upcoming, private,
  unavailable or incomplete. Within one sweep, titles that differ only by a duration suffix prefer
  the fuller cut; unsuffixed titles never collapse on text. Archive keeps its metadata-tolerant
  behaviour. Each successful check persists a closed summary of outcomes (queued, known, too short,
  too long, live, upcoming, private, unavailable, incomplete).
- **Archive.org and YouTube are peers.** Each registered remote is enumerated by its stored `kind`
  (Archive's bounded collection API; yt-dlp flat-playlist listing) through enumeration and download.
  URL inference is kept only for a URL an admin typed. Both share the same bounds, governor,
  acquisition record, held lifecycle, sidecar and admission authority.
- **Look for new clips** runs one bounded fetch pass for the selected source and then scans local
  sources. It may run before the source is due or with automatic timing off, but it keeps the
  source and provider switches, geography, deduplication, active claims and every ceiling. The
  response names the source and the result ("Checked Classic TV Commercials — 2 clips queued").

Three properties are not negotiable:

1. Only registered, **enabled** sources are polled.
2. Everything fetched arrives **held** and converges automatically; registration or an explicit
   one-item queue is the household's enrollment authority, and optional classification creates no
   second approval gate.
3. A limit that is reached is **reported**. The filler status read measures the live catalog and
   the governor measures the disk, so "nothing new arrived" is answerable after a restart, and the
   warning clears as soon as there is room.

A bulk backfill of a large collection is a pull's job, where a human approves the plan.

## Storage is reserved before Loomarr writes

One `storagegovernor.Governor` sits before every Loomarr-managed media write. Callers supply a
conservative estimate; the governor owns filesystem identity, capacity, managed usage, in-flight
reservations and the operator-facing decision.

- **Hard free-space reserve:** `clamp(10% of filesystem capacity, 2 GiB, 20 GiB)`.
- **Automatic filler allowance:** `min(10% of capacity, 20 GiB)`; a positive
  `filler.storage.library_budget_gb` replaces it. Availability is the smaller of the remaining
  allowance and free bytes above the hard reserve, less active reservations on that filesystem.
  Lowering the allowance pauses new automatic work and deletes nothing.
- A confirmed manual import may exceed the soft allowance, never the hard reserve. An unknown
  estimate or capacity is never permission to write.
- `Reserve(estimate)` is atomic per filesystem and returns a lease or one pause reason:
  `library_limit`, `host_reserve`, `estimate_unknown`, `capacity_unavailable`. Leases are taken
  before queueing, revalidated before execution, charged as output grows and released on success,
  cancellation or failure. Writes have a byte ceiling, so a dishonest provider length cannot
  overrun.
- An acquisition lease covers its staging ceiling, `source + max(source/4, 32 MiB)`. Transcode,
  split, prepared media and artwork reserve their own peaks when they run. Leases are per item and
  released as each download ends. An unsizable item is planned from its duration and height, else a
  512 MiB cap.
- Reservations group by filesystem, not path. The hard reserve aggregates every domain on the
  device; soft budgets stay per domain. A cross-filesystem move reserves the full copy first.
- Restart forgets leases but not bytes. Automatic cleanup removes only cancelled or incomplete
  private staging, superseded temporary output and unreferenced generated artifacts under a
  retention contract; never admitted clips, pinned media, review evidence, source masters, symlinks
  or unknown files.
- The server projects one capacity snapshot with a state (`healthy`, `approaching`, `paused`,
  `unknown`) and the actions `free_disposable_space`, `choose_another_folder` and
  `change_storage_limit`. `approaching` starts at the smaller of 1 GiB and 20% of the allowance.
  The browser does no capacity arithmetic.

**Two downloaders, two gates.** Archive.org needs only `ffmpeg`; YouTube needs `yt-dlp` and
`ffmpeg`. Availability is reported per source, never as one blanket verdict, because a missing
`yt-dlp` once blocked Archive downloads that never use it.

## Pulls

A **pull** is a plan Loomarr composed across sources, persisted in the approval queue beside title
proposals, and nothing downloads until it is approved (decision
[0013](decisions/0013-filler-pulls-and-tombstones.md)).

- It carries a title, proposer, rationale, plan rows (each droppable before approval), an aggregate
  estimate and an optional annotation-only note. The note never changes what downloads.
- **Approval enqueues through the ordinary ingest path.** A pull whose sources are all off is
  refused with the switch to flip. Dropping a row before approval is part of the gate.
- **The gate binds bulk composition, not an admin's own hands.** Queueing one searched clip stays
  direct.
- **A decision commits once.** Approval compares-and-sets the pull from pending, records the
  reviewed plan, note, actor and time, and creates one queued acquisition run in one short
  transaction; the downloader starts only after commit. A losing approval or dismissal gets a
  conflict. A pending pull that already has a historical run is refused for approval (it may be
  dismissed). On startup, runs left queued or running become visible acquisition errors, never
  replayed.

**Acquisition intent chooses exact remote items (V66).** A pull starts from a versioned intent:
desired roles, era observation range, audience, geography, maximum duration, missing taxonomy axes,
source allow-list, representation quality floor, item count and the catalog reason. An omitted
constraint means not requested; an unknown candidate field is unknown, never a match; an upload
date never becomes an era.

- The default intent comes from the same `PoolReport` and per-channel coverage the Filler overview
  shows. Deterministic policy owns selection; a model may suggest terms but cannot relax a
  constraint, invent metadata or cross approval.
- Planning is metadata-only: 90 seconds, at most 12 sources and 100 candidates. Remote identity is
  `(provider, registered source id, provider item id)`; URLs are payload.
- Every source gets a disposition (`enumerated`, `disabled`, `not_fetchable`, `not_allowed`,
  `geography_mismatch`, `source_limit`, `enumeration_failed`) and every item gets one, selected or
  a stable exclusion code (already catalogued or queued, previously declined, duplicate, or an
  unknown or mismatched era, duration, quality, role, audience or taxonomy, or ranked below the
  limit). Ranking is deterministic: fitness, representation quality, diversity, then identity.
- Approval revalidates each candidate's source, policy and novelty, then hands the **candidate
  URLs**, never the collection URL, to the manifest-backed ingest path.
- Scheduled acquisition adopts this selector only after parity tests; there must not be two ranking
  algorithms. Intent chooses promising evidence; it does not certify it.

**A starter pack fills a new install's first breaks.** `GET /v1/filler/discover?collection=<id>`
lists a curated Archive.org collection; the operator keeps or excludes rows and only the survivors
are fetched through the ordinary ingest path. It is a listing, not an acquisition; it is the
discovery path with a different argument, not a parallel one; and it is never required ("Start
from scratch" is always offered, and a vanished collection degrades to an empty list with the
reason). The starter collections are product-curated seeds that may change with a release, not
operator configuration.

## The clip lifecycle: held, then Ready

- **held:** recorded but not playable while required runtime work is incomplete or failed.
- **Ready:** exact playable bytes, enrollment authority and placement were committed together by the
  terminal-ready operation. Only Ready clips enter a pod.
- **not usable:** an objective failure, a positive non-filler finding, a composite container or
  removal.

Enrollment authority comes from enabling a source, opting a folder or library into filler, or
queueing one item. It is captured when the clip enters the conveyor, so disabling a source later
stops future work without rewriting outcomes. Nobody is asked for per-clip approval because an
optional classifier could not name the role.

**Role and placement are separate.** A commercial, promo, PSA, trailer or interstitial places in the
break body; a bumper or station ID places as a bookend. Enrollment grounds break-body placement for
an unclassified clip without relabelling it. Composites and programme excerpts are not playable. An
explicit channel kind filter still narrows to known roles.

The terminal operation atomically stores placement, clears the hold, settles the conveyor as Ready
and appends the activity outcome. Retries are idempotent; stale identity rolls back.

**Incoming** is `GET /v1/filler/incoming`, three disjoint groups: **preparing** (machine-owned rows),
**needs help** (current split choices backed by split confirmation) and **recently ready** (Ready
within `filler.incoming.ready_window`, default `24h`, one hour to 30 days, hot-applied; the resolved
window is returned with the projection). Each group is counted by the predicate that returns its
rows, and row arrays are capped at 100 with separately counted totals. Ready clips age out of
Incoming but stay in the Library. Rejections, retries, provider failures and optional enrichment
belong in Activity or Diagnostics. Bumpers, station IDs and compilations are never "add tags"
tasks.

A row carries one server-owned sentence. Its panel may reveal **Processing details**: the server's
ordered stages, each with a name, outcome, time and safe explanation, routine skipped stages behind
**Show skipped steps**. Only the active stage may carry progress: an exact 0–100 value when
measured, indeterminate otherwise, never percent Ready or an ETA. The projection never exposes a
private path, raw error, transcript, model response or credential.

## Media gates

Objective failures are handled automatically, with no badge or decision. Only content anomalies
that may be intentional go to a person.

**Rejected at the scan boundary**, never catalogued: shorter than 10 seconds, no audio stream, or no
video stream.

**The transcode pass inspects what it already decodes.** `blackdetect`, `silencedetect` and
`freezedetect` ride the mezzanine encode's filter chains and their intervals go into the sidecar.
An older mezzanine without a report gets one bounded inspection-only pass (the marker stops a
re-encode), within the ordinary transcode budget.

- At least 90% black or 90% silent is a hard reject, recorded with its coverage.
- A black or silent span of at least 5 s, or a frozen span of at least 10 s, goes to review.
  Freeze alone never auto-rejects.
- Review holds the clip out of rotation before the review row commits; a store failure leaves the
  row runnable and the persisted report re-emits the verdict without decoding again.
- Intervals are unioned and clamped to the probed duration; an open trailing event closes at the
  duration.

**Loudness is measured at ingest and applied at playout**, target `filler.target_lufs` (−23 LUFS).
Normalisation at playout is the default: reversible, one filter on a stream already being encoded.
On-file normalisation is an opt-in (`filler.conditioning.normalize_loudness`, default off) that
writes the playback derivative with `loudnorm` at the **same** target and records `normalizedLufs`
in the sidecar so it is never repeated. Playout still normalises, so older clips stay consistent.

**Brightness is measured by nothing and fixed by nothing.** A dim VHS transfer is what the source
looks like, and unlike loudness a correction could not be undone at playout.

**Language is a background job that rejects** confident non-target speech (maintainer,
2026-08-03). It runs after the clip is catalogued; inline would turn a folder scan into hours on
arm64.

- **Silence never rejects.** A wordless visual spot has no language and is often the best filler.
- A model handed silence guesses arbitrarily (a live run tombstoned two clips after sampling tape
  leader at −70 LUFS). So an ordinary commercial is inspected in full and longer recordings use the
  middle 30 seconds; a span below −50 LUFS (`ebur128`) is never asked about; and both backends need
  at least one non-empty transcribed segment before accepting a language.
- **One speech-recognition choice serves language and transcripts:** `asr.provider = whisper`
  (default; vendored `whisper-cli`, free, offline) or `hosted` (an OpenAI-compatible
  `/audio/transcriptions` endpoint with `verbose_json`). `ASR_URL`, `ASR_MODEL` and
  `ASR_API_KEY[_FILE]` may name a dedicated service; a blank URL reuses the hosted AI service, and an
  explicit speech URL never inherits the main AI key. Hosted audio leaves the house and may cost
  money, so it is opt-in. Ollama has no audio input.
- **An unavailable backend is a skip, not a retry.** The rung records why and the clip advances;
  backoff is for a backend that ran and failed. The Filler settings show the same reason beside a
  disabled expected-language control. On arm64 local Whisper is too slow to be useful, so the
  feature is off by default there.
- The seam is `MediaTools.Transcribe(ctx, file, startMs, endMs)`; hosted is a second implementation
  behind it.

## Source evidence and playable media

A convenient rendition is not source evidence, so every clip that reaches transcode has three media
roles (decision [0028](decisions/0028-source-evidence-and-playable-media.md)):

| Role | Use | Contract |
| --- | --- | --- |
| **source master** | exact acquired or supplied bytes; provenance, reprocessing, recovery | immutable, identified by full SHA-256 and byte length, never replaced |
| **evidence derivative** | inspection, segmentation, OCR, transcription, model input | reproducible from the master under one versioned recipe; no loudness or cosmetic change |
| **playback derivative** | what Tunarr or internal playout reads | H.264/yuv420p, AAC stereo 48 kHz, fast-start, bounded GOP; optional loudness policy lives here |

- The master is copied and hashed into `.loomarr-media/masters/<sha[0:2]>/<sha[2:4]>/<sha><ext>`
  under the filler filesystem before any derivative publishes; the scan never treats it as
  playable. Collisions, non-regular objects, escaping paths and incomplete copies fail closed, and
  every reuse re-verifies the bytes.
- The playback sidecar carries one versioned media-asset manifest binding the master and each
  derivative's role, input and output digests, recipe id and digest, tool identity, duration,
  streams and QC. Reuse requires every fact to match and the file to re-verify; paths are never
  identity. A rebuild recovers these relations without a database or repeated analysis.
- A consumed acquisition's master digest and its playback clip hash are verified separately; they
  are never compared with each other.
- **Archive picks a source representation**, not the cheapest file: originals over derivatives,
  then complete duration and dimensions, larger dimensions, plausible bitrate, byte length and
  filename. Unknown facts never beat observed ones, and the choice travels in provenance.
- **Recipes say exactly what changed.** The evidence recipe normalises timestamp origin and selects
  one video and one audio stream, and does nothing else implicitly. Any restoration transform needs
  its own recorded recipe and measurement. Both derivatives are built from the master, staged,
  fully decoded and checked, then atomically named; a failed check holds the clip and keeps the
  master.
- There is no automatic master garbage collection; masters stay until a complete ownership graph
  can prove one unreferenced and regenerable.

## Conditioning evidence for split children

Split children are measured, not trusted (V64, V65). Structure decisions are in
[filler-structure](filler-structure.md).

- **Measurement is evidence, never authority.** The media-tools inspector measures one bounded local
  artifact, optionally against one parent and up to eight intended cuts: container and per-stream
  duration and start, exact video cadence, A/V start and end skew, integrated loudness and true peak
  (a closed finite or digital-silence state), black, silence and freeze intervals, and signed
  per-stream edge errors against each cut. Unavailable is a first-class result, never zero. It has
  no verdict, threshold, rewrite or admission authority.
- **Its limits are part of the interface:** each input at most 1 GiB and 120 seconds, at most eight
  streams and eight cuts. Inputs are opened once without following links in any path component and
  snapshotted, so every tool reads the same bytes. Tool output must match the exact anchored
  detector and loudness grammar; anything malformed, duplicated or out of shape fails closed.
- **Lineage is written beside every child** at split confirmation: parent content identity and the
  reviewed interval. The transcode rung measures the cut bytes before rewriting and the staged
  replacement after, both against that interval, and the sidecar carries lineage and both
  measurements. A rebuild restores the parent relation from the sidecar; valid child lineage is what
  marks the parent composite, independent of scan order.
- **Publication is staged and reversible.** Confirmation snapshots the parent, cuts and sidecars the
  whole generation in hidden staging, and publishes held, tombstoned, non-runnable children. One
  store transaction files the parent, activates the child pipelines, consumes the proposal and
  switches generation, under a durable expiring proposal claim with a fencing token. Any failure
  removes the new media before its sidecar; a failed removal keeps the evidence as quarantine.
- **A pending re-key is a hold.** A conditioned replacement's sidecar carries a `pending` record
  until the catalog re-key commits, and the pre-rewrite sidecar records `supersededByHash`, so a
  crash or a failed cleanup never leaves an airable duplicate.
- **Full byte comparison at every reuse boundary.** Matching sparse ids never proves two artifacts
  equal.
- **Strict shape at the application boundary.** A conditioned child needs exactly one audio and
  one video stream, complete timing, cadence, skew and loudness, and one cut comparison with every
  edge available. Post-rewrite parent edges are derived by checked integer arithmetic in media-tools
  (`derivedParentEdgesAfterRewrite`), because re-encoded packets cannot be matched to the parent.
  Anything missing holds the child for review; cancellation is checked at every boundary and
  follows the same path. `normalizedLufs` is only a target marker, never loudness evidence.
- **The sidecar is the restart record**: a child with complete valid evidence is not decoded again;
  damaged or incomplete evidence holds.

## Acquisition runs and filler readiness (V59)

- **Every accepted download creates a durable acquisition run** first, recording trigger, source or
  pull, counts and state. Its id travels in the sidecar, through splitting, into each pipeline row,
  so reconnecting clients read history from the store; SSE is only a hint. A run that cannot be
  persisted does not start. Startup marks runs left active as interrupted errors.
- Acquisition work is owned by the application generation: request disconnect does not stop it,
  shutdown cancels and awaits it. Compilation detection records a durable operation keyed by the
  `jobId` from `POST /v1/filler/split`.
- **The Filler overview is one server-owned readiness projection** from fetch limits, the pipeline
  overview, the playable pool, per-channel coverage and recent runs. It returns one next action, in
  order: repair a stopped acquisition path, retry failed machine work, make pending decisions, fill
  an empty pool, improve the weakest channel, or nothing. Clients render it and never reconstruct
  health from counters.
- A channel's Filler section shows its saved coverage first. Per-clip overrides stay collapsed as
  **Prefer on this channel** and **Exclude from this channel**. Every real clip in a break preview
  plays through the content-hash media route; preview never stitches a pod asset.
- Clip details show the source's label, with the id kept for transport. File facts come from the
  sidecar's validated playback lineage, never from `ffprobe` during a read.
- Acquisition is not readiness: none of this weakens enablement, limits, grounding, required checks
  or the held-to-Ready transition.

## AI assist

Optional and opt-in, under the suggester's grounding rule (only clips that exist in the catalog):
classify and tag ingested filler, and assemble pods matched to a block while flagging gaps such as
a channel with no toy adverts from its era.
