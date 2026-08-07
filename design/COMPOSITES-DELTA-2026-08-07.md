# Composites & lineage — prototype delta, 2026-08-07 (V45)

The composites/lineage design landed as `loomarr-composites-desktop.dc.html` +
`loomarr-composites-mobile.dc.html` + `composites-data.js` (canonized from a Claude Design export,
prompt in `v45-composites-prompt.md`). This records the **look and structure** the FE builds from.

> Precedence (`design/README.md`): this describes **look and structure**. `docs/design.md` §10 V45
> wins on behaviour; the sample strings below are mock data, not a contract.

Assessed live (Playwright, both tabs) before canonizing — the design solves every hard V45 problem
and is ready to build against. Screenshots were taken of the Catalog (expanded break + filmstrip)
and Incoming (review flow) states.

## The two surfaces

### Catalog tab — composite as an expandable container

- Header/pool strip reframed for composites: `CLIPS 60 · BREAKS KEPT 2 · POOL 22:38 · REPEATS`, with
  the teaching line **"breaks are containers — only their ads air"** and `BREAKS KEPT · containers ·
  provenance`. This is the conceptual anchor for the whole feature.
- A filter `All · Singles · Breaks` + a `BREAKS — CONTAINERS, NOT AIRABLE` section above a
  `SINGLE CLIPS` section. No new tab — the 3-tab IA (Catalog · Incoming · Sources) is unchanged.
- A **composite row** collapses to `NN CUTS` badge · title · `COMPOSITE · NOT AIRABLE` badge (signal
  amber) · broadcast context `FOX · SEATTLE · 5/28/1996` · `41 ads · 16:11` · expand chevron.
- Expanded: **the time-scaled filmstrip** (block width ∝ duration — the Paramount 1:15 trailer block
  is visibly wider; confidence tints the blocks) with a `re-split` action, then the **segment grid**
  beneath. Each segment card shows name · brand · duration · `at MM:SS` (its position in the break —
  the provenance is the timecode) · confidence %. Dropped slivers render greyed with `dropped`.
- Footer: **"break file kept · every ad above links back here · dropped slivers stay in the file,
  never air"** — the lineage mental model, plain.

### Incoming tab — the split-review flow (the coupled copy flip)

- ⚠ **The mental model is CORRECTED, which is the coupled-with-backend part.** The confirm button is
  **"Confirm — file 14 ads under this break"** and the footer reads **"the break file is kept —
  re-split any time · segments stay linked to it · nothing is deleted."** This is the exact reversal
  of the V34 "the compilation is gone" copy the FE audit flagged. FE + BE ship together.
- **Confidence-gated review** is the spine (matches `docs/design.md` §10 V45 fusion-confidence +
  the FILLER-DELTA note that `conf >= autoConf` routes what a human sees): the summary chips read
  `11 look right · 3 need a look · 1 sliver dropped`, and only the uncertain segments list below
  under `NEEDS A LOOK (3)`, each with a specific reason ("hard cut mid-spot — this may be one 0:30,
  not two", "logo obscured — could be Rainier or Olympia", dedup: "identical fingerprint to a clip
  already in the catalog — keep both or skip?").
- The filmstrip highlights the ambiguous segments amber IN THE STRIP, with `Open in strip` to jump —
  the fusion confidence made SPATIAL (you can see two adjacent amber Taco Bell blocks = "one 0:30 or
  two?"). `SPLIT BY AI` badge in suggest magenta (the AI color).
- Per-segment actions: `Looks right` / `Open in strip`; per-clip `Looks right` / `Not right`.

## Decisions this design forces (tracked for the build)

1. ⚠ **The category taxonomy EXPANDS.** The mock uses `drinks · household · retail · telecom · local
   · beer · apparel · food · promo · ident` beyond the current 12-value enum (`toys · cereal · cars
   · tech · fast_food · movie_trailer · candy · games · psa · ident · bumper · general`). Maintainer
   decision (2026-08-07): **expand the backend enum to match** — it aligns with V45's topic-hierarchy
   goal. This is a `knownCategories` change (`internal/filler/tag.go`) + the `ClipDTO`/`SplitSegment`
   `enum:` tags + the FE `KIND`/category label maps, done as one unit so the tagger, the DTO, and the
   UI agree. NOT a schema migration (category is free TEXT in the store).
2. **`NN CUTS` vs `N ads`** differ by the dropped slivers (43 cuts → 41 ads on KCPQ). Correct, but
   surface the relationship (a tooltip) so it does not read as a bug.
3. **Broadcast context** (`network · station · market · date`) is shown per composite — the structured
   parser (§10 V45 Part 3) must populate it; absent → the row omits the context line, not a blank.

## What the FE build reuses vs. adds

- **Reuse**: `SegmentFilmstrip` (already time-scaled + accessible + visually-tested) for both the
  catalog-expanded lineage view and the review strip; `ConfidenceMeter` for the per-segment %; the
  `ClipCard` badge row for the `COMPOSITE · NOT AIRABLE` badge; the `NavTabs` 3-tab shell.
- **Add**: the expandable composite row, the broadcast-context line, the `All/Singles/Breaks` filter,
  the "file N ads under this break" confirm copy, and the lineage back-link on a segment card. All
  need `isComposite`/`parentHash` (+ broadcast fields, later) on `ClipDTO` — the OpenAPI regen gates
  each.
