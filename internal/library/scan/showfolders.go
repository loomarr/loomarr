package scan

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Show folders: which folder above an episode is its show's (past its season's, its grouping's,
// its copy's and its pack's folders), and the show's name and year that folder gives.

// seriesFolder is an episode's series folder relative to the root: the folder above its season
// folder, else the nearest folder whose name carries an id, else its own folder. An episode with
// no series folder, loose at the root's top or in a season folder there, takes the folder its show
// would have: its name before the episode mark. "" is the root, for one with no such name.
func seriesFolder(rel string) string { return seriesFolderOf(File{Rel: rel}) }

// folderBeside is the folder beside a loose episode, in its own folder dir, that is the show its
// name gives, "" for none: the show's folder, named as the show is with or without its year.
func folderBeside(file File, dir, show string) string {
	if dir != path.Dir(file.Rel) {
		return ""
	}
	want, _, _ := searchQuery(show, 0)
	for _, folder := range file.Notes.Folders {
		if title, _, _ := searchQuery(showFolderName(folder), 0); title != "" && strings.EqualFold(normName(title), normName(want)) {
			// A season pack's folder, "Ravenmoor.S01.1080p.WEB-DL", stands for its show's.
			if show := packShow(path.Join(dir, folder)); show != "" {
				return show
			}
			return path.Join(dir, folder)
		}
	}
	return ""
}

// namedInFull says whether a file's own name gives its show as the folder dir is named, whole:
// "TV/Unit S9/Unit S9 S01E01.mkv" is "Unit S9"'s, whose "S9" is its name and no season's.
func namedInFull(file File, dir string) bool {
	own := looseSeries(path.Base(file.Rel))
	return own != "" && sameWords(own, strings.TrimSpace(embeddedID.ReplaceAllString(path.Base(dir), "")))
}

// packShow is the show a season pack's release folder, or a complete series' copy, stands for:
// "Downloads/Bramble.House.S02.720p.WEB-DL/" and "Owlpen Season 1/" stand for "Bramble House" and
// "Owlpen", kept by name as their loose episodes are; one in a folder that names something,
// "Shelf/Owlpen Season 1/", for the show's folder beside it, "Shelf/Owlpen". "" for a folder that
// is no season's pack.
func packShow(dir string) string {
	base := path.Base(dir)
	// A season's disc label, "DAPPLE_ROAD_SEASON1_DISC2" or "Saltwick Bay S1D2", is its show's.
	// One in its show's own folder, "TV/Ashcombe/Ashcombe S1D1/", is that folder's.
	if m := seasonDisc.FindStringSubmatch(base); m != nil && dir != "." {
		if parent := path.Dir(dir); parent != "." && sameWords(showSpelling(m[1]), showFolderName(path.Base(parent))) {
			return parent
		}
		return showKey(path.Dir(dir), showSpelling(m[1]))
	}
	words := strings.FieldsFunc(base, func(r rune) bool { return strings.ContainsRune(" ._", r) })
	if dir == "." || len(words) < 2 || seasonToken.MatchString(words[0]) ||
		!slices.ContainsFunc(words[1:], func(w string) bool { return seasonToken.MatchString(strings.Trim(w, "()[]")) }) &&
			namedSeasonOf(base) == 0 && !completeWord.MatchString(base) {
		return ""
	}
	show := showFolderName(base)
	if show == base || show == "" || isSeasonFolder(path.Join(path.Dir(dir), show)) {
		return ""
	}
	if parent := path.Dir(dir); parent != "." && !shelfAt(parent) {
		return path.Join(parent, show)
	}
	return showKey(path.Dir(dir), showSpelling(show))
}

// showKey is how a show with no folder of its own, named show, is kept when its files are in
// container: in a shows' catch-all, as the show's folder there would be, "TV/Kettle Lane", so its
// season packs and loose episodes join the folder beside them; at the top or in any other
// catch-all, where no show keeps a folder ("Downloads/", "Torrents/"), by its name alone, so its
// releases there are one show wherever they are.
func showKey(container, show string) string {
	show = looseKey(show)
	if container != "." && showsFolderNames[folderKey(path.Base(container))] {
		return path.Join(container, show)
	}
	return show
}

