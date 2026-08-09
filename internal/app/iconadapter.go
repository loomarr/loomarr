package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// iconMaxSuggestions caps how many candidate icons a channel offers (§icon P2). A
// lineup can run to dozens of titles; the icon picker is a handful of visual choices,
// not an exhaustive catalog, so we stop well short of the full lineup.
const iconMaxSuggestions = 12

// iconLogoWidth is the rung a suggestion's `url` points at. It matches the w500 these URLs used
// to carry from TMDB directly, which is the size a channel tile wants and the size Tunarr gets
// pushed — the difference is that the bytes are now ours and the other rungs exist alongside it.
const iconLogoWidth = 500

// iconFetchBudget bounds the synchronous warm-up below.
//
// ⚠ It is a ceiling on the whole pass, not a per-image timeout, because what it protects is the
// operator's request: twelve posters fetched concurrently should take about one origin round-trip,
// and if TMDB is having a bad day the picker must still come back promptly with whatever landed
// rather than holding the connection open for the slowest of twelve.
const iconFetchBudget = 3 * time.Second

// iconAdapter maps the store + tmdb.Client + the image service to api.IconService (§icon P2):
// candidate channel-icon posters drawn from the channel's OWN lineup, so a Star Trek channel
// offers its five series' posters instead of a generic placeholder.
type iconAdapter struct {
	store  store.Store
	tmdb   *tmdb.Client
	images *images.Service
	fetch  *images.Fetcher
	log    *slog.Logger
}

// IconSuggestions reads the channel's lineup, resolves each entry's TMDB/TVDB id to a poster on
// TMDB, adopts those posters into the image service, and returns the ones whose bytes have landed.
//
// ⚠ **A suggestion is only offered once the image has REAL BYTES, and that is a correctness rule
// rather than a quality bar.** `Adopt` keys a row on a hash of the source URL, and the fetch
// re-keys it to the content hash the moment the bytes arrive — deleting the placeholder row. A
// suggestion's `url` is what the operator's pick becomes the channel's `logo`, which is stored and
// later pushed to Tunarr. Emitting a placeholder-hash URL would therefore mint a logo that works
// for under a minute and 404s forever after. Waiting for the content hash is the only way the URL
// we hand out is the URL that keeps resolving.
//
// The consequence is visible and acceptable: a cold picker can come back with fewer than twelve.
// `images-fetch` runs every minute, so re-opening it fills in. That is a strictly better failure
// than a full grid of icons that quietly rot.
//
// Best-effort per title throughout (§icon): one TMDB or adopt failure is logged and skipped rather
// than failing the whole request — one bad title must not deny every other suggestion.
func (a iconAdapter) IconSuggestions(ctx context.Context, channelID string) ([]api.IconSuggestion, error) {
	ch, err := a.store.GetChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}

	candidates := a.posterCandidates(ctx, ch, channelID)
	if len(candidates) == 0 {
		return []api.IconSuggestion{}, nil
	}

	// ⚠ No image service means no suggestions, NOT a fallback to TMDB's CDN. Removing third-party
	// origins from the operator's browser is what §22 is for, so a degraded install shows the
	// channel's designed empty state (its monogram) rather than quietly restoring the beacon.
	if a.images == nil || a.fetch == nil {
		return []api.IconSuggestion{}, nil
	}

	adopted := make([]images.Image, 0, len(candidates))
	for _, c := range candidates {
		// Public, because Tunarr fetches a channel icon machine-to-machine with no credentials
		// (§22 — visibility is a property of the image, not the route).
		rec, err := a.images.Adopt(ctx, c.src, images.IngestRequest{
			Role:       images.RoleIcon,
			Visibility: images.VisibilityPublic,
		})
		if err != nil {
			if a.log != nil {
				a.log.Warn("icon suggestion: adopt failed, skipping title", "channel", channelID, "url", c.src, "err", err)
			}
			continue
		}
		adopted = append(adopted, rec)
	}

	warm := a.fetch.FetchNow(ctx, adopted, iconFetchBudget)

	out := make([]api.IconSuggestion, 0, len(candidates))
	for _, c := range candidates {
		rec, ok := warm[c.src]
		if !ok {
			continue // still cold — the every-minute job owns it; the next open will show it
		}
		out = append(out, api.IconSuggestion{
			Title:     c.title,
			URL:       a.images.URLFor(rec.Hash, iconLogoWidth, images.FormatJPEG),
			ImageHash: rec.Hash,
		})
	}
	return out, nil
}

// posterCandidate is one lineup title paired with the TMDB URL its poster lives at.
type posterCandidate struct{ title, src string }

// posterCandidates resolves a channel's lineup to deduplicated TMDB poster URLs, capped at
// iconMaxSuggestions. Split out so IconSuggestions reads as the three phases it actually has —
// resolve, adopt, warm — rather than one loop doing all three.
func (a iconAdapter) posterCandidates(ctx context.Context, ch store.Channel, channelID string) []posterCandidate {
	seen := make(map[string]struct{}, len(ch.Lineup))
	out := make([]posterCandidate, 0, iconMaxSuggestions)
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
		out = append(out, posterCandidate{title: entry.Title, src: posterURL})
	}
	return out
}
