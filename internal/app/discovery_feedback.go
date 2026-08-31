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
	// Resolve tombstones within each scope before applying channel-over-household
	// precedence. A channel clear removes that channel override; it must not also
	// hide an otherwise-effective household signal for the same target.
	effective := make(map[string]bool, len(channel)+len(household))
	out := make([]suggest.FeedbackSignal, 0, len(channel)+len(household))
	appendScope := func(events []store.DiscoveryFeedback) {
		seen := make(map[string]bool, len(events))
		for _, event := range events {
			key := string(event.Target)
			if seen[key] {
				continue
			}
			seen[key] = true
			if event.Action == store.FeedbackClear || effective[key] {
				continue
			}
			effective[key] = true
			out = append(out, suggest.FeedbackSignal{Target: event.Target, Action: suggest.FeedbackAction(event.Action)})
		}
	}
	appendScope(channel)
	appendScope(household)
	return out, nil
}
