# Filler structure

Formerly `design.md` §10's V67 structure assessment, V45's composites and splitting, V34's detector
and V51a's identity rule. A long recording may hold one advert, a break of many, programme material
with spots, or nothing usable. This doc covers how Loomarr decides which, cuts children, and keeps
the parent. Child screening and tagging are in `filler-classification.md` (still in `design.md`
§10 until it moves); the per-clip pipeline is in [filler-pipeline](filler-pipeline.md), and child
conditioning evidence is in [filler](filler.md#conditioning-evidence-for-split-children).

## Long sources are quarantined at intake

Duration past `OverlongSegmentMs` (120 s) makes a clip a **long-source candidate**: not airable and
never matched into a pod, like a held clip. Duration only asks for assessment; it is never evidence
that the source is a compilation. Boundary detection belongs to `split`, not `probe`, so an
hour-long capture is decoded once.

`IsComposite` is a separate axis from `Kind`: a container is excluded from pods once, the safe
polarity, instead of every `filterKinds` caller special-casing a `composite` kind. While structure is
unresolved it means "long-source candidate"; only the structure assessment below makes it a
semantic verdict.

## Structure assessment (V67)

One deep module takes an exact V66 evidence asset plus independently timestamped observations and
returns one validated, content-addressed assessment (decision
[0029](decisions/0029-structure-before-children.md)). Callers never sequence detectors, interpret
model confidence or assemble cuts.

| Source structure | Meaning |
| --- | --- |
| `single_unit` | one usable item, including a long single-product infomercial |
| `compilation_break` | two or more independently bounded filler units back to back |
| `programme_with_spots` | programme material with inserted filler units; programme spans stay explicit |
| `ambiguous` | the evidence cannot safely distinguish the above |
| `unusable` | the source cannot support a complete trustworthy assessment |

**Observations stay independent.** Chapters, black, silence, transcript topic changes, OCR or logo
changes, and audio and visual continuity are durable source-relative facts with a kind, interval
or uncertainty window, producer identity, evidence digest and outcome. The reducer computes
coincidence; nothing flattens them into a score. A scene cut is never a boundary proposal; it fires
inside adverts as often as between them.

**Boundary fusion is deterministic and conservative:**

1. A declared chapter edge or compatible precise separators may propose a cut.
2. Agreement narrows the window; a conflict stays on the candidate and blocks unattended
   materialization.
3. Transcript, OCR, audio and visual changes may support, contradict or leave a candidate open. A
   modality that did not run is not negative evidence.
4. A bounded model may inspect only unresolved spans, citing supplied observation ids with full
   provider, model, prompt, digest, time, token and cost identity. It adds an observation; it cannot
   override a conflict or set the verdict.
5. No scene-only, duration-only, model-only or round-timestamp candidate becomes an automatic cut.

**The plan covers the whole timeline.** Intervals over `[0, duration)` are ordered, adjacent and
non-overlapping, each `keep` (two resolved boundaries and an established role), `discard` (a
closed reason, time retained) or `unresolved`. There are no implicit gaps; edits and partial
confirmation preserve coverage.

**Each `keep` interval gets its own role**: `commercial`, `promo`, `bumper`, `station_id`, `psa`,
`trailer`, `interstitial`, `programme_fragment`, `non_filler`, `ambiguous` or `unusable`. A parent's
kind is never copied. The last four are never admitted automatically. Lineage keeps the exact role
(`promo` survives there even though the catalog projects it as `interstitial`), and the sidecar
keeps the derived kind so a rebuild cannot collapse children to the parent's kind.

- The existing split-time vision call may return a `segment_role` observation alongside taxonomy,
  bound to source, span, frame digests, prompt, request, response, route, model and accounting.
  Missing identity or an unsupported role yields no claim.
- When frames cannot establish a role, one **direct-video escalation** through the certified
  `filler_video` route may inspect a fresh metadata-free MP4 of only that interval (at most 60 s and
  12 MiB; longer spans are held, never narrowed). A valid frame role suppresses it; any failure
  leaves the interval unresolved.

**Materialization needs agreement.** A deterministic **structure-decision reducer** takes complete
candidate assessments from at least two independently locked producer families (two routes to one
model family count once). They must agree on the source unit, interval count, each interval's
disposition and role, and the ordered joins, with joins within the locked tolerance (the reducer
takes the midpoint and revalidates coverage). Any conflict, failure, gap, disagreement or
unsupported candidate is an explicit hold; majority voting never decides.

- A **complete-timeline assessor** sees no peer answer and must describe every millisecond. One
  content-addressed assessment-media contract owns the derivative: MP4, H.264/yuv420p, 960×720 at
  30 fps with aspect-preserving padding, AAC stereo 48 kHz, 90 kHz timescale, stripped metadata,
  bitexact, at most 64 MiB, with the argument templates part of the recipe identity. Every
  assessment binds the source and the derivative as distinct identities. The preparer gets two
  roots, the applied filler root and a private assessment-media root, and neither is derived from
  the other.
- The runtime prepares and validates all media, and stays inside the certified duration and byte
  envelope, **before** any provider metadata refresh, reservation or request. Nothing extrapolates
  from shorter cases, truncates a reel or substitutes chunks; long reels need their own certified
  protocol.
- The OpenRouter adapter uses one fallback-disabled, zero-retention route and a strict schema,
  hashes everything, and durably reserves the request digest and worst-case charge in the shared
  filler-inference budget before sending. A journal refuses a second reservation for the same
  request. An unknown settlement keeps the full reservation; open and budget-held entries are
  recovery holds, never permission to resend.
- The reducer publishes an immutable artifact with every candidate and the reason for each
  decision. A child cut on a confirmed `keep` interval carries its SHA-256 in conditioning lineage;
  legacy or operator-edited cuts leave it absent.

**Structure authority is not broadcast admission.** A certified `compilation_break`, or the bounded
filler portion of `programme_with_spots`, may authorize automatic materialization of its complete
keep plan into held, non-airable children that enter the ordinary ingest ladder. That needs exact
evidence bytes, resolved boundaries and roles, complete explained coverage, and a source and signal
slice inside the locked structure certificate. It says nothing about safety, playability or
readiness. Unresolved intervals leave one concise structure review.

- **Failures stay at split review.** A structure-assessment failure, including a source outside the
  reviewed envelope, keeps the source and proposal at the split review rung; it never exhausts into
  a terminal Complete, creates children or confers authority. Cancellation keeps resumable work.
- Without a verified complete-timeline decision and matching authority, a proposal stays reviewable.
  The legacy calculation may run as a non-authorizing **shadow comparison**, recorded immutably with
  exact spans; failing to persist a required comparison blocks unattended materialization and never
  selects the comparison as a fallback.
- Children are prepared as **one replacement generation** from the exact reviewed source intervals
  through the V66 publisher. The parent, assessment and prior generation stay until every child and
  its lineage validates and the switch commits atomically, so a partial generation never replaces a
  complete one.

**Certification** uses a rights-cleared corpus split by source family: declared and chaptered
truth, authored compilations, programme excerpts with spots, long single units, same-brand joins,
wordless units, bumpers, slivers, damaged tails and adversarial non-joins. It scores source
structure, under- and over-splitting separately, boundary error at predeclared tolerances, purity,
coverage, role accuracy, abstention and operational failure per slice. Every automatic decision
must have zero wrong outcomes; abstentions count against coverage. The development certificate
(at least 30 of 60 decisions, 6 per source-unit category, 6 per declared difficult slice) permits
shadow evaluation only. Automatic materialization expands only for slices whose locked estimates
and bounds pass. Model training waits until measurement names a residual error class.

## Detection

The V34 detector is now a proposal and shadow input under V67; where they conflict, V67 governs.
Its design and measurements are archived in
[`filler-splitting-v34.md`](../engineering/archive/design-2026-09/filler-splitting-v34.md).

1. **Triage.** Chapters split for free, but most sources have none.
2. **Coarse split.** `blackdetect` and `silencedetect`, parsed in Go. Segments under the detection
   floor `max(MinSegmentMs, filler.min_duration)` (3 s sliver floor, 10 s catalog floor by default)
   become explicit discard intervals; `max()` so a `0s` catalog floor still drops fade artefacts.
   Scene-cut detectors were measured and rejected. Detection quality is a property of the source.
3. **Rescue.** A segment longer than a typical spot (not only over 120 s) gets transcript (Whisper)
   plus LLM cut points. The LLM must return exactly one entry for a single advert; without that it
   invented round-number cuts inside an infomercial. With no runnable Whisper, an over-long segment
   is **unsplittable**, never guessed.
4. **Role before enrichment.** Each interval gets its own role first; taxonomy enrichment adds
   product, era and audience only afterwards, and cannot repair an unresolved role.
5. **Deterministic discard.** A dHash over grey frames at 1/3 fps separates re-encoded duplicates
   from different adverts by a wide measured margin; duplicates and sub-floor segments are
   discarded before classification. Neither deletes source media. Ambiguity is never a discard.
   The proposal reports discarded time ("39 fragments under 10s discarded"), never silently.
6. **Per-segment language.** Each span samples its own bounded audio with the installation's
   detector. A confident mismatch is omitted and kept in the proposal receipt with interval,
   languages and reason; wordless, unknown and backend outcomes stay reviewable with a stable reason
   (`unavailable`, `failed`, `inconclusive`, `paused`). The work is cursor-persisted within the
   Whisper allowance, and a proposal is reviewable only when every span is terminal. The proposal
   records the language it used, so a later change offers a recheck. If every candidate is a
   mismatch, the split resolves with a receipt instead of an empty review. Confirmed children
   inherit results only for exactly matching intervals; client-supplied language is ignored.

**Proposals are persisted jobs.** `filler-split` is a scheduled job (on by default) that proposes
splits for long clips, so review can happen later and survive restarts. Nothing is written until
`POST /v1/filler/splits/{id}/confirm` commits a cut list.

## Auto-split and boundary confidence

`filler.autosplit.enabled` (default on) confirms confident segments without a click, gated on
`filler.autosplit.min_confidence`, a separate dial from tagging: a mis-cut plays half an advert.

- **Per segment.** A segment is cut when it passes every refusal and clears the threshold; the rest
  stay in a shrunken proposal (a reel of 52 becomes 47 clips and 5 cuts to review).
- **Refusals first, then the threshold.** A suggested era, `unsplittable`, a duplicate, an over-long
  or a sub-floor span refuses at any score. `boundaryScore` cannot see tag facts, so nothing
  launders a refusal into a score.
- **A confirmed segment is not airable.** It is a held child that still goes through conditioning,
  classification, screening and terminal readiness.

**Boundary confidence** (0–100 per segment) answers "did we cut in the right place?". It is
distinct from a clip's tagging confidence ("do we know what this is?"), and the split gate reads
only the first. It is a ceiling ladder:

| Evidence for one boundary | Ceiling | Basis |
| --- | --- | --- |
| chapter marker, or the reel's own start or end | 100 | declared |
| black **and** silence agreed | 90 | measured: 9 of 12 fades corroborated |
| one detector only | 65 | measured: the other 3 of 12 |
| transcript rescue alone | 50 | measured: ±2–3 s timing |
| truncated (an overlap moved this edge) | 40 | asserted |

Within one boundary the best evidence wins; across a segment's two boundaries the worst wins. Then
over-long caps at 50 and `unsplittable` at 20. Black is not ranked above silence, and duration is
not an input beyond the over-long cap (half of a real reel was correctly cut sub-10 s bumpers). A
single-span rescue removes the over-long cap rather than adding points. dHash, duplicates and vision
tags are deliberately not scored.

## Split review

- **Every cut plays in place.** A proposed segment has no bytes until confirm, so the preview is a
  byte-range window of the retained parent (`GET /v1/filler/media/{clipHash}`, served by
  `http.ServeContent`). The player is clamped to `[startMs, endMs]` and shows the segment's length.
  At most one player is mounted.
- **Stills first (V68).** Each segment gets one representative still from its exact span, ingested
  through the image service as proposal-owned artwork and released on confirm, replacement, rewind
  or expiry. A failed extraction shows **Preview unavailable**; re-detection is the retry.
- The proportional timeline is the overview: one ordered, scrolling list with each segment's still,
  name, duration, range, tags, language and any reason it needs attention. The actions are
  **Rename**, **Adjust timing**, **Join next**, **Remove** and **Keep clips**; detector evidence and
  transcripts sit under **Details**.
- **Names describe the segment (V69).** Chapter titles are kept. Otherwise the already-budgeted
  vision call or transcript rescue may propose a short name, accepted only when its identifying
  phrase appears in that segment's own signal; otherwise **Clip N from <recording name>** (or "from
  this recording"). Provenance (`source-authored`, `model-proposed`, `fallback`,
  `operator-edited`) stays in the name-edit disclosure, and the server, not the confirm body, owns
  it. Naming never feeds role, language, suitability, boundary confidence or admission.
