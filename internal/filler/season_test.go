package filler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/schedule"
)

var (
	seasonJul14 = time.Date(2026, time.July, 14, 20, 0, 0, 0, time.UTC)
	seasonDec20 = time.Date(2026, time.December, 20, 20, 0, 0, 0, time.UTC)
)

func seasonAd(id string, era int, tags ...string) filler.Clip {
	return filler.Clip{Hash: id, Path: id + ".mp4", Name: id, Kind: filler.Commercial, Era: era,
		Audience: filler.General, DurationMs: 15_000, Tags: tags, AssertedTags: tags}
}

// seasonCatalog is a 1990s channel's pool: plain ads, Christmas ads, a Christmas station ID and a
// plain one.
func seasonCatalog(plain, christmas int) []filler.Clip {
	var cat []filler.Clip
	for i := range plain {
		cat = append(cat, seasonAd(fmt.Sprintf("plain%02d", i), 1994))
	}
	for i := range christmas {
		cat = append(cat, seasonAd(fmt.Sprintf("xmas%02d", i), 1994, "christmas"))
	}
	cat = append(cat,
		filler.Clip{Hash: "xmas-ident", Path: "xmas-ident.mp4", Name: "xmas-ident", Kind: filler.StationID,
			Era: 1994, Audience: filler.General, DurationMs: 5_000, Tags: []string{"christmas"}, AssertedTags: []string{"christmas"}},
		filler.Clip{Hash: "ident", Path: "ident.mp4", Name: "ident", Kind: filler.StationID,
			Era: 1994, Audience: filler.General, DurationMs: 5_000})
	return cat
}

func seasonAdapter(cat []filler.Clip, history filler.AiringReader, cal schedule.Calendar, clock time.Time) *filler.PodAdapter {
	return filler.NewPodAdapter(stubCatalog{clips: cat}, history, func() filler.Policy {
		return filler.Policy{PodMax: 4, BreakDurationMs: 120_000, Cooldown: 4 * time.Hour, Calendar: cal}
	}, discardLogger()).WithClock(func() time.Time { return clock })
}

var seasonSel = filler.Selection{Era: filler.EraRange{From: 1990, To: 1999}, Audience: filler.General}

func inSeasonCount(p filler.Pod, tagged map[string]bool) (in, body int) {
	for _, e := range p.Entries {
		if e.Kind != filler.Commercial {
			continue
		}
		body++
		if tagged[e.Hash] {
			in++
		}
	}
	return in, body
}

func christmasHashes(cat []filler.Clip) map[string]bool {
	out := map[string]bool{}
	for _, c := range cat {
		for _, tag := range c.Tags {
			if tag == "christmas" {
				out[c.Hash] = true
			}
		}
	}
	return out
}

// Decision 5: a Christmas ad, or a Christmas station ID, never airs in July.
func TestSeason_OutOfSeasonClipNeverAirs(t *testing.T) {
	cat := seasonCatalog(12, 6)
	xmas := christmasHashes(cat)
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonJul14)
	for seed := range int64(200) {
		pod, err := a.PreviewAt(context.Background(), "ch", seed, seasonSel, seasonJul14)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range pod.Entries {
			if xmas[e.Hash] {
				t.Fatalf("seed %d: %s aired in July", seed, e.Hash)
			}
		}
	}
}

// A preview with no break start (the reconciler's pool check, a Tunarr filler list) is judged at
// the adapter's clock, as programming's seasonality is.
func TestSeason_PreviewWithoutAnInstantUsesTheClock(t *testing.T) {
	cat := seasonCatalog(0, 3)
	july := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonJul14)
	if july.HasPool(context.Background(), "ch", 1, seasonSel) {
		t.Error("in July a Christmas-only catalog has no pool")
	}
	december := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	if !december.HasPool(context.Background(), "ch", 1, seasonSel) {
		t.Error("in December a Christmas-only catalog has a pool")
	}
}

// Pins skip every matching rule (decision 4): a pinned Christmas ad airs in July.
func TestSeason_PinSkipsTheSeason(t *testing.T) {
	cat := seasonCatalog(4, 1)
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonJul14)
	sel := seasonSel
	sel.Pinned = []string{"xmas00"}
	pod, err := a.PreviewAt(context.Background(), "ch", 1, sel, seasonJul14)
	if err != nil {
		t.Fatal(err)
	}
	if in, _ := inSeasonCount(pod, map[string]bool{"xmas00": true}); in != 1 {
		t.Errorf("the pinned Christmas ad should air in July: %+v", pod.Entries)
	}
}

