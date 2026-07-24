package app

import (
	"context"
	"log/slog"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// iconMaxSuggestions caps how many candidate icons a channel offers (§icon P2). A
// lineup can run to dozens of titles; the icon picker is a handful of visual choices,
// not an exhaustive catalog, so we stop well short of the full lineup.
const iconMaxSuggestions = 12

// iconAdapter maps the store + tmdb.Client to api.IconService (§icon P2): candidate
// channel-icon posters drawn from the channel's OWN lineup, so a Star Trek channel
// offers its five series' posters instead of a generic placeholder. Thin, wiring-only
// — the resolution logic (key → poster) lives entirely here since neither the store
// nor tmdb.Client know about each other.
type iconAdapter struct {
	store store.Store
	tmdb  *tmdb.Client
	log   *slog.Logger
}

// IconSuggestions reads the channel's lineup and resolves each entry's TMDB/TVDB id to
// a poster URL. Best-effort per title (§icon): a single TMDB failure is logged and
// skipped rather than failing the whole request — one bad title must not deny every
// other suggestion. Results are de-duplicated by URL (a channel can carry the same
// title under more than one lineup entry) and capped at iconMaxSuggestions.
func (a iconAdapter) IconSuggestions(ctx context.Context, channelID string) ([]api.IconSuggestion, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(ch.Lineup))
	out := make([]api.IconSuggestion, 0, iconMaxSuggestions)
	for _, entry := range ch.Lineup {
		if len(out) >= iconMaxSuggestions {
			break
		}
		mt, provider, id, ok := provision.ParseKey(entry.Key)
		if !ok {
			continue // malformed/legacy key — nothing to resolve, skip
		}

		var posterURL string
		var perr error
		switch provider {
		case "tmdb":
			posterURL, perr = a.tmdb.PosterURL(ctx, mt, id)
		case "tvdb":
			posterURL, perr = a.tmdb.PosterURLByTVDB(ctx, id)
		default:
			continue // unrecognized provider — ParseKey only emits tmdb/tvdb today, but stay defensive
		}
		if perr != nil {
			// A per-title TMDB error (rate limit, transient 5xx, …) must not fail the whole
			// suggestions list — log it and move on to the next title (§icon best-effort).
			if a.log != nil {
				a.log.Warn("icon suggestion: poster lookup failed, skipping title", "channel", channelID, "key", entry.Key, "err", perr)
			}
			continue
		}
		if posterURL == "" {
			continue // no poster on TMDB for this title — a legitimate, non-error result
		}
		if _, dup := seen[posterURL]; dup {
			continue
		}
		seen[posterURL] = struct{}{}
		out = append(out, api.IconSuggestion{Title: entry.Title, URL: posterURL})
	}
	return out, nil
}
