# Clip taxonomy — design (V45)

The flat 12-value `category` string cannot express what curation needs (§10 V45): a channel rule
like "one **food** ad per break" cannot ask "is `cereal` a kind of food?", and a free-text set drifts
(`drinks` vs `beverages`, the LLM emitting `soda`). This designs a **robust, flexible, LLM-friendly
taxonomy**: a multi-tag model over an operator-editable, database-backed vocabulary.

Maintainer decisions (2026-08-07): **multi-tag set** (a clip carries several tags, not one category)
over a **database table** (operator-editable), designed to be **LLM-friendly**.

## The model — tags over a taxonomy graph

A clip has a **set of tags**, not a single `category`. Each tag references a **taxon** in a taxonomy
the operator can edit. A taxon is:

```
Taxon {
  slug:     "beer"                       // stable machine id, lowercase, the LLM emits this
  label:    "Beer"                       // human display
  parent:   "alcohol"  (nullable slug)   // the IS-A edge → rollups
  synonyms: ["brew", "lager"]            // LLM-friendliness: map a near-miss to the canonical slug
  axis:     "product" | "format" | "audience-cue" | "seasonal"   // independent dimension
  aliases_retired: ["beverage-alcoholic"] // renamed slugs still resolve (no silent drop on rename)
}
```

Parent edges form a **forest** (multiple roots within independent axes), not one tree — because a clip
is tagged on several independent axes at once:

- **product** axis: `beer` → `alcohol` → `drinks`; `cereal` → `food`; `dealer` → `cars`
- **format** axis: `commercial`, `psa`, `ident`, `promo`, `bumper` (what the clip IS, not what it
  sells; movie trailers live under the `movies` product/topic branch)
- **seasonal** axis: `christmas`, `back-to-school`, `memorial-day` (reuses the §10 holiday keyword IDs
  from the curation research)
- **audience-cue** axis: kept SEPARATE from the existing `audience` enum — a cue like `kids-cue` is a
  hint, not the audience verdict.

A clip's asserted tag set spans axes: the KCPQ Butterfinger-at-Christmas clip could assert
`{candy, christmas}`; its expanded matching set is `{candy, food, christmas}`.

⚠ **Rollups are stored DENORMALISED** (maintainer decision, 2026-08-07): when a clip is tagged
`beer`, `clip_tags` gets `beer` AND `alcohol` AND `drinks` rows, each flagged whether it is a LEAF
(the model/operator asserted it) or a ROLLUP (derived from the graph). A curation query `WHERE taxon
= 'food'` is then one index hit, no graph walk — the read path stays fast and simple, which matters
because pod assembly runs it per break per reconcile.

The cost this accepts, stated plainly: each owning write recomputes derived rows. A per-clip retag
reads the store's current closure and updates that clip's assertions, rollups, and category shadow in
one transaction. A graph edit rebuilds the closure and all catalog rollups set-wise in its own
transaction. The graph is the SOURCE OF TRUTH; the denormalised rows are a derived cache of it — the
same "synced cache" shape `clips` already is (§10 V38c). ⚠ Only ASSERTED rows survive directly;
rollups are always recomputed, so a parent remap never leaves stale ancestors behind.

## Why this shape

- **Multi-tag** is what real ads need — a Christmas beer ad is `beer` AND `christmas` AND `alcohol`;
  forcing one `category` loses two of those. Curation queries "no alcohol on a kids channel" and
  "one food ad per break" both become tag-set membership tests.
- **The parent graph makes rollups queryable** — "one food ad per break" is `count(tags ∋
  descendants(food)) ≤ 1`. A flat set cannot express it.
- **Operator-editable (DB)** — an operator adds `energy-drink` under `drinks`, or splits `local` into
  per-market taxa, without a code change. Seeded with a sensible default forest (below).
- **The forest-by-axis** stops the classic taxonomy mistake of cramming format + product + season +
  audience into one tree where `psa` and `beer` are false siblings.

## LLM-friendliness — the load-bearing property

The tagger must reliably emit valid tags from a model that has never seen our slugs. Four mechanics:

