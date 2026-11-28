package store

import (
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// Channels live in internal/platform/store/channelstore.

// channelInvalidations gives the channel store the durable Postgres wake-ups it publishes on
// writes and deletes.
type channelInvalidations struct{}

func (channelInvalidations) Saved(ch channelstore.Channel) (string, error) {
	return channelInvalidation(ch)
}

func (channelInvalidations) Deleted(id string) (string, error) {
	return marshalInvalidation(Invalidation{Kind: InvalidationChannel, ChannelID: id,
		Status: schedule.StatusDetached})
}

func (channelInvalidations) NotifyChannel() string { return postgresInvalidationChannel }
