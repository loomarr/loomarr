package schedule

import (
	"fmt"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// Scale benchmarks for ComputeDesiredAt — the function that dominates GET /v1/guide.
//
// WHY THESE EXIST. The guide re-runs ComputeDesiredAt per channel per request. On the
// maintainer's 4-channel install that measured ~120ms per channel (~375ms for the request even
// with the channels resolved concurrently), against a target of ≤50ms for the whole route. The
// question these answer is whether that cost is REDUCIBLE or intrinsic — and how it scales to
// the 200+ channels the guide is expected to serve.
//
// The fixtures mirror the maintainer's real data shape, read off the dev store rather than
// invented: lineups of 1–25 entries producing 14–87 slots, a 24h window, break interleave on.

// benchAvail resolves every movie key instantly, with no I/O. That is deliberate: the library
// round-trips are already memoized (internal/channels), so what is left to measure is the pure
// CPU of laying out a cycle. A fake that slept would re-measure the problem already solved.
type benchAvail struct{ episodesPerSeries int }

func (b benchAvail) Resolve(key provision.Key) (string, int64, bool) {
	if key.IsSeries() {
		return "", 0, false
	}
	return "item-" + string(key), 5_400_000, true // ~90m film
}

func (b benchAvail) ResolveEpisodes(key provision.Key) ([]ResolvedProgram, bool) {
	if !key.IsSeries() || b.episodesPerSeries == 0 {
		return nil, false
	}
	eps := make([]ResolvedProgram, 0, b.episodesPerSeries)
	for i := 1; i <= b.episodesPerSeries; i++ {
		eps = append(eps, ResolvedProgram{
			LibraryItemID: fmt.Sprintf("ep-%s-%d", key, i),
			DurationMs:    1_320_000, // ~22m episode
			Season:        1 + i/24,
			Episode:       i,
			Title:         fmt.Sprintf("Episode %d", i),
		})
	}
	return eps, true
}

// movieLineup builds an n-film lineup with the metadata enforcement actually reads (rating,
// genres, year, runtime) — an entry set missing those would skip filters and flatter the result.
func movieLineup(n int) []LineupEntry {
	out := make([]LineupEntry, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, LineupEntry{
			Key:            provision.Key(fmt.Sprintf("movie:tmdb:%d", 1000+i)),
			Title:          fmt.Sprintf("Film %d", i),
			DurationMs:     5_400_000,
			OfficialRating: Rating("PG-13"),
			Genres:         []string{"Action", "Adventure"},
			Year:           1980 + (i % 20),
			RuntimeSec:     5400,
			CollectionID:   -1, // resolved, standalone — no franchise grouping work
		})
	}
	return out
}

func benchChannel(id string) Channel {
	return Channel{
		ID: id, Name: "Bench " + id, Number: 42,
		Strategy:      Shuffle,
		BreaksPerHour: 4,              // break interleave ON, as the maintainer's channels run
		DefaultWindow: 24 * time.Hour, // the §6.5 rolling-window horizon
	}
}

// The per-channel cost, across the lineup sizes the maintainer's install actually has.
// Divide the ns/op by 1e6 for ms: this is the number that must fall under ~50ms/N for the
// route budget to be reachable by making the computation itself faster.
func BenchmarkComputeDesiredAt_ByLineupSize(b *testing.B) {
	at := time.Date(2026, 7, 26, 21, 0, 0, 0, time.UTC)
	for _, n := range []int{1, 6, 15, 25, 50, 100} {
		entries := movieLineup(n)
		ch := benchChannel("ch-bench")
		av := benchAvail{}
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				d := ComputeDesiredAt(ch, entries, av, PodFill, ChannelPolicy{}, at)
				if len(d.Slots) == 0 {
					b.Fatal("no slots — the fixture is not exercising the layout")
				}
			}
		})
	}
}

// A SERIES channel, which is the expensive shape: one entry expands to hundreds of episodes.
// The maintainer's Springfield Classics is exactly this (1 lineup entry → 64 slots).
func BenchmarkComputeDesiredAt_SeriesExpansion(b *testing.B) {
	at := time.Date(2026, 7, 26, 21, 0, 0, 0, time.UTC)
	for _, eps := range []int{50, 200, 800} {
		entries := []LineupEntry{{
			Key: provision.Key("series:tvdb:71663"), Title: "Bench Series",
			OfficialRating: Rating("TV-PG"), Genres: []string{"Animation"}, Year: 1991,
		}}
		ch := benchChannel("ch-series")
		av := benchAvail{episodesPerSeries: eps}
		b.Run(fmt.Sprintf("episodes=%d", eps), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				d := ComputeDesiredAt(ch, entries, av, PodFill, ChannelPolicy{}, at)
				if len(d.Slots) == 0 {
					b.Fatal("no slots")
				}
			}
		})
	}
}

// The whole-guide shape: what 200 channels costs if each is computed once. Sequential here on
// purpose — the handler runs them concurrently, so divide by the core count for the real
// wall-clock, but this is the total CPU the box must find either way.
func BenchmarkComputeDesiredAt_200Channels(b *testing.B) {
	at := time.Date(2026, 7, 26, 21, 0, 0, 0, time.UTC)
	// A mix matching the maintainer's install: mostly small movie channels, a few series ones.
	type chSpec struct {
		ch      Channel
		entries []LineupEntry
		av      Availability
	}
	specs := make([]chSpec, 0, 200)
	for i := 0; i < 200; i++ {
		if i%4 == 0 {
			specs = append(specs, chSpec{
				ch:      benchChannel(fmt.Sprintf("ch-%d", i)),
				entries: []LineupEntry{{Key: provision.Key("series:tvdb:71663"), Title: "Series", OfficialRating: Rating("TV-PG")}},
				av:      benchAvail{episodesPerSeries: 200},
			})
			continue
		}
		specs = append(specs, chSpec{
			ch:      benchChannel(fmt.Sprintf("ch-%d", i)),
			entries: movieLineup(15),
			av:      benchAvail{},
		})
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, s := range specs {
			ComputeDesiredAt(s.ch, s.entries, s.av, PodFill, ChannelPolicy{}, at)
		}
	}
}
