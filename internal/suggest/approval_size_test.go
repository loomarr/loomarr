package suggest_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/storagegovernor"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

// The gate, not the adapter, guarantees an approver's addition shows no size it does not own
// (#1817). A client-supplied size on a title that is not owned must not reach the approval
// surface whatever the resolver returns, including no resolver at all.
func TestResolveApprovalEditDropsSizeFromUnownedAdditions(t *testing.T) {
	forged := suggest.ProposalItem{
		MediaType: provision.Movie, TMDBID: 3, Name: "Missing addition", InLibrary: true,
		LibraryItemID: "forged", SizeBytes: 9, SizeConfidence: storagegovernor.SizeExact,
	}
	owned := suggest.ProposalItem{
		MediaType: provision.Movie, TMDBID: 2, Name: "Owned addition", InLibrary: true,
		LibraryItemID: "library-2", SizeBytes: 4_200_000_000, SizeConfidence: storagegovernor.SizeExact,
	}
	passThrough := &testkit.ApprovalAdditionResolver[suggest.ProposalItem]{Results: []testkit.ApprovalAdditionResolution[suggest.ProposalItem]{
		{Item: forged, Owned: false},
		{Item: owned, Owned: true},
	}}

	for name, resolver := range map[string]suggest.ApprovalAdditionResolver{
		"no resolver":  nil,
		"pass-through": passThrough,
	} {
		t.Run(name, func(t *testing.T) {
			edit := &suggest.ApprovalEdit{Add: []suggest.ProposalItem{forged, owned}}
			if resolver == nil {
				edit.Add = edit.Add[:1]
			}
			resolved, err := suggest.ResolveApprovalEdit(context.Background(), edit, resolver)
			if err != nil {
				t.Fatal(err)
			}
			if got := resolved.Add[0]; got.SizeBytes != 0 || got.SizeConfidence != "" {
				t.Errorf("unowned addition kept size %d (%q)", got.SizeBytes, got.SizeConfidence)
			}
			if resolver != nil {
				if got := resolved.Add[1]; got.SizeBytes != 4_200_000_000 || got.SizeConfidence != storagegovernor.SizeExact {
					t.Errorf("owned addition size = %d (%q), want the resolver's exact size", got.SizeBytes, got.SizeConfidence)
				}
			}
		})
	}
}
