package schedule_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit/libraryfixture"
)

// syndicatedComedyChannel is the shape of the production channel whose cycle preview took ~30s:
// four 1980s comedy series on syndication ordering, auto seasonal, the spacing rules and an era,
// computed inside Halloween season. All titles are invented.
func syndicatedComedyChannel(episodes, halloween int) (schedule.Channel, []schedule.LineupEntry, schedule.Availability, schedule.ChannelPolicy) {
	const policyJSON = `{"scope":{"dates":{"movieRelease":[{"from":1980,"to":1989}],"seriesPremiere":[{"from":1980,"to":1989}],"seriesAiring":[{"from":1980,"to":1989}]},"genres":{"include":["Comedy"]}},"audience":{},"separation":{},"ordering":"syndication","seasonal":{"mode":"auto"},"filler":{"era":{"from":1980,"to":1989}},"applied":[{"kind":"episodeNoRepeat","from":"168h0m0s","to":"84h0m0s"},{"kind":"episodeNoRepeat","from":"84h0m0s","to":"42h0m0s"},{"kind":"episodeNoRepeat","from":"42h0m0s","to":"24h0m0s"},{"kind":"seriesMinGap","from":"2h0m0s","to":"0s"},{"kind":"blockMax","from":"2","to":"unbounded"}]}`
	var policy schedule.ChannelPolicy
	if err := json.Unmarshal([]byte(policyJSON), &policy); err != nil {
		panic(err)
	}
	series := map[provision.Key]schedule.EpisodeResolution{}
	var entries []schedule.LineupEntry
	titles := []string{"The Mudpuddles", "Kettle Street", "Two Doors Down Under", "Lamplight Lane"}
	for si, title := range titles {
		key := provision.Key(fmt.Sprintf("series:tvdb:%d", 90070000+si))
		entries = append(entries, schedule.LineupEntry{
			Key: key, Title: title, OfficialRating: "TV-PG", Genres: []string{"Comedy"}, Year: 1982 + si,
		})
		progs := make([]schedule.ResolvedProgram, 0, episodes)
		for i := 1; i <= episodes; i++ {
			name := fmt.Sprintf("Episode %d", i)
			if i%(episodes/halloween) == 0 {
				name = fmt.Sprintf("A Halloween Visit %d", i)
			}
			progs = append(progs, schedule.ResolvedProgram{
				LibraryItemID: fmt.Sprintf("ep-%d-%d", si, i), Title: name, DurationMs: 1_320_000,
				Season: 1 + i/24, Episode: i, Year: 1983 + i%6, OfficialRating: "TV-PG",
				AirDate: time.Date(1983+i%6, time.Month(1+i%12), 1+i%27, 20, 0, 0, 0, time.UTC),
			})
		}
		series[key] = schedule.EpisodeResolution{Programs: progs}
	}
	ch := schedule.Channel{ID: "ch-synd", Name: "Syndicated", Number: 80, Strategy: schedule.Shuffle,
		BreaksPerHour: 4, DefaultWindow: 24 * time.Hour}
	return ch, entries, libraryfixture.Availability{Series: series}, policy
}

var oct27 = time.Date(2026, time.October, 27, 20, 0, 0, 0, time.UTC)

func BenchmarkComputeDesiredAt_SyndicatedHalloween(b *testing.B) {
	for _, n := range []int{40, 100, 200} {
		ch, entries, avail, policy := syndicatedComedyChannel(n, 6)
		b.Run(fmt.Sprintf("episodes=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				d := schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, policy, oct27)
				if len(d.Slots) == 0 {
					b.Fatal("no slots")
				}
			}
		})
	}
}

// A cold cycle of this channel took 5-8s on a fast machine, ~30s in production, and the guide,
// the packager and the maintenance sweep all wait on it. It is ~0.17s now; the bound leaves a slow
// CI runner an order of magnitude of room and still fails on the old cost. Serial: it measures
// wall-clock, so a parallel test would be measuring its neighbours.
func TestComputeDesiredAt_SyndicatedHalloweenCycleIsPrompt(t *testing.T) {
	schedule.SkipSerialUnderRace(t)
	ch, entries, avail, policy := syndicatedComedyChannel(40, 6)
	ch.ID = "ch-synd-prompt" // its own seed, so the deal memo starts cold
	start := time.Now()
	d := schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, policy, oct27)
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("cold cycle took %v, want well under 2s", took)
	}
	if len(d.Slots) == 0 {
		t.Fatal("no slots")
	}
}

func TestComputeDesiredAtCtx_StopsWhenCancelled(t *testing.T) {
	schedule.SkipSerialUnderRace(t)
	ch, entries, avail, policy := syndicatedComedyChannel(40, 6)

	t.Run("already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := schedule.ComputeDesiredAtCtx(ctx, ch, entries, avail, schedule.PodFill, policy, oct27); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})

	t.Run("expires mid-computation", func(t *testing.T) {
		ch := ch
		ch.ID = "ch-synd-cancel" // cold memo: the computation has real work to be interrupted in
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := schedule.ComputeDesiredAtCtx(ctx, ch, entries, avail, schedule.PodFill, policy, oct27)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want context.DeadlineExceeded (the cold computation finished inside 1ms?)", err)
		}
		if took := time.Since(start); took > time.Second {
			t.Fatalf("returned %v after a 1ms deadline: the computation ignores its context", took)
		}
	})

	t.Run("live context matches the plain call", func(t *testing.T) {
		ch := ch
		ch.ID = "ch-synd-live"
		want := schedule.ComputeDesiredAt(ch, entries, avail, schedule.PodFill, policy, oct27)
		got, err := schedule.ComputeDesiredAtCtx(context.Background(), ch, entries, avail, schedule.PodFill, policy, oct27)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.EqualFunc(want.Slots, got.Slots, func(a, b schedule.Slot) bool { return a == b }) {
			t.Fatal("ComputeDesiredAtCtx and ComputeDesiredAt disagree")
		}
	})
}
