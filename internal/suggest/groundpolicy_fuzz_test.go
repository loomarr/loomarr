package suggest

import (
	"testing"

	"github.com/mantonx/loomarr/internal/schedule"
)

// onLadderCeilings are the canonical rating strings NormalizeRating maps to itself
// (already upper-cased, no separators/variants). groundPolicy may only ever produce
// a ceiling from this closed set or "" — a hallucinated rating is dropped. Kept in
// sync with schedule.ladderRank (policy.go §4). If the ladder grows, this must too.
var onLadderCeilings = map[schedule.Rating]bool{
	"TV-Y": true, "TV-Y7": true,
	"TV-G": true, "G": true,
	"TV-PG": true, "PG": true,
	"TV-14": true, "PG-13": true,
	"TV-MA": true, "R": true, "NC-17": true,
}

// FuzzGroundPolicy is the untrusted-LLM policy boundary (suggester.go groundPolicy,
// programming-design §8: the model EXTRACTS rule values, deterministic code
// ENFORCES). It constructs a pickPolicy entirely from fuzz-provided bytes — every
// field is model-controlled garbage — and asserts the CORE SAFETY PROPERTY: no
// matter what the model emits, groundPolicy NEVER returns a schedule.ChannelPolicy
// that fails Validate(). A policy that reaches the enforcer is always a valid one.
//
// It also pins the two grounding sub-properties the boundary promises:
//   - an off-ladder ceiling never survives (result ceiling is "" or on-ladder);
//   - unknown ordering / seasonal enums are dropped (result enum is "" or known).
func FuzzGroundPolicy(f *testing.F) {
	// Seed corpus: a realistic valid policy, an all-hallucinated one, boundary
	// years, and the empty case — so the corpus exercises both survive + drop paths
	// before the fuzzer explores. Field order: ceiling, unrated, from, to, ordering,
	// seasonalMode, genreInc, genreExc, holiday.
	f.Add("TV-Y7", "exclude", 1990, 1999, "syndication", "auto", "Animation", "Horror", "christmas")
	f.Add("TV-SUPERSAFE", "maybe", -5, 999999, "sideways", "eventually", "", "", "")
	f.Add("tv_14", "allow", 0, 0, "shuffle", "off", "Comedy", "", "halloween")
	f.Add("", "", 0, 0, "", "", "", "", "")
	f.Add("Rated R", "", 2020, 1980, "sequential", "exclusive", "Sci-Fi", "Romance", "")
	f.Add("NC-17", "EXCLUDE", 1, 2147483647, "SEQUENTIAL", "AUTO", "Drama", "News", "newyear")

	f.Fuzz(func(t *testing.T,
		ceiling, unrated string,
		from, to int,
		ordering, seasonalMode string,
		genreInc, genreExc, holiday string,
	) {
		var raw pickPolicy
		raw.Audience.Ceiling = ceiling
		raw.Audience.Unrated = unrated
		raw.Era.From = from
		raw.Era.To = to
		raw.Ordering = ordering
		raw.Seasonal.Mode = seasonalMode
		raw.Seasonal.Holidays = []string{holiday}
		raw.Genres.Include = []string{genreInc}
		raw.Genres.Exclude = []string{genreExc}

		// Exercise the actual boundary. Lineup/acquisitions are irrelevant to the
		// scalar-policy safety property under test (they only affect series scoping,
		// which groundPolicy does not derive from these fields), so pass nil. Use a
		// kids-signaling intent so a proposed ceiling is KEPT (and thus canonicalized) —
		// the property under test is ceiling CANONICALIZATION; the drop-when-no-kids-signal
		// path (§4/§8) has its own dedicated tests. The no-signal case would simply drop
		// every ceiling to "", which trivially satisfies Validate but exercises nothing.
		p := groundPolicy(&raw, nil, nil, Intent{Description: "for kids"})

		// CORE SAFETY PROPERTY: whatever the model emitted, the returned policy is
		// always enforcement-valid. groundPolicy's final net drops to the empty
		// policy on any residual invalidity, and the empty policy validates — so
		// this must hold for every input.
		if err := p.Validate(); err != nil {
			t.Fatalf("groundPolicy returned a policy that fails Validate: %v\ninput=%+v\nresult=%+v", err, raw, p)
		}

		// Ceiling grounding: an off-ladder / hallucinated ceiling is dropped. The
		// survivor is either empty or one of the closed on-ladder canonical values.
		if p.Audience.Ceiling != "" && !onLadderCeilings[p.Audience.Ceiling] {
			t.Fatalf("off-ladder ceiling survived: input %q -> result %q", ceiling, p.Audience.Ceiling)
		}
		// And it must equal what NormalizeRating yields for the raw input — the one
		// canonicalizer the boundary trusts (never a raw pass-through).
		if want := schedule.NormalizeRating(ceiling); p.Audience.Ceiling != want {
			t.Fatalf("ceiling not canonicalized via NormalizeRating: input %q -> result %q, want %q", ceiling, p.Audience.Ceiling, want)
		}

		// Unrated grounding: only the two explicit modes survive; anything else
		// (including "") drops to UnratedDefault.
		switch p.Audience.Unrated {
		case schedule.UnratedDefault, schedule.UnratedExclude, schedule.UnratedAllow:
		default:
			t.Fatalf("unknown unrated policy survived: input %q -> result %q", unrated, p.Audience.Unrated)
		}

		// Ordering grounding: an unknown ordering mode is dropped to OrderInherit.
		switch p.Ordering {
		case schedule.OrderInherit, schedule.OrderSequential, schedule.OrderShuffle, schedule.OrderSyndication:
		default:
			t.Fatalf("unknown ordering survived: input %q -> result %q", ordering, p.Ordering)
		}
		// If the raw ordering was not an exactly-known mode, the result must have
		// been dropped to inherit (no fuzzy/case-insensitive acceptance). Note
		// OrderInherit is "" — a raw "" is already the dropped state.
		switch schedule.OrderingMode(ordering) {
		case schedule.OrderSequential, schedule.OrderShuffle, schedule.OrderSyndication:
			// known non-empty mode: survives as itself
			if p.Ordering != schedule.OrderingMode(ordering) {
				t.Fatalf("known ordering %q did not survive: got %q", ordering, p.Ordering)
			}
		default:
			// unknown or empty: must be dropped to inherit
			if p.Ordering != schedule.OrderInherit {
				t.Fatalf("off-enum ordering %q leaked as %q instead of inherit", ordering, p.Ordering)
			}
		}

		// Seasonal grounding: an unknown seasonal mode is dropped to SeasonalDefault.
		switch p.Seasonal.Mode {
		case schedule.SeasonalDefault, schedule.SeasonalOff, schedule.SeasonalAuto, schedule.SeasonalExclusive:
		default:
			t.Fatalf("unknown seasonal mode survived: input %q -> result %q", seasonalMode, p.Seasonal.Mode)
		}
		// A dropped seasonal mode must also drop its holidays — holidays only ride
		// with a known mode (groundPolicy sets both together or neither).
		if p.Seasonal.Mode == schedule.SeasonalDefault && len(p.Seasonal.Holidays) != 0 {
			t.Fatalf("holidays survived without a known seasonal mode: %+v", p.Seasonal.Holidays)
		}
	})
}
