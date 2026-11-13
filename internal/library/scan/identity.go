package scan

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

var (
	// embeddedID matches an id written into a name in any bracket Jellyfin reads: "{tmdb-1234}",
	// "[imdbid-tt0001234]", "(tvdbid-5678)", "[tmdbid=1234]".
	embeddedID = regexp.MustCompile(`(?i)[\[{(](tmdb|tmdbid|imdb|imdbid|tvdb|tvdbid)[-=]([a-z0-9]+)[\]})]`)
	// episodeMark matches an episode's mark: "S01E02", Emby's "S01xE02", "S01Ep02", "S01 - E02",
	// Spanish "T01E02" (temporada), "1x02" and "1x2". The episodes after it in a file holding
	// several are moreEpisodes'.
	episodeMark = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:[st](\d{1,4})(?:[ ._]*-[ ._]*|[ ._-]?)x?e(?:p[ ._]?)?(\d{1,4})|(\d{1,2})x(\d{1,3})(?:[^0-9]|$))`)
	// moreEpisodes match each further episode of a file holding several, after its mark, in every
	// style the apps write (Sonarr's six, Plex's, Emby's): one joined on ("E03", "xE03", "x03"),
	// one after a dash, plus, "&" or "and" ("-E03", "+E03", " and E03", " - S01E03", "-x03",
	// " - 1x03", " 1x03"), or a bare number straight after a dash ("-03"). A bare number is two or three
	// digits and ends there, and is never a picture's height, so "S01E02-1080p", "S01E02-720p" and
	// "S01E02-720" hold one episode.
	moreEpisodesS = regexp.MustCompile(`(?i)^(?:[ ._]*(?:[-+&]|and)[ ._]*|[ ._]?)(?:s(\d{1,4})[ ._-]?)?x?e(\d{1,4})`)
	moreEpisodesX = regexp.MustCompile(`(?i)^(?:(?:[ ._]*[-+&][ ._]*|[ ._]+)(\d{1,2})x|-?x)(\d{2,3})(?:[^0-9]|$)`)
	moreBare      = regexp.MustCompile(`(?i)^-(\d{2,3})(?:[^0-9a-z]|$)`)
	// airDateMark matches the day a daily or weekly show's episode aired: "2013-02-15",
	// "2013.02.15", "2013 02 15", "2013_02_15", "20130215", and Plex's day first, "15-02-2013" and
	// "15.02.2013". The two separators are one character, the same both times; a month or day of
	// one digit ("2013-2-5") is read between dashes, dots or underscores only.
	airDateMark = regexp.MustCompile(`(?:^|[^0-9])(?:((?:19|20)\d{2})([-._ ]?)(\d{1,2})([-._ ]?)(\d{1,2})|(\d{2})([-.])(\d{2})([-.])((?:19|20)\d{2}))(?:[^0-9]|$)`)
)

// idsIn returns the grounded ids written into one name.
func idsIn(name string) []mp.ExternalID {
	var out []mp.ExternalID
	for _, m := range embeddedID.FindAllStringSubmatch(name, -1) {
		ns := mp.Namespace(strings.TrimSuffix(strings.ToLower(m[1]), "id"))
		value := strings.ToLower(m[2])
		if !validEmbedded(ns, value) {
			continue
		}
		out = append(out, mp.ExternalID{Namespace: ns, Value: value})
	}
	return out
}

func validEmbedded(ns mp.Namespace, value string) bool {
	if ns == mp.NamespaceIMDB {
		return strings.HasPrefix(value, "tt") && len(value) > 2 && digits(value[2:])
	}
	return digits(value) && strings.TrimLeft(value, "0") != ""
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// markIn is where an episode's mark is in a name, as episodeMark's submatch indexes: the first
// written in full ("S01E01") wherever it is, else the first "1x02". A title's "2x4" before a full
// mark is the title's: "Block 2x4 S01E01".
func markIn(name string) []int {
	var first []int
	// Each search starts again just after a "1x02"'s digits: the separator after them may start the
	// next mark.
	for at := 0; at < len(name); {
		m := episodeMark.FindStringSubmatchIndex(name[at:])
		if m == nil {
			break
		}
		for i := range m {
			if m[i] >= 0 {
				m[i] += at
			}
		}
		if m[2] >= 0 {
			return m
		}
		if first == nil {
			first = m
		}
		at = max(m[9], at+1)
	}
	return first
}

// shortCross says whether a name's mark is a "2x4", its episode one digit: a title's words as
// likely as a mark ("12 - 2x4"), so a season folder that says another season outranks it.
func shortCross(name string) bool {
	m := markIn(name)
	return m != nil && m[2] < 0 && m[9]-m[8] == 1
}

// numberingIn parses an episode's season and episode from its file name.
func numberingIn(name string) (mp.Numbering, bool) {
	m := markIn(name)
	if m == nil {
		return mp.Numbering{}, false
	}
	group := func(i int) string {
		if m[2*i] < 0 {
			return ""
		}
		return name[m[2*i]:m[2*i+1]]
	}
	more, n, at := moreEpisodesS, mp.Numbering{Season: atoi(group(1)), Episode: atoi(group(2))}, m[5]
	if m[2] < 0 {
		more, n, at = moreEpisodesX, mp.Numbering{Season: atoi(group(3)), Episode: atoi(group(4))}, m[9]
		// A name with its year in brackets after "4x4" is a film's title: "10x10 (2018)", "The 4x4
		// Club (2016)". A show's year comes before its episode's mark: "Show (2019) - 1x02".
		if bracketYear.MatchString(name[m[9]:]) {
			return mp.Numbering{}, false
		}
	}
	// Each further episode is the next one on, in the same season: a file runs through its last.
	last := n.Episode
	for {
		rest := name[at:]
		next, bare := more.FindStringSubmatchIndex(rest), false
		if next == nil {
			next, bare = moreBare.FindStringSubmatchIndex(rest), true
			if next != nil {
				next = []int{next[0], next[1], -1, -1, next[2], next[3]}
			}
		}
		if next == nil {
			break
		}
		season, episode := n.Season, atoi(rest[next[4]:next[5]])
		if bare && (episode == 480 || episode == 576 || episode == 720) {
			break
		}
		if next[2] >= 0 {
			season = atoi(rest[next[2]:next[3]])
		}
		if season != n.Season || episode <= last || codecLike(rest[:next[5]]) {
			break
		}
		last, at = episode, at+next[5]
	}
	if last > n.Episode {
		n.EpisodeEnd = last
	}
	return n, true
}

// codecLike says whether what follows an episode's mark is a codec, "x264" or "x265", not its next
// episode.
func codecLike(s string) bool {
	s = strings.ToLower(s)
	return strings.HasSuffix(s, "x264") || strings.HasSuffix(s, "x265")
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// embeddedRecord is what a file's names say: ids in the names and, for an episode, its numbering
// and the ids in its series folder. seriesDir is the episode's series folder relative to the root.
//
// A name is read in one order, the surest reading first, and the first that holds decides:
//
//  1. an episode's mark in the file's name, or its own folder's ("S01E02", "1x02": numberingIn);
//  2. the day a daily show's episode aired, where a show is plainly there (datedEpisode);
//  3. a mark its folders let it give ("Ep 2", "Folge 2", "05 - Title" in a season folder:
//     numberingInFolder);
//  4. a fansub release loose at the top (looseAnime);
//  5. any file in a show's season folder, or beside its marked episodes;
//  6. otherwise a movie.
//
// Which show an episode is, is seriesFolderOf's; what is no title at all (extras, samples, a
// NAS's folders) the walk decides before this (walk.go's skippedDir and extraName); a movie's or
// show's search words and year are searchQuery's (match.go).
func embeddedRecord(file File) (mp.Record, string) {
	file = regrouped(file)
	segments := strings.Split(file.Rel, "/")
	name := segments[len(segments)-1]
	src := mp.Source{Basis: mp.BasisEmbeddedID}
	// Among films ("Movies/", "Movies/Kids/", a film's own folder there) a file is a film whatever
	// its title says: "Season One (2012)", "Grid 4x4", "1. Hazelgrot" in a trilogy's folder. Only a
	// season's and episode's mark, "S01E01", or a season's folder in a show's says a file there is an
	// episode, and only one with a show to be: "Movies/S01E01 (2019).mkv" names none, and "Movies"
	// is no show's name.
	// So does a miniseries' folder there whose episodes say so by their word, and give no years
	// as a saga's films do: "Movies/Reed Wood/Reed Wood - Episode 1.mkv" beside "… Episode 2".
	// So does a season's pack there, "Movies/Lumen Bay Season 1/01.mkv", and a folder's video named by
	// its episode's word and number alone, "Movies/Fennick Bank/Episode 1.mkv": no film is called so.
	// So does an episode's word and its number padded as an episode's is, with no year: "Gannet
	// Hope Ep01"; a saga's "Kettle Wars Episode 4 (1977)" is a film.
	if dir := path.Dir(file.Rel); amongFilms(shelved(file, dir)) && !inShowsSeason(dir) && !miniseriesAmongFilms(file) &&
		!seasonAmongFilms(file) && (!paddedEpisode.MatchString(plainStem(name)) || yearAnywhere.MatchString(plainStem(name))) &&
		(!sxxeyy.MatchString(file.Rel) && (!crossMark.MatchString(name) || crossTitle(segments)) || shelfAt(seriesFolderOf(file))) {
		return movieRecord(src, segments), ""
	}
	// A camera's or phone's clip, "MVI_1001", "IMG_2041", "GOPR0032", is a video of its own: its
	// number counts the camera's shots, never a show's episodes.
	if cameraClip.MatchString(plainStem(name)) {
		return movieRecord(src, segments), ""
	}
	// A part of a whole set, "Grimwold CD1" beside "Grimwold CD2", is a film's wherever it is: a
	// shows' folder's parts are no set (walk.go), and "Part 1 of 3" is a miniseries' (partIn).
	if dir := path.Dir(file.Rel); file.Part > 0 && !sxxeyy.MatchString(name) && !inShowsSeason(dir) {
		return movieRecord(src, segments), ""
	}
	// A file named as its own folder is, with its year, a film: "Series 7 (2001)/Series 7
	// (2001).mkv", "Episode 50 (2011)/Episode 50 (2011).mkv", as Radarr and Plex keep one.
	// So is its folder's only video named by the folder's title alone, "TV/Sundial Run
	// (2017)/Sundial Run.mkv": the folder gives the film's year.
	if len(segments) > 1 && bracketYear.MatchString(segments[len(segments)-2]) && !sxxeyy.MatchString(name) {
		folder := segments[len(segments)-2]
		title, _, _ := searchQuery(folder, 0)
		if sameWords(plainStem(name), folder) ||
			file.Sole && !yearAnywhere.MatchString(plainStem(name)) && title != "" && sameWords(plainStem(name), title) {
			return movieRecord(src, segments), ""
		}
	}
	// A show's film kept with it says so, with its year and no episode's mark: "Kaito Run Movie 1 -
	// Red Sky (2010)", "Westerly Movie (2012)".
	if showFilm.MatchString(stem(name)) && bracketYear.MatchString(name) && markIn(name) == nil {
		return movieRecord(src, segments), ""
	}
	// So is an anime's film in its own release's folder, "[Grp] Kitsune Hana Movie [BD]/[Grp]
	// Kitsune Hana Movie.mkv": its name numbers no episode, and its folder is no show's.
	if dir := path.Dir(file.Rel); showFilm.MatchString(plainStem(name)) && markIn(name) == nil && inAnime(shelved(file, dir)) &&
		showFilm.MatchString(path.Base(dir)) && !file.Notes.Episodes {
		if _, _, ok := animeRelease(name, true); !ok {
			return movieRecord(src, segments), ""
		}
	}
	// So is a release's film, tagged by its group, in its show's folder: "[Grp] Kage Hana - Movie 1 -
	// Dawn [1080p]".
	if leadingTags.MatchString(name) && animeFilm.MatchString(stem(name)) && markIn(name) == nil {
		return movieRecord(src, segments), ""
	}
	// So is one with no year named as its show's folder is, then "Movie": "TV/Ivelmoor/Ivelmoor -
	// The Movie", "Anime/Kame Shiro/Kame Shiro Movie 2 - Red Dawn".
	if m := folderShowFilm.FindStringSubmatch(trimReleaseNotes(leadingTags.ReplaceAllString(plainStem(name), ""))); m != nil &&
		markIn(name) == nil && len(segments) > 1 && normName(m[1]) == normName(showFolderName(segments[len(segments)-2])) {
		return movieRecord(src, segments), ""
	}
	n, ok := numberingIn(name)
	if m := dramaRelease.FindStringSubmatch(name); !ok && m != nil {
		n, ok = mp.Numbering{Season: 1, Episode: atoi(m[1])}, true
	}
	// "12 - 2x4" in Season 1 is its twelfth, titled "2x4": its season folder names another season.
	if season := seasonDirOf(path.Dir(file.Rel)); ok && shortCross(name) && season != "" {
		if s, known := seasonIn(path.Base(season)); known && s != n.Season {
			ok = false
		}
	}
	if !ok && len(segments) > 1 {
		// A file with no mark of its own in an episode's folder ("S02E05/video.mkv") is that episode.
		n, ok = numberingIn(segments[len(segments)-2])
	}
	if ok {
		rec := mp.Record{Source: src, Kind: mp.KindEpisode, Numbering: &n}
		seriesDir := seriesFolderOf(file)
		if seriesDir != "" {
			rec.Parent = idsIn(path.Base(seriesDir))
		}
		return rec, seriesDir
	}
	// A daily or weekly show names its episodes by the day they aired, in the show's folder: the
	// day, not a season and number, says which episode it is (season.go's byAirDate). A dated file
	// loose at the root's top belongs to the show a release's name gives, and to none otherwise.
	if day := airDateOrdered(name, file.Notes.MonthFirst); day != "" && datedEpisode(file) {
		rec := mp.Record{Source: src, Kind: mp.KindEpisode, Descriptive: &mp.Descriptive{PremiereDate: day}}
		seriesDir := seriesFolderOf(file)
		rec.Parent = idsIn(path.Base(seriesDir))
		return rec, seriesDir
	}
	if n, ok := numberingInFolder(file); ok {
		seriesDir := seriesFolderOf(file)
		return mp.Record{Source: src, Kind: mp.KindEpisode, Numbering: &n, Parent: idsIn(path.Base(seriesDir))}, seriesDir
	}
	// An anime release loose at the top, "[Group] Show - 07 [1080p].mkv", is that show's episode by
	// its absolute number, or its season's.
	if show, n, ok := looseAnime(name); ok && len(segments) == 1 {
		return mp.Record{Source: src, Kind: mp.KindEpisode, Numbering: &n}, show
	}
	// A file with no mark in a show's season folder ("Kettle Bay/Specials/Holiday Special.mkv") is
	// one of its episodes: the catalog, or its own NFO, places it. A season folder with no show
	// folder above it names no show, so a file there is still a movie.
	if dir := path.Dir(file.Rel); dir != "." && isSeasonFolder(dir) {
		if seriesDir := aboveSeasons(dir); seriesDir != "." {
			return mp.Record{Source: src, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}, seriesDir
		}
	}
	// A disc ripped from a show's season, its label naming both, "KETTLE_BAY_S1_D1/title_t00.mkv",
	// holds that show's episodes; the catalog, or each title's length, places them.
	if dir := path.Dir(file.Rel); dir != "." && seasonDisc.MatchString(path.Base(dir)) {
		seriesDir := seriesFolderOf(file)
		return mp.Record{Source: src, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}, seriesDir
	}
	// So is one beside a show's marked episodes in the show's own folder ("Kettle Bay/Pilot.mkv"
	// beside "Kettle Bay S01E01.mkv"), or in a show's folder in a shows' catch-all ("TV/Puddle
	// Pals/Big Splash.mp4", "TV/Hollowmere/Season 2/Disc 1/title_t00.mkv"); a catch-all's or a
	// dump's are their own.
	// So is one named by the show and a title beside its episodes numbered after it: "Busy Bees -
	// Christmas Buzz (2015)" beside "Busy Bees 101".
	dir := path.Dir(file.Rel)
	special := file.Notes.Numbered && showTitled(name, path.Base(dir))
	// So is one of two or more loose in a catch-all that name one show before a dash: "Kids/Hob
	// Duck - Big Pond.mp4" beside "Kids/Hob Duck - Wet Feet.mp4".
	if dashedOf(file) != "" {
		seriesDir := seriesFolderOf(file)
		return mp.Record{Source: src, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}, seriesDir
	}
	// So is one in a season's pack, "Movies/Hob Saga Season 1/Hob Saga - Pilot.mkv".
	if dir != "." && packShow(dir) != "" && !namedInFull(file, dir) {
		seriesDir := seriesFolderOf(file)
		return mp.Record{Source: src, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}, seriesDir
	}
	if dir != "." && (file.Notes.Episodes || file.Notes.Series || file.Notes.Titled || special || underShows(shelved(file, dir))) &&
		!file.Notes.Dump && !shelfAt(dir) {
		seriesDir := seriesFolderOf(file)
		return mp.Record{Source: src, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}, seriesDir
	}
	return movieRecord(src, segments), ""
}

// seasonDisc matches a disc's label that names a show's season and the disc: "KETTLE_BAY_S1_D1",
// "Kettle Bay S2 Disc 3", "HOLLOW_FEN_SEASON_2_DISC_1".
var seasonDisc = regexp.MustCompile(`(?i)^(.+?)[ ._-]+(?:s|season[ ._-]*)\d{1,2}[ ._-]*d(?:isc|isk)?[ ._-]*\d{1,2}$`)

// filmsFolderNames are catch-alls that hold films.
var filmsFolderNames = map[string]bool{
	"movies": true, "films": true, "filme": true, "peliculas": true, "películas": true, "kids movies": true,
	"film": true, "my movies": true, "movies hd": true, "hd movies": true, "4k movies": true,
	"movies 4k": true, "uhd movies": true, "kids films": true, "family movies": true,
}

// sxxeyy matches an episode's season and episode mark written in full: "S01E01", "s1e2".
var sxxeyy = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s\d{1,4}[ ._-]?e\d{1,4}`)

