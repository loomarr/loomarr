// Package proposals serves the channel-request routes under /v1/proposals (submit, list, approve,
// deny, outlook) and the First-channel Journey reads under /v1/proposal-jobs.
package proposals

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/outlook"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
)

// Store is the slice of the aggregate store these routes read and write.
type Store interface {
	httpcore.UserLister
	GetProposal(ctx context.Context, id string) (proposalstore.Proposal, error)
	ListProposalsByStatus(ctx context.Context, status string) ([]proposalstore.Proposal, error)
	ListProposalsByCreator(ctx context.Context, userID string) ([]proposalstore.Proposal, error)
	CommitProposalDenial(ctx context.Context, p proposalstore.Proposal) error
}

// Submitter queues a suggestion job for an intent.
type Submitter interface {
	// Submit enqueues a suggestion job for an intent and returns the job id.
	Submit(ctx context.Context, intent proposals.Intent, createdBy string) (jobID string, err error)
}

// Approver is the complete approval gate. Its implementation plans the channel and commits
// proposal, titles, and channel atomically before post-commit runtime work; a handler cannot
// reproduce or reorder that choreography.
type Approver interface {
	Approve(ctx context.Context, p proposalstore.Proposal, edit *accept.ApprovalEdit, approvedBy string) (accept.ApprovalResult, error)
}

// ProposalOutlook observes pending content without approval or persistence.
type ProposalOutlook interface {
	Assess(context.Context, proposalstore.Proposal, *accept.ApprovalEdit) (outlook.Assessment, error)
}

// ProposalDecisionQuality receives a denial only after its submitted-to-denied
// compare-and-swap commits. It cannot affect the HTTP decision outcome.
type ProposalDecisionQuality interface {
	ProposalDenied(context.Context, string, time.Time)
}

// Deps is what the proposal routes need from the rest of the server. Every field is optional: a
// nil service leaves its routes answering 501, as the API's nil-semantics describe.
type Deps struct {
	Store    Store
	Suggest  Submitter
	Approver Approver
	// The Proposal* names match the api.Options fields they are copied from.
	ProposalOutlook  ProposalOutlook
	ProposalWorkflow ProposalWorkflow
	ProposalProgress ProposalProgress
	// Settings gates submit and revise on the suggestions feature; nil reads as ready.
	Settings httpcore.FeatureGate
	// Activity records the Dashboard feed line for an approval; nil-safe on the Recorder.
	Activity *activity.Recorder
	// DecisionQuality and Notifications hear a denial after it commits.
	DecisionQuality       ProposalDecisionQuality
	ProposalNotifications suggest.ProposalNotifier
	// Holidays is the household's holiday vocabulary, so a proposal's episode preview agrees with
	// the engine's calendar.
	Holidays func() holidayvocab.Vocabulary
}

// Handlers serves the proposal routes.
type Handlers struct {
	store                 Store
	suggest               Submitter
	approver              Approver
	proposalOutlook       ProposalOutlook
	proposalWorkflow      ProposalWorkflow
	proposalProgress      ProposalProgress
	settings              httpcore.FeatureGate
	activity              *activity.Recorder
	decisionQuality       ProposalDecisionQuality
	proposalNotifications suggest.ProposalNotifier
	holidayVocab          func() holidayvocab.Vocabulary
}

// New returns the proposal handlers over d.
func New(d Deps) *Handlers {
	return &Handlers{
		store: d.Store, suggest: d.Suggest, approver: d.Approver, proposalOutlook: d.ProposalOutlook,
		proposalWorkflow: d.ProposalWorkflow, proposalProgress: d.ProposalProgress, settings: d.Settings,
		activity: d.Activity, decisionQuality: d.DecisionQuality, proposalNotifications: d.ProposalNotifications,
		holidayVocab: d.Holidays,
	}
}

// Register mounts every proposal and proposal-job route on api.
func (s *Handlers) Register(api huma.API) {
	s.registerProposals(api)
	s.registerProposalJourneys(api)
}

// holidays is the engine's holiday vocabulary, or the default one without a calendar wired.
func (s *Handlers) holidays() holidayvocab.Vocabulary {
	if s.holidayVocab == nil {
		return schedule.Calendar{}.Vocab()
	}
	return s.holidayVocab()
}