// completeWord matches a complete series' copy named so: "Hollyhock Lane (Complete Series)",
// "Kettle.Bay.Complete.1080p".
var completeWord = regexp.MustCompile(`(?i)(?:^|[ ._(\[])complete(?:[ ._)\]]|$)`)

// looseKey is the name a show with no folder of its own is kept by, the same however its releases
// spell it: its year bracketed, "Fernhill (2019) - S01E03" and "Fernhill.2019.S01E01"; and with no
// apostrophe, "Pemberton's Orchard" and "Pembertons.Orchard".
func looseKey(show string) string {
	// An apostrophe is a name's punctuation; a name of nothing else keeps it.
	if bare := strings.NewReplacer("'", "", "’", "").Replace(show); strings.TrimSpace(bare) != "" {
		show = bare
	}
	if m := looseYear.FindStringSubmatch(show); m != nil {
		return m[1] + " (" + m[2] + ")"
	}
	return show
}

// looseYear matches a show's name and the year after it, bare or bracketed: "Fernhill 2019",
// "Fernhill (2019)".
var looseYear = regexp.MustCompile(`^(.*\S)[ ._]+[(\[]?((?:19|20)\d{2})[)\]]?$`)

// seriesFolderOf is seriesFolder for a file the walk found. A copy of a show's folder inside it, as
// unpacking leaves one ("TV/Moss Hollow/Moss Hollow/Season 2/"), stands for the show's folder; so
// does a volume of it beside it ("Kids/Puddle Ducks Vol 2/" for "Kids/Puddle Ducks").
func seriesFolderOf(file File) string {
	dir := showFolderIn(regrouped(file))
	// A season's folder off a shelf is never a show's: its show's is the one above its seasons,
	// whatever the folders below it say ("A/Season One/S0E0/Season One S1E0/"). On a shelf, one its
	// episodes name, "TV/Season One/Season 1/Season One S01E01", is a show so named, as is one
	// carrying a show's id.
	if dir != "" && dir != "." && path.Dir(dir) != "." && isSeasonFolder(dir) && len(idsIn(path.Base(dir))) == 0 &&
		(!catchAllFolder(path.Base(path.Dir(dir))) || !sameWords(looseSeries(path.Base(file.Rel)), path.Base(dir))) {
		dir = aboveSeasons(dir)
	}
	if dir == "" || dir == "." || path.Dir(dir) == "." {
		return dir
	}
	parent, base := path.Dir(dir), path.Base(dir)
	if !shelfAt(parent) && !isSeasonFolder(parent) && sameWords(showFolderName(base), showFolderName(path.Base(parent))) {
		return parent
	}
	if bare := strings.TrimSpace(volumeTag.ReplaceAllString(base, "")); volumeTag.MatchString(base) && bare != "" {
		return path.Join(parent, bare)
	}
	return dir
}

// regrouped is a file as its show's own folder holds it when a folder there groups its episodes by
// something other than a season: "TV/Lisk Saga/Volume 2/" and "Kids/Abbot Bear/Abbot Bear Vol 1/"
// hold the show's episodes as its own folder does, and "Book Two/", "Chapter 2/" and "Year 2/"
// as "Season 2/" does. A films' shelf's ("Movies/Gorse Vale (2008)/Volume 1/") is a film's, and a
// disc's or part's folder is read where it is (movieRecord, seasonDirOf).
func regrouped(file File) File {
	if out := groupedIn(file); out.Rel != file.Rel {
		return out
	}
	dir := path.Dir(file.Rel)
	parent := path.Dir(dir)
	if dir == "." || parent == "." {
		return file
	}
	base := path.Base(dir)
	_, inNumbered := numberedIn(path.Dir(parent), path.Base(parent))
	if inNumbered || notesOnly(base) || episodeMark.MatchString(base) || normName(base) == normName(path.Base(parent)) {
		// One folder deeper, in a folder that groups a show's episodes: any folder in a season's
		// numbered alone, "Bircham/01/Disc 2/", is the season's, as one in "Season 1/" is
		// (seasonDirOf); "Bircham/All/x265/" and "Bircham/All/All!/" are copies of "Bircham/All/"'s
		// episodes, and "Bircham/All/1x02 - Gale/" an episode's own folder (showFolderAbove).
		inner := file
		inner.Rel = path.Join(parent, path.Base(file.Rel))
		if out := groupedIn(inner); out.Rel != inner.Rel {
			return out
		}
	}
	return file
}

