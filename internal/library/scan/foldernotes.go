package scan

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// Folder notes: what a folder's videos say together that none says alone (they share a show's
// name, number themselves by season and episode run together, are titled only, are a dump of
// several shows), read once per folder by the walk and given to each file.

// folderNotes are what a folder's videos say about each of them together.
type folderNotes struct {
	// Numbered is a folder two or more of whose videos number themselves after a name they share
	// ("kb 101", "kb 201").
	Numbered bool
	// Spelled is a folder two or more of whose videos number themselves by an episode's word after
	// a name they share (spelledEpisode, cjkEpisode): "Rook Hall Mini E01" beside "… E02", "Reed
	// Wood - Episode 1" beside "… Episode 2", "Kettle Hana 第05集" beside "… 第06集".
	Spelled bool
	// Hundreds is the hundred every video named by a leading number of three digits shares ("101 -
	// Tidewater", "102 - Low Water" share 1), and 0 when they share none, or one is "99" or a whole
	// hundred.
	Hundreds int
	// Composite is a folder whose videos all start with a season's number and an episode's run
	// together, in two seasons or more: "101 Pilot", "102 The Well", "201 Return", "1210 Late
	// Year" (compositeNumbers).
	Composite bool
	// Dump is a folder whose episodes name their shows, none of them the folder's own:
	// "Torrents/Kettle Bay S01E01.mkv" beside "Torrents/Ferrow Lane S02E03.mkv", or a genre's
	// "TV/Kids/Kettle Bay S01E04.mkv". It names no show, as a catch-all does.
	Dump bool
	// Episodes is a folder two or more of whose videos carry an episode's mark, or one holding
	// season folders: a show's own folder, whose unmarked videos ("Pilot.mkv", "Show - Christmas
	// Special.mkv") are its episodes too.
	Episodes bool
	// Leading is a folder two or more of whose videos start with their number ("01 - The Deep",
	// "2. Low Water") and none gives a year: a show's episodes, kept with no season folder.
	Leading bool
	// Series is a folder two or more of whose videos are named by the show its own name gives, and
	// none gives a year: "Puddle Ducks/Puddle Ducks - The Big Rain.mp4" beside "… - Windy Day.mp4",
	// "Pebble Pals Season 1/Pebble Pals 1.mp4". A film's folder holds one film, and a collection's
	// films give their years.
	Series bool
	// Root is the root's own name when it is a shelf ("TV" for "/media/TV" added as a root), ""
	// otherwise: the shelf predicates read the folder as if under it (shelved).
	Root string
	// Titled is a folder on an audience's shelf whose videos are a show's episodes named by their
	// titles alone (titledEpisodes): "Kids/Hop Pond/Frost Morning.mp4" beside "Big Carrot.mp4".
	Titled bool
	// Dashed are the shows a catch-all's loose videos name before a dash, two or more apiece with
	// no year or number: "Kids/Hob Duck - Big Pond.mp4" beside "Kids/Hob Duck - Wet Feet.mp4" are
	// Hob Duck's (dashedShows), keyed by normName.
	Dashed map[string]string
	// Folders are the folder's own folders, by name in order: a loose episode beside its show's
	// folder ("TV/Holm Lane S01E02.mkv" by "TV/Holm Lane/") is that show's.
	Folders []string
	// MonthFirst is a folder whose videos' days written with the year last are America's, month
	// first: one of them can be read no other way ("03-14-2023"), and none is day first ("14-03-2023").
	MonthFirst bool
	// Years are the year the folder's loose episodes give each show they name with one, by its
	// name (normName): "Pembroke.Gate.2019.S01E01" gives "pembroke gate" 2019. A show two of them
	// give two years is two shows, and has none.
	Years map[string]int
}

