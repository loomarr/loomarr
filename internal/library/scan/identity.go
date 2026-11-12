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
	name := stem(path.Base(file.Rel))
	before, _, _ := searchQuery(datedShow(name), 0)
	if before == "" {
		return false
	}
	// Loose at the top or in a catch-all ("Downloads/"), a release's name is its own show's:
	// "Evening.Ledger.2024.03.18.720p.WEB".
	if dir := path.Dir(file.Rel); dir == "." || catchAllFolder(path.Base(dir)) {
		return datedRelease(name)
	}
	show, _, _ := searchQuery(showFolderName(path.Base(seriesFolderOf(file))), 0)
	return relatedName(before, show)
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
	// So is one beside a show's marked episodes in the show's own folder ("Kettle Bay/Pilot.mkv"
	// beside "Kettle Bay S01E01.mkv"); a catch-all's or a dump's are their own.
	if dir := path.Dir(file.Rel); dir != "." && file.Notes.Episodes && !file.Notes.Dump && !catchAllFolder(path.Base(dir)) {
		seriesDir := seriesFolderOf(file)
		return mp.Record{Source: src, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}, seriesDir
	}
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
	return rec, ""
}

// seasonWords name a season in a folder's name, in the languages households name them in.
const seasonWords = `seasons|season|series|staffel|saison|temporada|stagione|sezon|seizoen|säsong|sasong|sæson|saeson|sesong|сезон`

// spelledNumbers are a season's number spelled out: "Season One", "Series Two".
var spelledNumbers = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
	"ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15,
	"sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19, "twenty": 20,
}

