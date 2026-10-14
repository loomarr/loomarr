# Suggester certification rules (archived)

Archived verbatim from `docs/design.md` §14.1 in #779 (2026-09-27). Not current guidance: the
current rules are in [`docs/design/suggester.md`](../../docs/design/suggester.md#evaluation), and the
evaluator in `internal/eval` is the source of truth.

**Semantic query development is not certification.** The authored query pilot is a
digest-pinned Development Corpus projected onto the existing `Suggester.Suggest` and
`Runner.Run` interfaces, not a replacement evaluator or a fresh release holdout. Each
case has a scenario group, a minimum-functionality/invariance/directional test type,
independent outcome expectations and a named source fixture. Group siblings belong
to one split; exposed development cases cannot be promoted into untouched holdout
evidence. The initial pilot reuses existing synthetic Catalog and public-reference
fixtures and adds explicitly synthetic network-era/ownership controls. These facts
do not certify a complete historical programming roster or a household Library.

Runner scorecard schema v14 adds `developmentCorpus`. A run configured as development
always emits `certified: false`, even when every deterministic assertion and quality
assessment passes; per-case results and the assessment remain useful development
feedback. The pilot binds its corpus, fixture digests and current prompt/tool/source
identities using optional manifest, source and supplemental Catalog digests on the
contract. Model-selection comparison rejects development scorecards even when
their certification flag is incorrectly set. Scripted model replies establish application/grade behavior only, never
real-model understanding. Existing frozen certification/release manifests, answers,
thresholds and production resource ceilings remain unchanged. Deliberately missing
anchors, forbidden matches, unwanted dates, sparse selections and ownership errors
must fail the same outcome assertions that accept the independent positive control.
Provider trials remain explicitly budgeted, opt-in and non-CI. Actual-fault recovery
qualification remains separately owned by #1195; no hypothetical failure earns
recovery credit. The staged plan and coverage inventory live in
`docs/engineering/plans/suggestion-query-evaluation.md`.

The query-expansion diagnostic selects exactly ten reviewed Development Corpus
behaviors: TGIF, the 1990s History editorial epoch, movie release era, mood,
audience ceiling, exclusions, a thin Library result, a valid empty result, and
add/remove refinements. It runs one trial per behavior through the production
Suggester and Runner and remains non-certifying. Its provider-free dry run validates
the exact ordered selection, manifest and fixture identities, a fresh digest-pinned
OpenRouter capability/price/privacy snapshot, one concrete model and ZDR upstream
provider, and all call/token/USD ceilings before any provider is constructed. The
live command is local-only and requires a separate explicit authorization latch.
Fallback and provider data collection remain disabled and required request
parameters remain enforced by the shared strict OpenRouter adapter.

The diagnostic reserves a conservative input bound for every hosted request by
serializing the exact messages and tool schemas, doubling their UTF-8 byte length,
and adding fixed framing headroom. The current ten representative scripted paths
fit a 40,000-token generator input reservation; a later or longer turn that does
not fit is refused before dispatch. The structural maximum is 240 generator calls
plus ten judge allowances, but the independent 2.5-million-token and USD 5 suite
ceilings may stop the run before that structural maximum. The dry-run artifact
states both quantities rather than implying that USD 5 guarantees all 250 calls.
The live JSON retains requested and resolved model/provider attribution, per-call
tokens and charges, corpus/snapshot identity, and failure stage. Its companion
summary maps every result to the predeclared capability; neither artifact retains
prompts, payloads, credentials, or household data.

The expansion family registry is exposed development planning, not executable
case-count or holdout evidence. Promotion requires source/oracle review,
independent negative controls and digest-bound fixture facts; questionable or
missing source fields remain unknown. A current catalog page does not prove
historical broadcast membership. Exact provider identities must be resolved
before regional title aliases become expectations. Registry rows, authored
requests, reviewed cases, scripted outcomes and provider-qualified outcomes are
reported separately. No exposed registry family is promoted into an untouched
holdout, and paid trials retain explicit call and monetary authorization.

Executable expansion batches are new, immutable Development Corpus versions,
loaded through the pilot's shared validation, Catalog adapters and Runner.
The current expansion version is a cumulative snapshot: it retains every prior
authored request and reviewed fixture while adding the next source family. Older
manifest, Catalog and source bytes remain embedded and digest-checked rather than
being rewritten or hidden behind per-batch public loader methods.
Source-review records live in the digest-bound manifest and distinguish provider
identity, historical membership and simulated Library availability. A reviewed
partial roster is not an exhaustive roster: only independently resolved titles
enter its outcome pool. Regional aliases and uncertain first-air dates remain
unqualified until their identities or date roles are independently established.
Every authored request carries strict distinct-title, ownership and date-axis
expectations; independent cooperative replies and deliberately wrong replies
cross the same production Suggester/Runner interfaces. New batches cannot rewrite
frozen pilot/certification facts or count scripted success as model qualification.

Movie-query development cases preserve field authority as part of the fixture
contract: release constraints use `movie_release`; cast, creator and combined
people calls are counted as distinct operations; original language is not origin
country; and either field remains absent when its reviewed source does not state
it. Refinement cases may carry both the free-text change and the current lineup
through the same production `Suggester.Suggest` seam, while required and forbidden
Keys independently grade retained, added and removed choices. A development case
may attach a versioned subjective rubric with directional positive/negative Keys,
but rubric authorship alone is not reviewed evidence. A subjective development
expectation may become `model-attested-development` only through a separate,
immutable authority artifact: two independently executed reviewers from distinct
registered model families receive the same blinded evidence packet and ordinal
rubric without request text, expected polarity, corpus Keys, ownership facts or
one another's answers. Their exact provider route, resolved model, local model
digest or fresh hosted route-capability snapshot digest, prompt version, evidence
and output digests, inference accounting and
per-axis results are recorded. Exact initial agreement is accepted; disagreement
requires an independent third reviewer from a third registered family, and any
missing, malformed, same-family or unresolved result leaves that title and axis
uncertain. The private alias-to-Key map may be applied only after this terminal
decision. An ordinary model-judge result, including one that shares the generator's
provider or model family, is not this authority. Model-attested evidence remains
exposed development evidence: it is neither human review nor certification,
qualification or sealed-holdout evidence. It may project only complete,
non-uncertain threshold decisions into subjective development expectations;
uncertain titles are omitted rather than guessed. Local inference is preferred,
and paid inference still requires an explicit call and monetary budget. Hosted
review records its data-retention posture. A non-ZDR route is allowed only for a
packet containing public evidence and no household, request, ownership, credential
or expected-answer data, with explicit maintainer authorization recorded as a
limitation; the request must still deny provider data collection. Rubric
metadata without that authority is intentionally not projected into
`Case.JudgeRubric` and cannot produce a scripted judge pass. Cross-jurisdiction
or non-board rating facts may exercise synthetic policy plumbing only; they cannot
promote an audience family to reviewed truth. A source-reviewed audience case names
one rating authority, uses only that authority's ordinal scale, and keeps unrated or
other-jurisdiction classifications outside the comparison. An explicitly reviewed
empty pool is a successful abstention only when the case declares that outcome,
requires zero grounded titles, and the production Suggester returns its ordinary
no-grounded-title failure after bounded retrieval; an empty successful Proposal or
a provider/generation fault is not equivalent.

New subjective-review packets do not use the original composite
`attentionalDemand` title score. Frozen `movie-mood-ordinal-v1` evidence remains
loadable and immutable, but no new Development Corpus expectation may depend on
that axis. The successor rubric separates facts that belong to a Title from facts
that belong to one playable Media Source or to the viewer's situation. Its only
title-level structural axis is `narrativeContinuityDependence`: `0` means the
evidence explicitly supports self-contained viewing with no continuing causal
state, `1` a recurring premise with local reorientation, `2` a continuing causal
thread, and `3` an explicitly interdependent, nonlinear or revelatory structure.
That axis requires at least two cited, source-attributed structural facts from
distinct authorities for the Title in the blinded packet: one direct-work source
and one independent structural source. Reviewers are instructed to mark the axis
uncertain without both, and the compiler independently quarantines it regardless
of a submitted number. Pace or scene-transition rate, spoken-text density,
language and subtitle reliance, audio-description availability and visual
dependence are properties of an exact representation or Observation and cannot
be manufactured from a title synopsis. `backgroundFriendliness` is viewer/context
policy rather than a title score and is not an authority axis.

The packet's rubric version owns an exact ordered axis registry. A submission must
contain every registered axis exactly once and no other score; uncertainty is
recorded per axis. Compilation resolves an axis only from exact agreement between
the two initial registered families or exact two-of-three agreement after an
independent third-family adjudication. Disagreement, an explicit reviewer
uncertainty or insufficient evidence quarantines only that axis. Corpus loading
rejects a subjective rule that names an axis outside its bound authority or uses
an uncertain decision, while allowing other resolved axes from the same Title.
Release year, country, language, rating authority, genre, credit role, collection
membership and other Catalog facts remain deterministic constraints evaluated
independently of the subjective authority. Expanded calibration must span multiple
decades, genres, regions and tonal directions and must exercise complete Intents
through the existing Runner; title scores alone are not outcome evidence.

Explicit Library-only wording is a hard suggestion qualifier, including “no
shows added from outside my library”; source-pool completion cannot restore
outside-Library picks as selections or alternates. A named block described “as
it felt in the 1990s” supplies editorial reference context, not a playback-date
limit. Explicit episode-airing requests and a separately submitted Era still
require anchored date interpretation; neither qualifier weakens grounding.

Request-role extraction must distinguish a movie franchise channel from a
television network reference. A request verb such as “Recreate” is not part of
a named lineup's label; possessive network and daypart context must not turn
one explicitly named block into conflicting labels. These corrections retain
the existing membership-evidence requirements and source protocol.

**Semantic certification is explicit, inference-spending, and behavioral through the final
schedule.** `make eval` remains an exploratory run that may skip when no live library, catalog, or
model is configured. `make eval-cert` is the release/operator assertion: missing configuration, an
unexecuted case, a failed deterministic gate, a schedule-materialization failure, or a judge score
below its declared floor fails the command. Its versioned corpus contains the exact
four starter-template Intents plus named-title, thematic, holiday, and adversarial grounding cases.
Holiday cases span family and adult seasonal requests, assert both exclusive mode and the exact
holiday subset, and
require outside-Library proposals where acquisition is allowed. The scorecard reports relevance and
serendipity separately: relevance rewards qualifier fidelity, while serendipity rewards coherent
less-obvious additions without treating randomness as novelty. The corpus uses the real production suggester and
catalog path. Exact user constraints are code-owned predicates: a named include/exclude, media mix,
or expected episode cannot be certified by a merely non-empty Proposal or by judge prose. A case's
`RequireTitles` and `ForbidTitles` use case-insensitive, trimmed, internal-whitespace-normalized whole
title equality over its explicitly declared `grounded` Proposal or `scheduled` materialization
evidence; substrings and near titles never satisfy an exact assertion. `ForbidTitleTerms` remains a
separately named heuristic only for cases that intentionally forbid a term family. Cases that
make a programming promise are materialized through `schedule.ComputeDesiredAt` with an explicit
clock/history and assert the concrete program identities/order after expansion, filtering, grouping,
and placement. The evaluator therefore has three nested contracts: Intent to grounded Proposal,
Proposal plus episode evidence to editorial selection, and selected pool plus Policy to desired
schedule. The same public Runner interface owns all three so exploratory and certification modes
cannot silently test different behavior.

The durable schedule-outcome contract is distinct from the proposal corpus because only an
already-owned lineup can be materialized. Hermetic Runner tests use explicitly synthetic fixture
episodes to prove that curated-series inclusions/exclusions, in-Library holiday selection, and an
atomic release-ordered movie franchise can each pass and fail. Those fixture identities are test
evidence only and never become live certification expectations.

Live schedule cases instead come from a declared real-Library evidence snapshot. Its closed,
versioned schema records a safe non-empty snapshot id; the exact series Key, Library item id,
complete episode evidence, and independently pinned included/excluded concrete identities for
curated and holiday selection; and the exact owned movie Keys, Library item ids, runtimes, TMDB
collection id, and independently pinned canonical franchise sequence. Before any generator
or judge provider is constructed, Loomarr re-reads the declared titles through
`library.ItemMetadataByID` and `library.ListEpisodes`, resolves every movie through TMDB collection
identity, and requires an exact match on every scheduling-relevant field. Missing cases, sparse
curation evidence without nonempty, disjoint include/exclude sets covering every present episode,
stale episode metadata, unplayable movies, mixed collections, a noncanonical franchise sequence, or
any other drift fail certification. Preflight never invokes `schedule.ComputeDesiredAt`: the Runner
compares its later production-scheduler output against those pinned independent expectations. The
snapshot id is appended to the scorecard corpus version.
Both live series cases are pinned to the authoritative `series:tmdb:456` Key; a self-consistent
snapshot substituting any other series identity fails before Library, TMDB, or provider work begins.
Their viewer requests remain semantic inputs to the production generator rather than snapshot
labels: curated requests are exactly `Classic Simpsons reruns from the golden era, curated for
variety`, and holiday requests are exactly `Christmas episodes of The Simpsons already in my
library`. The snapshot supplies independent owned-title and episode evidence; it never replaces the
viewer intent that generation and materialized outcome assertions must satisfy.
Before accepting episode or runtime evidence, one shared ownership check calls
`library.LookupDetail` with each exact TMDB media type/id. The title must be present and the returned
Library item id must equal the snapshot's `libraryItemId`; matching metadata from an unrelated owned
item cannot satisfy the evidence contract.

Fixture and live `ScheduleMaterializer` adapters call one shared pure projection through
`schedule.ComputeDesiredAt`. The live adapter bulk-reads owned movie runtime from
`library.ItemMetadataByID`, enumerates series through `library.ListEpisodes`, and resolves a movie's
authoritative TMDB collection identity before entering that projection. A snapshot-bound live
adapter also rejects a Proposal whose Key resolves to a different Library item id.
The snapshot binding is case-specific and exact: curated and holiday materialization accept only
their declared series Key, while franchise materialization accepts only TMDB movie Keys 85, 87, and
89. Any additional playable Lineup Key fails at the schedule stage; missing members remain the
deterministic `RequireKeys` contract. Acquisitions never enter schedule materialization.

Live schedule materialization is an explicit `LOOMARR_EVAL_LIVE_SCHEDULE=1` opt-in and requires
`LOOMARR_EVAL_SCHEDULE_EVIDENCE` to name the versioned JSON snapshot above. An exploratory
run without it runs the proposal corpus, omits the entire schedule-outcome corpus, and reports that
omission once. Certification mode fails before constructing or calling any provider when the opt-in
or a valid, consistent snapshot is absent, so a required scorecard cannot silently omit viewer
outcomes. The hermetic contract lane always uses fixed fixture evidence and never constructs live
Library, TMDB, generator, or judge adapters.

The subjective judge receives one typed, bounded evidence value from `Runner`; it does not receive
the raw Proposal or rediscover a schedule. That value keeps lineup and acquisition titles in
separate ownership sets and records each title's exact grounded key, name/year, source provenance,
optional rationale/confidence, genres, and rating. It also carries the suggester-extracted
`ProposalPolicy`, the trial's structural counts and grounding stage, and the ordered concrete program
identities already returned by `ScheduleMaterializer`. Scheduled episode entries additionally retain
the bounded grounded title, season/episode range, year, rating, community rating, overview, and tags
that the scheduler actually materialized. That is the minimum evidence with which a judge can assess
holiday or highlight intent; an opaque episode identity is not sufficient. The model prompt renders
those facts directly, including a deterministic prefix of the materialized programs; proposal title
names are not a substitute for scheduled evidence. A Proposal item whose canonical Key cannot be
constructed fails the judge stage before any prompt is sent; it never becomes an empty grounded key.
`JudgeMaxTitlesPerOwnership`, `JudgeMaxGenresPerItem`, `JudgeMaxPolicyValues`,
`JudgeMaxScheduledPrograms`, `JudgeMaxEpisodeTags`, and `JudgeMaxTextRunes` are the public bounds on
the two title sets, each title's genres, every open policy collection, the schedule sample, episode
tags, and each free-text field respectively. The evidence type has no credential, endpoint,
provider-request, or provider-response fields, so those values cannot enter the prompt through this
seam.

It writes a machine-readable scorecard naming the schema version, corpus version, independent
requested generator and judge provider/model identities (never credentials), trial configuration,
every case/trial outcome, and the aggregate certification result. The scorecard has no ambiguous
top-level provider/model compatibility fields: generator and judge identity are distinct even when
both roles happen to use the same route. Every non-empty judge rubric makes a successful judge stage
mandatory in exploratory and certification runs alike, regardless of whether any score floor is
declared. A missing judge, provider error, or unparseable response is an ordinary stage error and
fails that trial; it is never encoded as a magic score. A valid score remains in the closed `0..1`
range, including zero, and fails only the declared overall/relevance/serendipity floor. Successful
judge evidence must explicitly contain all three finite scores within that range plus the prompt's
non-blank reason. `Runner.Run` validates that contract independently of the configured `Judge`, so a
custom implementation cannot certify NaN, infinity, an out-of-range value, or a blank reason;
missing, null, or invalid evidence is a judge error, never defaulted or clamped. Schema v12
records exactly one first-failure stage on every failed trial from the closed vocabulary `retrieval`,
`generation`, `deterministic`, `structural_budget`, `schedule`, `judge`, and `budget_exhausted`;
later failures remain visible but never replace the first stage. `no_tool_call` and
`retrieval_empty` are retrieval failures; provider/model errors, malformed output, and an empty
selection after surfaced candidates are generation failures. The scorecard aggregates failed-trial
counts under that same vocabulary, including zero counts, while passed trials carry no failure
stage. Stochastic cases run serially for
an explicit bounded number of trials; the scorecard reports pass rate and min/median/max overall
quality, relevance, and serendipity
rather than hiding variance in one point score. Per-case tool-call and surfaced-candidate budgets are
hard failures, so an over-broad retrieval cannot masquerade as quality. The hermetic lane
uses fixed candidates and episode evidence and remains in the normal gate; real catalog/provider
certification stays manual and inference-spending.

Before constructing Library, TMDB, generator, or judge clients, every semantic run reports one
deterministic worst-case call budget: case count, trial count, maximum generator calls, maximum judge
calls, and their total. Generator calls are `cases × trials ×` the production Suggester's exported
structural bound; judge calls are at most `cases × trials`. Required certification additionally
requires positive `LOOMARR_EVAL_MAX_CALLS_PER_RUN` and `LOOMARR_EVAL_MAX_CALLS_PER_SUITE`
declarations. The per-run ceiling must cover the production generator bound plus one judge call, and
the suite ceiling must cover the complete computed total and be at least the per-run ceiling.
Missing, malformed, overflowing, or smaller declarations fail before any client or provider work;
the former single `LOOMARR_EVAL_MAX_CALLS` name is not accepted. Exploratory evaluation reports the
same budget without requiring ceilings. Required mode defaults an absent `LOOMARR_EVAL_TRIALS` to three,
but rejects an explicit malformed, zero, negative, or integer-overflowing value instead of silently
substituting that default. Every case/trial/call multiplication and total addition is checked; an
unrepresentable envelope returns an explicit overflow error with no wrapped budget values before
clients. The same exported Suggester bound supplies every real proposal
and live-schedule case's nonzero tool-call and candidate-surfacing budgets, so certification cannot
drift from the production loop by copying private magic numbers. The durable proposal corpus includes
an exact forbidden grounded Key, an explicit closed movie-only media-type gate, and an intent-backed
minimum genre-diversity gate; these predicates are not merely test fixtures. A closed media-type gate
rejects every grounded title outside its declared set.

Required certification also declares positive `LOOMARR_EVAL_MAX_TOKENS_PER_RUN`,
`LOOMARR_EVAL_MAX_SPEND_PER_RUN`, `LOOMARR_EVAL_MAX_TOKENS`, and
`LOOMARR_EVAL_MAX_SPEND` ceilings before clients. A run is one case trial and the suite is the complete
`Runner.Run` execution. Token ceilings count provider-reported prompt plus completion totals without
double-counting their reasoning/cache/modality detail categories; spend ceilings are exact
nonnegative decimal USD values and are accumulated without binary floating point.

Those limits are **pre-dispatch ceilings**, not merely thresholds checked after a billable response.
Before the first provider client exists, each OpenRouter role binds a fresh, digest-pinned immutable
route-resource snapshot: the exact requested model, ordered upstream provider family, active ZDR
endpoints, request input allowance, request completion-token cap, supported parameters, and exact
prompt/completion/internal-reasoning prices. Every generator turn carries the production 2,048-token
completion cap and every judge call carries its own explicit bounded completion cap; a provider
default is never accepted in a resource-bounded run. Because OpenRouter routing names the provider
family rather than one endpoint slug, the reservation uses the most expensive eligible active ZDR
endpoint in that family, never the cheapest observed endpoint. A higher reasoning-token price than
the shared estimator can bound fails closed.

Immediately before each individual generator or judge call, the shared ledger checks the actual
serialized messages and tool schema against the snapshot-backed input allowance, using twice the
UTF-8 byte length plus fixed chat-framing headroom as the conservative token bound, and checks the
actual completion limit against the role cap. It then temporarily reserves that input allowance plus
the completion cap and its worst-route USD price. `LOOMARR_EVAL_OPENROUTER_SNAPSHOT`,
`LOOMARR_EVAL_OPENROUTER_SNAPSHOT_SHA256`, `LOOMARR_EVAL_GENERATOR_MAX_INPUT_TOKENS`, and
`LOOMARR_EVAL_JUDGE_MAX_INPUT_TOKENS` are required for an OpenRouter run; caller-entered token or
spend reservations cannot replace the derived values. Other hosted adapters without a capability
snapshot retain explicit conservative reservation declarations. If either the run or suite would
exceed its declared limit with the reservation in place, or the exact request exceeds its role
allowance, the request is never dispatched. On return, the
reservation is atomically replaced by the smaller provider-reported actual usage and exact charge.
Thus exact-limit completion is allowed while no in-flight response can carry the run beyond the
declared authorization. OpenRouter key or workspace limits remain useful defense in depth, but they
are not proof of Loomarr's cap because an already-dispatched request may complete after a provider
budget is crossed.

Runner applies that reservation with overflow-safe arithmetic to every generator provider call and
every judge call, including generator repair/tool-loop calls inside one `Suggest`. Once a per-run
ceiling cannot admit the next reservation it records `budget_exhausted` and skips that run's judge or
later call while a later run may use its own allowance; once the suite ceiling cannot admit it no
subsequent run starts. Missing usage is sticky per provider call: reported
usage from one call can never conceal missing required tokens or hosted spend on another call. With
a corresponding hard ceiling, one hosted call whose token or spend usage is missing makes the
shared suite ledger permanently uncertain for that `Runner.Run`: no judge, later trial, or later
case may start another provider call. The trial that discovers the missing fact records the
uncertainty as a failure, and subsequent trials fail closed before generation. The scorecard retains
declared limits and observed totals. This is an execution gate, not post-hoc cost reporting;
provider attribution that is absent remains explicitly unknown rather than being invented to make
the ledger appear complete. Missing token or hosted-spend attribution is therefore
`budget_exhausted` because the ceiling cannot be proven; a local Ollama call remains explicitly
non-billed without fabricating a provider charge. If the same generator call already failed at
retrieval or generation, that earlier diagnosis remains the trial's first-failure stage while the
resource uncertainty still latches the suite and blocks subsequent calls. Exploratory runs may omit
hard token/spend ceilings but still report observed bounded attribution.

Required certification whose generator or judge wire is local/default Ollama also requires
`LOOMARR_EVAL_ALLOW_LOCAL=1` before any Library, TMDB, generator, or judge client is built. A hosted
fully hosted run does not require that local-resource acknowledgement. No target provisions or starts
Ollama.

Each provider call's reported attribution is projected into bounded, scrubbed scorecard evidence for
its own role. Generator and judge calls remain separate and retain requested and resolved
provider/model, every provider-neutral token category, exact decimal charge and currency when
reported, attempts, and latency. A reported charge is accepted only when its amount is a nonnegative
plain decimal of at most 64 runes and its currency is a three-letter uppercase ISO-style code. The
scorecard records `reported`, `missing`, or `invalid` charge status; invalid provider text is discarded
rather than truncated, copied, or inferred, while a valid decimal string is retained exactly. The
projection has no credentials, endpoints, prompts, response
payloads, or generation ids. Missing wire resolution or attempt metadata remains explicitly empty or
zero: requested identity is never copied into resolved identity, and absence never becomes one
attempt. Missing attribution remains an explicit all-zero/all-empty call record; the evaluator never
infers routing, usage, attempts, latency, or cost from configuration. Generator
evidence is capped by the same exported production call bound and judge evidence by one call per
trial, so provider output cannot make a scorecard unbounded. The uncapped observed generator-call
count remains separate from that serialized prefix. Exceeding the production model-call bound fails
the trial at `structural_budget`; truncation can never hide an overrun behind an apparently complete
scorecard.
Network and inference keep it outside comprehensive verification; a stored scorecard is evidence for one named
model/catalog snapshot, not a timeless claim that every provider is certified.

**Discovery feedback is explicit household editorial state, never inferred viewing behavior.**
An admin may record `keep`, `less`, `never`, or `surprise` against a grounded title identity at
household scope or for one channel. Each change appends an actor-attributed event; clearing a signal
appends a tombstone rather than rewriting history. Members may read the effective signals but cannot
mutate shared taste. Anonymous callers cannot read or write them.
The latest event per `(scope, target)` is the effective signal, with channel scope overriding the
household signal for that channel.
`GET /v1/discovery/feedback?scope=household` returns the effective household view. The same read at
channel scope returns the effective view *for that Channel*: channel events win, while unaffected or
cleared targets fall back to their household event. Each row retains the scope and actor of the event
that currently supplies it, so a client can explain whether the choice is Channel-specific or
inherited. Replacing an action appends the replacement event; undo appends `clear`. Undoing a Channel
override therefore reveals any household fallback rather than falsely presenting no preference.
Feedback targets are canonical provisioning keys accepted by `provision.ParseKey`; prefix-like or
otherwise malformed identities are rejected before persistence.

Admin proposal and Channel-lineup controls render the effective action as selected, explain its
scope, and offer Undo. Choosing another action replaces the effective action through the same
append-only endpoint. The controls never imply that feedback edits the current Proposal or playout:
the change applies to later fresh suggestion or re-curation ranking only.

A channel-scoped event names a currently persisted Channel identity. The store checks that identity
and appends the event or clear tombstone in one transaction; an absent identity returns
`store.ErrNotFound`, the admin API maps it to the bounded Channel-not-found 404, and no feedback row
is written. Authorization runs before this existence disclosure, so a member still receives 403.
Detached Channels remain persisted identities and therefore retain and accept their explicit
feedback. Hard purge is the identity boundary: deleting a Channel removes only that Channel's scoped
feedback in the same transaction, while household feedback remains independent. The forward schema
migration removes only legacy channel-scoped rows whose Channel identity is already absent; active
and detached Channel feedback and every household event survive. No detach, purge, playback,
approval, denial, or inactivity infers a feedback event.

Execution scope is server-derived and deliberately absent from persisted Intent JSON and public
proposal inputs. A durable `kind=recurate` Proposal Job resolves its one current owning Channel from
the trusted claimed Job id immediately before the worker invokes the Suggester, using the Channel's
unique non-empty `intent_ref`; only then does the worker attach the Channel id as in-memory discovery
scope. A missing, detached, unreadable, empty-id, or mismatched owner fails that Attempt
before provider or catalog access. Fresh suggestions and manual refines remain household-scoped.
The admin feedback API's `scopeId` identifies the editorial event being recorded; it never supplies
or overrides execution scope. This keeps channel feedback effective across durable requeue, claim,
and process restart without making a client-writable scope an authorization fact.

A single pure deterministic discovery ranker consumes grounded candidate metadata plus effective
signals. Explicit includes/excludes, grounding, audience safety, approval, and quotas remain outside
and above it. `never` is a hard candidate exclusion; `keep` protects an existing lineup item from
automatic retirement; and `less` is a bounded exact-title demotion that also demotes a same-genre
candidate when that relationship is present in the grounded batch. `surprise` never boosts the
marked title. Instead, the presence of an effective `surprise` signal enables a deterministic
diversity pass inside each unchanged relevance/preference band. The pass seeds its covered genres
from surprised targets present in the grounded batch, then gives each next candidate at most one
new-genre point in addition to the existing outside-Library novelty point;
canonical provisioning identity remains the final tie-break. A missing target or missing genre
metadata creates no inferred relationship. Relevance and preference bands therefore remain hard
floors while a grounded batch becomes more varied. Ranking affects only a later fresh proposal or
re-curation proposal. It never
edits current playout, and no playback/broadcast history is converted into taste.
`make eval-matrix` runs that unchanged corpus twice—once with the configured local generator and once
through OpenRouter's OpenAI-compatible endpoint—and writes separate named scorecards. Both generator
and judge use the branded `openrouter` adapter, not the generic `openai` identity. Each OpenRouter role
requires an exact immutable namespaced model slug and exactly one concrete upstream provider before
any external client is constructed. Router aliases, `latest` aliases, wildcard/automatic providers,
and comma-separated fallback orders are rejected. Every certification request sends that singleton
order with fallbacks disabled, parameter support required,
data collection denied, and zero-data-retention requested. Missing or malformed pinned routing fails
required certification at preflight. Branded OpenRouter certification additionally requires both
generator and judge base URLs to equal the canonical `https://openrouter.ai/api/v1`; a blank or
alternate endpoint fails before client construction. Generic/custom OpenAI-compatible providers
remain configurable outside this branded lane. Ordinary production fallback/resilience behavior is
not part of this certification lane. The selected resolved provider/model and provider-reported exact cost remain
scorecard evidence. Generator and judge models, routes, and identities are independent configuration
inputs even when the judge model falls back to the generator model; a judge-specific provider never
inherits the generator's scorecard identity. Because local
inference competes directly with playback, transcode, memory, and GPU capacity on an appliance,
the target refuses to start unless the operator explicitly sets `LOOMARR_EVAL_ALLOW_LOCAL=1` after
confirming the host is idle and has enough headroom. The target never starts, pulls, or configures a
model runtime itself, and its two generator legs run sequentially. The local leg may use an
OpenRouter model as an independent judge. A matrix is required evidence for discovery
tuning so behavior is not optimized around one local model's tool-call quirks; provider/model and
judge choices remain explicit run inputs, and no credential enters either artifact.
An expectation that only needs real grounded content counts owned lineup entries and outside-Library
acquisitions together; it asserts a particular ownership side only when that distinction is part of
the behavior under test. Each case also records only structural grounding diagnostics (title/genre/keyword call counts and the
number of candidates returned, never prompts or catalog text). An empty proposal is classified as
`no_tool_call`, `retrieval_empty`, `selection_empty`, `generation_error`, or `provider_error`; retrieval, provider
availability, and curation can therefore be tuned
independently instead of treating every zero-lineup result as a model-quality mystery.
