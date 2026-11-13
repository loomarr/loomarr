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

// A show's yearless seasons and its dated ones are two shows' only when the dated start again at
// season 1 past the yearless; a missing season, or the show's specials, keep them one.
func TestSeasonsApartOnlyOnARestart(t *testing.T) {
	for _, c := range []struct {
		yearless, dated []int
		apart           bool
	}{
		{[]int{1}, []int{2}, false},
		{[]int{1}, []int{3}, false},
		{[]int{1, 2}, []int{4}, false},
		{[]int{0, 1}, []int{2}, false},
		{[]int{0}, []int{3, 4}, false},
		{[]int{2}, []int{1}, false},
		{[]int{6}, []int{1}, true},
		{[]int{5, 6}, []int{1, 2}, true},
		{[]int{6}, []int{0, 1}, true},
	} {
		if got := seasonsApart(c.yearless, c.dated); got != c.apart {
			t.Errorf("seasonsApart(%v, %v) = %v, want %v", c.yearless, c.dated, got, c.apart)
		}
	}
}

// A folder numbers its videos by season and episode run together only when its numbers span two
// seasons' hundreds or more, each low in its hundred; a show counted through its run does not.
func TestCompositeNumbersSpanSeasons(t *testing.T) {
	for _, c := range []struct {
		names []string
		want  bool
	}{
		{[]string{"101 Pilot.mkv", "102 The Well.mkv", "201 Return.mkv"}, true},
		{[]string{"101 Pilot.mkv", "1210 Late Year.mkv"}, true},
		{[]string{"101 Pilot.mkv", "102 The Well.mkv"}, false},                        // one hundred reads either way
		{[]string{"99 Late.mkv", "100 Round.mkv", "101 On.mkv", "201 On.mkv"}, false}, // a run past a hundred
		{[]string{"101 Pilot.mkv", "150 Mid.mkv", "201 Return.mkv"}, false},           // 150 is no season's episode
		{[]string{"1984 Signals.mkv", "2001 Signals.mkv"}, false},                     // years
		{[]string{"101 Pilot.mkv", "201 Return.mkv", "Bonus.mkv"}, true},              // a name with no number says nothing
		{[]string{"101 Pilot.mkv", "201 Return.mkv", "07 Odd.mkv"}, false},            // a short number does
	} {
		if got := compositeNumbers(c.names); got != c.want {
			t.Errorf("compositeNumbers(%v) = %v, want %v", c.names, got, c.want)
		}
	}
}

// A season's word misspelled by a letter still names a season; words of their own do not.
func TestSeasonInReadsAMisspelledSeason(t *testing.T) {
	for name, want := range map[string]int{
		"Sesaon 3": 3, "Seson 2": 2, "Seasn 1": 1, "Seasoon 4": 4, "Seasin 5": 5, "SEASNO 6": 6,
	} {
		if got, ok := seasonIn(name); !ok || got != want {
			t.Errorf("seasonIn(%q) = %d, %v; want %d", name, got, ok, want)
		}
	}
	for _, name := range []string{"Reason 3", "Mason 2", "Seasonal 1", "Sason Bay 2", "Sea 1", "Sesaon"} {
		if got, ok := seasonIn(name); ok {
			t.Errorf("seasonIn(%q) = %d, true; want no season", name, got)
		}
	}
}
