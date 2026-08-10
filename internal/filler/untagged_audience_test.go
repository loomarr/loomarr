package filler

import "testing"

// The untagged-audience cliff (§10 V51f) and the guardrail that bounds the fix.
//
// ⚠ **The cliff:** `filler.ai_tagging` defaulted off for most of this project's life, so a real
// catalog is full of clips whose audience is `""`. `filterAudience` admits `c.Audience == aud ||
// c.Audience == General`, and `""` is neither — so the moment an operator picked an Audience,
// every rung emptied and the channel fell to its bumper card. The meter then said "nothing in the
// catalog fits", which reads as a catalog problem rather than a tagging one.

func untaggedCatalog() []Clip {
	return []Clip{
		commercial("grounded-kids", 1992, Kids),
		commercial("grounded-general", 1992, General),
		{Hash: "untagged", Path: "untagged", Kind: Commercial, Era: 1992, DurationMs: 30_000},
	}
}

// ⚠ **The safety asymmetry, asserted per audience so weakening it fails loudly.** `kids` and
// `family` never take an unclassified clip; `general` and `late_night` do. Family is in the
// forbidden half because family channels are watched by children.
func TestLadder_UngroundedAudienceIsAdmittedOnlyWhereItIsSafe(t *testing.T) {
	for _, tc := range []struct {
		aud       Audience
		wantAdmit bool
	}{
		{Kids, false},
		{Family, false},
		{General, true},
		{LateNight, true},
	} {
		t.Run(string(tc.aud), func(t *testing.T) {
			w := Window{Era: Year(1992), Audience: tc.aud, GapMs: 120_000, PodMax: 4}
			pools := candidatePools(untaggedCatalog(), w, Policy{})

			bottom := map[string]bool{}
			for _, c := range pools[len(pools)-1].clips {
				bottom[c.ID()] = true
			}
			if got := bottom["untagged"]; got != tc.wantAdmit {
				t.Errorf("%s channel: untagged clip admitted = %v, want %v", tc.aud, got, tc.wantAdmit)
			}

			// ⚠ Whatever the audience, an unclassified clip must NEVER reach an era rung: those
			// claim the clip is right for this channel, which is exactly what is not known.
			for _, p := range pools[:2] {
				for _, c := range p.clips {
					if c.ID() == "untagged" {
						t.Errorf("%s channel: untagged clip reached the %s rung", tc.aud, p.level)
					}
				}
			}
		})
	}
}

// A grounded match always beats an unclassified one, so admitting untagged clips can never
// downgrade a channel that has real matches — it only gives one that has none a floor.
func TestLadder_UngroundedNeverOutranksAGroundedMatch(t *testing.T) {
	w := Window{Era: Year(1992), Audience: LateNight, GapMs: 120_000, PodMax: 4}
	cat := []Clip{
		commercial("grounded", 1992, LateNight),
		{Hash: "untagged", Path: "untagged", Kind: Commercial, Era: 1992, DurationMs: 30_000},
	}

	pod := Assemble(cat, w, Policy{}, nil)
	if pod.MatchLevel != MatchExact {
		t.Fatalf("match level = %s, want exact — a grounded clip was available", pod.MatchLevel)
	}
	for _, e := range pod.Entries {
		if e.Path == "untagged" {
			t.Error("an unclassified clip displaced a grounded exact-era match")
		}
	}
}

// ⚠ The cliff itself: a late-night channel whose catalog is entirely untagged used to fall to the
// bumper card. It now fills. This is the test that would have caught the original bug.
func TestLadder_AnEntirelyUntaggedCatalogStillFillsABreak(t *testing.T) {
	cat := []Clip{
		{Hash: "u1", Path: "u1", Kind: Commercial, Era: 1992, DurationMs: 30_000},
		{Hash: "u2", Path: "u2", Kind: Commercial, Era: 1992, DurationMs: 30_000},
	}
	w := Window{Era: Year(1992), Audience: LateNight, GapMs: 120_000, PodMax: 4}

	pod := Assemble(cat, w, Policy{}, nil)
	if pod.MatchLevel == MatchBumperCard {
		t.Fatal("an untagged catalog fell to the bumper card — the V51f cliff is back")
	}
	if pod.MatchLevel != MatchAudience {
		t.Errorf("match level = %s, want audience — untagged clips belong on the bottom rung", pod.MatchLevel)
	}
}

// ...and the kids channel in the same state correctly does NOT fill. Falling to the bumper card
// here is the designed outcome: a visible, fixable state rather than unclassified adverts in
// front of children.
func TestLadder_AKidsChannelWithAnUntaggedCatalogFallsToTheCard(t *testing.T) {
	cat := []Clip{
		{Hash: "u1", Path: "u1", Kind: Commercial, Era: 1992, DurationMs: 30_000},
	}
	w := Window{Era: Year(1992), Audience: Kids, GapMs: 120_000, PodMax: 4}

	pod := Assemble(cat, w, Policy{}, nil)
	if pod.MatchLevel != MatchBumperCard {
		t.Fatalf("kids channel reached %s on an untagged catalog — the guardrail is gone", pod.MatchLevel)
	}
}
