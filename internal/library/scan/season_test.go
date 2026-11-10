package scan

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/localart"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A series' season posters reach its library art, each by its season, beside its own poster.
func TestASeriesListsItsSeasonPosters(t *testing.T) {
	h := newHarness(t)
	h.im.Art = localart.New()
	h.file("Ember Coast/Season 01/Ember Coast S01E01.mkv", 10)
	show := filepath.Join(h.root, "Ember Coast")
	for _, name := range []string{"poster.jpg", "season01-poster.jpg", "season-specials-poster.jpg"} {
		if err := os.WriteFile(filepath.Join(show, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h.run()

	row, ok := ProjectLibraryItem(h.item(seriesOrigin(h.root, "Ember Coast")))
	if !ok {
		t.Fatal("the series is not listed")
	}
	want := []librarystore.LibraryArt{
		{Kind: mp.KindPoster, Ref: filepath.Join(show, "poster.jpg")},
		{Kind: mp.KindSeasonPoster, Season: 0, Ref: filepath.Join(show, "season-specials-poster.jpg")},
		{Kind: mp.KindSeasonPoster, Season: 1, Ref: filepath.Join(show, "season01-poster.jpg")},
	}
	if !slices.Equal(row.Art, want) {
		t.Errorf("art = %+v, want %+v", row.Art, want)
	}
}
