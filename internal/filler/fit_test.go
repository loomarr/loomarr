package filler

import "testing"

// Per-clip fit (V35 item 1.7).
//
// ⚠ The headline test is TestFitFor_AgreesWithTheLadder, which cross-checks against
// `candidatePools` rather than re-asserting the predicates FitFor calls. Asserting them back
// would prove only that I wrote the same conditions twice; the property that matters is that a
// fit note cannot claim a rung the assembler disagrees with.

func commercial(path string, era int, aud Audience) Clip {
	// ⚠ Hash AND Path. Identity is the hash since V38c, and pin/exclude lists key on `ID()` — a
	// literal that sets only Path leaves the id empty, so every pin and exclusion silently misses.
	return Clip{Hash: path, Path: path, Kind: Commercial, Era: era, Audience: aud, DurationMs: 30_000}
}

// ⚠ THE load-bearing property: for every clip and every rung, FitFor's answer must match the
// pool the clip actually lands in. `channelpreview.go` records the v2 mock's own meter
// recomputing its buckets inline and disagreeing with reality — this is the guard against
// re-introducing that one layer down.
func TestFitFor_AgreesWithTheLadder(t *testing.T) {
	catalog := []Clip{
		commercial("exact", 1992, Kids),
		commercial("same-decade", 1995, Kids),
		commercial("other-era", 1975, Kids),
		commercial("general-aud", 1992, General),
		commercial("wrong-aud", 1992, LateNight),
		{Hash: "bumper", Path: "bumper", Kind: Bumper, Era: 1992, Audience: General, DurationMs: 5_000},
	}
	// ⚠ The varied dimension used to be `EraStrict` (a field no operator could set). It is now the
	// era RANGE, which is the thing that actually varies in the wild — and a single year, a span
	// and "any" put these clips in genuinely different rungs, so this stays a real cross-check
	// rather than the same assertion run twice.
	for _, era := range []EraRange{Year(1992), {From: 1990, To: 1999}, {From: 1975, To: 1992}, {}} {
		policy := Policy{}
		w := Window{Era: era, Audience: Kids, GapMs: 120_000, PodMax: 4}
		pools := candidatePools(catalog, w, policy)

		// The rung each clip really lands in: the FIRST (tightest) pool containing it, which is
		// exactly how fillCommercials chooses.
		landsIn := map[string]MatchLevel{}
		for _, p := range pools {
			for _, c := range p.clips {
				if _, seen := landsIn[c.ID()]; !seen {
					landsIn[c.ID()] = p.level
				}
			}
		}

		for _, c := range catalog {
			fit := FitFor(c, w, policy)
			want, isCandidate := landsIn[c.ID()]
			switch {
			case c.Kind != Commercial:
				// Bumpers never run the ladder; FitFor says so with no reason.
				if fit.Level != MatchBumperCard || fit.Reason != "" {
					t.Errorf("era=%v %s: bumper reported %s, want bumper_card with no reason",
						era, c.ID(), fit)
				}
			case isCandidate:
				if fit.Level != want {
					t.Errorf("era=%v %s: FitFor says %s, the ladder puts it in %s",
						era, c.ID(), fit.Level, want)
				}
				if fit.Reason != "" {
					t.Errorf("era=%v %s: a candidate carries reason %q", era, c.ID(), fit.Reason)
				}
			default:
				if fit.Level != MatchBumperCard {
					t.Errorf("era=%v %s: FitFor says %s, but no pool holds it", era, c.ID(), fit.Level)
				}
				// ⚠ A rejected clip must always say WHY. "Won't be picked" with no reason is
				// what sends an operator hunting through channel settings.
				if fit.Reason == "" {
					t.Errorf("era=%v %s: rejected with no reason", era, c.ID())
				}
			}
		}
	}
}

