package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/resolve"
)

// upcomingGuide is the guide's walk the upcoming index reads (resolve.Resolver).
type upcomingGuide interface {
	UpcomingProgrammes(ctx context.Context, channelID string, from, to time.Time) ([]resolve.UpcomingProgramme, error)
}

// upcomingWriter is the one store write the upcoming index makes.
type upcomingWriter interface {
	ReplaceUpcomingAirings(ctx context.Context, channelID string, revision int64, airings []channeldatastore.UpcomingAiring) (bool, error)
}

// upcomingIndex is the engine's UpcomingIndexer (#4476): it reads a committed channel's
// programmes from the guide's own walk and replaces the channel's stored upcoming airings with
// them, at the revision reconcile committed. A newer revision committed meanwhile wins: its own
// reconcile writes its rows.
type upcomingIndex struct {
	guide upcomingGuide
	store upcomingWriter
	log   *slog.Logger
}

func (u upcomingIndex) IndexUpcoming(ctx context.Context, ch channelstore.Channel, from, to time.Time) error {
	programmes, err := u.guide.UpcomingProgrammes(ctx, ch.ID, from, to)
	if err != nil {
		return err
	}
	airings := make([]channeldatastore.UpcomingAiring, 0, len(programmes))
	for _, p := range programmes {
		airings = append(airings, channeldatastore.UpcomingAiring{
			ChannelID: ch.ID, Start: p.Start, End: p.Stop, LibraryItemID: p.LibraryItemID, TitleKey: p.Key, Forecast: p.Forecast,
		})
	}
	return u.replace(ctx, ch, airings)
}

// ClearUpcoming replaces the channel's stored airings with none, through the same guarded write.
func (u upcomingIndex) ClearUpcoming(ctx context.Context, ch channelstore.Channel) error {
	return u.replace(ctx, ch, nil)
}

func (u upcomingIndex) replace(ctx context.Context, ch channelstore.Channel, airings []channeldatastore.UpcomingAiring) error {
	wrote, err := u.store.ReplaceUpcomingAirings(ctx, ch.ID, ch.Revision, airings)
	if err == nil && !wrote && u.log != nil {
		u.log.Debug("upcoming airings: a newer revision won, nothing written", "channel", ch.ID, "revision", ch.Revision)
	}
	return err
}
