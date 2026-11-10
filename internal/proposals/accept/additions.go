package accept

import (
	"context"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/provision"
)

// AdditionResolver re-reads a proposed title's library presence at the approval gate, so what an
// approved addition records about ownership, rating, genres and size comes from the Library and
// never from the proposal's own text.
type AdditionResolver struct {
	presence titlesearch.LibraryPresenceSource
}

// NewAdditionResolver returns the resolver over the library presence source, which is resolved at
// call time and may return nil while the media server is not configured.
func NewAdditionResolver(presence titlesearch.LibraryPresenceSource) AdditionResolver {
	return AdditionResolver{presence: presence}
}

// ResolveApprovalAddition returns the item with its library facts reset, then filled from the
// Library when it owns the title; the bool reports whether it does.
func (a AdditionResolver) ResolveApprovalAddition(
	ctx context.Context,
	item proposals.ProposalItem,
) (proposals.ProposalItem, bool, error) {
	key, err := (provision.Title{
		MediaType: item.MediaType, TMDBID: item.TMDBID, TVDBID: item.TVDBID,
	}).Key()
	if err != nil {
		return proposals.ProposalItem{}, false, fmt.Errorf("invalid title identity: %w", err)
	}
	mediaType, _, _, ok := provision.ParseKey(key)
	if !ok {
		return proposals.ProposalItem{}, false, errors.New("invalid title key")
	}
	item.InLibrary = false
	item.LibraryItemID = ""
	item.OfficialRating = ""
	item = item.WithLibrarySize(0)
	if a.presence == nil {
		return item, false, nil
	}
	presence := a.presence()
	if presence == nil {
		return item, false, nil
	}
	owned, found, err := presence.Present(ctx, mediaType, item.TMDBID, item.TVDBID)
	if err != nil {
		return proposals.ProposalItem{}, false, err
	}
	if !found {
		return item, false, nil
	}
	item.InLibrary = true
	item.LibraryItemID = owned.LibraryItemID
	item.OfficialRating = owned.OfficialRating
	item.Genres = append([]string(nil), owned.Genres...)
	return item.WithLibrarySize(owned.SizeBytes), true, nil
}