// groupedIn is a file as its show's own folder holds it when the folder it is in groups the
// show's episodes (regrouped), the file unchanged when it does not.
func groupedIn(file File) File {
	dir := path.Dir(file.Rel)
	parent := path.Dir(dir)
	if dir == "." || parent == "." || shelfAt(parent) || amongFilms(shelved(file, dir)) {
		return file
	}
	base := path.Base(dir)
	grouped := path.Join(parent, path.Base(file.Rel))
	season, numbered := numberedIn(parent, base)
	switch {
	case numbered:
		// "Bircham/01/" and "Hobtor/2/" are the show's seasons, numbered alone, and "Bircham/00/" its
		// specials; a year's "2022/11/" is its month's.
		grouped = path.Join(parent, "Season "+strconv.Itoa(season), path.Base(file.Rel))
	case isSeasonFolder(dir):
		return file
	case holdsWhole(base):
		// "Hobmoor/All/" and "Hobwick/Complete/" hold the show's episodes, as its own folder does.
	default:
		before, _, _, ok := groupingOf(base)
		if !ok || before != "" && !sameWords(before, showFolderName(path.Base(parent))) {
			return file
		}
		if _, n, ok := seasonGrouping(base); ok {
			grouped = path.Join(parent, "Season "+strconv.Itoa(n), path.Base(file.Rel))
		}
	}
	// A folder that groups a show's videos holds its episodes, named or not ("Volume 3/Tamsin Train
	// - Brave Engine.mp4"); they name their show, not the folder, which makes it no dump of several.
	file.Rel, file.Notes.Dump, file.Notes.Episodes = grouped, false, true
	return file
}

// groupingFolder matches a folder that groups a show's episodes by other than its season, its name
// before it or not: "Volume 2", "Vol.1", "Abbot Bear Vol 1", "Book One", "Chapter 2", "Year 1",
// "DVD1", "Collection 3", "Part 2", "Arc 1", "Cour 2", "Saga 3".
var groupingFolder = regexp.MustCompile(`(?i)^(?:(.+?)[ ._-]+)?(volume|vol|book|chapter|year|dvd|collection|part|arc|cour|saga)[ ._-]*(\d{1,2}|[a-z]+)$`)

// volumeTag matches a show's volume after its name: "Puddle Ducks Vol 2", "Puddle Ducks - Volume 3".
var volumeTag = regexp.MustCompile(`(?i)[ ._-]+vol(?:ume)?[ ._]*\d{1,2}$`)

