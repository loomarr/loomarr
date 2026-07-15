package schedule

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"
)

// FuzzDurationJSON asserts the Duration newtype round-trips through JSON
// (Unmarshal(Marshal(d)) == d) and that Unmarshal never panics on arbitrary
// bytes. The marshal side goes through time.Duration.String()/ParseDuration,
// which is exact for any int64 nanosecond value, so the round-trip must hold
// for every seed value.
func FuzzDurationJSON(f *testing.F) {
	for _, ns := range []int64{
		0,
		int64(168 * time.Hour),
		int64(720 * time.Hour),
		int64(2 * time.Hour),
		1,
		-1,
		int64(time.Nanosecond),
		1<<63 - 1, // max int64
		-1 << 62,
	} {
		f.Add(ns)
	}
	f.Fuzz(func(t *testing.T, ns int64) {
		d := Duration(ns)
		blob, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("marshal(%d): %v", ns, err)
		}
		var got Duration
		if err := json.Unmarshal(blob, &got); err != nil {
			t.Fatalf("unmarshal(%s) from marshal(%d): %v", blob, ns, err)
		}
		if got != d {
			t.Fatalf("round-trip: marshal(%d)=%s unmarshaled to %d", ns, blob, int64(got))
		}
	})
}

// FuzzDurationUnmarshalNeverPanics feeds arbitrary bytes to Duration.UnmarshalJSON
// and asserts it either errors cleanly or decodes — it must never panic. Valid
// duration strings and bare integers (tolerated as nanoseconds) decode; anything
// else is a clean error, never a crash or silent garbage.
func FuzzDurationUnmarshalNeverPanics(f *testing.F) {
	for _, b := range []string{
		`"168h"`, `"2h30m"`, `"0s"`, `"-5m"`, `168`, `0`, `-1`,
		`"not-a-duration"`, `""`, `null`, `[]`, `{}`, `"  "`, `1.5`,
		`"168h0m0s"`, `true`, `9223372036854775807`,
	} {
		f.Add([]byte(b))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		var d Duration
		// Must not panic. Both a nil error and a non-nil error are acceptable.
		err := d.UnmarshalJSON(b)
		if err == nil {
			// If it claimed success, re-marshaling must produce something that
			// itself round-trips (the value is a valid Duration).
			blob, merr := json.Marshal(d)
			if merr != nil {
				t.Fatalf("marshal after successful unmarshal(%s): %v", b, merr)
			}
			var again Duration
			if uerr := again.UnmarshalJSON(blob); uerr != nil {
				t.Fatalf("re-unmarshal(%s) after unmarshal(%s): %v", blob, b, uerr)
			}
			if again != d {
				t.Fatalf("unmarshal(%s) not stable: %d -> %s -> %d", b, int64(d), blob, int64(again))
			}
		}
	})
}

// onLadder reports whether r is one of the concrete ladder ratings.
func onLadder(r Rating) bool {
	_, ok := ladderRank[r]
	return ok
}

// FuzzNormalizeRating asserts two invariants of normalizeRating over arbitrary
// raw strings:
//   - idempotence: normalize(normalize(x)) == normalize(x). Once folded onto the
//     ladder (or to unrated ""), re-folding is a no-op.
//   - totality: the result is always either "" (unrated) or an on-ladder rating —
//     never some half-normalized off-ladder string.
func FuzzNormalizeRating(f *testing.F) {
	for _, s := range []string{
		"TV-Y7", "tv-y7", "TV_Y7", "Rated R", " pg-13 ", "NR", "Unknown",
		"", "   ", "rated tv-ma", "PG", "g", "TV-MA", "NC-17", "RATEDR",
		"____", "TV Y7", "nc_17", "\x00", "🎬", "tv-14 ",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		once := normalizeRating(raw)
		// Totality: on-ladder or the unrated sentinel.
		if once != "" && !onLadder(once) {
			t.Fatalf("normalizeRating(%q) = %q, which is neither \"\" nor on the ladder", raw, once)
		}
		// Idempotence.
		twice := normalizeRating(string(once))
		if twice != once {
			t.Fatalf("normalizeRating not idempotent: normalize(%q)=%q, normalize(%q)=%q", raw, once, once, twice)
		}
	})
}

