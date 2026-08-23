package playout

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

func prog(title, itemID string, mins int) schedule.Slot {
	return schedule.Slot{
		Kind: schedule.SlotProgram, Title: title, LibraryItemID: itemID,
		DurationMs: int64(mins) * 60_000,
	}
}

func breakGap() schedule.Slot { return schedule.Slot{Kind: schedule.SlotFiller} }

var epoch = time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)

// A channel is a WALL CLOCK, not a playlist that starts when someone watches. Someone tuning
// in 40 minutes into a 60-minute show must land 40 minutes in.
func TestAiringAt_MidProgramTuneInLandsAtTheRightOffset(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), prog("Predator", "b", 30)}

	got := AiringAt(slots, epoch, epoch.Add(40*time.Minute))
	if got.LibraryItemID != "a" {
		t.Fatalf("at +40m expected Heat, got %q", got.Title)
	}
	if got.Offset != 40*time.Minute {
		t.Errorf("offset = %v, want 40m — a joiner must not restart the show", got.Offset)
	}
	if got.Remaining != 20*time.Minute {
		t.Errorf("remaining = %v, want 20m", got.Remaining)
	}
	if got.StartedAt != epoch || got.Identity != "a" {
		t.Errorf("identity = %q at %v, want a at %v", got.Identity, got.StartedAt, epoch)
	}
}

// THE property the shared-encoder model rests on: two callers asking at the same instant get
// the same item at the same offset. One encode serves N viewers, so if this were unstable the
// viewers would be watching different things through one pipe.
func TestAiringAt_IsStableForTheSameInstant(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), breakGap(), prog("Predator", "b", 30)}
	at := epoch.Add(73 * time.Minute)

	first := AiringAt(slots, epoch, at)
	for i := 0; i < 20; i++ {
		if got := AiringAt(slots, epoch, at); got != first {
			t.Fatalf("call %d disagreed: %+v vs %+v", i, got, first)
		}
	}
}

// The cycle repeats — that is what makes a channel continuous without an infinite lineup.
func TestAiringAt_CycleRepeats(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), prog("Predator", "b", 30)}

	// 90m cycle. At +95m we are 5m into the second pass, i.e. Heat again.
	got := AiringAt(slots, epoch, epoch.Add(95*time.Minute))
	if got.LibraryItemID != "a" || got.Offset != 5*time.Minute {
		t.Errorf("at +95m want Heat @5m, got %q @%v", got.Title, got.Offset)
	}
	// And a channel running for weeks stays correct.
	got = AiringAt(slots, epoch, epoch.Add(3*7*24*time.Hour+10*time.Minute))
	if got.LibraryItemID != "a" || got.Offset != 10*time.Minute {
		t.Errorf("after three weeks want Heat @10m, got %q @%v", got.Title, got.Offset)
	}
}

// Break gaps occupy real time. A channel with breaks must not have them collapse to zero, or
// every program after the first airs at the wrong wall-clock.
func TestAiringAt_BreakGapsOccupyTime(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), breakGap(), prog("Predator", "b", 30)}

	// 60m program, then the break.
	got := AiringAt(slots, epoch, epoch.Add(60*time.Minute+5*time.Second))
	if got.Kind != schedule.SlotFiller {
		t.Errorf("just after the first program expected a filler gap, got %s (%q)", got.Kind, got.Title)
	}
	// The second program starts AFTER the gap, not at +60m.
	got = AiringAt(slots, epoch, epoch.Add(60*time.Minute+fillerSlotDuration+time.Second))
	if got.LibraryItemID != "b" {
		t.Errorf("after the gap expected Predator, got %q", got.Title)
	}
}

// A pending acquisition has unknown duration. Treating it as instantaneous would drift the
// cycle; treating it as some default would air silence for a made-up length. It is skipped.
func TestAiringAt_PendingSlotsAreNotAirable(t *testing.T) {
	slots := []schedule.Slot{
		{Kind: schedule.SlotPending, Title: "Con Air"}, // DurationMs 0
		prog("Heat", "a", 60),
	}
	got := AiringAt(slots, epoch, epoch)
	if got.LibraryItemID != "a" {
		t.Errorf("a pending slot must be skipped, got %q (%s)", got.Title, got.Kind)
	}
	if got.Offset != 0 {
		t.Errorf("skipping must not consume time: offset = %v", got.Offset)
	}
}

