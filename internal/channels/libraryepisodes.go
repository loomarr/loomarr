package channels

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/episodeevidence"
	"github.com/loomarr/loomarr/internal/platform/rating"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// LibraryEpisodeResolver expands a series lineup entry into the Library's episodes (§9, #2151
// step 16), mapping each to a schedule.ResolvedProgram so the schedule domain stays free of a
// library dependency. An episode that could not play is dropped and its editorial evidence is
// bounded. An id that is no library series expands to nothing: the media server is never asked.
func LibraryEpisodeResolver(st store.LibraryEpisodeReader) EpisodeResolver {
	return func(ctx context.Context, showItemID string) ([]schedule.ResolvedProgram, error) {
		rows, _, err := st.LibraryEpisodes(ctx, showItemID)
		if err != nil {
			return nil, err
		}
		out := make([]schedule.ResolvedProgram, 0, len(rows))
		for _, r := range rows {
			if episodeevidence.ValidatePlayable(r.ItemID, r.DurationMs, r.Season, r.Episode, r.EpisodeEnd) != nil {
				continue
			}
			evidence := episodeevidence.Sanitize(r.Score, r.Overview, r.Tags)
			// The catalog's premiere date is a calendar day; anything else is unknown, not an error.
			airDate, _ := time.Parse(time.DateOnly, r.PremiereDate)
			out = append(out, schedule.ResolvedProgram{
				LibraryItemID: r.ItemID,
				Title:         r.Name,
				DurationMs:    r.DurationMs,
				Season:        r.Season,
				Year:          r.Year,
				AirDate:       airDate,
				Episode:       r.Episode,
				EpisodeEnd:    r.EpisodeEnd, // §5 multi-part: single-file span end
				// Normalized HERE, at the edge, exactly as the binder does for a lineup entry —
				// so the schedule domain only ever compares ladder values (§4).
				OfficialRating:  rating.Normalize(r.Rating),
				CommunityRating: evidence.CommunityRating,
				Overview:        evidence.Overview,
				Tags:            evidence.Tags,
			})
		}
		return out, nil
	}
}
