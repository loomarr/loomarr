# Scheduling

Formerly `design.md` §9 (the scheduler), §5's airing history, and §10's break and pod policy and
break placement. The scheduler turns an approved Proposal and live availability into a durable,
filled channel on its playout backend, and keeps it that way. Everything upstream exists to feed
it. [`programming-design.md`](../programming-design.md) is authoritative for ChannelPolicy, the
relaxation ladder, separation, seasonality and ordering; this doc covers the machinery around them.

## Responsibilities

- Own each **Channel**: intent reference, number, name, logo, group, strategy, status, filler
  policy, and an optional Tunarr projection id.
- Compute **desired programming** from the approved lineup, and reconcile it for every active
  channel. Only when Tunarr is the effective backend does reconcile also push desired → remote
  actual, idempotently and with a minimal diff.
- Run the **backfill loop** so a channel is live at once and improves as content lands.

The scheduler, lineup, pods, ladder, determinism and approval gate are backend-agnostic. A playout
backend decides how bytes reach the television, never what plays or in what order; the same lineup
produces the same schedule on either backend, which is what makes the backend choice safe to
change per channel.

## Domain

- **`DesiredLineup`** is an ordered list of `Slot`s: `program` (a library item, once available),
  `pending` (awaiting acquisition) or `filler`/`flex`.
- The `DesiredLineup` persisted on the channel row is the **accepted broadcast snapshot**. Internal
  playout and the guide read it directly. `CyclePreview` is only the authoring and forecast surface:
  it depends on mutable airing history and availability, so recomputing it at playout time could
  move the same wall clock into an unrelated episode.