// ⚠ Excluded WINS over pinned, because Assemble seeds excluded ids into `used` before pin runs.
// A picker that showed "pinned" for a clip that never plays would be describing the store
// rather than the outcome.
func TestFitFor_ExcludedBeatsPinned(t *testing.T) {
	c := commercial("both", 1992, Kids)
	w := Window{Era: Year(1992), Audience: Kids, Pinned: []string{"both"}, Excluded: []string{"both"}}

	fit := FitFor(c, w, Policy{})

	if fit.Reason != FitExcluded {
		t.Errorf("reason = %q, want %q — exclude wins", fit.Reason, FitExcluded)
	}
	if !fit.Pinned || !fit.Excluded {
		t.Errorf("pinned=%v excluded=%v — both flags must be reported as stored", fit.Pinned, fit.Excluded)
	}
	if fit.Level != MatchBumperCard {
		t.Errorf("level = %s, want bumper_card — an excluded clip is not a candidate", fit.Level)
	}
}

// And the exclusion must really be what Assemble does, not just what FitFor claims.
func TestFitFor_ExcludedMatchesAssembly(t *testing.T) {
	catalog := []Clip{commercial("a", 1992, Kids), commercial("b", 1992, Kids)}
	w := Window{Era: Year(1992), Audience: Kids, GapMs: 120_000, PodMax: 4, Excluded: []string{"a"}, Seed: 1}

	pod := Assemble(catalog, w, Policy{}, nil)
	for _, e := range pod.Entries {
		if e.Path == "a" {
			t.Fatal("the excluded clip was assembled — FitFor and Assemble disagree")
		}
	}
	if FitFor(catalog[0], w, Policy{}).Reason != FitExcluded {
		t.Error("FitFor does not report the exclusion Assemble honours")
	}
}

// ⚠ **A PIN BYPASSES THE LADDER — that is what pinning is for.** A clip the audience filter
// would drop still plays when pinned, so the note must NOT report it as rejected.
//
// This test began life asserting the opposite ("a pin cannot rescue a wrong-audience clip"),
// which was plausible and wrong: it failed against real `Assemble` output on the first run. The
// wrong version would have told operators their explicit choice was being ignored at exactly
// the moment it was being honoured. Kept pointed at Assemble so it can only ever agree with
// what plays.
func TestFitFor_APinBypassesTheLadder(t *testing.T) {
	c := commercial("late", 1992, LateNight) // wrong audience for a kids channel
	w := Window{Era: Year(1992), Audience: Kids, Pinned: []string{"late"}, GapMs: 120_000, PodMax: 4, Seed: 1}

	fit := FitFor(c, w, Policy{})
	if fit.Reason != "" {
		t.Errorf("reason = %q, want none — a pin overrides the audience filter", fit.Reason)
	}
	if !fit.Pinned {
		t.Error("pinned flag not set")
	}

	// The claim is about ASSEMBLY, so assert it there rather than trusting the note.
	pod := Assemble([]Clip{c}, w, Policy{}, nil)
	var played bool
	for _, e := range pod.Entries {
		if e.Path == "late" {
			played = true
		}
	}
	if !played {
		t.Fatal("a pinned clip did not play — FitFor and Assemble disagree")
	}
}

// ⚠ But KIND and DURATION still gate a pin: `pickPinned` reads the kind-filtered catalog and
// checks durationEligible itself. So "a pin bypasses the ladder" is not "a pin bypasses
// everything", and the note must not over-promise.
func TestFitFor_APinStillObeysKindAndDuration(t *testing.T) {
	tooLong := Clip{Hash: "long", Path: "long", Kind: Commercial, Era: 1992, Audience: Kids, DurationMs: 600_000}
	w := Window{Era: Year(1992), Audience: Kids, Pinned: []string{"long"}, GapMs: 120_000, PodMax: 4, Seed: 1}
	policy := Policy{MaxClipMs: 60_000}

	if got := FitFor(tooLong, w, policy).Reason; got != FitDuration {
		t.Errorf("reason = %q, want %q — duration gates pins too", got, FitDuration)
	}

	pod := Assemble([]Clip{tooLong}, w, policy, nil)
	for _, e := range pod.Entries {
		if e.Path == "long" {
			t.Fatal("an over-long pin was assembled — the note's premise is wrong")
		}
	}
}

