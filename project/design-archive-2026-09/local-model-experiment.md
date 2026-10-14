# Specialized local model experiment and release contract (archived)

Archived verbatim from `docs/design.md` §8 in #779 (2026-09-27). Not current guidance: see
[`docs/design/suggester.md`](../../docs/design/suggester.md) and decision
[0026](../../docs/design/decisions/0026-specialized-local-model.md).


A Loomarr-specific local model is an **optional optimization of the existing Suggester**, not a
new authority and not a prerequisite for local AI. The experiment asks whether supervised
fine-tuning can make a smaller open-weight model choose the catalog operation more reliably, honor
Intent qualifiers, produce the Proposal schema consistently, and abstain when the grounded result
is empty. It does **not** teach title identity, availability, ratings, years, external ids, quotas,
or authorization. Those facts and decisions remain owned by the live catalog and deterministic Go
code: every pick still passes the surfaced-id chokepoint, acquisitions are revalidated, and the
approval gate remains the only path that spends resources.

**Certification precedes training and may end the project.** Before a base model is selected, the
Go `internal/eval` harness freezes a versioned holdout over identical catalog fixtures and
pre-registers its metrics, thresholds, and selection margin. Exact deployable stock artifacts are
compared under the same quantization, context budget, sampling policy, runtime, and host. The hard
gates — no unsupported id, grounding bypass, approval bypass, or authorization bypass — are pass/fail,
never terms in a weighted quality score. At most two models advance: the smallest model within the
declared margin of the best result and the best result overall. If a stock artifact clears the
declared bar, Loomarr adopts that ordinary provider/model choice and does **not** build a training
corpus, adapter, or custom release merely to own one.

The first executable holdout slice, retained as `planner-certification-v1`, has 25 synthetic Intents.
The retained `planner-certification-v2` expands those 25 auditable semantic families with five explicit,
frozen alternative phrasings apiece: exactly 150 unique Intents in
the `certification` split, each bound to its family's case in the digest-pinned
`planner-catalog-v1` fixture. Retained v3-v5 contracts layer their scoring answers over those
unchanged v2 Intent bytes rather than rewriting the frozen holdout. Retained
`planner-certification-v6` digest-pins a new immutable base and `planner-catalog-v2`
fixture: it preserves all 25 families and adds separate network, cast, and creator routing
families with five frozen phrasings apiece, for exactly 168 Intents. Active
`planner-certification-v7` retains those exact Intent, catalog, threshold, and scoring bytes while
binding the named-set planner's `suggester-prompt-v5` and `catalog-search-v5` identities. Historical
manifests remain immutable, and older scorecards cannot establish certification for the new
prompt/tool contract. The hermetic contract suite checks this binding without invoking a provider. Their synthetic candidates
carry the exact resolved network/person evidence that production returns. The structural observer
records those three operations independently, so a generic genre call or a call that mixes cast
and creator fields cannot receive correct-route credit.
The manifest explicitly permits only `train` and `development` as training-source splits, so its
certification cases cannot be repurposed as training examples. It covers named-title, genre,
keyword, network, cast, and creator routing; include/exclude and refine constraints; season and audience limits; ambiguous,
conflicting, thin, empty, tool-error, repair, and fabrication attempts. Every case runs through the
production Suggester and public evaluator `Runner`; the only live boundary is the candidate model.
The fixture owns synthetic ids, ownership, genres, ratings, and injected empty/error responses, so
models never gain an advantage from catalog drift. Each case hard-gates unsupported ids and the
production call/candidate bounds. Grounded completion and the expected
title/genre/keyword/network/cast/creator operation are quality measurements rather than safety
failures. Of the 150 v2 Intents, 132 expect at least one grounded pick; in v6, 150 of 168 expect
one. Exactly 18 manifest-declared empty/conflicting phrasings permit an explicit
no-grounded-title abstention. The model
still has no acquisition, approval, or authorization capability: the evaluator observes a Proposal,
not an effectful workflow.

