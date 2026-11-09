package scan

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
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
func indexManagers(ctx context.Context, managers []Manager, paths []store.LibraryRootPath) (managerIndex, []error) {
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
func pathMapper(authority string, paths []store.LibraryRootPath) func(string) string {
	type row struct{ from, to string }
	var rows []row
	for _, p := range paths {
		if p.Authority == authority && p.RootPath != "" {
			rows = append(rows, row{from: strings.TrimSuffix(path.Clean(filepath.ToSlash(p.AuthorityPath)), "/"), to: p.RootPath})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return len(rows[i].from) > len(rows[j].from) })
	return func(arrPath string) string {
		p := path.Clean(filepath.ToSlash(arrPath))
		for _, r := range rows {
			if p == r.from {
				return r.to
			}
			if strings.HasPrefix(p, r.from+"/") {
				return filepath.Join(r.to, filepath.FromSlash(strings.TrimPrefix(p, r.from+"/")))
			}
		}
		return ""
	}
}

var (
	// embeddedID matches an id written into a name: "{tmdb-1234}", "[imdbid-tt0001234]", "{tvdb-5678}".
	embeddedID = regexp.MustCompile(`(?i)[\[{](tmdb|tmdbid|imdb|imdbid|tvdb|tvdbid)[-=]([a-z0-9]+)[\]}]`)
	// episodeMark matches "S01E02", "S01E02E03", "S01E02-E03" and "1x02".
	episodeMark = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,4})[ ._-]?e(\d{1,4})(?:[ ._-]?-?e(\d{1,4}))?|(?:^|[^a-z0-9])(\d{1,2})x(\d{2,3})(?:[^0-9]|$)`)
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

// numberingIn parses an episode's season and episode from its file name.
func numberingIn(name string) (mp.Numbering, bool) {
	m := episodeMark.FindStringSubmatch(name)
	if m == nil {
		return mp.Numbering{}, false
	}
	if m[1] != "" {
		n := mp.Numbering{Season: atoi(m[1]), Episode: atoi(m[2])}
		if m[3] != "" && atoi(m[3]) > n.Episode {
			n.EpisodeEnd = atoi(m[3])
		}
		return n, true
	}
	return mp.Numbering{Season: atoi(m[4]), Episode: atoi(m[5])}, true
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
	if n, ok := numberingIn(name); ok {
		rec := mp.Record{Source: src, Kind: mp.KindEpisode, Numbering: &n}
		seriesDir := seriesFolder(file.Rel)
		if seriesDir != "" {
			rec.Parent = idsIn(path.Base(seriesDir))
		}
		return rec, seriesDir
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

// seasonFolder matches a season folder's name: "Season 1", "Season 01", "S01", "Specials".
var seasonFolder = regexp.MustCompile(`(?i)^(season[ ._-]*\d+|s\d+|specials)$`)

// seriesFolder is an episode's series folder relative to the root: the folder above its season
// folder, else the nearest folder whose name carries an id, else its own folder. An episode with
// no series folder, loose at the root's top or in a season folder there, takes the folder its show
// would have: its name before the episode mark. "" is the root, for one with no such name.
func seriesFolder(rel string) string {
	dir := path.Dir(rel)
	if dir == "." {
		return looseSeries(path.Base(rel))
	}
	if seasonFolder.MatchString(path.Base(dir)) {
		if parent := path.Dir(dir); parent != "." {
			return parent
		}
		return looseSeries(path.Base(rel))
	}
	for d := dir; d != "."; d = path.Dir(d) {
		if len(idsIn(path.Base(d))) > 0 {
			return d
		}
	}
	return dir
}

// looseSeries is the show an episode with no series folder names before its episode mark:
// "Example.Show.S01E02.mkv" is "Example Show", so loose episodes of two shows are two shows.
// With nothing before the mark it is the root, "": a season folder never names a show.
func looseSeries(name string) string {
	mark := episodeMark.FindStringIndex(name)
	if mark == nil {
		return ""
	}
	show := strings.NewReplacer(".", " ", "_", " ").Replace(name[:mark[0]])
	return strings.Trim(strings.Join(strings.Fields(show), " "), " -")
}