// An empty or wholly-unairable lineup is the offline card, not dead air or a panic.
func TestAiringAt_NothingAirableReportsFlex(t *testing.T) {
	for name, slots := range map[string][]schedule.Slot{
		"empty":       {},
		"all pending": {{Kind: schedule.SlotPending}, {Kind: schedule.SlotPending}},
	} {
		got := AiringAt(slots, epoch, epoch.Add(time.Hour))
		if got.Playable() {
			t.Errorf("%s: reported something playable: %+v", name, got)
		}
		if got.Kind != schedule.SlotFlex {
			t.Errorf("%s: kind = %s, want flex (the offline card)", name, got.Kind)
		}
	}
}

// A clock that moves backwards (NTP correction, DST) must wrap rather than go negative — a
// negative offset would seek before the start of the file.
func TestAiringAt_ClockBeforeEpochDoesNotGoNegative(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60)}
	got := AiringAt(slots, epoch, epoch.Add(-30*time.Minute))
	if got.Offset < 0 {
		t.Errorf("negative offset %v would seek before the file start", got.Offset)
	}
	if !got.Playable() {
		t.Errorf("a pre-epoch clock should still play something, got %+v", got)
	}
}

// Playable is the guard callers use before spawning an encode; it must reject the cases that
// have no input.
func TestAiring_PlayableRejectsUnstreamableSlots(t *testing.T) {
	if (Airing{Kind: schedule.SlotProgram, LibraryItemID: ""}).Playable() {
		t.Error("a program slot with no item id is not playable")
	}
	if (Airing{Kind: schedule.SlotFiller, LibraryItemID: "x"}).Playable() {
		t.Error("a filler slot is not a program — it resolves to a clip, not a library item")
	}
	if !(Airing{Kind: schedule.SlotProgram, LibraryItemID: "x"}).Playable() {
		t.Error("a program with an item id must be playable")
	}
	if !(Airing{Kind: schedule.SlotFiller, Source: "/clip.mp4"}).Playable() {
		t.Error("a resolved filler clip must be playable without becoming a program")
	}
}

// --- BroadcastsBetween: the guide's view of the same timeline ---

// THE PROPERTY THE WHOLE DESIGN RESTS ON: the guide and the encoder must agree. If they used
// separate arithmetic they would eventually diverge, and the symptom — "the guide says Heat but
// Predator is playing" — is the kind of bug nobody can reproduce on demand.
func TestBroadcastsBetween_AgreesWithAiringAt(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), breakGap(), prog("Predator", "b", 30)}

	// Sample the window at many instants; whatever AiringAt says is on must be the programme
	// the guide shows covering that moment.
	for m := 0; m < 200; m++ {
		at := epoch.Add(time.Duration(m) * time.Minute)
		want := AiringAt(slots, epoch, at)

		got := BroadcastsBetween(slots, epoch, at, at.Add(time.Minute))
		if len(got) == 0 {
			t.Fatalf("+%dm: guide is empty while AiringAt says %q", m, want.Title)
		}
		// The first entry is the one covering `at`.
		if got[0].Title != want.Title || got[0].Kind != want.Kind {
			t.Errorf("+%dm: guide says %q (%s), encoder plays %q (%s)",
				m, got[0].Title, got[0].Kind, want.Title, want.Kind)
		}
	}
}

// A programme already in progress must report its REAL start, not the window's start. A media
// server draws the current programme from its actual beginning; a clipped start renders as a
// show that appears to begin the moment you opened the guide.
func TestBroadcastsBetween_InProgressProgrammeKeepsItsRealStart(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), prog("Predator", "b", 30)}

	// Ask from 40 minutes in — Heat started at the epoch and runs to +60m.
	got := BroadcastsBetween(slots, epoch, epoch.Add(40*time.Minute), epoch.Add(90*time.Minute))
	if len(got) == 0 {
		t.Fatal("no programmes")
	}
	if got[0].Title != "Heat" {
		t.Fatalf("first programme = %q, want the one in progress", got[0].Title)
	}
	if !got[0].Start.Equal(epoch) {
		t.Errorf("start = %v, want the real start (%v) — a clipped start makes the show look "+
			"like it began when the guide was opened", got[0].Start, epoch)
	}
	if got[0].Duration() != 60*time.Minute {
		t.Errorf("duration = %v, want the programme's real 60m", got[0].Duration())
	}
}

// The window is covered CONTIGUOUSLY — no gaps between programmes, or the guide shows holes the
// channel does not actually have.
func TestBroadcastsBetween_CoversTheWindowWithoutGaps(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), breakGap(), prog("Predator", "b", 30)}
	got := BroadcastsBetween(slots, epoch, epoch, epoch.Add(6*time.Hour))

	if len(got) < 2 {
		t.Fatalf("only %d programmes over 6 hours", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i].Start.Equal(got[i-1].Stop) {
			t.Errorf("gap between %q (ends %v) and %q (starts %v)",
				got[i-1].Title, got[i-1].Stop, got[i].Title, got[i].Start)
		}
	}
	// And it must reach the end of the window.
	if last := got[len(got)-1]; last.Stop.Before(epoch.Add(6 * time.Hour)) {
		t.Errorf("coverage stops at %v, short of the 6h window", last.Stop)
	}
}