var (
	// seasonFolder matches a season folder's name and its number: "Season 1", "Season 01", "Series
	// One", "Staffel 1", "Säsong 1", with whatever notes follow a separator ("Season 2 [1080p]",
	// "Season 1 - 2019", "Season 01 Complete", "Season 1-2"); "S01" with only bracketed notes.
	seasonFolder = regexp.MustCompile(`(?i)^(?:(?:` + seasonWords + `)[ ._-]*(\d{1,4}|[a-z]+)(?:[ ._\-\[(].*)?|s(\d{1,4})(?:[ ._-]*[\[(][^\])]*[\])])*)$`)
	// namedSeason matches a season folder that starts with its show's name: "Kettle Bay Season 1",
	// "KB Season 1".
	namedSeason = regexp.MustCompile(`(?i)^(.+?)[ ._-]+(?:` + seasonWords + `)[ ._-]*(\d{1,4}|[a-z]+)(?:[ ._\-\[(].*)?$`)
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
	if m := ordinalSeason.FindStringSubmatch(name); m != nil {
		return atoi(m[1]), true
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

// seasonNumberWord is a season's number written in digits or spelled out.
func seasonNumberWord(w string) (int, bool) {
	if digits(w) {
		return atoi(w), true
	}
	n, ok := spelledNumbers[strings.ToLower(w)]
	return n, ok
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
	if _, ok := seasonNumberWord(m[2]); !ok {
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
	if dir == "." {
		return ownSeries(name)
	}
	// A catch-all folder ("Downloads/Complete", "Media", "TV/All Shows") names no show: an episode
	// there that names its own is that show's, as one loose at the top is. A catch-all is known by
	// its name, or by its episodes naming shows with none of them its own (a Dump): a show's own
	// folder names several too, when the show was renamed or its files spell it two ways, but one of
	// them is the folder's.
	if show := ownSeries(name); (catchAllFolder(path.Base(dir)) || file.Notes.Dump && dir == path.Dir(file.Rel)) && show != "" {
		return show
	}
	if season := seasonDirOf(dir); season != "" {
		if show := aboveSeasons(season); show != "." {
			return show
		}
		return looseSeries(name)
	}
	// A year's folder in a show's ("Evening Ledger/2024/") holds that year's episodes, as a season's
	// does.
	if yearToken.MatchString(path.Base(dir)) && path.Dir(dir) != "." && !catchAllFolder(path.Base(path.Dir(dir))) {
		dir = path.Dir(dir)
	}
	// A season pack unpacked into its show's folder ("Kettle Bay/Kettle.Bay.S02.1080p.WEB-DL/") is
	// that show's.
	if base := path.Base(dir); path.Dir(dir) != "." && showFolderName(base) != base &&
		relatedName(showFolderName(base), showFolderName(path.Base(path.Dir(dir)))) {
		dir = path.Dir(dir)
	}
	for d := dir; d != "."; d = path.Dir(d) {
		if len(idsIn(path.Base(d))) > 0 {
			return d
		}
	}
	return dir
}

// seasonToken is a word in a release folder's name that starts what it holds of a show: "S02",
// "S01-S03".
var seasonToken = regexp.MustCompile(`(?i)^s\d{1,2}(?:-s?\d{1,2})?$`)

// showYear matches the year an episode's file name gives its show, in brackets just before the
// episode's mark: "Show (1963) - S01E01", "Show (1963) - S01xE01", "Show.[1963].1x01".
var showYear = regexp.MustCompile(`[(\[]((?:18|19|20)\d\d)[)\]][\s._-]*(?:[Ss]\d{1,4}[xX]?[Ee]\d|\d{1,2}[xX]\d)`)

// episodeRange matches a fansub pack's bracketed run of episodes: "(01-12)", "[01~26]".
var episodeRange = regexp.MustCompile(`[(\[]\s*\d{1,4}\s*[-~]\s*\d{1,4}\s*[)\]]`)

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
	if leadingTags.MatchString(name) && episodeRange.MatchString(name) {
		name = strings.TrimSpace(name[:episodeRange.FindStringIndex(name)[0]])
		name = strings.TrimSpace(leadingTags.ReplaceAllString(name, ""))
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
		holds := seasonToken.MatchString(w) ||
			((w == "season" || w == "series" || w == "staffel" || w == "saison") && digits(next)) ||
			(w == "complete" && (next == "series" || next == "season" || next == "seasons"))
		if holds {
			if show := strings.Trim(strings.Join(words[:i], " "), " -."); show != "" {
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
		return datedShow(stem(name))
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
		return ""
	}
	show := strings.NewReplacer(".", " ", "_", " ").Replace(leadingTags.ReplaceAllString(name[:mark[0]], ""))
	return strings.Trim(strings.Join(strings.Fields(show), " "), " -")
}

var (
	// seasonEpisodeWords matches a mark written out: "Season 1 Episode 2", "Season 1 Ep 2",
	// "Staffel 1 Folge 2", "Temporada 1 Episodio 2".
	seasonEpisodeWords = regexp.MustCompile(`(?i)(?:^|[^\pL0-9])(?:` + seasonWords + `)[ ._-]*(\d{1,4})[ ._-]*(?:episode|ep|folge|episodio|épisode|aflevering|avsnitt|afsnit|odcinek)[ ._-]*(\d{1,4})(?:[^0-9]|$)`)
	// episodeWord matches an episode with no season: "Ep02", "Episode 2", "Ep. 2", "Folge 2",
	// "Avsnitt 2", "Episodio 2", "Aflevering 2", "Capítulo 2", "Серия 2", "Bölüm 2".
	episodeWord = regexp.MustCompile(`(?i)(?:^|[^\pL0-9])(?:episode|ep|folge|episodio|episódio|épisode|aflevering|avsnitt|afsnit|odcinek|cap[ií]tulo|серия|эпизод|bölüm|jakso)[ ._-]*(\d{1,4})(?:[^0-9]|$)`)
	// capMark matches a Spanish release's "Cap.117": season 1, episode 17.
	capMark = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])cap[ ._]?(\d{1,2})(\d{2})(?:[^0-9]|$)`)
	// cjkEpisode matches a Japanese or Chinese episode's mark: "第11話", "第12集", "第3回".
	cjkEpisode = regexp.MustCompile(`第\s*(\d{1,4})\s*[話话集回]`)
	// partOf matches a miniseries' "2of6", "2 of 6", "Part 2 of 6".
	partOf = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,3})[ ._-]*of[ ._-]*(\d{1,3})(?:[^0-9a-z]|$)`)
	// leadingNumber matches a name that starts with its episode's number: "05 - Tidewater".
	leadingNumber = regexp.MustCompile(`^(\d{1,3})(?:[ ._-]|$)`)
	// bracketYear matches a year in brackets.
	bracketYear = regexp.MustCompile(`[(\[](?:19|20)\d{2}[)\]]`)
	// seasonDotEpisode matches a season and episode written "3.05".
	seasonDotEpisode = regexp.MustCompile(`(?:^|[ _-])(\d{1,2})\.(\d{2})(?:[ ._-]|$)`)
	// bareEpisode matches an episode written "E03" with no season before it.
	bareEpisode = regexp.MustCompile(`(?i)(?:^|[ ._-])e(\d{1,4})(?:[ ._-]|$)`)
	// seasonTwoDigits matches a two-digit number after a show's name, with or without a dash:
	// "Kettle Bay - 03", "Kettle Bay 06".
	seasonTwoDigits = regexp.MustCompile(`[ ._](?:-[ ._]*)?\d{2}(?:[ ._-]|$)`)
	// leadingTags are a release group's bracketed tags before a name: "[Kumo] ", "[Kumo][Show]".
	leadingTags = regexp.MustCompile(`^(?:[ ._]*\[[^\]]*\])+[ ._]*`)
	// animeNumber matches an anime release's absolute episode after its show's name, " - 07" or
	// " - 013v2", ahead of its notes or its end.
	animeNumber = regexp.MustCompile(`(?i)^(.+?)[ ._]+-[ ._]+(\d{1,4})(?:v\d+)?(?:[ ._]+(?:end|final))?(?:[ ._]*$|[ ._]*[\[(]|[ ._]+-)`)
	// animeBrackets matches a release that brackets everything: "[Kumo][Show][13][1080p]".
	animeBrackets = regexp.MustCompile(`^\[[^\]]*\]\[([^\]]+)\]\[(\d{1,4})(?:v\d+)?\]`)
)

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
func looseAnime(name string) (string, mp.Numbering, bool) {
	s := stem(name)
	show, number := "", 0
	if m := animeBrackets.FindStringSubmatch(s); m != nil {
		show, number = m[1], atoi(m[2])
	} else if leadingTags.MatchString(s) {
		if m := animeNumber.FindStringSubmatch(leadingTags.ReplaceAllString(s, "")); m != nil {
			show, number = m[1], atoi(m[2])
		}
	}
	if number == 0 {
		return "", mp.Numbering{}, false
	}
	show = strings.TrimSpace(show)
	if m := animeSeason.FindStringSubmatch(show); m != nil {
		season := atoi(m[2] + m[3] + m[4]) // one of the three is the season
		return strings.TrimSpace(m[1]), mp.Numbering{Season: season, Episode: number}, true
	}
	return show, mp.Numbering{Absolute: number}, true
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
	if dir == "." {
		return mp.Numbering{}, false
	}
	name := stem(path.Base(file.Rel))
	season, inSeason, show := 1, false, path.Base(dir)
	if seasonDir := seasonDirOf(dir); seasonDir != "" {
		inSeason, show = true, path.Base(aboveSeasons(seasonDir))
		if n, ok := seasonIn(path.Base(seasonDir)); ok {
			season = n
		} else if m := namedSeason.FindStringSubmatch(path.Base(seasonDir)); m != nil {
			season, _ = seasonNumberWord(m[2])
		}
	}
	episode := func(e int) (mp.Numbering, bool) { return mp.Numbering{Season: season, Episode: e}, e > 0 }
	if m := seasonEpisodeWords.FindStringSubmatch(name); m != nil {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, atoi(m[2]) > 0
	}
	if _, n, ok := looseAnime(name); ok {
		return n, true
	}
	// A catch-all's file ("Movies/Ferrow Lane Part 2 of 3 (2010).mkv") has no show folder to number
	// it after.
	if !inSeason && catchAllFolder(show) {
		return mp.Numbering{}, false
	}
	if m := capMark.FindStringSubmatch(name); m != nil && atoi(m[2]) > 0 && !yearToken.MatchString(m[1]+m[2]) {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	if m := episodeWord.FindStringSubmatch(name); m != nil {
		return episode(atoi(m[1]))
	}
	if m := cjkEpisode.FindStringSubmatch(name); m != nil {
		return episode(atoi(m[1]))
	}
	if m := partOf.FindStringSubmatch(name); m != nil && atoi(m[1]) <= atoi(m[2]) {
		return episode(atoi(m[1]))
	}
	if m := leadingNumber.FindStringSubmatch(name); inSeason && m != nil {
		// "101 - Tidewater" in Season 1 is its first episode, when every numbered name in the folder
		// is in its hundred; "101" among "99" and "100" is the hundred and first.
		if n := atoi(m[1]); season > 0 && file.Notes.Hundreds == season {
			return episode(n % 100)
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
	// season's episode: an anime's number across its show is three digits, as Sonarr writes it.
	if ok && n.Absolute > 0 && inSeason && season > 0 && seasonTwoDigits.MatchString(name) {
		return episode(n.Absolute)
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
var catchAllNames = map[string]bool{
	"tv": true, "tv shows": true, "tvshows": true, "shows": true, "television": true, "movies": true,
	"films": true, "media": true, "video": true, "videos": true, "downloads": true, "download": true,
	"complete": true, "completed": true, "finished": true, "unsorted": true, "all shows": true,
	"anime": true, "new": true, "torrents": true, "torrent": true, "misc": true, "to sort": true,
	"tosort": true, "stuff": true, "inbox": true, "temp": true, "tmp": true, "various": true,
}

// catchAllFolder says whether a folder's name is a catch-all's.
func catchAllFolder(name string) bool {
	return catchAllNames[strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(name))), " ")]
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
	// Episodes is a folder two or more of whose videos carry an episode's mark: a show's own folder,
	// whose unmarked videos ("Pilot.mkv") are its episodes too.
	Episodes bool
}

// dumpFolder says whether a folder named folder, holding videos named names, is a Dump. A show's
// own folder whose files spell it two ways, or carry an old name beside its new one, has one of
// them its own ("Kettle Bay (US)" holding "Kettle.Bay.US.S01E01" and "KB S02E01").
func dumpFolder(folder string, names []string) bool {
	own, _, _ := searchQuery(showFolderName(folder), 0)
	shows := map[string]bool{}
	for _, name := range names {
		show := looseSeries(name)
		if show == "" {
			continue
		}
		title, _, _ := searchQuery(show, 0)
		if relatedName(title, own) {
			return false
		}
		shows[normName(title)] = true
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
		if m := numberedName.FindStringSubmatch(stem(name)); m != nil && !yearToken.MatchString(m[2]) {
			prefixes[strings.ToLower(strings.Join(strings.Fields(m[1]), " "))]++
		}
	}
	var notes folderNotes
	for _, n := range prefixes {
		notes.Numbered = notes.Numbered || n >= 2
	}
	// Every name that starts with a number starts with one in the same hundred: "101", "102".
	for _, name := range names {
		m := leadingNumber.FindStringSubmatch(stem(name))
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

// numberedName matches a name and the three- or four-digit number after it: "kb 101".
var numberedName = regexp.MustCompile(`^(.*[^0-9 ._-])[ ._-]+(\d{3,4})$`)