// showFolderIn is the show's folder a file's path names (seriesFolderOf).
func showFolderIn(file File) string {
	dir, name := path.Dir(file.Rel), path.Base(file.Rel)
	// A folder named by an episode's mark is the episode's own (Emby's "Season 2/S02E05/", a
	// release's "Kettle.Bay.S01E02.720p-GRP/"): its show is above it, and with none above, it names
	// the show as a loose episode's name does.
	// A file marked as another episode is not that episode's: "Block 2x4/Block 2x4 S01E01.mkv" is in
	// its show's folder.
	for dir != "." && episodeMark.MatchString(path.Base(dir)) {
		own, marked := numberingIn(name)
		if folder, _ := numberingIn(path.Base(dir)); marked && (own.Season != folder.Season || own.Episode != folder.Episode) {
			break
		}
		dir, name = path.Dir(dir), path.Base(dir)
	}
	// A folder named by a bare number in a show's folder is a season's when the episode's mark says
	// that season: "Kettle Bay/1/Kettle Bay S01E01.mkv".
	if n, ok := numberingIn(name); ok && path.Dir(dir) != "." && len(path.Base(dir)) <= 2 &&
		digits(path.Base(dir)) && atoi(path.Base(dir)) == n.Season {
		dir = path.Dir(dir)
	}
	// A catch-all folder ("Downloads/Complete", "Media", "TV/All Shows") names no show: an episode
	// there that names its own is that show's, as one loose at the top is. A catch-all is known by
	// its name, or by its episodes naming shows with none of them its own (a Dump): a show's own
	// folder names several too, when the show was renamed or its files spell it two ways, but one of
	// them is the folder's.
	show := ownSeries(name)
	if show == "" && inAnime(shelved(file, dir)) {
		show, _, _ = animeRelease(name, true)
	}
	// A folder's files numbered after a name they share, "Cartoons/Fizzy Fox 01.mp4" beside "… 02",
	// are that show's.
	if show == "" && file.Notes.Numbered && dir == path.Dir(file.Rel) {
		show = numberedShow(name)
	}
	if shelved, _, ok := shelvedEpisode(name, dir); show == "" && ok {
		show = shelved
	}
	// "Anime/Kame Ep 5.mkv" in a catch-all is Kame's: a shelf names no show, so the words before
	// the episode's word are its show.
	if m := episodeWord.FindStringIndex(plainStem(name)); show == "" && m != nil && m[0] > 0 && shelfAt(dir) {
		show = showSpelling(strings.TrimSpace(plainStem(name)[:m[0]]))
	}
	// And "Kids/Hob Duck - Big Pond.mp4" beside "… - Wet Feet.mp4" is Hob Duck's (dashedOf).
	if shared := dashedOf(file); show == "" && shared != "" && dir == path.Dir(file.Rel) {
		show = shared
	}
	// So, loose at the top or in a catch-all, is "Bramblecourt E01" beside "Bramblecourt E02".
	if m := spelledEpisode.FindStringSubmatch(plainStem(name)); show == "" && m != nil &&
		(dir == "." || shelfAt(dir)) && file.Notes.Spelled &&
		!leadingTags.MatchString(name) {
		show = showSpelling(strings.TrimSpace(m[1]))
	}
	show = looseKey(show)
	// "Pembroke.Gate.S01E02" beside "Pembroke.Gate.2019.S01E01" is the 2019 show's: the one year
	// its folder's episodes give it.
	if year := file.Notes.Years[normName(show)]; year != 0 && dir == path.Dir(file.Rel) {
		show += " (" + strconv.Itoa(year) + ")"
	}
	if (dir == "." || shelfAt(dir) || file.Notes.Dump && dir == path.Dir(file.Rel)) && show != "" {
		// Beside its show's own folder, "TV/Holm Lane S01E02.mkv" by "TV/Holm Lane/", it is that
		// show's.
		if own := folderBeside(file, dir, show); own != "" {
			return own
		}
		if dir == "." {
			return show
		}
		// A folder in a show's own that names no show, "Kettle Bay/new/" or a misspelt
		// "Kettle Bay/Seaosn 2/", holds that show's episodes.
		for d := path.Dir(dir); d != "."; d = path.Dir(d) {
			if !shelfAt(d) && !isSeasonFolder(d) && relatedName(show, showFolderName(path.Base(d))) {
				return d
			}
		}
		return showKey(dir, show)
	}
	if dir == "." {
		return show
	}
	if season := seasonDirOf(dir); season != "" {
		show := aboveSeasons(season)
		if show != "." && !shelfAt(show) {
			// "Hollyhock Lane (Complete Series)/S04/" is a copy of the show's, kept as its other
			// releases are.
			above := showFolderAbove(show)
			if pack := packShow(above); pack != "" && (path.Dir(above) == "." || shelfAt(path.Dir(above))) &&
				!namedInFull(file, above) {
				return pack
			}
			return above
		}
		// With no show's folder above, the top season folder is the show's when the episode names it
		// as its show: "TV/Series Seven/Series Seven S01E01.mkv".
		top := season
		for path.Dir(top) != show {
			top = path.Dir(top)
		}
		if own := looseSeries(name); own != "" && sameWords(own, path.Base(top)) {
			return top
		}
		return looseKey(looseSeries(name))
	}
	// A year's folder in a show's ("Evening Ledger/2024/") holds that year's episodes, as a season's
	// does, and so does a month's in it ("Morning Desk/2022/12/").
	if base := path.Base(dir); len(base) <= 2 && digits(base) && atoi(base) >= 1 && atoi(base) <= 12 &&
		yearToken.MatchString(path.Base(path.Dir(dir))) {
		dir = path.Dir(dir)
	}
	if yearToken.MatchString(path.Base(dir)) && path.Dir(dir) != "." && !shelfAt(path.Dir(dir)) {
		dir = path.Dir(dir)
	}
	dir = showFolderAbove(dir)
	for d := dir; d != "."; d = path.Dir(d) {
		if len(idsIn(path.Base(d))) > 0 {
			return d
		}
	}
	// A season pack's release folder with no show's folder above it, "Downloads/Bramble.House.S02.
	// 720p.WEB-DL/", stands for the show's folder beside it, so the show's other seasons, in their
	// own packs or its folder ("TV/Kettle Lane/"), are one show with it; as does a season's folder
	// named by its show, "Owlpen Season 1/" beside "Owlpen Season 2/".
	if show := packShow(dir); show != "" && !namedInFull(file, dir) {
		return show
	}
	return dir
}