// looseYears reads folderNotes.Years from a folder's videos' names.
func looseYears(names []string) map[string]int {
	years := map[string]int{}
	// A show's episode numbered alike with and without the year, "Blue.Fen.S01E01" beside
	// "Blue.Fen.2020.S01E01", is two shows' (an original and its remake): neither is the other's.
	type episode struct {
		title  string
		dated  bool
		season int
		number int
	}
	seen := map[episode]bool{}
	clash := map[string]bool{}
	seasons := map[string]map[bool][]int{}
	for _, name := range names {
		n, ok := numberingIn(name)
		if !ok {
			continue
		}
		key, dated := looseKey(ownSeries(name)), false
		if m := looseYear.FindStringSubmatch(key); m != nil {
			key, dated = m[1], true
		}
		e := episode{title: normName(key), dated: dated, season: n.Season, number: n.Episode}
		seen[e] = true
		if other := (episode{title: e.title, dated: !dated, season: e.season, number: e.number}); seen[other] {
			clash[e.title] = true
		}
		if seasons[e.title] == nil {
			seasons[e.title] = map[bool][]int{}
		}
		seasons[e.title][dated] = append(seasons[e.title][dated], n.Season)
	}
	// So is one whose seasons with and without the year are apart: "Moss.Ridge.S05E01" beside
	// "Moss.Ridge.2019.S01E01" is the original's, not the remake's.
	for title, s := range seasons {
		if seasonsApart(s[false], s[true]) {
			clash[title] = true
		}
	}
	for _, name := range names {
		m := looseYear.FindStringSubmatch(looseKey(ownSeries(name)))
		if m == nil || clash[normName(m[1])] {
			continue
		}
		title, year := normName(m[1]), atoi(m[2])
		if seen, ok := years[title]; ok && seen != year {
			years[title] = 0
			continue
		}
		years[title] = year
	}
	if len(years) == 0 {
		return nil
	}
	return years
}

// seasonsApart says whether a show's seasons kept without its year are an original's and those kept
// with it a remake's: the dated ones start again at season 1 while the yearless ones run on past
// them. "S06" beside "S01 (2019)" are two shows'; "S01" beside "S02 (2016)" or "S03 (2016)" are
// one show's, missing a season or not, and a show's specials (season 0) say nothing either way.
func seasonsApart(yearless, dated []int) bool {
	yearless = slices.DeleteFunc(slices.Clone(yearless), func(s int) bool { return s == 0 })
	dated = slices.DeleteFunc(slices.Clone(dated), func(s int) bool { return s == 0 })
	if len(yearless) == 0 || len(dated) == 0 {
		return false
	}
	return slices.Min(dated) == 1 && slices.Min(yearless) > slices.Max(dated)+1
}

// leadingEpisodes says whether a folder's videos are a show's numbered by their place alone: two or
// more start with a number and a separator ("01 - The Deep", "2. Low Water"), and none gives a
// year, as a collection's films do ("01 - Marrow Point (2010)").
func leadingEpisodes(folder string, names []string) bool {
	numbered, franchise := 0, 0
	_, folderYear, _ := searchQuery(folder, 0)
	own := showWordsOf(folder)
	for _, name := range names {
		s := stem(name)
		if _, year, _ := searchQuery(s, 0); year != 0 || bracketYear.MatchString(s) {
			return false
		}
		// A franchise's films in order are each named by it, "01 - Ironclad Sky", "02 - Ironclad Sky
		// Rising"; a show's episodes are named by their own titles, or by the show and a dash before
		// one: "01 - Ocean Halls - The Deep".
		if m := leadingPlace.FindStringSubmatchIndex(s); m != nil && len(own) > 0 {
			title := s[m[1]-1:]
			if words := strings.Fields(normName(title)); len(words) >= len(own) && slices.Equal(words[:len(own)], own) &&
				!showTitleDash.MatchString(title) {
				franchise++
			}
		}
		// "Fizzy Hen/01.mp4" beside "02.mp4" is numbered by its place alone; in a film's folder,
		// "Ashgrove (2015)/01.mkv", those are its titles.
		if bare := plainStem(name); leadingPlace.MatchString(s) || folderYear == 0 && digits(bare) && len(bare) == 2 && bare[0] == '0' {
			numbered++
		}
	}
	return numbered >= 2 && franchise < 2
}

