# Loomarr Programming Heuristics — Channel Policy Design

**Status:** Living companion to `design.md`; implemented policy and future changes are labelled in place.
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
    "collections": [],             // media-server collection (BoxSet) ids — §2.2
    "seasons":     {"min": 1, "max": 10},   // per-series season window
    "era":         {"from": 1990, "to": 1999}, // by first-air/release year, including episodes
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

`scope.era` applies at the playable programme boundary. A movie is judged by its release year;
a series entry is judged once by its own first-air year and every expanded episode is judged again
by that episode's production year. This is what makes `1989–1999 Simpsons` mean episodes from that
period instead of every episode of a series that happened to begin in 1989. An episode whose year
is unavailable passes the era check: scope is a taste filter rather than a safety boundary, and a
stale pre-year episode cache must not empty a Channel until its next refresh. An explicit
per-Lineup season window still narrows independently and wins by intersection.

**Single-series channels** (a Simpsons channel) auto-relax `seriesMinGap`/`blockMax` — separation there means *episode* spacing, not series spacing. The relaxation is rule-based, not LLM judgment.

### 2.1 Ownership (who may write each field, and what a refine does)

The blob mixes three owners. This is **structural in memory** (Go groups the fields into a proposal sub-struct, an operator sub-struct, and reconcile output) but the **wire JSON is flat and byte-identical** to the schema above — the LLM emits and this doc publishes the flat shape; the grouping never reaches the wire (no migration).

- **Proposal-owned** (`scope`, `audience`, `separation`, `ordering`, `seasonal`, and the seed of `rules`): the suggester extracts them; a fresh proposal (refine or re-curation) would normally refresh them.
- **Operator-owned** (`filler`, `window`, `autoCurate`): never emitted by the LLM; edited only on the channel page and preserved across every re-approval.
- **Reconcile output** (`applied`, the §7 relaxation record): enforcement writes it, the API surfaces it, it is rejected on write and recomputed each reconcile.

**Operator edits are sticky (the ownership *boundary*).** A proposal-owned field is refreshed by a later refine/re-curation **only until the operator explicitly sets it**; once set, it is marked operator-dirty (a persisted `operatorSet` list of field-paths, riding the flat blob — no migration) and a subsequent proposal **cannot overwrite it**. This is why the channel page can present era/ceiling/ordering/separation as directly editable without a refine silently reverting them. The **audience ceiling is never *relaxed*** even against a dirty value (the §4 safety asymmetry outranks stickiness — a dirty ceiling may be tightened by the fail-closed rules, never loosened).

⚠ **An EMPTY proposal-owned field is never pinned.** `PATCH /v1/channels/{id}` replaces `policy` **wholesale** — the operator's values win as a unit — so a body that omits a field is indistinguishable from one that deliberately cleared it. Pinning on "changed from current" therefore records an edit the operator may never have made, and the failure is silent and permanent: the field is blanked *and* marked operator-dirty, so no later refine or re-curation can ever restore it.

Observed on the dev "1980s Action Heroes": `operatorSet: ["ordering", "scope"]` with `scope: {}`. A single PATCH that changed **ordering** also carried an empty `scope`, which cleared the 1980s era window and pinned the emptiness. Six out-of-era titles (Guardians of the Galaxy 2014, The Rise of Skywalker 2019, Predator: Badlands 2025, …) then bound legitimately — enforcement was working, there was simply nothing left to enforce — and every re-curation since faithfully preserved a constraint that did not exist.

**The rule: a pin protects a VALUE, and an empty field has no value to protect.** Pinning one cannot preserve an operator's intent (there is nothing to preserve); it can only prevent a future proposal from filling the field in. So an empty result is treated as "not set" regardless of how it got there, and the omitted-vs-cleared ambiguity stops mattering. The asymmetry decides it: a wrongly-pinned empty field is silent permanent data loss, while a wrongly-unpinned one means a later refine re-suggests an era the operator can see and change again.

*This does not weaken stickiness*, which exists so a refine cannot revert a **deliberate** operator value — every non-empty edit still pins exactly as before. Nor is it a licence for a partial-PATCH client: the FE still sends the whole policy (`{...policy, scope: next}`), because clearing a field the operator *did* set must remain expressible.

**Rules merge by provenance.** Each `SchedulingRule` carries a `source` (`llm` or `operator`) + stable id. A refresh proposal **replaces only the `llm` rules and preserves the `operator` rules**, so "make weekends a TNG marathon" expressed *as a refine* adds/updates an LLM rule while a hand-authored rule is untouched — refine and hand-authoring **compose** instead of one clobbering the other. (This supersedes the earlier "operator-locked after first seed" behavior.)

A refine that says "add Christmas specials during the holiday window" changes those LLM-authored
rules, not the channel's whole seasonal identity: the base policy remains year-round and the
`holiday:christmas` rule activates its narrower slice on the calendar clock. "Turn this into a
Christmas channel" is the distinct identity operation and grounds to seasonal `exclusive`. The same
distinction applies to dayparts: morning/primetime/late-night curation is persisted policy evaluated
by the scheduler for both newly proposed and existing channels, not a request to invoke the model at
every clock boundary.

### 2.2 `scope.collections` — media-server collections, and why it is stamped

`scope.collections` restricts a channel to titles the operator has curated into a **media-server
collection** (Emby/Jellyfin call these **BoxSets**; Kometa and similar tools write them). It is a
hand-curated set — "my Halloween shelf" — which is precisely what makes it worth having: it is the
one scope field whose membership is an explicit human judgment rather than derived metadata.

⚠ **This is NOT `LineupEntry.CollectionID`, which is the TMDB franchise id (§5 ordering).** Two
different namespaces share the English word "collection": a TMDB collection is an integer
identifying a *franchise* (the Alien films), a BoxSet id is an opaque server-local string
identifying a *shelf someone made*. They are never interchangeable, and a filter that compared one
to the other would type-check and silently match nothing. The Go field is therefore named
`BoxSetIDs`, in the media server's own vocabulary, so the two cannot be confused at the point of
use. The wire/DTO name stays `collections` (it is what an operator calls it).

**Membership is STAMPED on the entry and healed at reconcile**, exactly like `OfficialRating`
(§4) and `CollectionID` (§5), for the reason those two are: `filterEntries` is a pure function
over `[]LineupEntry` with **no per-reconcile library I/O**, and that property is load-bearing for
scheduling latency rather than incidental. Resolving BoxSet membership inside the filter would put
a media-server round trip on the scheduling path, per entry, per pass — the N+1 shape that
dominated the guide-latency work (1910ms → 103ms; the first fix was removing exactly this).

⚠ **The `CollectionID` tri-state does not transfer, and the difference is a real trap.** `-1`
works as "resolved, standalone" because the field is an `int`. `BoxSetIDs` is a `[]string`, where
"not resolved yet" and "resolved, belongs to no collection" are **both empty** — so an entry in no
BoxSet would be re-fetched on every reconcile forever, reintroducing the N+1 through the back
door for the most common case. Resolution state is therefore carried by an explicit
`BoxSetsResolved bool`, not by nil-vs-empty: the entry is JSON-persisted, and a nil/empty slice
distinction does not survive a round trip through encoding/json, let alone a later refactor.

**The consequence, stated plainly:** adding a title to a BoxSet in Emby takes effect on the
channel's **next reconcile**, not instantly. That is the deliberate trade for keeping the
scheduler I/O-free, and it matches how a rating change already propagates.

**Fail-OPEN, unlike audience.** An unresolved entry (`BoxSetsResolved == false`) is **not**
filtered out — a media server that is down or slow must not silently empty a channel's lineup.
This is the opposite of the §4 audience ceiling, and the asymmetry is the same one §4 states: a
missing rating risks showing adult content to a kids channel, so it fails closed; a missing
collection membership risks showing an in-library title the operator did not shelve, which is a
taste miss, not a safety one. Scope is never a safety property (see the era grandfathering rule).

## 3. Separation & repetition ("don't show the same thing twice in a row")

Tunarr lineups are **cycles** — an ordered list that loops. So separation is enforced along the *cycle timeline including the wrap*: the last Simpsons block and the first must also honor the gap, or the loop seam betrays the illusion.

