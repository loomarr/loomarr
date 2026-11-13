package scan

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Word families: what a folder is, said by any word of a kind rather than by the few spellings a
// list would name. A family is matched on a name's words as normName writes them, each in the
// singular or the plural, so "Blooper", "Bloopers" and "BLOOPERS" are one; and a family's word
// may follow the title it is of ("Rowan Tor Extras", "Pike.Mere.S02.Extras.720p").

// extrasKinds are what a title's extras folder is named for, each as its words in the singular: a
// kind of extra in Kodi's, Jellyfin's, Plex's or a disc's words.
var extrasKinds = [][]string{
	{"extra"}, {"featurette"}, {"interview"}, {"scene"}, {"short"}, {"trailer"}, {"teaser"},
	{"other"}, {"sample"}, {"bonu"}, {"clip"}, {"backdrop"}, {"proof"}, {"supplement"}, {"supplemental"},
	{"blooper"}, {"outtake"}, {"gag", "reel"}, {"making", "of"}, {"behind", "the", "scene"},
	{"deleted", "scene"}, {"special", "feature"}, {"bonu", "feature"}, {"bonu", "material"},
	{"bonu", "content"}, {"bonu", "disc"}, {"extra", "disc"}, {"dvd", "extra"}, {"extra", "and", "bonu"},
	{"theme", "music"}, {"promo"}, {"commentary"},
	// An anime release's openings and endings without their credits, and its menus.
	{"creditless"}, {"nc"}, {"ncop"}, {"nced"}, {"ncop", "and", "nced"}, {"op", "and", "ed"}, {"op", "ed"},
	{"opening"}, {"ending"}, {"menu"},
}

// singular is a family's word as a name may write it: "bloopers" and "bonus" are "blooper" and
// "bonu", so a family lists each word once.
func singular(w string) string {
	if len(w) > 2 && strings.HasSuffix(w, "s") {
		return w[:len(w)-1]
	}
	return w
}

// extrasTail is how many of a name's last words, its release's notes aside, name a kind of extra
// (extrasKinds); 0 for none. "Kettle Moor S01 Bonus Features" ends in 2, "Pike Mere S02 Extras
// 720p" in 1 before its notes.
func extrasTail(words []string) (tail, notes int) {
	for len(words) > 1 && releaseToken(words[len(words)-1]) {
		words, notes = words[:len(words)-1], notes+1
	}
	for _, kind := range extrasKinds {
		if n := len(kind); n > tail && n <= len(words) && slices.EqualFunc(words[len(words)-n:], kind, func(w, k string) bool {
			return singular(w) == k || w == k
		}) {
			tail = n
		}
	}
	if tail == 0 {
		notes = 0
	}
	return tail, notes
}

// showKinds are the words a show's folder labels what it is with, in brackets after its name:
// "(TV Series)", "(Mini-Series)", "(Limited Series)", "(Docuseries)", "[TV]", "(Anime)". A label
// that tells two shows apart, "(US)", "(Animated)", is the name's.
var showKinds = map[string]bool{
	"tv": true, "series": true, "show": true, "mini": true, "miniseries": true, "limited": true,
	"docuseries": true, "docu": true, "documentary": true, "anime": true, "web": true,
}

// showLabel matches a bracketed group at a show's name's end.
var showLabel = regexp.MustCompile(`[ ._]*[(\[]([^)\]]*)[)\]][ ._]*$`)

// bareShowLabel matches a label after a show's name with no brackets that says it is a show:
// "The Long Watch Miniseries", "Cold Mere Limited Series", "Ashby TV Series". "Series" alone ends
// many a title's name ("Star Series").
var bareShowLabel = regexp.MustCompile(`(?i)[ ._-]+(?:mini[ ._-]?series|limited[ ._-]series|tv[ ._-]series|docu[ ._-]?series)$`)

// showLabelled says whether a folder's name labels it a show's, bracketed or not: "The Long Watch
// Miniseries", "Cold Mere (Limited Series)". It is a show's on any shelf, a films' too.
func showLabelled(name string) bool {
	if bareShowLabel.MatchString(name) {
		return true
	}
	m := showLabel.FindStringSubmatch(name)
	if m == nil {
		return false
	}
	words := strings.Fields(normName(m[1]))
	return slices.ContainsFunc(words, func(w string) bool { return w == "mini" || w == "miniseries" || w == "limited" || w == "docuseries" }) &&
		!slices.ContainsFunc(words, func(w string) bool { return !showKinds[w] })
}

// withoutShowLabels is a show's name less the labels after it that say what it is or when it ran
// (showKinds, a span of years "(2010-2015)" or "(2010 - )", a bare "Miniseries"), as many as it
// has; a name of nothing but a label keeps it.
func withoutShowLabels(name string) string {
	if loc := bareShowLabel.FindStringIndex(name); loc != nil && strings.TrimSpace(name[:loc[0]]) != "" {
		name = strings.TrimSpace(name[:loc[0]])
	}
	for {
		m := showLabel.FindStringSubmatchIndex(name)
		if m == nil {
			return name
		}
		inner := name[m[2]:m[3]]
		words := strings.Fields(normName(inner))
		kind := len(words) > 0 && !slices.ContainsFunc(words, func(w string) bool { return !showKinds[w] })
		span := runSpan.MatchString(strings.TrimSpace(inner))
		bare := strings.TrimSpace(name[:m[0]])
		if !kind && !span || bare == "" {
			return name
		}
		name = bare
	}
}