// crossTitle says whether a film's name with a "1x01" in it, among films, is a title's: one named
// as its own folder, "Movies/Gridwell 12x12/Gridwell 12x12.mkv", or a square's size, "Hexlet
// 10x10", which numbers a show's episode no more often than it names a film. "Movies/Brook 1x02.mkv"
// is a show's episode a download left there.
func crossTitle(segments []string) bool {
	name := segments[len(segments)-1]
	if len(segments) > 1 && sameWords(plainStem(name), segments[len(segments)-2]) {
		return true
	}
	n, ok := numberingIn(name)
	return ok && n.Season > 1 && n.Season == n.Episode && n.EpisodeEnd == 0
}

// crossMark matches a season and episode written "1x01", with the episode in two digits or more: a
// film's "4x4" or "5x5" is its title.
var crossMark = regexp.MustCompile(`(?i)(?:^|[ ._-])\d{1,2}x\d{2,3}(?:[ ._-]|$)`)

// inShowsSeason says whether a folder is, or is in, a season folder with a show's folder above it:
// "Kettle Bay/Season 1/", not "Movies/Season One (2012)/".
func inShowsSeason(dir string) bool {
	season := seasonDirOf(dir)
	if season == "" {
		return false
	}
	show := aboveSeasons(season)
	return show != "." && !shelfAt(show)
}

