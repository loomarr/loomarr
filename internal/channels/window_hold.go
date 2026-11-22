package channels

import (
	"encoding/json"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// dealtFrom is "<edited>.<dealt>", two fingerprints of a channel's deal (#4140 step 1).
//
// <edited> is what an operator decides: the lineup (which titles, in order, and the seasons and
// episodes each may air), the channel's policy without the relaxations reconcile writes back, its
// ordering, and the install's break, window and calendar settings (guide time zone, home country,
// holiday definitions). Library facts are left out on purpose,
// including the ratings and collections reconcile heals onto lineup entries: they change through
// the day without anyone deciding anything, and a change in them must not read as an edit.
//
// <dealt> is the deal those inputs and the library produce, which only a window that never turns
// reads: there a library change re-deals once, and the next sweep, dealing the same, keeps it.
//
// breaksPerHour is the channel's effective break density, which is 0 when it has no filler to air
// (a taxonomy edit or an emptied catalog). Whether breaks exist at all is decided there, so a
// channel that gains or loses its filler re-deals after the programme on air rather than airing
// empty breaks, or none, until the window turns.
func (e *Engine) dealtFrom(ch channelstore.Channel, breaksPerHour int, deal []schedule.Slot) string {
	h := fnv.New64a()
	blob, _ := json.Marshal(deal) // slots are plain values: a marshal cannot fail
	_, _ = h.Write(blob)
	return e.edited(ch, breaksPerHour) + "." + strconv.FormatUint(h.Sum64(), 16)
}

// edited is dealtFrom's <edited> half. It hashes JSON, not %v: policies hold optional fields
// behind pointers, which %v prints as addresses that differ on every read.
func (e *Engine) edited(ch channelstore.Channel, breaksPerHour int) string {
	type entry struct {
		Key                  string
		SeasonMin, SeasonMax int
		Episodes             schedule.EpisodeSelection
	}
	entries := make([]entry, 0, len(ch.Lineup))
	for _, l := range ch.Lineup {
		entries = append(entries, entry{string(l.Key), l.SeasonMin, l.SeasonMax, l.EpisodeSelection})
	}
	cal := e.Calendar()
	zone := ""
	if cal.Zone != nil {
		zone = cal.Zone.String()
	}
	blob, err := json.Marshal(struct {
		Lineup        []entry
		Policy        schedule.ChannelPolicy
		Strategy      schedule.Strategy
		BreaksPerHour int
		BreakLength   time.Duration
		Window        time.Duration
		Install       schedule.PendingPolicy
		Zone, Country string
		Holidays      any
	}{entries, ch.Policy.Decided(), ch.Strategy, breaksPerHour, e.breakDurationFor(), e.defaultWindowFor(), e.policy,
		zone, cal.Country, cal.Holidays})
	if err != nil {
		return "" // the same "" every sweep, so a policy that cannot marshal never reads as an edit
	}
	h := fnv.New64a()
	_, _ = h.Write(blob)
	return strconv.FormatUint(h.Sum64(), 16)
}

// windowDeal is what reconcile commits for an internal channel whose window has not turned: the
// accepted cycle, healed, when only library facts changed; or, after an edit, the programme on air
// followed by the new deal, from a new anchor at that programme's start. Either way nothing that
// has aired moves, so the scheduled ledger, which walks the accepted cycle from its anchor, records
// only what aired. A window that never turns has no next window to wait for, so a library change
// there re-deals after the programme on air as an edit does.
//
// An edit that leaves a kept programme ineligible (a tightened ceiling, a title taken out, a
// channel filtered to nothing) is not kept: the new deal airs from the anchor at once, as it did
// before the hold.
func (e *Engine) windowDeal(plan windowInputs) ([]schedule.Slot, time.Time) {
	wasEdited, wasDealt, _ := strings.Cut(plan.acceptedFrom, ".")
	edited, dealt, _ := strings.Cut(plan.dealtFrom, ".")
	bounded := !plan.windowEnds.IsZero()
	unchanged := plan.acceptedFrom == "" || wasEdited == edited && (bounded || wasDealt == dealt)
	if unchanged {
		return healAccepted(plan.accepted, plan.anchor, plan.now, plan.avail), plan.anchor
	}
	until := plan.now.Add(timeline.OnAirLookahead)
	if !hasProgramme(plan.next) {
		return plan.next, plan.anchor
	}
	aired := map[string]bool{}
	for _, b := range timeline.BroadcastsBetween(plan.accepted, plan.anchor, plan.now, until) {
		if b.Kind != schedule.SlotProgram {
			continue
		}
		if plan.eligible != nil && !plan.eligible[b.Key] {
			return plan.next, plan.anchor
		}
		aired[b.LibraryItemID] = true
	}
	from := plan.windowOpened
	if from.IsZero() || from.Before(plan.anchor) {
		from = plan.anchor
	}
	for _, b := range timeline.BroadcastsBetween(plan.accepted, plan.anchor, from, plan.now) {
		if b.Kind == schedule.SlotProgram && b.Start.Before(plan.now) {
			aired[b.LibraryItemID] = true
		}
	}
	tail := timeline.WithoutProgrammes(plan.next, aired)
	if !hasProgramme(tail) {
		tail = plan.next // everything the deal holds aired today: it airs again rather than nothing
	}
	// Keep from the anchor when the new cycle airs to the window's end without looping; otherwise
	// the kept part would air again, so the cycle starts at the programme on air instead.
	for _, fromAnchor := range []bool{!plan.windowEnds.IsZero(), false} {
		kept, start := timeline.KeepOnAir(plan.accepted, plan.anchor, plan.now, until, fromAnchor)
		if len(kept) == 0 {
			return plan.next, plan.anchor
		}
		kept = healAccepted(kept, start, plan.now, plan.avail)
		if !hasBreak(plan.next) {
			kept = withoutBreaksFrom(kept, start, until) // no filler to air: programmes run back to back
		}
		spliced := append(kept, tail...)
		if !fromAnchor || !start.Add(timeline.CycleLength(spliced)).Before(plan.windowEnds) {
			return spliced, start
		}
	}
	return plan.next, plan.anchor // unreachable: the second pass always returns
}

// windowInputs is what windowDeal reads.
type windowInputs struct {
	accepted     []schedule.Slot
	anchor       time.Time
	next         []schedule.Slot
	now          time.Time
	windowOpened time.Time
	windowEnds   time.Time // when the window on air hands over; zero for a window that never turns
	acceptedFrom string    // dealtFrom as the accepted cycle recorded it; "" before one was recorded
	dealtFrom    string    // dealtFrom as the channel stands
	eligible     map[provision.Key]bool
	avail        schedule.Availability
}

// healAccepted is the accepted cycle with each programme that has not finished airing and whose
// library item has vanished healed in place: a re-identified file (the same film, or the same
// season and episode) takes the new item id, and anything else becomes a placeholder at the same
// runtime, which airs filler. A placeholder whose title resolves again (a mount back after a
// flicker, a film that arrived) becomes the programme again in the same way. Timing never
// changes, so nothing else moves.
func healAccepted(accepted []schedule.Slot, anchor, now time.Time, avail schedule.Availability) []schedule.Slot {
	from := timeline.FirstUnfinished(accepted, anchor, now)
	var out []schedule.Slot
	for i, s := range accepted {
		placeholder := s.Kind == schedule.SlotFiller && s.Key != ""
		if i < from || s.Key == "" || s.Kind != schedule.SlotProgram && !placeholder {
			continue
		}
		healed, ok := healSlot(s, avail)
		if ok {
			continue
		}
		if out == nil {
			out = append([]schedule.Slot(nil), accepted...)
		}
		out[i] = healed
	}
	if out == nil {
		return accepted
	}
	return out
}

// healSlot reports (s, true) when s needs no heal: its library item still resolves, or it is a
// placeholder whose title still does not. Otherwise it returns the slot that replaces it.
func healSlot(s schedule.Slot, avail schedule.Availability) (schedule.Slot, bool) {
	placeholder := s.Kind == schedule.SlotFiller
	if id, _, ok := avail.Resolve(s.Key); ok {
		if id == s.LibraryItemID && !placeholder {
			return s, true
		}
		s.Kind, s.LibraryItemID = schedule.SlotProgram, id
		return s, false
	}
	var same string
	for _, p := range avail.ResolveEpisodes(s.Key).Programs {
		if p.LibraryItemID == s.LibraryItemID && !placeholder {
			return s, true
		}
		if same == "" && s.Episode > 0 && p.Season == s.Season && p.Episode == s.Episode {
			same = p.LibraryItemID
		}
	}
	if same != "" {
		s.Kind, s.LibraryItemID = schedule.SlotProgram, same
		return s, false
	}
	if placeholder {
		return s, true
	}
	s.Kind, s.LibraryItemID = schedule.SlotFiller, ""
	return s, false
}

// isBreak reports a commercial break; a keyed filler slot is a placeholder, not a break.
func isBreak(s schedule.Slot) bool { return s.Kind == schedule.SlotFiller && s.Key == "" }

func hasBreak(slots []schedule.Slot) bool {
	for _, s := range slots {
		if isBreak(s) {
			return true
		}
	}
	return false
}

// withoutBreaksFrom is kept, walked from start, without the breaks at its end that start at or
// after `until`: they have not aired and nothing is on air in them.
func withoutBreaksFrom(kept []schedule.Slot, start, until time.Time) []schedule.Slot {
	for n := len(kept); n > 0 && isBreak(kept[n-1]); n-- {
		if start.Add(timeline.CycleLength(kept[:n-1])).Before(until) {
			return kept[:n]
		}
		kept = kept[:n-1]
	}
	return kept
}

func hasProgramme(slots []schedule.Slot) bool {
	for _, s := range slots {
		if s.Kind == schedule.SlotProgram {
			return true
		}
	}
	return false
}
