package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// fillerBreakAdapter keeps planned breaks in the store: the pod is the JSON document the store holds
// beside the break's channel, start, slot length and media length.
type fillerBreakAdapter struct{ st store.Store }

func (a fillerBreakAdapter) SaveBreaks(ctx context.Context, breaks []pod.StoredBreak) error {
	rows := make([]clipstore.FillerBreak, 0, len(breaks))
	for _, b := range breaks {
		doc, err := json.Marshal(b.Pod)
		if err != nil {
			return fmt.Errorf("encode planned break: %w", err)
		}
		rows = append(rows, clipstore.FillerBreak{ChannelID: b.ChannelID, Start: b.Start, Ceiling: b.Ceiling,
			Length: b.Length(), PodJSON: string(doc), PlannedAt: b.PlannedAt})
	}
	return a.st.SaveFillerBreaks(ctx, rows)
}

func (a fillerBreakAdapter) LoadBreaks(ctx context.Context, channelID string, from, to time.Time) ([]pod.StoredBreak, error) {
	rows, err := a.st.ListFillerBreaks(ctx, channelID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]pod.StoredBreak, 0, len(rows))
	for _, r := range rows {
		var p pod.Pod
		if err := json.Unmarshal([]byte(r.PodJSON), &p); err != nil {
			return nil, fmt.Errorf("decode planned break %s@%d: %w", r.ChannelID, r.Start.UnixMilli(), err)
		}
		out = append(out, pod.StoredBreak{ChannelID: r.ChannelID, Start: r.Start, Ceiling: r.Ceiling, Pod: p, PlannedAt: r.PlannedAt})
	}
	return out, nil
}

func (a fillerBreakAdapter) KeepBreaks(ctx context.Context, channelID string, starts []time.Time) error {
	return a.st.KeepFillerBreaks(ctx, channelID, starts)
}
