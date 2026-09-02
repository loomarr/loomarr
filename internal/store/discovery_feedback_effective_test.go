package store

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

func TestEffectiveDiscoveryFeedbackResolvesReplacementOverrideAndFallback(t *testing.T) {
	target := provision.Key("movie:tmdb:603")
	other := provision.Key("series:tvdb:71663")
	household := []DiscoveryFeedback{
		{ID: "house-new", Scope: FeedbackHousehold, Target: target, Action: FeedbackKeep},
		{ID: "house-old", Scope: FeedbackHousehold, Target: target, Action: FeedbackLess},
		{ID: "house-other", Scope: FeedbackHousehold, Target: other, Action: FeedbackNever},
	}
	channel := []DiscoveryFeedback{
		{ID: "channel-clear", Scope: FeedbackChannel, ScopeID: "ch-1", Target: target, Action: FeedbackClear, CreatedAt: time.Unix(3, 0)},
		{ID: "channel-old", Scope: FeedbackChannel, ScopeID: "ch-1", Target: target, Action: FeedbackSurprise, CreatedAt: time.Unix(2, 0)},
	}

	got := EffectiveDiscoveryFeedback(household, channel)
	if len(got) != 2 || got[0].ID != "house-new" || got[1].ID != "house-other" {
		t.Fatalf("effective feedback after channel clear = %+v, want household fallback and unaffected household event", got)
	}

	channel[0] = DiscoveryFeedback{ID: "channel-replace", Scope: FeedbackChannel, ScopeID: "ch-1", Target: target, Action: FeedbackLess}
	got = EffectiveDiscoveryFeedback(household, channel)
	if len(got) != 2 || got[0].ID != "channel-replace" || got[1].ID != "house-other" {
		t.Fatalf("effective feedback after channel replacement = %+v, want channel override then household remainder", got)
	}
}