- Each channel row carries one **playout anchor**, stamped when a building or empty channel first
  goes live, so a first tune starts near the first accepted programme. Lineup edits, backend
  changes and restarts preserve it. It moves only when the **rolling window turns** (#1675).
- **Rolling windows turn with carry-over.** Windows are laid on the wall clock of `guide.timezone`
  (else the container's zone), so a daily window turns at local midnight, and a DST day is 23 or 25
  hours long. The programme on air across the boundary finishes on the arrangement it began in;
  the next window's slice starts at its end, and that end becomes the new anchor. One rule,
  `playout.WindowTurn`, is shared by reconcile, which commits the new cycle and anchor, and the
  playout resolver, which airs the next window from that end even before reconcile has run by
  arranging it with `CyclePreview`. The guide chains windows at the same ends, so no block is
  clipped at a boundary. If reconcile missed more than one window, the current window is walked
  from its own opening, and that one turn may join a programme part-way. Tunarr-backed channels
  cannot carry over: Tunarr loops the list it was given from its own clock and cannot be told
  where a window starts, so their anchor stays fixed and their guide still cuts at the boundary
  (#1691).
- **Availability resolution** turns an approved entry's key into `(library item id, duration,
  available)`. Duration comes from the media server's `RunTimeTicks`; a program slot always has a
  real `duration > 0`.
- **Series expansion.** A series entry expands at resolution into one program slot per episode that
  exists now, ordered by the strategy (season/episode order, or shuffled with the channel seed). The
  optional `SeasonMin`/`SeasonMax` window on the entry (inclusive, 0 = open) filters episodes and
  survives re-syncs; nothing in range yet means a `pending` slot. The episode cache rules are in
  [`library.md`](library.md#cached-series-episodes).

**Strategies** are shared by both backends: sequential, shuffle, and time-slot blocks.
**A shuffle is the channel's own and is dealt afresh each pass (#1676):** the seed is the stored one,
or, when a channel stores none, a hash of its id, so two channels over one pool differ; each trip of
the rolling window around the catalog is a new permutation, and the seam between trips keeps every
unit at least a repeat spacing from its last airing: never twice in a row, and, for a pool wider
than a window, never twice in one window. The rule and its one
bounded relaxation are in [programming-design §6.5](../programming-design.md).
**Ordering has one operator knob:** per-rule `How.Ordering` (within its window) >
`policy.ordering` > `Channel.Strategy`, the create-time default used only when `policy.ordering` is
unset. Omitted policy fields resolve to built-in Go constants; per-channel policy is the only
operator override.

**The Play order control explains its consequence.** A choice of order has a size: a channel set to
"In order" over 226 episodes loops front to back about every 3½ days, and nothing used to say so.
The cycle preview therefore carries `cycle` (`episodes`, `movies`, `series`, `runtimeMs`): one full
pass through the deck, counted before the rolling-window slice, from the same computation that ships
the guide. Pending titles are not counted and commercial breaks are not in the runtime. The
Programming page shows a plain-language line under Play order built only from those numbers and the
resolved ordering ("226 episodes, start to finish, then it starts over — about every 3½ days");
`syndication` is always described as a deck dealt once before anything repeats. Copy never claims a
reshuffle on each pass, so it holds whether or not the shuffle reseeds per pass.

## Reconciliation and backfill

- **On approval** the channel is built from the selected items that are available now, and the rest
  of the timeline is filled so it is live immediately. The default pending-slot policy is pod-fill; a
  "coming soon" card is the alternative. Unselected suggestions never silently enter the channel.
- **The fallback pool** is the channel's already-available selected titles (looped) plus its filler.
- **Backfill is stable:** a landed title fills its pending slot in place; a live channel is never
  globally reshuffled.
- The lineup is built under ChannelPolicy: hard filters (scope, fail-closed audience, seasonal
  bench), then seeded slotting, then the relaxation ladder (recorded and surfaced; audience and scope
  never relax), then pods. Separation holds across the cycle seam. The audience filter's exclusion
  report (`{overCeiling, unrated, items}`) is shown at review and at reconcile.
- **Reconcile is backend-neutral and idempotent.** Every active channel recomputes and persists its
  desired lineup, applied policy, status, healed metadata and next deadline. An internal channel
  stops there; a Tunarr channel also diffs and patches the remote channel. `POST
  /v1/channels/{id}/reconcile` is safe any time.
- **The periodic sweep** (`CHANNEL_RECONCILE_EVERY`, default `10m`) makes events a latency
  optimisation only: it survives a crash between event and convergence and makes PostgreSQL replicas
  correct without cross-instance events. It also revalidates every program slot against the library,
  so a vanished item falls back and the channel is flagged.
- **A zero `reconcile_deadline` means due now,** and the sweep's claim never excludes it; otherwise a
  channel whose first reconcile failed would be invisible forever. Opt-outs (`paused`, `detached`)
  are named in the status filter, never encoded as a magic deadline. A failed first reconcile is
  logged at ERROR and written to `activity`.
- **Channel numbers must be free in Tunarr too.** Every path that assigns a number (approval, create,
  renumber) asks `binder.NumberInUse`, which unions the store with Tunarr's channel list. On a
  collision Loomarr renumbers its own channel, never the occupant, and reports the move. An
  unreachable Tunarr falls back to store-only numbering.
- **Ownership:** Loomarr is authoritative for local desired state and, on Tunarr channels, for the
  managed projection; edits in Tunarr's UI are overwritten, and the UI says "Managed by Loomarr".
  Channels Loomarr did not create are never touched. Moving a channel to internal playout keeps its
  historical `tunarr_id` and makes no remote calls; purge is the only destructive operation.
- **Time zones:** time-slot schedules use the container's `TZ` and are wall-clock across DST.
  Per-channel time zones are #1576.

## Guide freshness

After a reconcile that creates, renames or deletes channels, the scheduler pokes the media server
(best-effort, idempotent). The two operations differ:

- **Tuner re-scan** (re-saving the M3U tuner host, `POST /LiveTv/TunerHosts`) re-reads the channel
  list. A new or removed channel needs this.
- **Guide refresh** (the `RefreshGuide` task) updates programme data for channels the server
  already knows. A lineup change on an existing channel needs only this.

## Tonight's highlights

`GET /v1/guide/highlights?from&to&limit` picks the few airings Home's **Tonight** calls out
(#1664), from the same real-airtime walk the XMLTV guide uses (no pending placeholders). Each
carries a typed `reason` the client words; no client ranks airings. What counts is premieres and
marathons (#1659 H3); movies and "new on this channel" don't.

- A **run** is one show's episodes back to back on a Channel. Breaks and dead air inside it don't
  end it; any other programme does.
- A run holding an episode 1 of season 1 or later is a `series_premiere` (season 1) or
  `season_premiere`. Specials (season 0) are not premieres. When three or more episodes follow from
  it, the premiere also carries the run (`episodes`, `untilMs`).
- Premieres count only where the Channel airs episodes in order at that airing's time
  (`schedule.OrderingAt`: the active rule's ordering, else the policy's, else the strategy's).
  Only `sequential` qualifies: `shuffle` and `syndication` (the default) deal a deck, so an episode 1
  there is chance. A run on such a Channel can still be a marathon.
- Otherwise a run of three or more is a `marathon`.
- One highlight per show on a Channel: a Channel looping a short pool airs the same premiere or
  marathon every cycle, and the rerun is not news, so the show's best-ranked one stands. The same
  show on another Channel is its own highlight.
- One highlight per run. Premieres outrank marathons and longer marathons outrank shorter; every
  Channel gets one before any gets a second, and the chosen few come back in airtime order.
  Paused and detached Channels are skipped.

The window defaults to six hours from now and is capped at 24; the client sends "tonight" in its
own clock. Artwork is resolved only for the chosen airings, in one batch.

## Airing history

`airings` records one row per airing of a unit, an episode or a film
(`{channel_id, library_item_id, aired_at, recorded_at, key}`), written by the playout resolver when
it resolves a programme for streaming. Re-resolving the same airing writes nothing new, so
`recorded_at` is when the airing was first observed. The write is best-effort: a failed insert is
logged and the programme still airs. Each write prunes that unit's rows older than eight days down
to the newest of them, so the table stays bounded by lineup size. It is Loomarr's own broadcast
record, not viewer watch state.

`LastAiredByChannel(channel, before)` returns the latest airing per unit among rows recorded
strictly before `before`. Placement passes the start of the rolling window it is arranging, so a
window's history is fixed when the window opens (#1674). A tune-in during the window, including one
that first records a programme which started before the boundary, shapes the next window and
cannot re-arrange the one on air. A channel on an unbounded window never opens a new window, so it
reads no history.

Recency is a **soft ranking signal**, not a constraint (`programming-design.md` §3.1). A 24-hour day
consumes about 13 films, so a week without repeats needs about 168 hours of content; a hard
constraint would relax on nearly every real channel and teach operators to ignore the ladder. The
signal spreads airings evenly; only more content fixes frequency.

On a shuffled channel wider than its window, recency trades next-day repeats out of the window's
tile and holds the window to the repeat spacing. What the window before aired comes from the record
where it has one and otherwise from that window's tile, less the units its own guard traded out
(#1939). History therefore never brings a unit back sooner, or keeps one waiting longer, than the
same channel with no history.

## Breaks and pods

A break is an **ad pod**: intro bumper, 2–4 matched commercials, return bumper, sized to the gap.

- **Matching:** country and local market first, then era to the block, audience to the channel, and
  category variety within a pod.
- **Per-channel selection (`policy.filler`, `FillerSelection`)** narrows `era`, `audience`,
  `categories`, `kinds`, `pinned` and `excluded`. `era` has three states: unset inherits the era
  `policy.scope.dates` implies, `{0,0}` is explicitly any, and a range matches both bounds. Exclusion wins
  over a pin. Optional `geography` may choose a market but never change the installation country,
  and is applied before pins. On first approval of a generated channel, one pure function seeds the
  selection from the Proposal (the era its scope dates imply; `TV-Y`/`TV-Y7` → `kids`, `G`…`TV-PG` → `family`, anything
  else → `general`); after that the operator owns it and refine or re-curation never overwrite it.
- **Derived context.** Where the operator left audience or era unset, `schedule.DeriveFiller`
  (applied in `channels.SelectionFrom`) derives them live: audience from the ceiling, else the
  highest rating in the lineup; era from the scope dates, else the lineup's release-year span.
- **Four separate decisions:** acquisition authorisation, catalog admission (terminal certified
  evidence only), deterministic channel matching, and per-channel pins and exclusions. None
  substitutes for another. A clip's eligibility change wakes only the affected channels, through the
  same `SelectionForChannel` and `FitForChannel` predicates coverage, preview and playout use;
  the sweep remains the crash-safe retry.
- **Density.** `FILLER_BREAK_DURATION` defaults to 2 minutes (#1670: 30 s breaks felt too short;
  `policy.breakDuration` per channel,
  minimum 30 s because Tunarr clamps smaller flex gaps). `policy.breaksPerHour = 0` is the only off
  switch. On internal playout the break length is a ceiling: a break slot is capped at its pod's
  playable duration, so the programme resumes when the clips end. That pod is sized from the
  catalog, the selection and the policy alone, **never the play history**: sized from the live
  rotation, every commercial that aired re-sized every break, and the timeline walked from the
  anchor moved by tens of seconds to minutes at each reconcile (2026-10-24). Each break that airs
  is then assembled to its own slot's length, which is its hard budget. **One function owns the length**
  (`filler.ResolveBreakDurationMs`): the channel's value when it is at least 30 s, else the install
  setting, else 2 minutes. The scheduler cuts the slot with it and the pod is assembled to it, so
  the two cannot disagree. **The break is a hard budget:** the intro bumper, the ads and the return
  bumper together never exceed it, because the slot is cut at exactly the break length and anything
  past it is lost. Assembly reserves the return bumper's real length before it places an ad, and
  an ad is never cut to fit.
- **A break with ads beats a break with bumpers.** If the bookends leave less room than the
  shortest ad, assembly drops the return bumper, then the intro bumper, before it gives up: a
  30 s break of 30 s ads still airs the ad. Only when no ad fits even alone is the break the 5 s
  card, and the channel then airs no breaks (no playable file).
- **Breaks use their time.** After the ads, assembly tops the break up with whole clips that fit
  what is left, longest first: a short ad, a station ID or a bumper, ahead of the return bumper.
  The top-up draws on the same ladder pools (audience ceiling, era, geography, quality and duration
  bounds), the same no-repeat set and the same rotation and spacing rules as the ads, with category
  variety relaxed rather than time left unused. It stops at the first moment nothing unused fits,
  so a break ends within the shortest unused clip of its slot, not 18 s early. `FILLER_POD_MAX` is
  a floor, never the reason a fitting clip is left out: the adapter sizes the clip ceiling from the
  break length over the shortest eligible clip (at most 64). Time, not count, bounds the pod.
- **A re-size never moves what is on air.** Every airing is found by walking the accepted cycle
  from its anchor, so a break that has aired keeps its length. On a reconcile that re-sizes breaks
  (the catalog changed), each break that has aired or starts within `channels.MidRollFreezeHorizon`
  keeps its accepted length, and only later breaks take the new one (`playout.KeepAcceptedBreaks`).
  The hold runs up to the first slot whose timing changed (a programme added, removed or
  re-timed), so what airs before an edit stays put; a change that moves nothing (a re-identified
  file, a healed title) does not end it. It never outlasts the break length: a shorter
  `breakDuration` is an edit and applies to every break at once, aired ones included, so it can
  move what is on air and restart the stream once. A smaller catalog stays held,
  and those breaks air what fits, then the card. A placeholder (a keyed filler slot at a missing
  programme's runtime) is not a break: its pod keeps the configured break length.
- **A schedule change restarts a stream only when it changes what is on air.** The packager reads
  the accepted cycle at every item boundary, so a change from its next lookup on reaches the stream
  by itself. Reconcile stops a channel's packager only when the new cycle airs something different
  over the packager's run-ahead (`playout.OnAirChanged`), and Postgres peer replicas apply the same
  rule to each committed version. A rolling-window turn keeps stop-on-change.
- **Previews are the break that airs.** The saved preview, the guide's per-break cards and both
  draft previews (`pods/preview`, `programming/preview`) build their selection through
  `channels.SelectionForPolicy`, which carries the policy's `breakDuration` and
  `commercialSpacing`. A draft that does not carry spacing uses the channel's saved value. The
  guide's card for a break is assembled to that break's slot, exactly as playout assembles it.
- **Placement** walks program durations and inserts a break after the program that crosses each
  `60 / breaksPerHour` minute threshold. Breaks are inserted only when a filler pool exists; with
  none, programmes play back to back and the next reconcile re-inserts breaks once clips land.
  Positions are deterministic for a lineup and seed.
- **Rotation (V58, household-wide since #2017).** The history is the household's, not a channel's:
  a commercial seen on any channel rests on every channel. One row per watched airing
  (`filler_airings`: channel, clip, scheduled start, recorded at) keeps every airing younger than a
  day and each clip's newest four beyond that, so it stays bounded. Internal playout writes it when
  the channel encoder resolves the clip, keyed by scheduled start, so tune-ins and rebuilds cannot
  inflate it; preview and reconcile never write it.
  **Only watched airings count:** a packager a neighbour warm keeps ready for a channel switch airs
  to nobody, and its lookups carry `playout.WithUnwatched` while no viewer (browser player or tuner)
  holds it or released it in the last 10 seconds. A viewer who switches to a warm channel mid-clip
  misses that clip's count, the cheaper error. Per-channel history let one TV switching channels
  see the same clips in the same order on two channels within the hour, and warm neighbours rested
  clips nobody saw.

  Assembly reads a snapshot of airings that aired before the break starts **and** were recorded at
  least a settle (`OnAirLookahead` + 3 s) before it. The second cut keeps a break fixed while it
  airs across channels: a viewer tuning in mid-clip elsewhere records an airing that started
  minutes earlier, at that moment, and the break's pod is assembled again at every clip's resolve.
  Playout resolves a break's first clip no earlier than its run-ahead before the break, so every
  airing a break counts was stored before its first clip was assembled. Within each rung it ranks:
  never aired, then least recently aired outside the channel's commercial spacing. The end-of-break
  top-up ranks the same way, the longer clip winning only among equals. A clip inside its spacing
  ranks behind every rested clip on any rung, so the ladder widens era before a clip repeats, and
  spacing relaxes only when the whole ladder is resting (the pod records `CooldownRelaxed`).
  Spacing never causes dead air. Pins come first and may repeat. **Each channel sets its own
  spacing (#1670), measured against the household's airings:**
  `policy.commercialSpacing` ("Repeat spacing · same commercial" in the channel's Filler section);
  nil inherits `filler.cooldown_seconds`, which defaults to 4 hours, and a present `0` lets a clip
  repeat in any later break. On Tunarr channels Tunarr owns rotation; Loomarr writes the channel's
  effective spacing as `fillerRepeatCooldown` and leaves the list cooldown at zero. Tunarr makes a
  clip unavailable for the whole cooldown and Loomarr's list is a pod's worth of clips, so the value
  is capped at the time the channel takes to air half the list (breaks per hour × break length against
  the list's runtime). A longer spacing would leave every clip cooling and Tunarr would fall back to
  flex. A larger list earns a longer spacing. Loomarr's own playout ranks by recency instead, so it
  honours the full spacing and relaxes it only when the whole ladder is resting.
- **Fallback ladder:** exact era → era widened by a decade → any audience-appropriate clip → clips
  with an ungrounded audience → the channel's bumper card. Never dead air. A pod fills from the
  tightest rung first and tops up from the rungs below it, so a small exact rung leads the break
  instead of being the whole break (#1684). The coverage level reports the rung breaks reach.

**Audience is an allowlist and never weakens** (a kids and teen guardrail):

- A `kids` channel admits only clips grounded `kids` or `family`, on every rung.
- An ungrounded-audience clip is admitted only to `general` or `late_night` channels.
- A new audience value admits nothing until someone decides it should.

Loomarr ships an embedded default bumper card and sets it as each Tunarr channel's fallback at
creation; operators can replace it per channel.

## Break placement by backend

| Backend | Where breaks go |
| --- | --- |
| Tunarr | Between programmes only, through a Loomarr-managed filler list attached to the channel and the flex gaps the scheduler leaves. |
| Loomarr (internal) | Between programmes and mid-programme. |

**Mid-programme breaks** are on by default on internal channels, with a per-channel off switch
`policy.midRoll` (decision [0034](decisions/0034-midroll-default-on.md)).

- Each source is measured once per revision: container chapters first, kept only when a one-second
  check reads black on both sides (YAVG ≤ 24 on 8-bit limited range) with no loud programme audio
  (mean ≤ −25 dB); otherwise a targeted black-and-silence search around each quarter hour. An
  unmeasured long programme is queued on the low-priority measurement worker and airs whole meanwhile.
- `schedule.PlaceMidRollCuts` is pure. The breaks-per-hour cadence runs through programmes; only
  programmes of 40 minutes or more split. A due break takes the measured fade (confidence ≥ 0.25)
  nearest its due point within ±5 minutes (`inventory.BreakSearchHalfWindow`), never leaving a part
  under 8 minutes. **No fade in the window means no break**, never a forced cut.
- A split programme airs as `Segment` parts with `SourceOffsetMs`; the seek flows through
  `Airing.Offset` into the packager. Programmes on air or starting within
  `channels.MidRollFreezeHorizon` (30 minutes) keep their accepted split (`PinnedCuts`).
- The guide shows a split programme as **one** entry spanning its parts and the breaks inside.
- `breaksPerHour = 0`, no filler pool, or a marathon rule (`NoBreaks`) means no mid-roll either.

## Tests that pin this

Formerly `design.md` §19.

- **Scheduler reconcile** against a mock Tunarr: idempotent (a second reconcile is a no-op),
  minimal-diff, and backfill (a pending slot filled with filler becomes the real title on
  `available` and is re-pushed; `unavailable` substitutes). Dropping the availability event entirely
  still backfills through the periodic sweep. One leader per channel under concurrency.
- **Pods** are seeded-deterministic (seed = channel + window) and respect era and audience matching,
  category variety, density and no-repeat-in-window; the fallback ladder degrades to a bumper card;
  filler never appears as a lineup programme; only real catalog clips are placed.
