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

## Derivative encode levels

Offline derivatives encode with constant quality on the host's own hardware family (#2121), using
`encode.FileVideoArgs`. Each family's level is **measured, not chosen**: `cmd/filler-encode-bench`
encodes a corpus of real source masters at candidate levels and scores each output against its
source with VMAF. The level is the highest whose per-clip VMAF stays at or above **95 for 90% of
the corpus** (the 10th percentile). `encode.FileLevel` records it. A family with a file mode but no
measurement encodes in software instead of guessing (`encode.FileFamily`), and so do the generic
encoders.

Measured on 30 archive commercials (mostly 480p H.264), 2026-10-30. Size and CPU are relative to the
software evidence recipe these levels replace (libx264 `slow`, CRF 14: VMAF p10 97.2):

| Family | Level | VMAF median / p10 | Size | Encode CPU | Status |
| --- | --- | --- | --- | --- | --- |
| NVENC (GeForce RTX 3080 Ti) | `-cq 25` | 96.8 / 95.2 | 54% | 6% (CPU decode in the bench) | measured |
| Software (libx264 `veryfast`) | `-crf 16` | 96.7 / 95.3 | 83% | 30% | measured |
| VAAPI (`-rc_mode CQP`) | — | — | — | — | not yet measured on an Arc: encodes in software |
| VideoToolbox (`-q:v`) | — | — | — | — | unverified: no Mac |

**One graph per derivative.** The host is the channels' own: the operator's pinned
`playout.encoder`, else the encoder detection published, and software until detection settles. The
stage reads it once per clip, so the recipes it records always name the encoder that made the bytes.
Decoded frames split once:

- The **encode branch** only converts the pixel format, on the device that holds the frames. On
  NVENC and VAAPI the source decodes on the GPU and never leaves it. If GPU decode fails, the
  encode runs once more with CPU decode before it counts as a failure (the playout ladder's rule).
- The **detector branch** downloads the full frames and runs `blackdetect` and `freezedetect` on
  them, exactly as before. A 160-pixel thumbnail was tried and rejected. On 74 real masters it
  matched `blackdetect`, but it averaged away the motion `freezedetect` measures: it invented
  freezes in 4 clips and doubled one clip's frozen time, enough to send good adverts to review. The
  full-size branch matched the CPU detectors exactly on every clip that diverged, and cost about
  13% more detector CPU with no extra wall time.

**One encode, two roles.** Evidence and playback share one recipe: 1-second keyframes and 256 kbps
audio. The evidence recipe is derived from the playback recipe, never restated. A top-level clip's
playback rendition is a byte copy of the evidence file, carrying the evidence encode's detector
findings, so the evidence picture is exactly the one that airs. A conditioned split child still
encodes, because its input is the conditioned cut, not the master. Recipes are `filler-*-v2`, and
the libx264 v1 records written before this change still validate, so no stored clip's lineage
breaks.

**A tune stops an encode.** The transcode rung runs under the playout budget's background context.
When a live transcode is admitted, the encode stops at once. The rung yields as a deferral: no
attempt is spent, and no other encode starts in that pass while playback runs. Transcription and
splitting only wait before they start. A long recording's transcription interrupted at every tune
would never finish.

The next level up fails the floor on both measured families (NVENC `-cq 27`: p10 94.1; x264 CRF 18:
p10 94.2). The worst single frame is lower on NVENC than on the old recipe (78 against 91 at these
levels). It is a pooled-minimum outlier, not what the floor gates on, and is noted here so a later
change in the rule starts from it.

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
  breaks ties within a rank; an upload date is never an era. The registered-source default is the
  source's own declared country only. A US source default yields to the British word "advert" in
  the item text, which leaves the clip unknown rather than labelling it GB.
- **Withdrawal.** An `unsupported` answer with no value means "this producer has no answer". It
  retracts the same producer's own earlier answer when the producer's version changes, and nothing
  else: it never displaces another producer's value or an operator's. That is how a corrected rule
  un-labels clips it labelled before. Bumping the producer version re-runs every clip, so no
  migration rewrites state. Withdrawing a geography clears only a `registered_source:` stamp.
- **The clip row mirrors accepted geography.** Airing reads the clip's geography columns, so every
  accepted geography answer overwrites them; precedence was already settled on the axis (#2061).
  A fill-only projection left Cleveland station news "national" after a stronger answer arrived.
  Rows located before the axis existed keep their value until an answer replaces it, and an
  operator's row (`geo_evidence = operator`) is never overwritten by the projection.
- **Broadcast stations** (#2061). The free pass resolves call signs in the title, filename,
  description and on-screen text against the broadcast station directory, on the clip's date. Only
  a cued call sign counts, because many are words (KING, WHO, WEEK): a licence suffix (`WABC-TV`),
  a network prefix (`ABC/WABC`) or a following channel number (`WJW 8`). A channel number counts
  only in mixed-case text that mentions broadcasting; all-capitals text and on-screen text (OCR is
  capitals and ad copy) need a suffix or prefix. The date comes from the title and filename only
  (one full date, else one year, else any time), because a description often dates the
  digitisation; digit runs joined to other digits by a hyphen are phone numbers, not years. Exactly
  one station must match; two mean none. The station's own material is `local` to it as a trusted
  mapping: a station ID, a named local segment (news update, eyewitness news, weather forecast,
  meteorologist, station sign-off) in the item text, or a title that leads with the station and
  names its promo, ident, news, weather or sign-off. On-screen text never makes a clip local. A bare
  "news", "weather" or "forecast", a network newscast ("Nightly News") and a slogan's "sign off" are
  not the station's own material. Anything else credited to a station, such as a network promo or an ad
  recorded off it, is `national` in the station's country, as a confidence-60 inference that keeps
  the station as provenance: a source's declaration outranks it, and research citing a local
  advertiser may overturn it. A `local` clip airs only where the channel's place is within the local
  radius of the station (#2062).
- **Passes, cheapest first:** a free local pass over preserved titles, descriptions, filenames,
  provenance and trusted mappings; then an automatic text pass when a text provider is configured
  (at most eight clips per JSON call, validated per item, brands only when literally present, never
  era or geography); then transcription and frame inspection, each with its own identity and
  per-pass bound. New transcript or visible text makes the cheaper passes eligible again. The same
  capability identity is never paid for twice.
- **Context research** may fill a missing era or country for publicly acquired clips from fixed-host
  structured sources (Wikidata, Wikipedia, Archive.org, Library of Congress), each switchable, with a
  model interpreting only the retrieved packet. It saves a cited report. Only a cited ISO country with
  confidence ≥ 60 may populate an empty geography, as an inference any stronger answer outranks.
  The model may also name a call sign and say whether the clip is that station's own material;
  Loomarr resolves the call sign against the station directory in the clip's year or decade, which
  is how a reused call sign is settled (#2063). A resolved station's own material is `local` to it,
  and anything else keeps it as provenance on a `national` answer. A call sign that resolves to no
  one station answers nothing, and without a station a report never says `local`. Era stays descriptive. Local paths,
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
