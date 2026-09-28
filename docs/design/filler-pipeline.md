# Filler ingest pipeline

Formerly `design.md` §10's V51b, V51d, V51e and V51g sections. Every arriving clip goes through one
ordered, watchable per-clip pipeline (decision [0018](decisions/0018-one-ingest-pipeline.md)) under
one `filler-pipeline` driver job. Admission rules and readiness evidence belong to the filler
classification docs; this doc covers how work flows.

## Readiness stages

`probe → transcode → split → screen → language → transcribe → vision → score`.

- Each stage answers two questions separately: does it apply to this clip in this install (no exec,
  re-evaluated while the clip is on the conveyor), and then the work. A missing optional capability
  records a **skipped** rung and never blocks Ready.
- **Sequential and budget-bounded.** One clip runs at a time: transcription is slow and ffmpeg
  competes with playout for the GPU. Per-run budgets (`MaxClips`, `MaxTranscodes`, `MaxWhisper`,
  `MaxVision`, `MaxSplits`) let a backlog drain over cycles.
- **Retries** back off 5 min, 30 min, 2 h, then resolve: a `probe` failure rejects (an unmeasurable
  file is not a clip); `transcribe`, `vision` and `language` skip and advance, so a missing
  transcript never strands a clip.
- **Running out of time is a deferral, not a failure.** A deadline returns the clip to the queue with
  its progress kept; any rung that treated a timeout as failure would loop forever.
- **Recovery rewinds the dependency suffix.** An admin may restart at the failed stage once the cause
  is fixed: that rung and everything downstream are cleared, retries reset, and a force-run marker
  persists across restarts. Upstream measurements and operator tags survive. A transcode rewind needs
  an explicit force flag because it replaces playable bytes.
- **Lifecycle is a domain answer.** `filler.Pipeline` projects persisted state into one vocabulary for
  the runner, Incoming and telemetry: `runnable`, `in_progress`, `scheduled`, `needs_decision`,
  `ready`, `complete` (a processed composite container, not playable), `rejected`, `dismissed`. A
  failed rung carries a stable failure code, the retryable rung and one sanctioned recovery action.
  Retrying a terminal execution failure and clearing its tombstone is one transaction, and the clip is
  held before it reappears.

State lives in **`filler_clip_pipeline`, a sibling of `clips`**, because `clips` is a synced cache
that has been dropped and rebuilt; pipeline state records work already paid for. The finished ladder
is one JSON document (`stages_json`); `stage`, `status`, `disposition` and `next_run` are columns
because the work list and Incoming filter on them.

## Budgets are per clip

The scheduler's unit of work is a clip, so **a rung may not spend per segment what the budget allows
per clip** (decision [0019](decisions/0019-stage-budget-is-per-clip-rate.md)).

- Splitting cuts; it does not describe. Each segment re-enters the pipeline as its own clip and pays
  its own stages.
- A per-segment model call is allowed only with an explicit budget setting sized per pass, for data
  the rung needs and cannot get later, from a signal that exists at that point in the ladder.
- The budget is a **rate**: `filler.pipeline.max_split_vision` bounds one pass, not the reel. Grounding
  persists on the split proposal, a partly-grounded reel defers and resumes, and termination requires
  progress. An existing proposal is re-grounded, never re-detected.
- Every segment operation is duration-bounded (`-ss` seek plus a bounded read), long recordings resume
  from the persisted proposal, and failure bookkeeping is written even when the run is cancelled.

The incident behind these rules is archived in
[`filler-v51g-budget-incident.md`](../../project/design-archive-2026-09/filler-v51g-budget-incident.md).

## Progressive enrichment

Readiness answers whether exact bytes may play; enrichment only improves descriptive metadata of
Ready clips, in the same driver but with separate authority (#1251). Preparation always advances
first and yields before optional enrichment starts.

- **Axes:** kind, era, brand, target audience, geography, language, and the taxonomy dimensions
  product, format, seasonal, audience cue and presentation. Each persists `missing`, `complete`,
  `unsupported` or `stale` with its value, evidence, rank, confidence, producer and version. A
  missing answer never holds, unpublishes or removes a Ready clip and never creates an Incoming task.
  Machine-produced `complete` geography must be non-empty.
- **Evidence precedence** is closed: operator correction > exact item metadata or content >
  trusted provider or network mapping > registered-source default > weak inference. Confidence only
  breaks ties within a rank; an upload date is never an era.
- **Passes, cheapest first:** a free local pass over preserved titles, descriptions, filenames,
  provenance and trusted mappings; then an automatic text pass when a text provider is configured
  (at most eight clips per JSON call, validated per item, brands only when literally present, never
  era or geography); then transcription and frame inspection, each with its own identity and
  per-pass bound. New transcript or visible text makes the cheaper passes eligible again. The same
  capability identity is never paid for twice.
- **Context research** may fill a missing era or country for publicly acquired clips from fixed-host
  structured sources (Wikidata, Wikipedia, Archive.org, Library of Congress), each switchable, with a
  model interpreting only the retrieved packet. It saves a cited report. Only a cited ISO country with
  confidence ≥ 60 may populate an empty geography, as `national`; era stays descriptive. Local paths,
  private metadata and transcripts never become public queries.
- **Web search** (Brave, or an operator's SearXNG) is an optional last resort, at most once per clip
  revision, reserved against a durable monthly ceiling before dispatch. It stores titles, URLs and
  snippets only and never fetches pages. Failures degrade the report, never readiness or playback.

## Watching it happen

- `GET /v1/filler/incoming` serves persisted stage state; SSE `filler_clip` frames are self-sufficient
  snapshots and only a latency optimisation.
- **Incoming is one conveyor:** decisions first, then clips still being prepared, one row per clip
  (reels included), then a separate refusals section. The ladder is served, and a disabled stage is a
  `skipped` rung, not an absent one.
- **Only transcode reports a real percentage** (ffmpeg `-progress` against the known duration). Other
  stages report step boundaries or nothing; a running stage with no measurement shows no bar. The
  database write throttle is `≥ 10 points since the last persisted value` OR `≥ 2 s`.
- **Preparation progress** (V68) is a server projection per attempt: every rung counts once, a skip
  counts as done, the value never moves backward within an attempt, and Ready is exactly 100. A Ready
  estimate appears only when recent comparable local runs support it, as a range, never a countdown.

## The catalog listing

- Offset pagination; `limit` defaults to 100 and caps at 500, with the default in the API, never the
  store (`ClipFilter.Limit == 0` means no limit).
- Sort by `name | duration | added | plays | confidence`; `added` uses `clips.created_at`.
- Search covers name, brand, visible text and tags with `LIKE`; there is no full-text engine
  ([0003](decisions/0003-federated-search.md)).
- Composites list as containers through the opt-in `ClipFilter.TopLevelOnly`; segments stay the
  airable clips, and pod assembly never sets it.
