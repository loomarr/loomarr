# Filler certification protocol (V61, archived)

Archived verbatim from `docs/design.md` §10 (V61, "Evidence-based classification and terminal
readiness") in #779 (2026-09-27). Not current guidance: the runtime readiness rules, routing and
accounting are in [`docs/design/filler-classification.md`](../../docs/design/filler-classification.md).
This is the certification-artifact, bakeoff, corpus, spoken-safety, review-packaging and
rights-review protocol that the dated certification reports in `docs/engineering/` cite.

Certification artifact schema v5 requires the per-inference-step ledger and the corpus-diversity
identity used by the statistical contract. Earlier schemas are rejected: no completed bakeoff
artifact depends on them, and preserving speculative compatibility would create an untested path
that can hide multiple calls behind one terminal attribution or bypass the diversity gate. The v5
prediction wire therefore has no scalar inference fields: every attempted call is a `steps` entry;
only deterministic outcomes and holds reached before a provider attempt may have none.

The inference-spending bakeoff reads a **raw evidence packet**, never the manifest's reviewed
`Evidence` or terminal labels. The packet is a closed, content-addressed input containing only
deterministic decoder/source-policy facts and bounded untrusted source text, transcript, OCR, frame,
audio, or video derivative references. Every referenced external derivative carries its hash and
measured byte/pixel/duration bounds. Packet identity must match the case's `evidenceSha256`; the
runner re-opens every derivative beneath one declared corpus root, refuses symlink escapes, and
verifies its exact bytes and hash. It refuses missing, extra, changed, or label-bearing fields before
calling a provider. The internal case ID remains a local ledger join and is omitted from provider
prompt content. This separation keeps the scorer's answer key and identity-correlated shortcuts out
of prompts and makes the exact provider input replayable.

One bakeoff run accepts a locked manifest, an exact packet set, a versioned admission policy, an
ordered role/rung route, and positive request/spend/concurrency ceilings; it emits an immutable
prediction ledger. Command boundaries that read a complete immutable JSON evidence or attestation
artifact use exact, case-sensitive member names and reject unknown or duplicate members, invalid
UTF-8, and trailing values; `null` and empty collections retain their distinct wire meanings. Writers
remain on the established canonical encoding so strengthening a reader cannot change an artifact's
bytes or SHA-256. The runner is serial by default. Before each call it reserves that rung's
predeclared maximum nanodollar charge and request count, then reconciles the provider-reported exact
charge without binary floating point. A call is refused when its reservation cannot fit. If a failed
call omits or corrupts settlement, its exact charge remains missing while the distinct recorded
reservation stays consumed for that run. Every
attempt is retained as a separate inference step, including route, modalities, derivative bounds,
tokens, charge, latency, attempts, generation id, and operational failure. The terminal prediction
aggregates those steps but never collapses a cascade into one falsely attributed rung.
Certification routes authorize exactly one provider attempt per invocation; an adapter may not hide
internal retries inside an aggregate attribution. A retry is a new runner invocation and ledger step.

An OpenRouter certification run is also bound to one immutable metadata snapshot fetched no more
than 24 hours before its declared run time. The snapshot makes one bounded authenticated model-catalog
request, one bounded ZDR-list request, and one bounded endpoint request per concrete candidate. It
freezes both the requested model ID and catalog canonical revision, model modalities, endpoint
selector slug, distinct returned provider name, strict-output parameters, status, exact price text,
and ZDR membership. Its SHA-256 is both the run's capability and price identity. Every route must bind
its requested and resolved model identities and resolve exactly within that snapshot before media is opened or spend is reserved; a
human-readable snapshot label, catalog-level capability claim, or provider-family name is not proof.

Routing is reason-driven rather than confidence-driven: deterministic and text evidence run first;
frames may run only for a predeclared missing/conflicting claim; direct video may run only for a
named temporal ambiguity; premium escalation may run only for a predeclared unresolved reason whose
measured value justified its marginal ceiling. After each rung the same pure
`filleradmission.Evaluator` evaluates the accumulated document. A terminal semantic result stops the
cascade; a provider, route, schema, extraction, or budget failure becomes an operational hold in the
ledger and can never be converted into `admit`, `reject`, or human review.
Route classes enforce this order: text is first, then frames, direct video, and premium; frame routes
accept only named visual claim gaps/conflicts; direct video accepts only `temporal_ambiguity`; and a
premium route must name and hash the frozen measurement artifact that justified its marginal ceiling.
Each request contains only signals whose modality the route declares; attribution must match that exact
set, so a cheaper route cannot receive or claim a more expensive modality.

Model roles are certified independently: lineup, filler text, filler frames, filler video, and
transcription may have different accuracy/cost frontiers. A normal install follows the last
certified role policy automatically; overrides are Advanced controls. Certification pins concrete
model and provider identities, disables fallback, requires the requested structured-output
parameters, snapshots capabilities/prices/privacy, and uses identical evidence across candidates.
Production resilience may use explicitly allowed fallback, but records the resolved endpoint and is
measured separately. A moving `latest` alias cannot authorize unattended filler decisions.

The evidence cascade is cost- and resource-bounded: deterministic checks, text/transcript/OCR,
near-full-resolution scene/end-biased frames, direct bounded video only for named temporal
ambiguity, premium escalation only where measured value exceeds marginal cost, then review. The
320 px UI preview is never semantic/OCR evidence. Hosted derivatives strip container metadata and
have hard frame, pixel, duration, byte, token, retry, concurrency, per-clip, daily, and evaluation
ceilings. Shared-appliance inference is serial by default. Privacy/provider constraints never relax
silently; an ineligible route leaves the clip held.

The default frame derivative is exactly four ordered JPEGs from bounded representative windows at
5%, one-third, roughly two-thirds, and 90% of the measured clip or segment span. Each window decodes
at most three seconds and the final window is the closing-card bias. Frames preserve their native
size through 1920 px wide (never upscale, preserve aspect ratio) and are encoded sequentially. This
is an extraction ceiling, not permission to send all four frames: the evidence router may send a
strict subset when a cheaper claim is already answered. A missing/invalid duration refuses visual
extraction rather than falling back to an unbounded decode.

The default hosted-video derivative is one metadata- and chapter-stripped MP4 containing H.264 video
and optional AAC audio. It spans at most 60 seconds of the measured clip or segment, fits within
1280×720 without upscaling or changing aspect ratio, and is at most 12 MiB before base64 expansion.
Extraction and upload are sequential, encoding uses one ffmpeg thread, and a size-limited reader
aborts encoding before buffering more than the ceiling. The request caps model output at 512 tokens
and refuses a response body over 256 KiB. The direct-video provider is a separate capability from
text and frame vision. The OpenRouter certification adapter accepts base64 transport only, a
concrete namespaced model whose
live metadata advertises video input, and one explicitly pinned upstream provider; it disables route
fallback, requires supported request parameters, and denies provider data collection while requiring
zero-data-retention routing. URL transport, thin or stale capability metadata, invalid measurements,
and any exceeded bound fail closed before a hosted request. This adapter supplies evidence only and
does not itself decide or change production admission.

Certification uses a versioned, source/similarity-separated development corpus and locked holdout.
The maintained contract requires at least 300 development cases and 1,126 independently clustered
holdout cases: 446 eligible positives, 446 deterministic-invalid controls, 147 semantic-invalid
controls, and 87 genuinely ambiguous, answerable-review cases. The eligible positives include at
least 82 commercials, 82 promos, 59 bumpers, 59 station IDs, 82 trailers, and 82 PSAs. No creator may
supply more than 10% of one eligible role, and no source may supply more than 25% of
eligible holdout cases. A holdout similarity cluster, campaign, and source master each contribute
exactly one case. Preparation derives source-family identity from the source authority and item ID;
alternate segments, encodes, and derivatives of one master therefore cannot pretend to be independent
observations. Campaigns and source families cannot cross the development/holdout split, and
near-duplicates cannot cross it either. It reports action-specific precision and coverage,
worst-slice results, review answerability, conflicts, schema/grounding/security failures, calibration,
latency, and total operating cost with confidence bounds. Unattended behavior expands only after the
predeclared gates in the certification artifact pass: zero observed prohibited admissions,
instruction escapes, or ungrounded taxonomy values; at least 99% observed auto-admit precision; at
least 99% deterministic-reject and 97% semantic-reject precision; then at least 90% valid-filler and
95% invalid-input automation with at most 10% review. Each safety-critical slice has its own gate.
Admission, deterministic-reject, semantic-reject, valid/invalid automation, and review-answerability
gates require both their point estimate and one-sided 95% Wilson lower bound. The precision and
answerability denominators above let one observed error
remain within each 99%, 97%, and 95% lower-bound target; two errors do not. The point estimates alone
do not certify a small corpus.

A certification manifest is itself content-addressed and records its lock time. Every case locks the
source media and captured evidence packet by SHA-256 plus item-level provenance: source authority,
stable item and media URLs, retrieved metadata hash/time, exact rights statement, rights decision
and reviewer, source representation/size, and bounded segment. Redistribution permission is
explicit; media that cannot be redistributed stays outside Git. A collection name or missing rights
field never implies permission. Each semantic reviewer receives an independently shuffled packet
whose random opaque aliases expose content/evidence hashes and bounded segment coordinates but no
internal case ID, split, cluster, creator, campaign, source filename, or labels. An owner-only alias
map binds that batch to the exact draft digest and is required for mechanical unblinding; reviewer
submissions use aliases, never case IDs. Two distinct reviewers in distinct blind-review batches
submit immutable label hashes covering disposition, reject class, content role, taxonomy, policy
flags, evidence spans, and any review question. The original blind submissions remain visible. Matching
submissions become the final labels directly; a disagreement requires a reasoned final adjudication
by a third identity. Rights adjudication and semantic labeling are separate records.

Eligible and semantic-invalid labels require one role from the closed review vocabulary. A
deterministic-invalid or ambiguous label may leave the role empty only when the supplied evidence
cannot establish it; there is no legacy `unknown` role and reviewers never invent a filler type merely
to fill the field. One bound signal may support multiple distinct evidence claims, such as a
transcript supporting role, brand, and product; only an exact repeated evidence record is invalid.

A semantic reviewer may be a person or a model-backed review run, but a model-backed identity is
valid only when a separate immutable attestation binds the exact blind-package manifest, prompt,
provider, concrete model identity, its local digest or hosted capability-snapshot identity,
transcript-set identity when used, completion time, per-case
latency and token accounting, and the completed submission hash. The runner receives only one
reviewer package, re-hashes every supplied signal, joins a transcript by the package audio hash rather
than exposing a case ID, runs serially with one attempt and a per-case timeout, and atomically
publishes the attestation and complete JSONL submission only after all 300 cases validate. Review A
cannot see review B, either submission, an alias map, source identity, or candidate output. The exact
model family used for review or adjudication is excluded from every scored candidate and cascade in
that corpus generation; changing a quantization or provider does not erase that exclusion. A third
model-backed adjudicator receives only the two completed labels and the same blind evidence for a
disputed alias, never candidate predictions. Model agreement is therefore review evidence, not an
automatic claim of truth: the locked development corpus can select candidates but cannot certify
production, and the independently clustered holdout repeats this label protocol before it is opened
to candidate scoring.

Before another full development lock, the 32-case temporal diagnostic factors the previously
conflated model output into two claims. `UnitAssessment` answers whether the bounded span is one
standalone unit, a compilation, a programme excerpt, unusable, or unclear. `RoleAssessment` exists
only for a standalone unit and answers commercial, promo, bumper, PSA, station ID, trailer,
interstitial, or unclear. Each non-operational answer cites only signal IDs from the identity-blind
packet; the validator resolves those IDs to their package-owned timestamps. Provider, schema,
budget, and transport failures remain explicit operational failures rather than semantic labels.
The local runner makes one constrained unit call for every case and a separate constrained role call
only after the unit call returns `standalone`; it records every call's axis, response hash, latency,
tokens, and failure rather than collapsing a two-call cascade into one attempt.
The hosted adapter's provider-facing schema contains only the closed class and one to four package-
owned decisive signal IDs. Free-form explanatory prose is deliberately not accepted over that seam:
routes advertising strict JSON have emitted unescaped titles, HTML fragments, and trailing braces in
otherwise usable explanations. The adapter derives the assessment's required audit sentence from the
closed class and cited IDs after strict decoding. This preserves the decision and its evidence while
preventing non-decision prose from turning a valid classification into a transport failure. Local
diagnostic adapters may retain a model-authored sentence, but comparison and disposition never use
sentence wording as a vote or confidence signal.
Two model families run independently on the exact packet, and the deterministic comparison reports
unit agreement separately from role agreement. More than 15% disputed cases, or one confusion that
accounts for more than half the disputes, stops the development run for contract repair instead of
being hidden behind adjudication. This diagnostic artifact is non-certifying and does not replace the
complete label or holdout contracts above.

The current temporal-structure gate is a private, mechanically constructed 60-case holdout; it does
not require another full blind human viewing pass. Its planner strictly decodes and hashes the exact
48-case selection, evidence manifest and private map, locked human assessment and attestation,
full-decode media-quality report, two-family broadcast-suitability comparison, the exact 300-case
reference audit behind the duplicate-family audit, that audit's exact digest-bound download ledger,
that recomputed duplicate-family graph, a content-bound transition-edge authority, and the
programme-parent inventory. The download ledger is bounded to 16 MiB, decoded only by the
reference-audit hostile-input decoder, and joins every audit case exactly by case ID, item ID,
content SHA-256, and canonical relative local path where present. An audit case's `Source` is a
dataset label, not provenance authority: the digest-bound ledger alone supplies the authority and
canonical HTTPS item URL used for known-filler lineage exclusion. It selects 12
quality-eligible, non-prohibited standalone anchors with two bumpers, three commercials, two promos,
two PSAs, and three trailers. Every anchor has distinct source bytes and duplicate-family identity;
the family authority covers every non-excluded reference case, not merely the later 48-case
selection, and the planner independently recomputes its canonical relationship graph before use.
The replacement certification cohort has 60 cases: 12 standalone units, 12 two-item compilations,
12 three-item compilations, 12 programme excerpts, and 12 programme excerpts with one inserted
filler unit. The combined construction receipt uses schema 3 and contract
`filler-temporal-structure-holdout-plan-v7`: it retains the accepted provenance, transition, and
holdout-exclusion authorities while binding the expanded cohort. Older receipts are not upgraded
by inference and cannot authorize construction under this contract.

Replacement planning receives new source material through one immutable **replacement candidate
pool**, never through the historical programme inventory or a second raw inventory. The leaf
`fillercandidatepool` package owns the strict schema, canonical validation, and digest of
`filler-replacement-candidate-pool-v1`. A separate composition package owns its one build interface
and may depend on `fillercandidatepool`, `fillercorpus`, `fillerquarantine`, `fillerreference`, and
`fillerreview`; the planner depends only on the leaf pool contract. This direction keeps quarantine
and review independent of their consumer and avoids the existing `fillerquarantine` to
`fillerreview` dependency becoming a cycle. The review package exposes one normalized, read-only
authority view from its existing strict loaders so the builder reuses the owning validators instead
of copying their policy.

The builder considers every row in one current schema-v5 corpus inventory and emits exactly one
sorted candidate record for it. Each record has the closed kind `standalone_anchor` or
`programme_parent`, the closed disposition `eligible` or `held`, and sorted unique hold reasons.
Structural corruption or a digest-swapped authority invalidates the entire build. A valid authority
that denies one candidate instead produces a held record, so absence cannot disguise a failed
qualification gate. Eligible records bind the case, source content SHA-256, byte count, duration,
canonical path beneath the declared source root, transport, exact source authority/item/page/media
identity, metadata digest, soundtrack expectation and frozen evidence, duplicate-family identity,
and the raw digest of every authority used to derive the record. Standalone records additionally
bind the locked semantic unit and role plus their measured transition edges. Programme parents have
no standalone role or transition claim. Adapter labels such as `PSA` and `Trailer`, hand-authored
pool fields, and projected worksheet columns are never semantic, rights, quality, soundtrack, or
audience authority.

The authority matrix is closed and transport-aware:

| Candidate | Required authority |
|---|---|
| Remote standalone anchor | Current inventory; applicable development or certification rights lock; exact materialization ledger; exact quarantine-inspection report; inspected source bytes; full-decode audiovisual quality; bound `present_expected` soundtrack claim and decoded audio; duplicate-family and prior-exposure clearance; explicit suitability/audience clearance; locked `standalone` unit and role; measured transition edges |
| Local standalone anchor | The same authorities except that the inventory's direct local representation replaces the materialization ledger and quarantine report; source bytes are still confined, rehashed, and fully decoded |
| Remote programme parent | The remote standalone authorities except semantic role and transition edges; explicit programme-parent kind authority replaces the standalone semantic claim |
| Local programme parent | The local standalone authorities except semantic role and transition edges; explicit programme-parent kind authority replaces the standalone semantic claim |

`present_expected` remains pre-download selection evidence; eligible candidates must also have an
audio stream and a successful full-source decode. `intentionally_silent`, `unknown`, absent audio,
failed or incomplete decode, prohibited suitability evidence, held rights, missing quarantine
evidence for remote bytes, duplicate or related-family collision, and every prior-exposure form have
closed hold reasons. Prior exposure covers exact source bytes, duplicate family, and programme
provenance across the cumulative adjudication chain. A family with any exposed or otherwise eligible
member resolves deterministically to at most one eligible representative; the others remain visible
as held collisions. Local media never receives a fabricated download ledger or quarantine report,
but no downstream authority is waived because of its transport.

Pool construction strictly reopens the generic inventory, rights decision, materialization ledger
where applicable, quarantine report where applicable, source bytes, quality, soundtrack, semantic,
transition, family, suitability, and cumulative prior-adjudication authorities through their owning
decoders. It binds their raw hashes as sorted named inputs and hashes canonical JSON with a fixed
build time. The same inputs and build time produce byte-identical output and digest. Frozen CDC,
Blender, USGS, and LOC fixtures exercise this join without network access. Building the pool creates
no request, download, media mutation, model call, training data, catalog item, schedule, certification
result, or production-admission grant; all corresponding authority flags are present and false.

Genesis planning retains the schema-v3/v7 receipt and its exact historical selection, review,
reference, transition, and programme-inventory inputs so the burned challenge remains reproducible.
Replacement planning uses the next current receipt contract, requires one exact current pool plus
the complete prior-adjudication chain, rejects every historical programme inventory and all raw
candidate-authority paths, and selects both anchors and programme parents only from eligible pool
records. It independently reproduces the pool digest, requires the pool's prior-exposure binding to
equal the reopened cumulative chain, and records the pool hash as `replacement_candidate_pool` in
the receipt. Candidate selection remains seed-deterministic, while the cumulative future-training
exclusion continues to include prior and newly selected source hashes, families, and programme
provenance. Historical pool or receipt schemas remain readable evidence only and are never upgraded
by inference.

**Transition evidence is a prior measurement authority, not a label inferred from the chosen
corpus.** A development-only generator measures all 48 exact evidence cases before role, safety, or
quality eligibility is consulted. It binds the evidence-manifest and private-map hashes, each exact
source SHA-256 and duration, the resolved FFmpeg path/version/binary SHA-256, one fixed generation
time, and the common renderer profile (960-by-720 aspect-preserving padding, 30 fps, yuv420p,
48 kHz stereo). For both the first and final 1,000 ms it retains the PTS-normalised, window-clamped black and
silence intervals plus RMS and peak support over the boundary-adjacent 100 ms. The detectors are
fixed at `blackdetect=d=0.040:pix_th=0.10` and `silencedetect=n=-40dB:d=0.040`; an interval touches a
boundary only when it begins or ends within one rendered frame (34 ms) of that boundary. Missing or
extra cases, repeated case/source identities, changed bytes/duration/tool/profile/policy, malformed
intervals, a missing required stream, or any decode failure invalidates the complete authority.
Generation performs no semantic classification, inference, rendering, training, or admission.

The earlier words *abrupt*, *black-frame*, and *audio-continuous* mixed a construction property with
two overlapping media cues and claimed more than an amplitude detector proves. Every compilation is
already a hard concatenation. Quota selection therefore uses three closed, factual strata derived
from the two independently measured edges, in precedence order: `black_boundary` when either edge
has boundary-touching black; `audible_nonblack_cut` when neither edge has boundary black and neither
has measured boundary silence; and `silence_touched_nonblack_cut` when neither edge has boundary
black and at least one has measured boundary silence. “Audible” here means only “no >=40 ms interval
below -40 dBFS was measured”; it is not a perceptual continuity or airworthiness claim. A pair whose
edge record is absent or failed is unresolved and ineligible.

Anchor and pair selection is one deterministic constraint search over the complete otherwise-
eligible pool, not role selection followed by a best-effort transition bolt-on. Anchor feasibility
uses stable private case-id order so a secret used for blinding cannot make planning time unbounded;
qualifying pair choice, case identity, parent choice, and public alias/order remain seed-ranked. The chosen anchors
construct 12 two-item compilations with four actual joins in each of the first 40%, middle 20%, and
final 40% of playback. Within every band, all eight source uses are distinct, exactly two pairs are
same-role and two cross-role, and at least one pair occupies each transition stratum; the fourth
stratum is seed-ranked rather than silently used to weaken another quota. If all role, family,
source, timing, role-pair, and transition constraints cannot be satisfied simultaneously, planning
fails.

Six distinct programme parents have unique bytes and unique `(provenance authority, reference)`
pairs. Every submitted parent, including an unselected parent, is rejected if its content hash,
canonical relative local path, authority/item-ID pair, or canonical reference URL repeats
an entry in the complete 300-row digest-bound filler download ledger (including unselected, held,
and excluded cases). A URL-identity collision is rejected even when bytes, local path, and item ID
differ, and does not depend on matching authority labels. A programme parent is bound to an immutable
local metadata cache and a local `fillercorpus` inventory-v4 source record under the configured source
root. The loader rejects missing, escaping, symlinked, or over-16-MiB metadata caches and source
records, and rejects malformed source records. It hashes the raw metadata cache bytes and requires
that digest to agree with both the programme inventory and the matched source record; this binds
the cache's byte integrity, not its source-specific semantic format. Source records use only the
canonical `fillercorpus` decoder, never an
arbitrary JSON self-description. A parent names the source record path and the exact source
`(authority, itemId)`; its public provenance `reference` is the canonical HTTPS item URL (no
fragment, lower-case host), and must equal that record's normalized `itemUrl`. The record's
`metadataUrl`, retrieval time, metadata digest/cache, and local representation path, SHA-256,
byte size, and duration must all bind the parent to the actual source-root bytes. Authority and
item ID compare exactly after trimming; URLs compare by this canonical form; local source-root
paths compare as slash-normalized relative paths. Unsupported source-record formats or ambiguous
identity matches are rejected. This is origin binding and known-filler lineage exclusion only: it
does not certify that a programme is globally non-filler.

Every anchor appears exactly three times across the three-item set: six cases contain an adjacent
same-role join and six contain only mixed-role joins. Each of the six seed-ranked programme parents
supplies a 30-second near-start excerpt at `[10s,40s)` and a 45-second near-end excerpt ending ten
seconds before the parent ends. Each parent is at least 120 seconds, and both excerpts retain at
least ten seconds of omitted context on either side. These are construction positions, not a
semantic scene-boundary claim. Each parent also supplies one early and one late inserted-spot
case, with the source's exact programme spans and one independently selected filler anchor. Every
anchor appears exactly once as an inserted spot. The receipt binds these additional constructions
and their exact spans alongside the original two-item transition constraints.

The planner emits only coordinator-private construction authoring and a receipt binding all input
and output digests, deterministic seed ranks, selected families, role quotas, two- and three-item compilation constructions,
their measured transition stratum and join positions, programme cuts, and inserted-spot constructions. The receipt is also the
future split authority: it declares `split=holdout` and enumerates every selected source SHA-256,
duplicate-family id, and programme provenance pair that later development/training corpus builders
must exclude. Missing or broader inferred exclusions are not accepted; a future split change needs
a new independently reviewed artifact. It performs no rendering or inference. The structure
challenge preparer requires the authoring and its structurally validated current-contract receipt,
then records both digests and the plan contract in private challenge authority; authoring alone is
not render authority. It renders and freshly blinds the bound plan before two distinct direct-video
model families assess it serially. The direct-video request contract reserves 4,096 completion tokens so provider-required hidden
reasoning cannot consume the output budget needed for the strict JSON answer. The hosted-model
interface requests one ordered, coverage-preserving timeline with a role, exclusive end timestamp,
and evidence for each interval. The first interval starts at zero and each later start is the
preceding end; redundant starts and a second whole-file classification are not requested. The
adapter coalesces adjacent programme-fragment observations, never adjacent filler intervals, then
derives the internal whole-file claim from the complete timeline. One filler interval is a
standalone unit, two or more non-programme intervals a compilation, one programme interval a
programme excerpt, and programme intervals surrounding filler a programme with spots. A single
ambiguous or non-filler interval is unclear; a single unusable interval is unusable; other mixed
shapes are unclear. Standalone role evidence comes from its sole interval. The shared strict
decoder retains every semantic and length constraint unsupported by a provider-facing schema.
Schemas use the subset accepted by the exact pinned route and do not use `uniqueItems`. The
untouched raw response remains the inference authority; transport adapters do not reinterpret it.
Every source
must have an audio stream. Before concatenation,
every segment is encoded deterministically to a common 960-by-720, 30-frame-per-second video and
48 kHz stereo AAC audio profile with aspect-preserving padding and a fixed video track time base;
The renderer probes each normalized part and the concatenated output, rejecting any departure
from 960-by-720 H.264/yuv420p at 30 fps with exactly one 48 kHz stereo AAC stream. The builder
independently validates the returned measured profile before publication. Challenge contract v3
binds those stream facts alongside each public video's dimensions and byte hash, and its loader
rejects missing or nonconforming profiles. These uniform technical facts disclose no source or
semantic labels. Historical challenge artifacts are not rewritten or upgraded by inference;
the measured part durations, rather than requested timestamps, remain the join
authority. Coverage-only suitability holds may remain as evaluation material;
prohibited and operational holds cannot be selected. The constructed truth can test unit boundaries
without a second blind full-corpus review, but it cannot establish broadcast suitability, enter
training data, or authorize production admission. Plan contract v4 requires the receipt to contain
`blindHumanAuditRequired=false`, `trainingAllowed=false`, and `productionAdmissionAllowed=false`.
Missing, null, or true dispositions are rejected before media probing or rendering. Older receipts
remain immutable historical evidence, not current-contract rendering authority. This explicit
no-full-blind-audit disposition does not replace targeted human adjudication of challenged anchors.

The single-video protocol is only the short-source slice. It cannot authorize general compilation
reels: a 579.5-second construction from the 12 independently reviewed anchors exceeds the 64 MiB
transport ceiling. The separately versioned long-reel protocol therefore plans complete primary
coverage in two-minute spans with 15 seconds of context on both sides, clamped only at source ends,
for at most 30 minutes and 15 windows. Primary spans partition the source exactly; overlap supplies
context but never creates a second boundary vote. A boundary exactly on a primary seam belongs to the
right-hand span. These limits describe protocol capacity, not production authority: every normalized
window must still fit the media byte ceiling, every window must complete for one assessor family to
produce one source-level candidate, and a real seam-focused certificate must authorize the duration
slice before use. Sparse sampling, chunk-majority voting, and silently lowering the canonical media
quality are not valid long-reel fallbacks.
Production chooses between those protocols once, before media preparation, from the immutable
source duration and an explicitly configured short-source ceiling that must equal the activated
short certificate's duration envelope. Sources at or below that ceiling use the complete-video
runtime; longer sources use the window runtime only through its declared 30-minute capacity. A
selected runtime's preparation, provider, evidence, or reduction failure is final for that attempt:
the router never falls through to another representation, because doing so would silently change
the prompt, media authority, accounting operation, and certification slice. A source outside both
duration envelopes holds before either runtime can prepare media or reserve provider spend.

One content-addressed media-set artifact embeds the complete plan and the ordinal, normalized media
identity, and lineage of every window; its ordered membership is the common input authority for all
assessor families. The preparer takes one full-hash- and sparse-hash-verified immutable source snapshot,
renders every declared media interval through the same canonical 960-by-720 profile as the short path,
fully decodes each output, and publishes media and lineage only by content address. Reuse re-hashes,
re-probes, and fully decodes the retained bytes; no partial set is returned after a failed window.
Each accepted window answer covers its complete media interval in source-relative
coordinates and binds that exact media set, its own ordinal, and the assessor profile. An attributable
transport or provider failure is retained as an operational-failure answer with no semantic timeline;
it holds that assessor family's complete source result rather than letting the remaining windows vote.
The deterministic stitcher requires exactly one answer for every planned window from one assessor
profile. For each adjacent pair it compares the complete ordered boundary sequence and the role on
both sides throughout their shared context. Matching observations no more than 2,000 ms apart become
one boundary at their deterministic mean; a missing, extra, differently typed, or farther-apart
observation holds the complete family result. A boundary observed only in non-owning context is not
projected, while a matched observation that straddles a primary seam is retained. Projection must
reproduce one ordered timeline covering `[0,duration)` and preserves adjacent same-role intervals:
two consecutive commercials are two units even though both carry `commercial`. The stitch artifact
retains the plan and every window answer and replays byte-for-byte; it is one assessor-family
candidate, not independent agreement and not split authority.

The direct-video runner's per-request nanodollar value is an **accounting reservation**, not a
provider-enforced total-price cap. OpenRouter does not expose such a cap for token-priced video.
The transport therefore preserves every syntactically valid provider-reported charge before it
checks the reservation. A charge above the reservation closes that attempt as
`over_reservation`, records the exact decimal and nanodollars, request/response digests, generation,
route, tokens, and reserved amount, and makes the structured answer semantically unusable. The
actual charge, even when it exceeds the run's authorized spend, is the consumed-spend truth; the
overrun is explicit and no later request may start. Missing or malformed settlement remains the
separate unknown-charge state and consumes the reservation. The runner must never describe either
the reservation or the run authorization as a hard provider billing limit. Before media is opened,
the runner also multiplies the pinned route's snapshot prompt/completion prices by a declared
worst-case input-token allowance and the fixed completion-token ceiling using exact decimal
arithmetic; a reservation smaller than that bound is refused before HTTP.

The production-domain hosted structure assessor profile separates durable semantic identity from one
live metadata capture. `modelDigest` hashes the requested model id, its exact catalog canonical revision, and the
provider creation identity. `capabilitySha256` hashes that model identity together with the selected
upstream provider name and selector slug, input/output modalities, quantization, context and token
limits, supported parameters, ZDR membership, implicit-cache behavior, and the selected reasoning
mode. Both projections exclude
the snapshot retrieval time, request/response accounting, endpoint liveness status, display names,
and prices: those are capture facts, not a different model or route capability. The complete
metadata snapshot SHA-256 remains separately bound to every certification family result and supplies
freshness, current liveness, exact price, and historical audit evidence. A live or calibration runner
must still validate a fresh under-24-hour snapshot and exact ZDR route before media is opened or spend
is reserved. Every provider reservation and settled call record also binds that complete snapshot
digest, so a production decision artifact's retained evidence can replay which live metadata capture
authorized its route and price. It may reproduce a certified profile from a later fresh snapshot only when both stable
digests match; a capability or canonical-model change holds for recertification, while a price change
changes the reservation evidence without pretending that the assessor itself changed.

The immutable structure-certification report reproduces the comparison from at least two locked,
distinct model-family assessment sets and binds its digest to the exact public manifest, private
construction authority, holdout authoring, and source-family receipt. Certification requires all 60
cases and at least six cases from each predeclared difficult slice: two-item compilations, three-item
compilations, adjacent same-role joins, mixed-role joins, programme cuts near either parent edge, and
inserted spots in either the early or late position. Every assessor must have zero operational
failures, under-splits, over-splits, incomplete timelines, wrong segment roles, or structural
boundary misses beyond 2,000 ms, both globally and in every slice. A passing report still grants no
training or production-admission permission; its only next action is the locked shadow comparison.

Long-reel certification is a separate content-addressed authority over the window protocol rather
than an extrapolation from that complete-video report. Its private suite binds every case to one real,
replay-valid window media set, a complete known-truth source timeline, and the complete set of derived
seam traits. Non-geometric traits such as a wordless join or high-motion window additionally name an
independent measured-evidence digest; a coordinator declaration alone does not prove them. The suite
must contain at least six cases in each predeclared slice: a boundary in shared overlap, immediately
left and immediately right of primary ownership seams, an adjacent same-role join, one unit crossing
a seam, a programme/filler join, a wordless join, a high-motion window, and the six largest
successfully encoded windows in the fixed corpus. The suite records and reproduces the resulting
minimum high-byte threshold; separately constructed over-ceiling cases must hold before inference
rather than being mislabeled as successful stress cases. Exactly two locked, distinct assessor families
supply complete persisted
stitches for every case. The certification judge replays each stitch, scores each family independently
against private truth, then feeds the same two source-level candidates through `fillerstructure.Reduce`
and scores that confirmed decision too. Any missing case or stitch, held family, operational failure,
under-split, over-split, wrong role, incomplete coverage, or boundary error beyond 2,000 ms fails the
affected slice and the whole certificate. The immutable report names the suite, assessor profiles,
slice counts, errors, and reducer contract. It always sets training and automatic-materialization
permission false: a pass is evidence from which a separate window-specific materialization authority
may later be issued after the locked short-versus-long shadow comparison.

That shadow comparison is a provider-neutral replay over the complete 28-case long-reel corpus, not
a second truth-scoring pass. For every opaque case it receives the two immutable reducer artifacts
produced from the same exact source: one through `complete_video` and one through
`window_media_set`. It revalidates both artifacts, requires the same reducer version, boundary
tolerance, source identity, and ordered pair of underlying model families, and requires each paired
family to retain the same provider, declared model, model digest, and capability snapshot across the
two representations. Prompt and evidence-contract identities remain representation-specific and are
therefore bound in the report rather than required to be equal. Both decisions must be confirmed and
must agree on whole-source unit and standalone role, interval count, every interval role and
disposition, and every internal boundary within the certified tolerance. Missing, duplicate, held,
wrong-kind, differently sourced, differently profiled, or semantically divergent cases fail the
whole shadow report; there is no majority or partial-slice pass. The content-addressed report binds
both artifact digests for every case and always leaves training and automatic materialization false.
A repository publication wrapper additionally binds the content and file digests of the passing
window certificate and both complete decision sets, so the family-result lineage checked before the
comparison remains recoverable rather than becoming an unrecorded coordinator precondition. The
wrapper embeds the self-contained replayable report and grants neither training, production
admission, nor automatic materialization authority.
A separately reviewed long-reel authority may consume only a complete passing report together with
the passing window certificate; the shadow report cannot activate production by itself.

Each family run is a separate truth-blind artifact over the complete 28-case public window-set
manifest. The runner receives only opaque aliases, exact public source and media-set authority, and
machine-local window paths; it cannot open case identifiers, construction truth, measured slice
labels, or another family's answers. It evaluates cases and windows serially through the same
production family runtime, returns no partial result after an error, and retains one replay-valid
stitch per alias plus every ordered call record and completed-operation publication that produced
it. The result reproduces provider-request count, known charge, conservative accounted spend, and
unknown-charge reservations and binds them with the exact assessor profile, manifest digest,
complete metadata-snapshot digest, completion time, and self-digest.
Publication reopens the complete public manifest and creates the private result file immutably. The
private certification join starts only after both complete family artifacts exist, attaches case
identifiers by exact media-set identity from the suite, and binds its wrapper digest to the public
manifest, suite content and file digests, and both family content and file digests. Neither the family
artifact nor its certification wrapper grants training or automatic-materialization permission.
The calibration command requires its declared request ceiling to equal the manifest's complete
window count before opening the provider transport, rejects an existing result path before spending,
and uses the production SQLite call ledger for aggregate spend reservation and settlement. Replaying
settled content-addressed evidence consumes no request; a crash-open request conflicts in that ledger
before transport rather than being retried speculatively.

The complete-video half of the shadow uses the same truth-blind, one-family-at-a-time discipline over
the public 28-case manifest. Its production preparer creates or revalidates one exact canonical
derivative per source before the family call. A completed-operation publication keyed by source,
derivative, assessor profile, current prompt digest, and current schema digest points to the full
settled call record only after response, structured output, and record bytes are durable. Restart
therefore replays accepted or closed failure evidence without another request; an interrupted call
whose reservation has no completed publication remains held by the durable inference ledger rather
than being guessed or repeated. Each complete-video family result retains those records and cost
totals and grants no authority. Only two complete family results may be reduced into the complete-
video shadow decision set, and both decision sets must descend from their exact family artifacts
before the representation comparison begins.

The first long-reel corpus plan reuses only the twelve family-distinct bounded anchors and six
programme parents already locked by the 60-case holdout. One private deterministic planner binds
that authoring and receipt and emits 28 programme-with-spots constructions: six place a same-role
filler join ten seconds inside shared seam context, six place it one second left of primary ownership,
six place it one second right, and six place one complete filler unit across a primary seam. Programme
material brackets every construction, filler sources are always used whole, same-role pairs never
share a source or source family, and exact source-relative truth is derived from requested parts rather
than entered separately. The programme prefix begins one third into its parent. For the four
duration-edge constructions, the suffix begins two thirds into that parent rather than being packed
against EOF; even the shortest locked parent therefore retains more than fifteen seconds between the
longest possible prefix and suffix and more than fifteen seconds after that suffix. The seam
constructions retain their separate ten-second end margin. Packet measurement found ten-second
timestamp holes at 100 seconds in one otherwise useful parent and near 650 seconds in another, so
neither an arbitrary ten-second prefix start nor a long EOF-relative edge suffix is continuous
evidence. Before publishing a plan, the planner verifies every requested part against its locked
source bounds and the applicable seam or duration-edge context margins; insufficient programme
parents fail planning without publishing a plan. The plan performs no rendering or model call. A later renderer measures the
canonical encoded parts and may publish a certification suite only when wordless, motion, and largest-
byte evidence also satisfy the fixed slice counts; insufficiency fails the corpus rather than causing
model-outcome-guided case substitution.
The remaining four constructions establish the first intended continuous production-duration
slice without changing those seam cohorts after seeing model output. Two place one whole bounded
filler between programme excerpts in a 121-second source, immediately above the 120-second short
slice. Two use the same real programme/filler/programme shape at 301 seconds, within the fixed
tolerance below the first 302-second windowed ceiling. The pairs use distinct filler families and
programme parents. These edge cases participate in window certification and the locked
complete-video comparison; later expansion beyond the complete-video transport envelope requires
a separate window-only duration certificate rather than pretending this first slice proves the
protocol's full 30-minute capacity.

Rendering that plan is one atomic, non-authorizing operation. It reopens and hashes every declared
source before any output is created, renders each construction through the canonical structure-media
recipe, completely decodes both output streams, and publishes only opaque case names plus exact
full-file, sparse-file, duration, profile, tool, and complete-coverage window-plan identities. Its
private authority retains the construction plan, source provenance, requested parts, measured encoded
part durations, and the resulting complete source-relative truth. Interior truth boundaries come from
the measured encoded part joins rather than requested timestamps; only a bounded final container-
duration reconciliation may extend or trim the final programme interval. A rendered case that leaves
its predeclared seam slice, exceeds the ordinary retained-source byte ceiling, lacks audio or video,
or fails complete decode aborts the entire publication. The resulting files remain corpus inputs, not
training examples or production-admission authority.

One subsequent atomic packager reopens that public/private join, snapshots every exact constructed
source into its own staging root, and invokes the production structure-window media preparer over the
already-bound complete-coverage plan. Its public manifest exposes only opaque aliases, exact source
identity, path-free media sets, and the local paths of their content-addressed windows; its private
authority binds those aliases back to the rendered truth. A missing window, byte or lineage drift,
profile mismatch, over-ceiling window, incomplete decode, or partial source snapshot aborts the whole
package. This packager does not measure semantic traits, call an assessor, assign a certificate, or
grant materialization authority.

The suite assembler accepts only those locked media and pre-model authorities; model responses are
not an input. A wordless join is proven when at least one independently bounded filler source beside
the planned join has a retained transcript artifact containing one or more closed non-speech markers
and no lexical segment. The source-to-evidence alias and transcript digest must replay through the
holdout receipt. Motion is measured over every prepared window as the mean absolute luma delta between
each pair of adjacent decoded frames; the evidence also retains frame count, sum, 95th percentile,
maximum, exact media identity, and ffmpeg identity. The highest-scoring window in each case competes
for the fixed six-case high-motion cohort, with stable case/ordinal tie-breaking, and the sixth score
becomes the reproduced cohort threshold. Selecting one window per case prevents several energetic
windows from one construction from satisfying the slice by themselves. Both measured slices are fixed
before inference, and insufficiency aborts suite publication rather than substituting cases after model
outcomes are known.

Before either paid representation run, one provider-free preflight reopens the complete public
window set and private certification suite, rehashes every retained source and window, and proves
that the suite contains exactly the same media sets. The operator must declare the already-certified
short-source ceiling and the intended first long-source ceiling. The preflight reports the observed
minimum and maximum source durations, window and byte envelope, exact per-family window and
complete-video request counts, and the four-run total. It is ready only when at least two sealed
cases occur within the fixed timeline tolerance above the short ceiling and at least two occur within
that tolerance below the intended long ceiling; any case beyond the intended ceiling also fails
readiness. The content-addressed report grants no authority and performs no provider or metadata
request. A failed report's only next action is to extend and rerender the sealed corpus before paid
assessment, preventing cost approval or a passing semantic score from silently standing in for an
unrepresented production duration slice.

The agreement policy itself is one provider-neutral production-domain module, not a scorer-owned
copy: both the challenge adapter and the eventual production runtime supply source-bound immutable
candidate assessments to the same pure reducer. Challenge aliases, private truth, provider clients,
and persistence stay outside that module. Its interface retains exact source identity, complete
timelines, assessor/model-family identity, immutable assessment identity, and every disagreement.
The shared assessment-input manifest represents either one complete normalized video or one complete
ordered window media set; it binds the source, media profile, every derivative identity and lineage,
and the window plan digest when applicable. Candidates name that manifest's digest. The reducer never
models a window set as one synthetic video and never needs provider or filesystem knowledge;
this makes implementation drift between the passing certificate and the deployed decision path a
compile-time architecture error rather than a rollout convention.
Production persists that reducer input and output as one content-addressed decision artifact before
the split proposal may consume it. The artifact identifies the reducer contract and boundary
tolerance, retains every independent complete-timeline candidate or operational failure, and
reproduces the decision byte-for-byte when validated. The proposal document binds the artifact's
media digest and duration to its exact retained source. An invalid, drifted, missing, or held artifact
cannot certify the heuristic assessment; a later certification authority must verify its declared
assessor slices rather than replacing those durable identities with a boolean callback.
The complete-plan gate independently checks that a confirmed artifact and the proposal assessment
name the same source unit and the same exhaustive ordered spans, roles, and filler/non-filler
dispositions. A certification callback therefore cannot turn a detector-only plan into model
agreement or approve a projection whose programme spans became filler children.
Structure release authority is itself a content-addressed document, not an injected predicate. It
locks the external certificate digest, reducer contract and tolerance, exact assessor/model-family/
provider/model/capability/prompt/evidence-contract profiles, and the allowed source-unit and segment-
role slices. Verification requires an exact profile set and a confirmed artifact; unknown profiles,
units, roles, held decisions, or an authority without explicit production permission fail closed.
The current short-source authority admits only the complete-video input kind. Window-set activation
requires its separately measured certificate and receives a distinct authority contract; a short-
source certificate cannot authorize a long reel merely because both use the same encode profile.
That long-reel authority binds the passing window certificate and short-versus-long shadow digests,
the canonical window-profile identity, the exact window assessor profiles, the observed source-
duration and per-window byte envelope, and only the source units and segment roles present in every
passing shadow case. Issuance records one bounded reviewer identity and canonical review time and
requires an explicit materialization-permission flag. Verification reconstructs the canonical plan
from the artifact's source, matches its plan digest and complete ordered item count, and checks every
window's measured duration, media profile, and byte ceiling. It can create held child work only;
training and broadcast admission remain separate authorities.
A second content-addressed deployment document turns that reviewed authority into an executable but
still fail-closed production configuration. It binds the authority digest; explicit automatic-
assessment permission; exactly the authority's ordered assessor ids, model families, and requested
models; each exact upstream provider name and selector slug; reasoning mode; worst-case input-token
allowance; per-request accounting reservation; and positive per-source and per-day spend ceilings.
The per-source ceiling must reserve every certified window for both families before activation. The
document contains no credential, filesystem path, current canonical model revision, mutable price, or
metadata capture. Production reads the OpenRouter credential from its existing provider secret,
fetches and caches a fresh canonical metadata snapshot for less than half its 24-hour validity,
recomputes every stable assessor profile, and requires exact equality with the reviewed authority
before preparing media. It then price-checks every route against the deployment reservation and uses
the complete fresh snapshot digest in every durable call reservation and settlement. Missing or
invalid authority, deployment, credential, route, snapshot, evidence store, media preparer, or budget
leaves independent assessment and certified materialization disabled; it never restores the heuristic
gate under a certified policy label.
The runtime assessment coordinator calls each configured complete-timeline assessor serially with
the same immutable conditioned-media identity and path. Its port exposes no prior answers. Each
adapter must return either a complete source-bound candidate or an attributable operational-failure
candidate; ordinary provider failures are domain evidence, while an error means the adapter could
not produce trustworthy evidence and aborts reduction. The coordinator rejects declared-profile or
source drift before creating the durable artifact.
The long-reel coordinator preserves those semantics in family-major order. It prepares one complete
window media set, then calls every window for the first assessor serially, durably commits each
validated window answer before starting the next, deterministically stitches and commits that family,
and only then repeats for the next assessor. An assessor receives exactly one path, its source-relative
window geometry, and the common media-set identity; the interface exposes no peer or earlier-family
answers. A normal provider failure returns an attributable operational-failure window and still closes
the family as a held stitch. An error, profile/media/ordinal drift, or failure to persist any answer or
stitch aborts without reduction. Only persisted, replay-valid family stitches become whole-source
candidates, and the final decision artifact is persisted before return.
Each window call crosses the persistence seam as one separately versioned recorded assessment, not
as a bare semantic answer. Its reservation and settlement embed the complete media-set authority and
bind the exact ordinal, assessor profile, prompt and schema digests, requested and resolved route,
generation, tokens, requested/reserved/charged/accounted nanodollars, closed state, raw-response and
structured-output digests, and the resulting semantic window-assessment digest. The request is
durably reserved before transport. Response and structured-output blobs plus the semantic assessment
publish before the settlement record, and the coordinator reloads that complete record before the
answer may enter stitching. Accepted output is parsed in window-local coordinates and projected
deterministically onto the plan's source-relative media interval; the exact planned interval, rather
than encoder-duration drift, remains the coverage authority. Budget holds, route drift, provider or
schema failure, unknown settlement, and reservation overrun each produce one closed operational-
failure assessment with no segments. Cancellation does not bypass settlement: once a reservation
exists, the adapter settles through a bounded context detached from caller cancellation. A paid or
reserved call therefore cannot influence a boundary from memory alone, disappear from accounting,
or be mistaken for semantic evidence.
The completed-call lookup is itself immutable and deterministic. An operation identity hashes the
exact media-set digest, ordinal, and complete assessor profile; its publication binds that operation
to exactly one call-record digest and is written only after all referenced evidence and the durable
settlement exist. On restart, the coordinator resolves this identity before invoking an assessor,
strictly reloads every referenced byte, and reuses the answer only when the complete authority
revalidates. A second, different record for one operation is a conflict, not a retry. A missing
publication permits a new call only when no durable reservation for that exact request exists; an
open crash reservation remains an explicit unknown-charge hold and must never cause an automatic
duplicate provider call. Completed earlier windows therefore resume without repayment while an
interrupted in-flight window fails closed instead of guessing whether the provider charged it.
The conditioned-media identity also binds a content-addressed, path-free lineage document. That
document names the original source identity, the complete canonical assessment-media profile, the
exact ffmpeg version and executable digest, and the normalized derivative's digest, byte count, and
measured duration. Production renders the derivative into staging, validates its streams and profile,
then atomically publishes both derivative and lineage beneath the hidden media tree. A retry may reuse
that derivative only after strictly decoding and re-hashing the lineage, re-hashing and re-probing the
media, and reproducing the operation key from source, profile, and tool identities. Filesystem paths
are locations, never authority. A missing, drifted, oversized, incomplete-stream, or out-of-profile
derivative fails before either assessor or provider is called.
The direct-video prompt version, identity-blind system instructions, dynamic duration message, JSON
schema, strict decoder, programme-only coalescing, whole-source unit derivation, evidence bounds,
and reducer-candidate projection are one `fillerstructure` contract used by evaluation and runtime.
Provider adapters supply transport and durable attribution only; they may not carry a private parser
or reinterpret interval roles.
Each runtime assessor returns one content-addressed assessment record rather than an unaudited
candidate. The record binds the exact source bytes and duration, declared assessor profile, request
and raw-response digests, requested and resolved route, generation, tokens, accounting reservation,
provider charge, closed operational state, and either one complete parsed timeline or no semantic
claim. The coordinator verifies the supplied raw bytes against that record and commits both through
its evidence-repository seam before projecting a reducer candidate. A missing response for a closed
transport failure remains explicit; an unsettled, budget-held, over-reservation, route-drifted, or
invalid-response record becomes attributable operational-hold evidence. Persistence failure aborts
the reduction, so a paid call can never influence a split from memory alone.
Only one confirmed artifact may project model-decided spans into V67. The projection validates and
replays the artifact, binds it to the proposal's exact source, and constructs the V67 assessment
from the artifact's exhaustive ordered intervals. Previous
chapter, black, silence, transcript, and sparse-frame observations remain content-addressed context;
their boundary effects are neutralized in the projected assessment so they cannot impersonate a
third assessor, add an interval, or override the certified reducer. The detector-authored proposal
remains intact for compatibility comparison; a later certified split projection joins metadata only
onto exact decided spans and does not carry stale detector failure or hold decisions. Programme and
non-filler intervals remain explicit discards and never enter the child-confirm list.

The certified gate does not translate complete-timeline agreement into the legacy detector's
boundary-confidence percentage or require that percentage to authorize the same cut a verified
artifact already establishes. It still applies deterministic duplicate and duration refusals,
grounded taxonomy requirements, the four exact-span screens, and the immutable structure authority.
The compatibility calculation is non-authorizing measurement only. Missing complete-timeline
evidence or materialization authority holds the proposal; it never selects the older automatic gate.

Opening that comparison can invalidate inherited anchor truth without authorizing a post-hoc score
repair. A targeted anchor-adjudication module consumes the exact public and private challenge,
plan authoring and receipt, all locked assessment sets named by the comparison, the immutable
comparison itself, and one reviewer submission. It first reproduces the comparison byte-for-byte.
The review target is then exactly every construction-authority `standalone` case named by the
comparison's diagnostic candidates: it cannot omit an inconvenient target or expand into a new
full-corpus audit. Each target records complete-span audiovisual coverage, explicit bounded
observations of the opening, ordered internal joins, and closing, one reviewer identity and fixed
review time, sorted unique decisive timestamps, a bounded rationale, the original closed unit/role,
and exactly one disposition: `confirmed_original`, `structural_disqualification`, or
`role_correction`. A structural disqualification must replace `standalone` with a non-standalone
unit and no role; a role correction must retain `standalone` and select a different valid role.
Model agreement selects what receives review but never becomes truth authority by itself.

The publisher preserves every original input and emits a new owner-only authority rather than
editing the human lock, plan, challenge, model responses, locks, or comparison. It binds every input
file hash, the plan's human-assessment and evidence-manifest hashes, the exact challenged source
bytes and duplicate family, the review decisions, the prior receipt's complete future-training
exclusion, and every rendered video hash exposed in a model request. Its output always declares the
evaluated challenge burned, training false, and production
admission false. A later holdout-plan contract must consume that authority as prior exposure: no
source bytes, duplicate family, or programme provenance from the burned challenge may appear in a
replacement challenge, and its future-training exclusion must be the exact cumulative union. Until
that replacement machinery and new source inventory exist, adjudication can explain and quarantine
the bad truth but cannot manufacture a corrected certification score. The planner makes lineage
explicit: an invocation is either a genesis plan with no prior adjudication, or a replacement plan
with one or more immutable prior adjudication authorities; omitting both or mixing the modes fails.
A replacement validates every prior authority, rejects any candidate whose source bytes, duplicate
family, or programme provenance appears in their cumulative exposure, binds their file hashes in the
new receipt, and publishes the sorted de-duplicated union of prior and newly selected exposure. The
genesis/replacement mode and prior-exposure set are part of the plan contract, so a caller cannot
silently forget burned evidence while requesting a replacement.

Suitability screening is repeated over every freshly rendered structure case because concatenation
and excerpt construction create new viewing contexts. Its prompt identity binds the system prompt,
sentinel dynamic content and schema, request title, and 4,096-token completion ceiling so mandatory
provider reasoning cannot consume the structured answer budget. A prohibited signal in any derivative is a
conservative source-level quarantine: the private construction authority projects the observation
back to every overlapping source segment, and every case derived from that source remains held even
when another model reports no signal. Repetition at the same source-relative interval strengthens
the quarantine evidence; it is not counted as independent corroboration. A model non-flag is never a
safety certificate, and majority voting cannot clear a prohibited observation. Quarantined media may
remain in the immutable evaluation package so the miss is measurable, but cannot enter training,
catalog ingestion, scheduling, or production. Operational and incomplete-modality outcomes also
remain held. Only an independently specified suitability-recall certification can turn complete
no-signal observations into an admission claim.

The source projection is one separate deterministic private seam. It consumes the exact public
structure manifest, private construction authority, two-family comparison, and both immutable
result files named by that comparison; the result files retain the observation ranges that the
summary comparison deliberately does not duplicate. It first reproduces the comparison from those
results, rejects a comparison timestamp earlier than either completed result, and then intersects
every observation with measured output segments. Encoder-duration drift expands the projected
source interval conservatively by the bounded render drift rather than narrowing it. Overlapping
same-kind, same-modality source intervals merge into one observation with ordered assessor/case
witnesses. The private report lists every source and derivative case, propagates a source quarantine
to derivatives that had no direct flag, and keeps `trainingAllowed`, `ingestionAllowed`,
`schedulingAllowed`, and `productionAdmissionAllowed` false. It performs no inference, catalog
write, media mutation, or admission decision.

The private projection also retains the verified immutable challenge-v1 evidence contract required
by #903 and the retained complete-source spoken diagnostic in #912. This is an archived diagnostic
input, not a current challenge or certification. Only the source-suitability and spoken-safety
projection seams may load it through the same private strict decoder; public challenge, assessment,
and rendering interfaces remain on
the current challenge and plan contracts. Separate strict archived input shapes reject fields
introduced by later contracts even when empty or null. They require the original explicit negative
production disposition, complete media/authority/alias/segment bindings, and the exact immutable
result/comparison bindings. The private validator requires the canonical assessment-media profile
for current challenges and its absence for challenge-v1 diagnostics, whose schema predates that
field. Decode and hash the same raw bytes; reject unknown, duplicate, trailing,
mismatched, ambiguous, or incomplete evidence before publication. Archived inputs cannot substitute
for current measured profiles, plan receipts, source provenance, or ledger evidence. No version
string, original artifact, or provider result is rewritten to make historical evidence appear current.
The projected report preserves the original input digests and all four false permissions, including
when reproducing an archived no-signal observation. This narrowly retained external evidence
contract does not provide a general legacy-loader option or an admission fallback.

Spoken-language safety is a separate complete-source evidence seam; neither the production catalog
transcript nor a direct-video model's spoken-language answer can satisfy it alone. The catalog
transcript is deliberately selective and normally samples only the `LanguageSpan` window, while the safety
seam transcribes `[0, measured source duration)` for every source in the exact corpus manifest and
every additional source named only by the construction authority. It validates the label-blind
packet set and its external media bytes against that corpus manifest before evaluating transcripts,
then binds the source, packet, extracted audio, ffmpeg, whisper executable, model, implementation,
timing, and completion identities in an immutable private transcript artifact. The smaller review
evidence set is a projection target, not the scanner's population. Wordless is a completed outcome
only after that full span runs successfully. Missing audio, an engine error, unordered or
out-of-range timing, identity drift, or an incomplete source set is a coverage hold, never a clean
observation.

One deep `PublishTemporalSpokenSafety` module interface owns strict authority loading, complete-span
transcript-artifact validation, private policy evaluation, source projection, canonical validation,
and atomic publication. Transcript production remains behind the existing digest-pinned
`BuildTranscripts` engine seam rather than being duplicated. Its versioned policy file is private
and uses opaque rule identifiers; report artifacts retain only the policy digest, rule identifier,
match class, and time range, never the raw
restricted phrase or transcript text. Exact policy variants may quarantine; deliberately ambiguous
variants hold coverage. A source-level quarantine propagates to every derivative through the same
construction authority as the visual projection, with no majority-vote clear. Certification uses a
separate source-disjoint positive/clean challenge and counts source families rather than derivatives;
it requires zero positive misses and a one-sided 95% exact Clopper-Pearson lower bound of at least
95% for source recall (59 independent positive sources with zero misses). Until that challenge
passes, the report keeps training, ingestion, scheduling, and production admission false even when
the measured sources contain no match.

Spoken-safety certification consumes that exact private projection plus a separate locked private
challenge authority authored no later than the projection under test. The authority binds the
corpus and policy rather than a post-run report, and uses opaque aliases and family identifiers to
label source-disjoint positive intervals and clean locale/slice controls; it contains no model output
and cannot be derived from the transcript under test. Every positive interval must overlap a
prohibited match from the projected source, and any missing/ambiguous transcript remains an
operational hold.
Recall is counted by independent source family, requires zero misses, and reports the one-sided 95%
exact Clopper-Pearson lower bound. Clean false-positive rates are reproduced independently for each
locked locale/slice and may not exceed 1%. The canonical certification report retains only opaque
case identities, counts, rates, authority digests, and outcomes. It never reproduces source identity,
transcript text, or policy phrases and grants no production permission even when the diagnostic
challenge passes. Generated/TTS controls are permanently marked `development` and can produce only
a diagnostic result; they cannot satisfy or be relabeled as the independent-source certification.

The maintained transcript projection above remains deterministic diagnostic history: it loads and
projects already-produced evidence and owns no network, process, retry, budget, or production-ingest
behavior. Measured development controls supersede the assumption that it can become the production
scanner by adding a decoder. Whisper-family transcripts missed prohibited positives; a stronger local
acoustic proposer retained the known positive but generated an impractical candidate queue. A hosted
native-audio adjudicator reduced that queue without earning negative authority, and a distinct
complete-video/audio route corroborated the reduced set. Those observations establish the next
production boundary, not a clean-source label or an admission certificate.

The initial spoken-safety stage delivers a private cascade core and an ephemeral in-memory attempt
record. Its evaluator and adapters remain unexported and unwired; this stage does not expose the
external evaluation operation or claim an immutable durable ledger. The next stacked integration must
deliver that operation and ledger before compatibility scoring or ingest wiring can consume the core.

The completed production **spoken-safety evaluator** is one deep Go module with one external evaluation operation
over an immutable source-authority document. It owns complete-source validation, bounded media planning,
the ordered cascade, deterministic reduction, and canonical result validation. Local acoustic proposal,
native-audio adjudication, complete-video/audio corroboration, and media extraction are private adapters;
their provider, process, and tool details do not leak into callers. The module emits claim-specific
evidence plus an immutable per-step ledger. It does not return or imply a filler-admission verdict, and
`filleradmission.Evaluator` remains the only terminal semantic authority.

The operation request supplies a stable run id and start time, the certification-authority digest, and the
source authority plus machine-local path. The source authority identifies only the immutable source,
measurement, policy, implementation, and tool facts that exist before a certification challenge is locked;
it does not contain the certification-authority digest. Keeping those identities separate is mandatory:
placing the certification digest in the source authority while the certification authority stores the source-
authority digest creates an unconstructable hash cycle. The request joins the two independently immutable
documents, and the durable run header binds both. The path remains excluded from every returned or durable
value. A first invocation atomically creates
the immutable run header before appending source-plan and proposal events; a repeated invocation of the same
completed run returns its already-canonical terminal evidence without repeating local or hosted work. An
existing incomplete run is never resumed in place or silently reissued: recovery closes it conservatively and
a caller starts a new bounded run id. The operation returns only the path-free run identity, closed evidence,
reducer result, and terminal event id/digest needed to reproduce certification input. Audio assessments retain
only sorted opaque matched-rule ids beside their closed state; they never retain a phrase, transcript, or quote.
It exposes neither adapters nor a provider response and cannot be used as an admission decision.

The cascade validates exact source bytes, measured duration, transformations, tool identities, and
complete modality coverage before inference. A certified local candidate proposer emits source-relative
candidate intervals. Each candidate is adjudicated by a pinned native-audio route. Only when every
candidate receives a valid absent result may a distinct pinned route inspect the complete source video
and audio. Calls are serial by default and retain the exact requested and canonical model, upstream
route, snapshot, modalities, media bounds, response, cost, reservation, settlement, and failure identity
required by the OpenRouter certification contract above.

Concrete runtime construction consumes the exact private certification-authority bytes, a reproduced
passing certification report, the private restricted-language policy, and the public spoken projection
authority as one joined deployment input. Callers do not select independent model, prompt, schema,
proposer, or capability strings: the factory reproduces their identities from those authorities and
refuses any disagreement before opening media, reserving spend, or contacting a provider. It refreshes
the bounded OpenRouter route snapshot before a new run, requires the same canonical model and ZDR,
fallback-disabled upstream capabilities used by certification, and refuses a current price ceiling above
the deployment reservation. Credentials, the configured ffmpeg location, storage adapters, and current
time remain runtime-only inputs and do not enter the portable authority.

The concrete factory does not by itself activate production execution. No spoken-safety boot setting or
default deployment exists until a real category corpus produces a passing report and an operator seals its
exact execution envelope. The activation change must declare those artifact paths in the settings registry,
load them once at boot with private-file checks, and fail closed to the existing qualification hold when any
artifact is absent or stale. Shipping a constructor against synthetic fixtures is never permission to spend,
project a complete axis, or admit a rendered child.

The rendered-child producer builds one source authority from the already content-verified evidence
derivative. It remeasures complete audio/video presence and duration with the ffprobe executable adjacent
to the configured ffmpeg, identifies both executable byte digests and banners, and requires the derivative
manifest's ffmpeg identity to match before the cascade can run. The first durable run time is also the
source measurement time. If screening crashes after the cascade settles but before the axis projection is
written, retry reuses that durable first-run time, reopens and remeasures the current exact bytes, and
replays the existing terminal run without another hosted request. A changed source, toolchain, policy,
certification, route, or operation identity conflicts or holds; a fresh timestamp may never turn the same
screening operation into a second billable run.

Reduction is deliberately asymmetric. One valid prohibited-presence observation quarantines the source;
a negative observation never votes it away. An unclear or disagreeing result, incomplete modality,
provider or local-runtime failure, stale identity, exceeded budget, or invalid schema holds
operationally. A presence response with malformed timing is sufficient to hold but cannot be projected
as a bounded fact. Two valid negative model observations produce only `candidate_rejected`: they never
mean clean, suitable, ingestible, schedulable, or admitted. A proposer that emits no candidates likewise
cannot establish clean coverage before the complete cascade is independently certified.

The reusable OpenRouter multimedia transport is a separate concrete infrastructure module rather than
part of the identity-blind review package or the generic text/tool LLM client. It hides capability-
snapshot validation, canonical model and upstream binding, zero-data-retention and fallback-disabled
routing, strict structured output, media ceilings, durable reservations, exact settlement, response
metadata, and raw-response binding. Review, certification, and production safety modules may consume
that behavior through private interfaces; none may weaken its route or accounting contract.

Before reservation or HTTP, the transport requires validated route authority derived from the exact
capability-snapshot bytes and expected digest. Validation binds freshness, requested and canonical
model identities, upstream/provider route, required input modalities, structured-output support, and
zero-data-retention eligibility. Unchecked strings or a syntactically valid digest cannot construct
that authority; a missing or zero authority fails before any reservation or request.

Subsequent production integration must be shadow-only. Its durable ledger is written before the compatibility score may
run; failure to persist leaves the ingest parked and recoverable. The result grants no catalog filing,
training, scheduling, or production-admission permission. An applied projection from certified
spoken-safety evidence into the admission document is a later design and rollout change. Its identity
must match the exact certification artifact; model, route, prompt/schema, media planner, policy, local
runtime, weight, or implementation drift returns a hold until recertified.

The spoken-safety ledger is not an admission-decision record. `filler_admission_decisions` stores the
terminal policy projection described above; a spoken-safety run instead records how one evidence attempt
executed, including work that crashed or never reached a semantic result. Persistence therefore owns an
immutable run header and append-only, ordinal events. The header binds the clip, complete-source authority,
source bytes, certification, policy, implementation, and start time. Events use a closed kind and payload
schema for source planning, local proposal, hosted-call reservation, hosted-call settlement, and the
terminal reduced result. Stable run/event identities and exact-payload conflict checks make a repeated
write idempotent without permitting history to be replaced.

A hosted-call reservation event and its V62 inference-budget reservation commit atomically before the
HTTP request starts. Settlement is a later append-only event bound to that reservation; it records the
request and response digests, requested and resolved model/provider identities, upstream route, modality
coverage, generation id, closed outcome or failure code, exact charged decimal/nanodollars, and reservation
disposition. A terminal event contains the canonical closed evidence and reducer result and references every
attempt event in order. The compatibility score may advance only after that terminal event commits. An
interrupted run remains visibly incomplete: startup/retry appends an operational terminal hold, marks any
unsettled reservation failed with unknown settlement, and starts a new bounded attempt rather than mutating
or silently replaying the old history. Old reservations continue to count against budget, so repeated
crashes cannot create unbounded spend.

`fillersafety` owns the narrow persistence port for those lifecycle facts; the SQL store is an adapter that
maps its closed reservation and settlement commands onto the generic V62 inference row and the spoken-safety
event in one transaction. The evaluator supplies deterministic per-run event/evaluation ids, exact media and
version identities, and the closed outcome or failure class. The store supplies only budget disposition and
persisted accounting facts. A budget-held reservation is itself durable and prevents the HTTP request; it
needs no settlement event. Once an accepted reservation exists, inability to persist its settlement or the
terminal event returns an operational error and leaves the run incomplete for startup recovery rather than
returning unrecorded semantic evidence.

The ledger stores only bounded public identities, closed states, interval coordinates, digests, accounting,
and opaque rule ids. Machine-local paths, source names, restricted variants, transcripts, quotes, private
policy JSON, prompts, media bytes, raw request bodies, raw provider responses, and free-form provider errors
are forbidden. Request/response SHA-256 values plus the provider generation id bind those private artifacts
without copying them into an ordinary application database or operator projection. No ledger read model is
ordinary-user-facing in this slice.

Ledger-owned run, event, evaluation, candidate, reservation, generation, clip, and implementation
identifiers are bounded opaque ASCII tokens using letters, digits, underscores, and hyphens. They
reject filesystem separators, dotted filename forms, URLs, whitespace, and control characters. A clip identity
remains the existing opaque clip key; this boundary does not invent a different digest format for it.
Public model/provider identities use a separate bounded representation that preserves legitimate route
slashes, revision dots, and provider display-name spaces. These values must come from the validated
route and response identities recorded by the atomic V62 helpers, never from source metadata or free-form
provider output. Their syntax is not evidence of route authority: the completed runtime integration must
retain the validated capability and response binding before writing those fields. The generic event
append operation cannot create reservation or settlement authority.

Restricted spoken language and broad visual suitability remain separate claims. Complete-video
suitability must ultimately cover every source, including a source for which the candidate proposer emits
no interval; a video corroboration performed only on spoken candidates cannot satisfy that obligation.
The admission evaluator combines independently certified claim evidence and never treats one no-signal
lane as authority for another.

The measured sherpa keyword proposer is not a production dependency until its runtime and exact model-weight
artifacts have explicit redistribution and use authority recorded in §14. The selected 2024 GigaSpeech archive's model-card
metadata says Apache License 2.0, but the archive contains no `LICENSE` or `NOTICE`, and upstream issue #3802
explicitly asks which terms apply to this exact model and remains unanswered. That is enough evidence for the
maintainer's private development measurement, not enough authority for Loomarr to bundle, fetch, recommend,
or imply commercial use of the weights. The engine pin and companion-file inventory are now known; the weight
authority and both-platform certification remain separate gates. A lower-recall decoder is not silently
substituted.

The legally unblocked baseline proposer is instead deterministic and weight-free. It partitions the verified
complete soundtrack into contiguous, non-overlapping 28-second source intervals, with the final interval ending
exactly at the source duration. The existing native-audio extractor supplies one second of clamped context on
each side, so every request remains at most 30 seconds of 16 kHz mono WAV and below the existing 2 MiB ceiling.
The union of candidate intervals is exactly `[0, duration)`; a boundary cannot become an uncovered negative.
The 4,096-candidate ceiling rejects rather than truncates a source beyond 31 hours. This strategy reads no
source bytes, starts no process, carries no model/runtime identity, and makes no safety classification: it
trades more serial hosted calls for auditable complete audio coverage. Its proposer identity explicitly names
the deterministic strategy and hashes its window configuration; the model-backed sherpa identity remains a
separate strategy with exact platform, runtime, and model hashes. Certification, route budgets, and private
source-family truth decide whether the weight-free baseline is good enough before any shadow wiring.

The development adapter consumes one private, mode-`0600` **acoustic keyword authority**, not raw policy
phrases on its interface. That versioned JSON binds the canonical policy digest, exact model manifest digest,
and an ordered set of opaque rule ids to their pre-tokenized BPE variants. The adapter verifies every token
against the pinned model vocabulary and writes a private ephemeral sherpa keyword file whose `@` label is only
the opaque rule id. It never derives tokens inside the server, places a phrase or token sequence in argv or an
environment variable, or accepts sherpa's keyword text as a result identity. The authority is prepared
offline with the same pinned `bpe.model`; adding SentencePiece or another tokenizer to the server is a separate
§14 decision, not hidden inside this adapter.

The adapter stages the exact runtime executable, ONNX Runtime library, model members, vocabulary, and private
keyword authority into one private workspace before use. A canonical manifest digest, rather than an archive
name, identifies the bytes that can affect inference. It extracts the complete verified source to 16 kHz mono
WAV, invokes the worker through the process-tree supervisor with bounded time and output, captures both streams
in memory, and discards stderr without logging it because sherpa prints source paths there. Stdout is strict
one-JSON-object-per-hit: unknown fields, an unknown/non-opaque keyword label, non-finite or unordered timing,
token/timestamp cardinality drift, output beyond the ceiling, or a non-zero/partial run fails the complete
proposal. Candidate intervals run from the first token timestamp through one 40 ms subsampled frame after the
last and are clamped only at the verified source endpoint. Native-audio adjudication extracts each of those
evidence intervals with the calibrated one second of context on both sides, clamped to the complete source;
the candidate identity and ledger interval remain the unexpanded acoustic evidence rather than pretending the
context was detected speech. These implementation details stay behind the private candidate-proposer seam;
the external evaluator interface remains one evaluation operation.

Production certification uses a locked, source-family-disjoint challenge of real speech, not generated
or transformed copies pretending to be independent observations. Positive coverage includes distinct
speakers and source families across the predeclared accents/locales, music and overlap, noise, speed and
pitch, codec, clipping, and placement slices. With zero misses it still requires at least 59 independent
positive source families for the one-sided 95% exact Clopper-Pearson recall lower bound to reach 95%.
Clean and near-match controls report their observed false-positive rate for every locked locale/slice;
any stronger confidence-bound target declares its sample population before the challenge is opened.
Known-script, consented real-speaker recordings may supply positive truth when licensed pre-labeled media
is unavailable, but two independent blind reviewer identities still verify audibility and timing and a
third adjudicates disagreement. A model-backed reviewer requires the immutable attestation and candidate-
family exclusion already specified above. The maintainer is not a required blind reviewer.

Cascade certification is a separate deterministic module over two private immutable documents. The authority is
authored before evaluation and binds the policy, evaluator/proposer and hosted-route identities, truth
provenance and rights/consent digests, opaque case and source-family ids, locale/slice coverage, positive rule
intervals, and two agreeing reviewer attestations (or a third adjudicator). A model-backed reviewer declares
its model family, which must be absent from every proposer/adjudicator/corroborator family in the same
authority. Each case binds the digest of its certification-independent source authority. Evaluation supplies
the completed certification-authority digest alongside that source authority and binds both separately in the
run header; neither digest is defined in terms of the other. The label-blind result manifest contains every authority alias exactly once with the complete
path-free run header and ordered ledger events; it contains no truth label. Every run must start after the
authority was authored, bind that exact authority digest as its certification identity, and end in the named
terminal event/digest. Missing, extra, duplicate, incomplete, identity-drifted, or non-canonical runs make the
whole score operationally invalid rather than silently reducing its denominator.

The scorer counts a positive source only when a valid audio detection carrying the expected opaque rule id
overlaps every declared positive interval. Another prohibited rule, a video-only flag, an unprojectable
presence, a hold, or a candidate interval without rule attribution is not a hit. Recall is by the authority's
unique source families. Clean false positives are any audio prohibited detection and are reported for both
locale and declared clean slices. A development authority can produce only `diagnostic_passed`; a
certification authority still requires at least 59 positive families, zero misses, a one-sided exact 95%
source-recall lower bound of at least 95%, at least 100 independent clean families so the declared 1%
observed false-positive ceiling has one-source granularity, zero coverage holds, and no declared clean slice
above that ceiling. This is explicitly an observed-rate gate, not a 95% upper confidence claim; the latter
would require at least 299 clean families with zero false positives. Every output permission remains false
regardless of status.

Authority locking is a separate offline operation over a private path-bearing draft, two independent complete
review bundles, an optional disagreement-only adjudication bundle, a private alias seed, and the exact source
and evidence bytes. It verifies source-authority and media identity, policy and implementation identity,
review order and draft binding, draft-pinned rights and truth-provenance digests against their current bytes,
family uniqueness, and all corpus minima
before emitting a path-free authority. Opaque case, family, and reviewer ids are keyed derivations from the
private seed. Reviewers are blind to evaluation output and one another; known-script reviewers may see the
claim they must verify. Model-backed primary/adjudicating reviewers are permitted only when their model
families differ from one another and from every evaluated proposer/adjudicator/corroborator family. This lock
does not download a dataset, create consent, transform media, run inference, or begin certification.

Matching a rights digest is necessary but is not a current rights decision, and a currently valid document is
not proof that it governs the source beside it. Before planning or decoding any source, the authority locker
asks the corpus owner to validate every case's exact rights bytes together with its exact truth-provenance bytes
and source authority at the fixed authority-authored time. Shared rights documents may be decoded and cached
once, but case binding is never deduplicated. The production command hard-wires that validator; callers cannot
omit it. The closed initial vocabulary is the complete VCTK release-authority v1 envelope and the canonical
known-script rights v1 envelope. VCTK validation rechecks the release identity, rights-review chronology,
complete member authority, hosted-evaluation contract and current term, then requires the provenance's named
member and original evidence authorities to occur in that release and its wrapped output identity to equal the
case source authority. Known-script validation rechecks the participant binding, every grant,
expiry/withdrawal state, time-sensitive music/noise rights, authority and transformation binding, then requires
the packaged output identity to equal the case source authority. Unknown, malformed, unsupported,
noncanonical, expired, withdrawn, unbound, or mismatched evidence invalidates the complete lock before media
work or publication. The exact hosted route was
already authorized immediately before each model request; the later lock establishes that the participant and
asset rights remain current, not that an expired grant retroactively authorized a call. A path-free immutable
authority is not a live withdrawal registry: any future production admission must recheck then-current rights
through a separately designed live-use boundary, and all output permissions remain false meanwhile.

A review verdict is `verified` or `rejected`; it is not another copy of the draft's proposed `positive` or
`clean` label. A verified positive retains the draft's exact proposed intervals, while every rejected verdict
and every clean verification carries no interval. This distinction is required so a reviewer can reject a
supposed clean control after hearing prohibited speech without first manufacturing positive timing truth, and
can reject a supposed positive whose intended phrase is inaudible. Two agreeing rejections cannot lock the
draft. A primary disagreement requires the existing independent adjudicator, whose `verified` verdict is the
only outcome that can establish the draft's proposed truth. A rejected adjudication also prevents publication.

Public clean-speech preparation is upstream and non-authorizing. The first pinned adapter accepts an already-
acquired VCTK 0.92 tree, an exact release/member manifest, a completed certification-rights contract, and a
private seed. It does not crawl or download the release. It verifies the archive/release, licence, README,
rights-review, transcript, speaker, microphone, and audio identities; p315 is excluded because the release does
not supply its transcript. From owner-screened eligible utterances it deterministically selects exactly one
utterance from each of 100 distinct speakers. Alternate microphones, takes, encodes, and later transformations
retain that speaker family and cannot increase the denominator.

Because the evaluated cascade requires complete audio and video, the adapter wraps each selected real utterance
in a deterministic neutral-video MP4 using exact ffmpeg/ffprobe executable identities and a fixed recipe. The
speech remains real but its encoding is a declared derivative; input/output hashes, decoded duration, recipe,
tool versions, and source-relative complete span are retained. Both output streams must also survive a complete
bounded decode; a header-valid derivative with a corrupt tail is refused. The adapter emits a private review-ready
cohort and owner map, not a certification authority or clean label. A later full-draft assembler combines it with the
positive and other clean-slice cohorts before any review bundle binds that exact draft. Reviewing the VCTK
cohort before assembly would bind the wrong digest and is forbidden. Raw media, transcripts, speaker ids,
paths, seed, and maps stay outside Git. Missing rights, release drift, an unsafe path, duplicate family/content,
tool drift, an incomplete output, or fewer than 100 eligible speakers fails before atomic publication. This
preparation performs no provider call, policy classification, certification run, or spend.

Consented known-script positive preparation is a second upstream, non-authorizing adapter over recordings
that already exist. Its one external interface accepts a private owner-authored cohort authority, a private
source root and alias seed, exact ffmpeg/ffprobe executables, a fixed preparation time, exact case and resource
ceilings, and a new output directory. It never discovers participants, solicits consent, records or synthesizes
speech, authors a restricted script, downloads media, or sends content to a provider. A file's presence is not
consent: every real participant has a separate bound consent document, signer-authority evidence, processor
schedule, withdrawal instructions, and owner-reviewed consent contract. That contract explicitly covers
collection, private storage, deterministic modification, evidence extraction, independent review, hosted model
evaluation, the participant's redistribution choice, retention and withdrawal, and no endorsement. An expired,
withdrawn, incomplete, ambiguous, or mismatched contract fails before any media tool runs.

The owner authority binds exactly one selected take or derivative for each private participant id and source
family. It also binds recording session/take identity, locale/accent, exact versioned script bytes, private
policy digest and policy-mapping evidence, dry master and selected audio bytes, source-relative intended
positive intervals, and a deterministic transformation record. That record names its recipe and digest,
rendering tool identity and time, master and output authorities, and every music/noise/mix asset with separately
reviewed rights covering the same transformations and hosted processors. Retakes, cuts, re-encodes, mixes,
clipping, placement derivatives, and alternate representations of one participant remain one source family and
cannot inflate the denominator. The selected cases must contain at least 59 unique participants, use only the
locked positive-slice vocabulary, and cover every required positive slice; a music-overlap declaration requires
a rights-cleared music asset.

Preparation reopens and hashes every authority-bound byte, derives opaque case/family ids from the private seed,
copies the selected script only as the private transcript, and wraps the selected real audio in the existing
deterministic neutral audiovisual recipe. Exact tool identity is checked before and after processing, both final
streams must fully decode, and every proposed interval must remain ordered, non-overlapping, rule-valid, and
bounded by the measured final source. Atomic private output contains one positive-candidate cohort, an owner map,
and per-case source, transcript, provenance, and rights documents. Restricted script text, participant identity,
input paths, and seed never enter public output, Git, logs, or errors. The script and preparation establish only
an intended claim: two independent reviews over the fully assembled draft, plus disagreement-only adjudication,
still establish certification truth.

Prepared spoken-safety cohorts use one private candidate contract. Every case binds its complete audiovisual
source, optional exact transcript, source family, rights and truth-provenance evidence, proposed `clean` or
`positive` claim, locale, slices, and any proposed positive intervals. A preparation adapter may propose those
facts from source-owned evidence, but the proposal is not certification truth. Clean candidates carry no positive
intervals; positive candidates carry sorted, non-overlapping, bounded intervals with the exact restricted-rule id.
The cohort document binds transcript bytes separately even when provenance also names the original transcript,
so a model or human reviewer cannot unknowingly assess a changed convenience copy.

One deep offline assembly module owns the transition from separately prepared cohorts to the single review seam.
Its interface is one operation over a private assembly plan, one private input root, and one new output directory.
The plan binds the exact private policy bytes; draft envelope and route identities; every cohort document/root,
kind, dataset, digest, and exact case count; the expected combined case count; and aggregate input, output, and
wall-time ceilings. The module revalidates current source, transcript, rights, provenance, policy, and cohort bytes;
requires one policy and implementation; rejects cross-cohort case, source-content, or source-family collisions;
and enforces the certification minima and complete positive/clean slice vocabulary before publication.

Assembly snapshots only referenced verified bytes into one self-contained `0700` tree. It writes private `0600`
case media, transcripts, provenance, and rights evidence; the canonical #929 path-bearing `draft.json`; the exact
policy; and two byte-identical primary-review worklists bound to the draft digest, policy, evidence, and case order.
Each worklist may
show the proposed claim and intervals needed to verify known-script evidence but contains no evaluation result,
other review, reviewer identity, or completed decision. Outputs are deterministic for the same plan and inputs,
created atomically without overwrite, and remain non-authorizing. Reviewers submit separate #929 review documents;
two complete independent agreements, or disagreement-only independent adjudication, are still required before the
authority locker can establish truth. Assembly runs no model, reviewer, certifier, downloader, or production ingest.

Independent routine review is one separate deep module, not a human playback loop hidden in the assembler.
Its interface is one operation over a private review plan, the assembled private root, an API credential, the
exact local ffmpeg executable, a private checkpoint directory, and a new output path. The plan binds the exact
draft, worklist, private policy, fresh OpenRouter capability/price/ZDR snapshot, reviewer and model-family
identities, one exact requested/resolved model and upstream endpoint, expected case count, and request, charge,
spend, input-byte, audio-byte, per-case-time, and wall-time ceilings. The reviewer model family must differ from
the draft's proposer, native-audio adjudicator, and complete-video corroborator. A second primary review uses a
separate plan, reviewer identity, checkpoint, and model family; the operation never sees the sibling review.

For each case the module first reopens and hashes the source, draft, worklist, policy, and evidence bindings,
then asks the rights owner to authorize the exact hosted processor before any media tool, checkpoint creation,
HTTP request, or spend reservation. A rights document bearing the known-script rights-envelope contract is strictly
decoded inside the corpus module; its participant binding, grants, expiry/withdrawal state, processor schedule,
and time-sensitive asset rights are revalidated at review time. Authorization requires an exact match on the
OpenRouter HTTPS base URL, requested and resolved model, upstream provider name and slug, and ZDR. A malformed
recognized envelope or unmatched route fails closed without logging consent contents, participant identity, or
private paths. Other rights contracts retain their existing validators and are not reinterpreted as participant
consent. After preflight, the reviewer extracts the complete soundtrack from the verified source snapshot into
bounded 16 kHz mono WAV. Calls
are serial, fallback-disabled, ZDR-only, and strict-schema. The prompt exposes the private policy, proposed
claim, opaque rule ids, and proposed intervals but no evaluation output or other review. The model may return
only `verified | rejected | unclear`, `clear | degraded | no_speech`, sorted opaque matched-rule ids, and the
indexes of proposed intervals it heard; it is instructed never to transcribe or quote speech. A positive is
verified only when every proposed interval is confirmed with its expected rule. A clean candidate is verified
only with no matched rule. A contrary decisive observation becomes `rejected`; unclear or degraded evidence is
an operational stop and cannot become a review verdict.

The maximum per-call charge is durably reserved before HTTP and exact usage is settled afterward. The private
checkpoint binds all input, prompt, schema, route, tool, and budget identities; accepted cases form a canonical
prefix and are not called again. A process interruption after reservation remains an unsettled hold rather than
an automatic replay. Source or identity drift, ambiguous output, provider failure, a stale route snapshot, or
exhausted resource limits leaves no review bundle. Only an exhaustive decisive pass publishes the canonical
model-backed authority-review document at mode `0600`; it performs no authority locking, certification scoring,
training, ingestion, scheduling, or production admission.

The model review embeds a bounded, path-free evidence record rather than asking the authority locker to trust a
free-form model-family string. That record binds the plan, worklist, policy and snapshot digests; exact requested
and resolved model and upstream endpoint; model family; prompt and schema digests; ffmpeg identity; fixed resource
ceilings; aggregate usage and settled charge; and every attempt's case id, request/response digest, generation id,
state, observation digest, token counts, and settled charge. It contains neither the response body nor source
content. The review envelope carries the canonical evidence digest, and every final reviewer attestation retains
that digest. Human reviews instead bind a separately authored evidence digest and cannot claim model evidence.

No model is trained or fine-tuned for this lane until governed source-disjoint labels exist and the
certified stock cascade demonstrably misses a locked gate. Existing unknown commercials and agreement
between candidate models remain development observations and cannot be promoted into training truth.

The temporal `unusable` answer is diagnostic history, not media-integrity truth. Media integrity,
presentation/source defects, broadcast suitability, semantic unit/role, and rights are five
independent claims with independent policy owners. The maintained media-integrity challenge consumes
the exact full-decode report produced through `filler.EvaluateMediaQuality`; it never copies or tunes
the production black, silence, or freeze thresholds. Its closed integrity slices are `no_video`,
`no_audio`, `decode_failure`, `near_total_black`, `near_total_silence`, `stuck_or_low_motion`, and
`clean_assessable`. Its separate presentation vocabulary is `screen_recapture`, `player_or_browser_chrome`,
`timecode_or_recording_overlay`, and `third_party_stock_watermark`. A presentation observation cannot
become an integrity failure merely because both occur in one clip.

Challenge preparation takes a private authority manifest, the exact schema-v2 full-decode report,
and a fresh secret seed. It emits a label-free public package containing only fresh opaque aliases
and content/measurement digests plus an owner-only map binding those aliases to source identities,
closed labels, expected `reject | review | continue | hold` outcomes, presentation observations, and
an explicit low-motion disposition. Public bytes are scanned for every private case id, evidence
alias, label, and seed before atomic publication. Missing or extra cases, duplicated aliases,
invalid media or measurement digests, changed tool/policy/report identity, unknown vocabulary, and
any label-bearing public field fail closed.

The locked scorer rejoins the package, private map, and exact full-decode report by digest and scores
each expected outcome against the production-policy result. A missing measurement or operational
failure observes `hold`, never `continue`. Each integrity slice reports cases, exact confusion counts,
accuracy, and the one-sided 95% Wilson lower bound. Presentation observations are counted separately
and never alter integrity correctness. The known noisy/static case may carry
`measured_gap` only when the current detector did not hold it; that gap remains visible and cannot be
converted into a passing detection or a threshold change. Every output remains private,
content-addressed, and `productionAdmissionAllowed: false` until a representative locked challenge
and the independent certification holdout pass their own predeclared gates.

When that stop fires, a stronger hosted model may inspect only the comparison's deterministic,
stratified calibration selection before another development-scale run. The selection is a separate
content-addressed artifact bound to the package, both local assessment hashes, and the comparison
hash; it contains only opaque aliases, reasons, and strata. It cannot silently expand to all disputed
cases or the 300-case corpus. The maintained calibration is serial and makes at most one unit call and,
only for `standalone`, one role call per selected case, so a 15-case selection has a hard 30-request
ceiling. It uses the same less-than-24-hour OpenRouter capability snapshot, exact requested/canonical
model and upstream route, ZDR, fallback-disabled strict-output contract as hosted review. Every call
reserves its declared maximum nano-USD charge in a private checkpoint before HTTP and settles exact
provider cost when returned. A normally returned failure with missing settlement keeps the distinct
reservation consumed; a crash-stale `reserved` call blocks automatic resume. Neither state is retried
or converted to a semantic label. The public result binds the ordinary provider-neutral assessment set
to the selection, capability snapshot, prompt, route, request ledger, known charge, and still-consumed
unknown-charge reservations. This diagnostic remains non-certifying and cannot authorize unattended
admission or the 300-case relabel by itself.

The shared `openroutermedia` capability snapshot is the sole owner of model, endpoint, and pricing
validation for runtime and certification. Schema 3 adds at most four canonical prompt-token pricing
tiers per endpoint. Thresholds are positive, strictly increasing integers; each tier contains only
its changed price fields and inherits omitted fields from the validated base pricing. Previously
captured schema-2 snapshots remain valid only without tiers; a schema-2 artifact carrying tiers is
invalid. The adapter preserves the recorded schema and never invents a tier, upgrades old evidence,
or bypasses the shared validator. Exact route, privacy, freshness, capability, and positive
reservation checks remain mandatory, and known over-reservation charges remain accountable holds.

A hosted review run additionally binds one exact upstream route from a capability snapshot no more
than 24 hours old. It requires image and text input plus strict structured output, zero-data-retention
eligibility, one provider attempt with fallback disabled, and response metadata proving the selected
route and catalog canonical model revision. Before every serial request it atomically records a
private `0700` checkpoint reservation, including the package-manifest, transcript-set, capability-
snapshot, prompt, requested and resolved model, upstream provider/route, reviewer, batch, request-body
SHA-256, and the unchanged request, per-request charge, and total nano-USD ceilings. Only then may the
request leave the process. A settled attempt records its generation, tokens, latency, exact charge,
state, and, when accepted, the hash of its normalized per-case submission; accepted submissions and
their matching calls are stored in the same `0600` checkpoint. An interrupted request whose exact
charge cannot be settled remains reserved and blocks automatic recovery rather than being treated as
free.

Exactly one process owns a hosted-review checkpoint at a time. Before loading checkpoint state, the
runner creates `<output>.private/active-run.lock` with `O_EXCL`, mode `0600`, and a record binding the
checkpoint-identity SHA-256, start time, and diagnostic process ID. It holds that lock across every
load, reservation, persistence write, HTTP request, validation, and return. A competing process fails
before HTTP. Every normal return verifies that the lock bytes still match the owner's digest, removes
the lock, and syncs the private directory; a failed release suppresses otherwise completed output.
A crash deliberately leaves the lock behind. Neither PID existence nor lock age may infer staleness.
After independently establishing that no owner remains, an operator must run the hosted-review CLI
with the same `--out` and `--recover-lock-sha256 <exact digest reported by the blocked runner>`; this
mode makes no provider request, atomically renames the lock to a digest-named recovery audit, and
exits. A missing or changed digest fails closed.

The same CLI has a strictly offline inspection mode only for Reviewer B's exact 300-case hosted-review
checkpoint. It accepts the exact package, transcript set, historical capability snapshot, route
identity, prompt identity, and original ceilings; re-hashes and validates all of them plus the exact
`0700` checkpoint directory, exact regular `0600` checkpoint, complete settled attempt accounting,
package order, and any bounded exact regular `0600` active lock; and rejects symlinks, other types, and
all other modes, including setuid, setgid, and sticky bits. The package and checkpoint directories are
opened without following their final symlink and retained as descriptor roots; package descendants are
opened component-by-component relative to that root without following symlinks. The package tree is an
exact closed set of `manifest.json`, its declared instructions and label template, every declared
signal, and only their required ancestor directories. The checkpoint tree contains exactly
`checkpoint.json` and, when present, `active-run.lock`. Any other file, directory, symlink, device, or
special object fails inspection regardless of its mode. The checkpoint,
optional lock, transcript set, and snapshot are likewise mode-checked and read from the same opened
descriptors, so a pathname replacement cannot redirect validation to one object and reading to
another. Historical inspection validates the snapshot's immutable schema, source, model, route,
capability, privacy, and digest identity but does not apply the live run's 24-hour freshness window.
The live runner still rejects a snapshot older than 24 hours before constructing or invoking its
request path. The offline CLI control path has neither credential lookup nor the provider-capable run
function available to its inspection branch. It never reads a credential, creates a lock, mutates any
input directory, or constructs a provider client.

Its content-addressed attestation is a sanitized inspection projection only: permitted artifact,
prompt, checkpoint, identity, and optional active-lock hashes; closed inspection status; aggregate
case and historical request counts; and historically recorded immutable request/monetary ceilings and
remaining allowance. Its SHA-256 is independently recomputable from the canonical emitted fields with
the digest field omitted. It emits no raw batch, reviewer, model, provider, route, or prompt-version value.
An incomplete checkpoint without a lock is `awaiting_explicit_maintainer_approval`. A present valid
lock is only `active_run_lock_present`: it proves neither stale ownership nor recovery authority.
Presence is tracked separately from lock bytes, so an empty `0600` lock is invalid rather than absent.
Every state reports provider execution unauthorized; inspection authorizes no provider call, recovery
run, or spend reservation. Missing inputs, identity or ceiling drift, unsafe modes, duplicate or
out-of-order state, an invalid lock, an unsettled reservation, or unverifiable accounting emit no
attestation and fail closed.

A resumed hosted review re-hashes every package, transcript, snapshot, prompt, request, and accepted
submission and requires the checkpoint identity and ceilings to match exactly. Identity drift,
unknown fields, permissive or symlinked state, duplicate aliases, duplicate accepted attempts,
unbound calls, and altered hashes fail before HTTP. Previously accepted aliases are never requested
again. Only the failed alias is retried, as a new separately reserved attempt, and every prior failed
or accepted request and exact charge continues to consume the original ceilings. The maintained
300-case route defaults to a hard 301-request ceiling so one failure can be retried; a second failure
exhausts that fixed recovery allowance, and configuration rejects a ceiling above 301. A missing or
mismatched route, usage charge, schema, or case result still aborts the batch.
The private checkpoint is not a label submission: public output appears only when exactly 300 unique
accepted aliases and their complete settled attempt ledger validate. Published attempts must follow
submission/package order: all failed attempts for one alias are contiguous immediately before that
alias's one final accepted attempt, and no later alias may interleave. `labels.jsonl` plus the
schema-2 `review-run.json` are then published together by one atomic directory rename. There is no
schema-1 resume adapter: runs created before private request reservation have no trustworthy accepted-
case or cumulative-charge ledger and must not be treated as resumable evidence.

The maintained review packager turns one such opaque packet into inspectable reviewer evidence; a
hash-only packet is not a completed review handoff. It consumes the exact draft, reviewer packet,
owner-only alias map, provider evidence-packet JSONL, and external derivative root. Before writing
anything it verifies every draft, alias, content, evidence-packet, and derivative hash. Its
reviewer-visible manifest retains only opaque aliases, bounded segment coordinates, sanitized decoder
facts, and alias-relative audio/frame/video paths; it omits source text, source-policy facts, case IDs,
split/cluster assignments, titles, filenames, creator, campaign, source family, provenance URLs, and
the private map. Media is materialized by an explicitly selected hard-link or copy mode, never by a
symlink, and the output is published atomically only after every materialized file re-hashes correctly.
The package includes an intentionally invalid empty label JSONL template in packet order so an
unfilled or partially filled handoff cannot be mistaken for a completed submission. Packaging does
not call a reviewer, infer labels, or make the two review batches independent; those remain separately
executed and attested steps.

Certification scores exactly one named split. Development examples cannot inflate locked-holdout
metrics, and exact or near-duplicate content cannot cross their source/similarity cluster boundary.
The replay report is deterministic for the same manifest, captured predictions, and explicit run
identity: generation time is an input, never the scorer's wall clock. The run predeclares positive
request, spend, and concurrency ceilings; captured attempts or charged cost beyond either ceiling
fail closed. Reports carry the exact manifest digest and one-sided Wilson bounds for admission,
rejection, automation, review, and slice accuracy, including an upper bound for review rate. Each
certification slice gate predeclares both a point threshold and a confidence lower bound.

An open-weight candidate is certified through the same label-blind packet and replay contract, not
through a separate favorable prompt. A local Ollama route is loopback-only, pins the exact model tag
and registry digest before inference, disables model thinking for bounded structured extraction,
uses one attempt at concurrency one, and records prompt/completion tokens plus wall latency. Local
execution has no provider charge, but still reserves request and execution ceilings; a mutable tag,
missing digest, non-loopback endpoint, malformed structured response, or exhausted output budget is
an operational failure. Hosted evidence does not establish quantization quality, resident memory,
throughput, or appliance suitability, and a successful local load does not establish accuracy.

The locked 300-case `development_seed` uses that same packet, policy, route, ceiling, and immutable
prediction contract for model selection. It may run only after both blind submissions and any needed
adjudications have been locked into every selected case. Its replay report is always non-certifying:
development evidence can select a candidate and justify a cascade, but cannot satisfy a holdout gate
or authorize unattended admission. Rejecting an unlocked or partially reviewed development manifest
is a pre-provider failure. This is not a compatibility path around certification; it is the explicit
non-scoring half of the development/holdout experiment.

The production-reference audit composes that immutable development manifest with one separate,
versioned, negative-only content-review artifact. The review binds the exact manifest digest and
keys every finding by unique content SHA-256, never by case ID, filename, collection, uploader text,
or source title. Each finding cites at least two distinct in-clip frame, transcript, OCR, audio, or
video evidence rows already bound into that exact manifest case. The audit rejects an unknown or
duplicate content identity, a missing or mismatched evidence row, an unsupported evidence kind or
closed reason, a future review time, or any artifact/input digest mismatch before screening. A valid
finding may only exclude the exact bytes; it cannot admit, relabel, add taxonomy, mutate the locked
manifest, or satisfy later human playback acceptance. The ordinary policy remains general for every
unlisted case. Positive source and segment duration are also required: zero-duration media is
unusable even when an upstream fact calls it usable. The audit owns the exact raw manifest, packet,
mapping, acquisition-ledger, and content-review bytes: it strictly decodes them, rejects duplicate
object keys at every depth plus unknown fields and trailing values, and derives their recorded
SHA-256 identities itself rather than accepting caller assertions. A usable decoder fact requires
`no_video=false`, `no_audio=false`, segment bounds wholly inside the positive source duration, and
exactly one valid hashed, positive-byte, positive-duration, positive-dimension `video/mp4` evidence
presentation. Missing streams or a missing/malformed presentation are unusable; impossible segment
bounds invalidate the audit rather than being reinterpreted as a hold.

The one retained 300-case development cohort crosses the household licensing retirement through a
single, offline, fail-closed refresh, not through a legacy reader in the audit or runtime. The refresh
owns the exact locked manifest, packet JSONL, product mapping, and negative content-review bytes from
the v3/v1 reference contract. Every legacy packet must be schema 1, bind its manifest case and content
hash, contain exactly one deterministic decoder fact and exactly one eligible source-policy fact with
the legacy evidence ID `source-license`, and otherwise match the closed packet shape. The refresh removes only that retired
licensing fact, advances the packet to schema 2 / `filler-evidence-v2`, recomputes the case evidence
digest, and advances the corpus lock. Case, content, source, provenance, semantic truth, role, slices,
taxonomy, policy flags, independent label attestations, and adjudication remain byte-equivalent under
their typed representation; a change to any of them invalidates the refresh. Because label
attestations bind the semantic label projection rather than the transport packet digest, this
declared removal does not manufacture or repeat semantic review.

The product mapping and negative review are re-bound only after the refreshed manifest bytes exist.
The negative finding must still name the same unique content SHA-256 and the same in-clip evidence
references; it cannot be added, removed, or edited during refresh. One path-free machine-readable
report records all old/new artifact hashes, the exact contract transition, 300/300 transformed cases,
300 removed licensing facts, and preserved label/adjudication denominators. Outputs are current-only,
created atomically in a new directory, and must pass the ordinary v4 audit; the application retains no
v3/v1 adapter, feature flag, or compatibility branch. The refresh performs no media mutation, network
request, provider call, spend, semantic relabel, admission, or maintainer acceptance.

The production-reference duplicate inventory is a separate deterministic derivative of one exact
production-reference audit artifact. It fingerprints every non-excluded case and no other case,
re-opens each acquired source only beneath the declared source root, and verifies the full source
bytes against the audit's content SHA-256 before decoding. The inventory records the source-audit
SHA-256, algorithm version, case ID, content identity, local-file identity, complete ordered visual
fingerprint, and complete audio envelope. Missing, extra, repeated, excluded, mutated, unbound, or
symlink-escaped sources invalidate the whole inventory; a diagnostic subset cannot be published as
the cohort inventory. Visual comparison uses a fixed two-frame-per-second centre-crop dHash sequence,
ignores near-flat frames as positive evidence, and requires sustained fixed-offset agreement across
at least 70% of the shorter useful sequence. Audio comparison uses fixed 100 ms RMS bins and requires
at least 90% normalized correlation across the same minimum coverage. Either modality may relate a
pair, but neither assigns semantic truth, editorial grade, scheduling use, admission, or a preferred
rendition. Related pairs form deterministic connected components. A transitive component that is not
a complete clique remains explicitly unresolved for full playback rather than being silently
collapsed. Once a family is recorded, every later development/holdout or inspection split treats it
as one indivisible similarity cluster: exact or near-duplicate members cannot cross a split boundary.

Gate B begins from one immutable inspection seed bound to the exact production-reference audit and
duplicate-inventory bytes. The seed names exactly 50 non-excluded, positive-duration sources within
the two-minute conditioning ceiling, records its four-frame triage method and limitations, preserves
the 32-clip proposal target and any role/source coverage shortfall, and requires full playback as its
next gate. Selection is a bounded inspection proposal, not a preferred-rendition decision: duplicate
membership is visible, a non-clique family stays unresolved, and selecting renditions for comparison
does not collapse or grade them. Archive and operator-authorized YouTube are independent first-class
partner lanes under the same provenance and inspection rules; absence or scarcity in either lane is
reported rather than making one conditional on failure of the other.

The Gate B selector owns and strictly decodes the exact raw Gate A, duplicate-inventory, and seed
bytes, derives all three SHA-256 identities itself, and recomputes the duplicate inventory from its
stored identity and relationship graph before returning any case; it does not reopen media or repeat
the quadratic fingerprint comparison at this later gate. Unknown or duplicate JSON members, trailing values,
identity/summary drift, a missing, extra, repeated, excluded, over-duration, content-mismatched, or
family-unbound case, an unreported family collision, or an empty/full-playback gate invalidates the
whole selection. The measurement command then re-opens each selected source beneath the declared
root without following an escaping symlink, verifies its full content SHA-256, and runs the bounded
conditioning contract above with one two-minute process deadline per source. Decoder/probe failures
become explicit technical holds; identity, containment, input-contract, time, or publication failures
abort the artifact instead. Output is immutable and deterministic for fixed inputs and time, records
every selected case exactly once, and reports only measured versus technical-hold counts and errors.
It neither edits media nor assigns filler identity, taxonomy, editorial grade, scheduling use,
acceptance, admission, or provider authority. At least 48 measured sources may proceed to complete
playback; otherwise Gate B records an honest technical shortfall and stops.

The one retained v3 inspection seed crosses the licensing retirement only through an offline,
fail-closed rebind after the current Gate A and duplicate-inventory bytes exist. The rebind strictly
decodes all three artifacts, requires the legacy seed identity, validates the current audit and full
duplicate graph, and changes only the contract version and the two artifact SHA-256 bindings. It then
passes the result through the ordinary Gate B selector before publication. The ordered 50 case IDs,
selection policy, triage method and limitations, explicit findings, status, and required next gate
remain byte-semantically unchanged. A path-free report records the input and output identities and
the preserved selection count. The rebind performs no media access, selection, backfill, editorial
decision, acceptance, or admission and creates no general v3 compatibility path.

Shared transcription is a separately locked provider artifact, not text pasted into a mutable packet.
For each case it binds the exact raw packet digest, audio signal identity/hash/bytes/duration, transcript
schema and prompt versions, whisper implementation identity, executable SHA-256, model filename and
SHA-256, generation time, wall latency, timed utterances, canonical joined text, and text SHA-256.
Generation revalidates the packet and WAV beneath the declared corpus root before invoking the engine,
runs serially with an explicit per-case timeout, refuses an existing output, and atomically publishes
only a complete JSONL set. A final spoken decoding window that overlaps the measured WAV tail is clipped
to that measured duration; a wholly out-of-range window, an extreme tail, or any earlier overlap remains
invalid. The engine's exact non-speech `[BLANK_AUDIO]` sentinel is discarded rather than treated as
semantic evidence. A zero-width timed segment is likewise discarded because it has no supported
location; reversed or overlapping timing remains invalid. A transcript set must revalidate all bindings before any classifier call and
contains one artifact for every selected
packet with exactly one certified WAV and no artifact for a packet without audio; ambiguous or
uncertified audio is invalid. This keeps deterministic unusable/wordless cases in the corpus without
inventing an audio binding or calling the speech engine. One set is generated once per speech-model candidate and reused
unchanged across text and vision candidates; classifiers never rerun speech-to-text privately.

The local Ollama adapter supports both text-only and ordered-frame routes through the same evidence
schema. A frame route maps the already verified JPEG bytes to Ollama's per-message `images` field in
packet order and exposes only the matching frame signal ids in the untrusted payload. It neither opens
paths itself nor changes the text-only wire shape. Local vision therefore receives the same four-frame
ceiling as hosted vision, while a model that cannot accept that unchanged route records an operational
failure rather than receiving a smaller favorable prompt.

Source inventory is a separate, non-certifying preflight. Its only live contract is strict
source-neutral schema v5: one snapshot may combine multiple captures, and every case carries the
authority, authority-qualified stable case ID, all capture IDs that discovered it, role hints, frozen metadata evidence, exact selected
representation, acquisition-time campaign and source-family identity when known, and the adapter's
explicit media-host allowlist. Capture-level request,
response-byte, predicted-media-byte, and wall-clock ceilings remain visible beside actual usage.
An adapter's configured snapshot time is the latest permitted source observation, not the time it
pretends the capture occurred. The published inventory and capture snapshot equal the latest search,
metadata, or representation-header observation actually consumed; crossing the configured ceiling
fails before publication. Live requests and cache hits are reported separately so a cache-backed
replay does not masquerade as either a fresh network capture or an unexplained zero-request result.
The combiner strictly decodes every capture artifact, sorts captures and cases by their stable IDs,
rejects duplicate capture identity, and folds a repeated case only when its frozen metadata and
representation are identical; the merged case retains the sorted union of capture IDs and discovery
role hints. Any conflicting repeated case fails closed before producing the single rights-review input.
The former schema v1 put `source` and `collection` at the document root, so it could represent only
one Archive.org collection. Schema v2 let a later split plan invent campaign and source-family
identity instead of freezing that provenance during acquisition. Schema v3 assigned each case to
only one capture, so legitimate role-specific discovery either duplicated the case or discarded part
of its provenance. No certified artifact consumes an older shape; all are rejected rather than
adapted or preserved.

An Archive capture uses the exact `archive.org/<collection>` authority that bounded its search; it
never attributes a non-Prelinger item to the Prelinger collection merely because both use the same
Archive transport. The closed host policy still permits only Archive's HTTPS media hosts for every
such collection-qualified authority. It uses one identified serial client, cached raw search/item responses, a minimum
inter-request delay, and explicit request, item, per-item byte, and total predicted-byte ceilings.
Search-level and item-level licences must agree and NC/ND candidates are excluded, but an allowlisted
uploader field remains only a candidate: independent rights adjudication is still required. Every
adapter freezes retrieval times, response hashes, selected representation identity/checksums, and
predicted bytes before any media download or model call. A partial bounded capture reports exactly
what it saw; it never widens the ceiling or treats a truncated search response as complete.

The non-scoring 300-case development set has one narrower, maintainer-authorized Archive policy. An
independent reviewer may accept the frozen exact-item assertion as operational acquisition authority
when search and item metadata agree on CC0, Public Domain Mark, the legacy Creative Commons public-
domain assertion, CC BY, or CC BY-SA and the capture has already excluded NC/ND. The locked rationale
must say that this is not a chain-of-title warranty, preserve attribution and ShareAlike obligations,
and bind the exact representation and metadata hashes. Media stays in the external corpus store.
This policy cannot qualify a scored holdout case, satisfy source-diversity gates, or authorize Loomarr
to ship the media; holdout rights continue to require the normal evidence threshold above.

Before a new reusable source adapter is built, a source-neutral **rights-yield pilot** locks exactly
ten metadata-only candidates from each qualified lane: Prelinger, Library of Congress, NASA, CDC,
and Wikimedia Commons. Blender is retired as a pilot lane because its live first-party surface could
not supply ten distinct trailer candidates without duplicate encodes, full films, or dead media
links; individually cleared Blender works may still enter the direct/static cohort
(`filler-corpus-blender`; `retired-ok`). Each lane records positive request, response-byte,
predicted-media-byte, and wall-clock ceilings plus actual usage; every candidate freezes its source
identity, role hints, metadata hash/time, source rights assertions, and one predicted media
representation; category-traversed sources also freeze their exact discovery path. The strict
decoder rejects semantic labels and rights decisions because this pilot
is discovery evidence only. A lane earns an adapter only after an independent reviewer approves at
least five product-relevant items without lowering the common rights threshold. No compatibility
format exists: no completed pilot predates this contract, so the unpopulated six-lane shape is
removed rather than decoded or migrated.

Pilot qualification uses a separate deterministic worksheet bound to the exact locked-pilot
SHA-256. It presents all fifty frozen rows and neutralizes spreadsheet formula prefixes while
leaving reviewer identity, review time, independence attestation, rights decision, product
relevance, rationale, redistribution assessment, credit, and restrictions blank. Locking requires
one named reviewer, an explicit independence attestation, complete decisions for every row, and no
immutable-cell changes. A lane qualifies only when at least five of its ten rows are both rights
approved and product relevant. The resulting yield report says `downloadAuthority: false` at both
report and decision level; it cannot be supplied to the media downloader and never substitutes for
the corpus acquisition ledger.

A new corpus-source adapter requires a product-relevance review before implementation. The review
must show useful coverage of the filler roles the certification corpus needs, such as commercials,
promos, bumpers, station IDs, trailers, and PSAs. Rights clarity, API convenience, and inventory
scale do not by themselves make a source representative.

The LOC, NASA, CDC first-party-page, and Commons commands share one promotion seam: the bounded lane
remains the ten-case qualification artifact, while an explicitly requested schema-v5 output carries
the same frozen evidence into full-corpus rights review. Full capture counts stay positive and
bounded but are not hard-coded to ten; request ceilings must cover the declared item count. The CDC
adapter still consumes authored first-party page/media pairs and therefore cannot manufacture extra
cases to meet a quota. Multiple role-specific captures combine only through the strict inventory
combiner, never by concatenating JSON or discarding their individual ceilings.

The generic first-party-page seam also recognizes one closed USGS video authority,
`usgs.gov/media/videos`. Every case binds the matching item slug to an exact
`https://www.usgs.gov/media/videos/<slug>` item and metadata URL. Its representation uses only the
exact `usgs-ocapsv2-public-output-media.s3.us-west-2.amazonaws.com` host, stays beneath the canonical
`/assets/palladium/production/s3fs-public/` output namespace, and ends in an `MP4` directory with an
`.mp4` object. The contract rejects other USGS paths, wildcard or lookalike hosts, sibling or input
S3 buckets, credentials, ports, query strings, fragments, encoded path separators or traversal, and
redirects outside the exact page and media hosts. Recognizing this authority permits only inventory
publication: page assertions, soundtrack evidence, rights review, quality review, suitability, media
acquisition, provider processing, certification, and production admission remain independent gates.

The same seam recognizes the closed National Park Service video authority,
`nps.gov/media/video`. Every case uses the exact `www.nps.gov` host and binds one
canonical uppercase NPS 8-4-4-16 item ID to the exact
`https://www.nps.gov/media/video/view.htm?id=<UUID>` item and metadata URL. Its
representation stays beneath
`/nps-audiovideo/legacy/articles/<matching-UUID>/` on that same exact host. The
contract rejects other NPS paths, UUIDs that do not match the item, lookalike or
wildcard hosts, credentials, ports, query strings, fragments, encoded path
separators or traversal, and redirects outside the exact host. Required page text,
metadata digest, bounded response and predicted-media ceilings, and HEAD evidence
remain mandatory. This authority permits only inventory publication: NPS credit and
the copyright-symbol exception remain rights-review evidence, while soundtrack
evidence, complete-span fire or unsafe-act suitability evidence, media acquisition,
provider processing, certification, and production admission remain independent
gates.

The Blender Open Movie authority is narrower still. `blender.org/open-movies` currently recognizes
only item `sintel-trailer-720p`: the exact `https://durian.blender.org/download/` project download
page and exact `https://download.blender.org/durian/trailer/sintel_trailer-720p.mp4` representation.
The inventory must retain the representation name and `video/mp4` type. Other Blender projects,
subdomains, download paths, mirrors, directory listings, and generic project or licence pages are
outside this authority. The generic page adapter still requires authored page text, bounded page
and HEAD capture, immutable metadata identity, exact HTTPS hosts, and closed redirect handling.
This source rule grants only inventory publication. Soundtrack evidence, exact CC BY 3.0 scope and
attribution, music and embedded-work review, suitability screening, acquisition, provider
processing, certification, and production admission remain independent gates.

Direct/static acquisition is an authored local capture, not a source adapter and not a licence
shortcut. Its schema-v3 manifest predeclares an exact item count and positive quotas for the known
corpus roles and identifies one contracting owner or first-party origin; separate owners use
separate manifests so the emitted source authority is not collapsed into a generic direct bucket.
Those acquisition quotas may describe any bounded lane and do not duplicate the final truth-denominator
and holdout-role gates owned by certification. Every case names a non-empty regular media file plus
separate rights and provenance evidence files beneath one declared root, along with non-empty creator,
campaign, and source-family identity that survives rights review unchanged.
`filler-corpus-direct` resolves symlinks, rejects root escapes and quota drift, streams SHA-256 over
every file under aggregate byte and wall-time ceilings, and emits local transport records into the
same strict schema-v5 inventory. It never creates media, infers a grant from a directory or
collection, or turns authored assertions into approval. The combined public-plus-direct inventory
still goes through one independent rights review; local media is already acquired and is therefore
skipped by the network downloader. No direct-manifest schema-v1 reader or fixed 100-item compatibility
path exists because no certified artifact consumes one.

Corpus preparation is one fail-closed bridge from the fully reviewed inventory to blind review and
provider evaluation. An authored schema-v4 plan explicitly identifies either `development_seed` or
`certification` and may otherwise choose only the development/holdout split, similarity cluster,
source segment, direct-video window, corpus version, evidence version, and predeclared slice gates.
The caller must name the matching preparation profile; a plan cannot silently select it. The
development profile prepares every and only rights-approved row from the reviewed inventory, requires
at least 300 cases all in the development split, and leaves held rows inert. Campaign, source-family,
and creator identities are retained when the source knows them but are not invented merely to prepare
development evidence. The certification profile requires all 1,426–1,600 inventory rows to be approved
and covered exactly once, including at least 300 development and 1,126 independently clustered holdout
cases; it retains the complete acquisition-provenance and confidence-bound requirements. Both profiles
reopen media beneath separate local/direct and download roots; recheck size and all available
SHA-256/SHA-1/MD5 identities; measure an at-most-five-minute bounded segment; and emit source-policy
and decoder facts plus source text, four near-full-resolution frames, one 16 kHz mono WAV of that
segment for direct-audio and shared transcription lanes, and one at-most-60-second 1280×720
direct-video derivative. It stages derivatives before
publication and enforces aggregate source bytes, derivative bytes, and wall time. Preparation also
computes a 64-bit difference hash for each of the four semantic frames. Cases for which at least
three corresponding frames are within eight bits must share one similarity cluster; later holdout
validation permits only one case from that cluster. This catches re-encodes and close derivatives at
the only seam that still has media bytes, while source-family and campaign identity catch shared
masters that frame sampling misses. The resulting
draft copies creator, campaign, and source-family provenance from the acquisition inventory rather
than allowing the split plan to author it. It contains no semantic truth, evidence labels, or review
answer. Each packet is
validated against its draft digest before either artifact is written; provider input therefore
cannot acquire a hidden answer key through corpus preparation. No hand-authored packet or older
preparation shape is accepted.

`filler-corpus-review` derives one reviewer-visible randomized packet and one owner-only alias map
from that draft. A fresh batch and map are generated independently for each reviewer. Development
drafts remain `development_seed` through blind review and label lock and cannot satisfy the
certification contract; certification drafts alone receive the certification sampling and composition
checks. The label lock
accepts only that still-unlocked, wholly unlabeled draft, both exact alias maps, and strict
current-schema JSON/JSONL; unknown or trailing fields are errors, not compatibility data. It validates both blind
submissions as complete labels before comparing their canonical hashes, including the submission
that an adjudicator does not select. A third reviewer therefore resolves a real semantic
disagreement; adjudication cannot turn an incomplete or malformed second review into evidence of
independent labeling.

Schema-v5 inventory gives every exact representation a required **soundtrack expectation**. Its
closed status is `present_expected`, `intentionally_silent`, or `unknown`; the claim also binds a
closed evidence kind, the SHA-256 of already-frozen first-party descriptive, encoded-stream, or
reviewed source-manifest
metadata, a bounded evidence locator, and the SHA-256 of the representation identity. The
representation identity includes transport, name, URL/path, media type, origin, byte and source-hash
facts, duration, and dimensions. Changing any of those facts therefore invalidates the soundtrack
claim instead of carrying it onto different bytes. Publication year, title, a URL, or an unbound
free-text assertion is never soundtrack authority. A newly captured lane must state the claim
explicitly; omission does not become `unknown`. Historical schema-v4 inventories and discovery lanes
remain immutable evidence, but cannot authorize a new download or be silently upgraded to v5.

Soundtrack expectation is pre-download selection evidence, not decoded-audio proof. The generic
inventory accepts all three statuses and production does not globally reject intentionally silent
works. The versioned `temporal_structure_replacement_v1` quarantine-acquisition purpose is narrower:
only `present_expected` may enter its default download plan. `intentionally_silent` and `unknown`
remain visible with the closed `soundtrack_intentionally_silent` and `soundtrack_unknown` hold
reasons, and the rights locker and downloader reproduce the representation and
evidence binding before it creates an output directory or sends the first request. The rights
worksheet exposes the exact soundtrack fields and binds the acquisition purpose; its locked decision
must carry the same purpose. The ordinary `local_quarantine_inspection` purpose preserves the generic
inspection lane. A decoded file can still fail the independent full-source audio, silence, and
quality gates. Restoration, synthesized music, or separately licensed soundtracks require a new
designed purpose with their own rights, lineage, inspection, suitability, and admission authority.

Media acquisition consumes a separate rights-review ledger; discovery output is never download
authority. The caller names one of three non-interchangeable profiles: `quarantine`, `development`,
or `certification`. Every `approved` row binds the inventory digest, authority-qualified case ID,
source metadata hash, reviewer, review time, rationale, purpose-specific authority, attribution, and
restrictions; `held` rows remain inert. The downloader preflights aggregate item and byte ceilings
before its first request, stays serial and identified, checks the initial URL and every redirect
against both the case's frozen allowlist and the built-in policy for that authority, bounds each body
by the inventoried size, verifies source checksums when present, and adds SHA-256. Query strings may
remain when they are part of the exact frozen representation URL; credentials and fragments never
may. The request ceiling counts the initial request and every redirect hop before that hop is sent.
For Met original images, the inventory freezes a metadata-digest cache key, and each bounded
GET attempt may add its deterministic run/case/attempt cache key to avoid inconsistent CDN cache
entries. These keys cannot change the selected host or image path, relax exact byte counts or
available source checksums, or expand the existing request and byte ceilings. Media and its download
ledger remain external to Git.
An incomplete, stale, oversized, or checksum-mismatched plan fails without producing a completed
ledger and cannot flow into blind semantic review.

`quarantine` is the narrow pre-review acquisition profile. Its schema-v6 worksheet locks a
schema-v1 quarantine contract that must grant only local copying/storage and local technical
inspection. Provider transfer, redistribution,
development/certification corpus preparation, training, catalog ingestion, scheduling, and
production admission are all present and false rather than inferred from omission. An approved
quarantine decision may therefore retain `redistributable=false`; it is authority to obtain and
measure an exact source locally, not a finding that the source may be published or used. The
download ledger records the profile, and every downstream corpus-preparation profile rejects a
quarantine contract even if its item identity and content hashes are otherwise valid. Promoting a
surviving source requires a new development or certification rights decision against the same
frozen inventory; a quarantine decision is never upgraded in place.

`filler-corpus-quarantine-inspect` is the sole post-download quarantine gate. It consumes the exact
schema-v5 inventory, schema-v2 quarantine download ledger, and the complete public/private authority
pair for the named prior holdout. It strictly re-establishes those identities before opening media,
resolves each ledger path beneath its declared root without symlink escape, and rechecks byte count,
ledger SHA-256, and every available inventory checksum. One full-source probe and decode then records duration,
dimensions, audio/video presence, and normalized black, silence, and freeze spans. The same complete
decode cadence used by the production-reference duplicate audit produces visual dHash and audio-RMS
sequences. New candidates are compared to one another and to every distinct source in the prior
holdout authority; exact source or rendered-case hash collisions and perceptual relationships are
reported separately. Missing prior source bytes make perceptual exposure `incomplete`, never `clear`.
The caller supplies a positive media-processing wall-time ceiling, which is recorded in the report
and covers candidate and prior-source hashing, probing, decoding, fingerprinting, and in-process
fingerprint alignment. Expiration publishes no partial report.
Missing audio or video, inventoried duration/dimension drift, unusable fingerprints, or black,
silent, or frozen coverage at or above 95% is a technical hold; lower coverage remains measured
evidence for later content review rather than an invented quality score.
The immutable report binds every raw input digest, tool identity, algorithm version, observation, and
comparison. A technically intact and exposure-clear result means only `eligible_for_rights_review`:
provider transfer, redistribution, corpus preparation, training, catalog ingestion, scheduling, and
production admission remain explicitly false. The command never repairs, transcodes, uploads, labels,
or promotes media, and any failed mechanical check leaves the case held in quarantine.

Development and certification rights review consume that quarantine disposition as a fail-closed
authority input. Schema-v6 `quarantine` review remains the pre-download local-copy/inspection path
and cannot consume a report that does not exist yet. A development or certification worksheet that
can select any non-local case instead requires one strict schema-v1 quarantine-inspection report.
The worksheet freezes the raw report SHA-256 and all four report input identities. Report binding
advanced development worksheets from schema v3 to v6 and certification worksheets from schema v4
to v7; the soundtrack authority advances their current schemas to v7 and v8 respectively.
Historical worksheets and their decisions remain readable evidence, but cannot authorize new preparation.

Selection is transport-aware and deterministic: it is the union of valid direct
`transport=local` cases and non-local cases that occur exactly once in the bound report as
`eligible_for_rights_review` with no hold reasons. A held or absent non-local inventory case cannot
enter the worksheet. A local-only inventory does not invent a download-ledger or inspection-report
requirement. The rights lock independently reopens and validates the exact report, reproduces that
selection, and attaches the report binding plus inspected content SHA-256 to every locked non-local
decision; worksheet or CSV projections are never trusted as inspection authority.

Preparation independently reopens the report before it creates a derivative staging directory or
provider-visible packet. Every non-local approval must carry the reproduced report binding, and the
actual local source SHA-256 must equal the inspected content SHA-256. A held or missing case, report
drift, swapped worksheet, legacy unbound decision, or source-byte mismatch fails before output.
Direct local media retains its inventory-bound path and is still rehashed against every available
inventory checksum. One deep `fillerquarantine` module owns strict report decoding and validation,
inventory/report identity, case uniqueness and disposition, transport-aware selection, and
content-hash requirements; review, lock, and preparation commands do not reimplement that policy.

A rights worksheet is a deterministic review aid, not authority. It records the digest of the exact
frozen inventory, presents every selected source assertion and representation fact, and leaves the
reviewer, time, decision, rationale, redistribution, attribution, and restriction fields inert. Its
spreadsheet view neutralizes formula-leading source text and keeps those authority columns blank.
Locking a completed spreadsheet re-reads the original inventory and inert JSON worksheet, rejects a
changed header, immutable cell, missing or duplicate row, incomplete decision, invalid time,
unattributed BY/BY-SA approval, or inconsistent redistribution claim, and only then atomically emits
the downloader's JSONL ledger. The completed ledger binds each row to both that inventory digest and
the item's metadata digest; copying an approval between inventory snapshots fails closed.

The development-only Met lane may reduce repetitive spreadsheet entry without reducing that item-level
lock. Its batch-completion aid accepts only the exact current inventory and inert worksheet, a complete
zero-hold Met metadata pre-screen bound to the pinned Open Access policy evidence, and one separately
authored maintainer attestation bound to all three artifact digests. Met search and object metadata
must have unambiguous field identities: duplicate JSON keys, including alternate casing that would
bind the same decoded field, fail discovery or hold pre-screening. Legitimate additional provider
fields remain allowed; they do not become rights assertions. The attestation names the reviewer
and review time, explicitly accepts the recorded non-copyright limitations, and authorizes only private
development-corpus copying, technical transformation, and evidence extraction. The aid reconstructs
every immutable worksheet row and fills the same decision columns for each independently identified
item; the ordinary rights locker remains the sole producer of downloader authority and revalidates all
rows. Current report-bound development worksheets retain their exact quarantine report and per-case
content bindings through batch proposal and completion; the batch aid must not strip those bindings,
synthesize eligibility, or reinterpret quarantine-only permission as development or redistribution
permission. Missing, malformed, or inconsistent bindings refuse the batch. The ordinary rights locker
independently reopens and validates the exact inspection report before producing any downloader
authority. The attestation time must be canonical UTC and no earlier than its bound pre-screen; this
offline development artifact has no rolling production-certificate lifetime. A held pre-screen case,
mixed authority, incomplete coverage, stale time, changed artifact,
unknown field, blank attestation, attribution requirement, or non-empty restriction refuses batch
completion and returns the reviewer to the ordinary item-level exception path. The batch attestation
does not grant certification, provider transfer, training, production, ingestion, scheduling, or
broadcast authority and is not a model decision or chain-of-title warranty.

The visual-corpus nomination lock accepts an explicit `exclude` disposition so a completed review
does not have to mislabel an unsuitable or uncertain downloaded work as positive or clean. An
excluded row makes no subject, generation, or diagnostic-slice assertion and publishes no candidate
or rights file. Blank and unknown dispositions still fail the complete review. The nomination set
binds the exact canonical completed-review digest, total reviewed count, excluded count, and every
published candidate; the locker reopens the original inventory, materialization ledger, worksheet,
and media before publication. This is a workflow exclusion only: it creates no truth, training,
provider-transfer, certification, ingestion, scheduling, production, or broadcast authority.

Nomination preparation also emits a private, non-blind keyboard review board beside the inert JSON
and CSV. It shows the institution-authored identity and the worksheet-bound source image, and exports
the ordinary completed CSV consumed by the locker. A reviewer may load a separate local model-
assistance manifest only when its worksheet, case, rank, and exact content identities reproduce. The
browser may then prioritize proposed positives and, after one explicit confirmation, mark every
non-proposal as excluded; it never authors a positive or clean decision. Those decisions require an
individual reviewer action except in a worksheet whose every row carries the sole canonical
`policy-clean-nomination` role. For that clean-control worksheet, the board may render a bounded page
of exact source images and let the reviewer explicitly confirm all eligible, previously undecided
images on that page as clean only after a fully bound clean-assistance manifest covers every exact
case with two distinct local vision-model families plus a local OCR text-safety screen, every image
has loaded, and the reviewer attests that they
checked the whole page for adult nudity, minors, sexual or graphic content, hate symbols or text, and
other broadcast-unsuitable content. A version-two assistance manifest may add one complete, source-bound
frontier audience-review ledger with a closed suitability-flag vocabulary. It records observations and routes
cases only: every record binds the worksheet case, rank, exact source SHA-256, review method, and non-authority
flags; absence is never truth. The board accepts the version-two manifest only when its ledger digest, reviewer,
vocabulary, all per-case record digests, and complete worksheet coverage reproduce. Any observed suitability
flag makes the case ineligible for page confirmation and is rendered as a concise factual candidate signal for
individual inspection. Those digests are consistency seals, not reviewer-authentication signatures; the reviewer
still explicitly selects the private manifest. The page attestation explicitly covers sexual content; minor or
age-ambiguous sexual risk; violence, gore, death, self-harm, animal harm, weapons, and frightening imagery;
tobacco, alcohol, drugs,
gambling, and regulated-product promotion; hateful or extremist material; and prohibited visible written
language. It does not call ordinary minor presence, religion, politics, war, historical context, or brand
presence automatically prohibited.

A model-positive, age-risk, overlap-hold, or targeted-review row
is never eligible for that page action and must receive an individual decision; loading assistance
also clears any earlier convenience clean decision on such a row. Positive decisions always remain
individual. Every unresolved row remains blank so locking fails. The assistance bytes are neither
copied into the worksheet nor accepted by the locker. The board is development ergonomics, not the
candidate-blind certification review, and the locker still reopens every source byte after the
review.

The certification holdout uses a distinct schema-v8 worksheet and schema-v1 rights contract, while
quarantine uses its schema-v6 worksheet and schema-v1 acquisition contract. Historical schema-v3
development and schema-v4 certification artifacts remain readable evidence but cannot authorize new
preparation. The caller must name
the `quarantine`, `development`, or `certification` profile before either locking rights decisions or
downloading media. Certification binds one maintainer/counsel-approved agreement identifier and
SHA-256 plus one exact processor and terms-snapshot SHA-256 into the inert worksheet. Each completed
per-master schedule then binds its own identifier and SHA-256, confirmed signer-authority evidence,
and separate grants for commercial evaluation, copying/storage, technical transcoding, bounded
frame/audio/transcript/OCR extraction, the named processor transfer, and the permitted redistribution
scope. Provider transfer is never inferred from redistribution, a public licence, or zero-data-
retention routing.

The per-master schedule records a closed status for embedded music, performers and voices, stock
footage and artwork, trademarks, privacy/publicity, and locations; worldwide territory; the granted
term and any expiry; attribution and restrictions; and, when ambiguity was escalated, the distinct
adjudicator and disposition. Approval requires every required grant, confirmed signer authority,
each embedded-rights category cleared or proven absent, the exact worksheet processor and terms
snapshot, a non-expired worldwide term, and digest-valid agreement, schedule, authority, and evidence
records. Blank, unknown, conflicting, expired, or mismatched facts emit stable hold reasons and no
certification authority. Executed agreements, schedules, reviewer identities, and media remain
private external artifacts; repository fixtures are synthetic and public evidence records only
content digests.

This mechanical contract does not approve its legal form. The maintainer or counsel separately names
the agreement, rights reviewer and ambiguity adjudicator, outreach identity/channel and compensation
ceiling, processor boundary, and first recruitment batch before any contact, signature, acquisition,
download, provider upload, or spend. Changing any of those approved identities invalidates the
affected schedule rather than being treated as a compatible metadata update.

