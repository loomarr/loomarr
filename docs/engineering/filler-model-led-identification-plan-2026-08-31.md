# Model-led filler identification plan

Date: 2026-08-31
Status: active execution plan for #548, #549, #555, #786, and #787; hosted calibration identified
invalid controls and weak local temporal assessors, so the full-corpus run remains paused

## Current execution evidence

PR #793 shipped the factored unit/role schema, strict evidence validation, deterministic comparison,
and two digest-pinned local assessment paths. Gemma 4 and Qwen3.5 each completed the same 32-case
identity-blind package with no operational failures. Their deterministic report found:

- 22/32 unit-structure agreements;
- 7/22 role agreements among cases where role was comparable;
- 7/32 exact agreements; and
- 25/32 cases requiring adjudication.

That is a systemic diagnostic failure, not development truth. It activates the stop rule below: do
not scale either local assessor or their adjudication rate to all 300 cases. A deterministic 15-case
selection now covers four unit disagreements, eight role disagreements, and three agreement controls
(selection SHA-256 `acfb309e7e09ea6efa868e0530c5fc0c642707e277c86dcbc8b006b00f89455f`).
PR #804 shipped a serial, snapshot-bound, zero-data-retention hosted runner with hard request,
per-request, and total-spend ceilings. PR #807 shipped the inference-free three-assessor report. The
first authorized Qwen3.8 attempt pinned the AkashML FP8 route and failed operationally on all 15 unit
requests with provider-side HTTP 502 responses. The same sealed selection then completed without
operational failures on Qwen 3.8/CoreWeave and Claude Opus 5/Amazon Bedrock. Those stronger families
agreed on unit structure for 12/15 cases and on 4/6 comparable roles. Both independently rejected the
supposed standalone/promo agreement control as a programme excerpt; Qwen also rejected the supposed
commercial control as promo. Prompt-v7 error repair improved local exact agreement to 11/32 but
worsened unit agreement to 19/32, with 13 Gemma-standalone/Qwen-programme-excerpt confusions. The
small local families and two old controls are therefore not temporal truth authorities.

### 48-case truth-review evidence execution

Issue #843 now owns the inference-free bridge into Phase 2. The deterministic selector strictly
joined the frozen 300-case draft, three reviewer packages and private maps, and the Mistral, Gemini,
and composite Gemma submission artifacts. With seed
`filler-temporal-truth-review-2026-09-01-v1`, it produced 48 unique content identities: exactly 16
agreement probes, 16 non-risk disagreement probes, and 16 high-risk probes, including exactly four
each for programme excerpts, compilations, unusable/unclear cases, and sub-15-second boundaries.
The private selection SHA-256 is
`1de478f5a9fda04944b93a744eeddeff3dcc65e262b55fe9b32da51738e552af`.

The first complete evidence build used original acquired media, not the old provider derivative,
and passed all 48 cases. It produced 469 ordered frames, 47 content-bound transcript artifacts, and
708 Apple Vision observations on 226 frames. Review videos cover the complete measured span from
9,783 to 120,000 ms; the largest is 41,248,821 bytes under the declared 64 MiB ceiling. The public
manifest SHA-256 is `f2182f358fe1d266391f6e776367496d95db3f5fbe7123995da980ae5dee588e`;
the owner-only map SHA-256 is
`53c68da97ef9642c3b5b1b7d1b893d96d954b5561d1d5e5e9c343c4431a90b5a`.
Every declared public frame/video hash and byte count re-verified, and an exact search found no
private case ID or source filename in the public tree. This artifact is evidence only and makes no
truth, accuracy, or certification claim.

Reproduce selection with `make filler-temporal-truth-select` and evidence with
`make filler-temporal-truth-prepare`, supplying the required `LOOMARR_FILLER_TRUTH_*` paths listed by
each target. The evidence command additionally pins generation time, ffmpeg/ffprobe executable
hashes, the optional OCR executable/source hashes, scene threshold, timeout, frame ceiling, and video
ceiling. Output paths are immutable and publication is one atomic directory rename.

