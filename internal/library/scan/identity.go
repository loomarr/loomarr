package scan

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Manager is one arr that knows which files are which items: Sonarr or Radarr. A full pass lists it
// whole; a folder pass reuses that list, reading again only an item the arr said changed.
// Authority is its name in the path map.
type Manager struct {
	Authority string
	Lister    mp.Lister
	// Fetcher reads one series (by TVDB id) or movie (by TMDB id); nil lists the arr again.
	Fetcher Fetcher
	// Address is where the arr is now, "" when not connected: a listing made at another address
	// is not reused. Nil never changes.
	Address func() string
}

// Fetcher reads one item of an arr.
type Fetcher interface {
	Fetch(ctx context.Context, ref mp.Ref) (mp.Record, error)
}

// ArrItem is an item an arr said changed: a series by TVDB id or a movie by TMDB id.
type ArrItem struct {
	Authority string
	Ref       mp.Ref
}

// managed is what one arr says one local file is. Records holds one record per episode in the
// file, lowest first; Series is the episode's series. Owner is the arr item it came from.
type managed struct {
	records []mp.Record
	series  *mp.Record
	owner   string
}

// managerIndex maps a local path to what an arr says it is.
type managerIndex struct {
	byPath map[string]managed
	// complete is false when any configured arr failed to list, so the absence of a file from the
	// index proves nothing.
	complete bool
	// unlisted are the local folders of the arrs that failed to list: only what they hold is
	// unknown.
	unlisted []string
}

// knows reports whether the index can say what path is: every arr listed, or none that failed
// manages a folder holding it.
func (idx managerIndex) knows(path string) bool {
	if idx.complete {
		return true
	}
	return !under(path, idx.unlisted)
}

// indexManagers lists every arr and keys its files by local path through the path map.
func indexManagers(ctx context.Context, managers []Manager, paths []librarystore.LibraryRootPath) (managerIndex, []error) {
	idx := managerIndex{byPath: map[string]managed{}, complete: true}
	var errs []error
	for _, m := range managers {
		records, err := m.Lister.List(ctx)
		if errors.Is(err, mp.ErrNotConfigured) {
			continue // an arr with no connection manages nothing
		}
		if err != nil {
			idx.complete = false
			errs = append(errs, err)
			for _, p := range paths {
				if p.Authority == m.Authority && p.RootPath != "" {
					idx.unlisted = append(idx.unlisted, p.RootPath)
				}
			}
		}
		local := pathMapper(m.Authority, paths)
		for i := range records {
			idx.add(m.Authority, local, records[i])
		}
	}
	return idx, errs
}

// add indexes one arr item's files: a movie's, or each episode's of a series.
func (idx managerIndex) add(authority string, local func(string) string, rec mp.Record) {
	owner := ownerKey(authority, rec)
	switch rec.Kind {
	case mp.KindMovie:
		idx.addFiles(local, rec.Files, rec, nil, owner)
	case mp.KindSeries:
		series := rec
		series.Children = nil
		for _, ep := range rec.Children {
			idx.addFiles(local, ep.Files, ep, &series, owner)
		}
	}
}

// refresh replaces what the index holds for one arr item with rec; a nil rec drops it, for an item
// the arr no longer has.
func (idx managerIndex) refresh(authority string, local func(string) string, ref mp.Ref, rec *mp.Record) {
	owner := ownerKey(authority, mp.Record{Kind: ref.Kind, IDs: []mp.ExternalID{ref.ID}})
	for p, m := range idx.byPath {
		if m.owner == owner {
			delete(idx.byPath, p)
		}
	}
	if rec != nil {
		idx.add(authority, local, *rec)
	}
}

// ownerKey names an arr item by the id the arr fetches it by: a series' TVDB id, a movie's TMDB id.
func ownerKey(authority string, rec mp.Record) string {
	ns := mp.NamespaceTMDB
	if rec.Kind == mp.KindSeries {
		ns = mp.NamespaceTVDB
	}
	for _, id := range rec.IDs {
		if id.Namespace == ns {
			return authority + "|" + string(ns) + ":" + id.Value
		}
	}
	return ""
}

func (idx managerIndex) addFiles(local func(string) string, files []mp.FileRef, rec mp.Record, series *mp.Record, owner string) {
	for _, f := range files {
		p := local(f.Path)
		if p == "" {
			continue
		}
		m := idx.byPath[p]
		m.records = append(m.records, rec)
		sort.SliceStable(m.records, func(i, j int) bool { return episodeOrder(m.records[i]) < episodeOrder(m.records[j]) })
		if series != nil {
			m.series = series
		}
		m.owner = owner
		idx.byPath[p] = m
	}
}

func episodeOrder(rec mp.Record) int {
	if rec.Numbering == nil {
		return 0
	}
	return rec.Numbering.Season*100000 + rec.Numbering.Episode
}

// pathMapper translates one authority's paths to local ones by the longest matching path-map row.
// A path no row covers translates to "".
func pathMapper(authority string, paths []librarystore.LibraryRootPath) func(string) string {
	type row struct {
		from, to string
		windows  bool
	}
	var rows []row
	for _, p := range paths {
		if p.Authority == authority && p.RootPath != "" {
			from, windows := arrSlashes(p.AuthorityPath)
			rows = append(rows, row{from: strings.TrimSuffix(from, "/"), to: p.RootPath, windows: windows})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return len(rows[i].from) > len(rows[j].from) })
	return func(arrPath string) string {
		p, windows := arrSlashes(arrPath)
		for _, r := range rows {
			// A Windows path names the same folder in any case: D:\Movies is d:\movies.
			key, from := p, r.from
			if windows || r.windows {
				key, from = strings.ToLower(key), strings.ToLower(from)
			}
			if key == from {
				return r.to
			}
			if strings.HasPrefix(key, from+"/") {
				return filepath.Join(r.to, filepath.FromSlash(p[len(from)+1:]))
			}
		}
		return ""
	}
}

// windowsPath matches a path as an app on Windows writes it: a drive ("D:\", "D:/") or a share
// ("\\nas\media").
var windowsPath = regexp.MustCompile(`^(?:[A-Za-z]:[\\/]|\\\\)`)