- **Hard floors:** `episodeNoRepeat` (an episode appears once per window), `movieNoRepeat` (longer — a movie reappearing in a week feels cheap).
- **Multi-part episodes stay together (a hard adjacency floor).** A two-parter ("The Best of Both Worlds (1)/(2)", "All Good Things… (1)/(2)") must air as an **atomic, in-order block** — Part 2 immediately after Part 1, never scattered by the shuffle or split by the rolling window (showing Part 2 with Part 1 out-of-window is worse than out-of-order). Detection is deterministic at episode-resolution time, from **either** signal: (a) the media server's `IndexNumberEnd` (a single file spanning e.g. episodes 25–26), **or** (b) a shared title base with a `(1)/(2)` or `Part N` suffix on **consecutive** episodes of the same season (the two-separate-files case, which is the common one). Grouped episodes carry a shared group id + within-group index; ordering (syndication/shuffle) permutes the group's **position** but never its internals, and the rolling-window truncation keeps a group **whole** (a group is one unit for the window budget). This is a floor, not a policy knob — a channel never *wants* a split two-parter.
- **Movie franchises stay together, in release order (the same floor, for films).** A franchise's films ("Raiders of the Lost Ark" → "Temple of Doom" → "Last Crusade") must air as an **atomic, in-release-order block**, never scattered by the shuffle (the reported bug: Temple → Crusade → *[unrelated film]* → Raiders). Detection is the **TMDB collection** (`belongs_to_collection.id`) — the authoritative signal, because it groups "Raiders of the Lost Ark" with the "Indiana Jones and the…" films even though they share **no title base** (a title heuristic can't). The collection id is resolved at reconcile (a per-movie TMDB lookup, healed onto the entry like `OfficialRating` — a tri-state field: `0` unresolved, `>0` a collection, `-1` resolved-standalone, so it's a one-time repair, not a per-sweep call) and stamped onto the lineup entry, so the **pure scheduler stays I/O-free**. Films sharing a collection get a shared group id + a **release-year** within-group index, then flow through the *same* collapse/expand atomicity as multi-part episodes — one mechanism, two sources (episode parts, franchise films). A collection with fewer than two present films isn't grouped (nothing to keep it with). Requires TMDB configured; without it, films simply aren't grouped (no worse than before).
- **Cycle-length consequence:** the no-repeat window implies a minimum pool size. If the pool can't fill the window, do **not** fail and do not silently violate — descend the **relaxation ladder (§7)**.
- All slotting is **seeded-deterministic** (seed = channel + cycle index, extending the main doc's pod rule) — same pool + same policy + same seed = same cycle, so tests reproduce exactly.

## 3.1. Recency ("don't bring back what just played")

Everything in §3 is **within-cycle**. When the cycle wraps, the deck replays from position and the
scheduler's memory resets to nothing — so a title recurs on a fixed positional clock rather than a
programmed one. Reported from the dev channel: Akira at Tue 21:53, Fri 13:33, Sat 02:10, Mon 01:30
— four airings in a week, at no interval anyone chose.

Recency closes that loop using `airings` (design.md §5): the last time each key aired **on this
channel**, consumed at placement.

- **A SOFT RANKING SIGNAL, not a constraint.** Among candidates that are equally valid under every
  §3 rule, the least-recently-aired wins; a title that has never aired sorts first. It cannot be
  "violated", so it has no ladder step (§7) and never emits a relaxation note.
- **⚠ Why soft, and why this is the honest design.** A hard "no repeat within N days" is
  *arithmetically unsatisfiable* on a real channel: a 24h day consumes ~13 films, so a week without
  repeats needs ~168h of content, and the dev channel has ~62h. Even 3 days is impossible there. A
  constraint that fails on every run produces a relaxation note on every run, and a ladder that
  always fires is a ladder operators learn to ignore — it would make the §7 output *less*
  trustworthy in exchange for a guarantee that was never available.
- **What it does and does not fix.** It spreads airings evenly and stops a title clustering near
  its own last showing, so repeats read as rotation rather than randomness. It does **not** make
  repeats rare: at 62h of content everything returns within ~2.5 days no matter how it is ordered.
  Only more content changes that frequency, which is what re-curation and adjacency candidates
  (§8.2/§8.3) are for. Saying so here is deliberate — an operator who reads "recency-aware" as
  "won't repeat" will file the same complaint again.
- **Determinism holds.** Last-aired timestamps are an *input*, so the same pool + policy + seed +
  history reproduce the same cycle. Tests pin history explicitly rather than relying on a clock.
- **Degrades to today's behaviour.** No history (a fresh install, a channel that has never aired,
  a store that cannot answer) ⇒ every candidate sorts equal and placement is exactly what it was
  before this existed. The signal is additive, never load-bearing.

## 4. Audience safety ("Saturday cartoons must never go adult")

The one heuristic where an error is a *harm*, not an aesthetic bug — so it fails closed:

- `ceiling` is a **closed ordered ladder** spanning both TV and film systems: `TV-Y < TV-Y7 < TV-G/G < TV-PG/PG < TV-14/PG-13 < TV-MA/R/NC-17`. Enforcement compares the item's `OfficialRating` (same field name on Emby and Jellyfin) mapped into the ladder; unmappable strings are treated as unrated.
- **The ceiling is enforced TWICE: on the entry, and again on every expanded episode.** A series entry is gated at its *series* rating in `filterEntries` — but that rating is a lossy **summary**, so `resolveEntry` re-checks each episode as it expands (`episodeVerdict`), and an episode rated above the ceiling is dropped even though its parent cleared the gate. A series whose *every* episode is refused resolves to **nothing**, never a pending slot: a pending slot means "approved, still acquiring", which advertises the title and (under PodFill) holds airtime for it — a show the ceiling refused is not late, it is excluded.
  - ⚠ **This paragraph used to say the opposite, and called it safe.** It described entry-only gating as "a small, deliberate safety *narrowing* … at worst a below-ceiling series with an occasional harder episode is admitted" — which is not a narrowing, it is the leak, stated as if it were the mitigation. Live proof (2026-08-10, maintainer's library): `90s Cartoon Classics`, `ceiling: TV-PG`, aired **King of the Hill** — a TV-PG series whose 275 episodes are 253 × TV-PG, **2 × TV-14**, and 20 × unrated. TMDB lists *both* TV-PG and TV-14 for the show. South Park and Futurama were dropped correctly at the entry gate, which is what made the leak look like correct behaviour.
  - **An episode with no rating of its own INHERITS its parent's** rather than failing closed — the one place §4's "never guess" is deliberately not applied. The parent has already cleared the ceiling as a whole title, so inheriting falls back to a check that just ran rather than guessing at unknown content; blank episode ratings are a metadata gap (20 of those 275) and refusing them would silently drop ~7% of an approved show. It is also what makes the change safe to deploy: `store.SeriesEpisodes` persists `[]ResolvedProgram`, so every cached row predating this field decodes as unrated, and a fail-closed reading would have emptied every kids channel until the refresh sweep repopulated the cache — trading a content leak for dead air, which §9 forbids just as firmly. The asymmetry still holds where it counts: an episode that **is** rated is judged on its own rating and can never be lifted by a permissive parent.
  - The rating/genres/year an *entry* is filtered on are **stamped onto the channel's approved lineup entry at create time** (when the full grounded candidate is in hand). Episode ratings and production years ride the episode cache (`library.ListEpisodes` requests `OfficialRating,ProductionYear`), so both gates stay pure entry/slot-set filters with no per-reconcile library I/O.
- **Kids ceilings (`TV-Y`…`TV-PG`) default `unrated: "exclude"`** — an item with missing or unmappable rating metadata is *excluded*, never guessed at. Metadata gaps are the real-world failure mode; a kids' channel must be safe against them by construction. Adult/general channels default `allow`.
- **Transparency at review:** the proposal shows the policy's effect — so gaps are visible *before* approval, and the fix (rate your media, or relax the policy) is a human decision.
  - **The suggester REFUSES its own unairable picks** (`refuseUnairable`, after `groundPolicy`). A grounded pick whose known rating is above the extracted ceiling is moved out of `Lineup`/`Acquisitions` into `Proposal.Refused` with the same `over_ceiling` vocabulary §4 uses elsewhere, and the approval card names it. ⚠ **Refused, not deleted** — the operator's usual fix is to raise the ceiling, and a pick that vanished between the model's answer and the approval screen is indistinguishable from one the model never made. It is not provisioned either: approval acts only on what it offered.
  - The refusal runs *after* `groundPolicy`, so every pick is judged against the final deterministic ceiling shown for approval.
  - **An UNRATED pick is refused when the intent explicitly requires child safety.** The refusal uses `over_ceiling`, the same operator-facing safety reason as a known harder rating. Rating enrichment and reconcile healing remain useful for ordinary family/teen proposals, but an explicit child-safety request cannot make unknown content actionable while waiting for metadata.
  - ⚠ **This closes a hole in the AUTHORIZATION model, not just a display gap.** Approval is the gate (design.md §7/§11); until this, it presented choices that were silently discarded downstream — the operator approved seven titles and got five — which teaches that the list is approximate, exactly the property approving exists to deny. Found live 2026-08-10 on a `TV-PG` "90s Saturday morning cartoons" channel that proposed South Park (TV-MA) and Futurama (TV-14).
- The LLM may *infer* the ceiling from intent, but it does not own the safety boundary. Explicit child-safety language (`kid-safe`, `for kids`, `safe for children`, preschool/toddler, or Saturday-morning kids programming) deterministically imposes a maximum of `TV-Y7`, even when the model omits a ceiling or proposes a looser one. A model may propose a stricter ceiling; it may never relax this bound. The final value is shown as an editable chip in review, and enforcement is the ladder comparison.
- **The ceiling is a kids/teen guardrail, not a general default — omit it unless the intent asks for one.** The audience ceiling exists for one purpose: so a channel a user asked to be *for kids or teens* can never show adult content. An unqualified channel is an **adult-default** channel — "1980s Action Heroes" obviously includes its R-rated films (Die Hard, Predator, The Terminator). So **a proposed ceiling is kept only when the intent carries a kids/teen signal** (words like "kids", "family", "cartoons", "all ages", a named kids property like "Bluey", an explicit low rating like "TV-Y", or a kids daypart). With **no such signal, any model-proposed ceiling is dropped** (→ no ceiling, everything admitted) — a small model's reflexive "action might be violent, better cap it at TV-14" must not silently strip the R-rated content the channel is *about*. This is enforced deterministically in `groundPolicy` (the prompt says "adult/no mention → omit," but the model isn't trusted to obey it). **The safety asymmetry is absolute:** dropping an *unjustified* ceiling only ever *loosens* (a content choice, reversible by the operator); when a kids/teen signal *is* present the ceiling stays and is **enforced fail-closed**. Grounded picks never raise a ceiling: a known harder title is refused, and an explicit child-safety request also refuses unrated titles. Loosen freely on adult channels; never loosen a kids channel.
- **Nor does an era (auto-widen-to-admit).** The same rule, for the same reason, on `scope.era`: a model-proposed year range is **widened just far enough to include the channel's own grounded picks**. The failure was live — a "Midnight Sci-Fi Horror" channel came back with `era.from: 1982` *and* **Alien (1979)** on its approved lineup, so the enforcer filtered out a title the operator had explicitly approved. The lineup said six, the guide aired four, and nothing named the missing two.

  Extraction and enforcement disagreeing about the same proposal is a **self-contradiction, not a preference** — exactly what the ceiling rule already resolves toward the content. A widen is also strictly safer than the audience version it mirrors: there is no era analogue of the kids line, because a year is a curation choice and never a safety property, so no bound is needed.

  The era still **constrains everything the model did not pick** — backfill, re-curation, and filler continue to respect it. Only the already-approved picks are grandfathered in, which is the narrowest fix that keeps the era meaningful. Dropping the era wholesale would have been simpler and worse: it would discard a scope the model may have inferred correctly ("modern sci-fi horror") and let a 1950s B-movie land later on a channel framed as modern.

## 5. Ordering modes ("feels like TV")

- `sequential` — S1E1 onward, loops at the end (binge/marathon channels).
- `shuffle` — seeded random, separation-constrained.
- `syndication` (default for TV) — random **without repeats until the eligible pool exhausts**, then reshuffle (a "deck deal"): the authentic weekday-rerun texture, and it makes `episodeNoRepeat` nearly free because the deck *is* a no-repeat structure. Each deck reshuffles under `seed XOR deckIndex` so successive decks differ yet every deck is deterministic for a given channel seed (the §7-mandated reproducibility).
- **Omitted `ordering` inherits the channel's `Strategy`.** A channel created without an explicit policy ordering keeps its existing `sequential`/`shuffle` behavior — the syndication default applies only when a policy explicitly requests it (or a template ships it). This keeps policy adoption non-breaking for existing channels.
- **One operator knob, one canonical precedence ladder.** Ordering resolves in exactly this order: **per-rule `How.Ordering` (within that rule's active window, §6.5) > `policy.ordering` > `Channel.Strategy` (the create-time stored default)**. `policy.ordering` is *the* operator-facing knob (edited in Programming → How it's ordered); `Channel.Strategy` is the default the binder seeds and is consulted only on inherit — it is **not** a second editable field (design.md §9). A rule's `How` overrides only for the slots that rule governs; outside any rule window, the base `policy.ordering` applies. This is the single ladder that makes "three ways to express ordering" read as one.
- **Rerun curation and narrative order are different promises.** A channel whose lineup is several series (a "Star Trek" franchise channel) defaults to `syndication` so the shows intermix instead of playing one to completion. A clearly curated single-series intent (`classic Simpsons`, `best episodes`, favorites, highlights, reruns) also resolves to `syndication`: its eligible season window is a pool to curate into a deterministic no-repeat deck, not an instruction to start at S1E1. Explicit chronological, start-to-finish, binge, or marathon language forces `sequential`, even when the model proposed shuffle or syndication. One series plus movies is not inferred as either case. This episodic rule does **not** loosen the movie-franchise floor above: films sharing a TMDB collection remain one atomic, in-release-order block even when the surrounding channel is shuffled or syndicated. The prompt guides the model, and `groundPolicy` enforces both deterministic outcomes so a small model cannot turn a rerun request into a box-set binge or a binge into a mixed deck.
- **Episode selection precedes episode ordering.** Every proposed series carries one proposal-owned
  closed mode through approval into its Lineup entry: `complete`, `highlights`, or `holiday`.
  Omitted/unknown legacy data means `complete`. Code derives the mode from explicit Intent after
  grounding; the model never names episodes or chooses the mode. Whole-word `classic`/`best` plus
  favorites, reruns, curated, or highlights language selects `highlights`; explicit chronological,
  start-to-finish, binge, or marathon language selects `complete`. A named built-in holiday selects
  only that holiday; explicit generic “holiday episodes/specials” selects across the closed built-in
  holiday vocabulary. Named holiday detection uses the shared Unicode-normalized whole-phrase
  matcher over every affirmative Intent field, including ordinary refine text such as “add
  Christmas specials”; “Christmasland” and “Valentinesque” are not holiday cues. `mustExclude`
  remains a negative grounding constraint and cannot trigger or
  suppress a positive episode mode. Holiday specificity wins over narrative ordering, so a chronological
  Christmas request selects Christmas episodes and orders only that pool sequentially.
  Built-in holiday ids and aliases have one immutable domain owner shared by Intent recognition and
  scheduler evidence/calendar matching. Calendar dates remain scheduler-owned and bind to those ids;
  no caller carries a parallel alias table.

  Editorial selection requires current editorial evidence. An episode-resolution result carries the
  safe playable deck and whether that evidence is available, rather than encoding freshness by
  blanking episode fields or asking schedulers to inspect cache state. An aged cache is live-refreshed;
  if refresh fails, a non-empty valid cached deck remains subject to the same season, era, and audience
  filters but `highlights` and `holiday` use that complete safe deck. An aged empty cache with failed
  refresh is unavailable. Fresh and successfully refreshed decks permit their declared editorial mode.

  Selection receives the pool only after season, era, and audience gates. It treats each standalone
  episode or detected multi-part story as one atomic unit, and returns units in canonical order for
  the existing ordering engine to place. A multi-part unit is rated only when every part has a valid
  rating, using the arithmetic mean of its part ratings; one missing/invalid part makes the unit
  unrated. `highlights` uses the Library's aggregate community rating
  only as a relative cohort signal: at least eight units must exist, at least 75% must carry a finite
  rating in `(0,10]`, and the chosen pool is the upper rated quartile with a four-unit floor and a
  48-unit cap. All units tied at the cutoff are included; a tie that consumes the rated cohort or
  exceeds the cap falls back to `complete`. The 75% floor prevents a sparse rated minority from
  defining “best”; four keeps a usable rerun deck; 48 bounds long-running series. Emby/Jellyfin does
  not expose rating vote counts, so cohort coverage is the only supported confidence gate: this
  slice makes no absolute-confidence claim and performs no scheduler-time enrichment call.

  One episode-evidence codec owns the rating and text/tag domain. The Library adapter and durable
  `SeriesEpisodes` cache both decode provider/cache JSON through it, while durable writes invoke the
  same sanitizer, so neither live responses nor legacy blobs can bypass newer bounds. Malformed
  editorial rating/text/tag JSON becomes unavailable evidence and unknown fields are ignored. A
  malformed mixed-type tag array makes the entire tag field unavailable; no successfully decoded
  prefix survives. Exact or Unicode-case-fold duplicate editorial
  members are also unavailable: the decoder preserves/counts object-member occurrences instead of
  accepting a map-iteration or last-member winner. Editorial corruption does not discard neighboring
  valid live episodes, while malformed playable structure still omits that live item. Every cached series episode must be an object
  in which each required playable member is present: a non-blank Library item id, positive runtime,
  non-negative season, positive episode number, and an episode end of zero or at least the starting
  episode. Absence is not interpreted as an explicit zero, and every required numeric member must
  have an actual JSON number type. Every non-null database value is decoded: an empty string, null
  document, wrong document shape, null/empty rows, exact or case-fold duplicates of those required playable members, and malformed or
  invalid playable identity, numbering, or runtime fail the whole cache read because scheduling
  cannot safely invent those facts or emit a blank/zero-duration slot.
  The Library adapter drops live items without a non-blank id, positive runtime, or present valid
  season/episode numbering. The durable write interface rejects the entire write when the same
  playable contract is violated; editorial repair remains tolerant. A valid `[]` alone represents
  an enumerated series with no playable episodes.

  `holiday` matches normalized whole words/phrases in the episode title and bounded overview/tags,
  restricted to the selected holiday ids (empty means all built-ins). A no-match holiday request,
  fewer than six rated units in an eight-episode fixture, malformed mode, or sparse/legacy cache row
  returns the complete already-safe pool. Selection never restores an audience/scope rejection and
  never manufactures dead air. Proposal review renders the mode before approval as `All episodes`,
  `Curated highlights`, or the named/generic holiday episode scope.

  `ApplyLineup` same-key replacement preserves the approved mode when the lossy lineup edit DTO
  omits it. Reordering or renaming a series therefore cannot silently reset highlights/holiday to
  complete. At approval, after all search additions and drops, the one server approval gate
  re-stamps every series in the lineup, acquisitions, and alternates from the Proposal's original
  Intent before Proposal serialisation and Lineup binding. Generation applies the same grounding to
  all three collections, so an alternate promoted later cannot silently default to a complete deck.
  Missing or client-injected modes therefore cannot cross the approval boundary; movies remain
  selector-free because episode selection does not apply to them.
  Before approval, the proposal API exposes one read-only selection preview derived server-side
  from the same trusted Intent. Search-added series rows render that preview even when the proposal
  originally contained only movies. The client does not parse cues or submit the projection;
  approval still re-derives and replaces every series selector before persistence.

## 6. Seasonality ("holiday episodes at holiday time — and only then")

Two symmetric behaviors, because knowing what October wants implies knowing what July doesn't:

- **Detection** (deterministic, not LLM judgment): an item is *seasonal* for holiday H if it matches H's keyword set against episode/movie title, media-server tags/keywords, or overview ("halloween," "christmas," "thanksgiving," …). Built-in calendar v1: Halloween (Oct 1–31), Thanksgiving-US (Nov 15–30), Christmas/holidays (Dec 1–26), New Year (Dec 27–Jan 2), Valentine's (Feb 1–14). Windows and keyword sets ship as data, not code; custom holidays/regions are future work.
  - **Entry and episode scope.** Entry-level seasonal bench/boost continues to match a movie or
    series title only. An explicitly holiday-curated series additionally carries a per-entry
    `holiday` episode selector; after episode safety/scope filtering it matches the episode's title,
    overview, and media-server tags against only the named holiday ids. This supports “Christmas
    Simpsons episodes” without mistaking every Comedy or Family episode for a holiday episode.
  - ⚠ **Genres are excluded deliberately, and this is a correction.** Detection originally hayed over title **+ genres**, which conflated *"this title is about a holiday"* with *"this title belongs to a genre that correlates with one"*. Those are different claims, and only the first is what a holiday window should act on. The failure was total rather than cosmetic: `horror` sat in the Halloween keyword set, so on a year-round horror channel **every** entry detected as seasonal, and `auto` mode benched all of them `out_of_season` for the eleven months outside October. The channel was legal to configure, reported `live`, and aired nothing. A genre is what a channel **is**; a holiday keyword is what a title is **about**. The rule's own example — *Christmas episodes in July break the spell* — is a title, and the fix restores exactly that reading. (A single genre-shaped keyword could have been dropped instead, but the mechanism would have re-fired on the next collision: a Romance channel near Valentine's, a Family channel near Christmas.)
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
- **Rolling window — a ROTATING slice of the catalog, not a fixed prefix.** A channel
  materializes only ~`Window` of runtime (default **24h**, per-channel/-rule overridable,
  global default `sched.window_hours`) rather than the whole run — so a channel schedules a
  manageable timeframe and is *curated over time*. The window keeps a **rotating slice** of
  the ordered deck whose start advances by the coarse window index (`floor(now/window)`):
  window 0 airs the first ~`Window` of runtime, window 1 continues where it left off, and
  the slice **wraps** past the end back to the deck head. A channel with 30h of films and a
  24h window airs `[film k … end, start … ]` each day with `k` advancing daily, so over a
  full cycle **every** title airs — the coverage invariant: across `ceil(totalRuntime /
  window)` consecutive windows, no program is starved (the whole catalog rotates through).
  This is what makes "continuously updated and curated within the window" true — without it,
  keeping only the deck *prefix* would loop the same head every window and permanently starve
  the tail (the bug this fixes). New titles (backfill / a manual add) join the deck and enter
  the rotation on the window that reaches them — the auto-updating half is free.
  **Idempotent + thrash-free:** the slice offset is a pure function of the window index, so it
  is **identical within a window** (two reconciles → byte-identical desired → no Tunarr
  re-push) and **advances exactly once at the boundary** (one re-push, the next slice). The
  slice operates on the *collapsed* deck (multi-part/franchise super-slots, §5), so the window
  seam never splits a two-parter or a franchise. `Window: 0` = the whole run (the "full binge"
  sentinel; a `marathon` rule sets it — no rotation, plays everything). A pool smaller than one
  window airs whole every window (nothing to rotate, nothing starved). The slice is floored to
  at least one program so a channel is never dark.
