package demolibrary_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/demolibrary"
)

// The invented catalogue is big enough to build channels from: about 40 series and 30 films
// (#1587), each with episodes, every one keyed outside the range real titles use.
func TestCatalogueIsChannelSized(t *testing.T) {
	lib := demolibrary.Invented()
	films, shows := 0, 0
	for _, ti := range lib.Titles {
		if ti.ProviderID() < demolibrary.ProviderIDBase {
			t.Errorf("%q keyed on %d, inside the range real titles use", ti.Name, ti.ProviderID())
		}
		switch ti.Kind {
		case demolibrary.Movie:
			films++
		case demolibrary.Series:
			shows++
			if len(lib.EpisodesOf(ti)) == 0 {
				t.Errorf("series %q has no episodes", ti.Name)
			}
		}
	}
	if shows < 40 || films < 30 {
		t.Fatalf("catalogue = %d series, %d films; #1587 asks for ~40 and ~30", shows, films)
	}
}
