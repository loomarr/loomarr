# Loomarr Programming Heuristics — Channel Policy Design

**Status:** Draft for implementation · companion to `design.md`
**Precedence:** the main doc owns the subsystems (Suggester §8, Scheduler §9); this doc owns the *programming intelligence* that runs between them — what makes a generated channel feel like television instead of a playlist. Conflicts → main doc on architecture, this doc on heuristics; fix the loser in the same PR.

---

## 1. The prime principle: the LLM extracts, deterministic code enforces

Every heuristic in this doc splits into two halves:

- **Extraction** (suggestion-time, §8 pipeline): the LLM interprets intent into a structured, schema-validated **ChannelPolicy** — "old-school Simpsons" → `{series: [tmdb:456], seasons: {min:1, max:10}}`; "90s Saturday cartoons" → `{audience: "kids", era: {from:1990, to:1999}, genres: ["Animation"]}`.
- **Enforcement** (schedule-time, §9 lineup builder): deterministic filters and constraint-aware slotting apply the policy. The LLM never places a program; it only proposes *rules*, and every rule it proposes is machine-checkable (grounding extends to policy: season ranges verified against the actual series, rating values from a closed enum, series ids resolved).

If enforcement lived in the model, one hallucination puts season 14 on the old-school channel. It doesn't, so it can't.

## 2. ChannelPolicy — the schema (the contract of this doc)

Stored per channel (JSON on the channel row), produced by the suggester, edited in proposal review and the channel editor, consumed by the lineup builder. Every field optional; omitted = global default (`config-design.md` §"per-channel tier").

```jsonc
{
  "scope": {                       // WHAT is allowed on this channel
    "series":      ["tmdb:456"],   // resolved ids only — never names
    "collections": [],             // media-server collections (Kometa etc.)
    "seasons":     {"min": 1, "max": 10},   // per-series season window
    "era":         {"from": 1990, "to": 1999}, // by first-air/release year
    "genres":      {"include": ["Animation"], "exclude": ["Documentary"]},
    "runtimeMax":  3600            // seconds; "nothing over an hour"
  },
  "audience": {                    // WHO it's for — safety-critical
    "ceiling":  "TV-Y7",           // closed enum ladder (§4)
    "unrated":  "exclude"          // exclude | allow  — kids default: exclude
  },
  "separation": {                  // HOW OFTEN things may recur (§3)
    "episodeNoRepeat": "168h",     // same episode: once per window
    "movieNoRepeat":   "720h",
    "seriesMinGap":    "2h",       // same series on MIXED channels
    "blockMax":        2           // max consecutive slots from one series
  },
  "ordering": "syndication",       // sequential | shuffle | syndication (§5)
  "seasonal": {                    // WHEN in the year (§6)
    "mode":     "auto",            // off | auto | exclusive
    "holidays": ["halloween", "christmas"]  // subset of built-in calendar
  }
}
```

**Single-series channels** (a Simpsons channel) auto-relax `seriesMinGap`/`blockMax` — separation there means *episode* spacing, not series spacing. The relaxation is rule-based, not LLM judgment.

## 3. Separation & repetition ("don't show the same thing twice in a row")

Tunarr lineups are **cycles** — an ordered list that loops. So separation is enforced along the *cycle timeline including the wrap*: the last Simpsons block and the first must also honor the gap, or the loop seam betrays the illusion.

