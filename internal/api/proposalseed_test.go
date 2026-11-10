package api_test

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// proposalSeeder is the slice of a store a seeded proposal needs.
type proposalSeeder interface {
	CreateProposal(context.Context, proposalstore.Proposal) error
	CreateJob(context.Context, proposalstore.Job) error
	GetJob(context.Context, string) (proposalstore.Job, error)
	testkit.LibraryItemSaver
}

// createProposalWithJob stores a proposal together with the job that produced it, and the library
// items its in-library picks name. A proposal always has its job: approval refuses to create a
// channel from one whose job is gone, so a fixture without the job would not be a state the
// product can reach. Likewise a pick names a library item the library has, which the title
// approval writes points at (#2793).
func createProposalWithJob(st proposalSeeder, ctx context.Context, p proposalstore.Proposal) error {
	var items []librarystore.LibraryItem
	picks := append(slices.Clone(p.Body.Lineup), p.Body.Acquisitions...)
	for _, refused := range p.Body.Refused {
		picks = append(picks, refused.Item)
	}
	for _, pick := range picks {
		if pick.LibraryItemID != "" {
			items = append(items, librarystore.LibraryItem{
				ID: pick.LibraryItemID, Kind: "movie", Name: pick.Name, SortName: pick.Name,
			})
		}
	}
	if err := st.SaveLibraryItems(ctx, items, time.Unix(1, 0)); err != nil {
		return err
	}
	if p.JobID != "" {
		if _, err := st.GetJob(ctx, p.JobID); errors.Is(err, store.ErrNotFound) {
			if err := st.CreateJob(ctx, proposalstore.Job{
				ID: p.JobID, Kind: "suggest", Status: "done", CreatedBy: p.CreatedBy,
				CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
			}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return st.CreateProposal(ctx, p)
}
