package app

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// upcomingGuide is the guide's walk the upcoming index reads (playoutresolve.Resolver).
type upcomingGuide interface {
	UpcomingProgrammes(ctx context.Context, channelID string, from, to time.Time) ([]playoutresolve.UpcomingProgramme, error)
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
	_, err = u.store.ReplaceUpcomingAirings(ctx, ch.ID, ch.Revision, airings)
	return err
}
