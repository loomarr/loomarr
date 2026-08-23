package testkit

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// ApprovalStore is the shared scripted persistence double for approval-coordinator tests.
// It records complete commits and can fail attempts in order without reproducing SQL behavior.
type ApprovalStore struct {
	Commits      []store.ProposalApproval
	CommitErrors []error
	CommitError  error
	Enqueued     int
	Latest       store.Proposal
	LatestError  error
	LatestFunc   func(call int) (store.Proposal, error)
	LatestCalls  int
}

func (*ApprovalStore) GetTitle(context.Context, provision.Key) (provision.Record, error) {
	return provision.Record{}, store.ErrNotFound
}

func (s *ApprovalStore) NewestProposalByStatusForJob(context.Context, string, string) (store.Proposal, error) {
	s.LatestCalls++
	if s.LatestFunc != nil {
		return s.LatestFunc(s.LatestCalls)
	}
	if s.LatestError != nil {
		return store.Proposal{}, s.LatestError
	}
	if s.Latest.ID == "" {
		return store.Proposal{}, store.ErrNotFound
	}
	return s.Latest, nil
}

func (s *ApprovalStore) CommitProposalApproval(_ context.Context, commit store.ProposalApproval) (int, error) {
	s.Commits = append(s.Commits, commit)
	if len(s.CommitErrors) > 0 {
		err := s.CommitErrors[0]
		s.CommitErrors = s.CommitErrors[1:]
		if err != nil {
			return 0, err
		}
	}
	if s.CommitError != nil {
		return 0, s.CommitError
	}
	return s.Enqueued, nil
}

// ApprovalChannels is the shared channel-planning/post-commit double. PlanFunc may override
// the default valid channel when a test needs a particular snapshot or mutation.
type ApprovalChannels struct {
	Plans     []store.Proposal
	Committed []string
	PlanError error
	PlanFunc  func(store.Proposal, int) store.Channel
}

func (c *ApprovalChannels) PlanApprovedChannel(_ context.Context, p store.Proposal) (store.Channel, error) {
	c.Plans = append(c.Plans, p)
	if c.PlanError != nil {
		return store.Channel{}, c.PlanError
	}
	if c.PlanFunc != nil {
		return c.PlanFunc(p, len(c.Plans)), nil
	}
	ch := store.Channel{}
	ch.ID = "ch_" + p.ID
	ch.IntentRef = p.JobID
	ch.Name = "Approved channel"
	ch.Number = len(c.Plans)
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusBuilding
	ch.ReconcileDeadline = time.Now().UTC()
	return ch, nil
}

func (c *ApprovalChannels) AfterApprovalCommitted(_ context.Context, channelID string) {
	c.Committed = append(c.Committed, channelID)
}
