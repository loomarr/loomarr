package main

import "github.com/loomarr/loomarr/internal/platform/store"

// channelProposalStore is what ensuring a demo channel reads and writes: the channel it may already
// have, and the proposal that creates it.
type channelProposalStore interface {
	store.ChannelStore
	store.ProposalRepository
}

// libraryReader looks the demo titles and their episodes up in the scanned Library.
type libraryReader interface {
	store.LibraryTitleReader
	store.LibraryEpisodeReader
}
