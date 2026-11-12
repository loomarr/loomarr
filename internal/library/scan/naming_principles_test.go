package scan

import (
	"fmt"
	"strings"
	"testing"
)

// The naming principles are rules the scanner keeps whatever a library is called: each is read
// over every spelling of a few invented shows, so a rule that holds for one row and not its
// neighbours fails here. The naming table pins examples; these pin what the examples have in
// common.

// principleShows are the shows each principle is read over: a plain name, one with its year, and
// one with a number in it.
var principleShows = []string{"Thornbury", "Larkfield Junction (2008)", "Station 7"}

// principleRead runs files through the real pass and holds each to its want.
func principleRead(t *testing.T, files map[string]string) {
	t.Helper()
	got := namingOutcomes(t, namingRow{id: "principle", files: files})
	for rel, want := range files {
		if got[rel] != want {
			t.Errorf("%s = %q, want %q", rel, got[rel], want)
		}
	}
}

// showName is the name an outcome gives a show: its folder's, year and all.
func showName(show string) string {
	return strings.NewReplacer("(", "", ")", "").Replace(show)
}

// TestPrincipleMarkedEpisodeIsKept: an episode marked "S01E04", titled after a spaced dash, is its
// show's episode whatever its title says and however it is written. A release's sample joins its
// name ("….x264-grp.sample"); a household's title is set apart.
func TestPrincipleMarkedEpisodeIsKept(t *testing.T) {
	titles := []string{"Sample", "Trailer", "Extras", "Featurette", "Behind the Scenes", "Deleted Scenes", "Interview", "Proof"}
	for _, show := range principleShows {
		files := map[string]string{}
		for i, title := range titles {
			for j, spell := range []func(string) string{strings.TrimSpace, strings.ToLower, strings.ToUpper} {
				e := i*3 + j + 1
				name := spell(fmt.Sprintf("%s - S01E%02d - %s", show, e, title))
				files[fmt.Sprintf("TV/%s/Season 1/%s.mkv", show, name)] = fmt.Sprintf("episode %s S1E%d", showName(show), e)
			}
		}
		t.Run(strings.Fields(show)[0], func(t *testing.T) { principleRead(t, files) })
	}
}

// TestPrincipleSpecialsFolderIsTheShows: a folder in a show's own folder that holds its specials,
// named by the show or not, is that show's season 0, one show with its seasons.
func TestPrincipleSpecialsFolderIsTheShows(t *testing.T) {
	for _, show := range principleShows {
		bare := strings.TrimSpace(strings.Split(show, " (")[0])
		files := map[string]string{
			fmt.Sprintf("TV/%s/Season 1/%s S01E01.mkv", show, bare): fmt.Sprintf("episode %s S1E1", showName(show)),
		}
		for i, folder := range []string{"Specials", "%s Specials", "%s - Specials", "%s.Specials", "OVA", "OVAs", "%s OVAs"} {
			if strings.Contains(folder, "%s") {
				folder = fmt.Sprintf(folder, show)
			}
			files[fmt.Sprintf("TV/%s/%s/%s S00E%02d.mkv", show, folder, bare, i+1)] = fmt.Sprintf("episode %s S0E%d", showName(show), i+1)
		}
		t.Run(strings.Fields(show)[0], func(t *testing.T) { principleRead(t, files) })
	}
}

// TestPrincipleGroupingFolderIsTheShows: a folder in a show's own folder that groups its episodes,
// by volume, book, chapter, year, DVD or collection, named by the show or not, holds the show's
// episodes, as its season folders do: one show, never a film named by the folder.
func TestPrincipleGroupingFolderIsTheShows(t *testing.T) {
	folders := []string{"Volume 1", "Vol 2", "Vol.3", "%s Vol 4", "%s - Volume 5", "Book One", "Book 2", "Chapter 3",
		"Year 1", "DVD1", "DVD 2", "Collection 1"}
	for _, shelf := range []string{"TV", "Kids", "Anime"} {
		for _, show := range principleShows {
			bare := strings.TrimSpace(strings.Split(show, " (")[0])
			files := map[string]string{
				fmt.Sprintf("%s/%s/Season 1/%s S01E01.mkv", shelf, show, bare): fmt.Sprintf("episode %s S1E1", showName(show)),
			}
			for i, folder := range folders {
				if strings.Contains(folder, "%s") {
					folder = fmt.Sprintf(folder, bare)
				}
				files[fmt.Sprintf("%s/%s/%s/%s S02E%02d.mkv", shelf, show, folder, bare, i+1)] = fmt.Sprintf("episode %s S2E%d", showName(show), i+1)
				// An unmarked title there is the show's too, for the catalog to place.
				files[fmt.Sprintf("%s/%s/%s/%s - Long Night %d.mkv", shelf, show, folder, bare, i+1)] = fmt.Sprintf("episode %s S?E?", showName(show))
			}
			t.Run(shelf+"-"+strings.Fields(show)[0], func(t *testing.T) { principleRead(t, files) })
		}
	}
}

// TestPrincipleAnimeExtraIsSkipped: a release's opening, ending, menu, preview or trailer in its
// episode's place is an extra however it is written, and its episodes beside it are the show's.
func TestPrincipleAnimeExtraIsSkipped(t *testing.T) {
	extras := []string{"OP", "ED2", "NCOP", "NCED 2", "Creditless OP", "Clean ED", "Textless OP 1", "Menu 01", "Preview 03",
		"Picture Drama 01", "Trailer", "PV", "CM 2", "Teaser"}
	for _, show := range []string{"Mori Kaze", "Hoshi no Uta", "Kettle 86"} {
		files := map[string]string{
			fmt.Sprintf("Anime/[Grp] %s - 01 [1080p].mkv", show): fmt.Sprintf("episode %s S?E?", show),
			fmt.Sprintf("Anime/[Grp] %s - 02 [1080p].mkv", show): fmt.Sprintf("episode %s S?E?", show),
		}
		for _, extra := range extras {
			files[fmt.Sprintf("Anime/[Grp] %s - %s [1080p].mkv", show, extra)] = "skip"
		}
		t.Run(strings.Fields(show)[0], func(t *testing.T) { principleRead(t, files) })
	}
}

// TestPrincipleDayFirstRecording: a recorder's recording named by its day first is its show's
// episode of that day on a shows' shelf, and a day's own video anywhere else.
func TestPrincipleDayFirstRecording(t *testing.T) {
	for _, day := range []string{"2023-03-08", "20230308", "2023.03.08", "2023_03_08"} {
		files := map[string]string{
			fmt.Sprintf("Recordings/%s Ironfield.ts", day):                   "episode Ironfield 2023-03-08",
			fmt.Sprintf("Recordings/%s Ironmoor Lane - The Well.ts", day):    "episode Ironmoor Lane 2023-03-08",
			fmt.Sprintf("Recordings/%s_2100_Channel5_Ironmere Road.ts", day): "episode Ironmere Road 2023-03-08",
			fmt.Sprintf("Home Videos/%s Beach Day.mp4", day):                 "movie " + strings.ReplaceAll(day, "_", " ") + " Beach Day -",
		}
		t.Run(day, func(t *testing.T) { principleRead(t, files) })
	}
}