- **One selected clip uses the shared Sheet (V70)**: preview, edits, era decision, **Join next**,
  **Remove** and details, with previous and next controls. Selection is transient local state;
  closing restores focus. **Keep clips** stays a reel-level action with the same payload.

## Keep the parent

Confirm keeps the compilation as a composite and gives each child a `parent_hash` (decision
[0017](decisions/0017-composites-keep-the-parent.md)). That buys provenance ("which break did this
air in?"), re-splitting when detection improves, and inherited broadcast context. The parent stays
in the store and on disk, marked composite and never airable; `parent_hash` is null for a
hand-dropped clip.

- Full confirmation files the parent and releases its hold as one operation; an upgrade pass
  releases stale holds only for filed composites with no surviving proposal.
- **A completed re-split replaces the prior generation.** When the last cut is confirmed, the store
  restores every child in the remembered generation and tombstones earlier children of the same
  parent that were not reproduced, keeping content-identical cuts and any clip a channel pins. Until
  then the prior generation stays airable. Bytes are never deleted, so a retired cut is restorable.
- **The split sweep is the one place Loomarr deletes operator media.** `filler-split-sweep` retires
  a reel whose leftover cuts nobody reviewed within `filler.split.review_window` (default 30 days,
  `0s` never): it drops the proposal and deletes the recording, only if the reel already produced
  clips. The catalog row survives with `clips.reaped_at` (written before the unlink, and skipped by
  `DeleteClipsNotIn`), so provenance and inherited context survive while re-splitting does not. The
  pipeline row is set to `filed` so the reel is not re-detected. `docs/help/filler.md` says so to
  operators.

**Splitting keys on identity, not location (V51a).** A proposal carries the compilation's hash
(`clipHash`) and derives its file location. A segment is hashed as soon as it is cut and filed at
`ClipRelPath(hash, ext)`, like intake. `dedup`'s self-exclusion compares identities. Test doubles
must key clips the way the real store does, with identity and location deliberately distinct.
