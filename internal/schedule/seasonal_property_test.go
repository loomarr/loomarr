package schedule

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

// Property tests for applySeasonal / holidayActive / newYearWindow (§6). These are
// in-package (they call the unexported enforcement directly, no ComputeDesired
// wrapper) and seeded so a failure reproduces from the trial index printed below.
// They complement — never duplicate — the four example tests in seasonal_test.go
// (external package), which pin specific frozen clocks; here we fuzz over random
// dates and entry sets and assert the structural invariants that must hold for ALL
// of them.

// seasonalVocab is a mix of clearly-seasonal titles (each hitting exactly one
// holiday's keyword set) and clearly-non-seasonal ones. Keeping the keyword hits
// disjoint per holiday lets the tests reason about active-vs-inactive membership.
var seasonalVocab = []struct {
	title    string
	seasonal bool // matches SOME built-in holiday keyword
}{
	{"Halloween Spooktacular", true},     // halloween
	{"A Haunted Ghost Story", true},      // halloween
	{"Turkey Day Feast", true},           // thanksgiving
	{"A Christmas Carol", true},          // christmas
	{"Santa's Big Night", true},          // christmas
	{"New Year's Eve Bash", true},        // newyear
	{"Be My Valentine", true},            // valentines
	{"Regular Weekday Show", false},      //
	{"Summer Blockbuster", false},        //
	{"The Quiet Documentary", false},     //
	{"Nine to Five Sitcom", false},       //
	{"Deep Space Adventure Hour", false}, //
}

// randomEntries builds a seeded random slice of LineupEntry drawn from the vocab,
// with distinct keys so kept/benched membership is unambiguous per key.
func randomEntries(g *rand.Rand, n int) []LineupEntry {
	entries := make([]LineupEntry, 0, n)
	for i := 0; i < n; i++ {
		v := seasonalVocab[g.Intn(len(seasonalVocab))]
		key := provision.Key(fmt.Sprintf("movie:tmdb:%d", i))
		entries = append(entries, LineupEntry{
			Key:            key,
			Title:          v.title,
			OfficialRating: "PG",
			Year:           1990 + g.Intn(30),
		})
	}
	return entries
}

// randomDate returns a seeded random UTC instant within a wide multi-year band so
// every holiday window (and the gaps between them) is exercised.
func randomDate(g *rand.Rand) time.Time {
	base := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	// ~8 years of minutes.
	return base.Add(time.Duration(g.Int63n(8*365*24*60)) * time.Minute)
}

// randomResolvedSeasonal returns a seeded ResolvedSeasonal with a CONCRETE mode /
// off-season (never the "" default sentinels — applySeasonal consumes the resolved
// form). Holidays is sometimes a subset, sometimes empty (= all built-ins).
func randomResolvedSeasonal(g *rand.Rand) ResolvedSeasonal {
	modes := []SeasonalMode{SeasonalOff, SeasonalAuto, SeasonalExclusive}
	offs := []OffSeason{OffSeasonLoop, OffSeasonDark}
	allHolidays := []string{"halloween", "thanksgiving", "christmas", "newyear", "valentines"}

	var holidays []string
	switch g.Intn(3) {
	case 0:
		holidays = nil // all built-ins
	case 1:
		holidays = []string{allHolidays[g.Intn(len(allHolidays))]} // single subset
	default:
		// random subset
		for _, h := range allHolidays {
			if g.Intn(2) == 0 {
				holidays = append(holidays, h)
			}
		}
	}
	return ResolvedSeasonal{
		Mode:      modes[g.Intn(len(modes))],
		Holidays:  holidays,
		OffSeason: offs[g.Intn(len(offs))],
	}
}

// keyCounts tallies how many times each key appears in a slice (kept may contain a
// key twice because of the auto boost, §6 :187).
func keyCounts(entries []LineupEntry) map[provision.Key]int {
	m := make(map[provision.Key]int, len(entries))
	for _, e := range entries {
		m[e.Key]++
	}
	return m
}

