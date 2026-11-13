package scan

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Show titles: a show named before a dash in its episodes' names, "Kettle Bay - Low Water", and a
// show folder's name read as words to match a file's against.

// dashedShows reads folderNotes.Dashed for a folder, rel to the root as the shelf predicates read
// it, from its videos' names. Only a catch-all's or a root's top is read: in a show's own folder
// its name is the folder's (namedSeries), and among films a franchise's films are named so.
func dashedShows(rel string, names []string) map[string]string {
	if rel != "." && !shelfAt(rel) || amongFilms(rel) {
		return nil
	}
	count, spelled := map[string]int{}, map[string]string{}
	for _, name := range names {
		show, ok := dashedShow(name)
		if !ok {
			continue
		}
		key := normName(show)
		count[key]++
		spelled[key] = show
	}
	var out map[string]string
	for key, n := range count {
		if n >= 2 {
			if out == nil {
				out = map[string]string{}
			}
			out[key] = spelled[key]
		}
	}
	return out
}

// dashedShow is the show a name gives before a spaced dash, with a title after it and no year,
// episode's mark or number: "Hob Duck - Big Pond" is "Hob Duck".
func dashedShow(name string) (string, bool) {
	s := plainStem(name)
	at := showTitleDash.FindStringIndex(s)
	if at == nil || episodeMark.MatchString(s) || strings.ContainsFunc(s, unicode.IsDigit) || leadingTags.MatchString(s) {
		return "", false
	}
	show, title := strings.TrimSpace(s[:at[0]]), strings.TrimSpace(s[at[1]:])
	if show == "" || strings.IndexFunc(title, unicode.IsLetter) < 0 {
		return "", false
	}
	return show, true
}

// dashedOf is the show a file loose among a catch-all's videos is one of by folderNotes.Dashed,
// "" for none.
func dashedOf(file File) string {
	show, ok := dashedShow(path.Base(file.Rel))
	if !ok || file.Notes.Dashed == nil {
		return ""
	}
	return file.Notes.Dashed[normName(show)]
}

// showTitled says whether a file's name is its folder's show, a dash and a title: "Busy Bees -
// Christmas Buzz (2015)" in "Busy Bees/".
func showTitled(name, folder string) bool {
	own, s := showWordsOf(folder), plainStem(name)
	at := showTitleDash.FindStringIndex(s)
	return len(own) > 0 && at != nil && slices.Equal(strings.Fields(normName(s[:at[0]])), own)
}

// showTitleDash matches the dash between a show's name and its episode's title: "Pebble Pals -
// Rainy Day", "Pebble_Pals_-_Rainy_Day".
var showTitleDash = regexp.MustCompile(`[ ._]-[ ._]`)

// showWordsOf is the words of the show a folder's name gives: "Pebble Pals Season 1" and "Sunny
// Hollow Volume 1" are "pebble pals" and "sunny hollow".
func showWordsOf(folder string) []string {
	name := showFolderName(folder)
	if m := namedSeason.FindStringSubmatch(name); m != nil {
		name = m[1]
	}
	words := strings.Fields(normName(name))
	for i := 1; i < len(words); i++ {
		if holdingOnly(words[i:]) {
			return words[:i]
		}
	}
	return words
}

// leadingPlace matches a name that starts with its place and a separator, then words: "01 - The
// Deep", "2. Low Water", "03_Tidewater".
var leadingPlace = regexp.MustCompile(`^(\d{1,3})(?:[ ._]*[-.][ ._]*|[ _]+)\pL`)
