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
	// episodeMark matches an episode's mark: "S01E02", Emby's "S01xE02", and "1x02". The episodes
	// after it in a file holding several are moreEpisodes'.
	episodeMark = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:s(\d{1,4})[ ._-]?x?e(\d{1,4})|(\d{1,2})x(\d{2,3})(?:[^0-9]|$))`)
	// moreEpisodes match each further episode of a file holding several, after its mark, in every
	// style the apps write (Sonarr's six, Plex's, Emby's): one joined on ("E03", "xE03", "x03"),
	// one after a dash or plus ("-E03", "+E03", " - S01E03", "-x03", " - 1x03"), or a bare number
	// straight after a dash ("-03"). A bare number is two or three digits and ends there, so
	// "S01E02-1080p" and "S01E02-720p" hold one episode.
	moreEpisodesS = regexp.MustCompile(`(?i)^(?:[ ._]*[-+&][ ._]*|[ ._]?)(?:s(\d{1,4})[ ._-]?)?x?e(\d{1,4})`)
	moreEpisodesX = regexp.MustCompile(`(?i)^(?:[ ._]*[-+&][ ._]*(\d{1,2})x|-?x)(\d{2,3})(?:[^0-9]|$)`)
	moreBare      = regexp.MustCompile(`(?i)^-(\d{2,3})(?:[^0-9a-z]|$)`)
	// airDateMark matches the day a daily or weekly show's episode aired: "2013-02-15",
	// "2013.02.15", "2013 02 15", "20130215", and Plex's day first, "15-02-2013" and "15.02.2013".
	// The two separators are one character, the same both times.
	airDateMark = regexp.MustCompile(`(?:^|[^0-9])(?:((?:19|20)\d{2})([-. ]?)(\d{2})([-. ]?)(\d{2})|(\d{2})([-.])(\d{2})([-.])((?:19|20)\d{2}))(?:[^0-9]|$)`)
)

// airDateIn is the day a file's name says its episode aired, YYYY-MM-DD; "" for none. A day that
// never was, such as "2024-02-30" or a number that only looks like one, is none.
func airDateIn(name string) string {
	for _, m := range airDateMark.FindAllStringSubmatch(name, -1) {
		day, same := m[1]+"-"+m[3]+"-"+m[5], m[2] == m[4]
		if m[1] == "" {
			day, same = m[10]+"-"+m[8]+"-"+m[6], m[7] == m[9]
		}
		if _, err := time.Parse(time.DateOnly, day); err == nil && same {
			return day
		}
	}
	return ""
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
	}
	// Each further episode is the next one on, in the same season: a file runs through its last.
	last := n.Episode
	for {
		rest := name[at:]
		next := more.FindStringSubmatchIndex(rest)
		if next == nil {
			next = moreBare.FindStringSubmatchIndex(rest)
			if next != nil {
				next = []int{next[0], next[1], -1, -1, next[2], next[3]}
			}
		}
		if next == nil {
			break
		}
		season, episode := n.Season, atoi(rest[next[4]:next[5]])
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
	// loose at the root's top has no show to belong to.
	if day := airDateIn(name); day != "" && len(segments) > 1 {
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
	// its absolute number.
	if show, abs, ok := looseAnime(name); ok && len(segments) == 1 {
		return mp.Record{Source: src, Kind: mp.KindEpisode, Numbering: &mp.Numbering{Absolute: abs}}, show
	}
	// A file with no mark in a show's season folder ("Kettle Bay/Specials/Holiday Special.mkv") is
	// one of its episodes: the catalog, or its own NFO, places it. A season folder with no show
	// folder above it names no show, so a file there is still a movie.
	if dir := path.Dir(file.Rel); dir != "." && isSeasonFolder(dir) {
		if seriesDir := aboveSeasons(dir); seriesDir != "." {
			return mp.Record{Source: src, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}, seriesDir
		}
	}
	// A movie is named by its file or its own folder. A folder further up is a collection or a
	// series, and a tvdb id never names a movie.
	rec := mp.Record{Source: src, Kind: mp.KindMovie}
	if edition := editionIn(name); edition != "" {
		rec.Descriptive = &mp.Descriptive{Edition: edition}
	}
	for i := len(segments) - 1; i >= 0 && i >= len(segments)-2 && len(rec.IDs) == 0; i-- {
		for _, id := range idsIn(segments[i]) {
			if id.Namespace != mp.NamespaceTVDB {
				rec.IDs = append(rec.IDs, id)
			}
		}
	}
	return rec, ""
}

var (
	// seasonFolder matches a season folder's name: "Season 1", "Season 01", "S01", "Specials",
	// "Series 1" and its other languages' words ("Staffel 1", "Saison 1", "Temporada 1"), with any
	// bracketed notes after it ("Season 2 [1080p]").
	seasonFolder = regexp.MustCompile(`(?i)^(?:(?:season|series|staffel|saison|temporada|stagione)[ ._-]*\d+|s\d+|specials)` +
		`(?:[ ._-]*[\[(][^\])]*[\])])*$`)
	// namedSeason matches a season folder that starts with its show's name: "Kettle Bay Season 1".
	namedSeason = regexp.MustCompile(`(?i)^(.+?)[ ._-]+(?:season|series|staffel|saison|temporada|stagione)[ ._-]*\d+` +
		`(?:[ ._-]*[\[(][^\])]*[\])])*$`)
)

// isSeasonFolder says whether dir, relative to the root, is a season folder. One that starts with a
// show's name is one only inside that show's folder: "Kettle Bay/Kettle Bay Season 1" is, while a
// "Kettle Bay Season 1" folder of its own, with no show folder above it, is the show's.
func isSeasonFolder(dir string) bool {
	name := path.Base(dir)
	if seasonFolder.MatchString(name) {
		return true
	}
	m := namedSeason.FindStringSubmatch(name)
	return m != nil && path.Dir(dir) != "." && sameWords(m[1], path.Base(path.Dir(dir)))
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
		return looseSeries(name)
	}
	// A catch-all folder ("Downloads/Complete", "Media", "TV/All Shows") names no show: an episode
	// there that names its own is that show's, as one loose at the top is. A catch-all is known by
	// its name, never by its files naming several shows: a show's own folder does that too, when
	// the show was renamed or its files spell it two ways.
	if show := looseSeries(name); catchAllFolder(path.Base(dir)) && show != "" {
		return show
	}
	if isSeasonFolder(dir) {
		if show := aboveSeasons(dir); show != "." {
			return show
		}
		return looseSeries(name)
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

// showFolderName is a show's name from its folder: the folder's own name, less its ids, or, for a
// release folder, its words before what the folder holds of the show ("Kettle.Bay.S02.1080p-GRP" and
// "Kettle Bay Complete Series (2010-2015)" are "Kettle Bay").
func showFolderName(folder string) string {
	name := strings.TrimSpace(embeddedID.ReplaceAllString(folder, ""))
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
	// seasonEpisodeWords matches a mark written out: "Season 1 Episode 2", "Season 1 Ep 2".
	seasonEpisodeWords = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:season|series|staffel|saison|temporada)[ ._-]*(\d{1,4})[ ._-]*(?:episode|ep)[ ._-]*(\d{1,4})(?:[^0-9]|$)`)
	// episodeWord matches an episode with no season: "Ep02", "Episode 2", "Ep. 2".
	episodeWord = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:episode|ep)[ ._-]*(\d{1,4})(?:[^0-9]|$)`)
	// partOf matches a miniseries' "2of6", "2 of 6", "Part 2 of 6".
	partOf = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,3})[ ._-]*of[ ._-]*(\d{1,3})(?:[^0-9a-z]|$)`)
	// leadingNumber matches a name that starts with its episode's number: "05 - Tidewater".
	leadingNumber = regexp.MustCompile(`^(\d{1,3})(?:[ ._-]|$)`)
	// seasonNumber is a season folder's number: "Season 2", "S02", "Kettle Bay Series 2".
	seasonNumber = regexp.MustCompile(`(?i)(?:season|series|staffel|saison|temporada|stagione|^s)[ ._-]*(\d+)`)
	// leadingTags are a release group's bracketed tags before a name: "[Kumo] ", "[Kumo][Show]".
	leadingTags = regexp.MustCompile(`^(?:[ ._]*\[[^\]]*\])+[ ._]*`)
	// animeNumber matches an anime release's absolute episode after its show's name, " - 07" or
	// " - 013v2", ahead of its notes or its end.
	animeNumber = regexp.MustCompile(`^(.+?)[ ._]+-[ ._]+(\d{1,4})(?:v\d+)?(?:[ ._]*$|[ ._]*[\[(]|[ ._]+-)`)
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
// "[Group][Show][07][1080p]": its show and its absolute episode. Without the group's tag a name is
// not read so: "Show - 07" could be a title of its own.
func looseAnime(name string) (string, int, bool) {
	s := stem(name)
	if m := animeBrackets.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1]), atoi(m[2]), atoi(m[2]) > 0
	}
	if !leadingTags.MatchString(s) {
		return "", 0, false
	}
	m := animeNumber.FindStringSubmatch(leadingTags.ReplaceAllString(s, ""))
	if m == nil || atoi(m[2]) == 0 {
		return "", 0, false
	}
	return strings.TrimSpace(m[1]), atoi(m[2]), true
}

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
	if isSeasonFolder(dir) {
		inSeason, show = true, path.Base(aboveSeasons(dir))
		if strings.EqualFold(path.Base(dir), "specials") {
			season = 0
		} else if m := seasonNumber.FindStringSubmatch(path.Base(dir)); m != nil {
			season = atoi(m[1])
		}
	}
	episode := func(e int) (mp.Numbering, bool) { return mp.Numbering{Season: season, Episode: e}, e > 0 }
	if m := seasonEpisodeWords.FindStringSubmatch(name); m != nil {
		return mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}, atoi(m[2]) > 0
	}
	if _, abs, ok := looseAnime(name); ok {
		return mp.Numbering{Absolute: abs}, true
	}
	if m := episodeWord.FindStringSubmatch(name); m != nil {
		return episode(atoi(m[1]))
	}
	if m := partOf.FindStringSubmatch(name); m != nil && atoi(m[1]) <= atoi(m[2]) {
		return episode(atoi(m[1]))
	}
	if m := leadingNumber.FindStringSubmatch(name); inSeason && m != nil {
		return episode(atoi(m[1]))
	}
	return numberAfterShow(name, showFolderName(show), file.Notes.Numbered)
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
	"anime": true, "new": true,
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
	return notes
}

// numberedName matches a name and the three- or four-digit number after it: "kb 101".
var numberedName = regexp.MustCompile(`^(.*[^0-9 ._-])[ ._-]+(\d{3,4})$`)