// showFolderAbove is the show's folder a folder of a show's files is part of: the folder itself,
// or the show's folder it is unpacked or copied into. A season pack's release folder
// ("Kettle Bay/Kettle.Bay.S02.1080p.WEB-DL/") is its show's, as is a copy named by the show and
// what it holds ("Kettle Bay/Kettle Bay Complete/", "Kettle Bay/Kettle Bay (Season 2)/",
// "Kettle Bay/Kettle Bay DVD 1/"), and one named only by what it holds ("Kettle Bay/Volume 1/"). A
// show in a franchise's folder, "Star Ridge/Star Ridge Origins/", is its own, as are a remake's and
// a sequel's: "Starhaven/Starhaven (1978)/", "Kettlebridge/Kettlebridge 2/".
func showFolderAbove(dir string) string {
	for dir != "." && path.Dir(dir) != "." {
		base, above := path.Base(dir), path.Base(path.Dir(dir))
		if shelfAt(path.Dir(dir)) || isSeasonFolder(path.Dir(dir)) {
			break
		}
		release := showFolderName(base) != base && relatedName(showFolderName(base), showFolderName(above))
		if !release && !copyOfShow(base, showFolderName(above)) && !holdingOnly(strings.Fields(normName(base))) &&
			!groupsOnly(path.Dir(dir), base) {
			break
		}
		dir = path.Dir(dir)
	}
	return dir
}

// copyOfShow says whether a folder's name is a show's name and words that say only what the folder
// holds of it: "Kettle Bay Complete", "Kettle Bay (Season 2)", "Kettle Bay DVD 1", "Kettle Bay
// 1080p".
func copyOfShow(folder, show string) bool {
	words, own := strings.Fields(normName(folder)), strings.Fields(normName(show))
	if len(own) == 0 || len(words) <= len(own) || !slices.Equal(words[:len(own)], own) {
		return false
	}
	return holdingOnly(words[len(own):])
}

// holdingOnly says whether a folder's words, after its show's name or alone, say only what it
// holds of a show: "Complete", "Volume 1", "DVD 2", "1080p". A number first, or a year, names
// another show: "Kettlebridge 2", "Starhaven (1978)".
func holdingOnly(words []string) bool {
	if len(words) == 0 || digits(words[0]) {
		return false
	}
	for _, w := range words {
		if !holdingWords.MatchString(w) || yearToken.MatchString(w) {
			return false
		}
	}
	return true
}

// holdingWords are words a show's folder carries to say what it holds of the show.
var holdingWords = regexp.MustCompile(`^(?:\d{1,4}|s\d{1,4}|complete|collection|` + seasonWords +
	`|dvd|disc|disk|vol|volume|box|boxset|set|pack|batch|full|all|episodes|bluray|web|webdl|webrip|hdtv|remux|\d{3,4}p|x26[45]|h26[45]|hevc|uhd|4k|sd|hd)$`)

