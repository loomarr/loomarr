# Findings: channels that don't loop, and lineups that evolve (#1670, #1671)

Research lane, target beta.10. Measured on the demo library (isolated lane backend) and in an
offline simulator that drives the real scheduler; the household numbers come from the supervisor
running the read-only kit (§9). Nothing here changes the product. Base commit `33b35dbe`.

## TL;DR

1. **Most repetition today is not a curation problem. It is three scheduling defects plus pool
   arithmetic.** The defects:
   - Recency history re-sorts the deck *inside* a window, which rewrites the guide under a viewer.
     Reproduced live.
   - The rolling window turns over at 00:00 UTC and cuts a programme mid-way every day, which is
     20:00 US Eastern. Reproduced live.
   - Every channel has shuffle seed 0 and a single deck that never reshuffles, so each channel
     replays the same loop with the same adjacencies. Confirmed by reading the code and every
     channel's trace.
2. **Watching makes it worse.** In simulation, the recency feedback loop has these effects:
   - A 27-film channel's 24-hour repeat rate rises from 1% (nobody watching) to 26% (an evening
     viewer) and 70% (TV always on).
   - What airs differs from the guide forecast 71–92% of the time.
   - Programmes are cut mid-way 2–23 times a day.
3. **The winning mechanism is an append-only timeline with a per-episode airing ledger.** Each
   window takes the least-recently-aired units, and the deck is reshuffled every pass. Make it
   household-aware: rank what the household *watched* behind what merely aired.
   - An evening viewer's 7-day repeat rate falls from 76% to 13% on a 27-film channel, and to 0%
     on 60-film, sitcom and mixed channels.
   - Cuts and guide misses fall to zero.
   - No LLM is involved.
4. **Filler is already right, and the lever there is pool size.** V58 records exposure only while
   someone watches, and that memory is viewer-centric: an evening viewer sees 0% clip repeats
   within 24h even from a 40-clip pool. A schedule-wide filler ledger (my first hypothesis) made it
   *worse* (44%), so that idea is a dead end.
5. **With the LLM off, nothing grows a channel.** Scheduled re-curation and the adjacency corpus
   both run through a suggester refine. The only automatic growth is new episodes of series
   already in the lineup. Part 2 proposes a deterministic path that uses the same approval queue
   and the same binder.

## Part 1: repetition and serendipity (#1670)

### 1.1 The kit

Everything is under `project/evidence/curation-1670/`, behind the `research` build tag, so normal
builds, vet and lint never see it.

- `kit/`: one metrics function shared by both tools, so a live number and a simulated number mean
  the same thing.