// miniseriesAmongFilms says whether a file in a folder of its own among films is a miniseries'
// episode: its folder's videos number themselves by an episode's word after the name they share
// (folderNotes.Spelled), and it gives no year, as a saga's film does ("Kettle Wars Episode 4
// (1977)").
func miniseriesAmongFilms(file File) bool {
	_, year, _ := searchQuery(stem(path.Base(file.Rel)), 0)
	return file.Notes.Spelled && year == 0 && !shelfAt(path.Dir(file.Rel))
}

// cameraClip matches a camera's or phone's own name for a clip: "MVI_1001", "IMG_2041",
// "DSC01234", "GOPR0032", "GH010032", "VID_20230308_101500", "PXL_20230308_101500123".
var cameraClip = regexp.MustCompile(`(?i)^(?:mvi|img|dsc[fn]?|vid|mov|pxl|gopr|gh\d{2}|gx\d{2}|dji|mah|clip)[ _-]?\d{4,}(?:[ _-]\d+)*$`)

// seasonAmongFilms says whether a file in a folder of its own among films is a show's episode by
// the folder's structure: the folder is a season's pack ("Lumen Bay Season 1", "Fennick.Cove.S02"),
// or the file is named by an episode's word and number alone ("Episode 1", "Ep 02").
func seasonAmongFilms(file File) bool {
	dir := path.Dir(file.Rel)
	if dir == "." || shelfAt(dir) {
		return false
	}
	// "Movies/Episode 7/Episode 7.mkv" is a film named as its own folder.
	s := plainStem(path.Base(file.Rel))
	if sameWords(s, path.Base(dir)) {
		return false
	}
	return episodeOnly.MatchString(s) || cjkEpisodeOnly.MatchString(s) || packShow(dir) != ""
}