// A seasonal tag with no dates (an operator's tag the holidays editor has not dated) is not a
// season: its clips air year-round.
func TestSeason_UndatedHolidayAirsYearRound(t *testing.T) {
	vocab, err := holidayvocab.New([]holidayvocab.Holiday{{ID: "advent", Label: "Advent"}})
	if err != nil {
		t.Fatal(err)
	}
	cat := []filler.Clip{seasonAd("advent00", 1994, "advent")}
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US", Holidays: &vocab}, seasonJul14)
	pod, err := a.PreviewAt(context.Background(), "ch", 1, seasonSel, seasonJul14)
	if err != nil {
		t.Fatal(err)
	}
	if in, _ := inSeasonCount(pod, map[string]bool{"advent00": true}); in != 1 {
		t.Errorf("an undated holiday's ad should air in July: %+v", pod.Entries)
	}
}

// Decision 5: in season, at most half of a break's commercials are in-season clips, and at least
// one when one is rested, however the pool leans.
func TestSeason_InSeasonShareIsAtMostHalfAndAtLeastOne(t *testing.T) {
	for _, tc := range []struct {
		name             string
		plain, christmas int
	}{
		{"mostly Christmas", 3, 12},
		{"one Christmas ad among many", 30, 1},
		{"even", 8, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cat := seasonCatalog(tc.plain, tc.christmas)
			xmas := christmasHashes(cat)
			a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
			for seed := range int64(200) {
				pod, err := a.PreviewAt(context.Background(), "ch", seed, seasonSel, seasonDec20)
				if err != nil {
					t.Fatal(err)
				}
				in, body := inSeasonCount(pod, xmas)
				if in < 1 {
					t.Fatalf("seed %d: no Christmas ad in December: %+v", seed, pod.Entries)
				}
				if in > max(1, body/2) {
					t.Fatalf("seed %d: %d of %d commercials are Christmas ads", seed, in, body)
				}
			}
		})
	}
}

// Pinned in-season ads count toward the half: the ladder adds Christmas ads only while the break
// stays at most half Christmas.
func TestSeason_PinnedInSeasonAdsCountTowardTheHalf(t *testing.T) {
	cat := seasonCatalog(30, 10)
	xmas := christmasHashes(cat)
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	sel := seasonSel
	sel.Pinned = []string{"xmas00", "xmas01"}
	for seed := range int64(100) {
		pod, err := a.PreviewAt(context.Background(), "ch", seed, sel, seasonDec20)
		if err != nil {
			t.Fatal(err)
		}
		if in, body := inSeasonCount(pod, xmas); in > max(2, body/2) {
			t.Fatalf("seed %d: %d of %d commercials are Christmas ads with two pinned", seed, in, body)
		}
	}
}

// The in-season ad that is guaranteed a place is a rested one: one still resting inside the
// spacing is not forced back on air.
func TestSeason_ARestingInSeasonAdIsNotForced(t *testing.T) {
	cat := seasonCatalog(30, 1)
	history := &householdHistory{airings: []filler.Airing{{ClipHash: "xmas00",
		AiredAt: seasonDec20.Add(-time.Hour), RecordedAt: seasonDec20.Add(-time.Hour)}}}
	a := seasonAdapter(cat, history, schedule.Calendar{Country: "US"}, seasonDec20)
	for seed := range int64(50) {
		pod, err := a.PreviewAt(context.Background(), "ch", seed, seasonSel, seasonDec20)
		if err != nil {
			t.Fatal(err)
		}
		if in, _ := inSeasonCount(pod, map[string]bool{"xmas00": true}); in != 0 {
			t.Fatalf("seed %d: the resting Christmas ad was forced back on air", seed)
		}
	}
}

// A rollup ancestor is not the clip's holiday: with `christmas` parented under the `holiday` group,
// a Christmas ad carries `holiday` in Tags, and the group is in season at Thanksgiving. The ad is
// still out of season until Christmas.
func TestSeason_ARollupAncestorIsNotTheClipsHoliday(t *testing.T) {
	ad := seasonAd("xmas00", 1994, "christmas")
	ad.Tags = []string{"christmas", "holiday"}
	nov20 := time.Date(2026, time.November, 20, 20, 0, 0, 0, time.UTC)
	a := seasonAdapter(nil, nil, schedule.Calendar{Country: "US"}, nov20)
	if got := a.FitForChannel("ch", seasonSel, ad).Reason; got != filler.FitSeason {
		t.Errorf("fit reason at Thanksgiving = %q, want %q", got, filler.FitSeason)
	}
}