- **Authorship is hybrid (§8 boundary intact).** The **LLM proposes a starter rule set**
  from intent ("Star Trek with weekend TNG marathons and Christmas episodes") — from the
  closed preset vocabulary, grounded + clamped (unknown tokens dropped, window clamped to
  `[1h,168h]`, daypart audience ceilings **stricter-only** — a rule may never *raise* a
  kids channel's ceiling, §4). The **user refines** rules in the channel "Programming
  rules" editor (chips, drag-to-priority). The LLM still only proposes rule VALUES;
  deterministic code evaluates `When` and enforces — the model never orders episodes.
- **No new scheduler.** Rules evaluate against the container wall-clock at reconcile
  time; periodic channel maintenance (§main-doc) already re-runs the pure lineup builder
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
- **Rule WHAT remains entry-level.** A generic `holiday-matched` or `genre` WHAT filter still
  narrows series/movies. Per-episode holiday precision is activated only by the approved series
  entry's `episodeSelection` policy, which has explicit holiday ids and episode metadata; it is not
  inferred from an arbitrary rule at reconcile time.

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
- **Proposal review + channel editor (§12):** policy renders as editable chips (ceiling, seasons, era, ordering, seasonal mode) + the exclusion report (§4) + a **cycle preview** (§8.1 `GET …/cycle?at=` — first N slots with the active-rule attribution) so "did old-school bind?" and "what airs Saturday 9am?" are answerable by looking.
  Every series row also names its proposal-owned episode-selection mode before approval: all
  episodes, curated highlights, or the explicit holiday scope. The review explains the selector;
  it does not let the client choose episode identities or bypass the deterministic binder.
  - ⚠ **The exclusion report reaches the CHANNEL EDITOR only; proposal review still does not show it.** Both preview endpoints (`GET …/cycle` and `POST …/programming/preview`) carry `excluded` — counts plus a per-item reason — and the cycle-preview panel renders it under the schedule. This half of the sentence was aspirational for as long as the type existed: `ComputeDesiredAt` filled the report on **every reconcile** and every caller discarded it, so a title the ceiling refused was invisible product-wide, and diagnosing one meant querying the media server by hand. **Reconcile still discards its copy** — it has no column and no event to put one in, and the preview recomputes the identical report from the same pure builder. Both remaining gaps are real and named rather than implied. The same chip surface is the **per-channel rules editor** (§8.1) on the channel page (§7 `PATCH .../{id}` writes `policy_json`); omitted chips inherit the built-in default (§9), and `audience` + explicit `scope` are shown as never-relaxed safety fields.
- **Filler is part of the channel editor too (§10):** `policy.filler` (the `FillerSelection`) is edited on the channel page alongside the rules chips — theme criteria (era/audience/category/kinds) + pinned/excluded clips — with a live pod sandbox (`POST …/pods/preview`) that re-assembles the actual break against the unsaved draft before Apply. It also rides `policy_json`, so it round-trips and inherits the same "omitted = any" default; a new channel seeds its filler era from `scope.era`.
- **Lineup builder (§9):** hard filters → eligible pool → seeded constraint-aware slotting (greedy with backtracking is sufficient at envelope scale) → relaxation ladder on failure → pods → push. The periodic sweep re-evaluates policy (seasonal windows roll, library grows, relaxations un-relax when the pool recovers).

## 8.1. The cycle-preview endpoint & the rules editor (making the engine legible + editable)

Two surfaces make curation rules *authorable* and their resolution *visible* — the whole point of first-match-by-priority is that it must never be a mystery.

**`GET /v1/channels/{id}/cycle?at=<rfc3339>` — the time-travel preview (read).** Any authenticated user; a pure, side-effect-free look at *what would air* at a chosen wall-clock. It runs the **identical** `ComputeDesiredAt` the reconciler runs — same channel, same approved lineup, same live availability, same policy — only with `now` set to `at` (default: the server's clock). Preview and reality therefore cannot disagree, exactly as the pod preview (§10) can't drift from the assembled pod: there is one code path, called twice. The response is:

- `at` — the resolved wall-clock the preview was computed for (echoed, so a client sees which moment it's looking at).
- `activeRule` — the rule `pickRule` selected for `at`: `{ id, label, priority, matched }`. `matched:false` (with a synthesized `label:"Base policy"`) means **no rule matched** and the channel fell through to its base whole-policy behavior. This is the attribution that makes overlap resolution answerable: "at Saturday 9am, the *Weekend TNG marathon* rule (priority 20) is active." It is derived from the *same* `pickRule` call the engine makes — never a re-implementation.
- `window` — the resolved rolling-window horizon for `at` (rule > channel > global default; `0`/`null` = whole run), so the preview explains *why* it shows ~24h of slots and not 800.
- `slots` — the first N program/pending/break slots of the resolved cycle (N capped, default 50), each `{ kind, title, key, seriesTitle, part }` — enough to see intermixing, marathons, and franchise/two-parter adjacency at a glance. Filler break gaps render as `kind:"break"` (Tunarr owns what plays into them; the preview shows the *gap*, not the clip).
- `trace` — the bounded `ScheduleTrace` emitted by that same `ComputeDesiredAt` call (§8.4), including hard-filter, availability, episode-selection, window, and placement facts. It is not reconstructed from `slots` or from the originating Proposal.

The preview is **read-only and never touches Tunarr or the store** — it is `ComputeDesiredAt` on already-loaded state. `at` in the past or far future is legal (that's the feature — "what airs next Christmas morning?"); an unparseable `at` is a 400.

**The "Programming rules" editor (`PATCH /v1/channels/{id}` → `policy_json`, admin) — authoring.** The channel page's Programming surface (design.md §12) renders `policy.rules` as an ordered list of **rule chips**, each showing its WHEN/WHAT/HOW tokens and priority. Authoring is **token-based, never raw predicates** — the same closed vocabulary the LLM uses (§6.6). **The vocabulary + lowering table is BE-authoritative, served by `GET /v1/programming/vocabulary`** (the WHEN/WHAT/HOW presets and how each lowers to `SchedulingRule` fields). The editor renders from it and the suggester's `groundRules` lowers with it — **one source, so hand-authored and LLM-authored rules are byte-identical by construction** (this closes the drift hazard of the FE re-implementing the §6.6 table). The BE re-validates on write (unknown tokens dropped, window clamped, audience stricter-only — enforced on *every* write path, not just the LLM's). Each rule carries a **`source` (`llm` or `operator`)** stamped at authoring time — the provenance a refresh proposal merges on (§8.2): a refine replaces the `llm` rules and preserves the `operator` ones. **Priority is drag-to-reorder** (list order *is* priority; higher = higher). `audience` and explicit `scope` remain never-relaxed safety fields, shown but not loosenable by a rule (§4, §7). Every save auto-reconciles (no rebuild).

**`POST /v1/channels/{id}/programming/preview` — the whole-definition draft preview (admin authoring).** The cycle preview above (`GET …/cycle`) and the pod preview (`GET …/pods`, §10) each preview *saved* state; this generalizes both to an **unsaved draft** — body `{lineup?, policy?}`, returns the shared shape `{at, activeRule, window, slots, trace, pods}` without persisting. It runs the **same** `ComputeDesiredAt` + pod assembler as reconcile (so a drafted lineup + rules + filler edit preview exactly what will air once applied), and is the call the Programming surface's **scheduling-rules draft** uses while editing (design.md §12 — the third review-before-apply surface, and the only block of that page that drafts; scope/ordering/auto-curate stay seamless because their edits are self-contained). ⚠ This sentence previously read "the one call the Programming surface uses while editing", which asserted a whole-page draft §12 did not sanction and the implementation never built — a companion doc contradicting the design doc, which AGENTS.md resolves in the design doc's favour. It **does not replace** the read paths: `GET …/cycle` stays the any-authenticated verification pane and `GET …/pods` the saved-pool read — the change is one shared response schema, not deleting the GETs. The preview shows play *order*, not wall-clock. Overview airtimes come from the backend actually streaming the channel: Tunarr's guide for Tunarr-backed channels and Loomarr's internal timeline for internal channels.

## 8.2. Self-updating channels (scheduled re-curation)

A channel built from an intent shouldn't be frozen at build time — as the library grows and new titles land, the channel should **keep curating itself** toward its original intent. Re-curation is that loop: on a schedule, a channel re-evaluates its intent against the *current* library and evolves its lineup — **preferring what's already there, weighting net-new acquisitions by quality + intent fit, and never bypassing the approval gate.**

**It is a composition over existing parts, not a new engine.** The suggester already (a) re-runs against a stored intent + the current lineup as context (the *refine* path, §7), (b) splits picks into **in-library** (free, instant) vs **acquisition** (must request), (c) scores theme-first (§8.1 suggester), and (d) re-validates acquisitions vs TMDB. Re-curation just *schedules* a refine and lets the results flow through the same approve → bind → reconcile pipeline. The genuinely new pieces are a scheduled job, a per-channel opt-in, and a channel-scoped auto-approve grant.

- **A scheduled job `channel-recurate`** (default **weekly**, `job.recurate.schedule`; Run-now-triggerable like every §18.1 job; LLM-cost-bearing so infrequent). For each **live, intent-backed, auto-curate-opted-in** channel (`IntentRef != "" ∧ AutoCurate ∧ status live/building`; hand-made / paused / detached / opted-out channels are skipped) it triggers a **refresh refine** — the channel's own `IntentRef` job re-run against its stored intent + current lineup, with no operator `RefineText` (the "change" is simply "re-evaluate against what's in the library now").
  The re-queued job is durably marked `kind=recurate`; the worker uses that server-owned marker
  to select the channel-scoped grant. It skips the requester's ordinary auto-approve grant for
  this run, and ordinary/manual refine jobs never reach the auto-curate grant. Without that
  discriminator, a requester grant could bypass the channel quality/cap thresholds, while a
  human refine on an opted-in channel could be silently treated as unattended work.
  Discovery-feedback scope follows the same durable authority: immediately before generation the
  worker resolves the one current Channel whose unique non-empty `intent_ref` equals the trusted
  claimed Job id and attaches that Channel id only to the in-memory Intent. The serializer never
  persists this execution field. Missing, detached, unreadable, empty, or mismatched ownership
  fails the Attempt before catalog or provider access; fresh suggestions and manual refines use
  household feedback only. An admin may name a Channel when recording an editorial feedback event,
  but that event field is not execution authority.
- **Library-aware + intent-weighted.** The fresh proposal already classifies picks. Re-curation leans on it: an **in-library** title that fits the intent is added to the lineup immediately (already `available` → airs next rotation, **zero acquisition, no approval needed** — it's a lineup extension, like a manual add). A **net-new** (not-in-library) title is only *requested* if its intent-fit **score clears a quality bar** (`recurate.min_score`) **and** the channel isn't at its **title cap** (`recurate.max_titles`); candidates are ranked by score and the top ones taken. Both knobs are global settings (`env > db > default`) with an **optional per-channel override** on the opt-in, so a channel can be more or less permissive than the fleet default.
- **Additive lineup binding — auto-curate ADDS, humans REPLACE.** This split lives in the `ChannelBinder`, keyed on the proposal's `approvedBy` audit field. A **human-in-the-loop** approval (manual approve, or a manual refine the operator drove) **replaces** the lineup — a person decided, including to remove titles. **Scheduled auto-curate** runs unattended, so it is **non-destructive**: it UNIONS the refreshed proposal's picks onto the channel's existing lineup and **keeps every still-available title the refresh merely didn't re-pick** (an LLM is stochastic; omission is not a decision to remove). Without this, weekly re-curation would *churn* — silently dropping good, available, on-theme titles nobody chose to drop. **Conservative pruning** is the one exception: auto-curate drops an existing title only when it is **clearly off-intent** — it has **genuinely left the library** (state `unavailable`), or the intent's `mustExclude` names it. A title that is still `available` (or an acquisition still in flight — `wanted`/`requested`/`downloading`) is never dropped by re-curation. Every drop is logged. So re-curation grows a channel toward its intent and prunes only the clearly-wrong; a refresh that finds nothing better leaves the channel exactly as it was.
- **Retirement at the cap — the turnstile (§8.2a).** Additive binding plus a title cap has an end state nobody chose: a channel grows to `recurate.max_titles` and then **freezes permanently**. `room` reaches 0, every future candidate is dropped, and re-curation keeps running, keeps spending tokens, and can never change anything again. Observed on the dev "1980s Action Heroes": 25 → 27 → 30 → 34 titles against a cap of 40, with nothing ever leaving. Growing and frozen were the only two states; neither is rotation.

  So a channel **above its rotation target (¾ of the cap)** trades rather than only grows: a genuinely better candidate **retires the weakest retirable title** even while free slots remain, and at the cap it does so instead of being discarded. Retiring only at 100% was itself the wrong trigger — it made the lineup a *ratchet*, appending every run and dropping nothing until the channel froze, so a lineup curated for weeks still led with its original picks ("why do I still see the same old movies"). A real station retires a film once it has had its run; it does not wait until the shelf is full. **Below the target nothing is retired at all** — a young channel should fill up, not churn. Confidence is the criterion — the same per-title score the quality bar reads, which is only meaningful now that the prompt calibrates it (§8.3). The cap becomes a turnstile rather than a wall.

  ⚠ **Nothing currently scheduled may be retired.** A title in the channel's `Desired` slots is airing in the current window — someone may be planning to watch it today, and pulling it out from under them is worse than a stale channel. `ch.Desired` is already persisted per channel, so the guard costs no new dependency: the retirable set is `lineup − scheduled`. Measured on the same channel: 34 in the lineup, 15 scheduled (protected), 19 retirable. **A retirement that cannot find an unscheduled, weaker title simply does not happen** — the newcomer is dropped over-cap exactly as before, which is the safe direction.

  Retirement is otherwise bounded by the same rules as everything else here: it happens only for a channel that opted into `AutoCurate`, only when the incoming candidate's confidence **exceeds** the retiring title's (never a tie — a coin-flip churn is the failure mode additive binding exists to prevent), and every retirement is logged with both titles and both scores. This narrows §8.2's "conservative pruning" rather than replacing it: the `unavailable` / `mustExclude` drops still apply on every run, cap or no cap.

  ⚠ **`recurate` DECIDES a retirement; the binder APPLIES it. One subsystem writes a channel's lineup.** The turnstile records its rotate-out keys on the proposal (`Proposal.Retired`), and the binder consumes them as a third `ApplyOpts.Drop` signal alongside `unavailable` and `mustExclude` — so every lineup write still goes through the single `schedule.ApplyLineup` primitive §9 requires. This is a correction: `recurate` originally trimmed `ch.Lineup` and persisted the channel *itself*, moments before the binder's additive union ran over the same field. Two writers, sequenced only by a code comment explaining that the trim had to land first or the union would put every retired title straight back — an ordering nothing could check and a new caller could not discover. Retirement is a **decision**, unlike the other two drop signals, which are observations: a retired title is usually still perfectly `available`, so the availability test would say "keep" and the swap would silently never happen. That is why it is checked first and needs no store read.

- **The approval gate is non-negotiable (prime directive #3).** A net-new acquisition NEVER becomes a `wanted` title by a direct write. It routes through the **one** `suggest.Approver` implementation — the same gate a human admin uses. Per the locked decision, unattended additions are a **per-channel `AutoCurate` opt-in** (not on by default): when set, a channel-scoped auto-approve grant approves the refreshed proposal *because the channel opted in*, recording `approvedBy: "auto-curate"` for the audit trail, still **bounded by the quality bar + title cap**. A channel that hasn't opted in still gets re-curation *proposals* in the approval queue for an admin to act on — re-curation surfaces "N new titles fit this channel," it just doesn't auto-request them. Auto-curate is a *gated convenience*, never a gate bypass.
- **One approval path (no drift).** A constructed **`Approver`** is the sole public seam used by
  the manual handler, the per-user auto-approve grant, and auto-curate. It applies the edit and
  audit stamp, asks the binder to derive the channel from that exact proposal, and commits the
  proposal + insert-only title records + intent-bound channel as one compare-and-swap
  transaction. The binder's planning half performs no write; its post-commit half owns only
  derived codec work and the immediate external reconcile. This makes an approved proposal
  without its local channel unrepresentable and leaves the due `building` channel as the
  durable retry when Tunarr is unavailable. The policy refresh is **not** a wholesale overwrite:
  it goes through the single merge that (a) skips any proposal-owned field the operator has
  marked **operator-set** (§2.1 stickiness), (b) keeps `filler`/`window`/`autoCurate` always, and
  (c) merges `rules` by **provenance** (replace `llm`, keep `operator`).

  **Every channel mutation participates in one revision protocol.** The persisted channel carries
  a monotonic row revision independent of its display timestamp. A new row begins at revision 1;
  a full replacement must name the revision it read and atomically advances it, while a stale save
  changes nothing. Claims and targeted derived writes advance the same counter, so a stale full
  snapshot cannot restore an old lease, codec, Tunarr id, clip override, desired lineup, or policy.
  Approval treats a stale channel as a rolled-back/replan result: proposal, titles, and channel
  still commit together. The same transaction also rejects a candidate when that job already has
  an approved proposal created at the same persisted second or later: channel CAS protects a stale
  channel snapshot, but this second guard prevents an older decision from reading a newer snapshot
  and then validly CAS-ing the older lineup over it. Same-second proposal order is unknowable at the
  stored timestamp precision, so the gate fails closed rather than guessing. Reconcile likewise
  reloads and recomputes after a stale final write; its
  Tunarr checkpoint is a targeted compare-and-swap of the server id **and number actually used**, so
  an auto-renumber survives a later lineup failure without overwriting a concurrent operator edit.
  If two Postgres replicas create before either
  checkpoint wins, the loser removes its unattached remote id after the durable row proves that a
  different id won; process-local channel locks are not treated as replica coordination. The
  operator PATCH carries the revision shown
  to the editor and returns 409 rather than replaying a stale whole-lineup/policy replacement onto
  a state the operator never saw. This protocol is adopted by **every** channel-row writer; a
  writer that does not advance or validate the revision reintroduces the lost-update bug.
- **Cost-guarded + idempotent.** A channel whose intent-hash + lineup are unchanged since its last re-curation is skipped (the suggester's intent-hash cache), so a weekly run that finds nothing new makes no LLM call and no proposal. A re-curation that produces an identical proposal is a no-op end to end (same bind, no reconcile push — the §6.5 idempotency).

The safety posture mirrors the rest of §4/§8: re-curation can *loosen* content over time (add titles) but never *weakens* a gate — the audience ceiling (§4, and the kids-only default), the approval gate, and the per-channel opt-in all still hold. It grows a channel toward its intent; it can't turn a kids channel adult or spend acquisitions a human never authorized.

## 8.3. Adjacency candidates (a deterministic second corpus)

Re-curation (§8.2) asks the LLM "what else fits this intent?". That is the right question for *interpreting* an intent ("gritty 80s action, nothing campy") and a weak one for *finding adjacency* inside an already-established set — the model free-associates from the intent text, not from the channel's actual contents, and it costs a token spend to do it.

**A channel's own lineup is a query.** TMDB's `/{movie,tv}/{id}/recommendations` is a behavioural graph ("people who watched this also watched…"), and walking it from every title a channel already has, then counting **consensus**, yields candidates that are on-theme by construction. This is §1's prime principle applied to discovery: the LLM extracts intent, deterministic code finds neighbours.

- **Consensus is the signal, not any single edge.** One film's recommendations are noisy; a title recommended by *several* of the channel's films is reliably on-theme. Rank by that count. Measured against the dev "1980s Action Heroes" (25 films): the consensus head was Terminator 2, Mad Max, Death Race 2000, Demolition Man, Missing in Action, Romancing the Stone — including two obvious holes (the channel had Mad Max 2 + Beyond Thunderdome but not *Mad Max*; The Terminator but not *T2*).
- **⚠ `recommendations`, never `similar`.** The two endpoints sound interchangeable and are not: `similar` is computed from genre/keyword overlap and is effectively noise — the same probe returned *Land of the Blind* for Die Hard, The Terminator **and** RoboCop, and *A Man Escaped* for Die Hard. `recommendations` is behavioural and coherent (Terminator → Doomsday, Replicant, Hardware; RoboCop → Repo Men, Upgrade, Chappie). Building on `similar` would produce baffling channels that read as a Loomarr bug.
- **It explains itself.** A candidate carries *why*: "recommended by 5 of your films." That is a better approval-card reason than an LLM paraphrase, and it is reproducible — same lineup, same candidates, no tokens.
- **No new dependency and no new safety surface.** TMDB is already sanctioned (design.md §14) and the endpoint needs no additional API scope. Candidates enter as `catalog.Candidate` carrying a `Source` that records where they came from, then flow through the **existing** chokepoints unchanged: grounding, `scope`/`audience` filters (§4 fail-closed), the re-curation quality bar + title cap (§8.2), and the single `suggest.Approver` gate. Nothing about the approval model moves.
- **⚠ NOT a `/v1/search` scope.** `GET /v1/search?scope=library|tmdb|all` (design.md §7.2) is a published enum over *corpora a human can type a query against*. Adjacency has no query — it is derived from a channel's lineup — so exposing it there would mean an enum value that ignores `q`, which is the same shape as the phantom `clips` scope design.md §7.2 records as a leak corrected rather than implemented. The `Source` value is **internal provenance** (`Candidate.Source` already exists for "which corpus surfaced this, for debugging"), and the retrieval is a catalog method the re-curation path calls directly — not a search enum, and not a new endpoint.
- **Complementary, not a replacement — the corpora MERGE.** Every re-curation run consults both: the LLM proposes against intent, the graph proposes against contents, and the union is ranked and cut by the existing quality bar + title cap (§8.2). They cover each other's weak end — a channel with a thin lineup has a thin graph, which is exactly when the LLM is strongest; a mature channel has a rich graph, which is exactly when the LLM starts repeating itself. Merging rather than alternating keeps a single run's output complete: a weekly job that consulted only one corpus would make "what did re-curation find?" depend on the parity of the week.
- **Gated by the same `AutoCurate` opt-in.** Adjacency candidates are *cheaper* than LLM ones (deterministic, no tokens), and it is tempting to run them for every channel on that basis. They are not cheaper in **consequence**: a net-new adjacency pick is an acquisition, exactly like a net-new LLM pick, and it must clear the same quality bar, the same title cap, and the same `suggest.Approver` gate under the same per-channel opt-in. A channel that has not opted in still gets *proposals* in the approval queue for an admin to act on (§8.2), from either corpus. Cost is not the axis the gate turns on — spending a user's disk and bandwidth is.

**Where the merge happens — pre-seeding, not post-merging.** The suggester's grounding chokepoint is `surfaced`: a map of every candidate a tool call returned this run, and `buildProposal` accepts a pick **iff** it appears there (§1 — the model cannot smuggle in an id the catalog never returned). That constrains where a second corpus can enter. Merging *after* generation would mean appending picks the model never saw and the chokepoint never checked — the one thing grounding exists to prevent. So adjacency candidates are **pre-seeded into `surfaced` before generation**, and offered to the model in the refine framing alongside the current lineup. Two consequences worth stating:

- The model still *chooses*. Pre-seeding widens what it may pick from; it does not place anything. A pre-seeded candidate the model ignores is simply not picked, exactly like a searched one.
- Grounding is unweakened, because the candidates are real `catalog.Candidate`s with real ids that went through `backfillPresence` — the same shape a tool call produces. The chokepoint's invariant ("every pick traces to a candidate the catalog actually returned") holds verbatim; the set it checks against is just no longer built exclusively by model-initiated tool calls.

This also means adjacency **cannot** be a pure post-processing step bolted onto re-curation — it has to reach the suggester. A run for a channel with no `IntentRef` (hand-made, never suggested) therefore gets no adjacency pass either, the same restriction §8.2 already places on re-curation.

**The quality bar reads a score the model assigns — which is why an unscored adjacency pick needs a floor.** `recurate.min_score_pct` (§8.2) filters on each acquisition's `confidence`, and an omitted one unmarshals to `0`, which the filter deliberately treats as "never clears a positive bar" — the conservative direction for spending, and correct for a title the model searched for and then declined to stand behind. An adjacency pick is not that case: it was *handed* to the model carrying a consensus the model neither computed nor can see. A model that scores only what it found itself would silently zero out the entire second corpus, and the drop count would read as "the bar is working". So an **unscored** adjacency pick is credited exactly at the default bar and no higher — enough to be considered on its consensus, never enough to outrank a title the model actually endorsed. A **scored** one keeps the model's number in both directions: if the model looked and judged it weak, that judgement stands. This is a floor on an absent score, never a bypass — the title cap, the per-channel opt-in, and the single `suggest.Approver` gate are all unchanged.

⚠ **`confidence` was undefined in the prompt, and the distribution shows it.** Across 265 stored proposal items the values were 63% exactly `1.00`, 96% ≥ `0.80`, and nothing at all between `0` and `0.50` — the model was asserting, not calibrating, because the only mention of the field anywhere in the prompt was the bare `"confidence":<0..1>` token in the output schema. A bar filtering on an uncalibrated score is close to a no-op. The system prompt now defines the bands (0.9–1.0 squarely-what-was-asked → below 0.4 a stretch), says explicitly not to give everything 0.9+, and tells the model that a pick it was handed still needs its own honest score. This shapes **every** proposal, not just adjacency ones — it was a pre-existing weakness that building the second corpus happened to expose.

**Deliberately deferred: TMDB `keywords`.** The endpoint works and looks tempting for thematic grouping, but the vocabulary is mixed — a probe across the same channel returned real themes (`dystopia`, `alien`, `creature`, `atomic bomb`) interleaved with mood tags (`excited`, `suspenseful`, `aggressive`), a structural tag (`sequel`), and a location (`los angeles, california`). Using it raw would group films by mood and geography and call it a theme. It needs a curated allowlist or a discriminating heuristic first — that is its own design decision, not a detail of this one. Logged in §9 rather than half-built here.

## 8.4 Proposal and scheduler decision-trace boundary

Proposal explanations are immutable evidence from the suggestion/ranking seam. The trace
must carry the exact lexicographic rank tuple and tie-break inputs, plus bounded grounded
dispositions, but must not become a second scoring algorithm or include model rationale as
deterministic evidence. Approval edits and later channel state do not rewrite its meaning.
The tuple's integer relevance component and closed request/tone/era/include/exclude/refine match
booleans are v1's safe explicit-constraint evidence; raw request terms are not copied into each
candidate trace.

Scheduler explanations are a separate seam owned by `schedule.ComputeDesiredAt`. Each run
returns its slots and one `ScheduleTrace` produced in the same pass. The trace is current-cycle
evidence over that run's approved Lineup, live `Availability`, resolved `ChannelPolicy`, and
wall-clock; a proposal trace must never be relabeled as current-channel or scheduler evidence.

`ScheduleTrace` is an ordered, append-only fact stream with four stages:

1. `hard_filter` — channel scope/audience, active-rule scope, and seasonal decisions. Never-
   relaxed audience and explicit scope refusals retain their existing closed reason names.
2. `availability` — a movie or series resolves to playable program(s), or its approved entry
   becomes the configured `pod_fill` / `coming_soon` pending outcome. An empty series resolution
   is unavailable, not an unexplained episode-selection drop.
3. `episode_selection` — each safe, in-season episode is kept or omitted by `complete`,
   `highlights`, or `holiday`. A stale editorial cache and the highlights/holiday safety
   fallbacks are explicit `full_run_fallback` facts; they never masquerade as positive matches.
4. `placement` — the fact records the effective ordering mode, any relaxation, its pre-window
   deck position, whether the rolling window retained or rotated it out, and its final cycle
   position. Inserted commercial gaps are placement facts with no content key.

The trace header carries version, effective ordering, the signed 64-bit seed as a lossless
decimal string, resolved window duration, and the
coarse window index. It is deterministic for the same `ComputeDesiredAt` inputs. Facts are
capped at 1,024 while total counts continue; 256 retained slots are reserved for placement so
episode-heavy traces cannot crowd the final stage out. The API reports both totals plus `truncated`.
The two cycle-preview endpoints expose this exact trace beside `slots`; they do not infer it
from the final slot list. UI explanations group the closed facts into concise “why this airs”
and “why not” details, while the raw bounded facts remain available to evaluation and support
tooling. The trace contains no provider payloads, prompts, rationale, secrets, paths, location,
or chain-of-thought.

## 9. Extensibility — the checklist for "I'm sure I can think of more"

Every future heuristic is added the same way; a heuristic is *done* when all five exist:

1. **Policy field** — schema + default + per-channel override (config doc tier). **v1 substrate:** built-in Go constants supply the default and `policy_json` on the channel row holds the per-channel override — a two-tier `channel-policy > built-in`. The `config-design.md` registry-*default* middle tier (the third tier) is deferred until the settings registry lands; it slots into the policy resolver later without touching enforcement. Omitted fields resolve to the built-in constant; an omitted `ordering` resolves to the channel's `Strategy`.
2. **Extractor hint** — one line in the suggester's system prompt + template updates, if the LLM should infer it.
3. **Deterministic enforcer** — filter or slotting constraint in the lineup builder; ladder position if relaxable (or listed never-relaxed).
4. **Proposal surface** — chip + effect visibility in review.
5. **Tests** — binding + violation + relaxation + determinism.

Candidates already visible from here (logged, not v1): dayparting audience ceilings (stricter mornings), episode-quality floors (community ratings), "premiere" slotting for newly-landed backfill, per-holiday custom calendars, inter-channel dedup (don't air the same movie on two channels the same night), **thematic grouping from TMDB `keywords`** (blocked on separating real themes from mood tags and locations — see §8.3), **director/cast blocks from TMDB `credits`** ("a John Carpenter night" — the data is clean, the open question is whether it's a rule `What` or an ordering mode), and a **recency constraint** ("nothing re-aired within N days"), which needs an aired-history signal Loomarr does not yet record.

⚠ **A candidate source is not a heuristic** and does not take the five-point shape above — it has no policy field and no slotting enforcer, because it feeds the *proposal* pipeline (§8/§8.2), not the lineup builder. Its checklist is different: a `catalog.Scope` value, a corpus fetch, entry through the existing grounding + audience + approval chokepoints, and provenance the approval card can show. §8.3 is the worked example.

## 9.1. Discovery certification owns provider-resource uncertainty

The discovery evaluator follows the slice-1 seam in
[`channel-discovery-next.md`](engineering/plans/channel-discovery-next.md): one public `Runner`
owns grounded generation, materialized schedule outcomes, judging, and the versioned scorecard.
Runner and schedule certification tests inject the eval-only semantic recording `Judge`, exercise
the public `Judge.Score(ctx, JudgeEvidence)` seam, validate the bounded typed evidence received at
that seam, and assert the resulting scorecard behavior and call count. They never reconstruct
semantic evidence from a rendered prompt. Production `modelJudge` renderer and provider
request/response tests are a separate, supplemental wire layer: they can prove serialization,
routing, parsing, and attribution, but cannot certify schedule semantics or repair, fill in, or
rebound evidence that `Runner` supplied incorrectly. Prompt-substring assertions and private prompt
parsers are therefore forbidden as certification evidence.
The production suggester's provider adapter is therefore also the only honest place to enforce a
resource ledger around every repair and tool-loop call inside one `Suggest`.

Per-run limits reset for each case trial, while per-suite limits and uncertainty belong to the
whole `Runner.Run`. With hard token or hosted-USD ceilings, a provider call whose required usage is
missing makes the suite ledger permanently uncertain; no later generator, tool-loop turn, or judge
may start. Ollama remains explicitly non-billed, rather than being assigned a fictional charge.
Failure-stage ownership remains independent of that resource latch: the first retrieval or
generation diagnosis in the current trial is retained even if its provider response also makes
accounting uncertain, while later trials fail before generation at `budget_exhausted`. This keeps
the scorecard's closed first-failure vocabulary diagnostic without allowing uncertain paid work to
continue.

## 10. Tests (extends main doc §19 — these join the phase 10/11 gates)

- **Binding:** "seasons 1–10" policy → zero slots outside the range across many seeded cycles; era and genre filters likewise.
- **Collections bind, and fail OPEN (§2.2):** a `scope.collections` channel airs only stamped members; an entry whose membership is **unresolved** still airs (a down media server must not empty a lineup), while a *resolved* non-member is excluded. The heal is one-time — a fully-stamped channel makes ZERO library calls on the next pass.
- **Fail-closed audience:** unrated item + `TV-Y7` ceiling → excluded, counted in the exclusion report; `TV-MA` never appears under any kids ceiling *including after full ladder relaxation*.
- **Per-episode ceiling (§4):** one fixture must reach **three different outcomes** — an episode rated under the ceiling airs, one rated *above* it is dropped **even though its series cleared the entry gate**, and an *unrated* one airs by inheriting its parent. A test that cannot tell those apart proves nothing, and neither can one asserting on program *keys*: every episode of a series shares the series key, so assertions run on library item ids. Also pinned: a permissive parent never lifts a rated episode, and a series the ceiling empties yields **no slot at all** rather than a pending one.
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