// namedSeries says whether a folder's videos are a show's named by the show its folder's name
// gives (folderNotes.Series). audience is a folder on an audience's shelf, "Kids/Gumdrop Garage/",
// where "Gumdrop Garage 1" beside "… 2" with none named "Gumdrop Garage" alone are a show's
// episodes, not a film's sequels: a kids' show is numbered so far more than a film is.
func namedSeries(folder string, names []string, audience bool) bool {
	own := showWordsOf(folder)
	if len(own) == 0 || catchAllFolder(folder) {
		return false
	}
	alone := slices.ContainsFunc(names, func(name string) bool {
		return slices.Equal(strings.Fields(normName(plainStem(name))), own)
	})
	named := 0
	for _, name := range names {
		s := plainStem(name)
		words := strings.Fields(normName(s))
		// The show's own film kept with it, "Peppy Hog Movie (2019)", says nothing of its episodes.
		if len(words) > len(own) && slices.Equal(words[:len(own)], own) && showFilm.MatchString(s) {
			continue
		}
		if _, year, _ := searchQuery(s, 0); year != 0 || bracketYear.MatchString(s) {
			return false
		}
		// "Ironvale 2" and "Ironvale 3" beside "Ironvale" are a film's sequels: a show's episodes
		// say more after its name.
		sequel := len(words) == len(own)+1 && digits(words[len(own)])
		if sequel && audience && !alone && slices.Equal(words[:len(own)], own) {
			named++
			continue
		}
		// One film's editions, "… - Extended" and "… - Theatrical", are no show's; nor is a film's
		// title that runs on from its franchise's, "Wren Lake and the Stone": an episode's title is
		// set apart from its show's by a dash.
		if len(words) > len(own) && slices.Equal(words[:len(own)], own) && editionIn(name) == "" && !sequel &&
			showTitleDash.MatchString(s) {
			named++
		}
	}
	return named >= 2
}

// titledEpisodes says whether a folder on an audience's shelf holds a show's episodes named by their
// titles alone (folderNotes.Titled): "Kids/Hop Pond/Frost Morning.mp4" beside "Big Carrot.mp4". A film's
// folder holds a video named as it is ("Tansy Fell/Tansy Fell.mp4") or its sequels ("Wren Lake 2");
// a collection's films give their years, and a franchise's folder says so ("Gorse Trilogy").
func titledEpisodes(rel string, names []string) bool {
	folder := path.Base(rel)
	// At an unlabelled root's top, three or more say it: read as films, "Bumbleton/Hats.mp4" may find
	// another film called so, where a film shelf read as a show finds none and waits in Needs you.
	topShow := unshelved(rel) && len(names) >= 3 && !contentShelves[folderKey(folder)]
	if !onAudienceShelf(rel) && !topShow || catchAllFolder(folder) || filmSeries.MatchString(folder) ||
		len(idsIn(folder)) > 0 || len(names) < 2 || len(partsOf(names)) > 0 {
		return false
	}
	own := showWordsOf(folder)
	if _, year, _ := searchQuery(folder, 0); year != 0 || len(own) == 0 {
		return false
	}
	for _, name := range names {
		s := plainStem(name)
		// The show's own film kept with it, "Moss Mouse Movie (2019)", says nothing of its episodes.
		if words := strings.Fields(normName(s)); len(words) > len(own) && slices.Equal(words[:len(own)], own) && showFilm.MatchString(s) {
			continue
		}
		if _, year, _ := searchQuery(s, 0); year != 0 || bracketYear.MatchString(s) {
			return false
		}
		if words := strings.Fields(normName(s)); len(words) >= len(own) && slices.Equal(words[:len(own)], own) {
			return false
		}
	}
	return true
}

// dumpFolder says whether a folder named folder, holding videos named names, is a Dump. A show's
// own folder whose files spell it two ways, or carry an old name beside its new one, has one of
// them its own ("Kettle Bay (US)" holding "Kettle.Bay.US.S01E01" and "KB S02E01"). A show's folder
// by where it is, in a shows' catch-all, is one when its episodes name two shows or more, as a
// genre's does ("TV/Sitcoms/"); one name it misspells ("TV/Hartwell/Hartwel S02E11.mkv") is still
// the show's.
func dumpFolder(folder string, names []string, showsOwn bool) bool {
	own, _, _ := searchQuery(showFolderName(folder), 0)
	shows, longer := map[string]bool{}, map[string]bool{}
	for _, name := range names {
		show := looseSeries(name)
		if show == "" {
			continue
		}
		title, _, _ := searchQuery(show, 0)
		if relatedName(title, own) {
			// The folder's name with more after it, "Kettle Bay" in "Kettle/", is its show's when
			// it is the only one; two, "Kettle Bay" and "Kettle Run", are a franchise's shows.
			if t, o := normName(title), normName(own); t != o && strings.HasPrefix(t+" ", o+" ") {
				longer[t] = true
				continue
			}
			return false
		}
		shows[normName(title)] = true
	}
	if len(longer) == 1 {
		return false
	}
	if len(longer) > 1 {
		return true
	}
	if showsOwn {
		return len(shows) > 1
	}
	return len(shows) > 0
}

