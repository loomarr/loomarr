package suggest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/proposals"
)

// No shipped starter template names a title, so none may yield an explicit title to look up
// (#1940). "like I watched as a kid — bright" and "keep it PG-13" are descriptive prose; treating
// them as titles sent them to the Catalog as required lookups, and one failed search failed the
// whole job.
func TestStarterTemplateDescriptionsNameNoExplicitTitles(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "packages", "core", "src", "templates", "templates.json"))
	if err != nil {
		t.Fatal(err)
	}
	var shipped []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &shipped); err != nil {
		t.Fatal(err)
	}
	if len(shipped) == 0 {
		t.Fatal("templates.json ships no starter templates")
	}
	for _, tpl := range shipped {
		intent := Intent{Intent: proposals.Intent{Description: tpl.Description}}
		if got := directIncludedTitles(intent); len(got) != 0 {
			t.Errorf("template %q (%q) yields explicit titles %q, want none", tpl.ID, tpl.Description, got)
		}
	}
}

// Prose after a cue is dropped, while a title after the same cue survives, including one that
// starts with a pronoun.
func TestExplicitTitlesDropDescriptiveClauses(t *testing.T) {
	for _, tc := range []struct {
		description string
		want        []string
	}{
		{"Cartoons like I watched as a kid", nil},
		{"Sitcoms like we used to watch", nil},
		{"Cartoons like they made in the 90s — bright, silly", nil},
		{"Back-to-back action, keep it PG-13", nil},
		{"Adventure movies like Indiana Jones and The Goonies", []string{"Indiana Jones", "The Goonies"}},
		{"Classic sitcoms like I Love Lucy", []string{"I Love Lucy"}},
		{"Family movies like We Bought a Zoo", []string{"We Bought a Zoo"}},
		{"Horror like It Follows — slow and creepy", []string{"It Follows"}},
		{"Include Carrie and Misery", []string{"Carrie", "Misery"}},
	} {
		got := directIncludedTitles(Intent{Intent: proposals.Intent{Description: tc.description}})
		if !slices.Equal(got, tc.want) {
			t.Errorf("directIncludedTitles(%q) = %q, want %q", tc.description, got, tc.want)
		}
	}
}
