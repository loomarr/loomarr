package scan

import (
	"context"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/store"
)

// Manager is one arr that knows which files are which items: Sonarr or Radarr, listed whole on
// every pass. Authority is its name in the path map.
type Manager struct {
	Authority string
	Lister    mp.Lister
}

// managed is what one arr says one local file is. Records holds one record per episode in the
// file, lowest first; Series is the episode's series.
type managed struct {
	records []mp.Record
	series  *mp.Record
}

// managerIndex maps a local path to what an arr says it is.
type managerIndex struct {
	byPath map[string]managed
	// complete is false when any configured arr failed to list, so the absence of a file from the
	// index proves nothing.
	complete bool
}

// indexManagers lists every arr and keys its files by local path through the path map.
func indexManagers(ctx context.Context, managers []Manager, paths []store.LibraryRootPath) (managerIndex, []error) {
	idx := managerIndex{byPath: map[string]managed{}, complete: true}
	var errs []error
	for _, m := range managers {
		records, err := m.Lister.List(ctx)
		if err != nil {
			idx.complete = false
			errs = append(errs, err)
		}
		local := pathMapper(m.Authority, paths)
		for i := range records {
			rec := records[i]
			switch rec.Kind {
			case mp.KindMovie:
				idx.addFiles(local, rec.Files, rec, nil)
			case mp.KindSeries:
				series := rec
				series.Children = nil
				for _, ep := range rec.Children {
					idx.addFiles(local, ep.Files, ep, &series)
				}
			}
		}
	}
	for p, m := range idx.byPath {
		sort.SliceStable(m.records, func(i, j int) bool { return episodeOrder(m.records[i]) < episodeOrder(m.records[j]) })
		idx.byPath[p] = m
	}
	return idx, errs
}

func (idx managerIndex) addFiles(local func(string) string, files []mp.FileRef, rec mp.Record, series *mp.Record) {
	for _, f := range files {
		p := local(f.Path)
		if p == "" {
			continue
		}
		m := idx.byPath[p]
		m.records = append(m.records, rec)
		if series != nil {
			m.series = series
		}
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
	if n, ok := numberingIn(name); ok && !file.Extra {
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
// folder, else the nearest folder whose name carries an id, else its own folder. "" is the root.
func seriesFolder(rel string) string {
	dir := path.Dir(rel)
	if dir == "." {
		return ""
	}
	if seasonFolder.MatchString(path.Base(dir)) {
		if parent := path.Dir(dir); parent != "." {
			return parent
		}
	}
	for d := dir; d != "."; d = path.Dir(d) {
		if len(idsIn(path.Base(d))) > 0 {
			return d
		}
	}
	return dir
}