// packSeason is the season a season pack's release name gives by its token after the show's name,
// "Pike.Moss.S02.1080p" 2; 0 for none, or for a pack of several ("S01-S03").
func packSeason(name string) int {
	words := strings.FieldsFunc(name, func(r rune) bool { return strings.ContainsRune(" ._", r) })
	for _, w := range words[min(1, len(words)):] {
		// "Bracken Hall (S07)" brackets its season.
		if w = strings.Trim(w, "()[]"); seasonToken.MatchString(w) && !strings.Contains(w, "-") {
			return atoi(w[1:])
		}
	}
	return 0
}

// seasonToken is a word in a release folder's name that starts what it holds of a show: "S02",
// "S01-S03".
var seasonToken = regexp.MustCompile(`(?i)^s\d{1,2}(?:-s?\d{1,2})?$`)

// showYear matches the year an episode's file name gives its show, in brackets just before the
// episode's mark: "Show (1963) - S01E01", "Show (1963) - S01xE01", "Show.[1963].1x01".
var showYear = regexp.MustCompile(`[(\[]((?:18|19|20)\d\d)[)\]][\s._-]*(?:[Ss]\d{1,4}[xX]?[Ee]\d|\d{1,2}[xX]\d)`)

// episodeRange matches a fansub pack's run of episodes, bracketed or after a dash: "(01-12)",
// "[01~26]", " - 01-12 [Batch]".
var episodeRange = regexp.MustCompile(`[(\[]\s*\d{1,4}\s*[-~]\s*\d{1,4}\s*[)\]]|[ ._]-[ ._]+\d{1,4}(?:[-~]|\s+~\s+)\d{1,4}(?:[ ._\[(]|$)`)

// batchWord matches the word a pack is named by: "[Batch]", "(batch)".
var batchWord = regexp.MustCompile(`(?i)[\[( ._]batch[\]) ._]|[\[( ._]batch$`)

// showYearIn is the year an episode's file name gives its show; 0 when it gives none.
func showYearIn(name string) int {
	m := showYear.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	year, _ := strconv.Atoi(m[1])
	return year
}

// showFolderName is a show's name from its folder: the folder's own name, less its ids, or, for a
// release folder, its words before what the folder holds of the show ("Kettle.Bay.S02.1080p-GRP" and
// "Kettle Bay Complete Series (2010-2015)" are "Kettle Bay"). A fansub pack's folder is named less
// its group, its run of episodes and its notes: "[Kumo] Sora no Kettle (01-12) [BD 1080p]" is "Sora
// no Kettle".
func showFolderName(folder string) string {
	name := strings.TrimSpace(embeddedID.ReplaceAllString(folder, ""))
	// "Copper Vale (TV Series)" and "Ledbury Row (2010-2015)" are "Copper Vale" and "Ledbury Row";
	// "(TV)" alone is its own name.
	name = withoutShowLabels(name)
	// "Puddle Ducks Vol 2" is "Puddle Ducks" (seriesFolderOf).
	if bare := strings.TrimSpace(volumeTag.ReplaceAllString(name, "")); bare != "" {
		name = bare
	}
	if (leadingTags.MatchString(name) || batchWord.MatchString(name)) && episodeRange.MatchString(name) {
		// A pack named by nothing but its tags and its run keeps its name.
		if pack := strings.TrimSpace(leadingTags.ReplaceAllString(strings.TrimSpace(name[:episodeRange.FindStringIndex(name)[0]]), "")); pack != "" {
			name = pack
		}
	}
	spaced := name
	if !strings.Contains(spaced, " ") {
		spaced = strings.NewReplacer(".", " ", "_", " ").Replace(spaced)
	}
	words := strings.Fields(spaced)
	for i := 1; i < len(words); i++ {
		w, next := strings.ToLower(words[i]), ""
		if i+1 < len(words) {
			next = strings.ToLower(strings.Trim(words[i+1], "()[]"))
		}
		// "Kettle Bay Complete", "Kettle Bay (Complete Series)", "Kettle Bay - The Complete Series".
		complete := strings.Trim(w, "()[]") == "complete" &&
			(next == "" || next == "series" || next == "season" || next == "seasons" || next == "collection" ||
				next == "box" || next == "boxset")
		// "Brine Sea (Season 1-3)" holds seasons 1 to 3.
		run, _, _ := strings.Cut(next, "-")
		bare := strings.Trim(w, "()[]")
		holds := seasonToken.MatchString(bare) ||
			((bare == "season" || bare == "seasons" || bare == "series" || bare == "staffel" || bare == "saison") && digits(run)) || complete ||
			(w == "the" && next == "complete")
		if holds {
			if show := strings.Trim(strings.Join(words[:i], " "), " -.("); show != "" {
				return show
			}
		}
	}
	// "Peppy Cow DVD", "Peppy Cow DVDs", "Peppy Cow Boxset", "Peppy Cow Box Set": the copy's words
	// after the name.
	if n := len(words); n > 2 && strings.EqualFold(words[n-2], "box") && strings.EqualFold(strings.Trim(words[n-1], "()[]-"), "set") {
		// "( Box Set" names nothing before it: it is the folder's name.
		if show := strings.Trim(strings.Join(words[:n-2], " "), " -.("); show != "" {
			words = words[:n-2]
			if !discWords[strings.ToLower(strings.Trim(words[len(words)-1], "()[]-"))] {
				return show
			}
		}
	}
	for n := len(words); n > 1; n-- {
		if !discWords[strings.ToLower(strings.Trim(words[n-1], "()[]-"))] {
			if n < len(words) {
				return strings.Trim(strings.Join(words[:n], " "), " -.(")
			}
			break
		}
	}
	return name
}

