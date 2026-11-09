package scan

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// The parity report (#2151 decision 8) is the gate before the cutover: what the scanner found on
// disk, compared file by file with what the media server lists. A file pairs by its path (the
// media server's through the path map), so a pairing failure here is one the cutover would hit.

// DurationTolerance is how far apart two durations of one file may be and still agree. Both read
// the container's own duration, so they differ only by rounding.
const DurationTolerance = int64(1000)

// DurationAgreementGate is the share of compared durations that must agree.
const DurationAgreementGate = 0.995

// ParityFile is one file the scanner found, with what its inventory item says.
type ParityFile struct {
	Path, Root string
	Kind       string
	Problem    string
	Name       string
	// ExternalIDs are the title's own ids; SeriesIDs an episode's show's.
	ExternalIDs, SeriesIDs []inventory.ExternalID
	Season, Episode        int
	// DurationMillis is from the file's header, 0 until it is read.
	DurationMillis int64
}

// FindingKind names what disagreed.
type FindingKind string

const (
	FindingServerOnly  FindingKind = "server-only"
	FindingScannerOnly FindingKind = "scanner-only"
	FindingIdentity    FindingKind = "identity"
	FindingDuration    FindingKind = "duration"
)

// Finding is one disagreement. Explained is why it is expected; "" is unexplained, and fails the
// gate (durations fail it only past DurationAgreementGate).
type Finding struct {
	Kind      FindingKind `json:"kind"`
	Path      string      `json:"path"`
	Name      string      `json:"name"`
	Detail    string      `json:"detail,omitempty"`
	Explained string      `json:"explained,omitempty"`
}

// ParityReport is the parity of one library.
type ParityReport struct {
	ServerFiles      int       `json:"serverFiles"`
	ScannerFiles     int       `json:"scannerFiles"`
	Paired           int       `json:"paired"`
	IdentityAgreed   int       `json:"identityAgreed"`
	DurationCompared int       `json:"durationCompared"`
	DurationAgreed   int       `json:"durationAgreed"`
	DurationUnread   int       `json:"durationUnread"`
	Findings         []Finding `json:"findings"`
}

// DurationAgreement is the share of compared durations that agree; 1 when none were compared.
func (r ParityReport) DurationAgreement() float64 {
	if r.DurationCompared == 0 {
		return 1
	}
	return float64(r.DurationAgreed) / float64(r.DurationCompared)
}

// Unexplained counts the presence and identity findings nothing explains.
func (r ParityReport) Unexplained() int {
	n := 0
	for _, f := range r.Findings {
		if f.Explained == "" && f.Kind != FindingDuration {
			n++
		}
	}
	return n
}

// Passed is the gate: no unexplained presence or identity finding, and durations agree.
func (r ParityReport) Passed() bool {
	return r.Unexplained() == 0 && r.DurationAgreement() >= DurationAgreementGate
}

// Parity compares the media server's files with the scanner's. roots are the folders in use.
func Parity(server []library.ServerFile, files []ParityFile, pathMap library.PathMap, roots []string) ParityReport {
	byPath := make(map[string]ParityFile, len(files))
	report := ParityReport{ServerFiles: len(server)}
	for _, f := range files {
		byPath[f.Path] = f
		if f.Problem == "" {
			report.ScannerFiles++
		}
	}
	paired := map[string]bool{}
	for _, s := range server {
		local, mapped := pathMap.Apply(s.Path)
		finding := Finding{Kind: FindingServerOnly, Path: s.Path, Name: s.Name}
		file, found := byPath[local]
		switch {
		case !mapped:
			finding.Explained = "the path map doesn't cover it"
		case !underAny(local, roots):
			finding.Path, finding.Explained = local, "not under a folder in use"
		case !found:
			finding.Path, finding.Detail = local, "the media server lists it; the scanner didn't find it"
		case file.Problem != "":
			finding.Path, finding.Explained = local, "the scanner skips it: "+file.Problem
		default:
			paired[local] = true
			report.Paired++
			report.compare(s, file)
			continue
		}
		report.Findings = append(report.Findings, finding)
	}
	for _, f := range files {
		if f.Problem != "" || paired[f.Path] {
			continue
		}
		report.Findings = append(report.Findings, Finding{Kind: FindingScannerOnly, Path: f.Path, Name: f.Name,
			Detail: "the scanner found it; the media server doesn't list it"})
	}
	slices.SortFunc(report.Findings, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Explained, b.Explained), cmp.Compare(a.Path, b.Path))
	})
	return report
}