// relatedName says whether a show's name is a folder's: the same words, one's words starting the
// other's ("Kettle Bay" in "Kettle Bay US"), or the folder's initials ("KB"), with or without a
// leading article ("The Kettle Bay").
func relatedName(show, folder string) bool {
	unArticled := func(s string) string {
		for _, a := range []string{"the ", "a ", "an "} {
			s = strings.TrimPrefix(s, a)
		}
		return s
	}
	a, b := unArticled(normName(show)), unArticled(normName(folder))
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasPrefix(a+" ", b+" ") || strings.HasPrefix(b+" ", a+" ") ||
		strings.EqualFold(strings.ReplaceAll(a, " ", ""), initials(folder))
}

// notesOf reads a folder's notes from the names of its videos.
func notesOf(names []string) folderNotes {
	prefixes := map[string]int{}
	for _, name := range names {
		if m := numberedName.FindStringSubmatch(plainStem(name)); m != nil && !yearToken.MatchString(m[2]) {
			prefixes[strings.ToLower(strings.Join(strings.Fields(m[1]), " "))]++
		}
	}
	spelled := map[string]int{}
	for _, name := range names {
		s, prefix := plainStem(name), ""
		if m := spelledEpisode.FindStringSubmatch(s); m != nil {
			prefix = m[1]
		} else if at := cjkEpisode.FindStringIndex(s); at != nil {
			prefix = s[:at[0]]
		} else {
			continue
		}
		spelled[normName(untagged(prefix))]++
	}
	var notes folderNotes
	for _, n := range prefixes {
		notes.Numbered = notes.Numbered || n >= 2
	}
	for prefix, n := range spelled {
		notes.Spelled = notes.Spelled || n >= 2 && prefix != ""
	}
	notes.Composite = compositeNumbers(names)
	// Every name that starts with a number of three digits starts with one in the same hundred:
	// "101", "102", beside a "07" written plainly. A whole hundred, "100" after "99", says the
	// numbers run on past it.
	for _, name := range names {
		m := leadingNumber.FindStringSubmatch(plainStem(name))
		if m == nil {
			continue
		}
		n := atoi(m[1])
		if n < 100 && n != 99 {
			continue
		}
		switch hundreds := n / 100; {
		case n < 100 || n%100 == 0 || (notes.Hundreds != 0 && notes.Hundreds != hundreds):
			return folderNotes{Numbered: notes.Numbered, Spelled: notes.Spelled, Composite: notes.Composite}
		default:
			notes.Hundreds = hundreds
		}
	}
	return notes
}

// leadingComposite matches a name that starts with its season's number and its episode's, run
// together in three or four digits: "101 Pilot", "1210 - Late Year".
var leadingComposite = regexp.MustCompile(`^(\d{3,4})(?:[ ._-]|$)`)

// compositeNumbers says whether a folder's videos number themselves by season and episode run
// together: every video that starts with a number starts with three or four digits, none a
// year's, each an episode from 1 to 49 of its hundred's season, and the hundreds are two or more.
// A show numbered through its whole
// run counts on past a hundred, "99", "100", "101", so a folder with a number under a hundred, a
// whole hundred or a high one in its hundred ("150") is none; one hundred alone, "101"-"112",
// reads either way and is left to the season's folder (Hundreds).
func compositeNumbers(names []string) bool {
	seasons := map[int]bool{}
	for _, name := range names {
		s := plainStem(name)
		// A video with no number first, "Pilot", "Bonus", says nothing of how the rest are numbered.
		if s == "" || s[0] < '0' || s[0] > '9' {
			continue
		}
		m := leadingComposite.FindStringSubmatch(s)
		if m == nil || yearToken.MatchString(m[1]) {
			return false
		}
		n := atoi(m[1])
		if n%100 < 1 || n%100 > 49 {
			return false
		}
		seasons[n/100] = true
	}
	return len(seasons) >= 2
}

// numberedName matches a name and the number after it, three or four digits or two padded, and
// the episode's title after a dash: "kb 101", "Fizzy Fox 01", "Fizzy Cat - 01 - Paws". "Ironvale 2"
// is a sequel's.
var numberedName = regexp.MustCompile(`^(.*[^0-9 ._-])[ ._-]+(\d{3,4}|0[1-9])(?:[ ._]+-[ ._]+.*)?$`)

// numberedShow is the name a file numbers itself after (numberedName), as a show's name: "Fizzy
// Fox 01" is "Fizzy Fox"; "" for none.
func numberedShow(name string) string {
	m := numberedName.FindStringSubmatch(plainStem(name))
	if m == nil || yearToken.MatchString(m[2]) {
		return ""
	}
	return showSpelling(strings.Trim(leadingTags.ReplaceAllString(m[1], ""), " ._-"))
}
