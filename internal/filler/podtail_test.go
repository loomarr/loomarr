package filler_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// The tail top-up exists to fill a few spare seconds, never to re-air a clip the household just
// saw: a short tail beats a repeat inside the cooldown.
func TestFillTail_NeverUsesAClipInsideItsCooldown(t *testing.T) {
	now := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)
	cat := []filler.Clip{
		{Hash: "ad", Path: "ad", Name: "ad", Kind: filler.Commercial, Era: 1992, Audience: filler.General, Category: "cars", DurationMs: 45_000},
		{Hash: "id1", Path: "id1", Name: "id1", Kind: filler.StationID, Era: 1992, Audience: filler.General, DurationMs: 10_000},
		{Hash: "id2", Path: "id2", Name: "id2", Kind: filler.StationID, Era: 1992, Audience: filler.General, DurationMs: 10_000},
		{Hash: "id3", Path: "id3", Name: "id3", Kind: filler.StationID, Era: 1992, Audience: filler.General, DurationMs: 10_000},
		{Hash: "id4", Path: "id4", Name: "id4", Kind: filler.StationID, Era: 1992, Audience: filler.General, DurationMs: 10_000},
	}
	policy := filler.Policy{Cooldown: time.Hour}
	build := func(lastPlayed time.Time) (pod filler.Pod, idents int) {
		w := filler.Window{
			ChannelID: "ch", Seed: 1, Era: filler.Year(1992), Audience: filler.General,
			GapMs: 80_000, PodMax: 4, SnapshotAt: now,
			Exposures: map[string]filler.Exposure{
				"id1": {PlayCount: 1, LastPlayedAt: lastPlayed},
				"id2": {PlayCount: 1, LastPlayedAt: lastPlayed},
				"id3": {PlayCount: 1, LastPlayedAt: lastPlayed},
				"id4": {PlayCount: 1, LastPlayedAt: lastPlayed},
			},
		}
		pod = filler.Assemble(cat, w, policy, map[string]bool{})
		for _, e := range pod.Entries {
			if e.Kind == filler.StationID {
				idents++
			}
		}
		return pod, idents
	}

	// Both idents are resting: the break reserves one at each end, but the tail must not add a third.
	resting, n := build(now.Add(-10 * time.Minute))
	if n > 2 {
		t.Errorf("the tail re-aired a resting ident: %s", podShape(resting))
	}

	// Both rested: the tail uses a third ident.
	rested, n := build(now.Add(-3 * time.Hour))
	if n != 3 {
		t.Errorf("a rested ident should fill the tail: %s", podShape(rested))
	}
	if rested.CooldownRelaxed {
		t.Errorf("rested clips must not mark the cooldown relaxed: %s", podShape(rested))
	}
}
