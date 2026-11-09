package store

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
)

// What hangs off channels without being the channel (favourites and recents, hidden ideas, airing
// history, discovery feedback) lives in internal/platform/store/channeldatastore. Its types and value sets
// are re-exported here by alias, for the store's own contract.
type (
	// FavouriteChannel is one channel a person starred (channeldatastore.FavouriteChannel).
	FavouriteChannel = channeldatastore.FavouriteChannel
	// RecentChannel is one channel a person tuned (channeldatastore.RecentChannel).
	RecentChannel = channeldatastore.RecentChannel
	// UserChannelLists is one person's guide filters (channeldatastore.UserChannelLists).
	UserChannelLists = channeldatastore.UserChannelLists
	// ScheduledAiring is one programme a channel's schedule put on air (channeldatastore.ScheduledAiring).
	ScheduledAiring = channeldatastore.ScheduledAiring
	// FeedbackScope is the scope of a discovery-feedback choice (channeldatastore.FeedbackScope).
	FeedbackScope = channeldatastore.FeedbackScope
	// FeedbackAction is a discovery-feedback choice (channeldatastore.FeedbackAction).
	FeedbackAction = channeldatastore.FeedbackAction
	// DiscoveryFeedback is one recorded choice (channeldatastore.DiscoveryFeedback).
	DiscoveryFeedback = channeldatastore.DiscoveryFeedback
	// FeedbackFilter selects feedback by scope (channeldatastore.FeedbackFilter).
	FeedbackFilter = channeldatastore.FeedbackFilter
)

// RecentChannelsKept bounds each person's Recent list (channeldatastore.RecentChannelsKept).
const RecentChannelsKept = channeldatastore.RecentChannelsKept

// Discovery-feedback scopes and actions (channeldatastore).
const (
	FeedbackHousehold = channeldatastore.FeedbackHousehold
	FeedbackChannel   = channeldatastore.FeedbackChannel

	FeedbackKeep     = channeldatastore.FeedbackKeep
	FeedbackLess     = channeldatastore.FeedbackLess
	FeedbackNever    = channeldatastore.FeedbackNever
	FeedbackSurprise = channeldatastore.FeedbackSurprise
	FeedbackClear    = channeldatastore.FeedbackClear
)

// ChannelPreferenceStore keeps per-person channel lists (#1666). Every method is keyed by user id;
// a paired device resolves to its user before it gets here, so the lists follow the person.
type ChannelPreferenceStore interface {
	// AddFavouriteChannel stars a channel. Idempotent: a repeat keeps the first added time.
	// ErrNotFound when the channel doesn't exist.
	AddFavouriteChannel(ctx context.Context, userID, channelID string, at time.Time) error
	// RemoveFavouriteChannel un-stars a channel; removing one that isn't starred is not an error.
	RemoveFavouriteChannel(ctx context.Context, userID, channelID string) error
	// RecordChannelTune moves a channel to the front of the person's recents (never backwards, so
	// a late report from a slow device can't reorder them) and drops all but the newest
	// RecentChannelsKept. ErrNotFound when the channel doesn't exist.
	RecordChannelTune(ctx context.Context, userID, channelID string, at time.Time) error
	// UserChannelLists reads both lists in one call, for the guide's first paint.
	UserChannelLists(ctx context.Context, userID string) (UserChannelLists, error)
}

// HiddenIdeaStore keeps the channel ideas each person hid on Home (#1665). Idea ids are the
// ideas package's stable ids ("genre:comedy"), so a hide outlives the library changing under it.
type HiddenIdeaStore interface {
	// HideIdea hides an idea for a person. Idempotent: a repeat keeps the first time.
	HideIdea(ctx context.Context, userID, ideaID string, at time.Time) error
	// UnhideIdea is the undo; unhiding an idea that isn't hidden is not an error.
	UnhideIdea(ctx context.Context, userID, ideaID string) error
	// HiddenIdeas is the set of a person's hidden idea ids.
	HiddenIdeas(ctx context.Context, userID string) (map[string]bool, error)
}

// DiscoveryFeedbackStore keeps the household's and channels' discovery choices.
type DiscoveryFeedbackStore interface {
	// AppendDiscoveryFeedback atomically requires a persisted Channel for channel scope; detached
	// Channels remain persisted, while a purged or unknown identity returns ErrNotFound.
	AppendDiscoveryFeedback(context.Context, DiscoveryFeedback) error
	ListDiscoveryFeedback(context.Context, FeedbackFilter) ([]DiscoveryFeedback, error)
	PurgeSupersededDiscoveryFeedback(context.Context) (int, error)
}

// channelDataCluster names the embedded channel-data store; see identityCluster for why an alias.
type channelDataCluster = channeldatastore.Store

// bindChannelData installs the channel-data cluster on s; bindClusters calls it on every
// construction.
func bindChannelData(s *sqlStore) *sqlStore {
	s.channelDataCluster = channeldatastore.New(s.kernel())
	return s
}
