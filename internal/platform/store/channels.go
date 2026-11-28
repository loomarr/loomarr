package store

import (
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// Channels live in internal/platform/store/channelstore. Their row type and conflict error are
// re-exported here by alias, for the store's own contract.
type (
	// Channel is the persisted form of a scheduler channel (channelstore.Channel).
	Channel = channelstore.Channel
)

// ErrChannelConflict reports that a channel write lost a uniqueness race. It is
// channelstore.ErrChannelConflict, the same value under a second name.
var ErrChannelConflict = channelstore.ErrChannelConflict

// channelInvalidations gives the channel store the durable Postgres wake-ups it publishes on
// writes and deletes.
type channelInvalidations struct{}

func (channelInvalidations) Saved(ch Channel) (string, error) { return channelInvalidation(ch) }

func (channelInvalidations) Deleted(id string) (string, error) {
	return marshalInvalidation(Invalidation{Kind: InvalidationChannel, ChannelID: id,
		Status: schedule.StatusDetached})
}

func (channelInvalidations) NotifyChannel() string { return postgresInvalidationChannel }