## Decision

Use multimodal models as the primary classifiers and evidence extractors. Do not ask the maintainer
to blind-label the 300-case development corpus. Human judgment is a measurement instrument for a
small calibration sample and, later, a contracted certification dataset; it is not the ordinary
classification path and it is not an ongoing operator workflow.

Do not fine-tune a model yet. First prove the best prompted evidence cascade on the existing corpus.
Train a narrow challenger only if the residual errors are systematic, the evidence is sufficient,
and at least 1,000 independently reviewed, group-separated examples exist without touching the
locked holdout.

## The actual classification problem

The recovered review runs mixed three questions into one large label:

1. Is the media technically usable?
2. Is the selected span one standalone, playout-safe unit or a container/excerpt?
3. If it is standalone filler, what role does it serve?

That shape hid the dominant failure. Reviewer A labeled 277/300 cases eligible while Reviewer B
labeled 153/300 eligible. Reviewer B also found 32 compilations and 73 programme excerpts, compared
with one compilation and 13 excerpts from Reviewer A. The disagreement is primarily about temporal
unit structure, not whether models can recognize an advertised product.

The model output must therefore be factored into two closed assessments:

- `UnitAssessment`: `standalone | compilation | programme_excerpt | unusable | unclear`, with
  observed transitions and timestamped evidence;
- `RoleAssessment`: only for a standalone unit, exactly one of `commercial | promo | bumper | psa |
  station_id | trailer | interstitial | unclear`, with timestamped evidence.

Brand, product, era, audience, and taxonomy are separate additive claims. They must not be required
to answer whether a clip is one safe filler unit, and their free-form detail must not make two
otherwise matching role labels appear to disagree.

## Module design

Keep one deep admission module behind the existing deterministic seam:

1. The evidence implementation produces a closed, content-addressed packet: decoder facts, ordered
   temporal frames, timestamped transcript, OCR, and bounded derivative identities.
2. Model adapters implement two internal inference roles: temporal structure and semantic role.
   Provider-specific request shapes, retries, structured-output repair, attribution, and cost remain
   inside the module. Hosted provider output is limited to the closed answer and package-owned
   decisive signal IDs; the adapter derives the required audit sentence locally. Free-form provider
   prose is not part of the hosted seam because it has broken nominally strict JSON without changing
   the underlying classification.
3. `filleradmission.Evaluator.Evaluate` remains the sole terminal decision interface. It consumes
   validated facts and returns `admit | reject | review` or an operational hold. It never reads a
   model's self-reported confidence.
4. `fillereval` scores captured assessments against a locked truth set. Production code and tests
   exercise the same admission interface; evaluation does not create a second decision ladder.

The external interface stays small: give the module a versioned evidence document and receive a
decision with reasons, evidence references, conflicts, and attribution. Individual prompts and
provider calls are internal seams because they vary; callers do not orchestrate the cascade.

## Evidence route

Run the cheapest sufficient evidence in order:

1. Deterministic decoder, duration, duplicate, rights, and policy checks.
2. Twelve ordered temporal frames plus timestamped transcript and OCR for unit structure. Four
   generic frames were enough to recognize subjects but not consistently enough to distinguish a
   standalone advert from a mixed 120-second sample.
3. Transcript plus selected near-full-resolution frames for semantic role and grounded attributes.
4. Bounded direct video only when the temporal assessor reports a named ambiguity such as mixed
   programme/filler transitions or several apparent units.
5. A second model family when the first assessment is unclear, contradicts deterministic evidence,
   or enters a safety-critical structural slice.
6. A premium model adjudicator only for a remaining model disagreement. It receives the evidence and
   the two cited assessments; it does not convert an operational failure into a semantic answer.

Every model may abstain. A schema failure, unavailable provider, budget exhaustion, or missing
modality is an operational hold, never `invalid` and never an automatic admission.

## Phased execution

### Phase 0 — repair the measurement contract

