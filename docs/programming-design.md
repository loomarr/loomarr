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
- **Multi-part episodes stay together (a hard adjacency floor).** A two-parter ("The Best of Both Worlds (1)/(2)", "All Good Things… (1)/(2)") must air as an **atomic, in-order block** — Part 2 immediately after Part 1, never scattered by the shuffle or split by the rolling window (showing Part 2 with Part 1 out-of-window is worse than out-of-order). Detection is deterministic at episode-resolution time, from **either** signal: (a) the media server's `IndexNumberEnd` (a single file spanning e.g. episodes 25–26), **or** (b) a shared title base with a `(1)/(2)` or `Part N` suffix on **consecutive** episodes of the same season (the two-separate-files case, which is the common one). Grouped episodes carry a shared group id + within-group index; ordering (syndication/shuffle) permutes the group's **position** but never its internals, and the rolling-window truncation keeps a group **whole** (a group is one unit for the window budget). This is a floor, not a policy knob — a channel never *wants* a split two-parter.
- **Movie franchises stay together, in release order (the same floor, for films).** A franchise's films ("Raiders of the Lost Ark" → "Temple of Doom" → "Last Crusade") must air as an **atomic, in-release-order block**, never scattered by the shuffle (the reported bug: Temple → Crusade → *[unrelated film]* → Raiders). Detection is the **TMDB collection** (`belongs_to_collection.id`) — the authoritative signal, because it groups "Raiders of the Lost Ark" with the "Indiana Jones and the…" films even though they share **no title base** (a title heuristic can't). The collection id is resolved at reconcile (a per-movie TMDB lookup, healed onto the entry like `OfficialRating` — a tri-state field: `0` unresolved, `>0` a collection, `-1` resolved-standalone, so it's a one-time repair, not a per-sweep call) and stamped onto the lineup entry, so the **pure scheduler stays I/O-free**. Films sharing a collection get a shared group id + a **release-year** within-group index, then flow through the *same* collapse/expand atomicity as multi-part episodes — one mechanism, two sources (episode parts, franchise films). A collection with fewer than two present films isn't grouped (nothing to keep it with). Requires TMDB configured; without it, films simply aren't grouped (no worse than before).
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
- **Multi-series channels default to `syndication`.** A channel whose lineup is several series (a "Star Trek" franchise channel) should *intermix* them (the deck deal), not play all of one then all of the next. Two levers, both in the **grounded policy** (never the channel `Strategy`): (1) the suggester *prompt* guides the LLM to pick `syndication` when the lineup has more than one series; (2) `groundPolicy` applies a deterministic fallback — when the model stated no ordering **and** the grounded lineup spans ≥2 distinct series (by provisioning `Key`), it sets the policy `ordering` to `syndication`. Because an explicit policy `ordering` **wins over** the channel-`Strategy` inherit (`Resolved`), this overrides the approve-time `Strategy: sequential` default without changing it — that `sequential` remains the correct fallback for a genuinely single-series channel, which stays `OrderInherit` → chronological binge. The model's explicit ordering choice always wins over the fallback. One series plus movies is *not* multi-series (conservative: only ≥2 distinct series qualifies).

## 6. Seasonality ("holiday episodes at holiday time — and only then")

Two symmetric behaviors, because knowing what October wants implies knowing what July doesn't:

- **Detection** (deterministic, at catalog/proposal time — not the LLM): an item is *seasonal* for holiday H if it matches H's keyword set against episode/movie title, media-server tags/keywords, or TMDB keywords ("halloween," "christmas," "thanksgiving," …). Built-in calendar v1: Halloween (Oct 1–31), Thanksgiving-US (Nov 15–30), Christmas/holidays (Dec 1–26), New Year (Dec 27–Jan 2), Valentine's (Feb 1–14). Windows and keyword sets ship as data, not code; custom holidays/regions are future work.
  - **v1 scope — entry-level detection.** Detection matches keywords against the fields stamped on the lineup entry (title + genres). Per-episode tag/keyword matching (and TMDB-keyword lookups) is future work — it needs per-episode metadata the library adapter doesn't yet surface. So in v1 a seasonal *series* is benched/boosted as a whole rather than episode-by-episode.
- **`mode: "auto"` (default):** in-window, seasonal items get a scheduling **boost** (weighted up, tasteful — not wall-to-wall); out-of-window, detected-seasonal items are **benched** (excluded). The bench is the half everyone forgets and the one viewers notice: *Christmas episodes in July break the spell.*
- **`mode: "exclusive"`:** the channel *is* the holiday (a December Hallmark-style channel): only in-window seasonal content airs; out of window the channel runs its `offSeason` fallback (loop scope without seasonal filter, or go dark — policy field, default loop).
- **`mode: "off"`:** no detection, no bench — for channels where a Halloween Simpsons episode in March is fine.
- Evaluation uses the container `TZ` wall-clock (main doc §9) at reconcile time; the periodic sweep naturally rolls channels into and out of windows within `CHANNEL_RECONCILE_EVERY` — seasonality needs no scheduler of its own.

## 6.5. Curation rules ("play different things at different times, like a real network")

The policy so far describes ONE deck that Tunarr loops forever — time-agnostic. Real
channels are wall-clock-conditional: weekend marathons, holiday programming, day-parts
(kids in the morning, drama at night). A **curation rule** is the unifying abstraction
— a `(WHEN, WHAT, HOW)` triple — and §5 ordering + §6 seasonality both compose into it
(seasonality *is* a `(when=holiday-window, what=keyword-match, how=boost)` rule).

**Seasonal-as-a-rule (the shared-calendar contract).** Seasonality (§6) is the *archetypal*
time-conditional rule and it is **unified with the rule engine at the calendar layer, not
rewritten as a generic rule** — a deliberate choice. `auto` mode does two asymmetric things
at once (bench out-of-window seasonal items **and** boost in-window ones by light
duplication) and `exclusive` mode has an `offSeason` fallback ladder (loop / dark); a
rule's intersect-only `What` and ordering-only `How` cannot express boost-by-duplication or
"bench items seasonal for a *different* holiday" without bloating the rule model. So the
seasonal *mechanism* (`applySeasonal`) stays intact, and the unification is that **the
holiday `When` predicate and the seasonal engine share ONE calendar** (`builtinCalendar`):
`When{Holiday:"christmas"}` is active *exactly* when the seasonal engine considers christmas
in-window — proven across every day of the year by a cross-consistency test, so the two can
never silently diverge. In `ComputeDesiredAt` they **compose**: a rule's `What` narrows the
pool first, then seasonal bench/boost runs on the narrowed set (so "a December holiday rule"
and seasonal `auto` reinforce rather than contradict). This keeps the seasonal regression
oracle green *by construction* — the mechanism is untouched; only the calendar is shared.

- **A rule = `{Priority, When, What, How, Window}`.** `When` is a deterministic,
  composable predicate (weekend/weekday, day-of-week, an hour range that wraps for
  late-night, a holiday-calendar id, a date range; all-zero = always-match). `What`
  **reuses `ScopePolicy`** and only ever *narrows* the eligible pool (never widens —
  it can't admit content the channel's own scope/audience excluded). `How` reuses the
  ordering + separation vocabulary plus `noBreaks` and a `marathon` sugar. `Window` is
  an optional per-rule override of the channel window (below).
- **Overlap resolution is first-match by (Priority desc, then list order)** — NOT a
  merge (merging two `What`s is unpredictable and can't be previewed). When several
  rules match a moment ("Saturday morning in December"), the highest-priority one wins;
  the natural default ordering mirrors a real programmer (holiday > weekend/daypart >
  base). When no rule matches, the channel falls through to its base whole-policy
  behavior. **Resolution is visible in the cycle preview** (§8, `?at=<time>`), so which
  rule is active at a given moment is answerable by looking, never a mystery.
- **Presets, not cron.** Users and the LLM compose from a closed, named vocabulary —
  WHEN: `weekend`/`weekday`, `mornings`/`primetime`/`late-night`, `holiday:christmas`;
  WHAT: `series:X`, `genre:kids`, `holiday-matched`, `all`; HOW: `marathon`,
  `syndication`, `shuffle`, `feature`. Every preset lowers to primitives that already
  exist (`marathon` = sequential + no breaks + unbounded block; `holiday-matched` = the
  §6 keyword engine; `syndication` = the §5 deck), so the engine is a composition +
  time-routing layer, not new scheduling math.
- **Rolling window.** A channel materializes only ~`Window` of runtime (default **24h**,
  per-channel/-rule overridable, global default `sched.window_hours`) rather than the
  whole ~800-episode run — so a channel schedules a manageable timeframe and is curated
  over time. The window START advances by a coarse time index folded into the deck seed
  (`floor(now/window)`), so it is **identical within a window** (idempotent reconcile —
  no re-push every sweep) and **advances at the boundary** (one re-push, the next slice
  of episodes). `Window: 0` = the whole run (the "full binge" sentinel; a `marathon`
  rule sets it). The window is floored to at least one program so a channel is never dark.
- **Authorship is hybrid (§8 boundary intact).** The **LLM proposes a starter rule set**
  from intent ("Star Trek with weekend TNG marathons and Christmas episodes") — from the
  closed preset vocabulary, grounded + clamped (unknown tokens dropped, window clamped to
  `[1h,168h]`, daypart audience ceilings **stricter-only** — a rule may never *raise* a
  kids channel's ceiling, §4). The **user refines** rules in the channel "Programming
  rules" editor (chips, drag-to-priority). The LLM still only proposes rule VALUES;
  deterministic code evaluates `When` and enforces — the model never orders episodes.
- **No new scheduler.** Rules evaluate against the container wall-clock at reconcile
  time; the periodic channel-sweep (§main-doc) already re-runs the pure lineup builder
  with a fresh `now`, so it *is* the refill loop — a rule/window boundary simply produces
  a different desired lineup on the next sweep. Because the desired lineup stays a pure
  function of `(seed, coarse-now, policy)`, reconcile idempotency holds within a slice.
- **Drift vs. rotation (a correctness rule).** A rule swapping the active pool (kids AM →
  drama PM) legitimately changes which programs air — this is NOT drift. Drift detection
  (§9 slot revalidation) therefore compares against the *eligible* set (what the library
  can currently supply), not the *selected* set, so "the library lost a title" (real
  drift → StatusDrifted) is cleanly separated from "the active rule rotated a title out"
  (normal, silent).
- **Backward compatible.** A channel with no rules and no window (`Rules==nil ∧ Window==0`)
  is byte-identical to today's behavior; existing channels are unaffected until a rule or
  window is set.
- **Known limitation — WHAT is series/movie-level, not episode-level.** A `holiday-matched`
  or `genre` WHAT filter resolves against a title's genres and (once threaded, §6-followup)
  keywords/overview — all of which live at the **series/movie** level. Neither our schedule
  entries nor TMDB carry episode-level thematic tags (TMDB has no episode `keywords`
  endpoint; keywords exist only on the series/movie). So a rule can say "in December, only
  series/movies tagged Christmas" but *cannot* precisely say "in December, only the Christmas
  *episodes* of an otherwise-evergreen series" — that would require per-episode `name`/
  `overview` text heuristics, logged as future work. A rule author (and the LLM prompt) must
  not promise episode-thematic precision the data can't back. Series-level and movie-level
  holiday matching *is* supported (the §6 keyword engine).

## 6.6. The preset lowering table (the closed authoring vocabulary)

The LLM and the UI author rules by composing **tokens** from a closed vocabulary; deterministic
code lowers each token to concrete `SchedulingRule` fields. The model never emits a raw
`WhenPredicate` or hour range — that would be the model doing scheduling math (§8 boundary
violation, unvalidatable). It emits `{when, what, how, priority?}` token strings; `groundPolicy`
lowers them, **drops unknown tokens**, and clamps. This is the exhaustive v1 table.

**WHEN tokens → `WhenPredicate`** (broadcast-standard dayparts, Eastern-style boundaries; evaluated against the container wall-clock):

| token | predicate | default priority |
| --- | --- | --- |
| `weekend` | Sat–Sun | 20 |
| `weekday` | Mon–Fri | 20 |
| `mornings` / `early-morning` | hours 6–10 | 30 |
| `daytime` | hours 10–17 | 30 |
| `primetime` | hours 20–23 | 40 |
| `late-night` | hours 23–2 (wraps) | 40 |
| `overnight` / `graveyard` | hours 2–6 | 40 |
| `holiday:<id>` | the §6 calendar window for `<id>` (christmas/halloween/thanksgiving/newyear/valentines) | 60 |
| `weekend-mornings` etc. | AND of the two (weekend ∧ mornings) | max of the two + 5 |

Priorities encode a real programmer's precedence: **holiday (60) > daypart-hours (30–40) > weekend/weekday (20) > base (0)**. A token with no match falls through to the base policy. Ties break by list order (§6.5).

**WHAT tokens → `*ScopePolicy`** (intersect-only, never widens; nil = inherit channel scope):

| token | scope |
| --- | --- |
| `all` | nil (no extra narrowing — the base) |
| `series:<key>` | `Series: [<key>]` (intersected with the channel's grounded picks; an ungrounded key is dropped) |
| `genre:<name>` | `Genres.Include: [<name>]` |
| `genre-not:<name>` | `Genres.Exclude: [<name>]` |
| `kids` / `family` | `Genres.Include` kid-safe genres **and** a stricter-only audience clamp (below) |
| `holiday-matched` | the §6 seasonal keyword filter (in-window seasonal items only) |
| `era:<from>-<to>` | `Era: {from,to}` (clamped) |

**HOW tokens → `RuleOrdering` + `Window`**:

| token | ordering | separation | breaks | window |
| --- | --- | --- | --- | --- |
| `syndication` | `OrderSyndication` (the §5 deck) | inherit | inherit | inherit |
| `shuffle` | `OrderShuffle` | inherit | inherit | inherit |
| `marathon` | `OrderSequential` | `BlockMax: 0` (unbounded — binge one show) | `NoBreaks: true` | `WindowFull` (don't truncate a binge) |
| `feature` | inherit | inherit | inherit | inherit + a light seasonal-style boost (future) |

Every token lowers to primitives that already exist — nothing here is new scheduling math. `marathon` = the three fields above (all present since Phase 1); `holiday-matched` = the §6 keyword engine; `syndication` = the §5 deck.

**Grounding + clamps (§8 / §4, in `groundPolicy`)** — the model proposes, deterministic code enforces:

- **Unknown token → dropped** (never a raw predicate). A rule that loses its WHEN/WHAT/HOW to drops degrades to the base policy — same failure contract as a dropped ordering.
- **Window clamp:** any per-rule window is clamped to `[1h, 168h]` (except the `marathon`/`WindowFull` sentinel).
- **Daypart audience ceiling is stricter-only (a §4 prime directive).** A `kids`/`family` WHAT may only *tighten* the channel's ceiling (e.g. TV-14 channel → TV-PG mornings), **never raise it** — a rule can never make a kids channel show adult content. Enforced at BOTH grounding (clamp the lowered ceiling to ≤ the channel ceiling) and enforcement (the §4 fail-closed gate is never bypassed by a rule). Defense in depth.
- **Series intersection:** a `series:<key>` WHAT is intersected with the channel's actually-grounded picks, so a rule can't scope to a series that never surfaced.

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
