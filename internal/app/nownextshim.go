package app

import (
	"context"
	"time"

	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/app/nownext"
)

// guideReader implements guideapi.GuideReader over the now/next router, mapping its entries to the
// API's.
type guideReader struct{ router nownext.Router }

func (g guideReader) NowNext(ctx context.Context, now time.Time) (map[string]guideapi.ChannelNowNext, error) {
	byChannel, err := g.router.NowNext(ctx, now)
	if err != nil {
		return nil, err
	}
	out := make(map[string]guideapi.ChannelNowNext, len(byChannel))
	for id, nn := range byChannel {
		out[id] = guideapi.ChannelNowNext{Now: toGuideEntry(nn.Now), Next: toGuideEntry(nn.Next)}
	}
	return out, nil
}

func (g guideReader) Upcoming(ctx context.Context, channelID string, now time.Time, limit int) ([]guideapi.NowNextEntry, error) {
	entries, err := g.router.Upcoming(ctx, channelID, now, limit)
	if err != nil {
		return nil, err
	}
	out := make([]guideapi.NowNextEntry, len(entries))
	for i, e := range entries {
		out[i] = guideapi.NowNextEntry(e)
	}
	return out, nil
}

func toGuideEntry(e *nownext.Entry) *guideapi.NowNextEntry {
	if e == nil {
		return nil
	}
	entry := guideapi.NowNextEntry(*e)
	return &entry
}
