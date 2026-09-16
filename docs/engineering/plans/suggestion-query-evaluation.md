# Semantic channel-query evaluation

Tracking: [#1276](https://github.com/loomarr/loomarr/issues/1276), extending
[#1015](https://github.com/loomarr/loomarr/issues/1015). Actual-fault recovery stays
with [#1195](https://github.com/loomarr/loomarr/issues/1195).

## Goal and limits

Catch ordinary requests whose meaning changes across phrasing, named lineups,
network eras, dates, exclusions and availability. A new user should not need to
know catalog terminology or discover the particular sentence the app accepts.

Use the existing `Suggester.Suggest`, `Runner.Run` and shared Catalog/reference
fixtures. Do not introduce another harness, application runtime or dependency.
Never relax grounding, membership evidence, audience policy, approval, structural
bounds or existing certification thresholds to improve a score.

This checkpoint delivers the authored development pilot and causal grader tests.
It does not deliver 1,000 independent semantic scenarios, provider qualification,
historical roster completeness, new UI or a completed browser acceptance sweep.
No paid inference or household integrations are needed for the local tests.

## Evidence audit

| Existing evidence | What it establishes | What it does not establish |
| --- | --- | --- |
| `TestReleaseGateRunsMoreThanOneThousandScriptedInputVariants` | 1,120 variants: seven fixed intents × 16 prefixes × ten suffixes, using scripted replies | 1,120 distinct meanings or model understanding |
| Frozen planner certification v16 | Versioned executable expectations and strict qualification thresholds | A new untouched holdout after those cases are exposed |
| Frozen release gate v15 | Bounded production grounding, membership, negative and outcome checks | Exhaustive historical sources or everyday language coverage |
| Isolated History-era preview and parent PR #1275 | One observed provider/browser journey and the focused era fix | Broad provider reliability or user-friendly recovery across all faults |

Code references: `internal/eval/release_gate_test.go`,
`internal/eval/certification_corpus_test.go`, and the immutable manifests under
`internal/eval/testdata/`. Preserve every existing manifest and source/catalog
fixture byte, answer, threshold and production ceiling.

## Method from primary-source investigation

Test capabilities and input behaviors, not merely aggregate accuracy. The
CheckList methodology distinguishes minimum-functionality, invariance and
directional-expectation tests. Apply those distinctions to independently authored
channel requests. [CheckList paper](https://aclanthology.org/2020.acl-main.442/)

Inspect final outcomes separately from plausible transcripts; use deterministic
graders for exact constraints and calibrated human/model assessment for subjective
quality. Repeated, isolated trials help reveal stochastic agent failures.
[Anthropic's agent-evaluation guide](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)

Our engineering inference: sibling requests and shared source families must stay
in one split, otherwise paraphrases leak answers into the holdout. More generated
sentences do not create independently reviewed truth or new capabilities.

## Delivered pilot inventory

`query-pilot-v1.json` has 72 authored requests in 12 scenario groups: 12 minimum
tests, 47 invariance tests and 13 directional tests. Every group has six requests.
This is exposed **development** evidence, not certification or an untouched split.

| Group | Behavior under test |
| --- | --- |
| TGIF membership | Terse/conversational block requests, Sabrina inclusion, explicit exclusion, unrelated title traps |
| History editorial epoch | The original 1990s request, Modern Marvels as an exemplar, later-network and neighboring-network traps, separate explicit episode dates |
| Broad comedy | Genre and episode-airing dates, exclusion of one matching show |
| Movie release era | Genre plus movie-release dates rather than episode dates |
| Matrix franchise | Franchise identity across ordinary phrasing, related-but-not-member trap, Animatrix exclusion |
| MCU Phase One | Named subset rather than every franchise title, an excluded constituent |
| Tom Hanks era | Performer plus movie-release decade, exclusion of a matching film |
| Cozy mysteries | Mood/genre meaning, a darker genre-neighbor trap, requested anchor |
| Apple science fiction | Platform plus genre, platform-only trap, explicit exclusion |
| Explicit title exclusion | The same genre pool with one and then two shows excluded |
| Library ownership | Owned-first choices, outside-library acquisition, library-only contrast |
| Exact named shows | One exact show versus an explicitly requested two-show lineup |

Expected keys, required/forbidden members, minimum distinct breadth, ownership and
date axes are authored separately from the cooperative model scripts. Scripts
use fixed tool inputs and picks, not a copy of the grader's acceptable-key list.
All cases exercise the public production suggestion path with observed calls.

Catalog/reference facts are explicitly synthetic. The existing catalog fixture
is reused, with a digest-bound supplemental catalog and pilot reference fixture.
The History fixture establishes reducer behavior; it is not independently
verified evidence of the full 1990s History Channel roster. TGIF and MCU synthetic
sources exercise membership enforcement, not live reference retrieval.

Scorecard v14 binds the manifest and all fixture digests plus prompt/tool/source
identities. Development always reports `certified: false`; model-selection
comparison rejects development evidence even if its certification flag is forged.

Causal controls compare independent positives with deliberately bad outcomes:
missing Sabrina, wrong TGIF member, unwanted episode cutoff, too few choices,
wrong library/acquisition split, duplicate choices, unclassified padding and an
invented selection rejected through the production grounding path.
The pilot also exposed and protects two production regressions: request verbs
polluting the TGIF source label, and a franchise channel treated as a TV network.

## Reproduce the checkpoint

From its owning worktree, these are network-free local tests, not live certification:

```sh
LOOMARR_EVAL_CONTRACT_ONLY=1 go test -race -tags eval ./internal/eval -count=1
go test -race ./internal/suggest -count=1
```

The full evaluation package retains the frozen manifest/gate preservation tests
and the existing 1,120 scripted variants. Run the repository's final affected
`make verify BASE=<fixed-parent>` once the diff is stable. Protected PR and merge
queue evidence remain required; scripted passes never imply a provider pass.

## Next checkpoints, in order

### First executable expansion batch

Authoring checkpoint [#1281](https://github.com/loomarr/loomarr/issues/1281) adds
`query-expansion-v1.json`: 30 unique authored requests, four minimum, twelve
invariance and fourteen directional tests. All share one exposed TGIF source
family; eight case categories do not create eight independent source families.
Together with the immutable pilot there are 102 authored development requests,
not 1,000 scenarios or 102 independent truth families.

The reviewed partial roster resolves Family Matters (`series:tmdb:2685`),
Step by Step (`series:tmdb:2617`), Boy Meets World (`series:tmdb:1777`) and the
1996 Sabrina sitcom (`series:tmdb:605`). The Philippine Full House adaptation
(`series:tmdb:31183`) is a nearby identity collision, not an American TGIF member.
Full House's original American identity is not yet independently resolved in
this batch; omitting it here does not assert nonmembership or change live lookup.
The [source note](../research/suggestion-query-source-truth.md) records the roles
and limitations. Source reviews, observation dates and uncertainty are pinned
inside the manifest; its digest and separate Catalog/source artifact digests
are attached to every configured scorecard. The reference excerpt is an authored
paraphrase of reviewed facts, not a full retrieved-page snapshot. The manifest
digest binds those exact authored review/excerpt bytes.

The new batch reuses the pilot's validation, production Catalog adapters,
Suggester and Runner, with no replacement evaluator. Library presence and ratings
are simulated: two owned titles and two acquisitions, never household facts.
Cooperative replies have independent literal picks/tool inputs. Exclusions,
required titles and dates are separately authored expectations. A typo request
also supplies a canonical MustInclude anchor; scripted success is not evidence
of real-model spelling correction. The partial pool tests distinct breadth
within four resolved titles, not completeness of the historical TGIF lineup.

Eight causal controls reject a missing Sabrina, wrong member, duplicate padding,
sparse results, wrong ownership, an invented playback limit, an outside-Library
selection and the wrong date axis. Check both per-case deterministic results and
the strict assessment: date-policy accuracy is enforced by the assessment,
whereas grounding, membership and ownership failures are deterministic results.
Every development scorecard remains non-certifying.

The production tests exposed and protect two bounded repairs: source-completed
members cannot override explicit Library-only wording; named blocks described
“as it felt in the 1990s” use editorial date context, while separately requested
airing/release dates and Era still require acknowledgment. Public Suggester tests
also protect negated Library-only wording from incorrectly dropping acquisitions.

Reproduce with the network-free evaluation command above and the focused public
Suggester regression group:

```sh
go test -race ./internal/suggest -run '^TestSuggest_(BlockEpoch|BlockEditorialEpoch|LibraryOnly|NegatedLibraryOnly)'
make lint PKG=./internal/eval
```

Explicit eval-package lint is necessary until the tag-only affected-package gap
in [#1279](https://github.com/loomarr/loomarr/issues/1279) is repaired. Full eval
tests and tagged lint pass; final affected verification and protected CI remain
required. No paid provider trial, sealed holdout or new Playwright preview is
claimed. MCU exact identity/alias mapping and additional History-era membership
sources remain next authoring prerequisites; the 100-family registry is still
the broader backlog, not a completed corpus.

### Remaining expansion checkpoints

The source-review checkpoint is tracked in [#1278](https://github.com/loomarr/loomarr/issues/1278).
Its [evidence note](../research/suggestion-query-source-truth.md) distinguishes
supported claims from missing or questionable metadata. The
[100-family registry](suggestion-query-family-registry.md) covers 20 capability
areas and targets 1,000+ reviewed requests; it is planning evidence, not an
executable expansion or provider pass. Authoring starts with source-reviewed
named-reference families, then deterministic constraints, qualitative/scope
behavior, and ongoing/fault journeys.

1. **Review truth before expansion.** Independently review the pilot's capabilities
   and negatives. Capture actual source URLs, retrieval date, supported membership
   and epoch boundaries for historical/network requests. Record incomplete or
   conflicting evidence explicitly. Do not invent a historical title allowlist.
2. **Build the 1,000+ request development set.** Maintain a registry of independent
   scenario groups and capability coverage, then author meaningful minimum,
   invariance and directional examples. Add typos, ambiguity, audience restrictions,
   regional/platform differences, library gaps, thin/empty pools and refinements.
   Generated variants may supplement reviewed groups but cannot inflate the
   independent-scenario count. Validate fixture membership and immutable digests.
3. **Seal a new holdout.** Assign whole scenario/source families to development or
   protected holdout before tuning. An independent reviewer owns holdout answers;
   exposed groups never become untouched evidence. Register a versioned manifest,
   grader, thresholds and resource envelope without rewriting old certification.
4. **Budget real-model trials.** Obtain explicit authorization for providers,
   models, repetitions, maximum calls and monetary spend. Reuse this factory and
   Runner with isolated fixtures. Bind exact model/provider routes and accounting;
   missing billing or completion evidence means no qualification claim. Compare
   outcome rates by capability as well as total rate. Recovery credit requires an
   encountered fault and successful recovery, under #1195.
5. **Qualify representative user journeys.** Use the isolated Vite preview and
   Playwright for terse TGIF, the original History era, broad mood, exact titles,
   exclusions, missing library media, genuine ambiguity and an actual failure.
   Verify useful choices, understandable recovery, retained picks on refinement,
   manual additions, keyboard/mobile use and no build before approval. Browser
   coverage complements semantic/provider checks; it cannot replace them.
6. **Maintain regressions.** Every confirmed miss gets a minimal authored case,
   independent source/outcome expectations and a relevant negative contrast.
   Rotate newly sealed holdouts after exposure; report counts by capability,
   scenario group, split and evidence type, not one impressive sentence total.