1. **Served vocabulary, BE is the single source.** `taxonomy.Forest.Vocab()` serves the model the
   current canonical slugs grouped by axis; `GET /v1/taxonomy` serves the UI the richer graph with
   labels, hierarchy, synonyms, aliases, and usage. The prompt says to choose only from the listed
   slugs. The model never guesses a slug blind, while the operator sees human labels and context.
2. **Grounding = resolve-or-drop, with synonym rescue.** Every tag the model returns is resolved:
   exact slug → keep; a known `synonym` or retired `alias` → map to the canonical slug (this is the
   LLM-friendly rescue — "brew" becomes `beer`); anything else → DROPPED, never persisted. Same
   anti-fabrication discipline as era/brand (§8): an ungrounded tag vanishes, it does not become a
   new taxon. Only an OPERATOR adds a taxon, never the model.
3. **Rollup is automatic, not the model's job.** The model emits asserted tags; the parent tags come
   from the graph. Usually the asserted tag is a specific graph leaf ("this is a beer ad"), but a
   broader `food` assertion is valid when the evidence cannot support `cereal`. The `clip_tags.leaf`
   column keeps its historical name but means *asserted*, not “currently has no children”. This is
   why the wire carries asserted tags separately from the full asserted-plus-rollup set: an editor
   must never feed derived ancestors back as new assertions.
4. **Confidence is clip-level, not invented per tag.** The model reports one confidence for its
   classification and the existing score ceiling can only lower it. Taxonomy grounding proves that
   an emitted term belongs to the vocabulary; unlike era and brand, it does not claim a literal text
   match for every known tag. Unknown terms are dropped, and uncertain known tags remain visible
   through the clip's overall confidence/review flow.

The same contract applies to every classifier modality. Text classification, clip keyframe vision,
and compilation-segment vision all receive `Forest.Vocab()`, return multiple tags, and pass through
the same resolver. A clip vision write commits its frame facts plus the additive asserted tags,
rollups, and category shadow together. Confirming a split proposal persists its grounded segment
tags on the new content-addressed child immediately; they are not left in proposal JSON for a later
job to rediscover.

## The seed forest (default taxonomy, operator-extendable)

Covers the composites-mock taxonomy and the original 12, arranged on axes:

- **product** → drinks (→ soda, juice, water, coffee, alcohol → beer, spirits); food (→ fast_food,
  cereal, candy, snacks, frozen); household; tech (→ telecom, appliances); cars; apparel; retail;
  toys; games; travel; local (→ dealer, restaurant-local, service-local)
- **format** → commercial, psa, ident, promo, bumper, interstitial
- **seasonal** → christmas, halloween, thanksgiving, back-to-school, memorial-day, holiday
- **audience-cue** → kids-cue, family-cue, late-night-cue (hints only; the older phrases resolve as
  synonyms)

`movie_trailer` is a product/topic child of `movies`, with `trailer` as a synonym; it is not also a
format node. `station-id` resolves to the canonical format node `ident`. One canonical home for each
concept avoids two slugs that mean the same thing.

⚠ The store keeps `category` as a compatibility shadow (the primary leaf product tag) so nothing that
reads `category` today breaks during migration — but new curation reads the tag set. The flat column
is derived from the tag set, not the other way round.

## What this touches (the build, doc-first)

- **New store tables**: `taxa` (the graph) + `clip_tags` (clip↔taxon, many-to-many), created by
  forward-only migrations and seeded from `taxonomy.SeedForest()` at boot when the graph is empty.
  `category` stays as a derived shadow.
- **`internal/taxonomy`** (new domain pkg): the taxon type, `Resolve(raw) → canonical|drop` with
  synonym/alias rescue, `Ancestors(slug)` for rollups, and `Forest.Vocab()` for the LLM.
- **Text and vision taggers**: emit tag SETs; ground each via `taxonomy.Resolve`; rollups are derived;
  prompts are served the live vocab. This replaces every hard-coded category list.
- **API**: taxonomy CRUD (operator edits) + tags on `ClipDTO`; the composites UI renders tag chips.
- **Curation** (later): rule predicates over tag-set membership incl. rollups.

