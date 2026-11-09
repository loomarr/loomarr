package store

import (
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// The channel cluster deletes a channel's own discovery feedback by scope name, and cannot import
// the store's FeedbackChannel; this pins the two values equal.
func TestChannelStoreFeedbackScopeIsTheStoresChannelScope(t *testing.T) {
	if channelstore.FeedbackChannelScope != string(FeedbackChannel) {
		t.Fatalf("channelstore.FeedbackChannelScope = %q, want store.FeedbackChannel %q",
			channelstore.FeedbackChannelScope, FeedbackChannel)
	}
}
