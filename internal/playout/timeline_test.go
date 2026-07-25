package playout

import (
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
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
}