- **Hard floors:** `episodeNoRepeat` (an episode appears once per window), `movieNoRepeat` (longer — a movie reappearing in a week feels cheap).
- **Texture rules (mixed channels):** `seriesMinGap` keeps two different Simpsons blocks from airing 30 minutes apart; `blockMax` permits the classic *two-in-a-row then move on* syndication feel without letting one series colonize an evening.
- **Cycle-length consequence:** the no-repeat window implies a minimum pool size. If the pool can't fill the window, do **not** fail and do not silently violate — descend the **relaxation ladder (§7)**.
- All slotting is **seeded-deterministic** (seed = channel + cycle index, extending the main doc's pod rule) — same pool + same policy + same seed = same cycle, so tests reproduce exactly.

## 4. Audience safety ("Saturday cartoons must never go adult")

The one heuristic where an error is a *harm*, not an aesthetic bug — so it fails closed:

- `ceiling` is a **closed ordered ladder** spanning both TV and film systems: `TV-Y < TV-Y7 < TV-G/G < TV-PG/PG < TV-14/PG-13 < TV-MA/R/NC-17`. Enforcement compares the item's `OfficialRating` (same field name on Emby and Jellyfin) mapped into the ladder; unmappable strings are treated as unrated.
- **v1 scope — series-level ceiling.** The current library adapter surfaces `OfficialRating` on movies/series but **not per-episode** (episodes carry only id/name/duration/season). So a series is gated at its *series* rating: a mixed-rating series clears or fails as a whole. This is a small, deliberate safety *narrowing* (a series never sneaks over the ceiling; at worst a below-ceiling series with an occasional harder episode is admitted). Per-episode ceilings are future work — add `OfficialRating` to `library.Episode` + `ResolvedProgram`, then filter at episode expansion. The rating/genres/year an item is filtered on are **stamped onto the channel's approved lineup entry at create time** (when the full grounded candidate is in hand), so enforcement is a pure entry-set filter with no per-reconcile library I/O.
- **Kids ceilings (`TV-Y`…`TV-PG`) default `unrated: "exclude"`** — an item with missing or unmappable rating metadata is *excluded*, never guessed at. Metadata gaps are the real-world failure mode; a kids' channel must be safe against them by construction. Adult/general channels default `allow`.
- **Transparency at review:** the proposal shows the policy's effect — "14 items excluded: 11 over ceiling, 3 unrated" — so gaps are visible *before* approval, and the fix (rate your media, or relax the policy) is a human decision.
- The LLM's job is only to *infer* the ceiling from intent ("cartoons," "Saturday morning," "for the kids" → `TV-Y7`); the inferred value is shown as an editable chip in review, and enforcement is the ladder comparison, nothing else.

## 5. Ordering modes ("feels like TV")

- `sequential` — S1E1 onward, loops at the end (binge/marathon channels).
- `shuffle` — seeded random, separation-constrained.
- `syndication` (default for TV) — random **without repeats until the eligible pool exhausts**, then reshuffle (a "deck deal"): the authentic weekday-rerun texture, and it makes `episodeNoRepeat` nearly free because the deck *is* a no-repeat structure. Each deck reshuffles under `seed XOR deckIndex` so successive decks differ yet every deck is deterministic for a given channel seed (the §7-mandated reproducibility).
- **Omitted `ordering` inherits the channel's `Strategy`.** A channel created without an explicit policy ordering keeps its existing `sequential`/`shuffle` behavior — the syndication default applies only when a policy explicitly requests it (or a template ships it). This keeps policy adoption non-breaking for existing channels.

## 6. Seasonality ("holiday episodes at holiday time — and only then")

Two symmetric behaviors, because knowing what October wants implies knowing what July doesn't:

- **Detection** (deterministic, at catalog/proposal time — not the LLM): an item is *seasonal* for holiday H if it matches H's keyword set against episode/movie title, media-server tags/keywords, or TMDB keywords ("halloween," "christmas," "thanksgiving," …). Built-in calendar v1: Halloween (Oct 1–31), Thanksgiving-US (Nov 15–30), Christmas/holidays (Dec 1–26), New Year (Dec 27–Jan 2), Valentine's (Feb 1–14). Windows and keyword sets ship as data, not code; custom holidays/regions are future work.
  - **v1 scope — entry-level detection.** Detection matches keywords against the fields stamped on the lineup entry (title + genres). Per-episode tag/keyword matching (and TMDB-keyword lookups) is future work — it needs per-episode metadata the library adapter doesn't yet surface. So in v1 a seasonal *series* is benched/boosted as a whole rather than episode-by-episode.
- **`mode: "auto"` (default):** in-window, seasonal items get a scheduling **boost** (weighted up, tasteful — not wall-to-wall); out-of-window, detected-seasonal items are **benched** (excluded). The bench is the half everyone forgets and the one viewers notice: *Christmas episodes in July break the spell.*
- **`mode: "exclusive"`:** the channel *is* the holiday (a December Hallmark-style channel): only in-window seasonal content airs; out of window the channel runs its `offSeason` fallback (loop scope without seasonal filter, or go dark — policy field, default loop).
- **`mode: "off"`:** no detection, no bench — for channels where a Halloween Simpsons episode in March is fine.
- Evaluation uses the container `TZ` wall-clock (main doc §9) at reconcile time; the periodic sweep naturally rolls channels into and out of windows within `CHANNEL_RECONCILE_EVERY` — seasonality needs no scheduler of its own.

## 7. The relaxation ladder (constraints degrade predictably, never silently)

When the eligible pool can't satisfy the policy (small library ∩ tight scope ∩ long no-repeat window), enforcement descends in order, and **every applied relaxation is recorded on the channel and surfaced in the UI** ("policy relaxed: repeat window 7d → 3d"):

1. Shorten `episodeNoRepeat`/`movieNoRepeat` (halve, floor 24h).
2. Relax `seriesMinGap`/`blockMax`.
3. Widen `era` by ±2 years per step (never past the intent's decade boundary if one was stated).
4. Pad with filler pods (§10 main doc — never dead air).

**Never relaxed, ever:** `audience` and explicit scope filters (series/seasons). A too-small kids' pool becomes a filler-heavy kids' channel — it does not become a less-kids channel. Mirrors the pod fallback ladder's philosophy: degrade quality, never safety or identity.

## 8. Pipeline placement & proposal surface

- **Suggester (§8):** output contract gains `policy` (schema above), grounded like everything else; templates ship pre-filled policies ("90s Saturday Morning" carries `TV-Y7` + era + genres out of the box); intent-hint copy teaches the constraint vocabulary.
- **Proposal review + channel editor (§12):** policy renders as editable chips (ceiling, seasons, era, ordering, seasonal mode) + the exclusion report (§4) + a **cycle preview** (first N slots with separation annotations) so "did old-school bind?" is answerable by looking. The same chip surface is the **per-channel rules editor** on the channel page (§7 `PATCH .../{id}` writes `policy_json`); omitted chips inherit the built-in default (§9), and `audience` + explicit `scope` are shown as never-relaxed safety fields.
- **Filler is part of the channel editor too (§10):** `policy.filler` (the `FillerSelection`) is edited on the channel page alongside the rules chips — theme criteria (era/audience/category/kinds) + pinned/excluded clips — with a live pod sandbox (`POST …/pods/preview`) that re-assembles the actual break against the unsaved draft before Apply. It also rides `policy_json`, so it round-trips and inherits the same "omitted = any" default; a new channel seeds its filler era from `scope.era`.
- **Lineup builder (§9):** hard filters → eligible pool → seeded constraint-aware slotting (greedy with backtracking is sufficient at envelope scale) → relaxation ladder on failure → pods → push. The periodic sweep re-evaluates policy (seasonal windows roll, library grows, relaxations un-relax when the pool recovers).

## 9. Extensibility — the checklist for "I'm sure I can think of more"

Every future heuristic is added the same way; a heuristic is *done* when all five exist:

1. **Policy field** — schema + default + per-channel override (config doc tier). **v1 substrate:** built-in Go constants supply the default and `policy_json` on the channel row holds the per-channel override — a two-tier `channel-policy > built-in`. The `config-design.md` registry-*default* middle tier (the third tier) is deferred until the settings registry lands; it slots into the policy resolver later without touching enforcement. Omitted fields resolve to the built-in constant; an omitted `ordering` resolves to the channel's `Strategy`.
2. **Extractor hint** — one line in the suggester's system prompt + template updates, if the LLM should infer it.
3. **Deterministic enforcer** — filter or slotting constraint in the lineup builder; ladder position if relaxable (or listed never-relaxed).
4. **Proposal surface** — chip + effect visibility in review.
5. **Tests** — binding + violation + relaxation + determinism.

Candidates already visible from here (logged, not v1): dayparting audience ceilings (stricter mornings), episode-quality floors (community ratings), "premiere" slotting for newly-landed backfill, per-holiday custom calendars, inter-channel dedup (don't air the same movie on two channels the same night).

## 10. Tests (extends main doc §19 — these join the phase 10/11 gates)

- **Binding:** "seasons 1–10" policy → zero slots outside the range across many seeded cycles; era and genre filters likewise.
- **Fail-closed audience:** unrated item + `TV-Y7` ceiling → excluded, counted in the exclusion report; `TV-MA` never appears under any kids ceiling *including after full ladder relaxation*.
- **Separation with wrap:** property test — no episode twice inside the window, no series-gap violation across the cycle seam.
- **Syndication deck:** every eligible episode airs exactly once per deck before any repeats.
- **Seasonality under a frozen clock:** Oct 15 boosts Halloween items; Jul 14 benches them; `exclusive` runs `offSeason` fallback out of window; window roll happens via the sweep.
- **Relaxation ladder:** tiny pool → ladder descends in order, records + surfaces each step, and *never* touches audience/scope; pool growth un-relaxes on a later sweep.
- **Determinism:** same pool + policy + seed → byte-identical cycle.

## 11. Build integration

- **Phase 10 (Scheduler):** policy enforcement, separation/ordering/seasonal evaluation, relaxation ladder, cycle preview endpoint data.
- **Phase 11 (Suggester):** policy extraction in the output contract + grounding of policy values + template policies.
- **Phase 13 (UI):** policy chips, exclusion report, cycle preview, relaxation banners.
- Global defaults + built-in holiday calendar: see the revised `config-design.md` (per-channel tier + new registry keys).
- Seed doc: incorporate as `docs/programming-design.md` in phase 14; the Concepts page inherits §1's extract/enforce principle.
