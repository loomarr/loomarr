//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/tmdb"
)

const (
	scheduleEvidenceSchemaVersion = 1
	liveScheduleSeriesKey         = provision.Key("series:tmdb:456")
)

// ScheduleEvidenceSnapshot is the declared, versioned real-Library evidence
// required before live schedule certification may construct inference providers.
type ScheduleEvidenceSnapshot struct {
	SchemaVersion int                       `json:"schemaVersion"`
	SnapshotID    string                    `json:"snapshotId"`
	Curated       ScheduleSeriesEvidence    `json:"curated"`
	Holiday       ScheduleSeriesEvidence    `json:"holiday"`
	Franchise     ScheduleFranchiseEvidence `json:"franchise"`
}

type ScheduleSeriesEvidence struct {
	Key               provision.Key             `json:"key"`
	Name              string                    `json:"name"`
	LibraryItemID     string                    `json:"libraryItemId"`
	Episodes          []ScheduleEpisodeEvidence `json:"episodes"`
	RequiredPrograms  []string                  `json:"requiredPrograms"`
	ForbiddenPrograms []string                  `json:"forbiddenPrograms"`
}

type ScheduleEpisodeEvidence struct {
	LibraryItemID   string   `json:"libraryItemId"`
	Title           string   `json:"title"`
	DurationMs      int64    `json:"durationMs"`
	Season          int      `json:"season"`
	Episode         int      `json:"episode"`
	EpisodeEnd      int      `json:"episodeEnd,omitempty"`
	Year            int      `json:"year,omitempty"`
	OfficialRating  string   `json:"officialRating,omitempty"`
	CommunityRating float64  `json:"communityRating,omitempty"`
	Overview        string   `json:"overview,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

// resolvedProgram is the single normalized projection shared by declared
// snapshot evidence and the live Library adapter. Source-field differences are
// handled before this seam; scheduler facts and defensive copies are not.
func (e ScheduleEpisodeEvidence) resolvedProgram() schedule.ResolvedProgram {
	return schedule.ResolvedProgram{
		LibraryItemID: e.LibraryItemID, Title: e.Title, DurationMs: e.DurationMs,
		Season: e.Season, Episode: e.Episode, EpisodeEnd: e.EpisodeEnd, Year: e.Year,
		OfficialRating:  schedule.NormalizeRating(e.OfficialRating),
		CommunityRating: e.CommunityRating, Overview: e.Overview, Tags: slices.Clone(e.Tags),
	}
}

type ScheduleMovieEvidence struct {
	Key           provision.Key `json:"key"`
	Name          string        `json:"name"`
	LibraryItemID string        `json:"libraryItemId"`
	DurationMs    int64         `json:"durationMs"`
	CollectionID  int           `json:"collectionId"`
}

type ScheduleFranchiseEvidence struct {
	Movies           []ScheduleMovieEvidence `json:"movies"`
	RequiredSequence []string                `json:"requiredSequence"`
}

// LiveScheduleCertification is the complete result of evidence preflight.
// Callers cannot obtain live cases without also obtaining the evidence-bound
// materializer that checks the generated Proposal against the same snapshot.
type LiveScheduleCertification struct {
	SnapshotID   string
	Cases        []Case
	Materializer ScheduleMaterializer
}

func (c LiveScheduleCertification) Case(name string) (Case, error) {
	for _, candidate := range c.Cases {
		if candidate.Name == name {
			return candidate, nil
		}
	}
	return Case{}, fmt.Errorf("live schedule certification case %q is not declared", name)
}

// LoadScheduleEvidence reads the closed snapshot format. Unknown fields and
// trailing JSON are rejected so a version never silently changes meaning.
func LoadScheduleEvidence(path string) (ScheduleEvidenceSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return ScheduleEvidenceSnapshot{}, fmt.Errorf("open live schedule evidence: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var snapshot ScheduleEvidenceSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return ScheduleEvidenceSnapshot{}, fmt.Errorf("decode live schedule evidence: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return ScheduleEvidenceSnapshot{}, fmt.Errorf("decode live schedule evidence: trailing JSON value")
	} else if err != io.EOF {
		return ScheduleEvidenceSnapshot{}, fmt.Errorf("decode live schedule evidence: %w", err)
	}
	return snapshot, nil
}

// PrepareLiveScheduleCertification verifies every scheduling-relevant snapshot
// fact and pinned oracle against public Library/TMDB seams without invoking the
// scheduler. It returns no provider dependency and necessarily completes before inference.
func PrepareLiveScheduleCertification(ctx context.Context, snapshot ScheduleEvidenceSnapshot, lib *library.Client, tm *tmdb.Client) (LiveScheduleCertification, error) {
	if snapshot.SchemaVersion != scheduleEvidenceSchemaVersion || !validScheduleSnapshotID(snapshot.SnapshotID) {
		return LiveScheduleCertification{}, fmt.Errorf("live schedule evidence requires schemaVersion %d and a snapshotId", scheduleEvidenceSchemaVersion)
	}
	if snapshot.Curated.Key != liveScheduleSeriesKey || snapshot.Holiday.Key != liveScheduleSeriesKey {
		return LiveScheduleCertification{}, fmt.Errorf("live curated and holiday evidence must both use Key %s", liveScheduleSeriesKey)
	}
	if lib == nil || tm == nil {
		return LiveScheduleCertification{}, fmt.Errorf("live schedule evidence requires Library and TMDB clients")
	}
	curated, curatedIDs, err := prepareSeriesEvidence(ctx, snapshot.Curated, schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights}, lib, "schedule_classic_simpsons_highlights", classicSimpsonsViewerRequest)
	if err != nil {
		return LiveScheduleCertification{}, fmt.Errorf("curated schedule evidence: %w", err)
	}
	holiday, holidayIDs, err := prepareSeriesEvidence(ctx, snapshot.Holiday, schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}}, lib, "schedule_owned_simpsons_christmas", simpsonsChristmasViewerRequest)
	if err != nil {
		return LiveScheduleCertification{}, fmt.Errorf("holiday schedule evidence: %w", err)
	}
	franchise, franchiseIDs, err := prepareFranchiseEvidence(ctx, snapshot.Franchise, lib, tm)
	if err != nil {
		return LiveScheduleCertification{}, fmt.Errorf("franchise schedule evidence: %w", err)
	}
	bound := make(map[provision.Key]string, len(curatedIDs)+len(holidayIDs)+len(franchiseIDs))
	for _, identities := range []map[provision.Key]string{curatedIDs, holidayIDs, franchiseIDs} {
		for key, id := range identities {
			if existing, ok := bound[key]; ok && existing != id {
				return LiveScheduleCertification{}, fmt.Errorf("live schedule evidence binds %s to inconsistent Library item ids", key)
			}
			bound[key] = id
		}
	}
	return LiveScheduleCertification{
		SnapshotID: snapshot.SnapshotID,
		Cases:      withProductionStructuralBounds([]Case{curated, holiday, franchise}),
		Materializer: snapshotBoundScheduleMaterializer{
			inner: NewLiveScheduleMaterializer(lib, tm),
			caseLibraryIDs: map[string]map[provision.Key]string{
				curated.Name:   curatedIDs,
				holiday.Name:   holidayIDs,
				franchise.Name: franchiseIDs,
			},
		},
	}, nil
}

func prepareSeriesEvidence(ctx context.Context, evidence ScheduleSeriesEvidence, selection schedule.EpisodeSelection, lib *library.Client, name, viewerRequest string) (Case, map[provision.Key]string, error) {
	if !evidence.Key.IsSeries() || keyTMDBID(evidence.Key) == 0 || evidence.Name == "" || evidence.LibraryItemID == "" || len(evidence.Episodes) == 0 {
		return Case{}, nil, fmt.Errorf("series identity and complete episode evidence are required")
	}
	if err := verifyLibraryOwnershipBinding(ctx, lib, evidence.Key, evidence.LibraryItemID); err != nil {
		return Case{}, nil, err
	}
	live, err := lib.ListEpisodes(ctx, evidence.LibraryItemID)
	if err != nil {
		return Case{}, nil, err
	}
	want := episodeEvidencePrograms(evidence.Episodes)
	got := libraryEpisodePrograms(live)
	if !reflect.DeepEqual(got, want) {
		return Case{}, nil, fmt.Errorf("snapshot %s drifted from Library episode evidence", evidence.Key)
	}
	base := Case{Name: name, Intent: Intent{Description: viewerRequest, MustInclude: []string{evidence.Name}}, MinLineup: 1, RequireKeys: []provision.Key{evidence.Key}}
	if selection.Mode == schedule.EpisodeHighlights {
		base.ExpectOrdering = "syndication"
		base.JudgeRubric = "A good viewer outcome is a coherent curated sample of the pinned owned series, using the concrete emitted episode identities and avoiding the pinned exclusions."
		base.MinJudgeScore = 0.65
		base.MinRelevanceScore = 0.7
		base.MinSerendipityScore = 0.3
	} else {
		base.ExpectSeasonalMode = "exclusive"
		base.ExpectSeasonalHolidays = slices.Clone(selection.Holidays)
		base.JudgeRubric = "A good viewer outcome is an owned, playable Christmas episode sample whose concrete emitted identity fits the holiday request and avoids every pinned non-holiday episode."
		base.MinJudgeScore = 0.7
		base.MinRelevanceScore = 0.75
		base.MinSerendipityScore = 0.2
	}
	all := episodeIdentities(evidence.Key, want)
	if err := validatePinnedEpisodeExpectations(all, evidence.RequiredPrograms, evidence.ForbiddenPrograms); err != nil {
		return Case{}, nil, err
	}
	base.RequireScheduledPrograms = slices.Clone(evidence.RequiredPrograms)
	base.ForbidScheduledPrograms = slices.Clone(evidence.ForbiddenPrograms)
	return base, map[provision.Key]string{evidence.Key: evidence.LibraryItemID}, nil
}

func prepareFranchiseEvidence(ctx context.Context, evidence ScheduleFranchiseEvidence, lib *library.Client, tm *tmdb.Client) (Case, map[provision.Key]string, error) {
	movies := evidence.Movies
	if len(movies) != 3 {
		return Case{}, nil, fmt.Errorf("exactly three Indiana Jones films are required")
	}
	wantKeys := []provision.Key{"movie:tmdb:85", "movie:tmdb:87", "movie:tmdb:89"}
	byKey := make(map[provision.Key]ScheduleMovieEvidence, len(movies))
	for _, movie := range movies {
		byKey[movie.Key] = movie
	}
	ids := make([]string, 0, len(movies))
	bound := make(map[provision.Key]string, len(movies))
	for _, key := range wantKeys {
		movie, ok := byKey[key]
		if !ok || movie.Name == "" || movie.LibraryItemID == "" || movie.CollectionID != 84 {
			return Case{}, nil, fmt.Errorf("missing or invalid Indiana Jones evidence for %s", key)
		}
		if err := verifyLibraryOwnershipBinding(ctx, lib, key, movie.LibraryItemID); err != nil {
			return Case{}, nil, err
		}
		ids = append(ids, movie.LibraryItemID)
		bound[key] = movie.LibraryItemID
	}
	metadata, err := lib.ItemMetadataByID(ctx, ids)
	if err != nil {
		return Case{}, nil, err
	}
	for _, key := range wantKeys {
		movie := byKey[key]
		if metadata[movie.LibraryItemID].RuntimeMs != movie.DurationMs || movie.DurationMs <= 0 {
			return Case{}, nil, fmt.Errorf("snapshot %s runtime drifted from Library", key)
		}
		collectionID, collectionErr := tm.CollectionID(ctx, provision.Movie, keyTMDBID(key))
		if collectionErr != nil {
			return Case{}, nil, collectionErr
		}
		if collectionID != movie.CollectionID {
			return Case{}, nil, fmt.Errorf("snapshot %s collection drifted from TMDB", key)
		}
	}
	canonical := keyStrings(wantKeys)
	if !slices.Equal(evidence.RequiredSequence, canonical) {
		return Case{}, nil, fmt.Errorf("franchise requiredSequence must pin canonical release order %v", canonical)
	}
	return Case{Name: "schedule_movie_franchise_release_order", Intent: Intent{Description: "Play the owned Indiana Jones movies together in release order", MustInclude: []string{byKey[wantKeys[0]].Name, byKey[wantKeys[1]].Name, byKey[wantKeys[2]].Name}}, MinLineup: 3, MinMovies: 3, RequireKeys: slices.Clone(wantKeys), RequireScheduledPrograms: slices.Clone(canonical), RequireScheduledSequence: slices.Clone(evidence.RequiredSequence), JudgeRubric: "A good viewer outcome is the complete pinned owned Indiana Jones trilogy emitted atomically in canonical release order, with no interleaved title.", MinJudgeScore: 0.7, MinRelevanceScore: 0.75, MinSerendipityScore: 0.2}, bound, nil
}

func verifyLibraryOwnershipBinding(ctx context.Context, lib *library.Client, key provision.Key, expectedLibraryItemID string) error {
	mediaType, provider, id, ok := provision.ParseKey(key)
	if !ok || provider != string(library.TMDB) || id <= 0 {
		return fmt.Errorf("live schedule evidence Key %s is not an exact TMDB identity", key)
	}
	libraryMediaType := library.Movie
	if mediaType == provision.Series {
		libraryMediaType = library.Series
	}
	detail, present, err := lib.LookupDetail(ctx, library.TMDB, strconv.Itoa(id), libraryMediaType)
	if err != nil {
		return fmt.Errorf("verify Library ownership binding for %s: %w", key, err)
	}
	if !present {
		return fmt.Errorf("verify Library ownership binding for %s: title is not present", key)
	}
	if detail.ID != expectedLibraryItemID {
		return fmt.Errorf("verify Library ownership binding for %s: Library returned item %q, snapshot declares %q", key, detail.ID, expectedLibraryItemID)
	}
	return nil
}

func validatePinnedEpisodeExpectations(present, required, forbidden []string) error {
	if len(required) == 0 || len(forbidden) == 0 {
		return fmt.Errorf("pinned episode expectations require nonempty included and excluded identities")
	}
	seen := make(map[string]string, len(required)+len(forbidden))
	for label, programs := range map[string][]string{"requiredPrograms": required, "forbiddenPrograms": forbidden} {
		for _, program := range programs {
			if !slices.Contains(present, program) {
				return fmt.Errorf("%s identity %q is not present in live episode evidence", label, program)
			}
			if previous, ok := seen[program]; ok {
				return fmt.Errorf("episode identity %q is duplicated or both %s and %s", program, previous, label)
			}
			seen[program] = label
		}
	}
	if len(seen) != len(present) {
		return fmt.Errorf("pinned episode expectations must classify every present episode as included or excluded")
	}
	return nil
}

func validScheduleSnapshotID(id string) bool {
	if id == "" || id != strings.TrimSpace(id) || len(id) > 96 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
