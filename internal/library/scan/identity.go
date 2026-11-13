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
func airDateIn(name string) string { return airDateOrdered(name, false) }

// airDateOrdered is airDateIn with a day written "05-06-2023" read month first when monthFirst: its
// folder's other days say America's order ("03-14-2023" beside it, folderNotes.MonthFirst).
func airDateOrdered(name string, monthFirst bool) string {
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
			if atoi(m[8]) > 12 || monthFirst && atoi(m[6]) <= 12 {
				day = m[10] + "-" + m[6] + "-" + m[8]
			}
		}
		if _, err := time.Parse(time.DateOnly, day); err == nil && same {
			return day
		}
	}
	return ""
}

// monthFirstDays reads folderNotes.MonthFirst from a folder's videos' names.
func monthFirstDays(names []string) bool {
	american, european := false, false
	for _, name := range names {
		for _, m := range airDateMark.FindAllStringSubmatch(stem(name), -1) {
			if m[1] != "" {
				continue
			}
			american = american || atoi(m[8]) > 12
			european = european || atoi(m[6]) > 12
		}
	}
	return american && !european
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
		(underShows(shelved(file, dir)) || file.Notes.Episodes) && !shelfAt(dir) {
		return true
	}
	if dateFirst(name) && !underShows(shelved(file, file.Rel)) {
		return false
	}
	if before == "" || cameraPrefixes[strings.ToLower(before)] {
		return false
	}
	if recording.MatchString(name) || recordedAt.MatchString(name) {
		// A recorded film is named as a guide lists one, with its year in brackets: "Iron Spire
		// (2011)_Film4_2022_05_01_21_00_00".
		return !bracketYear.MatchString(datedShow(name))
	}
	// So is one recorded loose, "Recorded TV/Grey Harbour (1958) 2022-01-01.ts": in a show's own
	// folder, "Ledger (2019) - 2024-03-18" is Sonarr's daily episode of the 2019 show.
	if recordedFilm(name, dir) {
		return false
	}
	// Loose at the top or in a catch-all ("Downloads/"), a release's name is its own show's:
	// "Evening.Ledger.2024.03.18.720p.WEB"; in a shows' catch-all ("Recordings/", "TV/") any dated
	// name is: "Harbour Watch 2022-01-05 21-00".
	if dir == "." || shelfAt(dir) {
		return showsFolderNames[folderKey(path.Base(dir))] || datedRelease(name)
	}
	show, _, _ := searchQuery(showFolderName(path.Base(seriesFolderOf(file))), 0)
	return relatedName(before, show)
}

// recordedFilm says whether a dated name loose at the top or in a catch-all is a film's recording:
// its year in brackets ends what it names before its day, "Grey Harbour (1958) 2022-01-01".
func recordedFilm(name, dir string) bool {
	return (dir == "." || shelfAt(dir)) && recordedTitle(name) != ""
}

// recordedTitle is a recorded film's name and year from its recording's, "" for none: the name
// before its day ends with its year in brackets, or with its year and the channel it was on,
// "Hallow Point (2009) - Channel 9 - 2023-12-24".
func recordedTitle(name string) string {
	before := strings.TrimSpace(datedShow(name))
	if m := yearThenChannel.FindStringIndex(before); m != nil {
		return strings.TrimSpace(before[:m[0]+len("(2009)")])
	}
	return ""
}

// yearThenChannel matches a film's year in brackets last, or before a channel's name after a dash:
// "(2009)", "(2009) - Channel 9".
var yearThenChannel = regexp.MustCompile(`\((?:19|20)\d{2}\)(?:\s*-\s*[^()]+)?$`)

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
	// A day in brackets, "Kite Hill (2023-01-30)", leaves its bracket behind.
	if show = strings.Trim(strings.Join(strings.Fields(show), " "), " -(["); show != "" {
		return show
	}
	// A recorder that names its recordings by their day first names the show after it, and the
	// time and channel it began: "2023-03-08 Ironfield", "20230308_2100_Channel5_Ironmoor Lane",
	// "20230309 Ironfield - The Well" (dateFirst).
	after := strings.NewReplacer(".", " ", "_", " ").Replace(name[m[1]:])
	after = afterDayNotes.ReplaceAllString(strings.TrimLeft(after, " -"), "")
	if title, _, found := strings.Cut(after, " - "); found {
		after = title
	}
	return strings.Trim(strings.Join(strings.Fields(after), " "), " -")
}