// arrSlashes is an arr's path with forward slashes, cleaned, and whether it is a Windows path. An
// arr on Windows writes backslashes, which are a file name's own characters on Linux, so they are
// turned only in a Windows path.
func arrSlashes(p string) (string, bool) {
	windows := windowsPath.MatchString(p)
	if windows {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	return path.Clean(p), windows
}

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
	// " - 1x03"), or a bare number straight after a dash ("-03"). A bare number is two or three
	// digits and ends there, and is never a picture's height, so "S01E02-1080p", "S01E02-720p" and
	// "S01E02-720" hold one episode.
	moreEpisodesS = regexp.MustCompile(`(?i)^(?:[ ._]*(?:[-+&]|and)[ ._]*|[ ._]?)(?:s(\d{1,4})[ ._-]?)?x?e(\d{1,4})`)
	moreEpisodesX = regexp.MustCompile(`(?i)^(?:[ ._]*[-+&][ ._]*(\d{1,2})x|-?x)(\d{2,3})(?:[^0-9]|$)`)
	moreBare      = regexp.MustCompile(`(?i)^-(\d{2,3})(?:[^0-9a-z]|$)`)
	// airDateMark matches the day a daily or weekly show's episode aired: "2013-02-15",
	// "2013.02.15", "2013 02 15", "2013_02_15", "20130215", and Plex's day first, "15-02-2013" and
	// "15.02.2013". The two separators are one character, the same both times; a month or day of
	// one digit ("2013-2-5") is read between dashes, dots or underscores only.
	airDateMark = regexp.MustCompile(`(?:^|[^0-9])(?:((?:19|20)\d{2})([-._ ]?)(\d{1,2})([-._ ]?)(\d{1,2})|(\d{2})([-.])(\d{2})([-.])((?:19|20)\d{2}))(?:[^0-9]|$)`)
)

// airDateIn is the day a file's name says its episode aired, YYYY-MM-DD; "" for none. A day that
// never was, such as "2024-02-30" or a number that only looks like one, is none.
func airDateIn(name string) string {
	pad := func(s string) string {
		if len(s) == 1 {
			return "0" + s
		}
		return s
	}
	for _, m := range airDateMark.FindAllStringSubmatch(name, -1) {
		day, same := m[1]+"-"+pad(m[3])+"-"+pad(m[5]), m[2] == m[4]
		if m[1] != "" && (len(m[3]) == 1 || len(m[5]) == 1) && (m[2] == "" || m[2] == " ") {
			continue
		}
		if m[1] == "" {
			day, same = m[10]+"-"+m[8]+"-"+m[6], m[7] == m[9]
			// Day first as Plex writes it; a month that can't be one is America's month first,
			// "03-14-2024".
			if atoi(m[8]) > 12 {
				day = m[10] + "-" + m[6] + "-" + m[8]
			}
		}
		if _, err := time.Parse(time.DateOnly, day); err == nil && same {
			return day
		}
	}
	return ""
}

// datedEpisode says whether a file its name dates is a show's episode: one in a season folder, or
// one whose name before its day is its show's ("Kettle Bay/Kettle Bay 2024-03-14.mkv"). A day alone
// does not make a show: "Home Videos/2019-06-01 Beach Day.mp4" is a day's video, and
// "Concerts/Ferrow Band - Live 2019.10.05.mkv" a film of one.
func datedEpisode(file File) bool {
	if seasonDirOf(path.Dir(file.Rel)) != "" {
		return true
	}
	name, dir := plainStem(path.Base(file.Rel)), path.Dir(file.Rel)
	before, _, _ := searchQuery(datedShow(name), 0)
	// A name that is its day alone, "Nightly Ledger/2024-03-14.mkv", is the episode of the show
	// whose folder it is in; a catch-all's, "Home Videos/2019-06-01.mp4", is a day's video.
	if before == "" && dir != "." && airDateMark.ReplaceAllString(name, "") == "" &&
		(underShows(dir) || file.Notes.Episodes) && !catchAllFolder(path.Base(dir)) {
		return true
	}
	if before == "" || cameraPrefixes[strings.ToLower(before)] {
		return false
	}
	if recording.MatchString(name) || recordedAt.MatchString(name) {
		// A recorded film is named as a guide lists one, with its year in brackets: "Iron Spire
		// (2011)_Film4_2022_05_01_21_00_00".
		return !bracketYear.MatchString(datedShow(name))
	}
	// Loose at the top or in a catch-all ("Downloads/"), a release's name is its own show's:
	// "Evening.Ledger.2024.03.18.720p.WEB"; in a shows' catch-all ("Recordings/", "TV/") any dated
	// name is: "Harbour Watch 2022-01-05 21-00".
	if dir == "." || catchAllFolder(path.Base(dir)) {
		return showsFolderNames[folderKey(path.Base(dir))] || datedRelease(name)
	}
	show, _, _ := searchQuery(showFolderName(path.Base(seriesFolderOf(file))), 0)
	return relatedName(before, show)
}

// recording matches a TV recorder's name for what it recorded, Windows Media Center's: its show,
// its channel, and the day and time it began, "Larchmont Road_ITV_2022_11_02_20_00_00". Each is
// that show's episode of that day.
var recording = regexp.MustCompile(`^(.+?)_[A-Za-z0-9 +]+_((?:19|20)\d{2})_\d{2}_\d{2}_\d{2}_\d{2}_\d{2}$`)

// recordedAt matches a DVR's name for what it recorded, with the day and the time it began after
// the show and the episode's title: "Wexcombe - Hedge (2023-01-02 21-00)". A film's name never
// carries the minute it began.
var recordedAt = regexp.MustCompile(`^(.+?)(?: - .+?)? \((?:19|20)\d{2}-\d{2}-\d{2}[ _T]\d{2}[-.:]\d{2}(?:[-.:]\d{2})?\)$`)

// cameraPrefixes are what a phone or camera names its videos by before their day: "VID_20190601_
// 123456", "PXL_20230704_…". A home video, never a show's episode.
var cameraPrefixes = map[string]bool{
	"vid": true, "img": true, "pxl": true, "mvi": true, "dsc": true, "mov": true, "dji": true,
	"gopr": true, "video": true, "screen recording": true, "signal": true, "whatsapp video": true,
}

// datedRelease says whether a dated name is a release's, written as a downloader leaves it: with
// dots for spaces, or with a release's words after its day.
func datedRelease(name string) bool {
	if !strings.Contains(name, " ") {
		return true
	}
	return slices.ContainsFunc(strings.FieldsFunc(name, func(r rune) bool { return strings.ContainsRune(" ._[]()-", r) }), releaseMark.MatchString)
}

