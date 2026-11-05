package settings

import (
	"slices"
	"testing"
)

// Kinds is the wire's closed set: every declared setting's kind is in it, and each one has a
// typed handle, so a kind cannot reach the spec without the code that reads it.
func TestKindsCoverTheRegistry(t *testing.T) {
	kinds := Kinds()
	for _, s := range NewRegistry().All() {
		if !slices.Contains(kinds, s.Kind) {
			t.Errorf("%s: kind %q is missing from Kinds()", s.Key, s.Kind)
		}
	}
	for _, k := range kinds {
		if _, err := handleType(k); err != nil {
			t.Errorf("Kinds() lists %q, which has no typed handle: %v", k, err)
		}
	}
}

// Presentations is the wire's closed set of editors: every declared presentation is in it, and
// the default (absent on the wire) is not.
func TestPresentationsCoverTheRegistry(t *testing.T) {
	presentations := Presentations()
	if slices.Contains(presentations, PresentationDefault) {
		t.Error("Presentations() lists the default, which the wire omits")
	}
	for _, s := range NewRegistry().All() {
		if s.Presentation != PresentationDefault && !slices.Contains(presentations, s.Presentation) {
			t.Errorf("%s: presentation %q is missing from Presentations()", s.Key, s.Presentation)
		}
	}
}
