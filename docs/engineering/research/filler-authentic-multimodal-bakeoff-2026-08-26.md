# Authentic filler multimodal bakeoff

Date: 2026-08-26

## Decision

Do not infer filler taxonomy from sparse frames alone. Continue with a larger stratified bakeoff only
after the model output contract uses the exact filler-role enum and the evidence ladder adds transcript,
audio, or bounded video. Keep inference evidence-producing; deterministic policy remains the admission
authority.

Claude Sonnet 4.6 with authentic source metadata is promising enough to continue testing. GPT-5.4 mini
under the current literal-evidence prompt is not.

This was a paid exploratory bakeoff, not the schema-v5 certification run. It answers whether the
multimodal approach is viable before acquiring and labeling the complete locked corpus; it does not
change the certification thresholds or satisfy issue #555.

## Corpus and method

The sample contained 11 authentic Internet Archive clips spanning commercials, PSAs, promos, a bumper,
a station ID, a trailer/TV spot, and an intentionally invalid mixed compilation. Each clip contributed
four JPEG frames at 10%, 35%, 65%, and 90% of duration.

The 44-cell matrix crossed:

- Claude Sonnet 4.6 and GPT-5.4 mini through pinned OpenRouter providers with no fallback;
- frames only; and
- the same frames plus the archive title, creator, date, and collections.

The existing evidence prompt required literal, signal-grounded facts and allowed abstention. Role scoring
normalized literal equivalents such as `advertisement` to `commercial`, `public service announcement`
to `psa`, and `TV spot` to `trailer`. All 44 final cells returned valid structured predictions.

## Results

| Model and evidence | Role coverage | Role accuracy | Known-date accuracy | Known-brand accuracy |
| --- | ---: | ---: | ---: | ---: |
| Claude Sonnet 4.6, frames | 4/11 (36%) | 4/11 (36%) | 1/10 (10%) | 8/9 (89%) |
| Claude Sonnet 4.6, frames + metadata | 11/11 (100%) | 9/11 (82%) | 10/10 (100%) | 9/9 (100%) |
| GPT-5.4 mini, frames | 2/11 (18%) | 2/11 (18%) | 1/10 (10%) | 2/9 (22%) |
| GPT-5.4 mini, frames + metadata | 4/11 (36%) | 4/11 (36%) | 8/10 (80%) | 6/9 (67%) |

Claude's two metadata-assisted role errors were material: an Easter Seals PSA became an advertisement,
and the mixed commercial/promo compilation became a commercial. Sparse frames also confused a news
bumper with a news broadcast and did not reliably distinguish promos, IDs, trailers, and compilations.

The final prediction matrix cost $0.16435770: $0.14178600 for Claude and $0.02257170 for GPT-5.4 mini.
Total account usage for execution was $0.46135845 because live route and adapter failures forced
diagnostic reruns whose outputs were discarded. The final matrix used 51,492 prompt tokens and 5,704
completion tokens.

## What the run exposed

1. Sparse frames identify brands much better than roles. Role depends on temporal structure, narration,
   and calls to action that four frames frequently miss.
2. Source metadata is valuable but cannot be gold. Titles made classification much better while still
   producing the two consequential Claude errors.
3. `content_role` currently has a free-form value. Models returned phrases such as `news broadcast`,
   `Message`, and copied titles. The next contract needs the exact taxonomy enum plus `unknown`.
4. Each media part must carry its immutable signal ID immediately before the payload. The old request
   shape listed images without binding them to IDs, causing invented attributions.
5. Live endpoint metadata is not sufficient preflight. The run encountered an account-level ZDR policy
   mismatch, an upstream shared-pool rate limit, unsupported optional request parameters, and a stable
   requested model resolving to a versioned deployment ID. The certification runner must prove one
   cheap request on the exact locked route before reserving a full matrix.

## Next bakeoff

Use a stratified development slice with enough cases per role to calculate per-class precision and
recall. Compare metadata-only, frames plus metadata, transcript plus metadata, and bounded video. Measure
compilation rejection separately, include an explicit abstention score, and require a preflight ledger
before paid execution. The locked schema-v5 holdout and issue #555 thresholds remain the certification
boundary.