// datedShow is the show a dated name names before its day: "Evening.Ledger.2024.03.18.720p" is
// "Evening Ledger"; "" for none.
func datedShow(name string) string {
	if m := recording.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1])
	}
	if m := recordedAt.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1])
	}
	m := airDateMark.FindStringSubmatchIndex(name)
	if m == nil {
		return ""
	}
	start := m[2] // the year first, else the day first
	if start < 0 {
		start = m[12]
	}
	show := strings.NewReplacer(".", " ", "_", " ").Replace(leadingTags.ReplaceAllString(name[:start], ""))
	return strings.Trim(strings.Join(strings.Fields(show), " "), " -")
}

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

// numberingIn parses an episode's season and episode from its file name.
func numberingIn(name string) (mp.Numbering, bool) {
	m := episodeMark.FindStringSubmatchIndex(name)
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
	segments := strings.Split(file.Rel, "/")
	name := segments[len(segments)-1]
	src := mp.Source{Basis: mp.BasisEmbeddedID}
	// Among films ("Movies/", "Movies/Kids/", a film's own folder there) a file is a film whatever
	// its title says: "Season One (2012)", "Grid 4x4", "1. Hazelgrot" in a trilogy's folder. Only a
	// season's and episode's mark, "S01E01", or a season's folder in a show's says a file there is an
	// episode, and only one with a show to be: "Movies/S01E01 (2019).mkv" names none, and "Movies"
	// is no show's name.
	if dir := path.Dir(file.Rel); amongFilms(dir) && !inShowsSeason(dir) &&
		(!sxxeyy.MatchString(file.Rel) && !crossMark.MatchString(name) || catchAllFolder(path.Base(seriesFolderOf(file)))) {
		return movieRecord(src, segments), ""
	}
	// A part of a whole set, "Grimwold CD1" beside "Grimwold CD2", is a film's wherever it is: a
	// shows' folder's parts are no set (walk.go), and "Part 1 of 3" is a miniseries' (partIn).
	if dir := path.Dir(file.Rel); file.Part > 0 && !sxxeyy.MatchString(name) && !inShowsSeason(dir) {
		return movieRecord(src, segments), ""
	}
	// A file named as its own folder is, with its year, a film: "Series 7 (2001)/Series 7
	// (2001).mkv", "Episode 50 (2011)/Episode 50 (2011).mkv", as Radarr and Plex keep one.
	if len(segments) > 1 && bracketYear.MatchString(segments[len(segments)-2]) && !sxxeyy.MatchString(name) &&
		sameWords(plainStem(name), segments[len(segments)-2]) {
		return movieRecord(src, segments), ""
	}
	n, ok := numberingIn(name)
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
	if day := airDateIn(name); day != "" && datedEpisode(file) {
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
	if dir := path.Dir(file.Rel); dir != "." && (file.Notes.Episodes || file.Notes.Series || underShows(dir)) && !file.Notes.Dump && !catchAllFolder(path.Base(dir)) {
		seriesDir := seriesFolderOf(file)
		return mp.Record{Source: src, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}, seriesDir
	}
	return movieRecord(src, segments), ""
}

// seasonDisc matches a disc's label that names a show's season and the disc: "KETTLE_BAY_S1_D1",
// "Kettle Bay S2 Disc 3".
var seasonDisc = regexp.MustCompile(`(?i)^.+[ ._-]s\d{1,2}[ ._-]*d(?:isc|isk)?[ ._-]*\d{1,2}$`)

// filmsFolderNames are catch-alls that hold films.
var filmsFolderNames = map[string]bool{
	"movies": true, "films": true, "filme": true, "peliculas": true, "películas": true, "kids movies": true,
	"film": true, "my movies": true, "movies hd": true, "hd movies": true, "4k movies": true,
	"movies 4k": true, "uhd movies": true, "kids films": true, "family movies": true,
}

// sxxeyy matches an episode's season and episode mark written in full: "S01E01", "s1e2".
var sxxeyy = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s\d{1,4}[ ._-]?e\d{1,4}`)

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
	return show != "." && !catchAllFolder(path.Base(show))
}

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

var (
	// seasonFolder matches a season folder's name and its number: "Season 1", "Season 01", "Series
	// One", "Staffel 1", "Säsong 1", with its notes; "S01" with only bracketed notes.
	seasonFolder = regexp.MustCompile(`(?i)^(?:(?:` + seasonWords + `)[ ._-]*(\d{1,4}|[a-z]+)` + seasonNotes + `|s(\d{1,4})(?:[ ._-]*[\[(][^\])]*[\])])*)$`)
	// namedSeason matches a season folder that starts with its show's name: "Kettle Bay Season 1",
	// "KB Season 1".
	namedSeason = regexp.MustCompile(`(?i)^(.+?)[ ._-]+(?:(?:` + seasonWords + `)[ ._-]*(\d{1,4}|[a-z]+)` + seasonNotes + `|s(\d{1,2}))$`)
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
	switch strings.ToLower(name) {
	case "specials", "special", "season specials", "season 00 specials", "extras specials":
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
		return 0, false
	}
	if m[2] != "" {
		return atoi(m[2]), true
	}
	return seasonNumberWord(m[1])
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
			return path.Join(dir, folder)
		}
	}
	return ""
}

// seriesFolderOf is seriesFolder for a file the walk found.
func seriesFolderOf(file File) string {
	dir, name := path.Dir(file.Rel), path.Base(file.Rel)
	// A folder named by an episode's mark is the episode's own (Emby's "Season 2/S02E05/", a
	// release's "Kettle.Bay.S01E02.720p-GRP/"): its show is above it, and with none above, it names
	// the show as a loose episode's name does.
	for dir != "." && episodeMark.MatchString(path.Base(dir)) {
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
	if show == "" && inAnime(dir) {
		show, _, _ = animeRelease(name, true)
	}
	// A folder's files numbered after a name they share, "Cartoons/Fizzy Fox 01.mp4" beside "… 02",
	// are that show's.
	if show == "" && file.Notes.Numbered && dir == path.Dir(file.Rel) {
		show = numberedShow(name)
	}
	if (dir == "." || catchAllFolder(path.Base(dir)) || file.Notes.Dump && dir == path.Dir(file.Rel)) && show != "" {
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
			if !catchAllFolder(path.Base(d)) && !isSeasonFolder(d) && relatedName(show, showFolderName(path.Base(d))) {
				return d
			}
		}
		return show
	}
	if dir == "." {
		return show
	}
	if season := seasonDirOf(dir); season != "" {
		show := aboveSeasons(season)
		if show != "." && !catchAllFolder(path.Base(show)) {
			return showFolderAbove(show)
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
		return looseSeries(name)
	}
	// A year's folder in a show's ("Evening Ledger/2024/") holds that year's episodes, as a season's
	// does.
	if yearToken.MatchString(path.Base(dir)) && path.Dir(dir) != "." && !catchAllFolder(path.Base(path.Dir(dir))) {
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
	base := path.Base(dir)
	if words := strings.FieldsFunc(base, func(r rune) bool { return strings.ContainsRune(" ._", r) }); dir != "." && len(words) > 1 &&
		!seasonToken.MatchString(words[0]) && (slices.ContainsFunc(words[1:], seasonToken.MatchString) || namedSeasonOf(base) > 0) {
		if show := showFolderName(base); show != base && show != "" && !isSeasonFolder(path.Join(path.Dir(dir), show)) {
			return path.Join(path.Dir(dir), show)
		}
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
		if catchAllFolder(above) || isSeasonFolder(path.Dir(dir)) {
			break
		}
		release := showFolderName(base) != base && relatedName(showFolderName(base), showFolderName(above))
		if !release && !copyOfShow(base, showFolderName(above)) && !holdingOnly(strings.Fields(normName(base))) {
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
	`|dvd|disc|disk|vol|volume|box|boxset|set|pack|full|all|episodes|bluray|web|webdl|webrip|hdtv|remux|\d{3,4}p|x26[45]|h26[45]|hevc|uhd|4k|sd|hd)$`)

// seasonToken is a word in a release folder's name that starts what it holds of a show: "S02",
// "S01-S03".
var seasonToken = regexp.MustCompile(`(?i)^s\d{1,2}(?:-s?\d{1,2})?$`)

// showYear matches the year an episode's file name gives its show, in brackets just before the
// episode's mark: "Show (1963) - S01E01", "Show (1963) - S01xE01", "Show.[1963].1x01".
var showYear = regexp.MustCompile(`[(\[]((?:18|19|20)\d\d)[)\]][\s._-]*(?:[Ss]\d{1,4}[xX]?[Ee]\d|\d{1,2}[xX]\d)`)

// episodeRange matches a fansub pack's run of episodes, bracketed or after a dash: "(01-12)",
// "[01~26]", " - 01-12 [Batch]".
var episodeRange = regexp.MustCompile(`[(\[]\s*\d{1,4}\s*[-~]\s*\d{1,4}\s*[)\]]|[ ._]-[ ._]+\d{1,4}[-~]\d{1,4}(?:[ ._\[(]|$)`)

// kindTag matches what a show's folder says it is, in brackets after its name: "(TV Series)",
// "(Miniseries)", "(Mini-Series)", "[TV]", "(TV Show)".
var kindTag = regexp.MustCompile(`(?i)[ ._]*[(\[](?:tv|tv[ ._-]?series|tv[ ._-]?show|(?:tv[ ._-]?)?mini[ ._-]?series|series|show)[)\]][ ._]*$`)

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
	// "Copper Vale (TV Series)" is "Copper Vale"; "(TV)" alone is its own name.
	if bare := strings.TrimSpace(kindTag.ReplaceAllString(name, "")); bare != "" {
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
			(next == "" || next == "series" || next == "season" || next == "seasons" || next == "collection")
		// "Brine Sea (Season 1-3)" holds seasons 1 to 3.
		run, _, _ := strings.Cut(next, "-")
		bare := strings.Trim(w, "()[]")
		holds := seasonToken.MatchString(w) ||
			((bare == "season" || bare == "seasons" || bare == "series" || bare == "staffel" || bare == "saison") && digits(run)) || complete ||
			(w == "the" && next == "complete")
		if holds {
			if show := strings.Trim(strings.Join(words[:i], " "), " -.("); show != "" {
				return show
			}
		}
	}
	return name
}

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
	mark := episodeMark.FindStringIndex(name)
	if mark == nil {
		mark = seasonEpisodeWords.FindStringIndex(name)
	}
	if mark == nil {
		mark = dashedEpisode.FindStringIndex(name)
	}
	if mark == nil {
		mark = sceneNumber.FindStringIndex(name)
	}
	if mark == nil {
		mark = partOfWord.FindStringIndex(name)
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
	return showSpelling(strings.Trim(leadingTags.ReplaceAllString(name[:mark[0]], ""), " ._-"))
}

// showSpelling is how a show's name loose in a file's name is written as a show: its words apart,
// and, written all in lower case, each word capitalised, so "brass.reed.s01e02" and
// "Brass.Reed.S01E01" are one show, "Brass Reed". A name in capitals is an acronym's, "KBX".
func showSpelling(name string) string {
	name = strings.Trim(strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(name)), " "), " -")
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
	// leadingNumber matches a name that starts with its episode's number: "05 - Tidewater".
	leadingNumber = regexp.MustCompile(`^(\d{1,3})(?:[ ._-]|$)`)
	// bracketYear matches a year in brackets.
	bracketYear = regexp.MustCompile(`[(\[](?:19|20)\d{2}[)\]]`)
	// seasonDashedEpisode matches a season and episode apart by a dash, "3 - 01", "2-03", read only
	// in that season's folder.
	seasonDashedEpisode = regexp.MustCompile(`(?:^|[ ._-])(\d{1,2})[ ._]*-[ ._]*(\d{2,3})(?:[ ._-]|$)`)
	// seasonDotEpisode matches a season and episode written "3.05".
	seasonDotEpisode = regexp.MustCompile(`(?:^|[ ._-])(\d{1,2})\.(\d{2})(?:[ ._-]|$)`)
	// bareEpisode matches an episode written "E03" with no season before it.
	bareEpisode = regexp.MustCompile(`(?i)(?:^|[ ._-])e(\d{1,4})(?:[ ._-]|$)`)
	// seasonTwoDigits matches a two-digit number after a show's name, with or without a dash:
	// "Kettle Bay - 03", "Kettle Bay 06".
	seasonTwoDigits = regexp.MustCompile(`[ ._](?:-[ ._]*)?\d{2}(?:[ ._-]|$)`)
	// leadingTags are a release group's bracketed tags before a name: "[Kumo] ", "[Kumo][Show]",
	// "{Kumo} ", "(Kumo) ". A round bracket's tag is a word, never a number: "(500) Lanterns" is a
	// title.
	leadingTags = regexp.MustCompile(`^(?:[ ._]*(?:\[[^\]]*\]|\{[^}]*\}|\(\pL[^)\s]*\)))+[ ._]*`)
	// trailingGroup matches a release group's tag at a name's end: "Show - 06 [Kumo]".
	trailingGroup = regexp.MustCompile(`[ ._]*\[(\pL[^\]\s]*)\]$`)
	// animeSpecial matches a fansub's special after its show's name, numbered on its own: " - SP01",
	// " - OVA 2", " - OAD1", " - Special 3".
	animeSpecial = regexp.MustCompile(`(?i)^(.+?)[ ._]+(?:-[ ._]+)?(?:sp|ova|oad|special)[ ._]*(\d{1,3})(?:v\d+)?(?:[ ._]*$|[ ._]*[\[(]|[ ._]+-)`)
	// animeNumber matches an anime release's absolute episode after its show's name, " - 07", " -
	// 013v2", " - E07", " - Episode 7", " - 05.5" (a recap after episode 5) or " - 01-03" (from the
	// first), ahead of its notes or its end; or with no dash, " 07" with two digits or more.
	animeNumber = regexp.MustCompile(`(?i)^(.+?)(?:[ ._]+-[ ._]+(?:(?:e|ep|episode)[ ._]*)?(\d{1,4})|[ ._]+(\d{2,4}))(?:\.5)?(?:-\d{1,4})?(?:v\d+)?(?:[ ._]+(?:end|final))?(?:[ ._]*$|[ ._]*[\[(]|[ ._]+-)`)
	// animeDashed is animeNumber's dashed form alone, read first so a number in a show's name stays
	// the show's: "Hoshizora 86 - 04" is episode 4 of "Hoshizora 86".
	animeDashed = regexp.MustCompile(`(?i)^(.+?)[ ._]+-[ ._]+(?:(?:e|ep|episode)[ ._]*)?(\d{1,4})()(?:\.5)?(?:-\d{1,4})?(?:v\d+)?(?:[ ._]+(?:end|final))?(?:[ ._]*$|[ ._]*[\[(]|[ ._]+-)`)
	// paddedAbsolute matches an episode's number across its show after a dash, at the end of a name,
	// written with a leading zero or in three digits: "Show - 08", "Show - 013v2".
	paddedAbsolute = regexp.MustCompile(`[ ._]-[ ._]+(?:0\d{1,3}|[1-9]\d{2})(?:v\d+)?$`)
	// dashedLast matches a dashed number at the end of a name: "Show - 7", "Show - 07v2".
	dashedLast = regexp.MustCompile(`[ ._]-[ ._]+\d{1,4}(?:v\d+)?$`)
	// releaseNotes matches a release's bracketed notes at the end of its name: "[1080p]", "(BD
	// 1080p x265)", "[WEB][AAC]".
	releaseNotes = regexp.MustCompile(`(?i)[\[(][^\])]*(?:\d{3,4}p|bd|web|hevc|x26[45]|aac|flac|bluray|dvd|hdtv)[^\])]*[\])][ ._]*$`)
	// animeBrackets matches a release that brackets everything: "[Kumo][Show][13][1080p]".
	animeBrackets = regexp.MustCompile(`^\[[^\]]*\]\[([^\]]+)\]\[(\d{1,4})(?:v\d+)?\]`)
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

// looseAnime reads a fansub release's own name, "[Group] Show - 07 [1080p]" or
// "[Group][Show][07][1080p]": its show and its absolute episode, or, where the show's name ends in
// its season ("Show S2 - 03", "Show 2nd Season - 03"), that season's episode. Without the group's
// tag a name is not read so: "Show - 07" could be a title of its own.
func looseAnime(name string) (string, mp.Numbering, bool) { return animeRelease(name, false) }

// animeRelease is looseAnime for a file in an anime's folder too ("Anime/"), where a name is a
// release's as if its group had tagged it: "Anime/Usagi Den 07 [1080p].mkv", "Anime/Kumo Tetsu - 07
// (BD 1080p) [Grp].mkv", "Anime/Usagi.Den.08.1080p.mkv", read without its release's words.
func animeRelease(name string, inAnime bool) (string, mp.Numbering, bool) {
	s := stem(name)
	if inAnime && !leadingTags.MatchString(s) {
		words := strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(s))
		if at := slices.IndexFunc(words, releaseToken); at > 0 {
			words = words[:at]
		}
		s = strings.Join(words, " ")
	}
	// A group's tag last, "Show - 06 [Kumo]", says a release as one first does, for a number written
	// as an anime's is, padded or after a dash: "Apollo Ridge 13 [Criterion]" is a film's.
	grouped := false
	if plain := trimReleaseNotes(s); !leadingTags.MatchString(plain) {
		if m := trailingGroup.FindStringSubmatchIndex(plain); m != nil && groupTag(plain[m[2]:m[3]]) {
			s, grouped = plain[:m[0]], true
		}
	}
	tagged := inAnime || leadingTags.MatchString(s)
	// A release's special, "[Kumo] Show - SP01" or "Show - OVA 2 [Kumo]", is the show's season 0's.
	if m := animeSpecial.FindStringSubmatch(leadingTags.ReplaceAllString(s, "")); m != nil && (tagged || grouped) && atoi(m[2]) > 0 {
		return showSpelling(m[1]), mp.Numbering{Season: 0, Episode: atoi(m[2])}, true
	}
	show, number := "", 0
	if m := animeBrackets.FindStringSubmatch(s); m != nil {
		show, number = m[1], atoi(m[2])
	} else if tagged || grouped || paddedAbsolute.MatchString(plainStem(name)) ||
		releaseNotes.MatchString(s) && dashedLast.MatchString(plainStem(name)) {
		// With no group's tag, a number written as Sonarr writes an anime's ("Show - 08", "Show -
		// 013"), or a dashed number last before a release's notes ("Show - 7 [1080p]"), say a name
		// is a release's; "Show - 7", "Show 11 [1080p]" and "Show - 03 - Tidewater [1080p]" may be
		// titles or a season's episodes. A year says a film, "[Tracker] Show 11 (2001)", unless a
		// dashed number follows it: "[Kumo] Show (2019) - 07" is the 2019 show's episode.
		bare := leadingTags.ReplaceAllString(s, "")
		m := animeDashed.FindStringSubmatch(bare)
		if m == nil {
			m = animeNumber.FindStringSubmatch(bare)
		}
		if m != nil && bracketYear.MatchString(s) && (m[2] == "" || !bracketYear.MatchString(m[1])) {
			m = nil
		}
		if m != nil && (tagged || m[2] != "" || grouped && strings.HasPrefix(m[3], "0")) && !yearToken.MatchString(matched(m[2], m[3])) {
			show, number = m[1], atoi(matched(m[2], m[3]))
		}
	}
	// A tagged release numbered as Japanese and Chinese write it: "[Kumo] Hoshi Kettle 第03話".
	if bare := leadingTags.ReplaceAllString(s, ""); number == 0 && bare != s {
		if m := cjkEpisode.FindStringSubmatch(bare); m != nil {
			show, number = strings.TrimSpace(bare[:strings.Index(bare, m[0])]), atoi(matched(m[1], m[2]))
		}
	}
	if number == 0 || show == "" {
		return "", mp.Numbering{}, false
	}
	show = showSpelling(show)
	if m := animeSeason.FindStringSubmatch(show); m != nil {
		season := atoi(matched(m[2], m[3], m[4]))
		return strings.TrimSpace(m[1]), mp.Numbering{Season: season, Episode: number}, true
	}
	return show, mp.Numbering{Absolute: number}, true
}

// groupTag says whether a bracketed word at a name's end is a release group's: not a release's
// note ("1080p", "HEVC"), an edition ("Extended") or a year.
func groupTag(inner string) bool {
	return !releaseToken(inner) && !releaseNote(inner) && !editionOnly([]string{inner}) && !yearToken.MatchString(inner)
}

// animeSeason matches a fansub show's name that ends in its season: "Show S2", "Show 2nd Season",
// "Show Season 2".
var animeSeason = regexp.MustCompile(`(?i)^(.+?)[ ._]+(?:s(\d{1,2})|(\d{1,2})(?:st|nd|rd|th)[ ._]+season|season[ ._]+(\d{1,2}))$`)

// numberingInFolder is the numbering a file's folders let its name give, where the name alone does
// not say it is an episode: a show's file "Ep02", "2of6" or "Season 1 Episode 2"; a season folder's
// "05 - Tidewater"; the show's name and a number, "Kettle Bay 104" (season 1, episode 4, as Kodi
// reads it) or, as Sonarr writes an anime's, "Kettle Bay - 013" (episode 13 of all). A season
// folder gives the season; with none it is the first. A file loose at the top has no show to say so.
func numberingInFolder(file File) (mp.Numbering, bool) {
	dir := path.Dir(file.Rel)
	// The name is read without its release's notes at its end: "Show - Part One [1080p]" is
	// "Show - Part One". A fansub's notes say what it is, so looseAnime reads them.
	raw, name := stem(path.Base(file.Rel)), plainStem(path.Base(file.Rel))
	season, inSeason, namedFolder, show := 1, false, false, path.Base(dir)
	if seasonDir := seasonDirOf(dir); seasonDir != "" {
		inSeason, show = true, path.Base(aboveSeasons(seasonDir))
		if n, ok := seasonIn(path.Base(seasonDir)); ok {
			season = n
		} else if m := namedSeason.FindStringSubmatch(path.Base(seasonDir)); m != nil {
			season, _ = seasonNumberWord(matched(m[2], m[3]))
		}
	} else if n := namedSeasonOf(show); n > 0 {
		// A show's season folder with no show folder above it, "Puddle Pals Season 2/", is the
		// show's and gives the season.
		season, namedFolder = n, true
	}
	episode := func(e int) (mp.Numbering, bool) { return mp.Numbering{Season: season, Episode: e}, e > 0 }
	// "Bellcote - Season 5 Episode 2" and "Dunbar.Season.3.Episode.1" name their season too, which
	// is theirs before the folder's.
	if m := seasonEpisodeWords.FindStringSubmatch(name); m != nil {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, atoi(m[2]) > 0
	}
	// "Spotty Dog - Episode 3" names its show and its episode wherever it is, loose or in a
	// catch-all: its season folder's, or the first season's. A fansub's, "[Kumo] Show - Episode
	// 17", is its number in the whole show (looseAnime).
	if m := dashedEpisode.FindStringSubmatch(name); m != nil && !leadingTags.MatchString(name) {
		return episode(atoi(m[1]))
	}
	// So does a scene release's "larchmont.road.312.hdtv-lol": season 3, episode 12.
	if m := sceneNumber.FindStringSubmatch(raw); m != nil && atoi(m[2]) > 0 {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	// A TV capture's part, "Zelston.Fall.Pt1.HDTV", "Ashlington.Rise.3of6.HDTV" or
	// "Jevington.E01.HDTV", is a broadcast's episode, never a film's part.
	if n, ok := capturedEpisode(raw); ok {
		return episode(n)
	}
	if m := partOfWord.FindStringSubmatch(name); m != nil && atoi(m[1]) <= atoi(m[2]) {
		return episode(atoi(m[1]))
	}
	if dir == "." {
		return mp.Numbering{}, false
	}
	// "Tiddlywinks Farm - Series 2 - 05 - Muddy Day" is series 2's fifth.
	if m := seasonDashEpisode.FindStringSubmatch(name); m != nil && atoi(m[2]) > 0 {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	if inSeason && season > 0 && !leadingTags.MatchString(name) {
		// "Kettle Bay - 3.05" or "1.05 Hedge" in Season 3 or 1, before "1" reads as a number alone;
		// and the season's number then the episode's apart by a dash, "Bramley Court - 3 - 01" or
		// "2-03" in Season 3 or 2.
		for _, re := range []*regexp.Regexp{seasonDotEpisode, seasonDashedEpisode} {
			if m := re.FindStringSubmatch(name); m != nil && atoi(m[1]) == season {
				return episode(atoi(m[2]))
			}
		}
		// A two-digit number in a season folder ("Kettle Bay - 03" or "Kettle Bay 06" in Season 2) is
		// the season's episode: an anime's number across its show is three digits, as Sonarr writes
		// it, and a fansub's carries its group's tag.
		n, ok := numberAfterShow(name, showFolderName(show), file.Notes.Numbered)
		// Three digits whose hundred is the folder's season, "Harbor 24 - 103" in Season 1, are its
		// episode as Kodi writes it.
		if ok && n.Absolute >= 100 && n.Absolute < 1000 && n.Absolute/100 == season && n.Absolute%100 > 0 {
			return episode(n.Absolute % 100)
		}
		if ok && n.Absolute > 0 && n.Absolute < 100 && seasonTwoDigits.MatchString(name) {
			return episode(n.Absolute)
		}
	}
	if _, n, ok := animeRelease(path.Base(file.Rel), inAnime(dir)); ok {
		return n, true
	}
	// A catch-all's file ("Movies/Ferrow Lane Part 2 of 3 (2010).mkv") has no show folder to number
	// it after, unless it is one of several numbered after a name they share ("Cartoons/Fizzy Fox
	// 01.mp4" beside "… 02").
	if !inSeason && catchAllFolder(show) && !file.Notes.Numbered {
		return mp.Numbering{}, false
	}
	if m := capMark.FindStringSubmatch(name); m != nil && atoi(m[2]) > 0 && !yearToken.MatchString(m[1]+m[2]) {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	if m := episodeWord.FindStringSubmatch(name); m != nil {
		return episode(atoi(m[1]))
	}
	if m := cjkEpisode.FindStringSubmatch(name); m != nil {
		return episode(atoi(matched(m[1], m[2])))
	}
	if m := partOf.FindStringSubmatch(name); m != nil && atoi(m[1]) <= atoi(m[2]) {
		return episode(atoi(m[1]))
	}
	// So do a show's episodes numbered alone in its own folder ("Ocean Halls/01 - The Deep.mkv").
	if m := leadingNumber.FindStringSubmatch(name); (inSeason || file.Notes.Leading) && m != nil {
		// "101 - Tidewater" in Season 1 is its first episode, when every numbered name in the folder
		// is in its hundred; "101" among "99" and "100" is the hundred and first.
		if n := atoi(m[1]); season > 0 && file.Notes.Hundreds == season {
			return episode(n % 100)
		}
		// A number alone with no season folder, "Fizzy Hen/01.mp4", is its place in the whole show.
		if !inSeason && !namedFolder && digits(name) {
			return mp.Numbering{Absolute: atoi(m[1])}, atoi(m[1]) > 0
		}
		return episode(atoi(m[1]))
	}
	if inSeason && season > 0 {
		// "Kettle Bay - 3.05" in Season 3, and "Kettle Bay E03" in any season.
		if m := seasonDotEpisode.FindStringSubmatch(name); m != nil && atoi(m[1]) == season {
			return episode(atoi(m[2]))
		}
		if m := bareEpisode.FindStringSubmatch(name); m != nil {
			return episode(atoi(m[1]))
		}
	}
	// "Kettle Bay E01" in the show's own folder, with no season folder, is its first season's.
	if m := bareEpisode.FindStringSubmatchIndex(name); !inSeason && m != nil {
		before, _, _ := searchQuery(name[:m[0]], 0)
		own, _, _ := searchQuery(showFolderName(show), 0)
		if relatedName(before, own) {
			return episode(atoi(name[m[2]:m[3]]))
		}
	}
	n, ok := numberAfterShow(name, showFolderName(show), file.Notes.Numbered)
	// A two-digit number in a season folder ("Kettle Bay - 03" or "Kettle Bay 06" in Season 2) is the
	// season's episode: an anime's number across its show is three digits, as Sonarr writes it. So
	// is one in a folder named by its show and season, "TV/Owlbeck Season 2/Owlbeck 01".
	if ok && n.Absolute > 0 && (inSeason || namedFolder) && season > 0 && seasonTwoDigits.MatchString(name) {
		return episode(n.Absolute)
	}
	// A show's folder in a shows' catch-all numbers its episodes by a number alone or after the
	// show's name, "TV/Ivory Fen/02.mkv" and "Anime/Hoshizora/Hoshizora 12.mkv": its episode by
	// its place in the whole show. In a film's folder, "Marrow Point 2" is the sequel.
	if !ok && !inSeason && (underShows(dir) || file.Notes.Series) {
		// A miniseries' "Ridge Point Part 2" and "Ridge Fall - Part Two" are its episodes.
		if _, part, ok := partIn(name + ".mkv"); ok && part > 0 {
			return episode(part)
		}
		if m := partWord.FindStringSubmatch(name); m != nil {
			if part, ok := spelledNumbers[strings.ToLower(m[1])]; ok {
				return episode(part)
			}
		}
		words, own := strings.Fields(normName(name)), showWordsOf(show)
		if len(words) == len(own)+1 && slices.Equal(words[:len(own)], own) || len(words) == 1 {
			if last := words[len(words)-1]; digits(last) && len(last) <= 3 && atoi(last) > 0 {
				// A folder named by its show and season, "Pebble Pals Season 1", gives the season.
				if namedFolder {
					return episode(atoi(last))
				}
				return mp.Numbering{Absolute: atoi(last)}, true
			}
		}
	}
	return n, ok
}

// numberAfterShow reads a number after a show's name: "Kettle Bay - 013" and "Kettle Bay 013" are
// its absolute episode 13, and "Kettle.Bay.102" its season 1, episode 2, as are the files of a
// folder that number theirs after a name they share ("kb 101", "kb 201"). A year is never one.
func numberAfterShow(name, show string, numbered bool) (mp.Numbering, bool) {
	words := func(s string) []string {
		return strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(s)))
	}
	// The show is its title without its year, and the name's year is passed over: "Kettle Bay (2019)
	// - 013" in "Kettle Bay (2019)".
	title, _, _ := searchQuery(show, 0)
	nameWords, showWords := slices.DeleteFunc(words(name), func(w string) bool {
		return len(w) == 6 && w[0] == '(' && w[5] == ')' && yearToken.MatchString(w[1:5])
	}), words(title)
	at := len(showWords)
	if at == 0 || len(nameWords) <= at || !slices.Equal(nameWords[:at], showWords) {
		if !numbered {
			return mp.Numbering{}, false
		}
		at = slices.IndexFunc(nameWords, func(w string) bool { return digits(w) })
		if at < 1 {
			return mp.Numbering{}, false
		}
	}
	dashed := nameWords[at] == "-"
	if dashed && len(nameWords) > at+1 {
		at++
	}
	num, _, _ := strings.Cut(nameWords[at], "-") // "001-003" starts at 1
	if !digits(num) || len(num) > 4 || yearToken.MatchString(num) {
		return mp.Numbering{}, false
	}
	n := atoi(num)
	switch {
	case dashed || num[0] == '0':
		return mp.Numbering{Absolute: n}, n > 0
	case len(num) >= 3:
		return mp.Numbering{Season: n / 100, Episode: n % 100}, n%100 > 0
	}
	// "Marrow Point 2" is a sequel as likely as an episode.
	return mp.Numbering{}, false
}

// catchAllNames are folders that hold a library's files rather than name a show.
// Those that hold shows only are showsFolderNames, and films only filmsFolderNames; these hold
// either, or both.
var catchAllNames = map[string]bool{
	"media": true, "video": true, "videos": true, "downloads": true, "download": true,
	"complete": true, "completed": true, "finished": true, "unsorted": true, "new": true,
	"torrents": true, "torrent": true, "misc": true, "to sort": true, "tosort": true, "stuff": true,
	"inbox": true, "temp": true, "tmp": true, "various": true, "mixed": true, "library": true,
	// A household's folders by audience and genre, and by kind in its language.
	"kids": true, "children": true, "family": true, "documentaries": true, "docs": true,
	"documentary": true, "films et séries": true, "comedy": true,
	// A household's folders by what it has watched.
	"new episodes": true, "to watch": true, "towatch": true, "watchlist": true, "unwatched": true,
	"watched": true, "currently watching": true, "new shows": true, "ongoing": true, "ended": true,
}

// alphabetFolder matches a folder a library is shelved into by its titles' first letters: "C",
// "D-F", "#", "0-9". A show is never named by a letter alone, as a shelf is.
var alphabetFolder = regexp.MustCompile(`^(?:#|0-9|\pL|\pL ?- ?\pL)$`)

// partWord matches a part spelled out at a name's end: "Ridge Fall - Part One".
var partWord = regexp.MustCompile(`(?i)(?:^|[ ._-])part[ ._-]+([a-z]+)$`)

// showsFolderNames are catch-alls that hold shows: a folder in one is a show's.
var showsFolderNames = map[string]bool{
	"tv": true, "tv shows": true, "tvshows": true, "shows": true, "television": true, "anime": true,
	"series": true, "tv series": true, "serien": true, "séries": true, "cartoons": true, "kids tv": true,
	"miniseries": true, "mini series": true, "recorded tv": true, "kids shows": true, "all shows": true,
	"recordings": true, "tv recordings": true, "kids series": true,
}

// amongFilms says whether a folder, relative to the root, is a films' catch-all or in one: the
// nearest catch-all of a kind, itself or above it, is a films' ("Movies/", "Movies/Kids/",
// "Movies/Ferrow Lane (2019)/"), not a shows' ("Movies/TV/Kettle Bay/").
func amongFilms(dir string) bool {
	for d := dir; d != "."; d = path.Dir(d) {
		switch key := folderKey(path.Base(d)); {
		case showsFolderNames[key]:
			return false
		case filmsFolderNames[key] || filmSeries.MatchString(path.Base(d)):
			return true
		}
	}
	return false
}

// inAnime says whether a folder, relative to the root, is an anime catch-all or in one.
func inAnime(dir string) bool {
	for d := dir; d != "."; d = path.Dir(d) {
		if animeFolderNames[folderKey(path.Base(d))] {
			return true
		}
	}
	return false
}

// animeFolderNames are the catch-alls that hold anime.
var animeFolderNames = map[string]bool{"anime": true, "animes": true, "anime series": true, "アニメ": true}

// filmSeries matches a folder named as films' run is, its franchise's name then the word last:
// "Thistledown Saga", "Hazelgrot Trilogy (2001-2005)". A show's run is never called so; "Saga Two"
// is a title, and "Collection" may be a show's box set.
var filmSeries = regexp.MustCompile(`(?i)^.+[ ._-](?:saga|duology|trilogy|quadrilogy|tetralogy|pentalogy|hexalogy)(?:[ ._]*[(\[][^)\]]*[)\]])*$`)

// underShows says whether a folder, relative to the root, is in a catch-all that holds shows.
func underShows(dir string) bool {
	for d := path.Dir(dir); d != "."; d = path.Dir(d) {
		if showsFolderNames[folderKey(path.Base(d))] {
			return true
		}
	}
	return false
}

// catchAllFolder says whether a folder's name is a catch-all's.
func catchAllFolder(name string) bool {
	key := folderKey(name)
	return catchAllNames[key] || showsFolderNames[key] || filmsFolderNames[key] || alphabetFolder.MatchString(key)
}

// folderKey is a folder's name as the catch-all lists read it: its words, in lower case.
func folderKey(name string) string {
	return strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(name))), " ")
}

