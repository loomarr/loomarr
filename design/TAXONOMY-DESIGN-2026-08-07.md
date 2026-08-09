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
  kind:     "product" | "format" | "audience-cue" | "seasonal"   // what AXIS this taxon is on
  aliases_retired: ["beverage-alcoholic"] // renamed slugs still resolve (no silent drop on rename)
}
```

Parent edges form a **forest** (multiple roots, one per `kind` axis), not one tree — because a clip
is tagged on several independent axes at once:

- **product** axis: `beer` → `alcohol` → `drinks`; `cereal` → `breakfast` → `food`; `sedan` → `cars`
- **format** axis: `movie_trailer`, `psa`, `ident`, `promo`, `bumper` (what the clip IS, not what it
  sells)
- **seasonal** axis: `christmas`, `back-to-school`, `memorial-day` (reuses the §10 holiday keyword IDs
  from the curation research)
- **audience-cue** axis: kept SEPARATE from the existing `audience` enum — a cue like `kids-toys` is a
  hint, not the audience verdict.

A clip's tag set spans axes: the KCPQ Butterfinger-at-Christmas clip →
`{candy, food, christmas}` (product leaf + its rollup + a seasonal tag).

⚠ **Rollups are stored DENORMALISED** (maintainer decision, 2026-08-07): when a clip is tagged
`beer`, `clip_tags` gets `beer` AND `alcohol` AND `drinks` rows, each flagged whether it is a LEAF
(the model/operator asserted it) or a ROLLUP (derived from the graph). A curation query `WHERE taxon
= 'food'` is then one index hit, no graph walk — the read path stays fast and simple, which matters
because pod assembly runs it per break per reconcile.

The cost this accepts, stated plainly: a **reindex job** must recompute a clip's rollup rows when
(a) the clip is re-tagged, or (b) the taxonomy graph changes (an operator remaps a parent). The
graph is the SOURCE OF TRUTH; the denormalised rows are a derived cache of it, rebuilt by the
reindex — the same "synced cache" shape `clips` already is (§10 V38c). ⚠ Only LEAF rows survive a
re-tag directly; rollup rows are always recomputed from the current graph, so a parent remap never
leaves stale ancestors behind.

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

1. **Served vocabulary, BE is the single source.** Like `schedule.BuildVocabulary()`, a
   `BuildTaxonomyVocab()` serves the LLM (and the FE editor) the current taxon slugs + labels +
   synonyms + a one-line gloss per axis. The prompt lists the CANONICAL slugs and says "choose from
   these; if unsure, choose the nearest parent." The model never guesses a slug blind.
2. **Grounding = resolve-or-drop, with synonym rescue.** Every tag the model returns is resolved:
   exact slug → keep; a known `synonym` or retired `alias` → map to the canonical slug (this is the
   LLM-friendly rescue — "brew" becomes `beer`); anything else → DROPPED, never persisted. Same
   anti-fabrication discipline as era/brand (§8): an ungrounded tag vanishes, it does not become a
   new taxon. Only an OPERATOR adds a taxon, never the model.
3. **Rollup is automatic, not the model's job.** The model emits leaf tags; the parent tags come from
   the graph. So the model only has to get the specific thing right ("this is a beer ad"), and "it is
   therefore a drink / alcohol" is derived — fewer ways for the model to be wrong.
4. **Confidence per tag rides the existing grounded ceiling** (§10 V38) — a tag grounded in the
   transcript ("...ice cold Budweiser...") scores high; one the model asserted without textual
   support is dropped, exactly like an ungrounded brand.

## The seed forest (default taxonomy, operator-extendable)

Covers the composites-mock taxonomy and the original 12, arranged on axes:

- **product** → drinks (→ soda, juice, water, coffee, alcohol → beer, spirits); food (→ fast_food,
  cereal, candy, snacks, frozen); household; tech (→ telecom, appliances); cars; apparel; retail;
  toys; games; travel; local (→ dealer, restaurant-local, service-local)
- **format** → commercial, psa, movie_trailer, ident, promo, bumper, station_id, trailer,
  interstitial
- **seasonal** → christmas, halloween, thanksgiving, back-to-school, memorial-day, holiday
- **audience-cue** → kids-toys, family-values, late-night-adult (hints only)

⚠ The store keeps `category` as a compatibility shadow (the primary leaf product tag) so nothing that
reads `category` today breaks during migration — but new curation reads the tag set. The flat column
is derived from the tag set, not the other way round.

## What this touches (the build, doc-first)

- **New store tables**: `taxa` (the graph) + `clip_tags` (clip↔taxon, many-to-many), seeded with the
  default forest via a forward-only migration. `category` stays as a derived shadow.
- **`internal/taxonomy`** (new domain pkg): the taxon type, `Resolve(raw) → canonical|drop` with
  synonym/alias rescue, `Ancestors(slug)` for rollups, `BuildTaxonomyVocab()` for the LLM + FE.
- **Tagger**: emits a tag SET; grounds each via `taxonomy.Resolve`; rollups derived; prompt served
  the vocab. Replaces the single `knownCategories` gate.
- **API**: taxonomy CRUD (operator edits) + tags on `ClipDTO`; the composites UI renders tag chips.
- **Curation** (later): rule predicates over tag-set membership incl. rollups.

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
2. **The reindex and re-embed jobs are SIBLINGS.** The taxonomy needs a reindex job (recompute rollup
   rows when a clip is re-tagged or the graph changes); the embedding needs a re-embed job (when the
   clip's text/tags change). Both are "derived-from-clip, rebuilt-on-change" background jobs — they
   share the transcribe/vision job pattern (opt-in, batched, record-every-outcome) and should not
   become two half-built job systems. V45a builds the reindex job; it is written so the re-embed job
   slots beside it.