// allStrategies is the closed set of channel strategies Resolved() inherits from.
var allStrategies = []Strategy{Sequential, Shuffle, TimeSlot}

// ladderRatings is every concrete ceiling on the rating ladder, used to drive the
// property loop across kids and non-kids ceilings.
var ladderRatings = []Rating{
	"TV-Y", "TV-Y7", "TV-G", "G", "TV-PG", "PG", "TV-14", "PG-13", "TV-MA", "R", "NC-17",
}

// randomPolicy builds a ChannelPolicy from r, exercising every resolvable field
// with a mix of zero (default) and concrete values.
func randomPolicy(r *rand.Rand) ChannelPolicy {
	orderings := []OrderingMode{OrderInherit, OrderSequential, OrderShuffle, OrderSyndication}
	seasonalModes := []SeasonalMode{SeasonalDefault, SeasonalOff, SeasonalAuto, SeasonalExclusive}
	offSeasons := []OffSeason{OffSeasonDefault, OffSeasonLoop, OffSeasonDark}
	unrateds := []UnratedPolicy{UnratedDefault, UnratedExclude, UnratedAllow}

	// Ceiling: sometimes none, otherwise a random ladder rating.
	var ceiling Rating
	if r.Intn(4) != 0 {
		ceiling = ladderRatings[r.Intn(len(ladderRatings))]
	}

	// Durations: sometimes zero (→ default), sometimes a concrete positive value.
	dur := func() Duration {
		if r.Intn(3) == 0 {
			return 0
		}
		return Duration(time.Duration(1+r.Intn(1000)) * time.Minute)
	}
	blockMax := 0
	if r.Intn(3) != 0 {
		blockMax = 1 + r.Intn(8)
	}

	return ChannelPolicy{
		Audience: AudiencePolicy{
			Ceiling: ceiling,
			Unrated: unrateds[r.Intn(len(unrateds))],
		},
		Separation: SeparationPolicy{
			EpisodeNoRepeat: dur(),
			MovieNoRepeat:   dur(),
			SeriesMinGap:    dur(),
			BlockMax:        blockMax,
		},
		Ordering: orderings[r.Intn(len(orderings))],
		Seasonal: SeasonalPolicy{
			Mode:      seasonalModes[r.Intn(len(seasonalModes))],
			OffSeason: offSeasons[r.Intn(len(offSeasons))],
		},
	}
}

