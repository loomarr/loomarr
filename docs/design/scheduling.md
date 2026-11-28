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
- Each channel row carries one **playout anchor**, to the millisecond, stamped when a building or
  empty channel first goes live, so a first tune starts near the first accepted programme. Backend
  changes and restarts preserve it. It moves when the **rolling window turns** (#1675), and when a
  re-deal inside a window starts the cycle again at the programme on air (below).
- **Nothing that has aired moves inside a window** (#4140). The channel row records what the
  accepted cycle was dealt from (`dealt_from`): what an operator decided (lineup titles and their
  season and episode choices, the channel's policy, its ordering,
  the install's break, window and calendar settings, and whether the channel has filler to air at
  all, since that decides whether it has breaks) and the deal itself. Library facts are not decisions:
  a re-read runtime, a second file for an episode (a remux upgrade), an added episode, a healed
  rating or collection.
  - **The decision half is fleet-wide state.** Every internal channel stores it, so a change to
    what it hashes, or to how a hashed type encodes, re-deals every channel after the programme on
    air at the first sweep after deploy. A new policy field keeps it only with `omitempty` and a
    zero default. A field without `omitempty`, or a non-pointer struct field (which `omitempty`
    never omits), changes every channel's digest once. A field reconcile writes back belongs
    outside the decided policy, or every library change reads as an edit.
  - **The forecast cache key is a different hash.** The guide caches each channel's forecast
    under a key the engine computes from the same settings snapshot the forecast reads: the
    channel id, the lineup and policy whole (library facts and written-back findings included,
    because the forecast reads them), the ordering strategy and shuffle seed, the creation time,
    the playout anchor, the accepted cycle, the install's break, window, calendar and backend
    settings, and the active rule and holidays at that instant. Anything else on the row is either
    derived during assembly or not a forecast input, and a test classifies every field. Inputs
    that are not on the row (availability, the filler pool, airing history, natural-break
    measurements) are bounded by the cache's five-minute TTL. The row's revision is not the key,
    because every reconcile save moves it.
  - **Library facts wait for the next window.** While the decisions are unchanged, reconcile keeps
    the accepted cycle and only heals, in place and at the same runtime, a programme that has not
    finished airing and whose file vanished: a re-identified file (the same film, or the same
    season and episode) takes the new item, and anything else becomes a placeholder that airs
    filler. A placeholder whose title resolves again becomes the programme again the same way, so
    a mount that drops out for one sweep costs that sweep only. On the install, a series rewritten through the day re-dealt the window at every
    reconcile; the programme on air changed under the viewer and the scheduled ledger, walking
    each new cycle from the anchor, recorded programmes that never aired.
  - **An edit applies at once, after the programme on air** (`Engine.windowDeal`). The programme on
    air and anything starting within the packager's run-ahead keep their times, with the break
    after each; the rest is the new deal without the programmes that aired since the window opened.
    When the new cycle airs to the window's end without looping, everything that aired is kept
    ahead of it and the anchor stays (`timeline.KeepOnAir`), so the guide still shows the earlier
    hours as they aired. Otherwise (a pool shorter than the window) the cycle starts at the
    programme on air, which becomes the anchor. An edit that leaves the programme on air
    ineligible, or the channel nothing to air, takes effect from the anchor at once.
  - **A window that never turns** has no next window to wait for, so a library change that changes
    the deal re-deals after the programme on air, once: the next reconcile deals the same and keeps
    it.
- **Rolling windows turn with carry-over.** Windows are laid on the wall clock of `guide.timezone`
  (else the container's zone), so a daily window turns at local midnight, and a DST day is 23 or 25
  hours long. The programme on air across the boundary finishes on the arrangement it began in;
  the next window's slice starts at its end, and that end becomes the new anchor. One rule,
  `timeline.WindowTurn`, is shared by reconcile, which commits the new cycle and anchor, and the
  playout resolver, which airs the next window from that end even before reconcile has run by
  arranging it with `CyclePreview`. The guide chains windows at the same ends, so no block is
  clipped at a boundary. If reconcile missed more than one window, the current window is walked
  from its own opening, and that one turn may join a programme part-way.
- **Tunarr channels cut at the window boundary, by decision** (#1691). Internal playout carries the
  programme on air over the rolling-window turn; Tunarr-backed channels do not. Tunarr loops the
  list it was given from its own clock and cannot be told where a window starts or at what offset to
  join the new list, so their anchor stays fixed (`channels.Engine.CarriesOver` is false), their guide
  cuts at the boundary (`fixedAnchorLegs`), and the programme airing there can be cut off at local
  midnight in `guide.timezone`, depending on when reconcile pushes the new list relative to Tunarr's
  loop. This is accepted rather than worked around: internal playout is the default backend, and
  delaying the push or offsetting the list would depend on Tunarr API behaviour Loomarr does not
  control.
- **Availability resolution** turns an approved entry's key into `(library item id, duration,
  available)`. Duration comes from the media server's `RunTimeTicks`; a program slot always has a
  real `duration > 0`.
- **Series expansion.** A series entry expands at resolution into one program slot per episode that
  exists now, ordered by the strategy (season/episode order, or shuffled with the channel seed). The
  optional `SeasonMin`/`SeasonMax` window on the entry (inclusive, 0 = open) filters episodes and
  survives re-syncs; nothing in range yet means a `pending` slot. Where a series' episodes come
  from is in [`library.md`](library.md).

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
  desired lineup, its findings, status, healed metadata and next deadline. The **findings** are
  what the reconcile found rather than what anyone decided: the relaxations it applied and the
  lineup titles that cannot air. They live in the channel row's `reconcile_findings_json`, apart
  from the operator's `policy_json`, and are written in the same save as the desired lineup, so a
  reader never sees one reconcile's lineup with another's findings. An internal channel
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
  renumber) asks `approval.Binder.NumberInUse`, which unions the store with Tunarr's channel list. On a
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
it resolves a programme for streaming. `aired_at` is when the programme began on air
(`Airing.ProgramStartedAt`): for a programme split at natural breaks, its first segment's start,
whichever segment the viewer tuned into, so one airing is one row (#4140). Re-resolving the same
airing writes nothing new, so `recorded_at` is when the airing was first observed. The write is best-effort: a failed insert is
logged and the programme still airs. Each write prunes that unit's rows older than eight days down
to the newest of them, so the table stays bounded by lineup size. It is Loomarr's own broadcast
record, not viewer watch state.

`LastAiredByChannel(channel, before)` returns the latest airing per unit among rows recorded
strictly before `before`. Placement passes the start of the rolling window it is arranging, so a
window's history is fixed when the window opens (#1674). A tune-in during the window, including one
that first records a programme which started before the boundary, shapes the next window and
cannot re-arrange the one on air. A channel on an unbounded window never opens a new window, so it
reads no history.

`airings` is the **watched** signal: a channel nobody tunes to records nothing there.
`scheduled_airings` (same shape) is the **scheduled** signal (#1677; "aired" means both, as separate
signals, #1678 decision 1). On internal playout, each reconcile that commits records every programme
the cycle accepted before the commit had started, up to where that cycle hands over at its window boundary. The
write follows the commit: a successful CAS proves that cycle was still the accepted one, so an
attempt that stops on a failed read, or loses to another replica's re-deal, records nothing, and
the retry records from the winning row. Each row is stamped `recorded_at = aired_at`, so it counts
for every window that opens after the programme started, however late the reconcile that writes
it. The write is idempotent and best-effort. Placement takes
each unit's later time across watched airings, the scheduled ledger, and the accepted cycle's own
programmes that started before the window opened. The last of these is read in memory, so a window
the playout resolver arranges at the turn, before reconcile has recorded the old cycle's final
programmes, comes out the same as reconcile's. Tunarr channels record no scheduled airings: Tunarr
loops its list on its own clock.

Recency is a **soft ranking signal**, not a constraint (`programming-design.md` §3.1). A 24-hour day
consumes about 13 films, so a week without repeats needs about 168 hours of content; a hard
constraint would relax on nearly every real channel and teach operators to ignore the ladder. The
signal spreads airings evenly; only more content fixes frequency.

On a shuffled channel wider than its window, recency trades next-day repeats out of the window's
tile and holds the window to the repeat spacing. What the window before aired comes from the record
where it has one and otherwise from that window's tile, less the units its own guard traded out
(#1939). History therefore never brings a unit back sooner, or keeps one waiting longer, than the
same channel with no history.

## Upcoming airings

`channel_upcoming_airings` holds each internal-playout channel's programmes for the next 48 hours
(`channels.PremeasureHorizon`), so a title's page can say when it is next on, and where, in one
indexed read (#4476, [decision 0052](decisions/0052-next-airing-is-stored.md)). Each row is
`{channel_id, starts_at_ms, ends_at_ms, library_item_id, title_key, forecast, revision}`: UTC epoch
milliseconds, the lineup entry's title key, whether the row lies past the window on air, and the
channel revision it was computed at. `library_item_id` is a foreign key to `library_items`, and a
row goes with its item: a forecast never keeps an item the catalog purge drops. The write leaves out
an airing whose item the catalog doesn't hold and writes the rest.

- **The writer.** After a committed reconcile has saved the channel and planned the next window's
  breaks, the engine's upcoming index replaces the channel's rows. They come from
  `Resolver.UpcomingProgrammes`, the guide's own window-by-window walk (the window on air from the
  accepted cycle, the next one with its planned breaks, later ones forecast), without the guide's
  metadata pass. Only programmes count: a programme slot with a library item, never a pending
  placeholder. The walk covers at most eight rolling windows, counting the one on air, so a channel
  with a short window stores less than 48 hours; a window under 6 hours always does.
- **The replace** runs in one transaction and only while the channel is still at the revision the
  rows were computed from; a write from a stale revision changes nothing. It is best-effort: a
  failure is logged and the reconcile stands. When the walk or the replace fails, the channel's rows
  are cleared at the committed revision, also best-effort, so a title's page shows no airing rather
  than an earlier revision's.
- **The read** for a library item returns the rows still airing (`ends_at_ms` after now, so the
  programme on air is included) on channels neither paused nor detached, earliest first. A row
  matches the item by its own item id or through `titles.key = title_key` for that item, the key
  that lists a title's channels: a movie both ways, a series through its episodes' title key. The
  read does no member filtering. The API that serves it sends a member no `episode` at all for an
  episode their page leaves out: no id, name or numbering.
- **Freshness.** Pausing and detaching save without a reconcile, which is why the read filters the
  channel's status. Purging a channel deletes its rows. A settings change reaches the rows at the
  next sweep, and so does a removed file: its airing stays until the channel's next sweep
  (by default within about twenty minutes: `channel.reconcile_every` is a ten-minute minimum wait
  between a channel's reconciles, and the sweep runs on its own ten-minute cadence), as it does in
  the guide. Forecast rows can lag the guide by one sweep; the window on air never does, because its
  programmes are fixed when it opens.
- **Tunarr channels** store no upcoming airings, the same as the scheduled ledger. Each reconcile
  of a channel that does not play internally replaces its rows with none, through the same guarded
  write, before it projects onto Tunarr and again after it commits. A channel moved to Tunarr is
  therefore not named on a title's page, even when the projection fails.

## Breaks and pods

A break is an **ad pod**: intro bumper, 2–4 matched commercials, return bumper, sized to the gap.

- **Matching:** country and place first (a `local` clip's broadcast station within
  `filler.local_radius_km` of the channel's place), then era to the block, audience to the channel,
  and category variety within a pod.
- **Per-channel selection (`policy.filler`, `FillerSelection`)** narrows `era`, `audience`,
  `categories`, `kinds`, `pinned` and `excluded`. `era` has three states: unset inherits the era
  `policy.scope.dates` implies, `{0,0}` is explicitly any, and a range matches both bounds. Exclusion wins
  over a pin. Optional `geography` replaces the installation's, another country included (#2030
  decision 16), and is applied before pins. An empty place inherits the installation's only in
  the installation's country (#2059 decision 8). Optional `language` is the channel's commercial
  language (below). On first approval of a generated channel, one pure function seeds the
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
  (`pod.ResolveBreakDurationMs`): the channel's value when it is at least 30 s, else the install
  setting, else 2 minutes. The scheduler cuts the slot with it and the pod is assembled to it, so
  the two cannot disagree. **The break is a hard budget:** the intro bumper, the ads and the return
  bumper together never exceed it, because the slot is cut at exactly the break length and anything
  past it is lost. Assembly reserves the return bumper's real length before it places an ad, and
  an ad is never cut to fit.
- **A break with ads beats a break with bumpers.** If the bookends leave less room than the
  shortest ad, assembly drops the return bumper, then the intro bumper, before it gives up: a
  30 s break of 30 s ads still airs the ad. Only when no ad fits even alone is the break the 5 s
  card, and the channel then airs no breaks (no playable file).
- **The length is a ceiling the least recently aired clips fill** (#3838 step 2). Ads are taken
  in rotation order (never aired, then least recently aired). When the next one does not fit what
  is left the break ends: it is not handed to whichever clip is short enough to fit, which let
  length decide what aired. Fixed from a history-free pod, the length used to favour the short ads
  about two to one over a run of breaks. A clip longer than the whole break is skipped and never
  blocks the line; it leads a longer break. A break can therefore end short of its slot.
- **Each break is assembled once, ahead of time, and stored** (#3838 step 2, `filler_breaks`).
  Reconcile plans a channel's breaks in order over its window: it assembles each break not yet
  stored for its channel and start, and cuts the break's slot to the media its pod holds. The
  programme after a break therefore resumes when the break's own clips end, and the offline card
  never fills the gap. A break with nothing to air takes no time. The guide, the previews and the
  stream read the stored pod (`PreviewAtMany`; the Guide's one batch for every row is
  `PreviewChannels`, the same read), so the guide shows what airs, and a half-aired
  break keeps its pod and length. A break is keyed by its channel and start: a lineup edit that
  moves a break gives it nothing stored, so it is assembled fresh, and reconcile deletes the
  breaks behind the anchor and the ones that moved. A stored break assembled to another ceiling
  (the break length setting changed) is assembled again. A break within the run-ahead of now is
  **fixed**: it is on air or about to be, so it keeps the length it was stored with, or its slot's
  when it was never planned, and is never assembled after the fact. Fixing by what was stored, not
  by the slot's current length, is what makes planning at any instant walk the timeline that aired.
- **The next window's breaks are planned before it turns.** Once the programme crossing a window's
  end finishes, the playout airs the next window straight away (#1675), arranged with the same
  computation reconcile commits, before any reconcile has committed it. So within the hour before
  the turn, reconcile also plans the next window's breaks from the turn's instant. The playout then
  only reads their lengths (`Engine.TurnedWindow`), and each break ends with its pod from the
  window's first minute. The guide forecasts the next window with the same planned lengths
  (`Engine.WithPlannedBreaks` over its cached arrangement), so it lists each programme there at
  the time playout airs it (#4551); windows after the next are not planned yet and keep their
  arranged break lengths. Reconcile stays the only writer. Each plan clears stored breaks only in its
  own stretch: the window on air plans up to its turn and clears everything behind it, and the next
  window clears only from the turn, so neither deletes the other's. A break whose start the plan
  ahead missed (the lineup changed between the plan and the turn) airs live, assembled as it starts.
- **What "seen this evening" means for a break planned ahead:** the household's real watched airings
  up to now plus this channel's own already-stored breaks, so no ad repeats within the channel's
  evening. Other channels' planned breaks do not count, because what they air depends on viewing
  that has not happened. A repeat across channels that follows from future viewing is therefore not
  prevented: a known limit.
  `FILLER_POD_MAX` is a floor, never the reason a fitting clip is left out: the adapter sizes the
  clip ceiling from the break length over the shortest eligible clip (at most 64).
- **A break prefers clips the household has not seen this evening** (#3838 decision 1, grill
  decision 22, amended 2026-11-18). An evening is learned from viewing, never a fixed hour: the run
  of airings on every channel with no silence longer than two hours (`pod.EveningStartBefore`, the
  rule that sizes the pool). An ad that aired since the evening began ranks behind every unseen ad
  on any channel, so it airs only in what the unseen ads leave, the ones seen longest ago first.
  Bookends are exempt, and so are pins. A thin set therefore repeats its oldest ads rather than
  shrinking its breaks: a strict bar emptied breaks to the station ID alone while the sets were
  below their recipe. Repeats fade as the set fills. How far below its recipe a set
  also takes fewer breaks an hour is a number the loop simulation sets, not a constant here.
- **A re-size never moves what is on air.** Every airing is found by walking the accepted cycle
  from its anchor, so a break that has aired keeps its length. A catalog change that re-sizes
  breaks is a library fact: the window keeps its breaks until it turns, and a smaller catalog's
  breaks air what fits, then the card. A shorter `breakDuration` is an edit: it applies from the
  programme after the one on air, and the breaks that aired keep their length (#4140). A
  placeholder (a keyed filler slot at a missing programme's runtime) is not a break: its pod keeps
  the configured break length.
- **A schedule change restarts a stream only when it changes what is on air.** The packager reads
  the accepted cycle at every item boundary, so a change from its next lookup on reaches the stream
  by itself. Reconcile stops a channel's packager only when the new cycle airs something different
  over the packager's run-ahead (`timeline.OnAirChanged`), and Postgres peer replicas apply the same
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
- **The pool question belongs to the backend.** An internal channel has a pool when its pod holds
  playable media: local files, with no Tunarr uuid needed. A Tunarr channel has a pool when its
  pod holds a clip its filler list can reference by uuid. Station material counts on both, because
  the fallback ladder airs it before the bumper card. Reconcile and the forecast (the guide,
  schedule diff, proposal outlook and internal playout's cycle lookups) assemble a channel's inputs
  through one path, and each arranges from one reading of the settings, so the forecast shows
  exactly the breaks that air. Three differences are
  deliberate: only reconcile holds the window on air and turns it, only reconcile heals the
  lineup's metadata (it persists), and only the forecast prewarms media-server durations. Only
  reconcile's pool question records the pod-assembly metric.
- **Rotation (V58, household-wide since #2017).** The history is the household's, not a channel's:
  a commercial seen on any channel rests on every channel. One row per watched airing
  (`filler_airings`: channel, clip, scheduled start, recorded at) keeps every airing younger than a
  day and each clip's newest four beyond that, so it stays bounded. Internal playout writes it when
  a viewer receives the clip's segments (`resolve.Resolver.RecordAiringServed`, called once per item
  from the packager's segment serve path), keyed by scheduled start, so tune-ins and rebuilds cannot
  inflate it; resolving the airing is a read, so preview, reconcile, the guide and the Watch
  pickers' `/tracks` never write it. Programme airing history is written at the same point.
  **Only served airings count:** a packager a neighbour warm keeps ready for a channel switch encodes
  to nobody, and nobody fetches its segments. A viewer who switches to a warm channel counts the clip
  they are served. Per-channel history let one TV switching channels
  see the same clips in the same order on two channels within the hour, and warm neighbours rested
  clips nobody saw.

  Assembly reads a snapshot of airings that aired before the break starts **and** were recorded at
  least a settle (`OnAirLookahead` + 3 s) before it. The second cut keeps a break fixed while it
  airs across channels: a viewer tuning in mid-clip elsewhere records an airing that started
  minutes earlier, at that moment, and the break's pod is assembled again at every clip's resolve.
  Playout resolves a break's first clip no earlier than its run-ahead before the break, so every
  airing a break counts was stored before its first clip was assembled. Within each rung it ranks:
  never aired, then least recently aired outside the channel's commercial spacing. A clip inside its spacing
  ranks behind every rested clip on any rung, so the ladder widens era before a clip repeats, and
  spacing relaxes only when the whole ladder is resting (the pod records `CooldownRelaxed`).
  Spacing never causes dead air. Within one evening a clip that aired ranks behind every unseen
  one (above), so spacing mostly governs repeats across evenings. Pins come first and may repeat. **Each channel sets its own
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
- **Fallback ladder:** exact era → era widened by a decade either side → the channel's station
  material (bumpers and idents, resting or not,
  #2036) → the channel's bumper card. Never dead air. A pod fills from the
  tightest rung first and tops up from the rungs below it, so a small exact rung leads the break
  instead of being the whole break (#1684). The coverage level reports the rung breaks reach.
  Nothing further away than the decade either side ever airs (#3838 decision 21): once the two
  rungs are spent, clips resting on those rungs, least recently aired first, come before the
  station material. Bookends keep their own rule: the channel's era first, then any era.
- **The station-only fallback holds the same one-station-segment limit as any other break.**
- **Holidays (#2030).** A clip is seasonal when a tag names a holiday with dates; a holiday is its
  seasonal tag. Out of season it never airs, bumpers and station IDs included. In season, at most
  half of a break's commercials are in-season clips, and at least one when a rested one fits: the
  best rested in-season commercial that fits is placed first, then takes a seeded spot among the
  commercials that keeps category variety. The half yields only to a break the other clips cannot
  fill: in-season commercials past it then take the places left, in rotation order. A tag counts only where the clip
  asserts it, never through a rollup ancestor. The season is the household calendar programming
  reads (`filler.home_country`, the stored holidays, `guide.timezone`), dated at the break's start;
  a question asked without one (the pool check, coverage, a clip's fit, a Tunarr filler list) is
  dated now, and the sweep rolls it with the date. A Tunarr list written before midnight on a
  season's last day can therefore air its clips until the next sweep. A break's length is sized
  with no seasons, as it is without play history, so a season opening never re-sizes the
  timeline. A holiday with no dates is not a season: its clips air year-round. Pins skip both
  rules, and an in-season pin counts toward the half. A clip's fit reports `season` out of season.
- **Commercial language (#2030).** Each channel has a commercial language (`policy.filler.language`, any ISO 639 form, mapped onto the detector's codes); empty inherits the
  household setting `filler.language`. It is independent of the channel's audio language. A
  break airs clips in that language plus wordless (`none`) ones, bumpers and station IDs included;
  when they run out the card airs. A clip whose language is undetermined airs only where the
  channel's language is the household default. Pins skip the rule. Coverage names the language,
  counts it as a criterion and reports the undetermined commercials; a clip's fit reports
  `language`.
- **Intake follows the channels (#2030 decision 13).** The language rung and the splitter's
  per-segment check admit the union of the channels' commercial languages
  (`language.IntakeSet` in `internal/filler/language`), read live: adding a channel in a new language admits it with no
  settings change, and removing the last such channel stops new admissions without touching clips
  already catalogued. A detached channel does not count; a paused one does. A channel with no
  language at all turns the gate off. A split proposal snapshots the set, and review offers a
  recheck when the channels' set has changed since.
- **Location.** A channel's own country and market replace the household's, another country
  included, so a channel of British shows can opt into British commercials. It is never inferred
  from the shows. The region preference stays the household's: the channel inherits it, less its
  own country.

**Audience is a ceiling, and an unknown audience is in no ceiling** (a kids and teen
guardrail, #3838 decision 8):

- A `kids` channel admits clips grounded `kids` or `family`.
- A `family` or `general` channel admits `kids`, `family` and `general`.
- A `late_night` channel admits everything grounded. A channel that names no audience admits every
  clip whose audience is grounded.
- A clip with no grounded audience airs on no channel. Doubt resolves to the stricter reading.
- A `kids` channel airs a kids or family clip only when its audience is backed (a rule cue, an
  operator, a listing or a second agreeing signal), not when one model's guess is all there is
  (`Clip.AudienceGuess`, `filler.md` "What Loomarr learns about a clip"). Every other ceiling
  takes the guess.
- A new audience value admits nothing until someone decides it should.
- The ceiling holds for bumpers and station IDs as it does for ads, and a channel's coverage counts
  the bookends its own ceiling admits.

**A clip airs only with its facts** (#3838 decision 6). A clip with no concrete role, no era or no
audience fits no channel, whatever the channel's settings; the geography boundary already refuses an
unknown country once the installation has one. The one catalog filter assembly, coverage and per-clip
fit share enforces it, so a fit note says `facts` rather than blaming a setting.

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
  category variety, density, no-repeat-in-window, the holiday season and half
  (`internal/filler/pod/season_test.go`) and the commercial language and location
  (`language_boundary_test.go`, `geography_test.go`); the fallback ladder degrades to a bumper card;
  filler never appears as a lineup programme; only real catalog clips are placed.
