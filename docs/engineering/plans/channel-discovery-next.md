# Channel discovery: trustworthy evaluation, episode curation, and household taste

- Status: investigated and filed; public test seams awaiting maintainer confirmation
- Base: `origin/main` at `510d9f7b`
- Delivery: three dependency-ordered issues and implementation slices
- Research: [`channel-discovery-quality-2026-08-23.md`](../research/channel-discovery-quality-2026-08-23.md)
- Top-three delivery: [#491](https://github.com/loomarr/loomarr/issues/491) ->
  [#492](https://github.com/loomarr/loomarr/issues/492) ->
  [#493](https://github.com/loomarr/loomarr/issues/493)

## Complete improvement backlog

| Issue | Improvement | Dependency role |
| --- | --- | --- |
| [#491](https://github.com/loomarr/loomarr/issues/491) | Schedule-level behavioral certification | First: proves every later quality claim |
| [#492](https://github.com/loomarr/loomarr/issues/492) | Individual classic, highlight, and holiday episode curation | Second: fixes the visible series-level gap |
| [#493](https://github.com/loomarr/loomarr/issues/493) | Explicit household feedback and deterministic reranking | Third: closes the taste loop |
| [#494](https://github.com/loomarr/loomarr/issues/494) | Relevance-preserving, metadata-rich grounded retrieval | Supplies stronger evidence to selectors/rankers |
| [#495](https://github.com/loomarr/loomarr/issues/495) | Deterministic daypart/holiday and opt-in weather context | Supplies provenance-bearing ambient context |
| [#496](https://github.com/loomarr/loomarr/issues/496) | Recommendation reasons and bounded decision traces | Makes decisions inspectable and evaluable |
| [#497](https://github.com/loomarr/loomarr/issues/497) | Exposure memory and cross-Proposal novelty | Prevents repeated plausible-but-stale results |
| [#498](https://github.com/loomarr/loomarr/issues/498) | Privacy-safe production quality measurement | Compares offline claims with delivery outcomes |

The issue boundaries follow ownership: retrieval evidence, ambient context, explanation, exposure,
and quality measurement can each evolve without changing the ranking interface. Implementation
checkboxes remain inside their owning issue rather than becoming backlog noise.

## Outcome

Channel discovery should be accurate enough to trust, surprising for defensible reasons, and
visibly curated at the programme level. A request such as “classic Simpsons” must produce a
deliberately selected episode deck rather than merely shuffling every episode in a season window.
Movie collections keep their existing, distinct promise: the collection is placed as an atomic
block in release order even when the surrounding channel is shuffled. New and existing channels
must also be able to learn explicit household taste without making approval, grounding, audience
safety, or deterministic scheduling depend on an LLM.

## Current evidence

The merged discovery pass is a strong retrieval baseline, not a complete recommendation system:

- the versioned 16-case corpus runs one generation per case against one configured provider and
  one library snapshot;
- exact named requirements can pass on a non-empty result because the hard gate has no expected
  key/title assertion;
- the judge sees title, year, and ownership, but not candidate provenance, metadata, policy
  effects, episode selection, or the final schedule;
- a curated single-series request is converted to a `syndication` deck and may receive a season
  window, but every eligible episode remains in the pool;
- episode metadata currently carries identity, runtime, season/episode, year, and rating only;
  it has no quality or thematic signals with which to select “best” or holiday episodes;
- catalogue discovery preserves a bounded upstream pool, then alphabetizes the owned and outside
  partitions before truncation, discarding useful relevance order inside each partition;
- denials, manual lineup edits, broadcasts, and playback do not form a household preference
  model. Airing history intentionally answers only “what did Loomarr broadcast?”;
- daypart and holiday rules are persisted when the request names them, but no ambient context is
  silently injected into a request. That remains the correct default: context should be explicit,
  inspectable, and user-controllable.

## Slice 1 — make semantic evaluation trustworthy

Extend the eval corpus and scorecard so a passing certification means the generated channel, not
just a plausible proposal, satisfied the request.

- Add exact required and forbidden grounded identities, expected media mix, minimum diversity,
  and bounded candidate/tool-cost assertions.
- Materialize representative proposals through the public scheduling seam and inspect concrete
  slots, including episode order/selection and the movie-collection release-order floor.
- Support repeated trials and provider/model profiles; report pass rate, worst case, dispersion,
  retrieval failures, generation failures, and judge failures separately.
- Pin the requested OpenRouter model and provider in the certification lane and record resolved
  router metadata; exercise ordinary routing/fallbacks only in a separate resilience lane.
- Give the judge grounded metadata, provenance, policy, and scheduled programme samples while
  keeping hard invariants deterministic.
- Keep certification opt-in and hosted-provider friendly. Never start Ollama automatically; make
  concurrency, trial count, and estimated call budget explicit before a run.

Exit: hermetic contract tests prove every new assertion can fail; a scorecard cannot certify a
named-title, curated-series, holiday, or franchise case without checking its concrete outcome.

## Slice 2 — curate individual episodes

Deepen the existing schedule module rather than creating a second scheduler.

- Extend the library episode projection and cached `schedule.ResolvedProgram` with the minimum
  stable signals needed for curation: community rating/vote confidence and thematic text/tags.
- Add a proposal-owned episode-selection policy to a series lineup entry. Modes must distinguish
  the default complete catalogue, highlights/classics, and holiday/thematic selection.
- Derive conservative episode intent deterministically from the grounded request and persist it
  through proposal → approval → lineup. Unknown signals degrade to the ordinary complete deck;
  they never cause dead air.
- Apply episode selection inside `schedule.ComputeDesiredAt`, after the never-relaxed audience and
  scope gates and before seeded ordering/windowing. The selector is pure and deterministic.
- Preserve multi-part adjacency, chronological requests, and the movie-franchise atomic
  release-order floor.

Exit: “classic/best Simpsons” selects and mixes a defensible episode subset; an explicit binge
stays chronological; a holiday request selects matching episodes; sparse metadata degrades safely;
and movie collections remain together in release order.

## Slice 3 — household feedback and deterministic reranking

Add an explicit shared-channel taste loop with four legible actions: keep, less like this, never,
and surprise me.

- Persist append-safe preference signals with actor, scope, target identity, reason/action, and
  timestamps on both SQLite and Postgres. `never` is a hard exclusion; the other actions are
  ranking signals. Authorization follows the existing member/admin channel-edit contract.
- Expose feedback next to grounded proposal and channel-lineup items, with a read surface that
  explains the effective household signal.
- Introduce one deterministic ranking function over grounded candidates. Relevance remains the
  first objective; availability, metadata quality, household affinity, novelty, and diversity
  contribute bounded, inspectable terms. “Surprise me” increases the novelty/diversity weight but
  never weakens grounding, audience safety, must-include, must-exclude, or approval.
- Feed the ranker into fresh suggestions and re-curation. A signal changes later proposals; it
  never mutates a live lineup behind the approval gate.
- Treat inferred playback behaviour as future work. V1 learns only from explicit controls so a
  shared television does not guess whose taste a passive airing represents.

Exit: identical candidates and signals produce identical ranked output; `never` cannot surface;
“keep” protects a title from automatic retirement; “less” demotes related candidates; “surprise”
widens novelty without admitting irrelevant items; all acquisitions still require the existing
approval/quota path.

## Proposed public test seams

The TDD workflow requires these seams to be confirmed before the first red test:

1. **Evaluation seam:** an eval `Runner` accepts a `suggest.Suggester`-compatible generator plus a
   schedule materializer and returns a versioned scorecard. Tests assert scorecard behaviour, not
   helper functions or prompt strings.
2. **Episode seam:** `schedule.ComputeDesiredAt` remains the public behavioural boundary. Adapter
   tests separately pin `library.ListEpisodes` metadata projection and proposal-to-lineup
   persistence.
3. **Feedback seam:** authenticated feedback HTTP routes plus one pure discovery ranker are the
   public boundaries. Store behaviour remains in the shared SQLite/Postgres conformance suite.

These seams keep the LLM as a grounded candidate proposer, the scheduler as the sole programme
placer, and the approval transaction as the sole path from recommendation to acquisition/channel
mutation.

## Resource and delivery contract

- No local Ollama or inference is started by tests or development commands.
- Run focused hermetic tests sequentially with `GOMAXPROCS=2` while editing; run one complete
  resource-capped required gate after stabilization.
- Do not run `make smoke*` from the agent session.
- Amend `docs/design.md` and `docs/programming-design.md` before each behavioural implementation.
- Deliver the slices as reviewable commits/PRs in dependency order; generated OpenAPI/frontend
  clients are changed only through their generators.
