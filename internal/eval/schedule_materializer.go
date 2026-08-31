//go:build eval

package eval

import (
	"context"
	"fmt"
	"slices"
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
	Materialize(context.Context, Case, suggest.Proposal) ([]MaterializedProgram, error)
}

// MaterializedProgram is the bounded-by-Judge construction input for one
// concrete scheduler output. It contains only grounded scheduling facts.
type MaterializedProgram struct {
	Identity        string   `json:"identity"`
	Title           string   `json:"title,omitempty"`
	Season          int      `json:"season,omitempty"`
	Episode         int      `json:"episode,omitempty"`
	EpisodeEnd      int      `json:"episodeEnd,omitempty"`
	Year            int      `json:"year,omitempty"`
	Rating          string   `json:"rating,omitempty"`
	CommunityRating float64  `json:"communityRating,omitempty"`
	Overview        string   `json:"overview,omitempty"`
	Tags            []string `json:"tags,omitempty"`
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

type snapshotBoundScheduleMaterializer struct {
	inner          ScheduleMaterializer
	caseLibraryIDs map[string]map[provision.Key]string
}

func (m snapshotBoundScheduleMaterializer) Materialize(ctx context.Context, c Case, proposal suggest.Proposal) ([]MaterializedProgram, error) {
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
		out = append(out, x.resolvedProgram())
	}
	return out
}

func libraryEpisodePrograms(e []library.Episode) []schedule.ResolvedProgram {
	out := make([]schedule.ResolvedProgram, 0, len(e))
	for _, x := range e {
		out = append(out, (ScheduleEpisodeEvidence{
			LibraryItemID: x.LibraryItemID, Title: x.Name, DurationMs: x.DurationMs,
			Season: x.Season, Episode: x.Episode, EpisodeEnd: x.EpisodeEnd,
			Year: x.ProductionYear, OfficialRating: x.OfficialRating,
			CommunityRating: x.CommunityRating, Overview: x.Overview, Tags: x.Tags,
		}).resolvedProgram())
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
) ([]MaterializedProgram, error) {
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

func (m liveScheduleMaterializer) Materialize(ctx context.Context, c Case, proposal suggest.Proposal) ([]MaterializedProgram, error) {
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
			titles[key] = FixtureTitle{LibraryItemID: item.LibraryItemID, Episodes: libraryEpisodePrograms(episodes)}
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
func materializeSchedule(c Case, proposal suggest.Proposal, titles map[provision.Key]FixtureTitle) ([]MaterializedProgram, error) {
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
	out := make([]MaterializedProgram, 0, len(desired.Slots))
	for _, slot := range desired.Slots {
		if !slot.IsProgram() {
			continue
		}
		identity := string(slot.Key)
		if slot.Season > 0 || slot.Episode > 0 {
			identity += fmt.Sprintf(":s%02de%02d", slot.Season, slot.Episode)
		}
		program := MaterializedProgram{
			Identity: identity, Title: slot.Title, Season: slot.Season, Episode: slot.Episode,
		}
		if fixture, ok := titles[slot.Key]; ok {
			for _, episode := range fixture.Episodes {
				if episode.LibraryItemID != slot.LibraryItemID {
					continue
				}
				program.EpisodeEnd = episode.EpisodeEnd
				program.Year = episode.Year
				program.Rating = string(episode.OfficialRating)
				program.CommunityRating = episode.CommunityRating
				program.Overview = episode.Overview
				program.Tags = slices.Clone(episode.Tags)
				break
			}
		}
		out = append(out, program)
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

func (a fixtureAvailability) ResolveEpisodes(key provision.Key) schedule.EpisodeResolution {
	fixture, ok := a[key]
	if !ok || len(fixture.Episodes) == 0 {
		return schedule.EpisodeResolution{}
	}
	return schedule.EpisodeResolution{Programs: slices.Clone(fixture.Episodes)}
}