// TestResolved_IsTotal is a seeded property loop asserting the core Resolved()
// contract: it produces a TOTAL policy with no zero-means-default leaks, applies
// the single-series relax rule, and resolves the unrated gate fail-closed under a
// kids ceiling. Randomness is seeded so any failure reproduces.
func TestResolved_IsTotal(t *testing.T) {
	const seed = 0x10ACC0DE
	r := rand.New(rand.NewSource(seed))

	for i := 0; i < 4000; i++ {
		p := randomPolicy(r)
		strategy := allStrategies[r.Intn(len(allStrategies))]
		singleSeries := r.Intn(2) == 0

		res := p.Resolved(strategy, singleSeries)

		// --- Totality: every resolvable field is concrete, no zero leaks. -------

		// Ordering never leaks the inherit sentinel.
		if res.Ordering == OrderInherit {
			t.Fatalf("iter %d: Resolved left Ordering as inherit (\"\") for strategy=%q policy=%+v", i, strategy, p)
		}
		if !res.Ordering.valid() {
			t.Fatalf("iter %d: Resolved produced an invalid ordering %q", i, res.Ordering)
		}

		// Unrated resolves to a concrete allow|exclude, never the default sentinel.
		if res.Unrated != UnratedExclude && res.Unrated != UnratedAllow {
			t.Fatalf("iter %d: Resolved left Unrated = %q, want a concrete allow|exclude", i, res.Unrated)
		}

		// Seasonal mode/off-season are concrete (never the "" default sentinels).
		if res.Seasonal.Mode == SeasonalDefault {
			t.Fatalf("iter %d: Resolved left Seasonal.Mode as default \"\"", i)
		}
		if res.Seasonal.OffSeason == OffSeasonDefault {
			t.Fatalf("iter %d: Resolved left Seasonal.OffSeason as default \"\"", i)
		}

		// Separation windows are all filled: episode/movie no-repeat are never zero
		// (no policy value maps them to zero), and are positive.
		if res.Sep.EpisodeNoRepeat <= 0 {
			t.Fatalf("iter %d: EpisodeNoRepeat = %v, want positive", i, res.Sep.EpisodeNoRepeat)
		}
		if res.Sep.MovieNoRepeat <= 0 {
			t.Fatalf("iter %d: MovieNoRepeat = %v, want positive", i, res.Sep.MovieNoRepeat)
		}

		// --- singleSeries ⇒ series-level separation relaxes to unbounded. -------
		if singleSeries {
			if res.Sep.SeriesMinGap != 0 {
				t.Fatalf("iter %d: singleSeries must relax SeriesMinGap to 0, got %v", i, res.Sep.SeriesMinGap)
			}
			if res.Sep.BlockMax != 0 {
				t.Fatalf("iter %d: singleSeries must relax BlockMax to 0, got %d", i, res.Sep.BlockMax)
			}
		} else {
			// A mixed channel keeps a bounded (positive) series gap and block max.
			if res.Sep.SeriesMinGap <= 0 {
				t.Fatalf("iter %d: mixed channel SeriesMinGap = %v, want positive", i, res.Sep.SeriesMinGap)
			}
			if res.Sep.BlockMax <= 0 {
				t.Fatalf("iter %d: mixed channel BlockMax = %d, want positive", i, res.Sep.BlockMax)
			}
		}

		// --- Unrated fail-closed gate: default resolves by the ceiling rank. ----
		// Only exercise the DEFAULT case; an explicit unrated policy always wins
		// and is asserted separately below.
		if p.Audience.Unrated == UnratedDefault {
			rank, hasCeiling := ladderRank[p.Audience.Ceiling]
			kidsCeiling := hasCeiling && res.Ceiling != "" && rank <= kidsCeilingRank
			if kidsCeiling {
				if res.Unrated != UnratedExclude {
					t.Fatalf("iter %d: unrated under kids ceiling %q (rank %d) = %q, want exclude", i, p.Audience.Ceiling, rank, res.Unrated)
				}
			} else {
				if res.Unrated != UnratedAllow {
					t.Fatalf("iter %d: unrated under ceiling %q = %q, want allow", i, p.Audience.Ceiling, res.Unrated)
				}
			}
		} else {
			// Explicit policy wins verbatim.
			if res.Unrated != p.Audience.Unrated {
				t.Fatalf("iter %d: explicit unrated %q was overridden to %q", i, p.Audience.Unrated, res.Unrated)
			}
		}

		// Ceiling passes through unchanged.
		if res.Ceiling != p.Audience.Ceiling {
			t.Fatalf("iter %d: Ceiling changed %q -> %q", i, p.Audience.Ceiling, res.Ceiling)
		}
	}
}

// TestResolved_UnratedExcludesUnderEveryKidsCeiling exhaustively pins the
// fail-closed direction: with the default unrated policy, EVERY ladder rating at
// or below the kids ceiling rank resolves to exclude, and every one above it (plus
// no ceiling) resolves to allow — under any strategy and single/mixed shape.
func TestResolved_UnratedExcludesUnderEveryKidsCeiling(t *testing.T) {
	for _, ceiling := range append([]Rating{""}, ladderRatings...) {
		rank, hasCeiling := ladderRank[ceiling]
		wantExclude := hasCeiling && ceiling != "" && rank <= kidsCeilingRank
		for _, strategy := range allStrategies {
			for _, single := range []bool{false, true} {
				p := ChannelPolicy{Audience: AudiencePolicy{Ceiling: ceiling}}
				got := p.Resolved(strategy, single).Unrated
				want := UnratedAllow
				if wantExclude {
					want = UnratedExclude
				}
				if got != want {
					t.Errorf("ceiling %q (rank %d, kids=%v) strategy=%q single=%v: unrated=%q want=%q",
						ceiling, rank, wantExclude, strategy, single, got, want)
				}
			}
		}
	}
}