// reportKeys returns the set of keys the report benched.
func reportKeys(r ExclusionReport) map[provision.Key]struct{} {
	m := make(map[provision.Key]struct{}, len(r.Items))
	for _, it := range r.Items {
		m[it.Key] = struct{}{}
	}
	return m
}

// TestSeasonal_ZeroOrOff_IsIdentity: a zero clock OR SeasonalOff ⇒ output entries
// are EXACTLY the input (same order, same set) and the report is empty. (Invariant 1.)
func TestSeasonal_ZeroOrOff_IsIdentity(t *testing.T) {
	g := rand.New(rand.NewSource(0x5EA5))
	for trial := 0; trial < 500; trial++ {
		entries := randomEntries(g, g.Intn(10))
		rs := randomResolvedSeasonal(g)

		// Two disabling conditions, tested independently.
		var now time.Time
		if trial%2 == 0 {
			// Zero clock, any resolved mode.
			now = time.Time{}
		} else {
			// Real clock, forced OFF mode.
			now = randomDate(g)
			rs.Mode = SeasonalOff
		}
		rp := ResolvedPolicy{Seasonal: rs}

		out, report := applySeasonal(entries, rp, now)

		if len(report.Items) != 0 {
			t.Fatalf("trial %d: disabled seasonality must leave an empty report, got %d items", trial, len(report.Items))
		}
		if len(out) != len(entries) {
			t.Fatalf("trial %d: disabled seasonality must be identity, len(out)=%d want %d", trial, len(out), len(entries))
		}
		for i := range entries {
			if out[i].Key != entries[i].Key {
				t.Fatalf("trial %d: disabled seasonality must preserve order at %d: got %s want %s",
					trial, i, out[i].Key, entries[i].Key)
			}
		}
	}
}

// TestSeasonal_AccountingIsExhaustive: for EVERY input entry, across every mode and
// every random date, the entry is either KEPT (appears >=1 in output) or BENCHED
// (appears in the report) — never lost, never both. And count-wise, kept + benched
// accounts for every input EXACTLY, modulo the single auto-boost duplicate per
// in-window active-seasonal entry (§6 :187). (Invariant 2.)
func TestSeasonal_AccountingIsExhaustive(t *testing.T) {
	g := rand.New(rand.NewSource(0xACC0)) //nolint:gosec
	for trial := 0; trial < 3000; trial++ {
		entries := randomEntries(g, g.Intn(12))
		rp := ResolvedPolicy{Seasonal: randomResolvedSeasonal(g)}
		now := randomDate(g)
		if g.Intn(20) == 0 {
			now = time.Time{} // occasionally exercise the zero-clock path here too
		}

		out, report := applySeasonal(entries, rp, now)

		keptCounts := keyCounts(out)
		benched := reportKeys(report)

		// Every input key is accounted for: kept XOR benched.
		for _, e := range entries {
			_, kept := keptCounts[e.Key]
			_, isBenched := benched[e.Key]
			if kept == isBenched {
				// kept==isBenched means (neither) or (both) — both are accounting bugs.
				t.Fatalf("trial %d (mode=%s off=%s now=%s): key %s kept=%v benched=%v — must be exactly one",
					trial, rp.Seasonal.Mode, rp.Seasonal.OffSeason, now, e.Key, kept, isBenched)
			}
		}

		// The report must never mention a key that wasn't an input.
		inputKeys := keyCounts(entries)
		for k := range benched {
			if _, ok := inputKeys[k]; !ok {
				t.Fatalf("trial %d: report benched a non-input key %s", trial, k)
			}
		}

		// Count-wise exhaustiveness: distinct-kept + benched == distinct inputs; and
		// total output slots == distinct-kept + (number of boost duplicates), where a
		// duplicate is any output copy beyond the first for a key.
		distinctInputs := len(inputKeys)
		distinctKept := len(keptCounts)
		if distinctKept+len(benched) != distinctInputs {
			t.Fatalf("trial %d (mode=%s): distinctKept(%d)+benched(%d) != distinctInputs(%d)",
				trial, rp.Seasonal.Mode, distinctKept, len(benched), distinctInputs)
		}

		// No key is ever kept more than TWICE (the boost adds at most one extra copy,
		// §6 :187) — a key kept 3+ times would be an unbounded-duplication bug.
		for k, c := range keptCounts {
			if c > 2 {
				t.Fatalf("trial %d: key %s kept %d times, boost must add at most one copy", trial, k, c)
			}
		}
	}
}

