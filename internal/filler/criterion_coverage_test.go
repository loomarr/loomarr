package filler

import "testing"

// The per-criterion breakdown (§10 V51f) — "which of my settings is costing me the clips".
//
// ⚠ **These assert the breakdown against `candidatePools`, never against numbers written by
// hand.** `coverage.go` exists because the mock's meter recomputed its buckets inline and
// disagreed with what aired; a breakdown that drifts is a worse version of that, because it names
// a specific setting for the operator to change.

// ⚠ **`Tags` is what category narrowing matches; `Category` is its DERIVED SHADOW (§10 V45a).**
// Written out rather than collapsed to one field, because a fixture that set only `Category` would
// report zero for the category criterion and look like a bug in the code under test — it cost a
// round trip here. Both are set, consistently, the way the tagger writes them.
func bodyClip(id string, era int, aud Audience, category string) Clip {
	return Clip{
		Hash: id, Path: id, Kind: Commercial, Era: era, Audience: aud,
		Category: category, Tags: []string{category}, DurationMs: 30_000,
	}
}

func breakdownCatalog() []Clip {
	return []Clip{
		// Four kids commercials across two eras and two categories.
		bodyClip("k92-toys", 1992, Kids, "toys"),
		bodyClip("k94-toys", 1994, Kids, "toys"),
		bodyClip("k92-food", 1992, Kids, "food"),
		bodyClip("k75-toys", 1975, Kids, "toys"),
		// A late-night one — wrong audience for a kids channel.
		bodyClip("ln92", 1992, LateNight, "toys"),
		// A bumper: bookend material, never break BODY, so it is outside the base entirely.
		{Hash: "bump", Path: "bump", Kind: Bumper, Era: 1992, Audience: General, DurationMs: 5_000},
	}
}

func criterion(report CoverageReport, c Criterion) int {
	for _, e := range report.Criteria {
		if e.Criterion == c {
			return e.Clips
		}
	}
	return -1
}

func TestCriterionCoverage_CountsEachSettingIndependently(t *testing.T) {
	w := Window{
		Era: EraRange{From: 1990, To: 1999}, Audience: Kids,
		Categories: []string{"toys"}, GapMs: 120_000, PodMax: 4,
	}
	report := Coverage(breakdownCatalog(), w, Policy{})

	for _, tc := range []struct {
		c    Criterion
		want int
		why  string
	}{
		// 1992, 1994, 1992 and the late-night 1992 are all inside 1990–1999; the 1975 one is not.
		{CriterionEra, 4, "clips inside 1990-1999, whatever else is wrong with them"},
		// The four kids clips; the late-night one is not admitted to a kids channel.
		{CriterionAudience, 4, "clips a kids channel may draw"},
		// Three "toys" clips among the commercials, plus the late-night toys one.
		{CriterionCategory, 4, "clips tagged toys"},
		// No kind narrowing, so every commercial passes.
		{CriterionKind, 5, "commercials under the default kind set"},
		{CriterionDuration, 5, "no bounds set, so everything is eligible"},
		{CriterionQuality, 5, "no floor set, so everything is eligible"},
	} {
		if got := criterion(report, tc.c); got != tc.want {
			t.Errorf("%s = %d, want %d (%s)", tc.c, got, tc.want, tc.why)
		}
	}
}

// ⚠ **The failure this whole feature is for: one setting at zero while the rest look healthy.**
// A kids channel on a late-night-only catalog has plenty of clips in era, category and duration —
// and cannot fill a single break. Before V51f the meter said "nothing in the catalog fits", which
// reads as "get more clips" rather than "change this one setting".
func TestCriterionCoverage_NamesTheOneSettingThatIsEmptyingTheLadder(t *testing.T) {
	cat := []Clip{
		{Hash: "a", Path: "a", Kind: Commercial, Era: 1992, Audience: LateNight, Category: "toys", DurationMs: 30_000},
		{Hash: "b", Path: "b", Kind: Commercial, Era: 1994, Audience: LateNight, Category: "toys", DurationMs: 30_000},
	}
	w := Window{Era: EraRange{From: 1990, To: 1999}, Audience: Kids, GapMs: 120_000, PodMax: 4}
	report := Coverage(cat, w, Policy{})

	if got := criterion(report, CriterionAudience); got != 0 {
		t.Errorf("audience = %d, want 0 — it is the setting emptying the ladder", got)
	}
	for _, c := range []Criterion{CriterionEra, CriterionCategory, CriterionKind, CriterionDuration} {
		if got := criterion(report, c); got != 2 {
			t.Errorf("%s = %d, want 2 — only the audience should read empty", c, got)
		}
	}
	if report.Level != MatchBumperCard {
		t.Errorf("level = %s, want bumper_card — nothing can fill this break", report.Level)
	}
}

// ⚠ The breakdown must AGREE with the ladder it describes: if a criterion reports zero, no rung
// can hold anything. This is the assertion that catches the two drifting apart, rather than a
// count someone typed.
func TestCriterionCoverage_AgreesWithTheLadder(t *testing.T) {
	for _, w := range []Window{
		{Era: EraRange{From: 1990, To: 1999}, Audience: Kids, GapMs: 120_000, PodMax: 4},
		{Era: Year(1992), Audience: LateNight, Categories: []string{"toys"}, GapMs: 120_000, PodMax: 4},
		{Era: EraRange{}, Audience: Family, GapMs: 120_000, PodMax: 4},
		{Era: Year(1800), Audience: General, GapMs: 120_000, PodMax: 4},
	} {
		report := Coverage(breakdownCatalog(), w, Policy{})

		// The audience criterion bounds the bottom rung: a rung can never hold a clip the
		// audience setting excludes, because the rung is built from that same predicate.
		if bottom := report.Rungs[len(report.Rungs)-1].Clips; bottom > criterion(report, CriterionAudience) {
			t.Errorf("window %+v: bottom rung holds %d clips but only %d pass the audience setting",
				w, bottom, criterion(report, CriterionAudience))
		}
		// Likewise the era criterion bounds the exact rung.
		if exact := report.Rungs[0].Clips; exact > criterion(report, CriterionEra) {
			t.Errorf("window %+v: exact rung holds %d clips but only %d are inside the era",
				w, exact, criterion(report, CriterionEra))
		}
	}
}