// folderNotes are what a folder's videos say about each of them together.
type folderNotes struct {
	// Numbered is a folder two or more of whose videos number themselves after a name they share
	// ("kb 101", "kb 201").
	Numbered bool
	// Hundreds is the hundred every video named by a leading number shares ("101 - Tidewater", "102
	// - Low Water" share 1), and 0 when they share none, or one is under 100 or a whole hundred.
	Hundreds int
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
	// Folders are the folder's own folders, by name in order: a loose episode beside its show's
	// folder ("TV/Holm Lane S01E02.mkv" by "TV/Holm Lane/") is that show's.
	Folders []string
}

// leadingEpisodes says whether a folder's videos are a show's numbered by their place alone: two or
// more start with a number and a separator ("01 - The Deep", "2. Low Water"), and none gives a
// year, as a collection's films do ("01 - Marrow Point (2010)").
func leadingEpisodes(folder string, names []string) bool {
	numbered := 0
	_, folderYear, _ := searchQuery(folder, 0)
	for _, name := range names {
		s := stem(name)
		if _, year, _ := searchQuery(s, 0); year != 0 || bracketYear.MatchString(s) {
			return false
		}
		// "Fizzy Hen/01.mp4" beside "02.mp4" is numbered by its place alone; in a film's folder,
		// "Ashgrove (2015)/01.mkv", those are its titles.
		if bare := plainStem(name); leadingPlace.MatchString(s) || folderYear == 0 && digits(bare) && len(bare) == 2 && bare[0] == '0' {
			numbered++
		}
	}
	return numbered >= 2
}