// compare checks one paired file's identity and duration.
func (r *ParityReport) compare(s library.ServerFile, f ParityFile) {
	switch verdict, detail := identityVerdict(s, f); verdict {
	case identityAgrees:
		r.IdentityAgreed++
	case identityUngrounded:
		r.Findings = append(r.Findings, Finding{Kind: FindingIdentity, Path: f.Path, Name: s.Name,
			Detail: detail, Explained: "unmatched: it waits in Needs you"})
	case identityRenumbered:
		r.Findings = append(r.Findings, Finding{Kind: FindingIdentity, Path: f.Path, Name: s.Name,
			Detail: detail, Explained: "the same Sonarr episode under another TVDB id"})
	default:
		r.Findings = append(r.Findings, Finding{Kind: FindingIdentity, Path: f.Path, Name: s.Name, Detail: detail})
	}
	switch {
	case f.DurationMillis <= 0:
		r.DurationUnread++
	case s.DurationMillis <= 0:
		// The media server knows no duration: nothing to compare.
	default:
		r.DurationCompared++
		if diff := s.DurationMillis - f.DurationMillis; max(diff, -diff) <= DurationTolerance {
			r.DurationAgreed++
			return
		}
		r.Findings = append(r.Findings, Finding{Kind: FindingDuration, Path: f.Path, Name: s.Name,
			Detail: fmt.Sprintf("media server %dms, scanner %dms", s.DurationMillis, f.DurationMillis)})
	}
}

// parityIdentity is whether two catalogs name a file alike.
type parityIdentity int

const (
	identityAgrees parityIdentity = iota
	identityConflicts
	identityNoCommonID
	identityUngrounded
	// identityRenumbered is one Sonarr episode (both read its id from Sonarr's NFO) that the
	// catalogs hold under different TVDB ids, and nothing else contradicts: one catalog's TVDB data
	// is older. Explained, since neither id names another episode for certain.
	identityRenumbered
)

// identityVerdict says whether the scanner names a file as the media server does. Ids agree when
// they share one and contradict none. An episode agrees by its own ids, or else by its show's ids
// and its numbering (decision: episodes bridge by episode TVDB id).
func identityVerdict(s library.ServerFile, f ParityFile) (parityIdentity, string) {
	own := idsAgree(s.ExternalIDs, f.ExternalIDs)
	if own == identityConflicts {
		detail := "ids differ: media server " + idList(s.ExternalIDs) + ", scanner " + idList(f.ExternalIDs)
		if sameSonarrEpisode(s.ExternalIDs, f.ExternalIDs) &&
			idsAgree(withoutNamespace(s.ExternalIDs, mp.NamespaceTVDB), withoutNamespace(f.ExternalIDs, mp.NamespaceTVDB)) != identityConflicts {
			return identityRenumbered, detail
		}
		return identityConflicts, detail
	}
	if s.Kind != string(kindEpisode) || own == identityAgrees {
		if own == identityAgrees {
			return identityAgrees, ""
		}
		if len(groundingIDs(f.ExternalIDs)) == 0 {
			return identityUngrounded, "the media server has " + idList(s.ExternalIDs)
		}
		return identityNoCommonID, "no id in common: media server " + idList(s.ExternalIDs) + ", scanner " + idList(f.ExternalIDs)
	}
	switch idsAgree(s.SeriesIDs, f.SeriesIDs) {
	case identityConflicts:
		return identityConflicts, "show ids differ: media server " + idList(s.SeriesIDs) + ", scanner " + idList(f.SeriesIDs)
	case identityAgrees:
		if s.Season == f.Season && s.Episode == f.Episode {
			return identityAgrees, ""
		}
		return identityConflicts, fmt.Sprintf("numbering differs: media server S%02dE%02d, scanner S%02dE%02d",
			s.Season, s.Episode, f.Season, f.Episode)
	}
	if len(groundingIDs(f.ExternalIDs)) == 0 && len(groundingIDs(f.SeriesIDs)) == 0 {
		return identityUngrounded, "the media server has " + idList(s.ExternalIDs)
	}
	return identityNoCommonID, "no id in common: media server " + idList(s.ExternalIDs) + " show " + idList(s.SeriesIDs) +
		", scanner " + idList(f.ExternalIDs) + " show " + idList(f.SeriesIDs)
}

// sameSonarrEpisode reports whether a and b carry one Sonarr episode id.
func sameSonarrEpisode(a, b []inventory.ExternalID) bool {
	for _, id := range a {
		if mp.Namespace(id.Namespace) == mp.NamespaceSonarr && slices.Contains(b, id) {
			return true
		}
	}
	return false
}

func withoutNamespace(ids []inventory.ExternalID, ns mp.Namespace) []inventory.ExternalID {
	return slices.DeleteFunc(slices.Clone(ids), func(id inventory.ExternalID) bool { return mp.Namespace(id.Namespace) == ns })
}

// groundingIDs are the ids two catalogs can share: TMDB, TVDB and IMDb.
func groundingIDs(ids []inventory.ExternalID) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, id := range ids {
		if mp.Namespace(id.Namespace).Grounds() {
			if out[id.Namespace] == nil {
				out[id.Namespace] = map[string]bool{}
			}
			out[id.Namespace][id.Value] = true
		}
	}
	return out
}

