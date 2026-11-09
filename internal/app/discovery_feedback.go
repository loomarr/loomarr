package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
)

type discoveryFeedbackSource struct{ store store.DiscoveryFeedbackStore }

func (s discoveryFeedbackSource) Signals(ctx context.Context, intent proposals.Intent) ([]suggest.FeedbackSignal, error) {
	household, err := s.store.ListDiscoveryFeedback(ctx, channeldatastore.FeedbackFilter{Scope: channeldatastore.FeedbackHousehold})
	if err != nil {
		return nil, err
	}
	var channel []channeldatastore.DiscoveryFeedback
	if intent.DiscoveryScopeID != "" {
		channel, err = s.store.ListDiscoveryFeedback(ctx, channeldatastore.FeedbackFilter{
			Scope: channeldatastore.FeedbackChannel, ScopeID: intent.DiscoveryScopeID,
		})
		if err != nil {
			return nil, err
		}
	}
	events := channeldatastore.EffectiveDiscoveryFeedback(household, channel)
	out := make([]suggest.FeedbackSignal, 0, len(events))
	for _, event := range events {
		out = append(out, suggest.FeedbackSignal{Target: event.Target, Action: suggest.FeedbackAction(event.Action)})
	}
	return out, nil
}