// runSpan matches the years a show ran, apart by a dash, the last left open or not: "2010-2015",
// "2010 - 2015", "2010–", "2010-present".
var runSpan = regexp.MustCompile(`(?i)^(?:19|20)\d{2}\s*[-–]\s*(?:(?:19|20)\d{2}|present|now)?$`)

// seasonNumbered is the season a folder in a show's numbers alone: "01" and "2" are seasons 1 and
// 2, "0" and "00" its specials', whatever spaces are around the number. ok is false for any other
// name.
func seasonNumbered(name string) (season int, ok bool) {
	if name = strings.TrimSpace(name); len(name) <= 2 && digits(name) {
		return atoi(name), true
	}
	return 0, false
}

// holdsWhole says whether a folder in a show's names only that it holds the show's episodes, with
// no number: "All", "Complete", "Episodes".
func holdsWhole(name string) bool {
	return holdingOnly(strings.Fields(normName(name))) && !strings.ContainsFunc(name, unicode.IsDigit)
}

// groupingOf reads a folder named as a grouping of a show's episodes (groupingFolder): the name
// before it, if any, its word in lower case and its number. "Abbot Bear Vol 1" is "Abbot Bear",
// "vol", 1; "Book Two" is "", "book", 2. A name whose word numbers nothing ("DVDA") is no grouping.
func groupingOf(name string) (before, word string, n int, ok bool) {
	m := groupingFolder.FindStringSubmatch(name)
	if m == nil {
		return "", "", 0, false
	}
	if n, ok = seasonNumberWord(m[3]); !ok {
		return "", "", 0, false
	}
	return m[1], strings.ToLower(m[2]), n, true
}

// numberedIn says whether a folder named by a number alone in the folder parent is a season's: in
// a year's folder, "2022/11/", it is a month's.
func numberedIn(parent, name string) (season int, ok bool) {
	if season, ok = seasonNumbered(name); !ok || yearToken.MatchString(path.Base(parent)) {
		return 0, false
	}
	return season, true
}

// notesOnly says whether a folder is named by a release's notes alone: "x265", "1080p", "WEB-DL
// 1080p". It holds a copy of the episodes of the folder it is in.
func notesOnly(name string) bool {
	words := strings.Fields(normName(name))
	return len(words) > 0 && !slices.ContainsFunc(words, func(w string) bool { return !releaseToken(w) })
}

// seasonOf says whether the folder dir is one of the show's above it keeps its episodes in by
// season: a season's folder, one numbered alone (numberedIn), or a year's, "Pell Daily/2021/". A
// folder that keeps them is a show's.
func seasonOf(dir string) bool {
	_, numbered := numberedIn(path.Dir(dir), path.Base(dir))
	_, _, ordinal := seasonGrouping(path.Base(dir))
	return isSeasonFolder(dir) || numbered || ordinal || yearToken.MatchString(path.Base(dir))
}

// seasonGrouping reads a grouping folder (groupingOf) whose word counts a show's seasons: "Book
// Two", "Chapter 2" and "Year 2" are season 2, as "Season 2" is.
func seasonGrouping(name string) (before string, season int, ok bool) {
	before, word, n, ok := groupingOf(name)
	if !ok || word != "book" && word != "chapter" && word != "year" {
		return "", 0, false
	}
	return before, n, true
}

// groupsOnly says whether a folder in parent only groups a show's episodes by its name: a season's
// number alone (numberedIn), words that say it holds the show whole, or a grouping's word and
// number with no name of its own ("Part 2", "Arc 1"). In a show's folder it is the show's
// (regrouped).
func groupsOnly(parent, name string) bool {
	if _, ok := numberedIn(parent, name); ok || holdsWhole(name) {
		return true
	}
	before, _, _, ok := groupingOf(name)
	return ok && before == ""
}

// shelfAt says whether the folder at dir, relative to the root, is a shelf: a catch-all its name
// says is one (catchAllFolder). A name the lists hold, "Movies", "Downloads", "A-C", is one
// anywhere; one only its words read as a shelf, "Animated Series", "DVD Rips", is one only at the
// top or among shelves: inside a show's folder, "Star Ridge/Animated Series/", it is the title's.
func shelfAt(dir string) bool {
	name := path.Base(dir)
	if !catchAllFolder(name) {
		return false
	}
	key := strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(name))), " ")
	if knownKey(key) || alphabetFolder.MatchString(key) {
		return true
	}
	parent := path.Dir(dir)
	return parent == "." || shelfAt(parent)
}

// extrasName says whether a folder is named by a kind of extra alone: "Extras", "Bloopers",
// "Making Of", "Others", "Bonus Features".
func extrasName(name string) bool {
	words := strings.Fields(normName(name))
	tail, notes := extrasTail(words)
	return tail > 0 && tail+notes == len(words)
}

// extrasAfter is the words a folder's name gives before the kind of extra it ends in: "Rowan Tor"
// of "Rowan Tor Extras"; nil for a name that ends in none, or is one alone.
func extrasAfter(name string) []string {
	words := strings.Fields(normName(name))
	tail, notes := extrasTail(words)
	if tail == 0 || tail+notes == len(words) {
		return nil
	}
	return words[:len(words)-tail-notes]
}