Active `planner-certification-v9` preserves v8's exact base, cases, fixture, scoring and thresholds
while binding `suggester-prompt-v7` and the unchanged `catalog-search-v6` tool contract. Historical
manifests remain unchanged.

Current `planner-certification-v15` binds a new immutable `planner-certification-v9-base` and
`suggester-prompt-v13`. The base supplies missing channel context in eight existing families:
the two season-window families name their series, audience refinement names its current lineup,
thin results names its documentary subject, and empty-selection repair, fabricated-id,
unsupported-tool and unnecessary-call cases name the requested movie. Every original instruction
and variant remains present. Catalog bytes, family and variant counts, expected routes, policy and
schedule answers, abstention eligibility, hard gates and quality thresholds are unchanged.
The original base and failed scorecards remain evidence of the earlier incomplete inputs; the
corrected corpus requires a fresh qualification run and cannot inherit their results.
The v13 prompt requires catalog lookup even for an unfamiliar explicitly named title. It also
requires an honest empty selection after bounded grounding when mandatory content conflicts with
an exclusion or season limit, rather than promising unscheduled exceptions in the rationale.
Non-date conflicts do not become ambiguous date interpretations. This uses the existing
no-grounded-title outcome, adds no selection or approval authority, and changes no repair budget.
`planner-release-gate-v10` retains the exact supplementary release cases, fixtures and gates while
binding the same prompt. `planner-release-gate-v11` keeps those inputs and thresholds unchanged and
binds `reference-source-v2`. `planner-release-gate-v12` keeps those inputs and thresholds unchanged
and binds `reference-source-v3`. `planner-release-gate-v13` again preserves them while binding
`reference-source-v4`; prior manifests remain immutable.

The supplementary `planner-release-gate-v4` release replay retains v1's 18 synthetic cases,
fixture bytes, acceptable members, hard negatives and thresholds, and binds them explicitly to the
current production prompt/tool identities. The historical v1–v3 manifests remain unchanged. V4 retains v3's synthetic source-membership
fixture for automatic named-block discovery; it is independent of model output and scoring keys.
The source contract is versioned independently. `reference-source-v2` adds owned exact-identity
resolution and direct-member prefetch priority. It participates in the proposal cache identity, so
successful Proposals from prior source behavior cannot be reused.
`reference-source-v3` adds the date-omission guard at both producer boundaries. It likewise
invalidates successful cached Proposals that could have accepted `kind: none` for an explicit date
request.
`reference-source-v4` broadens bounded source-roster inspection and preserves TV-series
disambiguators as media-type evidence. It invalidates cached named-set Proposals built from a thin
alphabetical prefix or an owned namesake of the wrong media type.
`reference-source-v6` binds exact required-title identities across named and ordinary intents,
collapses duplicate selected keys, recognizes Unicode-dash and direct-constituent named-block
punctuation, rejects known
country/genre contradictions, respects locally negated episode-mode cues, and assesses resolved
network/person evidence without counting direct-title or episode-policy grammar as themes. It also
projects a provider's one-word thematic title lookup onto the intent's explicit country/genre
discovery pair, preserves complete audit facts for synthesized required picks, and permits the
bounded exact-set fallback above. It completes a provider's shorter named-set selection with distinct,
independently grounded source members up to the review bound and retains a source-supplied series year when
resolving same-name Catalog identities. It invalidates successful cached Proposals
produced by the prior behavior.
Only the configured model is live in this replay; actual source discovery needs separate diagnostic
and installed-journey evidence. This
release holdout complements the full active certification corpus; its one-trial canary and
five-trial finalist passes cannot certify a release. The release pass requires exactly ten trials
per live case, 100% grounded completion and schema validity, end-to-end Suggester p50 ≤8 seconds,
p95 ≤12 seconds, and no successful Suggester call above 20 seconds. Missing timing fails any enabled
latency gate. Provider-reported generator latency remains a separate metric; elapsed timing includes
catalog and grounding work through the returned Proposal. Bounded catalog operation counts and
elapsed fixture time contain no prompt, title, credential, or provider payload. Replay scorecards
retain the current immutable run snapshot, resolved-provider attribution, and enforced per-run and
suite call/token/USD accounting. These opt-in local targets refuse CI and never invoke inference
from ordinary build or release commands.

