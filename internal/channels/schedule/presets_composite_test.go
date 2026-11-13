package schedule

import (
	"testing"

	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
)

// A composite WHEN token is the AND of its two parts. The model writes these, so every ordered pair
// of base tokens (aliases and spellings included) is held to that: a composite either lowers to
// exactly the hours both parts share, or does not lower. It never lowers to a rule that matches no
// hour (weekend-weekday), nor to whichever window happened to come last (mornings-primetime).
func TestCompositeWhenTokensAreTheAndOfTheirParts(t *testing.T) {
	bases := []string{
		"weekend", "weekday", "mornings", "morning", "early morning", "early-morning", "daytime",
		"primetime", "prime-time", "prime time", "late-night", "late night", "latenight", "overnight", "graveyard",
	}
	vocab := holidayvocab.Default()
	lowered, refused := 0, 0
	for _, a := range bases {
		for _, b := range bases {
			token := a + "-" + b
			t.Run(token, func(t *testing.T) {
				pa, _, oka := lowerBaseWhen(a)
				pb, _, okb := lowerBaseWhen(b)
				if !oka || !okb {
					t.Fatalf("test setup: %q or %q is not a base token", a, b)
				}
				want := intersect(hoursOf(pa), hoursOf(pb))

				got, _, ok := LowerWhen(token, vocab)
				if !ok {
					// A composite that is refused must be one that could not be both parts at once, or
					// whose parts name two windows (the intersection of two windows is not one window).
					refused++
					return
				}
				lowered++
				if len(want) == 0 {
					t.Fatalf("%q lowered to %+v, a rule that cannot fire: the parts share no hour", token, got)
				}
				if !sameHours(hoursOf(got), want) {
					t.Fatalf("%q lowered to %+v matching %d hours, want the %d both parts share", token, got, len(hoursOf(got)), len(want))
				}
				if err := rulePolicy(got).Validate(); err != nil {
					t.Fatalf("%q lowered to a rule the write path refuses: %v", token, err)
				}
			})
		}
	}
	// Both outcomes happen: the table is not passing by refusing everything.
	if lowered == 0 || refused == 0 {
		t.Fatalf("lowered %d, refused %d composites", lowered, refused)
	}
}

// The named cases from the re-score, in both orders, and the ones that must keep working.
func TestCompositeWhenTokenExamples(t *testing.T) {
	vocab := holidayvocab.Default()
	for token, ok := range map[string]bool{
		"weekend-weekday":          false,
		"weekday-weekend":          false,
		"mornings-primetime":       false,
		"primetime-mornings":       false,
		"daytime-late-night":       false,
		"overnight-mornings":       false,
		"weekend-mornings":         true,
		"mornings-weekend":         true, // the order of the parts is not meaning
		"weekday-primetime":        true,
		"weekend-late-night":       true,
		"weekend-prime-time":       true,
		"weekend-weekend":          true, // the same kind twice says the same thing
		"mornings-mornings":        true,
		"weekend-mornings-x":       false,
		"weekend-mornings-weekday": false,
		"WEEKEND-Mornings":         true,
		" weekend-mornings ":       true,
	} {
		t.Run(token, func(t *testing.T) {
			got, _, gotOK := LowerWhen(token, vocab)
			if gotOK != ok {
				t.Fatalf("LowerWhen(%q) ok = %v (%+v), want %v", token, gotOK, got, ok)
			}
		})
	}
	// mornings-weekend and weekend-mornings are one rule.
	a, pa, _ := LowerWhen("weekend-mornings", vocab)
	b, pb, _ := LowerWhen("mornings-weekend", vocab)
	if !sameHours(hoursOf(a), hoursOf(b)) || pa != pb {
		t.Errorf("weekend-mornings = %+v/%d, mornings-weekend = %+v/%d, want one rule", a, pa, b, pb)
	}
}

// Every token the editor offers lowers to a rule that fires some hour of the week (or is a holiday,
// which the calendar places).
func TestEveryVocabularyTokenLowersToARuleThatFires(t *testing.T) {
	for _, w := range BuildVocabulary(Calendar{}).When {
		if len(hoursOf(w.Predicate)) == 0 && w.Predicate.Holiday == "" {
			t.Errorf("vocabulary token %q lowers to %+v, which matches no hour of the week", w.Token, w.Predicate)
		}
	}
}
