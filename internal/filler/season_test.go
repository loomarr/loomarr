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
				// Past the half only once every plain ad is in the break (decision 34).
				if in > max(1, body/2) && body-in < tc.plain {
					t.Fatalf("seed %d: %d of %d commercials are Christmas ads with a plain ad left", seed, in, body)
				}
			}
		})
	}
}

// Decision 34: the half yields only to the card. With one plain ad in the catalog, Christmas ads
// fill the break instead of leaving it to the card, and the plain ad still airs.
func TestSeason_TheHalfYieldsWhenTheCardIsTheAlternative(t *testing.T) {
	cat := seasonCatalog(1, 12)
	xmas := christmasHashes(cat)
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	for seed := range int64(100) {
		pod, err := a.PreviewAt(context.Background(), "ch", seed, seasonSel, seasonDec20)
		if err != nil {
			t.Fatal(err)
		}
		if in, body := inSeasonCount(pod, xmas); body-in != 1 || in < 2 {
			t.Fatalf("seed %d: %d of %d commercials are Christmas ads, want all but the plain one, past the half", seed, in, body)
		}
		if left := 120_000 - pod.TotalMs; left >= 15_000 {
			t.Fatalf("seed %d: %d ms left to the card with Christmas ads that fit", seed, left)
		}
	}
}

// Decision 35: the guaranteed in-season ad takes a seeded place among the commercials, so a break
// does not always open on it.
func TestSeason_TheGuaranteedAdMovesAroundTheBreak(t *testing.T) {
	cat := seasonCatalog(30, 1)
	xmas := christmasHashes(cat)
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	positions := map[int]bool{}
	for seed := range int64(100) {
		pod, err := a.PreviewAt(context.Background(), "ch", seed, seasonSel, seasonDec20)
		if err != nil {
			t.Fatal(err)
		}
		at := -1
		n := 0
		for _, e := range pod.Entries {
			if e.Kind != filler.Commercial {
				continue
			}
			if xmas[e.Hash] {
				at = n
			}
			n++
		}
		if at < 0 {
			t.Fatalf("seed %d: no Christmas ad in December: %+v", seed, pod.Entries)
		}
		positions[at] = true
	}
	if len(positions) < 3 {
		t.Errorf("the Christmas ad took only positions %v over 100 breaks", positions)
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

// Decision 34 at the end of the break: long plain ads leave a tail only short Christmas ads fit, so
// the top-up spends it on them rather than leaving it to the card, and the plain ads still air.
func TestSeason_TopUpSpendsATailOnlyHolidayAdsFit(t *testing.T) {
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
		in, body := inSeasonCount(pod, xmas)
		if body-in != 2 {
			t.Fatalf("seed %d: %d plain ads aired, want the 2 that fit", seed, body-in)
		}
		if left := 120_000 - pod.TotalMs; left >= 7_000 {
			t.Fatalf("seed %d: %d ms left unfilled with Christmas ads that fit", seed, left)
		}
	}
}

// The guaranteed ad's seeded place keeps category variety: it never lands beside an ad of its own
// category. Every other ad's category is distinct, so only that move could make a repeat.
func TestSeason_TheGuaranteedAdKeepsCategoryVariety(t *testing.T) {
	var cat []filler.Clip
	for i := range 8 {
		c := seasonAd(fmt.Sprintf("plain%02d", i), 1994)
		c.Category = fmt.Sprintf("cat%02d", i)
		cat = append(cat, c)
	}
	for i := 1; i < 8; i++ {
		c := seasonAd(fmt.Sprintf("cereal%02d", i), 1994)
		c.Category = "cereal"
		cat = append(cat, c)
	}
	x := seasonAd("xmas", 1994, "christmas")
	x.Category = "cereal"
	cat = append(cat, x)
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	for seed := range int64(200) {
		pod, err := a.PreviewAt(context.Background(), "ch", seed, seasonSel, seasonDec20)
		if err != nil {
			t.Fatal(err)
		}
		var ads []filler.PodEntry
		for _, e := range pod.Entries {
			if e.Kind == filler.Commercial {
				ads = append(ads, e)
			}
		}
		for i, e := range ads {
			if e.Hash != "xmas" {
				continue
			}
			for _, j := range []int{i - 1, i + 1} {
				if j >= 0 && j < len(ads) && ads[j].Category == "cereal" {
					t.Fatalf("seed %d: the Christmas ad sits beside %s", seed, ads[j].Hash)
				}
			}
		}
	}
}

// The half yields only when nothing else fits, and a station ID fits: the tail spends its time on
// them before it adds Christmas ads past the half.
func TestSeason_StationIDsFillTheTailBeforeTheHalfYields(t *testing.T) {
	var cat []filler.Clip
	for i := range 6 {
		c := seasonAd(fmt.Sprintf("plain%02d", i), 1994)
		c.DurationMs = 40_000
		cat = append(cat, c)
		x := seasonAd(fmt.Sprintf("xmas%02d", i), 1994, "christmas")
		x.DurationMs = 7_000
		cat = append(cat, x)
		cat = append(cat, filler.Clip{Hash: fmt.Sprintf("ident%02d", i), Path: fmt.Sprintf("ident%02d.mp4", i),
			Kind: filler.StationID, Era: 1994, Audience: filler.General, DurationMs: 5_000})
	}
	a := seasonAdapter(cat, nil, schedule.Calendar{Country: "US"}, seasonDec20)
	for seed := range int64(100) {
		pod, err := a.PreviewAt(context.Background(), "ch", seed, seasonSel, seasonDec20)
		if err != nil {
			t.Fatal(err)
		}
		if in, body := inSeasonCount(pod, christmasHashes(cat)); in > max(1, body/2) {
			t.Fatalf("seed %d: %d of %d commercials are Christmas ads while station IDs fit: %+v", seed, in, body, pod.Entries)
		}
		idents := 0
		for _, e := range pod.Entries {
			if e.Kind == filler.StationID {
				idents++
			}
		}
		if idents < 4 { // two bookends, and the tail's 23 s holds at least two more
			t.Fatalf("seed %d: only %d station IDs aired: %+v", seed, idents, pod.Entries)
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
