package store

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
)

func TestEffectiveDiscoveryFeedbackResolvesReplacementOverrideAndFallback(t *testing.T) {
	target := provision.Key("movie:tmdb:90000603")
	other := provision.Key("series:tvdb:90071663")
	household := []DiscoveryFeedback{
		{ID: "house-new", Scope: channeldatastore.FeedbackHousehold, Target: target, Action: channeldatastore.FeedbackKeep},
		{ID: "house-old", Scope: channeldatastore.FeedbackHousehold, Target: target, Action: channeldatastore.FeedbackLess},
		{ID: "house-other", Scope: channeldatastore.FeedbackHousehold, Target: other, Action: channeldatastore.FeedbackNever},
	}
	channel := []DiscoveryFeedback{
		{ID: "channel-clear", Scope: channeldatastore.FeedbackChannel, ScopeID: "ch-1", Target: target, Action: channeldatastore.FeedbackClear, CreatedAt: time.Unix(3, 0)},
		{ID: "channel-old", Scope: channeldatastore.FeedbackChannel, ScopeID: "ch-1", Target: target, Action: channeldatastore.FeedbackSurprise, CreatedAt: time.Unix(2, 0)},
	}

	got := channeldatastore.EffectiveDiscoveryFeedback(household, channel)
	if len(got) != 2 || got[0].ID != "house-new" || got[1].ID != "house-other" {
		t.Fatalf("effective feedback after channel clear = %+v, want household fallback and unaffected household event", got)
	}

	channel[0] = DiscoveryFeedback{ID: "channel-replace", Scope: channeldatastore.FeedbackChannel, ScopeID: "ch-1", Target: target, Action: channeldatastore.FeedbackLess}
	got = channeldatastore.EffectiveDiscoveryFeedback(household, channel)
	if len(got) != 2 || got[0].ID != "channel-replace" || got[1].ID != "house-other" {
		t.Fatalf("effective feedback after channel replacement = %+v, want channel override then household remainder", got)
	}
}
