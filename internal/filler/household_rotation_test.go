package filler_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// householdHistory is the store's rotation history in memory: every airing the household watched,
// recorded the way playout records one, as it resolves the clip ahead of air.
type householdHistory struct{ airings []filler.Airing }

func (h *householdHistory) FillerAirings(context.Context) (filler.AiringHistory, error) {
	return filler.AiringHistory{Airings: h.airings, Settle: 15 * time.Second}, nil
}

// #2017, the household's evening of 2026-10-24: one TV, two 1990s channels sharing one pool of
// commercials, the viewer switching between them every half hour for four and a half hours. With
// per-channel history each channel ranked its own airings only, so the two aired the same clips in
// the same order and the evening saw 26 different commercials out of the pool. The household's
// history is one: no commercial it saw airs again inside the four hour spacing while the pool, its
// widened era included, still has a rested one, and a channel nobody was watching counts nothing.
func TestHouseholdRotation_AnEveningOfChannelSurfingNeverRepeatsInsideTheSpacing(t *testing.T) {
	var cat []filler.Clip
	add := func(prefix string, n, era int) {
		for i := range n {
			id := fmt.Sprintf("%s%02d", prefix, i)
			cat = append(cat, filler.Clip{Hash: id, Path: id + ".mp4", Name: id, Kind: filler.Commercial, Era: era,
				Audience: filler.General, Category: fmt.Sprintf("cat%d", i%6), DurationMs: int64(10_000 + 5_000*(i%5))})
		}
	}
	add("nineties", 44, 1994) // the pool the household's two 1990s channels draw on
	add("eighties", 20, 1984) // the widened era behind it
	for i := range 4 {
		id := fmt.Sprintf("ident%d", i)
		cat = append(cat, filler.Clip{Hash: id, Path: id + ".mp4", Name: id, Kind: filler.StationID, Era: 1994,
			Audience: filler.General, DurationMs: 5_000})
	}
	history := &householdHistory{}
	spacing := 4 * time.Hour
	adapter := filler.NewPodAdapter(stubCatalog{clips: cat}, history, func() filler.Policy {
		return filler.Policy{PodMax: 4, BreakDurationMs: 120_000, Cooldown: spacing}
	}, discardLogger())
	sel := filler.Selection{Era: filler.EraRange{From: 1989, To: 1999}, Audience: filler.General}

	evening := time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC) // 21:00 EDT
	const breakEvery = 25 * time.Minute                      // 2.4 breaks an hour, as both channels air
	channels := []struct {
		id     string
		offset time.Duration
	}{{"cartoon-classics", 0}, {"rewind", 12 * time.Minute}}
	watching := func(at time.Time) string { return channels[int(at.Sub(evening)/(30*time.Minute))%2].id }

	type airing struct {
		channel string
		at      time.Time
	}
	lastSeen := map[string]airing{}
	distinct, watched := map[string]bool{}, 0
	for at := evening; at.Before(evening.Add(4*time.Hour + 30*time.Minute)); at = at.Add(time.Minute) {
		for _, ch := range channels {
			if (at.Sub(evening)-ch.offset)%breakEvery != 0 || at.Sub(evening) < ch.offset {
				continue
			}
			pod, err := adapter.PreviewAt(context.Background(), ch.id, seedFor(ch.id, at), sel, at)
			if err != nil {
				t.Fatal(err)
			}
			if watching(at) != ch.id {
				continue // the neighbour warm airs it to nobody: nothing is recorded
			}
			watched++
			clipAt := at
			for _, e := range pod.Entries {
				if e.Kind == filler.Commercial {
					if prev, ok := lastSeen[e.Hash]; ok && clipAt.Sub(prev.at) < spacing {
						t.Errorf("%s aired on %s at %s, %v after the household saw it on %s",
							e.Hash, ch.id, clipAt.Format("15:04"), clipAt.Sub(prev.at), prev.channel)
					}
					lastSeen[e.Hash] = airing{ch.id, clipAt}
					distinct[e.Hash] = true
				}
				history.airings = append(history.airings,
					filler.Airing{ClipHash: e.Hash, AiredAt: clipAt, RecordedAt: clipAt.Add(-12 * time.Second)})
				clipAt = clipAt.Add(time.Duration(e.DurationMs) * time.Millisecond)
			}
		}
	}
	if watched < 10 {
		t.Fatalf("the evening watched %d breaks; the scenario no longer models it", watched)
	}
	if len(distinct) < 40 {
		t.Errorf("the evening aired %d different commercials over %d watched breaks, want the pool used", len(distinct), watched)
	}
}

// seedFor stands in for channels.PodSeedAt: a stable seed per channel and break start.
func seedFor(channel string, at time.Time) int64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s@%d", channel, at.UnixMilli())
	return int64(h.Sum64() >> 1)
}