// episodeOnly matches a name that is an episode's word and number and nothing else: "Episode 1",
// "Ep.02", "Folge 3".
var episodeOnly = regexp.MustCompile(`(?i)^(?:episode|ep|e|folge|episodio|episódio|épisode|aflevering|avsnitt|afsnit|odcinek|cap[ií]tulo)[ ._-]*(\d{1,4})$`)

// paddedEpisode matches an episode's word and its number padded with a zero at a name's end, after
// the show's name: "Gannet Hope Ep01", "Gannet Hope - Episode 03".
var paddedEpisode = regexp.MustCompile(`(?i)\S[ ._-]+(?:ep|episode)[ ._]?0\d{1,2}$`)

// cjkEpisodeOnly matches a name that is a Japanese, Chinese or Korean episode's number alone:
// "第1話", "第05集", "第3回", "1화".
var cjkEpisodeOnly = regexp.MustCompile(`^(?:第\s*\d{1,4}\s*[話话集回]|\d{1,4}\s*화)$`)

// folderShowFilm matches a show's name, then its film's word and number, then its title after a
// dash or nothing: "Ivelmoor - The Movie", "Kame Shiro Movie 2 - Red Dawn".
var folderShowFilm = regexp.MustCompile(`(?i)^(.+?)[ ._:-]+(?:the[ ._]+)?(?:movie|film|gekijouban)(?:[ ._]*\d{1,2})?(?:[ ._]+-[ ._]+.*)?$`)

