package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

type discoveryFeedbackSource struct{ store store.DiscoveryFeedbackStore }

func (s discoveryFeedbackSource) Signals(ctx context.Context, intent suggest.Intent) ([]suggest.FeedbackSignal, error) {
	household, err := s.store.ListDiscoveryFeedback(ctx, store.FeedbackFilter{Scope: store.FeedbackHousehold})
	if err != nil {
		return nil, err
	}
	var channel []store.DiscoveryFeedback
	if intent.DiscoveryScopeID != "" {
		channel, err = s.store.ListDiscoveryFeedback(ctx, store.FeedbackFilter{
			Scope: store.FeedbackChannel, ScopeID: intent.DiscoveryScopeID,
		})
		if err != nil {
			return nil, err
		}
	}
	events := store.EffectiveDiscoveryFeedback(household, channel)
	out := make([]suggest.FeedbackSignal, 0, len(events))
	for _, event := range events {
		out = append(out, suggest.FeedbackSignal{Target: event.Target, Action: suggest.FeedbackAction(event.Action)})
	}
	return out, nil
}
