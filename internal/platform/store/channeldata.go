package store

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
)

// What hangs off channels without being the channel (favourites and recents, hidden ideas, airing
// history, discovery feedback) lives in internal/platform/store/channeldatastore.

// UpcomingAiringStore keeps each internal-playout channel's programmes for the next two days
// (#4476, scheduling.md "Upcoming airings"), so a title's next airing is one indexed read.
type UpcomingAiringStore interface {
	// ReplaceUpcomingAirings replaces a channel's rows in one transaction while the channel is still
	// at revision, and reports whether it wrote; a stale revision or a gone channel changes nothing.
	ReplaceUpcomingAirings(ctx context.Context, channelID string, revision int64, airings []channeldatastore.UpcomingAiring) (bool, error)
	// UpcomingAiringsForItem returns a library item's airings not ended by now, on channels neither
	// paused nor detached, earliest first: a movie by its id, a series through its title key.
	UpcomingAiringsForItem(ctx context.Context, libraryItemID string, now time.Time) ([]channeldatastore.UpcomingAiring, error)
}

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
	// channeldatastore.RecentChannelsKept. ErrNotFound when the channel doesn't exist.
	RecordChannelTune(ctx context.Context, userID, channelID string, at time.Time) error
	// UserChannelLists reads both lists in one call, for the guide's first paint.
	UserChannelLists(ctx context.Context, userID string) (channeldatastore.UserChannelLists, error)
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
	AppendDiscoveryFeedback(context.Context, channeldatastore.DiscoveryFeedback) error
	ListDiscoveryFeedback(context.Context, channeldatastore.FeedbackFilter) ([]channeldatastore.DiscoveryFeedback, error)
	PurgeSupersededDiscoveryFeedback(context.Context) (int, error)
}