// showFilm matches a show's film named by the word after the show's name: "Kaito Run Movie 1",
// "Westerly Movie", "Westerly: The Movie", "Hana Kettle Gekijouban".
var showFilm = regexp.MustCompile(`(?i)^.+?[ ._:-]+(?:the[ ._]+)?(?:movie|film|gekijouban)(?:[ ._-]+\d{1,2})?(?:[ ._-]|$)`)

// movieRecord is a movie's embedded record, from the path's segments.
func movieRecord(src mp.Source, segments []string) mp.Record {
	name := segments[len(segments)-1]
	// A movie is named by its file or its own folder: the folder above a part's own ("CD1/"). A
	// folder further up is a collection or a series, and a tvdb id never names a movie.
	rec := mp.Record{Source: src, Kind: mp.KindMovie}
	if edition := editionIn(name); edition != "" {
		rec.Descriptive = &mp.Descriptive{Edition: edition}
	}
	depth := 2
	if len(segments) > 2 && partFolder.MatchString(segments[len(segments)-2]) {
		depth = 3
	}
	for i := len(segments) - 1; i >= 0 && i >= len(segments)-depth && len(rec.IDs) == 0; i-- {
		for _, id := range idsIn(segments[i]) {
			if id.Namespace != mp.NamespaceTVDB {
				rec.IDs = append(rec.IDs, id)
			}
		}
	}
	return rec
}