// TestSeasonal_ExclusiveDarkOutOfWindow_IsEmpty: in exclusive mode with
// OffSeason=dark, ANY date that is out of window (no selected holiday active) yields
// an EMPTY program set, with every input benched as out_of_season. (Invariant 3.)
//
// We construct the out-of-window condition deterministically: pick the "valentines"
// holiday and a date far from every window (mid-July), so no holiday is active.
func TestSeasonal_ExclusiveDarkOutOfWindow_IsEmpty(t *testing.T) {
	g := rand.New(rand.NewSource(0xDA54)) //nolint:gosec
	for trial := 0; trial < 500; trial++ {
		entries := randomEntries(g, 1+g.Intn(10))
		// A date guaranteed to fall in NO built-in window: mid-July (Jul 3–14). The
		// built-in windows are Oct, mid-Nov, Dec, Dec27–Jan2, Feb1–14 — none cover July.
		year := 2020 + g.Intn(8)
		day := 3 + g.Intn(12)
		now := time.Date(year, time.July, day, 12, 0, 0, 0, time.UTC)

		rp := ResolvedPolicy{Seasonal: ResolvedSeasonal{
			Mode:      SeasonalExclusive,
			OffSeason: OffSeasonDark,
			// Any non-empty holiday subset; none of them are active in July.
			Holidays: []string{"christmas", "valentines"},
		}}

		out, report := applySeasonal(entries, rp, now)

		if len(out) != 0 {
			t.Fatalf("trial %d: exclusive+dark out of window must be empty, got %d", trial, len(out))
		}
		if len(report.Items) != len(entries) {
			t.Fatalf("trial %d: exclusive+dark out of window must bench every input: benched %d want %d",
				trial, len(report.Items), len(entries))
		}
		for _, it := range report.Items {
			if it.Reason != "out_of_season" {
				t.Fatalf("trial %d: bench reason must be out_of_season, got %q", trial, it.Reason)
			}
		}
	}
}

// TestSeasonal_NewYearBoundary_AlwaysActive: any date in [Dec 27, Jan 2] (the
// year-straddling window, §6) is ALWAYS active for the newyear holiday, across many
// years and both sides of the boundary. This exercises holidayActive's prior-year
// check and newYearWindow directly. (Invariant 4.)
func TestSeasonal_NewYearBoundary_AlwaysActive(t *testing.T) {
	g := rand.New(rand.NewSource(0x0EFE)) //nolint:gosec
	var newyear holiday
	for _, h := range builtinCalendar {
		if h.id == "newyear" {
			newyear = h
		}
	}
	if newyear.id == "" {
		t.Fatal("newyear holiday not found in builtinCalendar")
	}

	for trial := 0; trial < 2000; trial++ {
		year := 2000 + g.Intn(60)
		// Pick a random instant inside the boundary window [Dec 27 year, Jan 2 year+1].
		// Enumerate the seven boundary days explicitly so both sides are covered.
		days := []struct {
			y int
			m time.Month
			d int
		}{
			{year, time.December, 27},
			{year, time.December, 28},
			{year, time.December, 29},
			{year, time.December, 30},
			{year, time.December, 31},
			{year + 1, time.January, 1},
			{year + 1, time.January, 2},
		}
		pick := days[g.Intn(len(days))]
		// Random time-of-day within the day.
		now := time.Date(pick.y, pick.m, pick.d,
			g.Intn(24), g.Intn(60), g.Intn(60), 0, time.UTC)

		if !holidayActive(newyear, now) {
			t.Fatalf("trial %d: newyear must be active at %s (inside [Dec27,Jan2])", trial, now)
		}

		// And a sanity anchor on newYearWindow itself: the picked instant is within the
		// window computed for the DECEMBER-side year (the window's canonical start year).
		start, end := newYearWindow(year)
		if now.Before(start) || now.After(end) {
			t.Fatalf("trial %d: %s outside newYearWindow(%d)=[%s,%s]", trial, now, year, start, end)
		}
	}
}