## Operator contract and graph integrity (V55)

The editable graph is one deep module: callers ask it to apply a semantic change, not to choreograph
row writes, closure replacement, and catalog reindexing themselves. Every create, edit, or delete is
validated against the prospective whole forest and then commits the graph, `taxa_closure`, derived
rollups, and the `category` compatibility shadow in one store transaction. The following states are
rejected before any write:

- a missing, self, cross-axis, or cyclic parent;
- an unsupported axis, malformed stable slug, or blank label;
- a case-insensitive collision between any slug, synonym, or retired alias.

A slug is immutable through edit. A rename is a deliberate migration because curation rules and
persisted tags name the slug; the old slug belongs in `retiredAliases`. Deleting an unused
intermediate taxon promotes its children to the deleted node's parent. Deleting a taxon that is
directly asserted on any catalog clip is refused until those clips are retagged, so “tidying the
vocabulary” cannot silently discard library knowledge.

`GET /v1/taxonomy` is also the library accounting read model. Alongside the forest it reports the
catalog total, the number with at least one asserted tag, unique coverage on each axis, and for each
taxon playable direct assertions, total descendant matches, and all stored direct assignments. The
last count includes held, removed, and composite records and drives deletion safety; the taxonomy UI
uses those facts to show coverage, hierarchy, and edit impact. Taxon counts and per-axis populations
link to server-filtered catalog views, so pagination never turns the accounting into a client-side
estimate. Axis coverage is counted independently, but absence stays neutral because seasonal and
audience cues are intentionally sparse. The clip editor writes `assertedTags`; `tags` remains the full
expansion used for matching and display. Brand, era, audience verdict, and scheduler `kind` remain
separate facts with distinct grounding or playout contracts. The optional `format` taxonomy axis is
descriptive and operator-editable; `kind` is the closed field that decides how the scheduler may use
the clip, so a similarly named format tag never changes playout behavior. The clip editor can correct
or clear brand directly:
the classifier is additive and will not overwrite an existing brand, so the operator must remain the
recovery authority without representing advertisers as taxonomy nodes.

This is a phase of its own (call it V45a, the taxonomy), landing BEFORE the flat-category FE work —
because the composites UI should render tags, not a single category, from the start.

## Relationship to the embedding column (V45 Part 4)

The taxonomy and the embedding both answer "does this clip fit a channel's theme?" — but at opposite
ends of a spectrum, and the taxonomy CHANGES the embedding's role:

- **The taxonomy is the STRUCTURED, deterministic half** — `beer IS-A alcohol IS-A drinks`, `christmas`.
  It answers "no alcohol on a kids channel", "one food ad per break", "a Christmas-themed break"
  precisely. This is exactly what the embedding prototype FAILED at (it ranked a sci-fi promo below a
  Christmas candy ad — §10, embedding findings).
- **The embedding is the FUZZY residue** — tone/vibe similarity ("clips that feel like this nostalgic
  drama") and near-duplicate detection. Things the taxonomy cannot enumerate.

⚠ **The taxonomy SHRINKS the embedding's job, which is a simplification, not a conflict.** The
prototype already demoted embeddings to "dedup + lexical search"; with the taxonomy covering the
structured half of theme, the embedding's remaining role is narrower still — pure vibe similarity and
content-dedup. That reinforces "do NOT build a standalone vector DB": an even smaller job does not
justify more infrastructure.

Two design consequences to honour when the embedding lands (NOT built in V45a):

1. **Tags FEED the embedding.** Embedding `"beer alcohol christmas · <transcript>"` (grounded tags
   prepended) yields a better semantic vector than raw transcript alone — the tags anchor the fuzzy
   text. So the embedding must be generated AFTER tagging, from tags + transcript, not from transcript
   alone.
2. **Reindexing is not a background-job seam.** A graph edit rebuilds the small closure table and all
   denormalised rollups with set-based SQL before its transaction commits; a clip re-tag rebuilds
   that clip's expansion in its own write. If embeddings are reconsidered later, their lifecycle
   must not weaken this atomic taxonomy contract or make graph correctness depend on a worker.