// showSpelling is how a show's name loose in a file's name is written as a show: its words apart,
// and, written all in lower case, each word capitalised, so "brass.reed.s01e02" and
// "Brass.Reed.S01E01" are one show, "Brass Reed". A name in capitals is an acronym's, "KBX".
func showSpelling(name string) string {
	name = strings.Trim(strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(name)), " "), " -")
	// "[Grp] Hana Kettle (TV) - 06" is "Hana Kettle"'s, as its folder would be (showFolderName).
	name = withoutShowLabels(name)
	words := strings.Fields(name)
	// A release's name in capitals, "HOB.LANE.S01E04", is its show's words shouted: each of two or
	// more is a word, three letters or longer. An acronym's, "KBX" or "CSI NY", stays.
	shouted := len(words) >= 2 && name == strings.ToUpper(name) && !slices.ContainsFunc(words, func(w string) bool {
		return utf8.RuneCountInString(w) < 3 || strings.ContainsFunc(w, func(r rune) bool { return !unicode.IsLetter(r) })
	})
	if shouted {
		name = strings.ToLower(name)
		words = strings.Fields(name)
	}
	if name != strings.ToLower(name) {
		return name
	}
	for i, w := range words {
		r, size := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[size:]
	}
	return strings.Join(words, " ")
}

