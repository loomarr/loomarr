package main

import (
	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/proposals/accept"
)

// proposalStore is what seeding an approved channel reaches: the proposal and its approval, the
// channel, its titles and the library items it plays. Each smaller step takes its one role.
type proposalStore interface {
	approval.Store
	accept.ApproveStore
	store.ProposalRepository
	store.ChannelStore
	store.TitleStore
	librarystore.LibraryItemStore
	store.UserStore
}
