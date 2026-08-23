//go:build eval

package eval

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
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

type fixtureScheduleMaterializer struct {
	titles map[provision.Key]FixtureTitle
}

func NewFixtureScheduleMaterializer(titles map[provision.Key]FixtureTitle) ScheduleMaterializer {
	return fixtureScheduleMaterializer{titles: titles}
}

func (m fixtureScheduleMaterializer) Materialize(
	_ context.Context, c Case, proposal suggest.Proposal,
) ([]string, error) {
	entries := make([]schedule.LineupEntry, 0, len(proposal.Lineup))
	for _, item := range proposal.Lineup {
		key, err := item.Key()
		if err != nil {
			return nil, fmt.Errorf("schedule proposal item %q: %w", item.Name, err)
		}
		fixture, ok := m.titles[key]
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
	}, entries, fixtureAvailability(m.titles), schedule.PodFill, proposal.Policy, time.Time{})
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