- `measure/`: **read-only** HTTP. It calls `GET /v1/channels`, `GET /v1/channels/{id}/cycle?at=`
  (the pool, from the trace's placement facts) and `GET /v1/guide` one day at a time, including
  each break's pod.
  - ⚠ The guide is a forecast from current state. Recency and filler exposure are frozen at read
    time, so it measures "if nobody watches from now on".
- `sim/`: hour-by-hour replay through the real `schedule.ComputeDesiredAt`,
  `playout.BroadcastsBetween` and `filler.Assemble`. It models reconcile (hourly), `RecordAiring`
  while watched, and the fixed playout anchor. It uses six synthetic channel shapes (sizes and
  runtimes only) and three viewing patterns: nobody, evenings 19–23, always on.

Metrics (`kit/metrics.go`):

| metric | meaning |
|---|---|
| **R24h / R7d** | share of airings whose episode/film already aired in the previous 24h / 7d (the first 24h / 7d of the span only warm the lookback) |
| primeR7d | R7d over airings starting 19:00–23:00 local: what an evening viewer experiences |
| gap p10/p50/p90 | re-air interval percentiles, hours |
| adjRep | share of consecutive pairs A→B already seen earlier in the span: the "same loop again" texture |
| cover | distinct units aired ÷ pool |
| cuts/day | programmes that did not air to their end |
| EPG miss | share of 5-min samples where what aired ≠ the guide forecast made at the window start |
| C60m / C24h | share of clip plays whose clip played in the previous hour / day |

### 1.2 Baselines

**Demo library, live forecast, 14 days** (`demo-baseline.txt`). Channels 101–106 are the demo
seed. Channel 190 is the union of all demo lineups, made through the API to get a pool larger
than a day.

| ch | order | pool | pool length | R24h | gap p10 / p50 / p90 (h) | max airings/day | adjRep |
|---|---|---|---|---|---|---|---|
| 101 | shuffle | 36 | 6.6 h | 100% | 6.6 / 6.6 / 6.6 | 5 | 97% |
| 102 | shuffle | 21 | 7.7 h | 100% | 7.7 / 7.7 / 7.7 | 4 | 96% |
| 103 | sequential | 6 | 4.4 h | 100% | 4.4 / 4.4 / 4.4 | 7 | 97% |
| 104 | sequential | 4 | 2.0 h | 100% | 2.0 / 2.0 / 2.0 | 13 | 99% |
| 105 | shuffle | 30 | 11 h | 100% | 11 / 11 / 11 | 3 | 96% |
| 106 | shuffle | 20 | 8.1 h | 100% | 8.1 / 8.1 / 8.1 | 4 | 97% |
| 190 | shuffle | 117 | ~41 h | 19% | 16.9 / 41.2 / 44.0 | 2 | 86% |

All demo pools are shorter than a day, so each channel is a fixed-period loop: p10 = p50 = p90,
and nearly every adjacency repeats. No filler aired, because the demo pool had 0 eligible clips.
Filler numbers come from the simulator and the household run.

**Simulator, 14 days, current pipeline** (`sim-14d.txt`, excerpt):

| shape (pool) | viewing | R24h | R7d | primeR7d | adjRep | cuts/day | EPG miss |
|---|---|---|---|---|---|---|---|
| films-27 (50 h) | nobody | 1% | 100% | 67% | 69% | 0.9 | 0% |
| films-27 | evenings | 26% | 100% | 76% | 49% | 5.1 | 78% |
| films-27 | always-on | 70% | 100% | 87% | 35% | 21.5 | 90% |
| films-60 (112 h) | nobody | 2% | 100% | 26% | 53% | 1.0 | 0% |
| films-60 | evenings | 9% | 93% | 57% | 33% | 5.4 | 85% |
| sitcom-5×100 (183 h) | nobody | 0% | 43% | 2% | 43% | 0.9 | 0% |
| sitcom-5×100 | evenings | 7% | 59% | 17% | 2% | 3.6 | 73% |
| mixed 3×40 + 20 films (124 h) | evenings | 7% | 92% | 35% | 42% | 2.4 | 71% |
| kids-6×26 (29 h) | evenings | 44% | 100% | 70% | 15% | 3.2 | 80% |

### 1.3 Where the repetition comes from, ranked by evidence

1. **Pool arithmetic.** A channel airs 24h a day. With a pool of P hours, a unit comes back every
   ~P hours no matter how it is ordered. The floor on a viewer's 7-day repeat rate is
   `max(0, 1 − P / (7·W))`, where W is the hours a day they actually watch. Every demo channel is
   under this floor. No ordering can fix that; only more content, rules that widen the pool, or
   fewer advertised hours can. §3.1 of the programming design already says so honestly.
2. **Recency feedback re-sorts the deck mid-window (defect).** Measured in the sim (table above)
   and reproduced live (`evidence/curation-1670/live-repros.md` §A).
   - The mechanism: `RecordAiring` is keyed by title (a series key covers all its episodes), and
     every reconcile (every 5 min) passes the history to `byRecency`. Under syndication the history
     then goes through a fixed-seed shuffle, so the recency preference is lost *and* the deck is
     still permuted.
   - `windowSlice` then cuts the new deck at the same offset, and playout walks it from a fixed
     anchor. The result is a different programme at the same instant.
   - Consequences:
     - The tiling guarantee (every unit once per cycle) breaks.
     - Repeats rise with viewing.
     - The guide rewrites the block on air.
     - One tune-in pushed a whole series out of the next 3 hours.
   - Not verified: whether the running encoder switches mid-file or at its next resolve.
3. **Identical loops (defect plus doc drift).**
   - `Channel.Shuffle.Seed` is only ever set by `cmd/seed`, so all seven channels trace
     `seed: "0"`.
   - `syndicationDeck` emits ONE deck. The "reshuffle under `seed XOR deckIndex`" described in
     programming-design §5 and in its own header comment does not exist.
   - With `windowSlice` tiling one fixed deck, loop n+1 replays loop n pair for pair: adjRep is
     43–98% with nobody watching.
4. **Window-boundary cut (defect).** `windowIndex = unix / window`, so a 24h window turns at 00:00
   UTC (20:00 US Eastern daylight time). The new slice is entered mid-phase from the fixed anchor,
   and `segmentedBroadcasts` clips both sides. Live: a 22-minute episode cut at 16 minutes, and the
   next one joined 8 minutes in (§B). This happens about once a day on every channel whose pool is
   longer than its window (0.9–1.0 cuts/day in the sim).
5. **No growth with the LLM off.** `recurate.Runner` requeues a suggester refine per eligible
   channel. Adjacency candidates (§8.3) are only pre-seeded into that same LLM run. So on the
   household neither runs, and pools stay at their approval-day size apart from new episodes.
6. **Filler: pool size, not selection.** V58 rotation (new → oldest outside cooldown → oldest)
   over viewer-fed exposure gives these evening-viewer repeat rates:

   | clips | evening C24h | always-on C24h | max plays of one clip per day (always-on) |
   |---|---|---|---|
   | 40 | 0% | 98% | 5 |
   | 150 | 0% | 94% | 2 |
   | 600 | 0% | 0% | 1 |

   Selection is fine; an always-on household needs roughly `plays per day ÷ 1` clips to see no
   daily repeats, about 600 at 2 breaks/hour × 4 clips.
   - ⚠ `internal/filler/logrepeat.go` (named in the brief) is log-line throttling, not viewer
     repetition.
   - ⚠ Not measured: rules, marathons and seasonal windows. The demo seed has none. By
     construction a rule's WHAT narrows the pool, so within its window the floor in (1) rises, and
     a `marathon` sets `WindowFull`, which plays the whole narrowed deck in order and does not
     rotate. The household run will show whether this matters there.

### 1.4 Prior art

- **Linear practice.**
  - Stripping (the same show in the same slot daily), dayparts and rerun spacing are the textbook
    tools: Eastman & Ferguson, *Media Programming: Strategies and Practices*.
  - A network "rests" a film between runs rather than cycling it. That matches §8.2a's turnstile,
    minus the rest.
  - Viewers tolerate repeats they chose (a stripped sitcom) and dislike repeats they didn't (the
    same film twice in a week). Familiar-vs-new is a *placed* ratio, not an accident of the loop.
