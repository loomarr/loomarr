package proposaloutlook

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

func TestRunwayCountsConcreteProgramsAndFirstRepeatInActualOrder(t *testing.T) {
	minute := int64(time.Minute / time.Millisecond)
	cycle := channels.CycleResult{Slots: []schedule.Slot{
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:1", LibraryItemID: "one", DurationMs: 90 * minute},
		{Kind: schedule.SlotFiller, DurationMs: 20 * minute},
		{Kind: schedule.SlotPending, Key: "movie:tmdb:9", DurationMs: 120 * minute},
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:2", LibraryItemID: "two", DurationMs: 60 * minute},
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:1", LibraryItemID: "one", DurationMs: 90 * minute},
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:3", LibraryItemID: "three", DurationMs: 45 * minute},
	}}
	got := summarize(cycle)
	if got.Programs != 3 || got.UniqueRuntimeMs != 195*minute || got.FirstRepeatMs == nil || *got.FirstRepeatMs != 150*minute {
		t.Fatalf("runway must exclude placeholders/breaks and stop fresh order at the repeat: %+v", got)
	}
	cycle.Slots[2], cycle.Slots[3] = cycle.Slots[3], cycle.Slots[2]
	cycle.Slots[1], cycle.Slots[4] = cycle.Slots[4], cycle.Slots[1]
	got = summarize(cycle)
	if got.FirstRepeatMs == nil || *got.FirstRepeatMs != 90*minute || !got.Thin {
		t.Fatalf("earlier repeated program must shorten runway: %+v", got)
	}
}

func TestWindowAndUnknownProgramsDoNotInventARepeatGuarantee(t *testing.T) {
	hour := int64(time.Hour / time.Millisecond)
	got := summarize(channels.CycleResult{Window: 2 * time.Hour, Slots: []schedule.Slot{
		{Kind: schedule.SlotProgram, LibraryItemID: "one", DurationMs: hour},
		{Kind: schedule.SlotProgram, LibraryItemID: "two", DurationMs: hour},
		{Kind: schedule.SlotProgram, LibraryItemID: "unknown-duration"},
		{Kind: schedule.SlotProgram, DurationMs: hour},
	}})
	if got.Programs != 2 || got.UniqueRuntimeMs != 2*hour || !got.WindowLimited || got.FirstRepeatMs != nil {
		t.Fatalf("window-limited observed runtime is a lower bound: %+v", got)
	}
}

func TestEditorialMixDoesNotReclassifyMissingRunEvidence(t *testing.T) {
	p := suggest.Proposal{Lineup: []suggest.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 1, EditorialRole: suggest.EditorialCore},
		{MediaType: provision.Movie, TMDBID: 2, EditorialRole: suggest.EditorialAdjacent},
		{MediaType: provision.Movie, TMDBID: 3, EditorialRole: suggest.EditorialDiscovery},
		{MediaType: provision.Movie, TMDBID: 4, InLibrary: true, Source: "library"},
	}}
	got := editorialMix(p, []schedule.LineupEntry{{Key: "movie:tmdb:1"}, {Key: "movie:tmdb:2"}, {Key: "movie:tmdb:3"}, {Key: "movie:tmdb:4"}, {Key: "movie:tmdb:5"}})
	if got != (Mix{Core: 1, Adjacent: 1, Discovery: 1, Unknown: 2}) {
		t.Fatalf("ownership and missing history cannot invent editorial evidence: %+v", got)
	}
}