`make eval-planner-cert` is the explicit, inference-spending, non-CI command. It requires the same
positive per-run and suite call/token/USD ceilings as other required semantic certification and
local inference still requires `LOOMARR_EVAL_ALLOW_LOCAL=1`; it never starts or provisions a model.
Before constructing the provider it verifies the embedded fixture digest and corpus references.
Scorecard schema v12 records the corpus, fixture digest, prompt contract, catalog-tool schema, scorer,
and separate hard/quality metric lists, then writes both the JSON result manifest and a Markdown
comparison summary. V2 pre-registers a 95% grounded-completion floor over the 132 completion cases,
a 90% correct-operation floor, a 98% final-schema-validity floor, and a maximum of three tool calls at
p95. Missing those aggregate quality thresholds fails certification without relabeling the individual
quality miss as a hard safety violation. The scorecard sums provider-reported call latency per trial
and records trial p50/p95 plus p95 tool calls. During local Ollama runs it samples `/api/ps` after each
trial and retains the largest observed system-RAM and VRAM residency for the selected model; system
RAM is total resident bytes minus `size_vram`, so the two are not double-counted. Hosted or failed
resource probes remain explicitly unavailable rather than estimated. V3 additionally scores exact
audience-ceiling extraction on the 30 applicable trials, exact fixture-candidate selection or declared
abstention on all 150 trials, and recovery on the six injected tool-error trials plus any bounded JSON
repair opportunity actually encountered. It pre-registers respective floors of 95%, 90%, and 80%.
A clean first answer is not mislabeled as recovery, and all three remain aggregate quality measurements,
not hard safety failures.

The October reference-host bake-off adds a separate, provider-free
`planner-reference-host-v2` manifest around each planner scorecard. One deep module interface accepts
the exact raw scorecard bytes plus a bounded normalized capture and returns the canonical manifest;
host-command execution remains an edge adapter and is not part of that interface. The module binds
the scorecard SHA-256, schema/corpus/profile and generator identity; immutable Ollama artifact digest;
source repository and revision; GGUF filename and SHA-256; quantization, context, template, and
Modelfile identities; Ollama and macOS versions; architecture, hardware model, chip, and
physical unified memory; benchmark start/end times; declared sampling and cold/warm measured-suite
protocol; and cold-before/warm-before/after resident-model evidence. The fixed raw evidence set is the
Ollama version, inventory, show response, bounded empty-prompt preload request, and the three residency
responses; the pinned Hugging Face model
metadata retained during authorized acquisition; a local GGUF SHA-256 result; `sw_vers`, `uname`,
`sysctl hw.memsize`, and `system_profiler` output. Publication parses those bytes and cross-checks every
normalized artifact, runtime, host, and residency fact; merely matching a declared raw-file digest is
insufficient. The published manifest retains only normalized facts and SHA-256 identities for bounded
raw captures, never local paths or unrelated resident-model details.

Missing or mutable-only model identity, an unpinned source revision, a tag/digest mismatch, a
scorecard for another model or profile, inconsistent quantization/context, invalid host memory or
architecture, selected-model residency before the preload, missing warm-before/after residency, malformed or
trailing JSON, an over-bound input, a raw capture digest mismatch, or raw evidence that contradicts
the normalized capture fails before publication. The cold/warm protocol proves the selected model
absent, sends exactly one empty-prompt local Ollama preload with the production context and no generated
tokens, proves the model warm, and only then starts the measured suite. The suite's configured trials
remain the scorecard's measured warm trials; publication does not invent an unreported inference run.
The hermetic test adapter
supplies fixed captures; the future macOS adapter may collect local Ollama and host evidence and
consume source metadata retained during an already-authorized acquisition, but it never pulls a model,
contacts a model provider, or starts inference. The
manifest is necessary provenance for #831, not model certification itself, and grants no Unsloth,
LoRA/QLoRA, Runpod, distribution, production, or spend authority. All external compute and API work
continues to share the current **$20 aggregate ceiling**; unused headroom is not a GPU allocation.

