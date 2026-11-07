package schedule

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/holidayvocab"
)

// The editor offers the holidays the household's country keeps (#2030): a US household is not
// offered Canada Day, which would never be in season for it, and Britain gets Bonfire Night.
func TestVocabulary_HolidaysAreTheHouseholdCountrys(t *testing.T) {
	holidaysIn := func(country string) []string {
		var out []string
		for _, w := range BuildVocabulary(Calendar{Country: country}).When {
			if id, ok := strings.CutPrefix(w.Token, "holiday:"); ok {
				out = append(out, id)
			}
		}
		return out
	}
	us, gb := holidaysIn(""), holidaysIn("GB")
	if !slices.Contains(us, "independence-day") || slices.Contains(us, "canada-day") || slices.Contains(us, "bonfire-night") {
		t.Errorf("US holidays = %v, want the Fourth of July and neither Canada Day nor Bonfire Night", us)
	}
	if !slices.Contains(gb, "bonfire-night") || slices.Contains(gb, "thanksgiving") || slices.Contains(gb, "independence-day") {
		t.Errorf("GB holidays = %v, want Bonfire Night and neither Thanksgiving nor the Fourth of July", gb)
	}
}

// The vocabulary is only trustworthy if every token it ships lowers, through the SAME Lower*
// functions the write path uses, to exactly the value it carries. Otherwise the editor would
// render a picker whose preview disagrees with what the BE stores — the very drift the
// vocabulary endpoint exists to kill.
func TestVocabulary_TokensLowerToTheirCarriedValues(t *testing.T) {
	holidays := holidayvocab.Default()
	v := BuildVocabulary(Calendar{})

	if want := len(whenVocabTokens) + len(holidays.Calendar("US")); len(v.When) != want {
		t.Fatalf("WHEN vocab dropped a token: got %d, want %d", len(v.When), want)
	}
	for _, w := range v.When {
		pred, prio, ok := LowerWhen(w.Token, holidays)
		if !ok {
			t.Errorf("WHEN token %q is in the vocabulary but does not lower", w.Token)
			continue
		}
		if !reflect.DeepEqual(pred, w.Predicate) || prio != w.Priority {
			t.Errorf("WHEN token %q: vocab carries %+v/p%d but lowers to %+v/p%d", w.Token, w.Predicate, w.Priority, pred, prio)
		}
	}

	if len(v.How) != len(howVocabTokens) {
		t.Fatalf("HOW vocab dropped a token: got %d, want %d", len(v.How), len(howVocabTokens))
	}
	for _, h := range v.How {
		ord, win, ok := LowerHow(h.Token)
		if !ok {
			t.Errorf("HOW token %q is in the vocabulary but does not lower", h.Token)
			continue
		}
		if !reflect.DeepEqual(ord, h.Ordering) || (win == WindowFull) != h.WindowFull {
			t.Errorf("HOW token %q: vocab carries %+v/full=%v but lowers to %+v/win=%v", h.Token, h.Ordering, h.WindowFull, ord, win)
		}
	}

	// The static WHAT set (all/kids/family/holiday-matched) — parametric series:/genre:/era:
	// are excluded by design (composed from the lineup client-side).
	if len(v.What) != len(whatVocabTokens) {
		t.Fatalf("WHAT vocab dropped a token: got %d, want %d", len(v.What), len(whatVocabTokens))
	}
	for _, w := range v.What {
		// kids/family carry a scope; all/holiday-matched carry none (nil) — assert the scope
		// matches what LowerWhat produces, so the editor narrows exactly as the BE will.
		scope, _, _ := LowerWhat(w.Token)
		if (scope == nil) != (w.Scope == nil) {
			t.Errorf("WHAT token %q: vocab scope-nil=%v disagrees with LowerWhat scope-nil=%v", w.Token, w.Scope == nil, scope == nil)
		}
	}
}