var (
	// seasonEpisodeWords matches a mark written out: "Season 1 Episode 2", "Season 1 Ep 2",
	// "Staffel 1 Folge 2", "Temporada 1 Episodio 2".
	seasonEpisodeWords = regexp.MustCompile(`(?i)(?:^|[^\pL0-9])(?:` + seasonWords + `)[ ._-]*(\d{1,4})[ ._,-]*(?:episode|ep|folge|episodio|épisode|aflevering|avsnitt|afsnit|odcinek)[ ._-]*(\d{1,4})(?:[^0-9]|$)`)
	// episodeWord matches an episode with no season: "Ep02", "Episode 2", "Ep. 2", "Folge 2",
	// "Avsnitt 2", "Episodio 2", "Aflevering 2", "Capítulo 2", "Серия 2", "Bölüm 2".
	episodeWord = regexp.MustCompile(`(?i)(?:^|[^\pL0-9])(?:episode|ep|folge|episodio|episódio|épisode|aflevering|avsnitt|afsnit|odcinek|cap[ií]tulo|серия|эпизод|bölüm|jakso)[ ._-]*(\d{1,4})(?:[^0-9]|$)`)
	// dashedEpisode matches an episode's word and number after its show's name: "Spotty Dog -
	// Episode 3", "Spotty Dog - Ep 03", "Spotty Dog Episode 09", "Hana.Ame.Ep01"; "Ep" apart from
	// its number needs the dash.
	dashedEpisode = regexp.MustCompile(`(?i)[ ._]+(?:-[ ._]+(?:episode|ep\.?)[ ._]*|episode[ ._]*|ep\.?)(\d{1,4})(?:[^0-9]|$)`)
	// sceneNumber matches an old scene release's season and episode written as one number before a
	// TV capture's word: "larchmont.road.312.hdtv", "brindlemoor.207.pdtv". A film's "451.1080p"
	// is none.
	sceneNumber = regexp.MustCompile(`(?i)[ ._]([1-9])(\d{2})[ ._](?:hdtv|pdtv|dsr|tvrip|dvb)(?:[ ._-]|$)`)
	// tvCapture matches a word a release recorded from a broadcast carries: "HDTV", "PDTV", "DSR".
	tvCapture = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:hdtv|pdtv|dsr|dsrip|tvrip|dvbrip|satrip)(?:[ ._-]|$)`)
	// capturedPart matches a part's mark in a captured release: "Pt1", "Part.2", "Part 3".
	capturedPart = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:pt|part)[ ._-]?(\d{1,2})(?:[ ._-]|$)`)
	// capMark matches a Spanish release's "Cap.117": season 1, episode 17.
	capMark = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])cap[ ._]?(\d{1,2})(\d{2})(?:[^0-9]|$)`)
	// cjkEpisode matches a Japanese or Chinese episode's mark: "第11話", "第12集", "第3回"; or a
	// number before its word, as Korean, Russian and Turkish write it: "3화", "3 серия", "3. Bölüm".
	cjkEpisode = regexp.MustCompile(`(?i)第\s*(\d{1,4})\s*[話话集回]|(?:^|[^0-9])(\d{1,4})\.?[\s_]*(?:화|회|серия|bölüm)`)
	// partOfWord matches a miniseries' "Part 2 of 6" after its name, which says what it is wherever
	// it is: "Ashen Coast - Part 1 of 3".
	partOfWord = regexp.MustCompile(`(?i)[ ._-]+part[ ._-]*(\d{1,3})[ ._-]*of[ ._-]*(\d{1,3})(?:[^0-9a-z]|$)`)
	// partOf matches a miniseries' "2of6", "2 of 6", "Part 2 of 6".
	partOf = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,3})[ ._-]*of[ ._-]*(\d{1,3})(?:[^0-9a-z]|$)`)
	// leadingNumber matches a name that starts with its episode's number, bare, bracketed or after
	// a hash: "05 - Tidewater", "[01] Pilot", "(03) Found", "#04 Home".
	leadingNumber = regexp.MustCompile(`^[\[(#]?(\d{1,3})[\])]?(?:[ ._-]|$)`)
	// bracketYear matches a year in brackets.
	bracketYear = regexp.MustCompile(`[(\[](?:19|20)\d{2}[)\]]`)
	// seasonDashedEpisode matches a season and episode apart by a dash, "3 - 01", "2-03", read only
	// in that season's folder.
	seasonDashedEpisode = regexp.MustCompile(`(?:^|[ ._-])(\d{1,2})[ ._]*-[ ._]*(\d{2,3})(?:[ ._-]|$)`)
	// seasonDotEpisode matches a season and episode written "3.05".
	seasonDotEpisode = regexp.MustCompile(`(?:^|[ ._-])(\d{1,2})\.(\d{2})(?:[ ._-]|$)`)
	// bareEpisode matches an episode written "E03" with no season before it.
	bareEpisode = regexp.MustCompile(`(?i)(?:^|[ ._-])e(\d{1,4})(?:[ ._-]|$)`)
	// dramaRelease matches a Korean drama's release, its episode then the day it aired as YYMMDD:
	// "Kettle.Love.E05.240301.1080p"; or a web release's episode then its notes,
	// "Hoshi.no.Kaze.E11.1080p.WEB". It says an episode wherever it is.
	dramaRelease = regexp.MustCompile(`(?i)[ ._-]e(\d{2,4})[ ._-](?:\d{6}|\d{3,4}p|web(?:-?dl|rip)?|hdtv|x26[45]|hevc)(?:[ ._-]|$)`)
	// nightMark matches a miniseries' night after its name and a dash: "Ash Vale (2008) - Night 2".
	nightMark = regexp.MustCompile(`(?i)[ ._]+-[ ._]+night[ ._]*(\d{1,2})(?:[^0-9]|$)`)
	// spelledEpisode matches a name, then an episode's word and its number: "Rook Hall Mini E01",
	// "Reed Wood - Episode 1", "Ash Vale (2008) - Night 1". Its groups are the name and the number.
	// An "E" stands apart from the name and touches its number: "Ironvale 2" is a sequel's.
	spelledEpisode = regexp.MustCompile(`(?i)^(.*?[^ ._-])[ ._]+(?:-[ ._]+)?(?:(?:episode|ep\.?|night)[ ._]*|e)(\d{1,3})(?:[^0-9]|$)`)
	// seasonTwoDigits matches a two-digit number after a show's name, with or without a dash:
	// "Kettle Bay - 03", "Kettle Bay 06".
	seasonTwoDigits = regexp.MustCompile(`[ ._](?:-[ ._]*)?\d{2}(?:[ ._-]|$)`)
	// leadingTags are a release group's bracketed tags before a name: "[Kumo] ", "[Kumo][Show]",
	// "{Kumo} ", "(Kumo) ". A round bracket's tag is a word, never a number: "(500) Lanterns" is a
	// title.
	leadingTags = regexp.MustCompile(`^(?:[ ._]*(?:\[[^\]]*\]|\{[^}]*\}|\(\pL[^)\s]*\)))+[ ._]*`)
	// cjkTag matches a group's tag in CJK brackets before a name: "【Grp】Kettle Hana 第05集". Alone,
	// "【Lantern Cove】(2016)", the brackets hold the title.
	cjkTag = regexp.MustCompile(`^[ ._]*【[^】]*】[ ._]*`)
	// trailingGroup matches a release group's tag at a name's end: "Show - 06 [Kumo]".
	trailingGroup = regexp.MustCompile(`[ ._]*\[(\pL[^\]\s]*)\]$`)
	// ovaWord matches an OVA's or OAD's word, a special's whatever follows it.
	ovaWord = regexp.MustCompile(`(?i)[ ._-]o(?:va|ad)(?:[ ._\[(-]|$)`)
	// animeSpecial matches a fansub's special after its show's name, numbered on its own: " - SP01",
	// " - OVA 2", " - OAD1", " - Special 3", " - OVA".
	animeSpecial = regexp.MustCompile(`(?i)^(.+?)[ ._]+(?:-[ ._]+)?(?:sp|ova|oad|special)[ ._]*(\d{1,3})?(?:v\d+)?(?:[ ._]*$|[ ._]*[\[(]|[ ._]+-)`)
	// animeNumber matches an anime release's absolute episode after its show's name, " - 07", " -
	// 013v2", " - E07", " - Episode 7", " - 05.5" (a recap after episode 5) or " - 01-03" (from the
	// first), ahead of its notes or its end; or with no dash, " 07" with two digits or more.
	animeNumber = regexp.MustCompile(`(?i)^(.+?)(?:[ ._]+-[ ._]+(?:(?:e|ep|episode)[ ._]*)?(\d{1,4})|[ ._]+(\d{2,4}))(?:\.5)?(?:-\d{1,4})?(?:v\d+)?(?:[ ._]+(?:end|final))?(?:[ ._]*$|[ ._]*[\[(]|[ ._]+-)`)
	// animeDashed is animeNumber's dashed form alone, read first so a number in a show's name stays
	// the show's: "Hoshizora 86 - 04" is episode 4 of "Hoshizora 86". "- #09" numbers one too.
	animeDashed = regexp.MustCompile(`(?i)^(.+?)[ ._]+-[ ._]+(?:(?:e|ep|episode)[ ._]*|#)?(\d{1,4})()(?:\.5)?(?:-\d{1,4})?(?:v\d+)?(?:[ ._]+(?:end|final))?(?:[ ._]*$|[ ._]*[\[(]|[ ._]+-)`)
	// paddedAbsolute matches an episode's number across its show after a dash, at the end of a name,
	// written with a leading zero or in three digits: "Show - 08", "Show - 013v2".
	paddedAbsolute = regexp.MustCompile(`[ ._]-[ ._]+(?:0\d{1,3}|[1-9]\d{2})(?:v\d+)?$`)
	// dashedLast matches a dashed number at the end of a name: "Show - 7", "Show - 07v2".
	dashedLast = regexp.MustCompile(`[ ._]-[ ._]+\d{1,4}(?:v\d+)?$`)
	// releaseNotes matches a release's bracketed notes at the end of its name: "[1080p]", "(BD
	// 1080p x265)", "[WEB][AAC]".
	releaseNotes = regexp.MustCompile(`(?i)[\[(][^\])]*(?:\d{3,4}p|bd|web|hevc|x26[45]|aac|flac|bluray|dvd|hdtv)[^\])]*[\])][ ._]*$`)
	// animeBrackets matches a release that brackets its number after its group's tag and its show,
	// the show bracketed or not: "[Kumo][Show][13][1080p]", "[Kumo] Show [06] [1080p]".
	animeBrackets = regexp.MustCompile(`^\[[^\]]*\][ ._]*(?:\[([^\]]+)\]|([^\[\]]*\pL[^\[\]]*?))[ ._]*\[(\d{1,4})(?:v\d+)?\]`)
)

