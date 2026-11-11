package store

import (
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// Channels live in internal/platform/store/channelstore. Their row type, codecs and sentinel errors are
// re-exported here by alias, for the store's own contract.
type (
	// Channel is the persisted form of a scheduler channel (channelstore.Channel).
	Channel = channelstore.Channel
)

// The two legal channel broadcast codecs (channelstore).
const (
	BroadcastCodecH264 = channelstore.BroadcastCodecH264
	BroadcastCodecHEVC = channelstore.BroadcastCodecHEVC
)

// ErrChannelConflict reports that a channel write lost a uniqueness race. It is
// channelstore.ErrChannelConflict, the same value under a second name.
var ErrChannelConflict = channelstore.ErrChannelConflict

// ErrChannelStale reports that a channel mutation was planned from an older revision. It is
// channelstore.ErrChannelStale, the same value under a second name.
var ErrChannelStale = channelstore.ErrChannelStale

// ErrUnknownLibraryReference reports that a channel names a library item or collection the library
// has no row for. It is channelstore.ErrUnknownLibraryReference under a second name.
var ErrUnknownLibraryReference = channelstore.ErrUnknownLibraryReference

// channelCluster names the embedded channel store; see identityCluster for why an alias.
type channelCluster = channelstore.Store

// channelInvalidations gives the channel store the durable Postgres wake-ups it publishes on
// writes and deletes.
type channelInvalidations struct{}

func (channelInvalidations) Saved(ch Channel) (string, error) { return channelInvalidation(ch) }

func (channelInvalidations) Deleted(id string) (string, error) {
	return marshalInvalidation(Invalidation{Kind: InvalidationChannel, ChannelID: id,
		Status: schedule.StatusDetached})
}

func (channelInvalidations) NotifyChannel() string { return postgresInvalidationChannel }

// bindChannels installs the channel cluster on s; bindClusters calls it on every construction, after
// the dialect set s.channelClaimSQL.
func bindChannels(s *sqlStore) *sqlStore {
	var inv channelstore.Invalidations // nil on SQLite: no wake-ups are published
	if s.dialect == DialectPostgres {
		inv = channelInvalidations{}
	}
	s.channelCluster = channelstore.New(s.kernel(), s.channelClaimSQL, inv)
	return s
}