// discWords are the words a show's copy is named by after its name.
var discWords = map[string]bool{"dvd": true, "dvds": true, "bluray": true, "blu-ray": true, "boxset": true, "batch": true}

// ownSeries is the show an episode's own name gives: before its mark (looseSeries), or a dated
// release's before its day (datedShow).
func ownSeries(name string) string {
	if show := looseSeries(name); show != "" {
		return show
	}
	if airDateIn(name) != "" {
		return datedShow(plainStem(name))
	}
	return ""
}

// looseSeries is the show an episode with no series folder names before its episode mark:
// "Example.Show.S01E02.mkv" is "Example Show", so loose episodes of two shows are two shows.
// With nothing before the mark it is the root, "": a season folder never names a show.
func looseSeries(name string) string {
	if show, _, ok := looseAnime(name); ok {
		return show
	}
	mark := markIn(name)
	if mark == nil {
		mark = seasonEpisodeWords.FindStringIndex(name)
	}
	for _, re := range []*regexp.Regexp{dashedEpisode, dramaRelease, nightMark, cjkEpisode, sceneNumber} {
		if mark == nil {
			mark = re.FindStringIndex(name)
		}
	}
	if mark == nil {
		mark = partOfWord.FindStringIndex(name)
	}
	if mark == nil && countedPart(name) > 0 {
		mark = partOf.FindStringIndex(name)
	}
	if m := titledPart.FindStringSubmatchIndex(name); mark == nil && m != nil {
		mark = []int{m[3], m[3]}
	}
	if mark == nil && tvCapture.MatchString(name) {
		for _, re := range []*regexp.Regexp{capturedPart, partOf, bareEpisode} {
			if mark = re.FindStringIndex(name); mark != nil {
				break
			}
		}
	}
	if mark == nil {
		return ""
	}
	return showSpelling(strings.Trim(untagged(name[:mark[0]]), " ._-"))
}

// untagged is a show's name before its episode's mark with the release group's tags before it
// taken off: "[Grp] Hana Kettle", "【Grp】Kettle Hana". So is the site a download names itself
// after: "www.Torrentsite.com - Larch.Holm" (trackerPrefix).
func untagged(name string) string {
	if bare := trackerPrefix.ReplaceAllString(name, ""); strings.IndexFunc(bare, unicode.IsLetter) >= 0 {
		name = bare
	}
	// A tag with nothing after it is the show's name in brackets, no group's: "[Tolland Mews]" of
	// "[Tolland Mews] S01E09".
	bare := leadingTags.ReplaceAllString(name, "")
	if strings.IndexFunc(bare, unicode.IsLetter) < 0 {
		return strings.NewReplacer("[", "", "]", "", "【", "", "】", "").Replace(name)
	}
	name = bare
	if rest := cjkTag.ReplaceAllString(name, ""); strings.TrimSpace(rest) != "" {
		return rest
	}
	return name
}
