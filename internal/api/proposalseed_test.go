package api_test

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// proposalSeeder is the slice of a store a seeded proposal needs.
type proposalSeeder interface {
	CreateProposal(context.Context, store.Proposal) error
	CreateJob(context.Context, store.Job) error
	GetJob(context.Context, string) (store.Job, error)
}

// createProposalWithJob stores a proposal together with the job that produced it. A proposal
// always has its job: approval refuses to create a channel from one whose job is gone, so a
// fixture without the job would not be a state the product can reach.
func createProposalWithJob(st proposalSeeder, ctx context.Context, p store.Proposal) error {
	if p.JobID != "" {
		if _, err := st.GetJob(ctx, p.JobID); errors.Is(err, store.ErrNotFound) {
			if err := st.CreateJob(ctx, store.Job{
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
