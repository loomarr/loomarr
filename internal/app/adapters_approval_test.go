package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/storagegovernor"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

func TestApprovalAdditionAdapterOwnsLibraryPresenceAtTheGate(t *testing.T) {
	presence := &catalogfixture.Presence{Hits: map[int]catalog.Presence{
		672: {LibraryItemID: "library-672", OfficialRating: "PG", Genres: []string{"Fantasy"}, SizeBytes: 4_200_000_000},
	}}
	adapter := approvalAdditionAdapter{presence: func() catalog.LibraryPresence { return presence }}

	owned, isOwned, err := adapter.ResolveApprovalAddition(context.Background(), suggest.ProposalItem{
		MediaType: provision.Movie, TMDBID: 672, Name: "Owned", InLibrary: false, OfficialRating: "G",
		SizeBytes: 1, SizeConfidence: storagegovernor.SizeEstimated,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !isOwned || !owned.InLibrary || owned.LibraryItemID != "library-672" || owned.OfficialRating != "PG" || len(owned.Genres) != 1 {
		t.Fatalf("owned addition = %+v, %v; want authoritative Library metadata", owned, isOwned)
	}
	if owned.SizeBytes != 4_200_000_000 || owned.SizeConfidence != storagegovernor.SizeExact {
		t.Fatalf("owned addition size = %d (%q); want the Library's exact 4200000000", owned.SizeBytes, owned.SizeConfidence)
	}

	missing, isOwned, err := adapter.ResolveApprovalAddition(context.Background(), suggest.ProposalItem{
		MediaType: provision.Movie, TMDBID: 673, Name: "Missing", InLibrary: true,
		LibraryItemID: "forged-id", OfficialRating: "G", SizeBytes: 9, SizeConfidence: storagegovernor.SizeExact,
	})
	if err != nil {
		t.Fatal(err)
	}
	if isOwned || missing.InLibrary || missing.LibraryItemID != "" || missing.OfficialRating != "" ||
		missing.SizeBytes != 0 || missing.SizeConfidence != "" {
		t.Fatalf("missing addition = %+v, %v; want forged presence and size removed", missing, isOwned)
	}
}
