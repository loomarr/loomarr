# Model-led filler identification plan

Date: 2026-08-31
Status: proposed execution plan for #548, #549, #555, #786, and #787

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
   inside the module.
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

Commission two people other than the maintainer to answer only the two short questions above. Use a
stratified 48-case calibration sample:

- 16 random unanimous model cases, so correlated model mistakes remain detectable;
- 16 model disagreements or abstentions;
- 16 high-risk compilations/programme excerpts and short boundary cases.

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

1. Correct #786 and #787 so they no longer assign 332 blind labels to the maintainer.
2. Implement the factored assessment schema and disagreement report against the existing artifacts.
3. Run the 32-case model-led diagnostic.
4. Run the two-family 300-case relabel only after the diagnostic contract is sound.
5. Build the 48-case calibration package and commission its independent review.
6. Use the resulting development truth to run #555; keep production in shadow until the separate
   holdout passes.