// TestSeasonal_NewYearWindow_Shape: newYearWindow(year) is exactly
// [Dec 27 year 00:00, the final nanosecond of Jan 2 year+1], straddling the boundary. Fuzz over
// years to catch any off-by-year regression. (Supports invariant 4.)
func TestSeasonal_NewYearWindow_Shape(t *testing.T) {
	g := rand.New(rand.NewSource(0x0517)) //nolint:gosec
	for trial := 0; trial < 500; trial++ {
		year := 1970 + g.Intn(200)
		start, end := newYearWindow(year)

		wantStart := time.Date(year, time.December, 27, 0, 0, 0, 0, time.UTC)
		wantEnd := time.Date(year+1, time.January, 3, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)
		if !start.Equal(wantStart) {
			t.Fatalf("trial %d: newYearWindow(%d) start=%s want %s", trial, year, start, wantStart)
		}
		if !end.Equal(wantEnd) {
			t.Fatalf("trial %d: newYearWindow(%d) end=%s want %s", trial, year, end, wantEnd)
		}
		// The window must cross a year boundary (end year > start year).
		if end.Year() <= start.Year() {
			t.Fatalf("trial %d: newYearWindow(%d) must straddle the boundary, got [%s,%s]", trial, year, start, end)
		}
		// A date just OUTSIDE each edge must not be in the window (tight bounds).
		justBefore := wantStart.Add(-time.Second)
		justAfter := wantEnd.Add(time.Second)
		if !justBefore.Before(start) || !justAfter.After(end) {
			t.Fatalf("trial %d: window bounds not tight for year %d", trial, year)
		}
	}
}

// FuzzHolidayActive_NewYearBoundary: native fuzzing (Go 1.18+) over a raw unix
// second — every second inside the [Dec27,Jan2] window (for the year of that
// instant) must be active for newyear, and holidayActive must never panic on any
// instant. This backstops the seeded loop with the fuzzer's corpus exploration.
func FuzzHolidayActive_NewYearBoundary(f *testing.F) {
	// Seed corpus: a few boundary instants and a few clearly-outside ones.
	f.Add(time.Date(2026, time.December, 27, 0, 0, 0, 0, time.UTC).Unix())
	f.Add(time.Date(2027, time.January, 2, 23, 59, 59, 0, time.UTC).Unix())
	f.Add(time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC).Unix())
	f.Add(time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC).Unix())
	f.Add(int64(0))

	var newyear holiday
	for _, h := range builtinCalendar {
		if h.id == "newyear" {
			newyear = h
		}
	}

	f.Fuzz(func(t *testing.T, unix int64) {
		now := time.Unix(unix, 0).UTC()
		got := holidayActive(newyear, now)

		// Compute the ground-truth membership: now is in the boundary window if it
		// falls in newYearWindow(now.Year()) OR newYearWindow(now.Year()-1). This is
		// the same disjunction holidayActive uses, expressed independently.
		curS, curE := newYearWindow(now.Year())
		prevS, prevE := newYearWindow(now.Year() - 1)
		want := within(now, curS, curE) || within(now, prevS, prevE)

		if got != want {
			t.Fatalf("holidayActive(newyear, %s) = %v, want %v", now, got, want)
		}
	})
}

// within reports whether t is in the inclusive [start,end] window.
func within(t, start, end time.Time) bool {
	return !t.Before(start) && !t.After(end)
}
