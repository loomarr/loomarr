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
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// ScheduleMaterializer carries a grounded Proposal through Loomarr's real pure
// scheduler and returns the concrete program identities a viewer would receive.
// The live and fixture adapters differ only in where playable metadata comes from.
type ScheduleMaterializer interface {
	Materialize(context.Context, Case, suggest.Proposal) ([]string, error)
}

// FixtureTitle is the hermetic catalog entry used by behavioral schedule cases.
// It supplies the same playable facts the live Library adapter supplies without
// network or inference.
type FixtureTitle struct {
	LibraryItemID string
	DurationMs    int64
	CollectionID  int
	Episodes      []schedule.ResolvedProgram
}

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

func (c LiveScheduleCertification) Case(name string) Case {
	for _, candidate := range c.Cases {
		if candidate.Name == name {
			return candidate
		}
	}
	return Case{}
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
	curated, curatedIDs, err := prepareSeriesEvidence(ctx, snapshot.Curated, schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights}, lib, "schedule_classic_simpsons_highlights")
	if err != nil {
		return LiveScheduleCertification{}, fmt.Errorf("curated schedule evidence: %w", err)
	}
	holiday, holidayIDs, err := prepareSeriesEvidence(ctx, snapshot.Holiday, schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}}, lib, "schedule_owned_simpsons_christmas")
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
		Cases:      []Case{curated, holiday, franchise},
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

