package schedule

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/holidayvocab"
)

// The vocabulary is only trustworthy if every token it ships lowers, through the SAME Lower*
// functions the write path uses, to exactly the value it carries. Otherwise the editor would
// render a picker whose preview disagrees with what the BE stores — the very drift the
// vocabulary endpoint exists to kill.
func TestVocabulary_TokensLowerToTheirCarriedValues(t *testing.T) {
	v := BuildVocabulary()

	if len(v.When) != len(whenVocabTokens)+len(holidayvocab.Definitions()) {
		t.Fatalf("WHEN vocab dropped a token: got %d, want %d", len(v.When), len(whenVocabTokens)+len(holidayvocab.Definitions()))
	}
	for _, w := range v.When {
		pred, prio, ok := LowerWhen(w.Token)
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

func TestBuiltinCalendarCoversEveryOwnedHolidayIdentity(t *testing.T) {
	definitions := holidayvocab.Definitions()
	if len(builtinCalendar) != len(definitions) {
		t.Fatalf("calendar has %d windows for %d owned holiday ids", len(builtinCalendar), len(definitions))
	}
	for _, definition := range definitions {
		if !knownHoliday(definition.ID) {
			t.Fatalf("owned holiday %q has no scheduler calendar window", definition.ID)
		}
	}
}