// The cycle repeats, so a long window replays the lineup rather than running out.
func TestBroadcastsBetween_RepeatsTheCycle(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), prog("Predator", "b", 30)}
	got := BroadcastsBetween(slots, epoch, epoch, epoch.Add(5*time.Hour))

	// 90m cycle over 5h ⇒ at least 6 programmes.
	if len(got) < 6 {
		t.Errorf("%d programmes over 5h of a 90m cycle — the cycle is not repeating", len(got))
	}
	titles := map[string]int{}
	for _, b := range got {
		titles[b.Title]++
	}
	if titles["Heat"] < 3 || titles["Predator"] < 3 {
		t.Errorf("cycle did not repeat evenly: %v", titles)
	}
}

// Filler is RETURNED, not filtered. The XMLTV guide must not advertise breaks (#12), but
// Loomarr's own time-grid shows them — filtering here would make this useless to one caller.
// The decision belongs to the renderer.
func TestBroadcastsBetween_IncludesFillerForTheCallerToDecide(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), breakGap(), prog("Predator", "b", 30)}
	got := BroadcastsBetween(slots, epoch, epoch, epoch.Add(2*time.Hour))

	var sawFiller bool
	for _, b := range got {
		if b.Kind == schedule.SlotFiller {
			sawFiller = true
		}
	}
	if !sawFiller {
		t.Error("filler was filtered out here; that choice belongs to the renderer (#12)")
	}
}

// An empty or unairable lineup yields no guide, rather than a panic or a fabricated programme.
func TestBroadcastsBetween_NothingAirableIsEmpty(t *testing.T) {
	for name, slots := range map[string][]schedule.Slot{
		"empty":       {},
		"all pending": {{Kind: schedule.SlotPending}, {Kind: schedule.SlotPending}},
	} {
		if got := BroadcastsBetween(slots, epoch, epoch, epoch.Add(4*time.Hour)); len(got) != 0 {
			t.Errorf("%s: got %d programmes, want none", name, len(got))
		}
	}
}

// A backwards or zero window is not an error, just empty — a media server may ask for anything.
func TestBroadcastsBetween_InvalidWindowIsEmpty(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60)}
	if got := BroadcastsBetween(slots, epoch, epoch.Add(time.Hour), epoch); len(got) != 0 {
		t.Errorf("backwards window returned %d programmes", len(got))
	}
	if got := BroadcastsBetween(slots, epoch, epoch, epoch); len(got) != 0 {
		t.Errorf("zero window returned %d programmes", len(got))
	}
}

// A lineup of very short items over a long window must terminate rather than hang the request.
func TestBroadcastsBetween_IsBounded(t *testing.T) {
	slots := []schedule.Slot{{Kind: schedule.SlotFiller}} // 30s fallback each
	done := make(chan int, 1)
	go func() {
		done <- len(BroadcastsBetween(slots, epoch, epoch, epoch.Add(365*24*time.Hour)))
	}()
	select {
	case n := <-done:
		if n == 0 {
			t.Error("no programmes at all")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("BroadcastsBetween did not terminate on a year-long window of 30s items")
	}
}

// --- BroadcastsWithPending: the time-grid's view, which shows what the encoder skips ---

func pendingSlot(title string) schedule.Slot {
	return schedule.Slot{Kind: schedule.SlotPending, Title: title}
}

// THE INVARIANT THE WHOLE PROJECTION RESTS ON: showing pending slots must not move anything that
// actually airs. If a nominal width ever reached the cycle arithmetic, every programme after it
// would shift and the ENCODER would be the one that is wrong — the guide would look right while
// playout aired silence for a made-up length.
func TestBroadcastsWithPending_DoesNotMoveAirableProgrammes(t *testing.T) {
	without := []schedule.Slot{prog("Heat", "a", 60), prog("Predator", "b", 30)}
	with := []schedule.Slot{prog("Heat", "a", 60), pendingSlot("Dune 2"), prog("Predator", "b", 30)}

	base := BroadcastsBetween(without, epoch, epoch, epoch.Add(6*time.Hour))
	got := BroadcastsWithPending(with, epoch, epoch, epoch.Add(6*time.Hour))

	var airable []Broadcast
	for _, b := range got {
		if !b.Nominal {
			airable = append(airable, b)
		}
	}
	if len(airable) != len(base) {
		t.Fatalf("%d airable programmes with a pending slot, %d without", len(airable), len(base))
	}
	for i := range base {
		if !airable[i].Start.Equal(base[i].Start) || airable[i].Title != base[i].Title {
			t.Errorf("programme %d moved: %q@%v, want %q@%v — a pending slot changed real airtime",
				i, airable[i].Title, airable[i].Start, base[i].Title, base[i].Start)
		}
	}
}

// THE GATE'S REQUIREMENT: a pending slot and a filler pod must be DISTINGUISHABLE. The old
// `gap bool` collapsed both to one value, so a UI could not draw them differently.
func TestBroadcastsWithPending_PendingAndFillerAreDistinguishable(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), breakGap(), pendingSlot("Dune 2"), prog("Predator", "b", 30)}
	got := BroadcastsWithPending(slots, epoch, epoch, epoch.Add(2*time.Hour))

	var sawFiller, sawPending bool
	for _, b := range got {
		switch b.Kind {
		case schedule.SlotFiller:
			sawFiller = true
		case schedule.SlotPending:
			sawPending = true
			if !b.Nominal {
				t.Error("a pending block must be marked Nominal — its times are invented")
			}
			if b.Title != "Dune 2" {
				t.Errorf("pending title = %q, want the awaited title", b.Title)
			}
		}
	}
	if !sawFiller || !sawPending {
		t.Errorf("filler=%v pending=%v — the grid cannot draw what it cannot tell apart",
			sawFiller, sawPending)
	}
}