func prepareSeriesEvidence(ctx context.Context, evidence ScheduleSeriesEvidence, selection schedule.EpisodeSelection, lib *library.Client, name string) (Case, map[provision.Key]string, error) {
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
	base := Case{Name: name, Intent: Intent{Description: evidence.Name, MustInclude: []string{evidence.Name}}, MinLineup: 1, RequireKeys: []provision.Key{evidence.Key}}
	if selection.Mode == schedule.EpisodeHighlights {
		base.ExpectOrdering = "syndication"
	} else {
		base.ExpectSeasonalMode = "exclusive"
		base.ExpectSeasonalHolidays = slices.Clone(selection.Holidays)
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
	return Case{Name: "schedule_movie_franchise_release_order", Intent: Intent{Description: "Play the owned Indiana Jones movies together in release order", MustInclude: []string{byKey[wantKeys[0]].Name, byKey[wantKeys[1]].Name, byKey[wantKeys[2]].Name}}, MinLineup: 3, MinMovies: 3, RequireKeys: slices.Clone(wantKeys), RequireScheduledPrograms: slices.Clone(canonical), RequireScheduledSequence: slices.Clone(evidence.RequiredSequence)}, bound, nil
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

type snapshotBoundScheduleMaterializer struct {
	inner          ScheduleMaterializer
	caseLibraryIDs map[string]map[provision.Key]string
}

func (m snapshotBoundScheduleMaterializer) Materialize(ctx context.Context, c Case, proposal suggest.Proposal) ([]string, error) {
	allowed, ok := m.caseLibraryIDs[c.Name]
	if !ok {
		return nil, fmt.Errorf("live schedule case %q has no snapshot lineup binding", c.Name)
	}
	for _, item := range proposal.Lineup {
		key, err := item.Key()
		if err != nil {
			return nil, err
		}
		expected, ok := allowed[key]
		if !ok {
			return nil, fmt.Errorf("live schedule case %q proposal contains undeclared lineup key %s", c.Name, key)
		}
		if item.LibraryItemID != expected {
			return nil, fmt.Errorf("schedule proposal %s uses Library item %q, snapshot requires %q", key, item.LibraryItemID, expected)
		}
	}
	return m.inner.Materialize(ctx, c, proposal)
}

func episodeEvidencePrograms(e []ScheduleEpisodeEvidence) []schedule.ResolvedProgram {
	out := make([]schedule.ResolvedProgram, 0, len(e))
	for _, x := range e {
		out = append(out, schedule.ResolvedProgram{LibraryItemID: x.LibraryItemID, Title: x.Title, DurationMs: x.DurationMs, Season: x.Season, Episode: x.Episode, EpisodeEnd: x.EpisodeEnd, Year: x.Year, OfficialRating: schedule.NormalizeRating(x.OfficialRating), CommunityRating: x.CommunityRating, Overview: x.Overview, Tags: slices.Clone(x.Tags)})
	}
	return out
}
func libraryEpisodePrograms(e []library.Episode) []schedule.ResolvedProgram {
	out := make([]schedule.ResolvedProgram, 0, len(e))
	for _, x := range e {
		out = append(out, schedule.ResolvedProgram{LibraryItemID: x.LibraryItemID, Title: x.Name, DurationMs: x.DurationMs, Season: x.Season, Episode: x.Episode, EpisodeEnd: x.EpisodeEnd, Year: x.ProductionYear, OfficialRating: schedule.NormalizeRating(x.OfficialRating), CommunityRating: x.CommunityRating, Overview: x.Overview, Tags: slices.Clone(x.Tags)})
	}
	return out
}
func episodeIdentities(key provision.Key, episodes []schedule.ResolvedProgram) []string {
	out := make([]string, 0, len(episodes))
	for _, e := range episodes {
		out = append(out, fmt.Sprintf("%s:s%02de%02d", key, e.Season, e.Episode))
	}
	return out
}
func keyTMDBID(key provision.Key) int {
	_, provider, id, ok := provision.ParseKey(key)
	if !ok || provider != "tmdb" {
		return 0
	}
	return id
}
func keyStrings(keys []provision.Key) []string {
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = string(key)
	}
	return out
}

type fixtureScheduleMaterializer struct {
	titles map[provision.Key]FixtureTitle
}

func NewFixtureScheduleMaterializer(titles map[provision.Key]FixtureTitle) ScheduleMaterializer {
	return fixtureScheduleMaterializer{titles: titles}
}

func (m fixtureScheduleMaterializer) Materialize(
	_ context.Context, c Case, proposal suggest.Proposal,
) ([]string, error) {
	return materializeSchedule(c, proposal, m.titles)
}

type liveScheduleMaterializer struct {
	library *library.Client
	tmdb    *tmdb.Client
}

// NewLiveScheduleMaterializer hydrates already-owned proposal entries through
// the production Library and TMDB client seams before using the same pure
// projection as fixture evaluation.
func NewLiveScheduleMaterializer(lib *library.Client, tm *tmdb.Client) ScheduleMaterializer {
	return liveScheduleMaterializer{library: lib, tmdb: tm}
}

func (m liveScheduleMaterializer) Materialize(ctx context.Context, c Case, proposal suggest.Proposal) ([]string, error) {
	if m.library == nil || m.tmdb == nil {
		return nil, fmt.Errorf("live schedule materializer requires Library and TMDB clients")
	}
	movieIDs := make([]string, 0, len(proposal.Lineup))
	for _, item := range proposal.Lineup {
		if item.LibraryItemID == "" {
			return nil, fmt.Errorf("schedule lineup item %q has no owned library identity", item.Name)
		}
		if item.MediaType == provision.Movie {
			movieIDs = append(movieIDs, item.LibraryItemID)
		}
	}
	metadata, err := m.library.ItemMetadataByID(ctx, movieIDs)
	if err != nil {
		return nil, fmt.Errorf("load schedule movie metadata: %w", err)
	}
	titles := make(map[provision.Key]FixtureTitle, len(proposal.Lineup))
	for _, item := range proposal.Lineup {
		key, keyErr := item.Key()
		if keyErr != nil {
			return nil, fmt.Errorf("schedule proposal item %q: %w", item.Name, keyErr)
		}
		if item.MediaType == provision.Series {
			episodes, episodeErr := m.library.ListEpisodes(ctx, item.LibraryItemID)
			if episodeErr != nil {
				return nil, fmt.Errorf("load schedule episodes for %s: %w", key, episodeErr)
			}
			resolved := make([]schedule.ResolvedProgram, 0, len(episodes))
			for _, episode := range episodes {
				resolved = append(resolved, schedule.ResolvedProgram{
					LibraryItemID: episode.LibraryItemID, Title: episode.Name, DurationMs: episode.DurationMs,
					Season: episode.Season, Episode: episode.Episode, EpisodeEnd: episode.EpisodeEnd,
					Year: episode.ProductionYear, OfficialRating: schedule.NormalizeRating(episode.OfficialRating),
					CommunityRating: episode.CommunityRating, Overview: episode.Overview, Tags: slices.Clone(episode.Tags),
				})
			}
			titles[key] = FixtureTitle{LibraryItemID: item.LibraryItemID, Episodes: resolved}
			continue
		}
		meta, ok := metadata[item.LibraryItemID]
		if !ok || meta.RuntimeMs <= 0 {
			return nil, fmt.Errorf("schedule movie %s has no playable runtime metadata", key)
		}
		collectionID, collectionErr := m.tmdb.CollectionID(ctx, item.MediaType, item.TMDBID)
		if collectionErr != nil {
			return nil, fmt.Errorf("load TMDB collection for %s: %w", key, collectionErr)
		}
		titles[key] = FixtureTitle{
			LibraryItemID: item.LibraryItemID, DurationMs: meta.RuntimeMs, CollectionID: collectionID,
		}
	}
	return materializeSchedule(c, proposal, titles)
}

// materializeSchedule is the one pure fixture/live projection. All I/O and
// provider-specific payloads are resolved before this boundary.
func materializeSchedule(c Case, proposal suggest.Proposal, titles map[provision.Key]FixtureTitle) ([]string, error) {
	entries := make([]schedule.LineupEntry, 0, len(proposal.Lineup))
	for _, item := range proposal.Lineup {
		key, err := item.Key()
		if err != nil {
			return nil, fmt.Errorf("schedule proposal item %q: %w", item.Name, err)
		}
		fixture, ok := titles[key]
		if !ok {
			return nil, fmt.Errorf("schedule fixture has no playable metadata for %s", key)
		}
		entries = append(entries, schedule.LineupEntry{
			Key: key, Title: item.Name, DurationMs: fixture.DurationMs,
			SeasonMin: item.SeasonMin, SeasonMax: item.SeasonMax,
			EpisodeSelection: item.EpisodeSelection,
			OfficialRating:   schedule.NormalizeRating(item.OfficialRating),
			Genres:           item.Genres, Year: item.Year, CollectionID: fixture.CollectionID,
		})
	}

	desired := schedule.ComputeDesiredAt(schedule.Channel{
		ID: "eval-" + c.Name, Strategy: schedule.Sequential, Shuffle: schedule.ShuffleParams{Seed: 1},
	}, entries, fixtureAvailability(titles), schedule.PodFill, proposal.Policy, time.Time{})
	out := make([]string, 0, len(desired.Slots))
	for _, slot := range desired.Slots {
		if !slot.IsProgram() {
			continue
		}
		identity := string(slot.Key)
		if slot.Season > 0 || slot.Episode > 0 {
			identity += fmt.Sprintf(":s%02de%02d", slot.Season, slot.Episode)
		}
		out = append(out, identity)
	}
	return out, nil
}

type fixtureAvailability map[provision.Key]FixtureTitle

func (a fixtureAvailability) Resolve(key provision.Key) (string, int64, bool) {
	fixture, ok := a[key]
	if !ok || key.IsSeries() || fixture.LibraryItemID == "" || fixture.DurationMs <= 0 {
		return "", 0, false
	}
	return fixture.LibraryItemID, fixture.DurationMs, true
}

func (a fixtureAvailability) ResolveEpisodes(key provision.Key) ([]schedule.ResolvedProgram, bool) {
	fixture, ok := a[key]
	if !ok || len(fixture.Episodes) == 0 {
		return nil, false
	}
	return slices.Clone(fixture.Episodes), true
}

func scheduledChecks(c Case, programs []string) []string {
	if len(c.RequireScheduledPrograms) == 0 && len(c.ForbidScheduledPrograms) == 0 && len(c.RequireScheduledSequence) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(programs))
	for _, program := range programs {
		seen[program] = true
	}
	var failures []string
	for _, required := range c.RequireScheduledPrograms {
		if !seen[required] {
			failures = append(failures, fmt.Sprintf("required scheduled program %q is missing", required))
		}
	}
	for _, forbidden := range c.ForbidScheduledPrograms {
		if seen[forbidden] {
			failures = append(failures, fmt.Sprintf("forbidden scheduled program %q is present", forbidden))
		}
	}
	if len(c.RequireScheduledSequence) > 0 && !containsSequence(programs, c.RequireScheduledSequence) {
		failures = append(failures, fmt.Sprintf("required scheduled sequence %v is missing from %v", c.RequireScheduledSequence, programs))
	}
	return failures
}

func containsSequence(programs, sequence []string) bool {
	if len(sequence) > len(programs) {
		return false
	}
	for start := 0; start+len(sequence) <= len(programs); start++ {
		if slices.Equal(programs[start:start+len(sequence)], sequence) {
			return true
		}
	}
	return false
}
