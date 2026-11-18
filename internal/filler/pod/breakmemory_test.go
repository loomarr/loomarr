package pod_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

func memoryCatalog() []pod.Clip {
	cat := catalogOf(30, 30, 30, 30, 30, 30)
	cat = append(cat, pod.Clip{Hash: "ident", Path: "ident.mp4", Role: pod.StationID, Era: 1992,
		Audience: pod.General, DurationMs: 5_000})
	return cat
}

func memoryBreak(t *testing.T, catalog []pod.Clip, history []pod.Airing, start time.Time, seed int64) pod.Pod {
	t.Helper()
	adapter := pod.NewAdapter(stubCatalog{clips: catalog}, &stubExposures{items: history}, func() pod.Policy {
		return pod.Policy{BreakDurationMs: 120_000, PodMax: 4}
	}, discardLogger())
	brk, err := adapter.PreviewAt(context.Background(), "ch-1", seed, pod.Selection{}, start)
	if err != nil {
		t.Fatal(err)
	}
	return brk
}

func airedOn(channel string, hashes []string, at time.Time) []pod.Airing {
	var out []pod.Airing
	for i, h := range hashes {
		when := at.Add(time.Duration(i) * time.Minute)
		out = append(out, pod.Airing{ChannelID: channel, ClipHash: h, AiredAt: when, RecordedAt: when.Add(-time.Minute)})
	}
	return out
}

// A break airs only clips that have not aired this evening, counted household-wide (#3838
// decision 1): an ad another channel aired half an hour ago is not in this break, whatever its
// channel's own rotation says.
func TestABreakAirsNoAdThatAiredThisEvening(t *testing.T) {
	start := time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)
	seen := []string{"ad00", "ad01", "ad02"}
	history := airedOn("other-channel", seen, start.Add(-40*time.Minute))
	for seed := int64(0); seed < 40; seed++ {
		for _, e := range memoryBreak(t, memoryCatalog(), history, start, seed).Entries {
			for _, id := range seen {
				if e.Hash == id {
					t.Fatalf("seed %d: %s aired again within the evening", seed, id)
				}
			}
		}
	}
}

// Versions of one ad are one ad: a cut-down copy of an ad seen this evening is not in the break.
func TestABreakAirsNoVersionOfAnAdSeenThisEvening(t *testing.T) {
	start := time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)
	cat := memoryCatalog()
	for i := range cat {
		if cat[i].Hash == "ad00" {
			cat[i].Variants = []string{"ad01"}
		}
	}
	history := airedOn("other-channel", []string{"ad01"}, start.Add(-40*time.Minute))
	for seed := int64(0); seed < 40; seed++ {
		for _, e := range memoryBreak(t, cat, history, start, seed).Entries {
			if e.Hash == "ad00" || e.Hash == "ad01" {
				t.Fatalf("seed %d: %s aired though its other version aired this evening", seed, e.Hash)
			}
		}
	}
}

// An evening is learned from viewing, not a clock hour (#3838): a silence longer than the evening
// gap ends it, and the ads of the last one may air again.
func TestAnEveningEndsAfterALongSilence(t *testing.T) {
	start := time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)
	seen := []string{"ad00", "ad01", "ad02", "ad03", "ad04", "ad05"}
	history := airedOn("other-channel", seen, start.Add(-5*time.Hour))
	aired := map[string]bool{}
	for seed := int64(0); seed < 40; seed++ {
		for _, e := range memoryBreak(t, memoryCatalog(), history, start, seed).Entries {
			aired[e.Hash] = true
		}
	}
	// Rotation leads with the ads aired longest ago, so the first of them air; all of them
	// would have been barred had the old evening carried over.
	for _, id := range seen[:3] {
		if !aired[id] {
			t.Errorf("%s never aired in a new evening, though the household was away for hours", id)
		}
	}
}

// An evening also ends after six hours of viewing, whichever comes first: a channel left on all
// night is not one evening of days, and ads air again after the cap.
func TestAnEveningEndsAfterSixHoursOfViewing(t *testing.T) {
	start := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	first := start.Add(-8 * time.Hour)
	// ad00 airs at the start of an unbroken night: another clip every half hour until the break.
	history := airedOn("other-channel", []string{"ad00"}, first)
	for at := first.Add(30 * time.Minute); at.Before(start); at = at.Add(30 * time.Minute) {
		history = append(history, airedOn("other-channel", []string{"night-filler"}, at)...)
	}
	// One ad, so rotation has nothing else to prefer: it airs unless the evening bar holds it.
	oneAd := append(catalogOf(30), memoryCatalog()[len(memoryCatalog())-1])
	aired := false
	for seed := int64(0); seed < 40 && !aired; seed++ {
		for _, e := range memoryBreak(t, oneAd, history, start, seed).Entries {
			aired = aired || e.Hash == "ad00"
		}
	}
	if !aired {
		t.Error("ad00 aired eight hours ago, in the first evening of an all-night run, and is still barred")
	}

	// Inside the cap the same ad stays barred: five hours into the run it is the same evening.
	soon := first.Add(5 * time.Hour)
	for seed := int64(0); seed < 40; seed++ {
		var shortHistory []pod.Airing
		for _, a := range history {
			if a.AiredAt.Before(soon) {
				shortHistory = append(shortHistory, a)
			}
		}
		for _, e := range memoryBreak(t, memoryCatalog(), shortHistory, soon, seed).Entries {
			if e.Hash == "ad00" {
				t.Fatalf("seed %d: ad00 aired five hours into the evening that it aired in", seed)
			}
		}
	}
}

// A thin set shortens its breaks: with every ad already aired this evening, the break is its
// station ID alone, never a repeat and never a stretch of the same ads (#3838 grill decision 22).
func TestAThinSetShortensItsBreakToTheStationID(t *testing.T) {
	start := time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)
	var all []string
	for i := 0; i < 6; i++ {
		all = append(all, fmt.Sprintf("ad%02d", i))
	}
	history := airedOn("other-channel", all, start.Add(-30*time.Minute))
	for seed := int64(0); seed < 20; seed++ {
		brk := memoryBreak(t, memoryCatalog(), history, start, seed)
		if len(brk.Entries) != 1 || brk.Entries[0].Hash != "ident" {
			t.Fatalf("seed %d: break = %+v, want the station ID alone", seed, brk.Entries)
		}
	}
}
