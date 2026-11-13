package suggest

import (
	"testing"

	"github.com/loomarr/loomarr/internal/proposals"
)

// A theme typed in title case is a theme; a label with a real proper noun or brand is still the
// name of a set whose membership grounding must prove (#3363).
func TestBareProperNameNamesSetTellsThemesFromNamedSets(t *testing.T) {
	for _, tc := range []struct {
		label    string
		namedSet bool
	}{
		{"Holiday Films", false},
		{"Christmas Movies", false},
		{"Space Westerns", false},
		{"Saturday Morning Cartoons", false},
		{"Halloween Specials", false},
		{"Cozy Mystery Nights", false},
		{"Late Night Comedies", false},
		{"Family Movie Night", false},
		{"Studio Ghibli", true},
		{"Must See TV", true},
		{"Classic Mudpuddles", true},
		{"Old School", true},
		{"The Muppets", true},
	} {
		if got := bareProperNameNamesSet(tc.label); got != tc.namedSet {
			t.Errorf("bareProperNameNamesSet(%q) = %t, want %t", tc.label, got, tc.namedSet)
		}
	}
}

// The prod failure: "Holiday Films" reached grounding as a named set and every candidate was
// dropped for unproven membership. As a theme it needs no membership evidence.
func TestHolidayFilmsNeedsNoMembershipEvidence(t *testing.T) {
	for _, description := range []string{"Holiday Films", "Saturday Morning Cartoons"} {
		if requiresMembershipEvidence(Intent{Intent: proposals.Intent{Description: description}}) {
			t.Errorf("%q demands membership evidence; want a fuzzy theme", description)
		}
	}
	if !requiresMembershipEvidence(Intent{Intent: proposals.Intent{Description: "Studio Ghibli"}}) {
		t.Error(`"Studio Ghibli" no longer demands membership evidence`)
	}
}
