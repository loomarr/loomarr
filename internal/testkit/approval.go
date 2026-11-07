package testkit

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
)

// ApprovalStore is the shared scripted persistence double for approval-coordinator tests.
// It records every commit attempt and can fail attempts in order without reproducing SQL
// behavior. Like the store, it refuses a commit that does not validate and a second approval
// of one proposal.
type ApprovalStore struct {
	Commits      []store.ProposalApproval
	CommitErrors []error
	CommitError  error
	Enqueued     int
	Latest       store.Proposal
	LatestError  error
	LatestFunc   func(call int) (store.Proposal, error)
	LatestCalls  int

	approved map[string]bool
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
	if err := validateApproval(commit); err != nil {
		return 0, fmt.Errorf("approve proposal %s: %w", commit.Proposal.ID, err)
	}
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
	if s.approved[commit.Proposal.ID] {
		return 0, store.ErrProposalNotSubmitted
	}
	if s.approved == nil {
		s.approved = map[string]bool{}
	}
	s.approved[commit.Proposal.ID] = true
	return s.Enqueued, nil
}

// validateApproval is the store's pre-transaction check of an approval commit.
func validateApproval(commit store.ProposalApproval) error {
	if commit.Proposal.Status != "approved" {
		return fmt.Errorf("terminal status is %q", commit.Proposal.Status)
	}
	ch := commit.Channel
	if err := ch.Validate(); err != nil {
		return fmt.Errorf("invalid channel: %w", err)
	}
	if err := ch.Policy.Validate(); err != nil {
		return fmt.Errorf("invalid channel policy: %w", err)
	}
	if ch.IntentRef == "" {
		return fmt.Errorf("channel %s has no intent ref", ch.ID)
	}
	for _, rec := range commit.Titles {
		want, err := rec.Title.Key()
		if err != nil {
			return fmt.Errorf("invalid title %s: %w", rec.Key, err)
		}
		if rec.Key != want {
			return fmt.Errorf("title key %q does not match %q", rec.Key, want)
		}
		switch {
		case rec.State == provision.Available && rec.LibraryID == "":
			return fmt.Errorf("available title %s has no library id", rec.Key)
		case rec.State == provision.Wanted && rec.Deadline.IsZero():
			return fmt.Errorf("wanted title %s has no deadline", rec.Key)
		case rec.State != provision.Available && rec.State != provision.Wanted:
			return fmt.Errorf("title %s has invalid approval state %q", rec.Key, rec.State)
		}
	}
	return nil
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
	return ApprovedChannel(p, len(c.Plans)), nil
}

// ApprovedChannel is a channel plan the store accepts for p, numbered number. A PlanFunc starts
// from it and changes only what its test is about.
func ApprovedChannel(p store.Proposal, number int) store.Channel {
	ch := store.Channel{}
	ch.ID = "ch_" + p.ID
	ch.IntentRef = p.JobID
	ch.Name = "Approved channel"
	ch.Number = number
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusBuilding
	ch.ReconcileDeadline = time.Now().UTC()
	return ch
}

func (c *ApprovalChannels) AfterApprovalCommitted(_ context.Context, channelID string) {
	c.Committed = append(c.Committed, channelID)
}
