package kit

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

type seriesLibrary map[provision.Key][]schedule.ResolvedProgram

func (seriesLibrary) Resolve(provision.Key) (string, int64, bool) { return "", 0, false }

func (l seriesLibrary) ResolveEpisodes(k provision.Key) schedule.EpisodeResolution {
	return schedule.EpisodeResolution{Programs: l[k]}
}

// traceFor arranges `shows` × `episodes` 22-minute episodes with two breaks an hour, through the
// real scheduler, and returns the trace as the cycle endpoint serialises it plus the break count.
func traceFor(t *testing.T, shows, episodes int) (Trace, int) {
	t.Helper()
	lib := seriesLibrary{}
	var entries []schedule.LineupEntry
	for s := range shows {
		k := provision.Key(fmt.Sprintf("series:tvdb:%d", 100+s))
		for e := 1; e <= episodes; e++ {
			lib[k] = append(lib[k], schedule.ResolvedProgram{
				LibraryItemID: fmt.Sprintf("s%d-e%d", s, e), Title: fmt.Sprintf("ep %d", e),
				DurationMs: 22 * 60000, Season: 1, Episode: e,
			})
		}
		entries = append(entries, schedule.LineupEntry{Key: k, Title: fmt.Sprintf("show %d", s)})
	}
	ch := schedule.Channel{ID: "c", Name: "c", Number: 1, Strategy: schedule.Shuffle,
		DefaultWindow: 24 * time.Hour, BreaksPerHour: 2, BreakDurationMs: 120000}
	policy := schedule.ChannelPolicy{}
	policy.Ordering = schedule.OrderSyndication
	d := schedule.ComputeDesiredAt(ch, entries, lib, schedule.PodFill, policy, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))

	breaks := 0
	for _, s := range d.Slots {
		if s.Kind == schedule.SlotFiller && s.Key == "" {
			breaks++
		}
	}
	b, err := json.Marshal(d.Trace)
	if err != nil {
		t.Fatal(err)
	}
	var tr Trace
	if err := json.Unmarshal(b, &tr); err != nil {
		t.Fatal(err)
	}
	return tr, breaks
}

func TestPoolFromTrace(t *testing.T) {
	cases := []struct {
		name            string
		shows, episodes int
		wantTruncated   bool
		wantOK          bool
	}{
		// Whole trace recorded; breaks are placement facts too and must not count as pool.
		{"small deck with breaks", 2, 20, false, true},
		// More than 256 placement facts: the household case that used to read 0.
		{"deck past the placement bound", 5, 100, true, true},
		// So many episode-selection facts that the non-placement stages were truncated too.
		{"trace cannot answer", 5, 200, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr, breaks := traceFor(t, c.shows, c.episodes)
			if tr.Truncated != c.wantTruncated {
				t.Fatalf("truncated = %v, want %v (factTotal %d)", tr.Truncated, c.wantTruncated, tr.FactTotal)
			}
			if c.wantOK && breaks == 0 {
				t.Fatal("fixture has no breaks; it cannot pin their exclusion")
			}
			pool, ok := PoolFromTrace(tr, breaks)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if want := c.shows * c.episodes; ok && pool != want {
				t.Fatalf("pool = %d, want %d", pool, want)
			}
		})
	}
}
