package channels

import (
	"context"
	"time"
)

// ForecastSaved exposes one side of ScheduleDiffDraft's walk — the saved channel's — so the
// external tests can hold it against what the encoder airs, not only against the other side.
func (e *Engine) ForecastSaved(ctx context.Context, channelID string, from, to time.Time) ([]ForecastAiring, error) {
	ch, err := e.store.GetChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out, _, err := e.forecast(ctx, ch, from.In(e.now().Location()), to)
	return out, err
}