// afterDayNotes matches the time and channel a recorder names after a recording's day: "2100 ",
// "21-00 Channel5 ".
var afterDayNotes = regexp.MustCompile(`(?i)^(?:\d{2}[-:.]?\d{2}(?:[-:.]?\d{2})?[ -]+)?(?:channel[ ]?\d+[ -]+)?`)

// dateFirst says whether a name starts with its day: "2023-03-08 Ironfield". Only a shows' shelf's
// is a show's episode; elsewhere it is a day's video ("Home Videos/2019-06-01 Beach Day.mp4").
func dateFirst(name string) bool {
	at := airDateMark.FindStringIndex(name)
	return at != nil && strings.Trim(leadingTags.ReplaceAllString(name[:at[0]], ""), " ._-") == ""
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
		(!sxxeyy.MatchString(file.Rel) && !crossMark.MatchString(name) || shelfAt(seriesFolderOf(file))) {
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
	if len(segments) > 1 && bracketYear.MatchString(segments[len(segments)-2]) && !sxxeyy.MatchString(name) &&
		sameWords(plainStem(name), segments[len(segments)-2]) {
		return movieRecord(src, segments), ""
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
	// "Christmas_Specials" and "시즌.1" are spelled with their words apart as "Christmas Specials" is.
	name = strings.Join(strings.Fields(strings.NewReplacer("_", " ", ".", " ").Replace(name)), " ")
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
			(next == "" || next == "series" || next == "season" || next == "seasons" || next == "collection")
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
	// "Peppy Cow DVD", "Peppy Cow DVDs", "Peppy Cow Boxset": the copy's words after the name.
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
var discWords = map[string]bool{"dvd": true, "dvds": true, "bluray": true, "blu-ray": true, "boxset": true}

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
// taken off: "[Grp] Hana Kettle", "【Grp】Kettle Hana".
func untagged(name string) string {
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

// trailingEpisodeWord matches an episode's word last in a show's name, before the number it reads:
// "Mizu no Oto Ep", "Mizu no Oto Episode".
var trailingEpisodeWord = regexp.MustCompile(`(?i)[ ._-]+(?:ep|episode)\.?$`)

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
	// An OVA with no number, "[Kumo] Show - OVA", is still the season's; a "Special" alone may be a
	// title's word.
	if m := animeSpecial.FindStringSubmatch(leadingTags.ReplaceAllString(s, "")); m != nil && (tagged || grouped) &&
		(atoi(m[2]) > 0 || m[2] == "" && ovaWord.MatchString(m[0])) {
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
		// A year after a dashed number of two digits or more is the release's, "[Grp] Kaze Tori - 03
		// (2019)", "- 12 (2019)", "- 112 (2019)": a film is never numbered so.
		if m != nil && bracketYear.MatchString(s) && (m[2] == "" || !bracketYear.MatchString(m[1]) && len(m[2]) < 2) {
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
	// "Mizu no Oto Ep 05" is "Mizu no Oto"'s: the word before a number is the number's.
	if bare := trailingEpisodeWord.ReplaceAllString(show, ""); strings.TrimSpace(bare) != "" {
		show = bare
	}
	// "[Grp] Kaze no Brindle (BD 1080p) - 05" is "Kaze no Brindle"'s: its notes are the release's.
	if bare := trimReleaseNotes(show); strings.TrimSpace(bare) != "" {
		show = bare
	}
	show = showSpelling(show)
	// "[Grp] Kame Specials - 01" and "[Grp] Kame OVA - 02" are Kame's season 0's.
	if m := showSpecials.FindStringSubmatch(show); m != nil {
		return strings.TrimSpace(m[1]), mp.Numbering{Season: 0, Episode: number}, true
	}
	// So are "[Grp] Kame - Special - 01" and its "[Grp] Kame - Recap - 06.5": a recap's number is
	// the episode it follows, never its own.
	if m := animeAside.FindStringSubmatch(show); m != nil {
		if strings.EqualFold(m[2], "special") {
			return strings.TrimSpace(m[1]), mp.Numbering{Season: 0, Episode: number}, true
		}
		return strings.TrimSpace(m[1]), mp.Numbering{Season: 0}, true
	}
	if m := animeSeason.FindStringSubmatch(show); m != nil {
		season := atoi(matched(m[2], m[3], m[4]))
		return strings.TrimSpace(m[1]), mp.Numbering{Season: season, Episode: number}, true
	}
	return show, mp.Numbering{Absolute: number}, true
}

var (
	// titledPart matches a miniseries' part with its own title after it: "Wild Shore (2020) - Part
	// 1 - Tides". A film's part never has one.
	titledPart = regexp.MustCompile(`(?i)^(.+?)[ ._]+-[ ._]+part[ ._]*(\d{1,2})[ ._]+-[ ._]+\S`)
	// dashedSeason matches a show's name, then its season and episode apart by a dash: "Lower Wick
	// 1-01".
	dashedSeason = regexp.MustCompile(`^(.+?)[ ._]+(\d{1,2})-(\d{2,3})(?:[ ._]|$)`)
)

// shelvedEpisode is the show and numbering a name gives as a show's file in a shows' catch-all,
// "TV/Lower Wick 1-01.mkv": its season and episode apart by a dash. Anywhere else "1-01" may be a
// title's. An anime's "Shiro Neko - 01-02", after a dash, a group's tag or on an anime's shelf, is
// a batch of episodes from its first (animeRelease).
func shelvedEpisode(name, dir string) (string, mp.Numbering, bool) {
	m := dashedSeason.FindStringSubmatch(plainStem(name))
	if m == nil || dir == "." || !showsFolderNames[folderKey(path.Base(dir))] || atoi(m[3]) == 0 ||
		strings.HasSuffix(strings.TrimSpace(m[1]), "-") || leadingTags.MatchString(m[1]) || animeFolderNames[folderKey(path.Base(dir))] {
		return "", mp.Numbering{}, false
	}
	return showSpelling(m[1]), mp.Numbering{Season: atoi(m[2]), Episode: atoi(m[3])}, true
}

// countedPart is the part a name with no year counts out of its whole, "Wild Moor 1of3" 1: a
// miniseries' episode. 0 for a name that counts none, or gives a year, as a film's part does.
func countedPart(name string) int {
	m := partOf.FindStringSubmatch(name)
	if m == nil || atoi(m[1]) < 1 || atoi(m[1]) > atoi(m[2]) || atoi(m[2]) < 2 {
		return 0
	}
	if _, year, _ := searchQuery(name, 0); year != 0 || bracketYear.MatchString(name) {
		return 0
	}
	return atoi(m[1])
}

// groupTag says whether a bracketed word at a name's end is a release group's: not a release's
// note ("1080p", "HEVC"), an edition ("Extended") or a year.
func groupTag(inner string) bool {
	return !releaseToken(inner) && !releaseNote(inner) && !editionOnly([]string{inner}) && !yearToken.MatchString(inner)
}

// animeSeason matches a fansub show's name that ends in its season: "Show S2", "Show 2nd Season",
// "Show Season 2".
var animeSeason = regexp.MustCompile(`(?i)^(.+?)[ ._]+(?:s(\d{1,2})|(\d{1,2})(?:st|nd|rd|th)[ ._]+season|season[ ._]+(\d{1,2}))$`)

// kodiTitled matches Kodi's episode named by its show, its season and episode in three digits,
// and its title, apart by dashes: "Cutters Bay - 214 - Late".
var kodiTitled = regexp.MustCompile(`^.+?[ ._]+-[ ._]+([1-9]\d{2})[ ._]+-[ ._]+\S`)

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
		} else if showSpecials.MatchString(path.Base(seasonDir)) {
			season = 0
		} else if m := namedSeason.FindStringSubmatch(path.Base(seasonDir)); m != nil {
			season, _ = seasonNumberWord(matched(m[2], m[3]))
		}
	} else if n := namedSeasonOf(show); n > 0 {
		// A show's season folder with no show folder above it, "Puddle Pals Season 2/", is the
		// show's and gives the season.
		season, namedFolder = n, true
	} else if n := packSeason(show); n > 0 && packShow(dir) != "" {
		// So does a season pack's release folder, "Pike.Moss.S02.1080p/".
		season, namedFolder = n, true
	}
	episode := func(e int) (mp.Numbering, bool) { return mp.Numbering{Season: season, Episode: e}, e > 0 }
	// MakeMKV's title, "Kite Shore Disc 1_t01", numbers its disc's titles, never the show's episodes:
	// every title on disc 1 would claim the first. Its length or the catalog places it.
	if makeMKVTitle.MatchString(name) {
		return mp.Numbering{}, false
	}
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
	// On an anime's shelf, an untagged "Mizu no Oto Ep 05" is as "Tsukishiro Kan Episode 09" is.
	if m := episodeWord.FindStringSubmatch(name); m != nil && !leadingTags.MatchString(name) && inAnime(shelved(file, dir)) {
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
	// "Docs/Wild Moor 1of3" is a miniseries' first of three wherever it is; a film's part gives its
	// year, "Ferrow Lane 2 of 3 (2010)".
	if countedPart(name) > 0 {
		return episode(countedPart(name))
	}
	if m := titledPart.FindStringSubmatch(name); m != nil && atoi(m[2]) > 0 {
		return episode(atoi(m[2]))
	}
	if _, n, ok := shelvedEpisode(path.Base(file.Rel), dir); ok {
		return n, true
	}
	// Kodi's "Cutters Bay - 214 - Late" is season 2's fourteenth: three digits between dashes, its
	// episode's title after. An anime's number across its show is padded, "- 014 -", or on its shelf.
	if m := kodiTitled.FindStringSubmatch(name); m != nil && !leadingTags.MatchString(name) && !inAnime(shelved(file, dir)) {
		if n := atoi(m[1]); n%100 > 0 && (!inSeason || n/100 == season) {
			return mp.Numbering{Season: n / 100, Episode: n % 100}, true
		}
	}
	// So is "Fennick 1-05" in Fennick's own folder: the show's name, then its season and episode
	// apart by a dash.
	if m := dashedSeason.FindStringSubmatch(name); m != nil && dir != "." && !inSeason && atoi(m[3]) > 0 &&
		!leadingTags.MatchString(name) && !inAnime(shelved(file, dir)) {
		before, _, _ := searchQuery(m[1], 0)
		if own, _, _ := searchQuery(showFolderName(show), 0); relatedName(before, own) {
			return mp.Numbering{Season: atoi(m[2]), Episode: atoi(m[3])}, true
		}
	}
	if dir == "." {
		// Loose at the top, "Bramblecourt E01" beside "Bramblecourt E02" number one show's episodes.
		// A fansub's "[Kumo] Show - E20" is numbered across its show (looseAnime).
		if m := spelledEpisode.FindStringSubmatch(name); m != nil && file.Notes.Spelled && !leadingTags.MatchString(name) {
			return episode(atoi(m[2]))
		}
		return mp.Numbering{}, false
	}
	// "Tiddlywinks Farm - Series 2 - 05 - Muddy Day" is series 2's fifth.
	if m := seasonDashEpisode.FindStringSubmatch(name); m != nil && atoi(m[2]) > 0 {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	// So is a folder named by its show and season, "Bracken Hall (S07)", but for an anime's pack.
	seasoned := inSeason || namedFolder && !leadingTags.MatchString(path.Base(dir)) && !inAnime(shelved(file, dir))
	if seasoned && season > 0 && !leadingTags.MatchString(name) {
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
	if _, n, ok := animeRelease(path.Base(file.Rel), inAnime(shelved(file, dir))); ok {
		return n, true
	}
	// A catch-all's file ("Movies/Ferrow Lane Part 2 of 3 (2010).mkv") has no show folder to number
	// it after, unless it is one of several numbered after a name they share ("Cartoons/Fizzy Fox
	// 01.mp4" beside "… 02").
	if !inSeason && catchAllFolder(show) && !file.Notes.Numbered && !file.Notes.Spelled {
		return mp.Numbering{}, false
	}
	// "Rook Hall Mini E01" beside "… E02", and "Ash Vale - Night 1" beside "… Night 2".
	if m := spelledEpisode.FindStringSubmatch(name); m != nil && file.Notes.Spelled {
		return episode(atoi(m[2]))
	}
	if m := capMark.FindStringSubmatch(name); m != nil && atoi(m[2]) > 0 && !yearToken.MatchString(m[1]+m[2]) {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, true
	}
	if m := episodeWord.FindStringSubmatch(name); m != nil {
		return episode(atoi(m[1]))
	}
	// "Hollow Sea/E01.mkv": a name that is its episode's mark alone is its folder's show's.
	if m := episodeOnly.FindStringSubmatch(name); m != nil {
		return episode(atoi(m[1]))
	}
	if m := cjkEpisode.FindStringSubmatch(name); m != nil {
		return episode(atoi(matched(m[1], m[2])))
	}
	if m := partOf.FindStringSubmatch(name); m != nil && atoi(m[1]) <= atoi(m[2]) {
		return episode(atoi(m[1]))
	}
	// So do a show's episodes numbered alone in its own folder ("Ocean Halls/01 - The Deep.mkv").
	if m := leadingNumber.FindStringSubmatch(name); (inSeason || namedFolder || file.Notes.Leading) && m != nil {
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
	if !ok && !inSeason && (underShows(shelved(file, dir)) || file.Notes.Series) {
		// A miniseries' "Ridge Point Part 2" and "Ridge Fall - Part Two" are its episodes.
		// Its "Part 2" straight after the show's name is no film's sequel here (partIn's titlesOwnPart).
		if _, part, ok := partIn(name + ".mkv"); ok && part > 0 {
			return episode(part)
		}
		if m := capturedPart.FindStringSubmatch(name); m != nil && atoi(m[1]) > 0 {
			return episode(atoi(m[1]))
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
	// A league's matches are kept as a daily show's episodes are, by their day.
	"sports": true, "sport": true,
}

// amongFilms says whether a folder, relative to the root, is a films' catch-all or in one: the
// nearest catch-all of a kind, itself or above it, is a films' ("Movies/", "Movies/Kids/",
// "Movies/Ferrow Lane (2019)/"), not a shows' ("Movies/TV/Kettle Bay/").
//
// A films' run's folder ("Thistledown Saga/") is among films unless a shows' catch-all holds it: a
// show may be called a saga ("TV/Lisk Saga/").
func amongFilms(dir string) bool {
	run := false
	for d := dir; d != "."; d = path.Dir(d) {
		switch key := folderKey(path.Base(d)); {
		case showsFolderNames[key]:
			return false
		case filmsFolderNames[key]:
			return true
		case filmSeries.MatchString(path.Base(d)):
			run = true
		}
	}
	return run
}

// rootShelf is a root's own name when it is a shelf of a kind or an audience's (walker.shelf): a
// household that adds "/media/TV" and "/media/Movies" as two roots keeps its shows and films apart
// by them, as one root's "TV/" and "Movies/" do.
func rootShelf(root string) string {
	base := filepath.Base(filepath.Clean(root))
	if key := folderKey(base); showsFolderNames[key] || filmsFolderNames[key] || animeFolderNames[key] || audienceNames[key] {
		return base
	}
	return ""
}

// shelved is a folder of a file's, relative to the root, as the shelf predicates (underShows,
// amongFilms, inAnime) read it: under the root's own name when that is a shelf, so "Hop Glen" in a
// root named "TV" is read as "TV/Hop Glen".
func shelved(file File, dir string) string {
	if file.Notes.Root == "" {
		return dir
	}
	return path.Join(file.Notes.Root, dir)
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

// folderKey is a folder's name as the catch-all lists read it: its words, in lower case. A name
// the lists don't hold is read by its words when every one of them is a shelf's (shelfWords): one
// with a films' word is a films' shelf ("Christmas Movies", "Movies - Kids", "Movies2"), one with
// a shows' word a shows' ("TV Show", "TV 2"), and any other a catch-all ("DVD Rips", "Westerns").
func folderKey(name string) string {
	key := strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(strings.ToLower(name))), " ")
	if knownKey(key) {
		return key
	}
	words := strings.Fields(strings.ReplaceAll(key, "-", " "))
	if joined := strings.Join(words, " "); knownKey(joined) {
		return joined
	}
	// "Toddlers" and "Kids & Family" are an audience's shelf, as "Kids" is.
	if !slices.ContainsFunc(words, func(w string) bool { return !audienceWords[w] }) && len(words) > 0 {
		return "kids"
	}
	// A copy's word alone, "4K/" or "BluRay/" in a film's folder, is its version's folder.
	if !slices.ContainsFunc(words, func(w string) bool { return !copyWords[w] }) {
		return key
	}
	kind := "misc"
	for i, w := range words {
		// "Movies2" and "TV 2" are a second shelf of their kind.
		if m := shelfNumber.FindStringSubmatch(w); m != nil && m[1] != "" {
			w = m[1]
		} else if m != nil && i > 0 {
			continue
		}
		switch shelfWords[w] {
		case "":
			return key
		case "films":
			kind = "movies"
		case "shows":
			if kind != "movies" {
				kind = "tv"
			}
		}
	}
	if len(words) == 0 {
		return key
	}
	return kind
}

// knownKey says whether a folder's key is on a shelf list.
func knownKey(key string) bool {
	return catchAllNames[key] || showsFolderNames[key] || filmsFolderNames[key] || animeFolderNames[key] ||
		audienceNames[key] || contentShelves[key]
}

// audienceWords are the words an audience's shelf is named by.
var audienceWords = map[string]bool{
	"kids": true, "children": true, "family": true, "toddler": true, "toddlers": true, "&": true, "and": true,
	"childrens": true, "children's": true, "kid's": true, "kids'": true,
}

// copyWords name one copy's kind, as a film's version folder is named.
var copyWords = map[string]bool{"dvd": true, "bluray": true, "blu": true, "ray": true, "hd": true, "4k": true, "uhd": true}

// shelfNumber matches a shelf's word with its number after it, "movies2", or the number alone.
var shelfNumber = regexp.MustCompile(`^([a-z]*?)\d{1,2}$`)

// shelfWords are the words a household's shelf is named by, and the kind each says: a films',
// a shows', or neither (an audience, a genre, a disc's or a copy's word).
var shelfWords = map[string]string{
	"movies": "films", "movie": "films", "films": "films", "film": "films", "cinema": "films", "kino": "films",
	"filme": "films", "filmes": "films", "filmer": "films", "feature": "films", "features": "films",
	"tv": "shows", "shows": "shows", "show": "shows", "series": "shows", "television": "shows",
	"kids": "any", "children": "any", "family": "any", "toddler": "any", "toddlers": "any", "christmas": "any",
	"holiday": "any", "classic": "any", "classics": "any", "westerns": "any", "western": "any", "disney": "any",
	"pixar": "any", "horror": "any", "action": "any", "animated": "any", "animation": "any", "cartoon": "any",
	"dvd": "any", "dvds": "any", "rips": "any", "ripped": "any", "bluray": "any", "blu": "any", "ray": "any",
	"blurays": "any", "hd": "any", "4k": "any", "uhd": "any", "old": "any", "my": "any", "all": "any", "and": "any",
	"&": "any", "sci": "any", "fi": "any", "scifi": "any", "drama": "any", "thriller": "any", "thrillers": "any",
	"musicals": "any", "foreign": "any", "backup": "any", "backups": "any", "archive": "any", "more": "any",
}

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

// audienceNames are the catch-alls a household sorts by who watches, which hold films and shows
// alike: a folder in one is a film's or a show's by what its videos say.
var audienceNames = map[string]bool{"kids": true, "children": true, "family": true}

// unshelved says whether a folder, relative to the root, has nothing above it but mere catch-alls
// ("Media/Library/Bumbleton/"): nothing says what its videos are but themselves.
func unshelved(rel string) bool {
	for d := path.Dir(rel); d != "."; d = path.Dir(d) {
		if key := folderKey(path.Base(d)); !catchAllNames[key] || audienceNames[key] || contentShelves[key] {
			return false
		}
	}
	return true
}

// contentShelves are the shelves a household keeps videos on that are neither films nor shows:
// each video on one is its own.
var contentShelves = map[string]bool{
	"home videos": true, "home movies": true, "family videos": true, "concerts": true, "music videos": true,
	"sports": true, "workouts": true, "fitness": true, "stand-up": true, "standup": true, "comedy specials": true,
	"youtube": true, "clips": true, "camera": true, "phone": true, "vacation": true, "holidays": true,
}

// onAudienceShelf says whether a folder, relative to the root, is on an audience's shelf: the
// first folder above it that is no mere catch-all ("Media", "Library") is an audience's.
func onAudienceShelf(rel string) bool {
	for d := path.Dir(rel); d != "."; d = path.Dir(d) {
		key := folderKey(path.Base(d))
		if audienceNames[key] {
			return true
		}
		if !catchAllNames[key] {
			return false
		}
	}
	return false
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
			return folderNotes{Numbered: notes.Numbered, Spelled: notes.Spelled}
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