- **ErsatzTV.**
  - Playouts "individually track the ordered playback of collection items"
    ([docs](https://ersatztv.org/docs/scheduling/playouts/)). History persists across builds.
  - "Shuffle in order" keeps each show chronological while mixing shows
    ([classic schedules](https://ersatztv.org/docs/scheduling/classic/)).
  - Release notes fix "shuffle_sequence losing the shuffled order at the end of each build" and
    give "different blocks … different random seeds"
    ([v25.6.0](https://newreleases.io/project/github/ErsatzTV/ErsatzTV/release/v25.6.0)).
  - That is the same shape of defect as §1.3(3), fixed the same way: persistent playback state and
    a per-pass seed.
- **Tunarr.**
  - Per-clip and per-list cooldowns, plus an exponential "time waited" raffle so the clip played
    longest ago is favoured
    ([how filler is chosen](https://tunarr.com/configure/library/filler-selection/)).
  - Time/random slot tools for dayparts
    ([time slots](https://tunarr.com/configure/scheduling/time-slots/),
    [slot editor](https://tunarr.com/configure/scheduling/random-slots/)).
  - Loomarr's V58 rotation is the deterministic equivalent.
- **Recommender research.**
  - Novelty, diversity, serendipity and coverage as distinct beyond-accuracy objectives: Kaminskas
    & Bridge, "Diversity, Serendipity, Novelty, and Coverage", ACM TiiS 7(1), 2016.
  - Calibration (the mix shown should match the mix the person likes, not collapse to the top
    genre): Steck, "Calibrated Recommendations", RecSys 2018. For a channel this means keeping the
    channel's identity proportions.
  - Topic diversification: Ziegler et al., WWW 2005.
  - Explore/exploit with explanations: McInerney et al., "Explore, Exploit, and Explain", RecSys
    2018.
- **Playlists.**
  - Sequencing and transitions matter as much as selection: Bonnin & Jannach, "Automated
    Generation of Music Playlists", ACM CSUR 47(2), 2014.
  - Familiarity strongly drives choice, so a stable core plus rotating discoveries beats pure
    novelty: Ward, Goodman & Irwin, "The same old song", *Marketing Letters* 25, 2014.

### 1.5 What "serendipity" means operationally

Serendipity isn't directly measurable without the LLM or user feedback. It rests on three things
we *can* measure:

1. **Not seen recently.** Viewer repeat rate (R7d over what the household watched) is at or near
   its floor.
2. **Not in the same order.** Fresh adjacency: few repeated A→B pairs.
3. **Reachable depth.** Coverage with no starvation: every unit airs within
   `ceil(pool/window) + 1` windows, including the never-watched deep cuts.

The *delight* part, thematic threading and "forgotten gems", is an ordering preference that sits
on top of these. The LLM may enrich it (a reason line); it never gates it.

**Gates we can hold (kit-measured, 14-day sim plus household forecast):**

| gate | target |
|---|---|
| cuts/day | 0 (a published block is never truncated by rotation) |
| on-air block identity | never changes after it starts: EPG miss 0% at fixed history |
| R24h when pool ≥ 2 × window | 0% |
| viewer R7d (primeR7d for the evening pattern) | ≤ floor + 10 pts |
| adjRep, pool ≥ 3 × window | ≤ 25% |
| coverage | 100% within `ceil(pool/window) + 1` windows |
| filler, evening viewer C24h | 0% while the evening's clip plays < pool |

### 1.6 Mechanisms, ranked

| # | mechanism | measured effect (sim, 14 d) | cost | risk |
|---|---|---|---|---|
| 1 | **Freeze the arrangement per window.** Read history only at window commit, key it per episode/film, and give each channel a real seed plus `seed XOR pass`. | Removes the viewer-induced churn: EPG miss 71–92% → 0, cuts from 2–23/day down to the boundary's ~1. The "nobody" rows are the frozen-history proxy. adjRep falls with the per-pass reshuffle (measured inside #3). | S | Low. Existing tests pin determinism; history becomes a commit-time input. |
| 2 | **Append-only timeline with carry-over at the boundary.** The programme crossing the boundary finishes and the next batch starts after it. Align the boundary to `guide.timezone`, not UTC. | cuts/day 0.9–1.0 → 0 | M. The playout anchor becomes "end of the previous batch"; the guide walk follows. | Medium. Touches the shared walk that keeps the grid and the encoder in step; needs the same one-function discipline. |
| 3 | **Airing ledger + LRU batch rotation** (`ledger-lru`) | R24h 0% on every pool > 24 h. adjRep: films-27 69% → 17%, films-60 53% → 10%, mixed 60% → 8%. Coverage 100%. | M: a unit-level ledger table plus batch selection before placement | Low. Identity unchanged, same pool. |
| 4 | **Household-aware freshness** (`watched-lru`): watched units rank behind merely aired ones; never-watched go into the household's viewing hours | Evening primeR7d: films-27 76% → 13%, films-60 57% → 0%, sitcom 17% → 0%, mixed 35% → 0%, kids-6×26 70% → 58% | M: needs a per-unit watch record (§2.3) and the household's viewing hours | Low. The sim aligned batches only crudely to the evening, so these are conservative. |
| 5 | **Novelty budget per daypart:** operator-visible "primetime gets the least-seen; overnight takes the repeats" | Subsumed by #4 once viewing hours are known; as a rule preset it works with nobody watching | S on top of #3 | Low |
| 6 | **Thematic threading:** adjacent items share a facet (genre, era, collection, cast) at a set rate | Not measured; needs facets on units | M | Medium. Can fight separation and calibration. |
| 7 | **Filler: grow the pool, keep V58** | Pool size is the only lever (§1.3 item 6) | Pipeline work, not selection work | Low |
| ✗ | Filler exposure from every assembled break | Evening C24h 0% → 44% at 40 clips: worse | — | Dead end |

## Part 2: lineups that evolve (#1671)

### 2.1 The channel lifecycle

A channel moves through these states; it never mutates silently:

`Seeded → Growing → Established → (Resting / Refreshing / Seasonal arc) → Split | Merge | Retire`

- **Seeded:** the approved proposal.
- **Growing:** below the rotation target (¾ of the cap, §8.2a). It only adds.
- **Established:** at the target; the turnstile trades weakest for better.
- **Resting (new):** a title benched from rotation for N weeks after a full run. This is scheduler
  state in the ledger (#3), *not* a lineup removal, so it needs no approval and the binder stays
  the only writer.
- **Seasonal arc:** the existing seasonal engine overlays the pool; no change.
- **Split / merge / retire:** proposals only.

**What happens automatically, and what needs approval:**

| change | automatic? |
|---|---|
| Ordering, rotation, resting, daypart placement | Automatic. This is scheduling, not lineup. |
| New episodes of lineup series | Automatic (already true) |
| An in-library title that matches the channel's scope and ceiling, below the target | Automatic only for channels that opted into AutoCurate (today's rule); otherwise a proposal |
| Any acquisition, any removal of an available title, or any change to name, scope, audience or rules | Approval, always. Audience ceilings never loosen. |
| Split, merge, retire | Approval, always |

### 2.2 The lineup as a whole

Deterministic, LLM-free signals over the library and the channel set:

- **Gaps:** library facets (genre × era × rating) with runtime that no channel's scope admits.
  These are candidates for a new channel. It could feed #1665 channel ideas directly.
- **Overlap:** the Jaccard share of a channel pair's pools. Above a threshold, propose a merge or
  a differentiating rule. Seed 0 on every channel currently makes overlapping channels air
  overlapping content in correlated order.
- **Daypart coverage:** the household's viewing hours (§2.3) against the channels with kids or
  primetime rules.
- **Retire a channel:** it goes unwatched (tune-ins) for N weeks. Growing never needs a trigger
  from viewing.

### 2.3 Signals: what exists today (checked on the lane backend)

| signal | status |
|---|---|
| Airing history (`airings`) | One row per channel × *title key*, last airing only, written only while watched. No API read. It feeds recency, and is the source of defect §1.3(2). |
| Filler exposure (V58) | Per clip × channel play count + last played, viewer-fed. Good. |
| Tune-ins and dwell | `GET /v1/playout/sessions` is a live snapshot only; nothing is persisted. **Missing.** |
| Media-server play state | Not read: the inventory fetch sets `EnableUserData=false`. Available if we opt in: per-user watched and last played. This is the cheapest household-freshness source. |
| Library growth | The library scan exists. Per-title "date added" was not checked in this lane. |
| Requests | The Seerr queue poll exists. |
| Seasonal calendar | `builtinCalendar` + `holidayvocab` (shared, tested). |

The minimum new signal is a **unit-level watch log**: channel, unit, start, and seconds watched,
written by playout. It serves the ledger (#3), household freshness (#4), channel retirement and
dwell. Media-server play state is a complementary source for what was watched outside Loomarr.

### 2.4 Using the existing paths: no second lineup writer

- **Deterministic growth proposal (LLM-free).** A job builds a `Proposal` from:
  - in-library titles that match the channel's scope, ceiling and era, ranked by facet similarity
    to the current lineup;
  - TMDB adjacency consensus (§8.3), run *without* the model;
  - the channel's own never-aired or rested stats.

  It goes into the existing approval queue. AutoCurate channels auto-approve in-library adds
  under the target, exactly as today. The binder applies every change.
- **With the LLM on,** the same proposal gets a reason line and can reorder candidates. It never
  adds a candidate the deterministic step did not surface: grounding stays the chokepoint.
- **Diff-shaped proposals.** `+6 titles, −2, rested 3, why: never aired in 9 weeks / matches era
  1980–89 / 4 of 5 lineup neighbours recommend it`. The reasons are templated from the facts that
  produced them.
- **History and pace.**
  - A per-channel change log comes from `Proposal` + binder applies; the binder is already the
    single point.
  - A pace limit: at most k% of the pool changed per week. Identity stays recognisable because
    scope and rules are unchanged, and calibration keeps genre proportions within ±10 points.

**UX in words (the maintainer makes the mocks):**

- The channel page gets an "Evolving" strip: the last change, the next proposal, and the reason
  line.
- Proposals read as a diff with per-row reasons, one tap to reject a row, and "Undo" on the change
  log.
- Home shows "New on your channels this week" drawn from never-aired units.

## 3. Phased plan with gates

| phase | scope | gate |
|---|---|---|
| **P1: correctness (beta.10)** | Real per-channel seed + per-pass reshuffle; history read only at window commit, keyed per unit; window boundary on `guide.timezone` with carry-over | kit sim: cuts/day = 0, EPG miss = 0 under all viewing patterns; adjRep ≤ 25% for pools ≥ 3 × window; live repro A and B no longer reproduce; existing determinism tests green |
| **P2: ledger rotation** | Unit-level airing ledger + LRU batch; watch log from playout | R24h 0% for pools > 24 h; coverage 100% within `ceil(pool/window) + 1`; household forecast re-measured with the same command |
| **P3: household freshness + daypart novelty** | Watched-rank, viewing-hours placement, optional media-server play state | Viewer R7d within floor + 10 pts on the household |
| **P4: deterministic evolution (#1671)** | LLM-free growth proposals, diff UX, change log, pace limit, resting, gap/overlap proposals | No lineup write outside the binder (no test pins this today; add one with P4); ≤ k%/week churn; identity check (genre mix within ±10 pts) |

## 4. Open questions for the maintainer

1. **Fixed epoch vs. append-only.** Mechanism #2 changes how the guide maps to the clock. It stays
   one shared walk, but the anchor moves each window. Is that acceptable for Tunarr-backed
   channels too, or internal playout only?
2. **What "aired" means with on-demand playout.** Nothing encodes when nobody watches. Should the
   ledger count scheduled airings (the linear illusion: the guide said it aired) or only watched
   ones? The sim says both matter: aired for tiling, watched for freshness.
3. **Media-server play state.** OK to read per-user watched data (`EnableUserData`)? It is the
   cheapest household-freshness signal, but it is per user, not per household.
4. **Resting and pace.** Defaults for the rest length (e.g. 4 weeks after a full run) and the
   weekly change cap (e.g. 15% of the pool).

## 5. Household measurement (the supervisor runs it; read-only)

```
cd <a checkout of this branch>
LOOMARR_TOKEN=<the install's API token> \
  go run -tags research ./project/evidence/curation-1670/measure \
  -base <household base URL> -days 14 -out /tmp/curation-household.json
```

This issues GETs only: `/v1/channels`, `/v1/channels/{id}/cycle`, and 14 × `/v1/guide` a quarter
of a second apart. Each guide call recomputes every channel and assembles each break's pod, the
same work as scrolling the guide a day at a time.

Reproduce the rest: `go run -tags research ./project/evidence/curation-1670/sim -days 14`.
