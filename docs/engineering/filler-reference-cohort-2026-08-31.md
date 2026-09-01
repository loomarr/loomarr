# Production-ready filler reference cohort

**Status:** active design and execution contract
**Decision date:** 2026-08-31
**Contract version:** `filler-reference-cohort-2026-08-31-v3`
**Tracking:** [#741](https://github.com/loomarr/loomarr/issues/741)
**Scope:** the rights-locked 300-case recovery development corpus
**Output:** 32 exact, playback-ready clips accepted by the maintainer

## Decision

The temporal 32-case challenge is classifier diagnostic evidence. It is not a candidate filler
library and must not be repackaged as one. The new reference cohort starts from the full 300-case
development corpus and optimizes for clips a person would deliberately schedule, not for difficult
role disagreements.

The maintainer's job begins only after Loomarr has proposed a finished artifact. For each proposed
clip the review surface plays the exact cleaned output, shows its readiness grade and production
taxonomy, and asks for **Yes** or **No plus a reason**. It does not ask the maintainer to derive
structure, role, evidence citations, or taxonomy from scratch.

This cohort is development and product-acceptance evidence. It does not replace the independent
1,126-case certification holdout or its confidence denominators.

## Why the earlier workflow stopped

The current temporal challenge deliberately concentrates role ambiguity. Twelve of its 32 cases
(37.5%) carry the prior role `programme_excerpt`. Across the full recovery cohort, the completed
legacy model-label artifact marks 271 cases eligible even though eight of those are programme excerpts and
two are compilations. The production admission evaluator correctly treats corroborated
`programme_excerpt` and `compilation` as non-filler. The older label artifact used `eligible` more
broadly and therefore cannot stand in for catalog admission or editorial readiness.

Four concepts remain separate in every artifact and report:

1. **Semantic role:** what the bytes contain.
2. **Catalog admission:** whether evidence and policy permit the clip to enter the filler catalog.
3. **Editorial readiness:** whether the exact prepared clip is useful enough to schedule.
4. **Maintainer acceptance:** whether the named human accepts the exact output hash.

No value in one layer silently implies a value in another.

Version 3 adds one sparse, content-bound contract-review input to Gate A. The locked development
manifest remains immutable semantic evidence, but a corroborated mistake in its labels cannot become
a candidate merely because two model reviewers and an adjudicator repeated it. The supplemental
review can only exclude exact bytes; it cannot admit, relabel, add taxonomy, or stand in for the
maintainer's later playback acceptance.

## Named exemplars

The following examples bind the initial editorial interpretation. They are evidence for the rubric,
not hidden overrides in a scorer.

### Positive anchor

`archive.org/vhscommercials/y-2mate.is-kix-commercial-on-pbs-kids-tv-funding-sponsor-2001-hd-version-2-3-kjd`
is a historical Kix funding-sponsor example proposed for the kids/family lane. Gate A deliberately
holds it: the currently bound development evidence does not ground a production product assertion
for this source. It may become a candidate only when immutable, content-bound evidence establishes
its product and the exact prepared output is played and accepted. The audit never infers cereal
taxonomy, audience, or admission from its filename, collection, or this narrative example.

### Negative anchors

- `archive.org/movie_trailers_unsorted/TheInvisibleBoy-Trailer` is held out. Existing evidence
  identifies programme footage rather than an established, complete trailer unit.
- `archive.org/movie_trailers_picfixer/CodeOfTheCactusTrailer` is rejected from the candidate
  library. The recorded legacy model label is `programme_excerpt` despite its Archive trailer
  filename; the audit's exclusion rests on the bound semantic role, not that filename.
- `archive.org/classic_tv_commercials/VID20181114WA0037` is rejected from the candidate library.
  A model treated an amateur six-minute video's opening text as a broadcast promo; collection
  membership and a promotional-looking card do not establish broadcast filler.

Age alone is not a rejection rule. Vintage material must nevertheless be complete, authentic
filler with a credible scheduling use such as a classic-TV channel. "Old and public domain" is not
an editorial use case.

## Ordered admission gates

Every candidate passes these gates in order. A later gate cannot repair an earlier failure.

### 1. Rights and immutable identity

- The exact source representation is present in the recovery rights ledger.
- Required credit and development-only restrictions remain attached.
- Source bytes, prepared input, and every output have SHA-256 identities.
- Rights eligibility authorizes processing; it does not claim editorial value.

### 2. Objective media usability

- The file decodes and has one video and one audible audio presentation.
- The audit derives every input digest from the exact raw bytes it strictly decodes. Duplicate JSON
  object keys at any depth, unknown fields, and trailing values fail the whole audit for the
  manifest, packets, mapping, acquisition ledger, and content review.
- Source and reviewed-segment duration are both positive. A zero duration is unusable evidence, not
  an empty clip that can pass because it falls below no positive threshold.
- The reviewed segment starts at or after zero and ends within the source duration. A decoder fact
  marked usable is still unusable when `no_video` or `no_audio` is true or when it lacks exactly one
  valid hashed, positive-byte, positive-duration, positive-dimension MP4 evidence presentation.
- Duration, dimensions, cadence, A/V timing, black, silence, freeze, integrated loudness, and true
  peak are measured through the existing media-tools contracts.
- Existing hard failures remain hard failures: missing streams, unprobeable media, and the designed
  dead-air thresholds never become human taste questions.

### 3. Filler identity

- The bytes establish one complete commercial, promo, bumper, station ID, PSA, trailer, or
  legitimate interstitial.
- `programme_excerpt` and `compilation` never enter the candidate library.
- A source filename, collection, uploader title, or prior model answer cannot establish this gate
  without in-clip evidence.
- A commercial requires a grounded product from the production taxonomy, as the existing admission
  evaluator requires.

### 4. Editorial utility

The exact clip must be something a reasonable curator could intentionally schedule. At least one
specific use must be stated: for example kids programming, a 1990s block, classic movies, a local
station break, or a general-audience pod. The following fail this gate:

- amateur or personal material mistaken for broadcaster packaging;
- an arbitrary dramatic scene with no self-contained filler function;
- an incomplete opening or ending;
- capture residue, unrelated slates, or several distinct items left as one clip;
- a duplicate or inferior rendition with no additional scheduling value;
- archival novelty whose only rationale is availability or age.

Editorial utility is a bounded proposal, not a new unattended production authority. It is confirmed
by the maintainer against the played output.

### Sparse Gate A content review

[`evidence/filler-reference-content-review-2026-08-31.json`](evidence/filler-reference-content-review-2026-08-31.json)
is the versioned negative-only correction seam for content mistakes already established during
contract review. It binds the exact locked manifest SHA-256. Each finding binds one unique content
SHA-256, one closed exclusion reason, a reviewer identity and time inherited from the artifact, and
at least two distinct in-clip evidence rows that exist on that exact manifest case. Decoder,
filename, collection, uploader metadata, case ID, and source title cannot support a finding.

The audit verifies the whole artifact before screening any case. An unknown or duplicate content
hash, missing or mismatched evidence row, unsupported evidence kind or reason, future review time,
extra field, or input digest mismatch aborts the complete audit. A valid finding adds an exclusion;
it never changes the locked truth, role, taxonomy, or review history. Cases absent from the sparse
review continue through the ordinary general policy. Adding another finding requires a contract
revision and review of the exact content-bound evidence; it is not a place for per-case code.

### 5. Matchability

The proposal contains only grounded production fields:

- `kind`: commercial, promo, bumper, station ID, PSA, trailer, or interstitial;
- production taxonomy assertions over product, format, seasonal, and audience-cue axes;
- `era` when a source-owned date or in-clip signal supports it;
- `audience`: kids, family, general, or late-night;
- `brand` when visible or spoken evidence supports it;
- a plain-language intended channel use.

Reviewer-only free-text labels are mapped through the versioned production-taxonomy mapping. An
unknown label is dropped or held; it never creates a new production taxon.

## Readiness grades

Grades describe the exact prepared artifact, not model confidence, historical importance, or image
resolution alone.

| Grade | Meaning | Candidate-library treatment |
| --- | --- | --- |
| A | Complete, clean, clearly identifiable, grounded, and useful for a defined channel | Eligible for the 32-clip proposal |
| B | Legitimate and playable but niche, visibly degraded, or narrowly useful | Eligible only when its stated use justifies it |
| C | Uncertain boundary, identity, taxonomy, or editorial value | Held outside the proposed 32 |
| Reject | Not filler, broken, incomplete, misleading, duplicated without value, or not reasonably schedulable | Excluded with a stable reason |

A low-resolution era-appropriate commercial may be B rather than rejected. A pristine programme
excerpt is still rejected. Technical polish cannot compensate for the wrong content.

Stable editorial reason codes begin with:

- `not_filler`
- `incomplete_unit`
- `capture_residue`
- `multiple_items`
- `non_broadcast_material`
- `no_scheduling_use`
- `inferior_duplicate`
- `technical_quality_hold`
- `duration_too_short`
- `taxonomy_unresolved`
- `sensitive_placement_hold`

Free-text detail accompanies the code. New codes require a contract revision rather than ad hoc
strings in the reviewer.

## Cleanup contract

Cleanup is conservative and lineage-bound. It never uses generative restoration and never silently
changes the clip's meaning.

1. Inspect the full available source and identify one intended filler unit.
2. Propose `[start,end)` against source time with the evidence for both edges.
3. Preserve opening and closing cards that establish sponsor, schedule, release, station, or break
   function.
4. Remove only verified capture debris, unrelated leaders/trailers, or excess terminal dead time.
5. Produce a mezzanine compatible with Loomarr's existing H.264/AAC, SDR, `yuv420p` playback
   contract while preserving source aspect ratio and cadence where supported.
6. Measure the result with the existing quality evidence. Preview loudness through the same
   production-equivalent `-23 LUFS` handling rather than claiming a different ingest policy.
7. Record source hash, source interval, transformation arguments/version, output hash, and all
   before/after measurements.
8. If either edge or the meaning of a card is uncertain, hold the candidate instead of guessing.

Any change to the output bytes invalidates the prior human decision.

## Selection design

The audit begins with all 300 cases and uses the locked manifest, packet set, rights/download ledger,
production-taxonomy mapping, and sparse content review before any new provider call. The old
adjudication currently yields 261
nominally eligible filler-role cases; those are screened inputs, not 261 accepted clips.

Selection proceeds as follows:

1. Deterministically exclude objective failures and corroborated non-filler roles.
2. Produce a read-only inventory of conflicts, missing production tags, duration/quality concerns,
   likely duplicates, and source concentration.
3. Inspect real video for a replacement pool larger than 32. A target of at least 48 viable
   candidates permits rejection without lowering standards.
4. Prefer useful coverage of the product's default commercial, bumper, and station-ID path while
   retaining strong promos, PSAs, trailers, or interstitials where they add a real use case.
5. Cover materially different audience and era placements. Include strong kids/family examples such
   as Kix and prevent ungrounded-audience material from entering that lane.
6. Avoid duplicate campaigns, near-identical cuts, and source dominance where alternatives exist.
7. Never promote a weaker item merely to satisfy a role, source, or era quota. Record an honest
   coverage shortfall instead.

The first visual triage is frozen in
[`evidence/filler-reference-inspection-seed-2026-08-31.json`](evidence/filler-reference-inspection-seed-2026-08-31.json).
It identifies 50 sources for full playback and explicitly records the current station-ID/bumper
coverage shortfall. Selection at this stage is permission to inspect, not an editorial grade or a
promise that the source will enter the 32. The superseded preliminary seed was bound to historical v2
audit/family inputs; the tracked seed is now regenerated against the v3 Gate A
and duplicate-family identities. Corrected conditioning measurement reaches all 50 selected sources
with no technical holds. That result establishes only a playback-inspection pool—it assigns no grade,
taxonomy, scheduling use, admission, or maintainer acceptance—and the seed is not the sparse
content-review authority.

Duplicate review operates on full-source, order-preserving visual fingerprints rather than four
relative-position frames. The detector ignores near-flat frames, tolerates bounded leaders and
trailers, and requires sustained agreement across at least 70% of the shorter useful sequence.
Connected matches are reported as evidence, not silently collapsed: a non-clique family is held for
inspection because transitive similarity can join distinct cuts. One preferred rendition is chosen
only after full playback. Completeness, lack of capture overlays, correct boundaries, and audible
quality outrank nominal pixel dimensions; every unchosen rendition receives
`inferior_duplicate` or a documented distinct-use justification.

The selection algorithm may rank evidence completeness and measured quality. It cannot assign the
final editorial grade without inspecting the exact video unit.

## Playback acceptance package

The offline reviewer contains one exact output per page and shows:

- HTML5 video playback with audio;
- case number and duration;
- proposed grade and intended channel use;
- proposed kind, era, audience, brand, and production taxonomy;
- concise cleanup summary and any material quality limitation;
- **Yes** and **No** actions;
- a required reason code and short explanation for **No**.

The browser autosaves locally. Export validates 32 decisions and binds each answer to the output
SHA-256, cohort version, reviewer identity, and review timestamp. It does not expose previous model
votes in the acceptance view.

Rejected clips return to a rework or replacement queue. A revised output is a new identity and must
be played again. The final locked cohort contains 32 accepted hashes, not 32 initial proposals.

## Model use and spend

- Existing local and paid outputs are audit inputs, never truth by repetition.
- No new paid OpenRouter inference occurs without a fresh, explicit USD authorization.
- Before new inference, deterministic screening and existing evidence identify the residual cases
  where a model can materially reduce work.
- When inference is warranted, independent model outputs propose role, taxonomy, and editorial
  concerns. Deterministic Go policy resolves schema and taxonomy; the maintainer resolves acceptance.
- The later bakeoff evaluates admission, editorial suitability, taxonomy, and channel placement
  against locked human decisions. It does not score a model against labels the same model authored.

## Phase gates

### Gate A — contract and inventory

- This `filler-reference-cohort-2026-08-31-v3` contract is versioned, linked to [#741](https://github.com/loomarr/loomarr/issues/741), and referenced from active work.
- A deterministic command audits exactly 300 bound cases and emits stable counts and per-case holds.
- The negative-only content review is versioned, manifest-bound, content-hash keyed, and cites only
  exact in-clip evidence rows; mutation, duplicate/unknown content, or unsupported authority fails
  the whole audit.
- Named exemplar tests prove Kix remains held without immutable product evidence and the three
  negative anchors cannot be promoted by filename or collection membership.

### Gate B — prepared proposal

- At least 48 candidates survive real-video inspection or the shortfall is reported.
- Exactly 32 proposed outputs have complete lineage, quality evidence, grade, taxonomy, and use case.
- Every output decodes and passes the objective media gate.
- No proposed output is a programme excerpt or compilation.

### Gate C — maintainer acceptance

- Every decision binds the reviewed output hash.
- No answer is inferred from page navigation or absence of a response.
- Rejections are reworked or replaced until 32 exact outputs are accepted.

### Gate D — reusable method

- The selected OpenRouter path is evaluated on separate unseen candidates after explicit spend
  authorization.
- Results report admission precision, rejection errors, taxonomy accuracy, channel-placement
  accuracy, cost, latency, and residual human-review rate separately.
- The work is complete only when both the accepted cohort and a reproducible method for producing
  another cohort exist.

## Non-goals

- The 32 accepted clips do not certify unattended admission for arbitrary internet media.
- This work does not weaken the independent holdout or rights requirements.
- This work does not define a blanket age or resolution cutoff.
- This work does not turn model confidence into admission authority.
- This work does not require the maintainer to annotate raw evidence or justify accepted clips.
