package scan

import (
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Season folders: a folder that holds one season of a show ("Season 1", "S02D1", "Staffel 3",
// "Specials", "Kettle Bay Season 2", "Sesaon 3"), and the season it gives.

// seasonWords name a season in a folder's name, in the languages households name them in.
const seasonWords = `seasons|season|series|staffel|saison|temporada|stagione|sezon|seizoen|säsong|sasong|sæson|saeson|sesong|сезон|kausi|sezona|évad|sezóna`

// spelledNumbers are a season's number spelled out: "Season One", "Series Two".
var spelledNumbers = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
	"ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15,
	"sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19, "twenty": 20,
}

// seasonNotes is what may follow a season folder's number: anything after a dash or in brackets
// ("Season 1 - 2019", "Season 2 [1080p]", "Season 1-2"), or a release's words ("Season 1 1080p
// WEB-DL", "Season 01 Complete", "Season 1 Disc 2"). Any other word makes the name a show's:
// "Series 7 Club", "Season 25 Club".
const seasonNotes = `(?:[ ._]*[-\[(].*|[ ._]+(?:\d{3,4}p|complete|(?:19|20)\d{2}|web(?:-?dl|rip)?|blu-?ray|bdrip|brrip|hdtv|dvd(?:rip)?|x26[45]|h\.?26[45]|hevc|uhd|4k|remux|multi|disc|disk|dvd)(?:[ ._].*)?)?`

// seasonMarkNotes is what may follow a season's bare mark, "S02", in a folder's name: its disc,
// "S02D1", "S02 Disc 1"; the year it aired after a dash, "S01 - 2010"; and bracketed notes.
const seasonMarkNotes = `(?:[ ._-]*(?:d|disc|disk|dvd)[ ._-]*\d{1,2})?(?:[ ._]*-[ ._]*(?:19|20)\d{2})?(?:[ ._-]*[\[(][^\])]*[\])])*`

var (
	// seasonFolder matches a season folder's name and its number: "Season 1", "Season 01", "Series
	// One", "Staffel 1", "Säsong 1", with its notes; "S01" with only its disc, year or bracketed
	// notes (seasonMarkNotes).
	seasonFolder = regexp.MustCompile(`(?i)^(?:(?:` + seasonWords + `)[ ._-]*(\d{1,4}|[a-z]+)` + seasonNotes + `|s(\d{1,4})` + seasonMarkNotes + `)$`)
	// namedSeason matches a season folder that starts with its show's name: "Kettle Bay Season 1",
	// "KB Season 1", "Kettle Bay S02D1".
	namedSeason = regexp.MustCompile(`(?i)^(.+?)[ ._-]+(?:(?:` + seasonWords + `)[ ._-]*(\d{1,4}|[a-z]+)` + seasonNotes + `|s(\d{1,2})` + seasonMarkNotes + `)$`)
	// seasonDashEpisode matches a season's word and number, then its episode's, apart by dashes:
	// "Series 2 - 05 - Muddy Day", "Season 1 - 03".
	seasonDashEpisode = regexp.MustCompile(`(?i)(?:^|[^\pL0-9])(?:` + seasonWords + `)[ ._]*(\d{1,2})[ ._]*-[ ._]*(\d{1,3})(?:[ ._]*-|[ ._]*$)`)
	// cjkSeason matches a Japanese, Chinese or Korean season folder: "第1期", "第2季", "シーズン1",
	// "시즌 1".
	cjkSeason = regexp.MustCompile(`^(?:第\s*(\d{1,2})\s*[期季部]|(?:シーズン|시즌)\s*(\d{1,2}))$`)
	// ordinalSeason matches a season folder named by its place: "1st Season", "2nd Series".
	ordinalSeason = regexp.MustCompile(`(?i)^(\d{1,2})(?:st|nd|rd|th)[ ._-]+(?:season|series)(?:[ ._\-\[(].*)?$`)
)

// seasonIn is the season a season folder's own name gives, "Specials" 0; ok is false for a name
// that names no season. A name in brackets is read inside them: "(Season 1)".
func seasonIn(name string) (season int, ok bool) {
	if len(name) > 2 && (name[0] == '(' && name[len(name)-1] == ')' || name[0] == '[' && name[len(name)-1] == ']') {
		name = name[1 : len(name)-1]
	}
	// "Christmas_Specials" and "시즌.1" are spelled with their words apart as "Christmas Specials" is.
	name = strings.Join(strings.Fields(strings.NewReplacer("_", " ", ".", " ").Replace(name)), " ")
	// The year it aired first, "2019 - Season 6", as "S03 - 2012" gives it last.
	if m := yearThenSeason.FindStringSubmatch(name); m != nil {
		return seasonIn(m[1])
	}
	switch strings.ToLower(name) {
	case "specials", "special", "season specials", "season 00 specials", "extras specials", "ova", "ovas", "oad", "oads":
		return 0, true
	}
	// "Christmas Specials", "Holiday Specials": a show's specials by when they aired.
	if words := strings.Fields(strings.ToLower(name)); len(words) == 2 && words[1] == "specials" && specialsKinds[words[0]] {
		return 0, true
	}
	if m := ordinalSeason.FindStringSubmatch(name); m != nil {
		return atoi(m[1]), true
	}
	if m := cjkSeason.FindStringSubmatch(name); m != nil {
		return atoi(matched(m[1], m[2])), true
	}
	m := seasonFolder.FindStringSubmatch(name)
	if m == nil {
		// A season's word misspelled by a letter, "Sesaon 3", "Seson 2", "Seasn 1", with its number.
		if words := strings.Fields(name); len(words) == 2 && len(words[1]) <= 2 && digits(words[1]) && seasonTypo(words[0]) {
			return atoi(words[1]), true
		}
		return 0, false
	}
	if m[2] != "" {
		return atoi(m[2]), true
	}
	// A name written all in lower case writes its numeral so too: "season iv".
	if name == strings.ToLower(name) {
		if n, ok := romanNumbers[strings.ToUpper(m[1])]; ok {
			return n, true
		}
	}
	return seasonNumberWord(m[1])
}

// yearThenSeason matches a season's folder named by its year, a dash, then the season: "2019 -
// Season 6", "2020-S01".
var yearThenSeason = regexp.MustCompile(`^(?:19|20)\d{2} ?- ?(\S.*)$`)

// seasonTypo says whether a word is "season" misspelled by one letter: two swapped ("Sesaon"),
// one missing ("Seson"), one extra ("Seasoon") or one wrong ("Seasin"). It starts with the "s" a
// typist rarely misses, so "Reason" and "Mason" are words of their own.
func seasonTypo(word string) bool {
	w := strings.ToLower(word)
	return w != "" && w[0] == 's' && isASCIILetters(w) && oneSlip(w, "season")
}

// oneSlip says whether w is want typed with one slip: two letters swapped, one missing, one extra
// or one wrong. The same word is no slip.
func oneSlip(w, want string) bool {
	if w == want {
		return false
	}
	switch len(w) - len(want) {
	case 0:
		var diff []int
		for i := range len(w) {
			if w[i] != want[i] {
				diff = append(diff, i)
			}
		}
		return len(diff) == 1 ||
			len(diff) == 2 && diff[1] == diff[0]+1 && w[diff[0]] == want[diff[1]] && w[diff[1]] == want[diff[0]]
	case -1:
		return oneMissing(w, want)
	case 1:
		return oneMissing(want, w)
	}
	return false
}

// oneMissing says whether short is long with one letter left out.
func oneMissing(short, long string) bool {
	for i := range len(long) {
		if long[:i]+long[i+1:] == short {
			return true
		}
	}
	return false
}

// isASCIILetters says whether a word is ASCII letters alone.
func isASCIILetters(w string) bool {
	return !strings.ContainsFunc(w, func(r rune) bool { return r < 'a' || r > 'z' })
}

// specialsKinds are the words a show's specials folder is named by before "Specials".
var specialsKinds = map[string]bool{
	"christmas": true, "holiday": true, "xmas": true, "halloween": true, "easter": true, "tv": true, "bonus": true,
	"festive": true, "anniversary": true,
}

// seasonNumberWord is a season's number written in digits or spelled out.
func seasonNumberWord(w string) (int, bool) {
	if digits(w) {
		return atoi(w), true
	}
	if n, ok := spelledNumbers[strings.ToLower(w)]; ok {
		return n, true
	}
	// "Season IV", in capitals as a numeral is written: "Season ii" is no season's.
	n, ok := romanNumbers[w]
	return n, ok
}

// namedSeasonOf is the season a folder named by its show and season gives, "Owlpen Season 2" 2; 0
// for a name that gives none.
func namedSeasonOf(folder string) int {
	m := namedSeason.FindStringSubmatch(folder)
	if m == nil {
		return 0
	}
	n, _ := seasonNumberWord(matched(m[2], m[3]))
	return n
}

// matched is the one group of several that took part in a match, where each group is in an
// alternative of its own: "第(\d+)話|(\d+)화" fills one or the other, never both, as a regexp's
// alternatives never match together.
func matched(groups ...string) string {
	for _, g := range groups {
		if g != "" {
			return g
		}
	}
	return ""
}

// romanNumbers are a season's number as a Roman numeral.
var romanNumbers = map[string]int{
	"I": 1, "II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6, "VII": 7, "VIII": 8, "IX": 9, "X": 10,
	"XI": 11, "XII": 12, "XIII": 13, "XIV": 14, "XV": 15,
}

// isSeasonFolder says whether dir, relative to the root, is a season folder. One that starts with a
// show's name, or its initials, is one only inside that show's folder: "Kettle Bay/Kettle Bay
// Season 1" and "Kettle Bay/KB Season 1" are, while a "Kettle Bay Season 1" folder of its own, with
// no show folder above it, is the show's.
func isSeasonFolder(dir string) bool {
	name := path.Base(dir)
	if _, ok := seasonIn(name); ok {
		return true
	}
	// "Westerly Specials", "Westerly - Specials" and "Tamsworth.Specials" in the show's own folder
	// are its specials; "TV/Kettle Specials/" alone is a show's own folder.
	if m := showSpecials.FindStringSubmatch(name); m != nil && path.Dir(dir) != "." && sameWords(m[1], path.Base(path.Dir(dir))) {
		return true
	}
	m := namedSeason.FindStringSubmatch(name)
	if m == nil || path.Dir(dir) == "." {
		return false
	}
	if _, ok := seasonNumberWord(matched(m[2], m[3])); !ok {
		return false
	}
	show := path.Base(path.Dir(dir))
	return sameWords(m[1], show) || strings.EqualFold(m[1], initials(show))
}

// showSpecials matches a show's specials' folder named by the show: "Westerly Specials", "Westerly
// - Specials", "Tamsworth.Specials", "Kaito Run OVAs". Its group is the show's name.
var showSpecials = regexp.MustCompile(`(?i)^(.+?)[ ._-]+(?:specials|ovas?|oads?)$`)

// animeAside matches an anime release's show named with a special's or a recap's word after a
// dash, ahead of its number: "Kame - Special" of "[Grp] Kame - Special - 01".
var animeAside = regexp.MustCompile(`(?i)^(.+?)[ ._]+-[ ._]+(special|recap|summary)$`)

// initials are a name's words' first letters: "Kettle Bay (2001)" is "KB".
func initials(name string) string {
	var b strings.Builder
	for _, w := range strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '.' || r == '_' || r == '-' }) {
		if r, _ := utf8.DecodeRuneInString(w); unicode.IsLetter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// seasonDirOf is the season folder a file's folder dir is, or sits in ("Season 1/Disc 2/",
// "Season 1/x265/"), "" for none.
func seasonDirOf(dir string) string {
	switch {
	case dir == ".":
		return ""
	case isSeasonFolder(dir):
		return dir
	case path.Dir(dir) != "." && isSeasonFolder(path.Dir(dir)):
		return path.Dir(dir)
	}
	return ""
}

// aboveSeasons is the folder above a season folder and any season folders it sits in
// ("Show/Season 1/Season 1" is "Show", as an archive unpacked into its own name leaves it); "." is
// the root, for season folders with no show folder above them.
func aboveSeasons(dir string) string {
	for dir != "." && isSeasonFolder(dir) {
		dir = path.Dir(dir)
	}
	return dir
}

// sameWords says whether two names have the same words, whatever their case and separators.
func sameWords(a, b string) bool {
	words := func(s string) string {
		return strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(strings.ToLower(s))), " ")
	}
	return words(a) == words(b)
}