// namedSeries says whether a folder's videos are a show's named by the show its folder's name
// gives (folderNotes.Series).
func namedSeries(folder string, names []string) bool {
	own := showWordsOf(folder)
	if len(own) == 0 || catchAllFolder(folder) {
		return false
	}
	named := 0
	for _, name := range names {
		s := plainStem(name)
		if _, year, _ := searchQuery(s, 0); year != 0 || bracketYear.MatchString(s) {
			return false
		}
		words := strings.Fields(normName(s))
		// "Ironvale 2" and "Ironvale 3" beside "Ironvale" are a film's sequels: a show's episodes
		// say more after its name.
		sequel := len(words) == len(own)+1 && digits(words[len(own)])
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
	var notes folderNotes
	for _, n := range prefixes {
		notes.Numbered = notes.Numbered || n >= 2
	}
	// Every name that starts with a number starts with one in the same hundred: "101", "102".
	for _, name := range names {
		m := leadingNumber.FindStringSubmatch(plainStem(name))
		if m == nil {
			continue
		}
		n := atoi(m[1])
		switch hundreds := n / 100; {
		case n < 100 || n%100 == 0 || (notes.Hundreds != 0 && notes.Hundreds != hundreds):
			return folderNotes{Numbered: notes.Numbered}
		default:
			notes.Hundreds = hundreds
		}
	}
	return notes
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