// idsAgree is identityAgrees when a and b share an id and contradict none, identityConflicts when
// a namespace both have holds different values, and identityNoCommonID otherwise.
func idsAgree(a, b []inventory.ExternalID) parityIdentity {
	left, right := groundingIDs(a), groundingIDs(b)
	shared := false
	for ns, values := range left {
		other, ok := right[ns]
		if !ok {
			continue
		}
		overlap := false
		for v := range values {
			overlap = overlap || other[v]
		}
		if !overlap {
			return identityConflicts
		}
		shared = true
	}
	if shared {
		return identityAgrees
	}
	return identityNoCommonID
}

func idList(ids []inventory.ExternalID) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id.Namespace+":"+id.Value)
	}
	slices.Sort(parts)
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ",")
}

func underAny(path string, roots []string) bool {
	for _, root := range roots {
		if path == root || strings.HasPrefix(path, strings.TrimSuffix(root, "/")+"/") {
			return true
		}
	}
	return false
}

// ParityStore is what ParityFiles reads.
type ParityStore interface {
	LibraryRoots(ctx context.Context) (librarystore.LibraryRoots, error)
	LibraryFiles(ctx context.Context, root string) ([]librarystore.LibraryFile, error)
}

// ParityFiles reads every present file under the folders in use, with what its inventory item
// says; the folders in use; and the path map discovery derived, the media server's rows first.
func ParityFiles(ctx context.Context, st ParityStore, inv inventory.Service) ([]ParityFile, []string, library.PathMap, error) {
	roots, pathMap, err := rootsInUse(ctx, st)
	if err != nil {
		return nil, nil, nil, err
	}
	var out []ParityFile
	for _, root := range roots {
		rows, err := st.LibraryFiles(ctx, root)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, row := range rows {
			if !row.MissingAt.IsZero() {
				continue
			}
			file := ParityFile{Path: row.Path, Root: root, Kind: row.Kind, Problem: row.Problem}
			if row.Problem == "" && row.ItemID != "" {
				if err := file.readItem(ctx, inv, row); err != nil {
					return nil, nil, nil, err
				}
			}
			out = append(out, file)
		}
	}
	return out, roots, pathMap, nil
}

// rootsInUse is the folders in use and the path map discovery derived, the media server's rows
// first: they are the paths a media-server file names.
func rootsInUse(ctx context.Context, st ParityStore) ([]string, library.PathMap, error) {
	all, err := st.LibraryRoots(ctx)
	if err != nil {
		return nil, nil, err
	}
	var roots []string
	for _, r := range all.Roots {
		if r.InUse {
			roots = append(roots, r.Path)
		}
	}
	paths := slices.Clone(all.Paths)
	slices.SortStableFunc(paths, func(a, b librarystore.LibraryRootPath) int {
		return cmp.Compare(appRank(a.Authority), appRank(b.Authority))
	})
	var rules []string
	for _, p := range paths {
		rules = append(rules, p.AuthorityPath+"=>"+p.RootPath)
	}
	return roots, library.ParsePathMap(strings.Join(rules, "\n")), nil
}

// appRank puts the media server's own folder names first: they are the paths the report maps.
func appRank(authority string) int {
	switch authority {
	case string(library.Emby), string(library.Jellyfin):
		return 0
	}
	return 1
}

// readItem fills what the file's inventory item says: its scan origin's facts and its source's
// header duration.
func (f *ParityFile) readItem(ctx context.Context, inv inventory.Service, row librarystore.LibraryFile) error {
	item, ok, err := inv.Item(ctx, inventory.ItemRef{ID: inventory.ItemID(row.ItemID)})
	if err != nil {
		return fmt.Errorf("read inventory item of %s: %w", row.Path, err)
	}
	if !ok {
		return nil
	}
	f.ExternalIDs = item.ExternalIDs
	origin := fileOrigin(row.Root, relTo(row.Root, row.Path))
	for _, o := range item.Origins {
		if o.Key != origin {
			continue
		}
		facts := o.Observation.Facts
		f.Name, f.Season, f.Episode = facts.Name, facts.SeasonNumber, facts.EpisodeNumber
		for _, rel := range facts.Relationships {
			if rel.Kind != string(kindSeries) {
				continue
			}
			if ns, value, ok := strings.Cut(rel.ExternalID, ":"); ok {
				f.SeriesIDs = append(f.SeriesIDs, inventory.ExternalID{Namespace: ns, Value: value})
			}
		}
	}
	for _, source := range item.Sources {
		for _, o := range source.Origins {
			if o.Key == origin && o.Locator.Path == row.Path {
				f.DurationMillis = o.Observation.Facts.DurationMillis
				if source.Measurement != nil && source.Measurement.Revision == source.Revision {
					f.DurationMillis = cmp.Or(source.Measurement.Observation.Facts.DurationMillis, f.DurationMillis)
				}
			}
		}
	}
	return nil
}

// relTo is path within root, with forward slashes, as the walk names it.
func relTo(root, path string) string {
	return strings.TrimPrefix(path, strings.TrimSuffix(root, "/")+"/")
}