// capturedEpisode is the episode a TV capture's name numbers by its part ("Pt1", "Part 2"), its
// place in a run ("3of6") or its episode alone ("E01").
func capturedEpisode(name string) (int, bool) {
	if !tvCapture.MatchString(name) {
		return 0, false
	}
	if m := capturedPart.FindStringSubmatch(name); m != nil && atoi(m[1]) > 0 {
		return atoi(m[1]), true
	}
	if m := partOf.FindStringSubmatch(name); m != nil && atoi(m[1]) > 0 && atoi(m[1]) <= atoi(m[2]) {
		return atoi(m[1]), true
	}
	if m := bareEpisode.FindStringSubmatch(name); m != nil && atoi(m[1]) > 0 {
		return atoi(m[1]), true
	}
	return 0, false
}

// plainStem is a file's name without its extension and the release's notes at its end:
// "Kettle Bay - Part One [1080p] (WEB).mkv" is "Kettle Bay - Part One".
func plainStem(name string) string { return trimReleaseNotes(stem(name)) }

// trimReleaseNotes is a name without the release's bracketed notes at its end.
func trimReleaseNotes(s string) string {
	for {
		loc := releaseNotes.FindStringIndex(s)
		if loc == nil || loc[0] == 0 {
			return s
		}
		s = strings.TrimRight(s[:loc[0]], " ._")
	}
}

// stem is a file's name without its extension.
func stem(name string) string {
	if dot := strings.LastIndex(name, "."); dot > 0 {
		return name[:dot]
	}
	return name
}
