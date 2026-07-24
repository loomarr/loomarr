package schedule

import (
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// DesiredLineup is the ordered set of Slots a channel *should* have right now,
// computed from its approved lineup + current availability (§9). It is the
// "desired" half of desired-vs-actual reconciliation: the reconcile engine diffs
// it against Tunarr's actual channel state and applies the minimal calls.
type DesiredLineup struct {
	ChannelID string
	Slots     []Slot
	// Excluded reports what the audience/scope hard filters dropped (§4), surfaced
	// at reconcile + proposal so a metadata gap is visible. Empty when nothing was
	// filtered (or no policy is set).
	Excluded ExclusionReport
	// Applied records the relaxation-ladder steps taken this computation (§7),
	// written back onto the channel's policy by the reconcile engine and surfaced in
	// the UI. Empty when the eligible pool satisfied the policy with no relaxation.
	Applied []AppliedRelaxation
	// EligibleKeys is the set of program keys the library can currently supply for this
	// channel — the resolved programs BEFORE the rolling-window truncation and any rule
	// narrowing removed some from the aired slice (§6.5). Drift detection compares
	// against THIS, not Slots: a key that left Slots but is still eligible was rotated
	// out by the window/rule (normal, not drift); a key gone from Eligible truly vanished
	// from the library (real drift → StatusDrifted).
	EligibleKeys map[provision.Key]bool
}

// ProgramCount returns how many slots are real playable programs (§9: a channel
// with zero programs must still be live via filler/flex — never dead air).
func (d DesiredLineup) ProgramCount() int {
	n := 0
	for _, s := range d.Slots {
		if s.IsProgram() {
			n++
		}
	}
	return n
}

// Availability answers, for a provisioning Key, whether the title is available
// and (if so) its library item id. The reconcile engine backs this with the
// store/library; pure tests back it with a map. Mirrors §9's "compute desired
// from the approved lineup + live availability".
type Availability interface {
	// Resolve returns (libraryItemID, durationMs, true) if key is available now,
	// else ("", 0, false). durationMs is the program runtime from the library (§9,
	// §10 RunTimeTicks) — a program slot needs it (Tunarr rejects duration ≤ 0).
	// It must be side-effect free from the caller's view.
	//
	// For a SERIES key, Resolve returns available=false: a show isn't directly
	// playable (no single item/runtime). Series resolve via ResolveEpisodes.
	Resolve(key provision.Key) (libraryItemID string, durationMs int64, available bool)

	// ResolveEpisodes expands a SERIES key into its episode programs, in
	// season/episode order (§9 series expansion). Returns (nil, false) for a
	// non-series key or a series with no playable episodes yet (→ pending slot).
	// Each episode carries its own library item id + duration. Side-effect free.
	ResolveEpisodes(key provision.Key) (episodes []ResolvedProgram, available bool)
}

// ResolvedProgram is one concrete playable program (a movie or a single episode)
// with the fields a program Slot needs. Used by series expansion (§9). Season is
// the episode's season number (0 for a movie / unknown), used to apply a lineup
// entry's optional season range.
type ResolvedProgram struct {
	LibraryItemID string
	Title         string
	DurationMs    int64
	Season        int
	// Episode / EpisodeEnd are the episode number and (for a single-file multi-part
	// item) its last spanned number — the media server's IndexNumber/IndexNumberEnd.
	// Feed multi-part detection (§5): 0 = unknown/movie. EpisodeEnd 0 = single episode.
	Episode    int
	EpisodeEnd int
	// PartGroup / PartIndex are assigned by assignPartGroups (§5 multi-part): all parts of
	// one two-parter share a PartGroup, PartIndex is their 1-based play order. "" = standalone.
	// resolveEntry copies these onto the program Slot so the deck keeps a group atomic.
	PartGroup string
	PartIndex int
}

// PendingPolicy is what to place where a lineup entry's title is not yet
// available (§9 pending-slot policy). Default is pod-fill.
type PendingPolicy string

const (
	// PodFill: fill the gap with matched filler so the timeline is continuous
	// (§9 default — "never dead air").
	PodFill PendingPolicy = "pod_fill"
	// ComingSoon: leave an explicit pending slot (a "coming soon" interstitial
	// card is a config alternative, §9). The slot stays SlotPending.
	ComingSoon PendingPolicy = "coming_soon"
)

// LineupEntry is one item of a channel's *approved* lineup (from a proposal,
// §8): the intent-level "this should play here". Availability decides whether it
// becomes a program slot or a pending slot this reconcile.
type LineupEntry struct {
	Key        provision.Key // provisioning key of the intended title
	Title      string        // display label
	DurationMs int64         // known runtime, if any (0 = unknown)
	// SeasonMin/SeasonMax optionally constrain a SERIES entry to a season range
	// (inclusive; 0 = unbounded on that end) — an intent-level filter such as
	// "old-school Simpsons" (§9 series expansion). Ignored for movies.
	SeasonMin int
	SeasonMax int
	// Policy-enforcement metadata, stamped from the grounded ProposalItem at
	// channel-create time (programming-design §4): the audience filter, era/genre
	// scope, and runtime cap read these off the entry so enforcement stays a pure
	// entry-set filter with no per-reconcile library I/O. For a series these are the
	// SERIES' values (v1 series-level ceiling; episodes carry no per-episode rating).
	OfficialRating Rating   // media-server content rating, normalized at enforcement
	Genres         []string // for genre scope + seasonal keyword matching
	Year           int      // for era scope + seasonal windowing
	RuntimeSec     int      // for the runtimeMax cap; 0 = unknown (not filtered on runtime)
	// CollectionID is the TMDB collection (franchise) a MOVIE belongs to (§5 franchise
	// ordering): films sharing a CollectionID > 0 are kept together, in release-year order,
	// as an atomic block. Tri-state so the reconcile heal is a ONE-TIME repair, not a
	// per-sweep TMDB call: 0 = not resolved yet (heal it); >0 = a real collection id;
	// -1 = resolved, standalone (no collection) — a settled answer, never re-fetched.
	// assignFranchiseGroups groups only CollectionID > 0. Healed at reconcile like
	// OfficialRating, so the scheduler reads it with no I/O.
	CollectionID int
}

// inSeasonRange reports whether an episode season falls within the entry's
// optional [SeasonMin, SeasonMax] (0 = unbounded on that end).
func (e LineupEntry) inSeasonRange(season int) bool {
	if e.SeasonMin > 0 && season < e.SeasonMin {
		return false
	}
	if e.SeasonMax > 0 && season > e.SeasonMax {
		return false
	}
	return true
}

// ComputeDesired turns an approved lineup + current availability into a
// DesiredLineup under a strategy and pending policy (§9). It is PURE: same
// inputs (including the channel's shuffle seed) always yield the same slots, so
// the reconcile engine's output is reproducible and tests need no mock Tunarr.
//
// Ordering:
//   - Sequential: entries stay in approved order.
//   - Shuffle: entries are permuted by a seed derived from the channel (§9/§10
//     determinism) — the same channel reshuffles identically every reconcile,
//     so a live channel's guide is stable, not re-randomized on every sweep.
//   - TimeSlot: block assignment is applied by the caller's block plan; here we
//     keep approved order and let the reconcile layer map onto Tunarr time
//     slots. (v1: treated as Sequential for slot derivation; block math is
//     applied at push time.)
//
// Availability mapping (per entry):
//   - available            → SlotProgram (LibraryItemID set)
//   - not available, PodFill → SlotFiller (pod-fill placeholder; §10 fills it)
//   - not available, ComingSoon → SlotPending (explicit gap)
//
// ComputeDesired is the policy-free entry point (an empty ChannelPolicy + zero
// clock): existing callers and tests get byte-identical behavior to the old blind
// ordering. Reconcile calls ComputeDesiredAt with the channel's policy + a clock.
func ComputeDesired(ch Channel, entries []LineupEntry, avail Availability, policy PendingPolicy) DesiredLineup {
	return ComputeDesiredAt(ch, entries, avail, policy, ChannelPolicy{}, time.Time{})
}

// ComputeDesiredAt is the ChannelPolicy-aware desired-lineup builder (programming-
// design §3–§7). It is PURE — same inputs (channel seed + policy + clock) always
// yield the same slots — so reconcile output is reproducible and tests need no mock
// Tunarr. `now` drives seasonality (§6); a zero `now` disables it (seasonal mode
// treated as off), which is why the policy-free wrapper passes time.Time{}.
//
// Pipeline: filter entries (scope + audience §4, never-relaxed) → seasonal bench
// (§6) → resolve each entry to slot(s) → policy-aware slotting (ordering §5 +
// separation-with-wrap §3) → relaxation ladder on shortfall (§7) → interleave
// breaks (§10, unchanged). The channel's Strategy supplies the ordering when the
// policy omits it (a policy-less channel keeps its existing behavior).
func ComputeDesiredAt(ch Channel, entries []LineupEntry, avail Availability, pending PendingPolicy, policy ChannelPolicy, now time.Time) DesiredLineup {
	// Curation rules (§6.5): pick the active rule for this wall-clock, if any. Its WHAT
	// narrows the pool and its HOW/Window override ordering + the horizon. No matching
	// rule ⇒ the channel's base whole-policy behavior (rule == zero value).
	rule, _ := pickRule(policy.Rules, now)

	rp := policy.Resolved(ch.Strategy, singleSeriesEntries(entries))
	rp = applyRuleHow(rp, rule.How) // overlay the active rule's ordering/separation

	// Hard filters first (§4 audience fail-closed + scope) — the never-relaxed gate.
	// The rule's WHAT can only NARROW this set (applied next), never widen it. This
	// channel-eligible set (post-audience/scope, PRE-rule-narrowing) is what "the
	// library can supply for this channel" means for drift detection (§6.5): a program
	// rotated out by a rule or the window is still eligible; only a library loss makes a
	// key drop out of it.
	channelEligible, report := filterEntries(entries, rp)

	// Rule WHAT: intersect the channel scope with the active rule's scope (§6.5). This
	// is what makes "weekend = only TNG" or "mornings = kids genre" work — it can only
	// subtract from the already-audience-safe channel-eligible set.
	eligible := applyRuleScope(channelEligible, rule.What)
	// Seasonal bench/boost (§6): out-of-window seasonal items are benched (removed);
	// in-window ones are marked for a scheduling boost. A zero clock ⇒ no-op.
	eligible, seasonalReport := applySeasonal(eligible, rp, now)
	report.merge(seasonalReport)

	// Franchise grouping (§5): tag movie entries sharing a TMDB collection so a franchise
	// (Raiders → Temple of Doom → Last Crusade) plays together, in release-year order, as an
	// atomic block — the movie analogue of the multi-part episode floor. Assigned on the
	// entries here (they carry CollectionID + Year); resolveEntry copies the tags onto each
	// movie slot, and the same collapse/expand keeps the block whole under shuffle/window.
	franchiseGroups := assignFranchiseGroups(eligible)

	// Resolve each eligible entry to its program slot(s) — movie → one, series → one
	// per in-range episode (§9 expansion).
	slots := make([]Slot, 0, len(eligible))
	for _, e := range eligible {
		slots = append(slots, resolveEntry(e, avail, pending, franchiseGroups)...)
	}

	// EligibleKeys: the distinct program keys the library can currently supply, captured
	// from the CHANNEL-eligible set (post-audience/scope, but PRE-rule-narrowing, PRE-
	// seasonal-bench, and PRE-window-truncation). The reconcile drift check compares against
	// THIS, so a key rotated out by the active rule, a seasonal bench, or the rolling window
	// reads as "rotated" (normal), not "vanished" (drift). We resolve `channelEligible`
	// directly rather than reusing the aired `slots` precisely because `slots` is already
	// post-rule/post-seasonal — reusing it would mis-flag a benched-but-in-library program
	// as drift. A pending/filler placeholder is not "supplied", so only program slots count.
	eligibleKeys := make(map[provision.Key]bool, len(channelEligible))
	for _, e := range channelEligible {
		// Franchise grouping is irrelevant to key eligibility (we only collect keys here), so
		// pass nil — grouping only shapes the aired ORDER, not which titles are supplied.
		for _, s := range resolveEntry(e, avail, pending, nil) {
			if s.IsProgram() {
				eligibleKeys[s.Key] = true
			}
		}
	}

	// Rolling window (§6.5): resolve the horizon (rule > channel > global default) and
	// the coarse window index for this moment. The index folds into the deck seed so the
	// ordering is IDENTICAL within a window (idempotent reconcile — no re-push every
	// sweep) and ADVANCES to the next slice of episodes at a window boundary.
	window := resolveWindow(rule.Window, policy.Window, ch.DefaultWindow)
	seed := ch.Shuffle.Seed ^ windowIndex(now, window)

	// Multi-part atomicity (§5): collapse each two-parter into one super-slot so ordering
	// can't split or reorder its parts and the window counts the group as one unit. Ordering
	// + truncation run on the collapsed deck; expandGroups restores the real parts (adjacent,
	// in PartIndex order) afterward.
	collapsed, expand := collapseGroups(slots)

	// Policy-aware ordering + separation (§3/§5), then the relaxation ladder (§7)
	// records any windows it had to loosen to fill the cycle.
	ordered, applied := slotWithRelaxation(collapsed, rp, seed)

	// Truncate the ordered deck to ~window of runtime (§6.5) — materialize a manageable
	// horizon, not the whole ~800-episode run. window <= 0 (WindowFull / unbounded) keeps
	// the whole deck (backward compat + the "full binge" sentinel). A super-slot carries the
	// group's TOTAL runtime, so a two-parter is kept whole or dropped whole by the budget.
	ordered = truncateToWindow(ordered, window)

	// Expand super-slots back into their real parts now that ordering + windowing are done.
	ordered = expandGroups(ordered, expand)

	// Breaks: the active rule may suppress them (a marathon runs uninterrupted, §6.5).
	brk := ch
	if rule.How.NoBreaks {
		brk.BreaksPerHour = 0
	}

	return DesiredLineup{
		ChannelID:    ch.ID,
		Slots:        interleaveBreaks(brk, ordered),
		Excluded:     report,
		Applied:      applied,
		EligibleKeys: eligibleKeys,
	}
}

// resolveWindow picks the effective rolling-window horizon: a rule's Window wins, then
// the channel policy's Window, then the global default. WindowFull (< 0) at any level
// means "no truncation" (the whole run). 0 means "inherit the next level"; a 0 at the
// global default too ⇒ unbounded (the policy-free path preserves today's behavior).
func resolveWindow(ruleW, channelW Duration, defaultW time.Duration) time.Duration {
	pick := func(d Duration) (time.Duration, bool) {
		switch {
		case d == WindowFull:
			return 0, true // explicit "whole run" → no truncation
		case d > 0:
			return d.Std(), true
		default:
			return 0, false // 0 → inherit
		}
	}
	if w, ok := pick(ruleW); ok {
		return w
	}
	if w, ok := pick(channelW); ok {
		return w
	}
	if defaultW > 0 {
		return defaultW
	}
	return 0 // unbounded
}

// windowIndex is the coarse, deterministic index of the window `now` falls in — a
// floor of the wall-clock by the window duration. Constant within a window (so two
// reconciles in the same window fold the SAME value into the seed → identical order →
// no Tunarr re-push) and +1 across a boundary (advancing the deck to the next slice).
// A zero window or zero clock ⇒ 0 (no advance), preserving the un-windowed behavior.
func windowIndex(now time.Time, window time.Duration) int64 {
	if window <= 0 || now.IsZero() {
		return 0
	}
	return now.Unix() / int64(window/time.Second)
}

// truncateToWindow keeps the deck prefix whose summed program runtime first meets the
// window (§6.5). window <= 0 ⇒ the whole deck (unbounded). Always keeps at least one
// program so a channel is never dark, even if that one program exceeds the window.
func truncateToWindow(slots []Slot, window time.Duration) []Slot {
	if window <= 0 || len(slots) == 0 {
		return slots
	}
	budget := window.Milliseconds()
	var acc int64
	kept := 0
	for i, s := range slots {
		kept = i + 1
		if s.IsProgram() {
			acc += s.DurationMs
			if acc >= budget {
				break
			}
		}
	}
	return slots[:kept]
}

// breakGapMs is the placeholder duration for an inserted commercial break — the
// pod assembler sizes the actual pod to this gap (§10 default 2-minute break).
const breakGapMs = 120_000

// interleaveBreaks inserts SlotFiller break gaps between program slots at the
// channel's BreaksPerHour density (§10). Runtime-aware and snapped to program
// boundaries (Tunarr only breaks between programs): walk the slots accumulating
// program runtime, and after a program that pushes the running total past the
// next 60/BreaksPerHour-minute threshold, emit one break and reset. Never a
// trailing break (nothing after it to return from) and never two in a row.
func interleaveBreaks(ch Channel, slots []Slot) []Slot {
	if ch.BreaksPerHour <= 0 || len(slots) == 0 {
		return slots
	}
	thresholdMs := int64(60*60*1000) / int64(ch.BreaksPerHour) // ms of runtime per break
	out := make([]Slot, 0, len(slots)+len(slots)/2)
	var acc int64
	for i, s := range slots {
		out = append(out, s)
		if !s.IsProgram() {
			continue // only program runtime counts toward the break cadence
		}
		acc += s.DurationMs
		// A break only makes sense between two programs — not after the last slot.
		hasLater := false
		for _, n := range slots[i+1:] {
			if n.IsProgram() {
				hasLater = true
				break
			}
		}
		if acc >= thresholdMs && hasLater {
			out = append(out, Slot{Kind: SlotFiller, DurationMs: breakGapMs})
			acc = 0
		}
	}
	return out
}

// resolveEntry turns one approved lineup entry into its desired slot(s). franchise carries
// the movie-franchise group tags (§5) computed across the eligible set; a movie whose Key is
// in it gets a PartGroup so its franchise stays together. Nil/absent ⇒ no franchise grouping.
func resolveEntry(e LineupEntry, avail Availability, policy PendingPolicy, franchise map[provision.Key]franchiseTag) []Slot {
	// A series expands into its episodes (each a program slot).
	if e.Key.IsSeries() {
		if eps, ok := avail.ResolveEpisodes(e.Key); ok && len(eps) > 0 {
			// Keep only the in-range episodes (in season/episode order), then tag multi-part
			// groups over that ordered set (§5) so consecutive-episode detection sees the
			// real neighbors. inRange preserves order, so parts stay contiguous for detection.
			inRange := eps[:0:0]
			for _, ep := range eps {
				if e.inSeasonRange(ep.Season) { // outside the entry's season range → drop
					inRange = append(inRange, ep)
				}
			}
			assignPartGroups(string(e.Key), inRange)
			out := make([]Slot, 0, len(inRange))
			for _, ep := range inRange {
				out = append(out, Slot{
					Kind:          SlotProgram,
					Key:           e.Key,
					LibraryItemID: ep.LibraryItemID,
					Title:         ep.Title,
					DurationMs:    ep.DurationMs,
					PartGroup:     ep.PartGroup,
					PartIndex:     ep.PartIndex,
				})
			}
			if len(out) > 0 {
				return out
			}
			// The range matched no in-library episodes yet → pending (like an
			// unavailable series), not an empty channel.
		}
		// Series with no playable (in-range) episodes yet → a single placeholder.
		return []Slot{pendingSlot(e, policy)}
	}

	// A movie (or any non-series) resolves to a single program slot.
	if itemID, durationMs, ok := avail.Resolve(e.Key); ok {
		if durationMs == 0 {
			durationMs = e.DurationMs // fall back to the entry's own duration
		}
		slot := Slot{
			Kind:          SlotProgram,
			Key:           e.Key,
			LibraryItemID: itemID,
			Title:         e.Title,
			DurationMs:    durationMs,
		}
		// A movie in a franchise group carries its collection PartGroup + release-order
		// index, so collapse/expand keeps the franchise together, in order (§5).
		if tag, ok := franchise[e.Key]; ok {
			slot.PartGroup, slot.PartIndex = tag.group, tag.index
		}
		return []Slot{slot}
	}
	return []Slot{pendingSlot(e, policy)}
}