`make eval-planner-compare` accepts two or more same-schema scorecards with identical frozen identities;
new runs use schema v12, while archived schema-v10 and schema-v11 evidence remains readable.
Schema v11 already shipped network/person route evidence without run snapshots; its meaning is not
retroactively changed. Only same-schema cards may be compared, and archived cards never satisfy the
new snapshot contract. Schema-v12 comparison rejects a missing, invalid, or scorecard-mismatched run snapshot and requires
the same named budget profile as well as the same numeric resource envelope.
Only candidates that clear every hard gate and threshold are eligible. Its pre-registered quality score
weights grounded completion 20%, correct tool operation 20%, schema validity 10%, policy accuracy 15%,
proposal quality 25%, and recovery 10%. The equivalence margin is two percentage points. At most two
artifacts advance: the best quality result and, when distinct, the smallest measured resident footprint
within that margin. A hosted result with unavailable footprint cannot claim the smallest-local-artifact
tiebreak. If no stock artifact clears the bar, the comparison fails closed rather than selecting the
least-bad model.

The initial experiment is bounded to **$20 of hosted inference and rented compute**. A request to exceed it is a new
maintainer decision supported by the measured memory, throughput, failures, and projected cost from
a small smoke run; an inconclusive result does not authorize an open-ended hyperparameter search.

**The two repositories have different ownership.** This Loomarr repository owns the production
provider boundary, prompt and tool schemas, frozen certification fixtures, the Go evaluator, hard
gates, compatibility checks, and operator experience. A separate `loomarr-models` repository owns
the non-application research toolchain: its pinned Linux/CUDA training image, training/validation
code, reviewed training traces, dataset manifests, adapters, conversion recipes, model cards, and
release manifests. Python, Transformers, TRL, PEFT, bitsandbytes, MLX, and model-conversion tools
therefore never become Loomarr runtime or build dependencies. A model artifact is consumed through
the existing Ollama/OpenAI-compatible boundary exactly like any other model; the training
repository is never imported, executed, or required by this application.

**Training data is a separate consent boundary.** Certification holdout cases never enter the
training or development splits. The first training corpus contains only synthetic Intents,
synthetic identities, frozen catalog fixtures, and human-reviewed target tool traces. Raw household
prompts, viewing or request history, library inventories, Proposal approval/denial notes, databases,
filesystem paths, and provider credentials are not training data. Any future use of real traces
requires a prior design amendment that specifies affirmative consent, minimization and redaction,
access and key custody, region/provider constraints, retention and deletion, incident response,
and human correction. A de-identified derivative is not assumed safe merely
because direct identifiers were removed.

**One release means one certified set of bytes.** The supported end-user artifact is a merged,
quantized GGUF tested through the production Ollama path; a LoRA/QLoRA adapter and unquantized merge
remain reproducibility artifacts, not something a household operator assembles. Training datasets
are versioned in dedicated model-data repositories under Loomarr's namespace,
training source and manifests live in the separate GitHub `loomarr-models` repository, the canonical
model card and weights live in a version-pinned Hugging Face model repository under that same
namespace, and the exact namespaced Ollama package is the convenience install target. Every release
manifest records the base revision, dataset and training manifest, converter and quantizer, GGUF
checksum, Ollama content digest, quantization, evaluation revision,
prompt/tool contract, and minimum Loomarr and Ollama versions.

Loomarr never selects `latest` for a curated release and never silently downloads or swaps
multi-gigabyte weights. A new immutable version is pulled explicitly, verified by digest,
capability-probed, exercised with a bounded real tool-call canary, and only then activated; the
previous model remains available for rollback until the operator removes it. The ordinary AI
profile remains model-less, arbitrary compatible models remain selectable, and hosted/custom
providers remain first-class. Model weights are never embedded in the Loomarr image: application
and model releases have independent size, hardware, update, and rollback lifecycles.