// Each rejection predicate reports itself, so the UI can name the setting to change.
func TestFitFor_NamesTheRejectingPredicate(t *testing.T) {
	base := Window{Era: Year(1992), Audience: Kids}

	for _, tc := range []struct {
		name   string
		clip   Clip
		window Window
		policy Policy
		want   FitReason
	}{
		{
			name:   "kind the channel does not use",
			clip:   Clip{Hash: "b", Path: "b", Kind: Bumper, Era: 1992, Audience: Kids, DurationMs: 5_000},
			window: Window{Era: Year(1992), Audience: Kids, Kinds: []string{"commercial"}},
			want:   FitKind,
		},
		{
			name:   "too long for the break",
			clip:   Clip{Hash: "long", Path: "long", Kind: Commercial, Era: 1992, Audience: Kids, DurationMs: 600_000},
			window: base,
			policy: Policy{MaxClipMs: 60_000},
			want:   FitDuration,
		},
		{
			name:   "below the quality floor",
			clip:   Clip{Hash: "grainy", Path: "grainy", Kind: Commercial, Era: 1992, Audience: Kids, DurationMs: 30_000, Quality: "240p"},
			window: base,
			policy: Policy{MinQualityHeight: 720},
			want:   FitQuality,
		},
		{
			name:   "not in the channel's categories",
			clip:   Clip{Hash: "car", Path: "car", Kind: Commercial, Era: 1992, Audience: Kids, DurationMs: 30_000, Category: "auto"},
			window: Window{Era: Year(1992), Audience: Kids, Categories: []string{"toys"}},
			want:   FitCategory,
		},
		{
			name:   "wrong audience",
			clip:   commercial("late", 1992, LateNight),
			window: base,
			want:   FitAudience,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FitFor(tc.clip, tc.window, tc.policy).Reason; got != tc.want {
				t.Errorf("reason = %q, want %q", got, tc.want)
			}
		})
	}
}

// ⚠ A general-audience clip fits ANY channel — the one asymmetry in filterAudience, and the
// reason a kids channel is not limited to clips tagged `kids`.
func TestFitFor_GeneralAudienceFitsAnyChannel(t *testing.T) {
	fit := FitFor(commercial("gen", 1992, General), Window{Era: Year(1992), Audience: Kids}, Policy{})

	if fit.Reason != "" || fit.Level != MatchExact {
		t.Errorf("got %s, want an exact match with no reason", fit)
	}
}

// ⚠ **This replaces `TestFitFor_StrictEraSkipsTheWidenedRung`, and the replacement is the point.**
// That test asserted the behaviour of `Policy.EraStrict` (retired-ok) — a field set in tests and NOWHERE else,
// so it proved a branch no operator could reach. Narrowing the RANGE is how a channel gets
// strictness now, and unlike the flag it moves a clip between rungs in a way the operator can see
// and undo. Same intent, reachable mechanism.
func TestFitFor_NarrowingTheRangeMovesAClipDownTheLadder(t *testing.T) {
	c := commercial("1995", 1995, Kids)

	for _, tc := range []struct {
		name string
		era  EraRange
		want MatchLevel
	}{
		{"inside the range", EraRange{From: 1990, To: 1999}, MatchExact},
		{"a decade either side", Year(1992), MatchWidened},
		// ⚠ 1975–1980 widens to 1965–1990, which still does not reach 1995 — so the clip falls to
		// the audience rung. Under the old decade-BUCKET rule there was no way to express this:
		// widening always snapped to one decade, so a range could never be too far away.
		{"outside even the widened range", EraRange{From: 1975, To: 1980}, MatchAudience},
		{"any era", EraRange{}, MatchExact},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := Window{Era: tc.era, Audience: Kids}
			if got := FitFor(c, w, Policy{}).Level; got != tc.want {
				t.Errorf("era %+v: level = %s, want %s", tc.era, got, tc.want)
			}
		})
	}
}
