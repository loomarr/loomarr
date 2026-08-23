# Channel discovery quality: evaluation, episode curation, and household feedback

**Compiled 2026-08-23.** Scope: the next three improvements to Loomarr's grounded channel
discovery pipeline, with an emphasis on accurate-but-serendipitous results and predictable behavior
on resource-constrained home servers.

## Executive conclusion

The next step is not a larger prompt or a larger local model. Loomarr should keep the LLM at the
boundary it already documents—extract intent and choose only from grounded candidates—then add
three deterministic layers around it:

1. **A behavioral certification harness that scores the final scheduled programs, not only the
   proposal.** Run exact invariants on every case, repeat the stochastic cases, and compare pinned
   generator/judge configurations through OpenRouter.
2. **An episode selector before the existing ordering engine.** “Classic,” “best,” and holiday
   requests must choose individual episodes using cached episode metadata; only then should
   `sequential`, `shuffle`, or `syndication` order the selected pool. Movie-franchise atomicity stays
   a separate hard constraint.
3. **A small, inspectable household preference model and deterministic reranker.** Explicit
   `keep`, `less like this`, `never`, and `surprise me` signals should adjust grounded candidates
   through relevance, preference, availability, quality, novelty, and diversity terms. No training
   service, vector database, or always-on local inference is needed.

This preserves Loomarr's strongest property: the model can search and interpret, but deterministic
code owns identity, safety, approval, eligibility, and scheduling.

## What the repository has today

The current design already establishes the right macro-boundary:

- [`docs/programming-design.md`](../../programming-design.md) says the LLM extracts a structured
  `ChannelPolicy`, while deterministic scheduling enforces it.
- [`internal/eval`](../../../internal/eval) exercises the real suggester, real catalog, real hosted
  or local provider, hard assertions, and an optional LLM judge.
- [`internal/suggest/suggester.go`](../../../internal/suggest/suggester.go) makes catalog tool calls,
  bounds surfaced candidates, and revalidates every selected title at the grounding chokepoint.
- [`internal/library/episodes.go`](../../../internal/library/episodes.go) currently caches only the
  playable episode identity, title, duration, season/episode numbers, year, multipart marker, and
  content rating.
- [`internal/schedule/separation.go`](../../../internal/schedule/separation.go) turns an already
  eligible episode pool into a deterministic no-repeat syndication deck. It intentionally does not
  decide which episodes are “best” or “classic.”
- [`docs/programming-design.md`](../../programming-design.md) keeps multipart episodes and films in
  the same TMDB collection as atomic, internally ordered groups. That is an ordering floor, not an
  editorial score.

Three measured gaps follow from those boundaries:

1. A corpus case can demand “The Matrix” while the hard gate checks only that the lineup is nonempty;
   exact inclusion is left to the judge rubric. Likewise, the classic-series case asserts
   `ordering=syndication`, not which episodes reach the schedule.
2. The judge sees title, year, and library presence, but not the final episode sequence, policy
   exclusions, acquisition state, or why each deterministic score was assigned.
3. The episode cache has too little information to distinguish a celebrated episode, a holiday
   episode, a generic episode in the right season range, and an unavailable episode.

## 1. Make evaluation trustworthy

### Research findings