- Replace the free-form taxonomy comparison with exact comparison of `UnitAssessment` and
  `RoleAssessment`.
- Produce a deterministic confusion report for the existing Mistral, Gemini, and Gemma artifacts by
  duration, source lane, unit structure, and content role.
- Keep the existing 300-case lock as immutable candidate-label history, not accepted truth.
- Recast the 32-case temporal package as a diagnostic set. Re-run it through the new model roles
  before asking for any manual judgment.

Gate: the same inputs produce byte-identical packets and scoring; every disagreement is assigned to a
named structural or role confusion; no free-form taxonomy difference counts as a role disagreement.

Implementation and reproducibility passed. Hosted calibration separated transport defects, weak
local models, and five bounded contract/evidence gaps, but it also proved that two old agreement
controls are unsafe. Phase 0 therefore remains at its declared repair boundary: replace the invalid
controls and independently review only the disputed/high-risk slice before any 300-case run. Do not
keep tuning prompts against the same 32 cases.

### Phase 1 — model-led 300-case relabel

- Run two independent model families over all 300 packets with the same closed schema and evidence.
- Automatically accept a development candidate label only when both agree on unit structure and
  role, all cited evidence references validate, and deterministic policy finds no contradiction.
- Route disagreements through one separately versioned adjudicator.
- Do not hide a weak pipeline behind adjudication: if more than 15% of cases need adjudication, or one
  confusion accounts for more than half the disputes, stop and repair the evidence/prompt contract
  before another full run.
- Record request count, latency, exact model/provider route, charged cost, schema failures, abstention,
  and per-slice agreement.

Gate: 300 complete model assessments; zero invalid evidence references or open-vocabulary roles;
model agreement and adjudication rates reported by structural slice.

### Phase 2 — small independent calibration, not a 300-case human audit

Select the calibration sample before any new stronger-model or human answer exists. The immutable
Mistral, Gemini, and Gemma 300-case recovery history is selection evidence only, never truth. Build a
stratified 48-case set from that frozen history:

- 16 exact three-artifact unit/role agreements, so correlated historical mistakes remain detectable;
- 16 historical disagreements or ambiguities; and
- 16 high-risk compilations, programme excerpts, unusable/unclear media, and short boundary cases.

The selector uses a declared seed and SHA-256 ranking over content identity, fails rather than silently
borrowing from an underfilled quota, and records the digest of every input. The old recovery schema
is normalized conservatively: deterministic-invalid becomes `unusable`; ambiguous becomes `unclear`
even when the same row suggests a structural role; explicit programme excerpt or compilation comes
next; only then may an eligible row become `standalone`. This precedence prevents the old conflated
truth/role fields from promoting an ambiguous or structurally unsafe span into a control.

Materialize new evidence from the complete acquired media, not the old four-frame/60-second reviewer
derivatives. Each opaque case gets one complete-span bounded review video, at most twelve ordered
near-full-resolution frames including boundary and scene-change evidence, a content-hash-bound
timestamped transcript when audio exists, and optional local OCR bound to the exact frame hashes.
Source filenames, old aliases, case IDs, labels, assessor identities, and selection reasons remain in
the owner-only map and selection ledger. The public package is content-addressed and published only
after every derivative verifies.

Only after that evidence is sealed do the stronger Qwen and Claude families and two people other than
the maintainer answer the same two short questions in fresh, independently shuffled blind batches.
Neither model answers nor old recovery labels are shown to the human reviewers. This order avoids a
circular sample definition in which strong-model disagreements would be required to select the very
calibration evidence intended to validate those models.

The reviewer sees ordered frames, OCR, transcript, and optionally the bounded clip, but not model
answers, source identity, filenames, or prior labels. The UI should support autoplay, speed control,
keyboard answers, and one decisive timestamp. Detailed taxonomy authoring is out of scope.

Gate for development use: zero false `standalone` results on the high-risk structural slice; at
least 95% agreement with the independent final assessment on standalone-versus-not; at least 90%
role agreement among standalone cases; every miss has a named error class. These are development
selection gates, not production certification claims.

