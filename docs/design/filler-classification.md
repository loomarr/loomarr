# Filler classification

Formerly `design.md` §10's V38 tagging confidence, V44 metadata, V45 curation metadata and taxonomy,
V61–V63 readiness, V67 child screening, V68 visual safety and the beta release report. This doc
covers what Loomarr learns about a clip, how sure it is, and what may make a clip Ready. The catalog
fields and grounding ladder are in [filler](filler.md#the-catalog); the pipeline stages are in
[filler-pipeline](filler-pipeline.md); structure and splitting are in
[filler-structure](filler-structure.md). The certification protocol is archived in
[`filler-certification-protocol.md`](../engineering/archive/design-2026-09/filler-certification-protocol.md).

## Readiness is evidence-based; confidence never publishes

One Go-owned **terminal-ready module** makes the production decision after required runtime
processing (decision [0030](decisions/0030-readiness-is-evidence-based.md)). It takes the exact
clip, its durable enrollment authority and the completed conveyor state, and returns Ready, not
usable, or a real Needs-help task whose answer changes durable state. Retries, provider failures,
exhausted budgets and missing optional enrichment are operational states in Diagnostics; relabelling
cannot turn them into chores.

- **One runtime path**, with no `shadow` or `applied` mode. Before any publication write the module
  validates clip identity, requires enrollment authority, derives placement without inventing a
  role, and checks the required work finished. One transaction records the Ready event, stores
  placement, clears `clips.held` and settles the conveyor row. A missing or changed clip, stale row,
  absent authority, composite or objective failure rolls it all back; a repeat is idempotent.
- **The store has no generic `held=false` writer.** Pipeline and operator code may hold or
  tombstone; only the terminal-ready transaction publishes a non-composite. Composites have their
  own constrained operation and stay out of pods regardless.
- Enrichment and diagnostic scoring run before terminal readiness. A skipped optional classifier is
  a visible stage fact that cannot stop an enrolled clip; a positive media failure still rejects.
  Licence metadata never enters the decision.

**Evidence is claim-specific.** The development evaluator accepts one closed, versioned evidence
document with claim roles for media usability, recording date or era, brand, product, content role
and sensitive-policy flags. Each fact names its extractor kind, source, bounded location and any
inference that produced it; the Go policy assigns authority, and evidence cannot rank itself.

- Decoder measurements own media usability. A source-owned date or recording sidecar outranks a year
  spoken in a clip. A filename year and a different spoken year are a conflict, not two votes.
- Independent corroboration means distinct extractor kinds over distinct derivatives; repeated
  tokens from one transcript, frame or model call count once. Role and product need at least one
  in-clip signal: filename plus uploader description is one uploader talking twice.
- A high-confidence commercial needs a corroborated product from the closed taxonomy; open-text
  brand cannot substitute. A clip may still be Ready with enrollment-grounded break-body placement
  while its role stays unclassified.
- All text from filenames, metadata, transcripts, OCR and frames is untrusted data with no
  instruction authority. Contradiction is a first-class result: invoke one bounded extra rung or
  abstain with a question. More conflicting tokens never raise confidence.
- `filleradmission.Evaluator.Evaluate` is deterministic development and certification tooling with
  no provider, decoder, store, clock or network, and it is not a runtime gate. Every inference it
  cites must be referenced by a fact; a failed or unavailable step is an operational hold, never a
  semantic verdict; model confidence is never read by policy.

**Development measurement stays out of the household conveyor.** Certification comparisons measure
classifiers and keep disagreement evidence; they cannot create, delay or reverse a Ready
transition. Any model, provider, prompt, schema, taxonomy, extractor or policy change invalidates
the affected measurement until it is replayed.

## Routing and inference accounting

- **Routing is reason-driven, not confidence-driven.** Deterministic and text evidence run first;
  frames only for a named missing or conflicting visual claim; direct video only for a named
  temporal ambiguity; premium escalation only for a predeclared reason whose measured value
  justified its ceiling. After each rung the same evaluator reruns; a failure becomes an operational
  hold and never `admit`, `reject` or review. A request carries only the modalities its route
  declares.
- **Bounded derivatives.** The frame derivative is at most four ordered JPEGs from windows at 5%,
  one-third, two-thirds and 90% of the span (each decoding at most 3 s, the last biased to the end
  card), native size up to 1920 px, never upscaled. The router may send a subset. The hosted video
  derivative is one metadata- and chapter-stripped H.264/AAC MP4 of at most 60 s, within 1280×720,
  at most 12 MiB, with one ffmpeg thread, 512 output tokens and a 256 KiB response limit. The 320 px
  UI still is never semantic evidence. Hosted routes disable fallback, require zero data retention,
  and fail closed before any request when a bound or capability check fails.
- **Roles are one versioned inference policy (V62):** `lineup`, `filler_text`, `filler_frames`,
  `filler_video` and `transcription`, each resolved to one concrete route with capability, privacy,
  fallback and budget constraints. Compatibility proves a route accepts the modality and schema;
  only certification proves quality. The last certified policy is the automatic default. Without
  one, the configured compatible route is labelled unverified and cannot expand unattended
  decisions. Manual choices are **Advanced overrides**, recorded on every evaluation. A moving
  `latest` alias never authorizes unattended filler decisions.
- **Every adapter returns an attribution envelope**: requested and resolved model and provider,
  modalities, token categories, exact provider-reported charge and currency, latency, attempts and
  generation id. OpenRouter's `usage.cost` is the billing fact; missing facts stay missing. Local
  price estimates are a separate value tied to the run's price snapshot.
- **Accounting is durable.** One row per evaluation moves atomically from reservation to immutable
  settlement, keeping exact decimal charge text plus an integer nanodollar projection. Per-clip,
  UTC-day and certification-run ledgers reserve before the call; shared-appliance inference is
  serial by default; exhaustion leaves the evaluation held and recoverable.
- The evaluation cache key includes every semantic input (evidence, extractor, prompt, schema,
  model, provider, role policy, taxonomy, policy, modality, derivative bounds), so a change cannot
  reuse an old answer.

## Readiness audit and operator surfaces (V63)

The terminal-ready module appends an immutable effective event (clip, enrollment reference,
placement, pipeline identity, outcome, time). Human state changes are append-only actions; skipping
a diagnostic is navigation, not an action. The store contract is the same on SQLite and Postgres:
events are idempotent by id, event, placement, hold release and settlement share one transaction,
and reads use keyset pagination on `(created_at, id)` with counts from the row predicates.

Four server-owned projections:

- **Overview**: the latest outcome per clip, operational counts and at most one ranked next action.
  A later Ready clears an older hold from health without erasing it from Activity.
- **Needs help**: only current exceptional choices, each with one plain question, the decisive
  evidence and the allowed closed actions. Playback is required before a media-content answer.
  Queued work, retries, optional classification, provider and budget holds are ineligible, and the
  browser never invents a task or action.
- **Activity**: the bounded audit of Ready and not-usable events and real human actions.
- **Diagnostics**: each clip's latest hold with one server-authored recovery plan: an automatic
  retry with its time, an allowed manual retry, a configuration destination, or inspection of the
  held media. No provider text, paths, prompts or evidence locations.

Manual retries use a separate append-only recovery ledger: a caller request id, the authenticated
actor, and a recheck that the hold is still the latest retryable one. Repeats are no-ops and a
reused id for another decision conflicts; the hold stays visible until a newer result exists.
Overview and Activity are member-readable; Needs help, Diagnostics and every action are admin-only
(403 otherwise). Incoming stays a calm progress summary, never one review form per clip; confidence
detail and certification comparisons are Advanced or development views.

## What Loomarr learns about a clip

**Transcripts (V44).** The splitter's Whisper output is kept on confirmed segments, and the
`transcribe` job backfills selectively: only when source text is thin or a text-only pass left the
clip untagged. On arm64 local Whisper is impractical, so the job is off by default there.

**Brand** is accepted only when it appears literally in a text signal or on screen. An ungrounded
brand is dropped; no brand is the honest common case.

**Vision** samples keyframes and asks for brand, category and the `visibleText` it read.

- Brand and era must be supported by `visibleText`: they are specific facts, and a model that emits
  one it did not see has fabricated something checkable.
- Category must resolve to an existing taxon but need not be printed on screen: judging a toy advert
  as toys by seeing toys is what vision is for. (Requiring on-screen text admitted zero categories
  on a real reel and made split auto-confirm impossible.)
- Keyframes are near-full resolution and end-biased (above); resolution mattered more than the model
  for reading text. The default local vision model is `qwen2.5vl:7b`, which returns honest fragments
  where the alternative confidently invented a date that would then have grounded itself.
- A free frame-heuristic tier (black-and-white, 4:3 or 16:9) only seeds an unconfirmed era for a
  clip with no other era signal and never overrides a grounded tag.
- The hosted vision path costs money and leaves the house, so it is opt-in; the local Ollama path
  keeps it in the house. Image-free text requests stay byte-for-byte unchanged.

Transcript, brand and visible text persist to the sidecar as well as the store.

**Broadcast context (V45).** A parser over titles and filenames (Go `regexp` plus a callsign table)
extracts network, station, market and full air date, only when they literally appear. It runs on the
composite and its output propagates to every child.

**Thematic tags rank; structured filters decide.** The tagger extracts closed-vocabulary mood, tone,
topic, channel-fit and sensitivity labels from the segment's transcript and vision text. Pod
eligibility stays exact, deterministic `WHERE` clauses (era, audience, category, no repeat brand,
network and market), and the theme tags only order an already-eligible pool. Embeddings were
measured and rejected for theme matching; duplicates use persisted dHash evidence. There is no
vector column, `sqlite-vec`, pgvector or embedding role.

## The clip taxonomy (V45a)

A clip carries a **set** of tags, each a taxon `{slug, label, parent, synonyms, kind}` in a forest
by axis: **product** (`beer` → `alcohol` → `drinks`), **format**, **seasonal** (reusing the holiday
keyword ids) and **audience-cue**. The taxonomy tables are `taxa`, `clip_tags` and `taxa_closure`,
seeded with a default forest.

- **Rollups are queryable**: "one food advert per break" is a count over `descendants(food)`.
- **Classifiers return tags from the live, served vocabulary**; each resolves to a slug, maps a
  synonym or retired alias to its canonical taxon, or is dropped. Only an operator adds a taxon.
  The `category` column is a compatibility shadow of the most specific product tag.
- **Asserted is not leaf.** `clip_tags.leaf` historically means "asserted"; a broad honest assertion
  such as `food` stays asserted if children are added later. Reads expose asserted tags separately
  from the full set, and editors write only assertions.
- **The graph is valid or the edit does not happen.** Slugs, synonyms and aliases are unique; parents
  exist on the same axis; cycles and cross-axis edges are refused. Deleting a taxon with directly
  asserted clips is refused; deleting an unused intermediate reparents its children.
- **Edits are previewed through the same prospective-graph contract they commit**: affected clips,
  playable clips, lineage changes, channel selections and resolver terms, computed by the server.
- **Rollups are stored denormalised** so `WHERE taxon = 'food'` is one index hit. A `taxa_closure`
  table (rebuilt in Go from the forest) makes the rollup rebuild one dialect-neutral
  `INSERT … SELECT`. A graph edit, closure, rollups and category shadow commit in one transaction;
  startup runs the same rebuild as a backstop. Affected channels reconcile afterwards, with the
  channel sweep as the retry.
- The taxonomy view leads with per-axis coverage and catalog links for gaps; axis absence is
  neutral. The raw forest lives under Filler → Advanced → Classification vocabulary; members may
  read it, admins edit it. Brand, kind, era and audience stay separate structured fields, and the
  clip editor can correct or clear brand.

With brand, broadcast context and tags, pod assembly can avoid repeat advertisers, filter by network,
market and era window, pace breaks by measured loudness, and rank by theme, all from auditable
signals. Operator exclusions and quotas use the scheduling rules layer.

**Tagging confidence (V38)** is a 0–100 diagnostic score. Grounding facts cap it (an ungrounded era
can never reach the fully grounded score, a hard ceiling to sabotage-test), and the model's own
confidence may only lower it. It prioritises review and never publishes; the former threshold,
`auto_filed` state and withdrawal route are retired.

**Provider settings.** Provider connections live on the AI page (the lineup model, an Advanced
vision override, and one Speech recognition section: built-in or connected), and feature toggles and
behaviour live on the Filler page. Text and inherited vision resolve one active provider selection
(`llm.provider`, `llm.hosted_provider`, `llm.api_key.<brand>`); endpoint presence, not a key, decides
availability.

## Child screening (V67)

Every materialized child is screened after its final playback derivative exists; a result over the
parent span, a stream-copy cut or a preflight rendition cannot prove the bytes that will air.

- One content-addressed **screening subject** binds the child lineage, source master, evidence
  derivative, playback derivative, recipes, measurements and the parent interval, with no paths.
  Each evaluator reopens its artifact and reproduces the subject's byte identity before deciding.
- Five independent closed outcomes: `visual_safety`, `spoken_safety`, `written_safety`, `rights`
  and `playback_integrity`. The three safety axes inspect the complete evidence derivative; a
  tagger, a few frames, a missing transcript or model prose cannot satisfy them. Each outcome is
  `pass`, `reject` or `hold` with an opaque reason and the digest of one private evidence record.
  Restricted phrases and descriptions never enter public records.
- For a safety axis, `pass` means category evidence is complete, not that the content suits every
  audience. Positive findings drive the audience policy below; incomplete coverage is an axis hold.
- A coordinator runs exactly one evaluator per axis, serially and without showing one another's
  answers, persists each axis before the next and the validated aggregate before returning. Each
  evaluator owns repeat-safe settlement, so a retry replays rather than paying again, and the
  aggregate's time is the latest axis time.
- **The screening rung** sits right after split and applies only to children. Any failure, missing
  coordinator, non-passing result or absent release authority resolves to review or rejection, never
  to enrichment. Exhausted failures park the rung. A startup repair holds and rewinds pre-rung
  children that advanced without a screening record. The admin filing endpoint may file a
  materialized child only when the pipeline proves screening completed.
- **Production runs a qualification runtime.** The deterministic playback verifier runs now; visual,
  spoken and written safety each have a non-authorizing evaluator that reopens the evidence, records
  its identity and returns a durable `hold` naming the missing certification. So no child reaches
  enrichment or release while a safety authority is absent. Evidence lives under the filler root's
  excluded `.loomarr/segment-screening` tree.
- **Playback integrity** reopens the playback file, verifies full SHA-256, length and sparse id,
  reprojects the sidecar (at most 64 MiB, no symlinks) and requires valid derivative QC and, for a
  child, agreement between manifest and conditioning measurement. A sparse-hash match alone never
  passes.

**Airworthiness is audience policy over evidence.** Safety evaluators keep closed suitability flags
separate from axis verdicts: sexual content and nudity; minors and age ambiguity; weapons, threats,
violence, gore, death, animal harm and self-harm; tobacco, alcohol, drugs and gambling; hateful
targeting, extremist symbols, slurs, profanity and explicit language; frightening imagery; and
regulated-product promotion, each with severity and depiction versus promotion or instruction.
Brand and ordinary commercial presence are enrichment facts; religion, politics, war and history
are context, never prohibitions; `minor_present` is not prohibited in itself.

Profiles are `all_ages`, `general_audience` and `restricted_archive`. A deterministic policy maps
certified flags to `allow`, `review` (Airworthiness hold) or `reject`; no model chooses an audience
verdict. An observed reject always wins; otherwise every relevant flag needs complete coverage from
an axis record matching the configured authority, and missing coverage, an uncertified flag,
profile drift or conflicting evidence holds.

| Suitability flag or family | `all_ages` | `general_audience` |
| --- | --- | --- |
| adult nudity; minor or age-ambiguous sexual risk | reject | reject |
| sexual activity or sexualized presentation | reject | low reviews; moderate/high rejects |
| weapon depiction | promotion/instruction or moderate/high rejects; low reviews | promotion/instruction rejects; high reviews; low/moderate allows |
| threat; non-graphic violence | moderate/high rejects; low reviews | high rejects; moderate reviews; low allows |
| graphic violence or gore | reject | reject |
| human death or corpse | moderate/high rejects; low reviews | high rejects; low/moderate reviews |
| animal harm or death | moderate/high rejects; low reviews | high rejects; moderate reviews; low allows |
| self-harm or suicide | reject | promotion/instruction or high rejects; low/moderate reviews |
| tobacco, alcohol, drugs or gambling | promotion/instruction or moderate/high rejects; low depiction reviews | promotion/instruction rejects; high reviews; low/moderate allows |
| regulated-product promotion | reject | reject |
| hateful or extremist symbol | promotion/instruction or high rejects; low/moderate reviews | promotion/instruction rejects; high reviews; low/moderate allows |
| hateful targeting; slur or degrading language | reject | reject |
| profanity | moderate/high rejects; low reviews | high rejects; moderate reviews; low allows |
| explicit sexual language | reject | moderate/high rejects; low reviews |
| frightening, disturbing, or severe injury or medical imagery | high rejects; low/moderate reviews | high rejects; moderate reviews; low allows |
| ordinary minor presence; age ambiguity alone; religious suffering; war or military; ordinary brand presence | allow | allow |

`restricted_archive` keeps the same observations but always holds unattended playout: it is a
retention profile. Certification is per policy and per flag: a corpus that certifies adult-nudity
detection certifies nothing else.

- **Producer results are projected through an immutable authority** that binds the producer policy,
  certification and implementation, the opaque id, and its flag, severity and context, after
  reproducing the producer's canonical result. A known positive can publish an observation and
  reject even with other coverage incomplete; an unknown id, missing timing or stale identity is
  incomplete evidence. A negative covers only the flags its certificate claims.
- A visual result with several match ids and several intervals never invents an id-to-interval
  mapping; unbound matches add no timed observations and leave coverage incomplete.
- The spoken and visual adapters reopen the evidence and reproduce its digest, replay a settled
  operation before calling the producer, and pass the same operation id so the producer's ledger
  replays too. Any drift is an invalid operation, never a permissive fallback.

**Rights are provenance, not a screening decision.** Licence values stay exact metadata on sources,
candidates, sidecars and clips; they are never an input, preference, screening axis or admission
claim. Private household playback does not ask anyone to adjudicate copyright. The project's own
corpus acquisition keeps its separate rights controls.

Screening records support classification and configured protection without becoming a second
approval system. A positive objective failure or a configured positive safety finding can make a
clip not usable; an unavailable model, incomplete certification or missing classification fact
cannot block an otherwise valid enrolled clip. Screening facts used at runtime bind the exact bytes
and profile; stale evidence is ignored, never treated as a pass.

### Complete-source visual safety (V68)

`visual_safety` is a complete-source question, not the taxonomy vision rung. One deep module owns
source validation, the coverage plan, adapter observations, reduction and the canonical result; its
interface takes a path-free source authority plus a local path and returns one content-addressed,
non-admitting result.

- The authority binds source id, full SHA-256, length, duration, video-stream identity, the private
  visual-policy digest, measurement time and probing tool. The module snapshots a regular
  non-symlink file and reproduces the authority before extraction or spend. No filename, metadata,
  tag or parent classification enters the verdict.
- One versioned **coverage profile** declares maximum duration, target interval, maximum drift,
  maximum observations and the shortest restricted display it covers. The planner includes the
  first and last representable milliseconds, and a profile whose floor is not strictly greater than
  its largest gap is refused. Any missing, duplicate, corrupt, out-of-order or drifted frame makes
  coverage incomplete, never negative.
- Observations are `prohibited`, `no_signal`, `incomplete` or `failed`, carrying only opaque ids and
  reason codes publicly. A portable complete-coverage lane is mandatory; a bounded direct-video lane
  runs only on declared escalation; an optional Apple Sensitive Content Analysis lane may add a
  source-level observation only with its entitlement, policy, OS identity, digest and result bound,
  and its negative is never a standalone clear. Any unsupported or failed lane is `incomplete` or
  `failed`, never `no_signal`.
- **Reduction is asymmetric.** Any valid prohibited observation quarantines the source and every
  mapped derivative. Otherwise missing mandatory coverage, a failed lane or drift holds. Only complete
  valid negatives yield `no_prohibited_visual_observed`, which is evidence, not authority.
- **Certification** uses rights-cleared, source-family-disjoint positives and clean controls with
  truth locked first. Every positive at or above the display floor must quarantine; with zero misses,
  at least 59 independent positive families give a one-sided 95% lower recall bound of at least 95%.
  Generated and transformed variants never inflate the family count. Until a certificate and a
  release authority exist, the qualification evaluator returns `visual_safety_not_certified`.

## Beta release readiness is one fail-closed report

`/v1/filler/readiness` answers whether an install can use filler now; it does not certify a release.
A beta candidate is releasable only when one local, versioned manifest binds the exact server, Web
and Android TV candidate identities to accepted evidence, and evaluates to a canonical **GO** or
**HOLD** report. Issues and comments are coordination, never release authority.

- `internal/fillerrelease` is one deep module: manifest bytes, an evidence filesystem and a report
  time in, the complete report out. The command owns flags, I/O and exit status; tests substitute an
  in-memory filesystem. It has no network, provider, deployment or admission authority.
- The manifest (schema 2) names the tag, commit, image digest, Web build, Android TV artifact and
  configuration profile. Its cohort binds exactly 32 unique Ready clips (source master, lineage,
  playback derivative and sidecar hashes) in frozen seed order, each with successful Range playback,
  and at most one duplicate family.
- It references seven bounded pipeline results (media preparation, duplicate control, configured
  language, suitability, readiness, playback, descriptive enrichment), each with explicit
  denominators, abstentions, holds and prohibited admissions; separate Archive.org and
  operator-authorized YouTube journeys; Web and Android TV emulator journeys (a named emulator; a
  physical device is refused); deployment and rollback evidence; and SBOM, signature, provenance and
  notice artifacts.
- Anything unknown, duplicated, unsafe, missing, stale, inconsistent or unverified holds. GO needs
  every requirement passing for the same candidate with zero residual decisions and failures.
  Reports are deterministic with a self-digest; the command writes the report on HOLD, exits zero
  only for GO, and treats unreadable input as an execution error.