// The end-of-break top-up holds the half too: long plain ads leave a tail only short Christmas
// ads fit, and the top-up must not spend it on them.
func TestSeason_TopUpKeepsTheHalf(t *testing.T) {
	var cat []filler.Clip
	for i := range 6 {
		c := seasonAd(fmt.Sprintf("plain%02d", i), 1994)
		c.DurationMs = 40_000
		cat = append(cat, c)
	}
	for i := range 6 {
		c := seasonAd(fmt.Sprintf("xmas%02d", i), 1994, "christmas")
		c.DurationMs = 7_000
		cat = append(cat, c)
	}
	xmas := christmasHashes(cat)
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	for seed := range int64(100) {
		pod, err := a.PreviewAt(context.Background(), "ch", seed, seasonSel, seasonDec20)
		if err != nil {
			t.Fatal(err)
		}
		if in, body := inSeasonCount(pod, xmas); in > max(1, body/2) {
			t.Fatalf("seed %d: %d of %d commercials are Christmas ads", seed, in, body)
		}
	}
}

// Bumpers and station IDs are not commercials: an in-season station ID never uses up the half.
func TestSeason_InSeasonBookendsDoNotCountTowardTheHalf(t *testing.T) {
	cat := []filler.Clip{seasonAd("plain00", 1994), seasonAd("plain01", 1994),
		seasonAd("xmas00", 1994, "christmas"), seasonAd("xmas01", 1994, "christmas")}
	for i := range 4 {
		id := fmt.Sprintf("xmas-ident%d", i)
		cat = append(cat, filler.Clip{Hash: id, Path: id + ".mp4", Name: id, Kind: filler.StationID, Era: 1994,
			Audience: filler.General, DurationMs: 5_000, Tags: []string{"christmas"}, AssertedTags: []string{"christmas"}})
	}
	xmas := christmasHashes(cat)
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	for seed := range int64(50) {
		pod, err := a.PreviewAt(context.Background(), "ch", seed, seasonSel, seasonDec20)
		if err != nil {
			t.Fatal(err)
		}
		// Four commercials fit the break; with bookends counted, only one Christmas ad would.
		if in, body := inSeasonCount(pod, xmas); body != 4 || in != 2 {
			t.Fatalf("seed %d: %d Christmas ads of %d commercials, want 2 of 4: %+v", seed, in, body, pod.Entries)
		}
		// And the half never refuses a bookend: the top-up spends the spare time on the two
		// Christmas idents the bookends left, though the commercials already hold their half.
		idents := 0
		for _, e := range pod.Entries {
			if e.Kind == filler.StationID {
				idents++
			}
		}
		if idents != 4 {
			t.Fatalf("seed %d: %d Christmas idents aired, want all 4: %+v", seed, idents, pod.Entries)
		}
	}
}

// A break's length never moves with the season: reconcile sizes breaks year-round, so a season
// opening at midnight does not re-size the timeline (the 2026-10-24 regression).
func TestSeason_BreakLengthDoesNotMoveWithTheSeason(t *testing.T) {
	cat := seasonCatalog(2, 6)
	july := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonJul14)
	december := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	for seed := range int64(20) {
		j := july.PlayableDurationMs(context.Background(), "ch", seed, seasonSel)
		d := december.PlayableDurationMs(context.Background(), "ch", seed, seasonSel)
		if j != d {
			t.Fatalf("seed %d: break length %d ms in July, %d ms in December", seed, j, d)
		}
	}
}

// The clip picker says why a Christmas ad cannot air in July, and that it can in December.
func TestSeason_FitReportsOutOfSeason(t *testing.T) {
	ad := seasonAd("xmas00", 1994, "christmas")
	july := seasonAdapter(nil, nil, schedule.Calendar{Country: "US"}, seasonJul14)
	if got := july.FitForChannel("ch", seasonSel, ad).Reason; got != filler.FitSeason {
		t.Errorf("July fit reason = %q, want %q", got, filler.FitSeason)
	}
	december := seasonAdapter(nil, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	if fit := december.FitForChannel("ch", seasonSel, ad); fit.Reason != "" {
		t.Errorf("December fit = %v, want a fit", fit)
	}
}