RecList argues that a single aggregate ranking metric misses nuanced deployment behavior and
proposes use-case-specific behavioral tests over black-box recommenders. The key lesson for Loomarr
is to test named capabilities and slices—grounding, safety, must-includes, catalog exploration,
episode curation, holiday behavior, and schedule texture—not just a single average judge score.
([RecList paper](https://arxiv.org/abs/2111.09963))

LLM judges are useful but are not stable truth. Controlled studies find position bias in
LLM-as-a-judge decisions, so subjective scores should be secondary evidence, order-balanced where
comparison is involved, and never replace exact program invariants.
([position-bias study](https://aclanthology.org/2025.ijcnlp-long.18/))

OpenRouter's default path may load-balance across providers and use fallbacks. Its documentation
also states that “latest” aliases can change and recommends concrete model slugs for reproducible
regression tests. Therefore a scorecard must record both model and resolved provider, and the
certification lane should pin them; a separate resilience lane may exercise normal routing and
fallbacks.
([provider routing](https://openrouter.ai/docs/guides/routing/provider-selection),
[version pinning](https://openrouter.ai/docs/guides/routing/routers/latest-resolution))

OpenRouter can expose the selected endpoint and all routing attempts through opt-in router metadata,
so provider attribution need not be inferred from the requested model name.
([router metadata](https://openrouter.ai/docs/guides/features/router-metadata))

Accuracy and diversity are competing list-level objectives, and neither alone describes a useful
recommendation set. They should be reported independently rather than hidden inside one opaque
overall number.
([Parapar and Radlinski, RecSys 2021](https://research.google/pubs/towards-unified-metrics-for-accuracy-and-diversity-for-recommender-systems/))

### Design implications for Loomarr

Treat evaluation as three nested contracts:

| Layer | Input → output | Required evidence |
|---|---|---|
| Retrieval/generation | intent → grounded proposal | exact IDs, must-include and must-exclude predicates, library/outside mix, policy extraction, tool-call and candidate budgets |
| Editorial selection | proposal + episode metadata + preferences → selected pool | relevance floor, episode predicates, holiday matches, novelty/diversity, explanation trace |
| Broadcast outcome | selected pool + policy + fixed clock/history → desired lineup | final episode IDs/order, movie-group atomicity, daypart/holiday activation, no-repeat and safety invariants |

Concrete harness changes:

- Add exact `RequireTitle`, `RequireKey`, `RequireEpisode`, `ForbidEpisode`, and schedule predicates.
  A hard user constraint must never be represented only in prose sent to a judge.
- Materialize a fixture-backed `schedule.DesiredLineup` for behavioral cases. Assert the program
  sequence after episode expansion, policy filtering, grouping, and ordering.
- Partition cases by intent axis: literal title, era, audience, exclusion, classic/best episode,
  sequential binge, curated reruns, ordered movie franchise, holiday, daypart, weather/mood,
  sparse library, empty retrieval, conflicting qualifiers, and adversarial text.
- Run deterministic fixture cases on every normal test gate. Keep real-provider evaluations opt-in
  and serial.
- For hosted evaluation, run each stochastic case at least three times initially and report pass
  rate plus min/median/max relevance and serendipity. Three is a pragmatic Loomarr starting point,
  not a claim that three samples establish statistical significance; increase repetitions only for
  cases whose variance changes a release decision.
- Compare at least two pinned generator configurations against an independently pinned judge.
  Randomize or reverse comparative presentation order when a judge sees alternatives. Retain the
  raw per-run structured evidence needed to explain regressions, without storing credentials.
- Add a small hand-reviewed “gold” subset. A release should not pass solely because one model grades
  another model favorably.
- Record latency, tokens/cost, tool calls, candidates surfaced, grounding stage, resolved model, and
  resolved provider. Quality per dollar and quality per second matter on a homelab product.

### Resource envelope

- Never run an uncapped local Ollama matrix as a default gate.
- Run hosted cases sequentially with explicit per-run and whole-suite request/token budgets.
- Stop the suite when its budget is exhausted and report `budget_exhausted`, not a misleading quality
  failure.
- Cache immutable catalog fixtures for the hermetic lane. Real library/TMDB evaluation remains a
  separate integration lane so provider drift does not make normal tests flaky.

## 2. Add real episode-level editorial curation

### Research findings

TMDB exposes season and episode detail resources, and its episode-group API explicitly distinguishes
original-air-date, absolute, DVD, digital, story-arc, production, and TV orderings. That proves there
is no single universal episode order to infer from season/episode numbers alone.
([season details](https://developer.themoviedb.org/reference/tv-season-details),
[episode details](https://developer.themoviedb.org/reference/tv-episode-details),
[episode-group types](https://developer.themoviedb.org/reference/tv-episode-group-details))

TMDB's TV discovery API exposes useful title-level filters including vote average/count, origin
country, original language, runtime, networks, genres, and keywords. These are also evidence that
popularity alone is an unnecessarily lossy retrieval signal.
([TMDB TV discovery](https://developer.themoviedb.org/reference/discover-tv))

TMDB collection details provide the first-party franchise grouping resource Loomarr already uses for
movies. Editorial reranking must not reorder members inside one of those atomic movie groups.
([TMDB collection details](https://developer.themoviedb.org/reference/collection-details))

### Separate selection from ordering

The key distinction is:

```text
intent + grounded series + episode metadata
                  │
                  ▼
        deterministic episode selector
        (which episodes are eligible/preferred)
                  │
                  ▼
       existing ordering and constraint engine
       (when those episodes air in the cycle)
```

For `classic Simpsons`, “classic” should narrow and rank the episode pool; `syndication` should then
deal that selected pool without chronological progression. For `watch The Simpsons from the start`,
the selector admits the requested scope and `sequential` orders it. The same selection mechanism can
identify Christmas or Halloween episodes before the calendar rule boosts or exclusively activates
them.

Movies follow a different invariant. Candidate groups may be shuffled among other programming, but
films within a TMDB franchise group remain an atomic block in release order. The episode reranker
must receive and return groups as indivisible units where that floor applies.

### Minimum episode metadata

Extend the materialized episode cache, populated by the existing asynchronous refresh path, with:

- overview/summary;
- genres and media-server tags where available;
- air or production date;
- community rating and rating-count/confidence where available;
- external episode IDs sufficient for an optional cached TMDB enrichment;
- a normalized set of detected holiday/topic terms and their provenance;
- a metadata-fetched timestamp/version so old sparse cache rows can fail open for taste, while
  audience safety continues to use its existing fail-closed rule.

Do not fetch any of this from the scheduler. The cache remains a materialized answer and the
scheduler remains I/O-free. TMDB enrichment should be bounded, cached, optional, and performed by the
existing maintenance/job infrastructure. An unavailable metadata provider must degrade to a less
curated eligible pool, not dead air.

### Deterministic selector contract

The selector should accept a normalized editorial intent plus playable episodes and return selected
episodes with score components and reason codes. A practical first score is:

```text
episodeScore = intentMatch + eraMatch + holidayMatch + qualityConfidence
             + householdAffinity - explicitNegative
```

Rules before scores:

- audience, explicit exclusions, availability, season bounds, multipart atomicity, and `never` are
  hard filters;
- “best” may use quality only when backed by sufficient rating count/confidence, avoiding one-vote
  outliers;
- “classic” is not synonymous with “early”: combine the grounded era/season window with episode
  quality and theme evidence;
- holiday classification should match episode title, overview, and explicit tags. A whole series is
  not a holiday episode merely because one episode is;
- stable score ties use canonical episode identity, then a channel/cycle seed only where deliberate
  variety is wanted.

Every selected episode should carry reason codes such as `era_match`, `holiday_title`,
`quality_supported`, `household_keep`, and `diversity_pick`. This makes the behavior testable and lets
the UI explain it without asking another model.

## 3. Add explicit household feedback and deterministic reranking

### Research findings

Explicit, scrutable user models let people understand and efficiently correct recommendations;
research comparing such models with conventional item-rating approaches found that users could
adjust them more directly without giving up baseline recommendation quality.
([Balog, Radlinski, and Arakelyan, SIGIR 2019](https://research.google/pubs/transparent-scrutable-and-explainable-user-models-for-personalized-recommendation/))

Conversational-recommendation research identifies distinct feedback intents such as rejection,
already-seen, feature critique, adding a constraint, and providing more preference detail. Loomarr
should not collapse all negative actions into one undifferentiated “dislike.”
([Cai and Chen feedback-intent taxonomy](https://www.comp.hkbu.edu.hk/~lichen/download/RecSys19_LBR.pdf))

MMR is a simple greedy reranking method that explicitly trades query relevance against similarity to
already selected items. It is appropriate here because Loomarr already has a bounded candidate set
and needs an inspectable diversity mechanism, not another learned serving system.
([original MMR paper](https://www.cs.cmu.edu/afs/cs/Web/People/jgc/publication/MMR_DiversityBased_Reranking_SIGIR_1998.pdf))

A large-scale user study found that perceived serendipity depends on relevance as well as novelty,
unexpectedness, and timeliness, and is associated with user satisfaction. “Surprising” but irrelevant
results therefore do not meet the product goal.
([Chen et al., WWW 2019](https://www.comp.hkbu.edu.hk/~lichen/download/p240-chen.pdf))

Time, holiday, location, and similar factors are first-class contextual variables in context-aware
recommenders, but the literature distinguishes fully observed from partial or unobserved context.
Loomarr should pass only context it actually knows, with provenance, instead of asking a model to
invent weather or household circumstances.
([Adomavicius et al., AI Magazine 2011](https://onlinelibrary.wiley.com/doi/10.1609/aimag.v32i3.2364))

### Feedback vocabulary

Start with four deliberately different controls:

| Control | Persistence | Deterministic effect |
|---|---|---|
| `keep` | channel/household | protect the item from automatic retirement and add positive affinity to its grounded features |
| `less like this` | household | soft penalty for the item and supported features; it may still appear when the intent explicitly demands it |
| `never` | household | hard exclusion of the exact canonical identity; broader feature bans require an explicit separate action |
| `surprise me` | one request or bounded channel mode | lower the relevance weight only within a floor and raise novelty/diversity; do not create a durable dislike |

Store the actor for audit, but apply the signal at the household/channel scope chosen by the product
contract. Do not infer preference from proposal denial, channel deletion, or playback interruption:
those actions have multiple meanings and would silently turn operational behavior into taste data.

### Reranker contract

Apply hard constraints first, compute transparent base scores second, then greedily rerank:

```text
base(c) = relevance(c)
        + preference(c)
        + quality(c)
        + availability(c)
        + context(c)

pick(c | selected) = lambda * base(c)
                   - (1 - lambda) * maxSimilarity(c, selected)
                   + boundedNovelty(c)
```

`lambda` is a fixed product policy in the first version, shifted within a safe range by `surprise
me`. It should not become a global setting until real feedback shows an operator-facing knob is
necessary. Similarity can initially be a deterministic weighted overlap of media type, series,
franchise, genres, decade, keywords/tags, creators/cast where grounded, and runtime band. This is
cheap, explainable, and adequate for the bounded 6–8-pick proposal.

Availability is a preference, not a prison: owned titles make a channel playable immediately, while
high-quality outside-library discoveries remain eligible and are intentionally represented when the
acquisition budget permits. Approval and per-user acquisition quotas continue to own whether those
discoveries are acquired.

### Sparse-data behavior

- With no feedback, scores reproduce the non-personalized baseline.
- Exact `never` takes effect immediately and visibly.
- Soft feedback is confidence-weighted so one click does not erase an entire genre.
- Positive and negative evidence decays or can be removed; the stored event remains auditable.
- Show the reasons that materially changed a result (“kept on this channel,” “less like a title you
  marked,” “new to your library,” “fits rainy late night”).
- Keep time/daypart/holiday context deterministic from Loomarr's configured timezone. Weather is
  opt-in external context with location, freshness, and failure provenance; stale or absent weather
  contributes zero rather than blocking discovery.

## Recommended implementation sequence

### 1. Behavioral certification first

Build exact proposal predicates, fixture-backed schedule outcomes, repetition/provider metadata, and
cost caps. Add regression cases for classic-vs-chronological TV, holiday episodes, and ordered movie
franchises before changing selection behavior. This makes the next two steps measurable.

### 2. Episode selector second

Amend the programming design, enrich the episode-cache contract, implement a pure selector, and wire
it before existing ordering. Begin with classic/best/holiday intent features and reason codes. Retain
the sparse-cache fail-open behavior for taste and the current fail-closed audience behavior.

### 3. Feedback and reranking third

Define the household-feedback domain and authorization semantics, add forward-only persistence and
store conformance, expose the four controls, then apply signals through the same selector/reranker.
Keep `never` as a hard identity filter; all other effects remain bounded, inspectable score terms.

## Acceptance evidence for the three issues

1. **Evaluation:** the corpus fails if a named must-include is absent; a scorecard shows repeated run
   distributions, resolved provider/model, budget/cost, and final scheduled IDs; no normal test needs
   network access.
2. **Episodes:** a classic single-series fixture selects a defensible non-chronological episode pool;
   explicit start-to-finish remains chronological; holiday episodes are selected individually; movie
   franchise members remain adjacent and in release order.
3. **Feedback/reranking:** identical inputs produce identical rankings; `keep`, `less`, and `never`
   have distinct tested effects; `surprise me` increases diversity without crossing the relevance
   floor; outside-library candidates remain present and acquisition still requires the existing
   approval/quota path.

## Explicit non-goals

- Training a household model or adding a vector database.
- Running an always-resident local LLM for ranking or evaluation.
- Fetching TMDB or media-server metadata from the scheduling hot path.
- Treating playback stops, deletions, or proposal denial as implicit dislike.
- Letting personalization weaken grounding, audience safety, authorization, approval, or quota
  enforcement.
- Replacing the existing ordering engine with an LLM-authored schedule.
