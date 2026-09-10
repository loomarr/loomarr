// Package proposaloutlook explains an exact pending proposal using read-only
// Library observations and the same channel planner and scheduler as approval.
package proposaloutlook

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// Library is the existing media adapter's read-only metadata boundary.
type Library interface {
	ItemMetadataByID(context.Context, []string) (map[string]library.ItemMetadata, error)
}

// Titles observes arrivals already recorded by the normal provisioning path.
type Titles interface {
	GetTitle(context.Context, provision.Key) (provision.Record, error)
}

type observation struct {
	programs map[provision.Key][]schedule.ResolvedProgram
	unknown  map[provision.Key]bool
	missing  map[provision.Key]bool
}

func (o observation) Resolve(key provision.Key) (string, int64, bool) {
	programs := o.programs[key]
	if key.IsSeries() || len(programs) == 0 {
		return "", 0, false
	}
	return programs[0].LibraryItemID, programs[0].DurationMs, true
}

func (o observation) ResolveEpisodes(key provision.Key) schedule.EpisodeResolution {
	return schedule.EpisodeResolution{Programs: o.programs[key], EditorialUnavailable: o.unknown[key]}
}

func observe(ctx context.Context, titles Titles, lib Library, resolveEpisodes channels.EpisodeResolver, proposal suggest.Proposal, lineup []schedule.LineupEntry) observation {
	out := observation{programs: map[provision.Key][]schedule.ResolvedProgram{}, unknown: map[provision.Key]bool{}, missing: map[provision.Key]bool{}}
	ids := map[provision.Key]string{}
	for _, item := range proposal.Lineup {
		if key, err := item.Key(); err == nil && item.InLibrary && item.LibraryItemID != "" && ids[key] == "" {
			ids[key] = item.LibraryItemID
		}
	}
	requested := make([]string, 0, len(lineup))
	for _, entry := range lineup {
		if titles != nil {
			record, err := titles.GetTitle(ctx, entry.Key)
			if err == nil && record.State == provision.Available && record.LibraryID != "" {
				ids[entry.Key] = record.LibraryID
			} else if err != nil && !errors.Is(err, store.ErrNotFound) {
				out.unknown[entry.Key] = true
			}
		}
		if ids[entry.Key] != "" {
			requested = append(requested, ids[entry.Key])
		} else {
			out.missing[entry.Key] = true
		}
	}
	if lib == nil {
		for _, entry := range lineup {
			out.unknown[entry.Key] = true
		}
		return out
	}
	metadata, metadataErr := lib.ItemMetadataByID(ctx, requested)
	for _, entry := range lineup {
		id := ids[entry.Key]
		if id == "" {
			continue
		}
		meta, found := metadata[id]
		if !found {
			if metadataErr != nil {
				out.unknown[entry.Key] = true
			} else {
				out.missing[entry.Key] = true
			}
			continue
		}
		if !entry.Key.IsSeries() {
			if meta.RuntimeMs > 0 {
				out.programs[entry.Key] = []schedule.ResolvedProgram{{LibraryItemID: id, DurationMs: meta.RuntimeMs}}
			} else {
				out.unknown[entry.Key] = true
			}
			continue
		}
		if resolveEpisodes == nil {
			out.unknown[entry.Key] = true
			continue
		}
		episodes, err := resolveEpisodes(ctx, id)
		if err != nil {
			out.unknown[entry.Key] = true
			continue
		}
		for _, episode := range episodes {
			if episode.LibraryItemID == "" || episode.DurationMs <= 0 {
				out.unknown[entry.Key] = true
				continue
			}
			out.programs[entry.Key] = append(out.programs[entry.Key], episode)
		}
		if len(out.programs[entry.Key]) == 0 && !out.unknown[entry.Key] {
			out.missing[entry.Key] = true
		}
	}
	return out
}