// The pending marker sits where the content WILL land (§9 backfills in place), so it must be
// anchored to the programme it precedes rather than floating anywhere in the window.
func TestBroadcastsWithPending_AnchoredToTheFollowingProgramme(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), pendingSlot("Dune 2"), prog("Predator", "b", 30)}
	got := BroadcastsWithPending(slots, epoch, epoch, epoch.Add(90*time.Minute))

	var pend, next Broadcast
	for i, b := range got {
		if b.Kind == schedule.SlotPending {
			pend = b
			if i+1 < len(got) {
				next = got[i+1]
			}
			break
		}
	}
	if pend.Title == "" {
		t.Fatal("no pending block emitted")
	}
	if next.Title != "Predator" {
		t.Fatalf("pending precedes %q, want the programme it backfills before", next.Title)
	}
	if !pend.Stop.Equal(next.Start) {
		t.Errorf("pending stops at %v but the next programme starts at %v — the marker must "+
			"abut what follows it", pend.Stop, next.Start)
	}
	if pend.Duration() != NominalPendingDuration {
		t.Errorf("pending width = %v, want the nominal %v", pend.Duration(), NominalPendingDuration)
	}
}

// A lineup with NOTHING airable has no timeline to anchor to, so there is nowhere to place a
// pending marker. Empty is the honest answer, not a floating block at an invented time.
func TestBroadcastsWithPending_NothingAirableIsEmpty(t *testing.T) {
	slots := []schedule.Slot{pendingSlot("Dune 2"), pendingSlot("Tron 3")}
	if got := BroadcastsWithPending(slots, epoch, epoch, epoch.Add(6*time.Hour)); len(got) != 0 {
		t.Errorf("got %d blocks from a lineup where nothing airs", len(got))
	}
}

// A pending slot at the END of the lineup precedes the FIRST programme of the next cycle pass —
// the cycle repeats, so "the slot after the last one" is slot zero, not nothing.
func TestBroadcastsWithPending_TrailingPendingWrapsToTheNextPass(t *testing.T) {
	slots := []schedule.Slot{prog("Heat", "a", 60), prog("Predator", "b", 30), pendingSlot("Dune 2")}
	got := BroadcastsWithPending(slots, epoch, epoch, epoch.Add(4*time.Hour))

	for i, b := range got {
		if b.Kind != schedule.SlotPending {
			continue
		}
		if i+1 >= len(got) {
			t.Fatal("pending block emitted with nothing following it")
		}
		if got[i+1].Title != "Heat" {
			t.Errorf("trailing pending precedes %q, want Heat (the next pass's first programme)",
				got[i+1].Title)
		}
		return
	}
	t.Error("no pending block emitted for a lineup ending in a pending slot")
}

// Nominal blocks must never reach a media server's EPG: their times are invented, so a listing
// would promise a programme at a moment it will never air.
func TestAdvertisable_RejectsNominalBlocks(t *testing.T) {
	b := Broadcast{Kind: schedule.SlotProgram, Title: "Dune 2", Nominal: true}
	if advertisable(b) {
		t.Error("a nominal block was advertisable — XMLTV would promise an airing that never happens")
	}
}