If the gate fails, expand only the failing slice and revise the evidence route. Do not default to
reviewing all 300.

### Phase 3 — candidate and cascade bakeoff

- Score economical local/open-weight and hosted candidates on identical packets and the repaired
  development labels.
- Compare single-model, two-model fallback, frame, and direct-video-on-ambiguity routes.
- Select on action-specific precision, automation coverage, worst-slice behavior, latency, and total
  cost per 1,000 clips. Do not use self-reported confidence.
- Preserve the 300-case set as development-only. It may select a route but cannot authorize
  unattended production admission.

Gate: one preferred cascade and one fallback policy, with a complete error/cost report and no
uncounted provider failures.

### Phase 4 — production certification without maintainer labeling

The independently clustered holdout remains necessary because no model can grade itself. Contract
the narrow unit/role labeling work rather than handing it to the maintainer. The review UI and schema
from Phase 2 keep each judgment short; richer product/taxonomy claims are evaluated separately.

The holdout stays unopened to candidate development, preserves source/campaign/similarity separation,
and is large enough for the predeclared one-sided confidence gates in §10. Model-generated evidence
may prepare frames, OCR, and transcripts, but the blinded reviewers do not see candidate answers.

Gate: the existing #548 thresholds pass, including the confidence lower bounds and each
safety-critical slice. A small sample cannot substitute for this production claim.

### Phase 5 — shadow rollout

- Run the certified cascade in shadow before it controls filing.
- Audit every rung disagreement and a random sample of agreements.
- Promote deterministic rejection first, then only the semantic slices that independently pass.
- Track false admission, false rejection, abstention/review rate, provider failure, p95 latency, and
  cost per 1,000 clips.
- Re-run affected evaluation whenever the model, provider, prompt, evidence extractor, schema,
  taxonomy, or admission policy changes.

The target operating experience is automatic classification for ordinary clips and one concise
question only for genuine ambiguity. Calibration and certification are release engineering work,
not routine operator work.

## Fine-tuning decision gate

Fine-tuning becomes a useful experiment only when all of these are true:

- the prompted cascade misses a predeclared accuracy, latency, privacy, or cost target;
- the residual error is systematic and not caused by missing temporal evidence or an unclear label;
- at least 1,000 clean, role-balanced, campaign/source/similarity-grouped training examples exist;
- a separate development validation split and the locked certification holdout remain untouched;
- the trained artifact can be pinned by model, weights, quantization, runtime, and prompt/schema
  identity and can emit the same evidence contract.

At that point compare one narrow fine-tuned multimodal/open-weight extractor with the prompted
cascade. Do not train a general Loomarr LLM, and do not grant a learned score direct admission
authority.

## Immediate work order

1. Complete: #786 and #787 no longer assign 332 blind labels to the maintainer.
2. Complete in #793: implement the factored assessment schema, evidence validation, and deterministic
   disagreement report.
3. Complete: the sealed 15-case slice ran through Qwen 3.8 and Claude Opus 5 with zero final
   operational failures. It identified five bounded error classes and proved two old controls unsafe.
4. In progress in #843: deterministically select 48 cases from immutable recovery history and build
   their stronger, complete-span, identity-blind evidence. Do not use fresh Qwen, Claude, or human
   answers to select the sample.
5. Pending in #846: give that same sealed evidence to Qwen, Claude, and two independent human
   reviewers through fresh blind batches with the two-question submission lock. This is a short
   unit/role review, not a blind 300-case audit.
6. When the 64 GB Apple-silicon host is available, benchmark pinned Qwen 3.8 27B MLX against the same
   sealed diagnostic and hosted Qwen result; promote it only if it reproduces the stronger-family
   behavior.
7. Pending the repaired Phase 0 gate: run the two-family 300-case relabel. Do not substitute mass
   adjudication for a passing diagnostic.
8. Pending calibrated development truth: run #555 and keep production in shadow until the separate
   holdout passes.
