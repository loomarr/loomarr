// Package proposalworkflow owns the durable Proposal Job lifecycle and the
// authoritative First-channel Journey composed from it.
package proposalworkflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

const (
	WorkflowVersionLegacy = 0
	WorkflowVersion1      = 1
)

var (
	ErrForbidden    = errors.New("proposal workflow: forbidden")
	ErrInvalidState = errors.New("proposal workflow: invalid state")
)

type JobStatus string

const (
	JobQueued  JobStatus = "queued"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

type AttemptStatus string

const (
	AttemptRunning     AttemptStatus = "running"
	AttemptSucceeded   AttemptStatus = "succeeded"
	AttemptFailed      AttemptStatus = "failed"
	AttemptInterrupted AttemptStatus = "interrupted"
)

type Milestone string

const (
	MilestoneGenerating       Milestone = "generating"
	MilestoneAwaitingApproval Milestone = "awaiting_approval"
	MilestoneDenied           Milestone = "denied"
	MilestoneBuilding         Milestone = "building"
	MilestoneLive             Milestone = "live"
	MilestoneFailed           Milestone = "failed"
)

type Action string

const (
	ActionWait        Action = "wait"
	ActionReview      Action = "review"
	ActionEdit        Action = "edit"
	ActionRetry       Action = "retry"
	ActionOpenChannel Action = "open_channel"
	ActionCheckAI     Action = "check_ai"
)

type FailureCode string

const (
	FailureNoGroundedTitles FailureCode = "no_grounded_titles"
	FailureGenerationFailed FailureCode = "generation_failed"
	FailureSelectionEmpty   FailureCode = "selection_empty"
	FailureBudgetExhausted  FailureCode = "budget_exhausted"
)

type Failure struct {
	Code    FailureCode
	Message string
	Trace   suggest.DecisionTrace
}

type ProposalStatus string

const (
	ProposalSubmitted ProposalStatus = "submitted"
	ProposalDenied    ProposalStatus = "denied"
	ProposalApproved  ProposalStatus = "approved"
)

type ProposalRef struct {
	ID         string
	Status     ProposalStatus
	ApprovedBy string
	DenyReason string
	ModSummary string
	Note       string
	Proposal   suggest.Proposal
}

type ChannelRef struct {
	ID     string
	Status schedule.ChannelStatus
}

type Attempt struct {
	Version     int
	Number      int
	Status      AttemptStatus
	StartedAt   time.Time
	CompletedAt time.Time
	Failure     *Failure
}

// Record is the repository-owned state from which Workflow derives one Journey.
// It is deliberately not an HTTP DTO.
type Record struct {
	Version  int
	JobID    string
	OwnerID  string
	Status   JobStatus
	Intent   suggest.Intent
	Attempts []Attempt
	Proposal *ProposalRef
	Channel  *ChannelRef
	// ReachedLive is durable workflow evidence, distinct from the Channel's
	// mutable current status. It keeps the first-channel milestone monotonic
	// when an operator later pauses a live Channel.
	ReachedLive bool
	FailureCode FailureCode
	// Diagnostic is retained for operator logs and deliberately never copied to
	// the caller-visible Journey.
	Diagnostic   string
	FailureTrace suggest.DecisionTrace
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Viewer carries authenticated identity resolved by the caller. Admin is an
// authorization fact; it does not imply a user id for break-glass tokens.
type Viewer struct {
	UserID string
	Admin  bool
}

// Journey is the authoritative caller-visible state of one Proposal Job.
type Journey struct {
	Version   int
	JobID     string
	Milestone Milestone
	Intent    suggest.Intent
	Attempts  []Attempt
	Proposal  *ProposalRef
	Channel   *ChannelRef
	Failure   *Failure
	Actions   []Action
	CreatedAt time.Time
	UpdatedAt time.Time
}

type repository interface {
	Load(context.Context, string) (Record, error)
}

type listRepository interface {
	ListIDs(context.Context, string, bool, int) ([]string, error)
}

type ListOptions struct {
	Mine      bool
	Milestone Milestone
}

// Workflow keeps lifecycle and projection rules behind one interface. The
// production constructor will install the Store-backed repository; tests use
// newWorkflow so they exercise this same interface without exposing that seam.
type Workflow struct {
	repository repository
	execution  executionRepository
}

func newWorkflow(repository repository) *Workflow {
	workflow := &Workflow{repository: repository}
	workflow.execution, _ = repository.(executionRepository)
	return workflow
}

// Inspect returns one owner-authorized authoritative Journey.
func (w *Workflow) Inspect(ctx context.Context, viewer Viewer, jobID string) (Journey, error) {
	record, err := w.repository.Load(ctx, jobID)
	if err != nil {
		return Journey{}, fmt.Errorf("load Proposal Job %q: %w", jobID, err)
	}
	if !viewer.Admin && (viewer.UserID == "" || viewer.UserID != record.OwnerID) {
		return Journey{}, ErrForbidden
	}
	legacy := record.Version == WorkflowVersionLegacy
	if legacy && len(record.Attempts) != 0 {
		return Journey{}, fmt.Errorf("%w: legacy Job has versioned Attempt history", ErrInvalidState)
	}
	if !legacy && record.Version != WorkflowVersion1 {
		return Journey{}, fmt.Errorf("%w: version=%d status=%q", ErrInvalidState, record.Version, record.Status)
	}
	if !legacy {
		if err := validateAttempts(record); err != nil {
			return Journey{}, err
		}
	}
	// Version 0 is the pre-history persisted form. Project it into the current
	// Journey schema without fabricating Attempts; its first new claim performs
	// the durable version transition.
	record.Version = WorkflowVersion1

	milestone := MilestoneGenerating
	actions := []Action{ActionWait}
	switch record.Status {
	case JobQueued, JobRunning:
	case JobDone:
		if record.Proposal == nil {
			return Journey{}, fmt.Errorf("%w: done job has no Proposal", ErrInvalidState)
		}
		switch record.Proposal.Status {
		case ProposalSubmitted:
			milestone = MilestoneAwaitingApproval
			if viewer.Admin {
				actions = []Action{ActionReview}
			}
		case ProposalDenied:
			milestone = MilestoneDenied
			actions = []Action{ActionEdit, ActionRetry}
		case ProposalApproved:
			if record.Channel == nil || record.Channel.ID == "" {
				return Journey{}, fmt.Errorf("%w: approved Proposal has no intent-bound Channel", ErrInvalidState)
			}
			milestone, err = approvedMilestone(record)
			if err != nil {
				return Journey{}, err
			}
			actions = []Action{ActionOpenChannel}
		default:
			return Journey{}, fmt.Errorf("%w: done job has Proposal status %q", ErrInvalidState, record.Proposal.Status)
		}
	case JobFailed:
		milestone = MilestoneFailed
		failure := safeFailure(record.FailureCode, record.FailureTrace)
		actions = []Action{ActionRetry}
		if failure.Code == FailureNoGroundedTitles {
			actions = []Action{ActionEdit, ActionRetry}
		} else if viewer.Admin {
			actions = []Action{ActionRetry, ActionCheckAI}
		}
		return journeyFrom(record, milestone, actions, &failure), nil
	default:
		return Journey{}, fmt.Errorf("%w: version=%d status=%q", ErrInvalidState, record.Version, record.Status)
	}

	return journeyFrom(record, milestone, actions, nil), nil
}

// List returns bounded authoritative Journeys newest-first. Member reads are
// always caller-scoped; Mine only changes admin behavior.
func (w *Workflow) List(ctx context.Context, viewer Viewer, options ListOptions) ([]Journey, error) {
	lister, ok := w.repository.(listRepository)
	if !ok {
		return nil, fmt.Errorf("%w: list repository unavailable", ErrInvalidState)
	}
	all := viewer.Admin && !options.Mine
	if !all && viewer.UserID == "" {
		return []Journey{}, nil
	}
	ids, err := lister.ListIDs(ctx, viewer.UserID, all, 100)
	if err != nil {
		return nil, fmt.Errorf("list Proposal Jobs: %w", err)
	}
	journeys := make([]Journey, 0, len(ids))
	for _, id := range ids {
		journey, err := w.Inspect(ctx, viewer, id)
		if err != nil {
			return nil, err
		}
		if options.Milestone == "" || journey.Milestone == options.Milestone {
			journeys = append(journeys, journey)
		}
	}
	return journeys, nil
}

func journeyFrom(record Record, milestone Milestone, actions []Action, failure *Failure) Journey {
	return Journey{
		Version: record.Version, JobID: record.JobID, Milestone: milestone,
		Intent: record.Intent, Attempts: cloneAttempts(record.Attempts),
		Proposal: cloneProposal(record.Proposal), Channel: cloneChannel(record.Channel),
		Failure: cloneFailure(failure), Actions: append([]Action(nil), actions...),
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func safeFailure(code FailureCode, traces ...suggest.DecisionTrace) Failure {
	var trace suggest.DecisionTrace
	if len(traces) > 0 {
		trace = traces[0]
	}
	if code == FailureNoGroundedTitles || code == FailureSelectionEmpty || code == FailureBudgetExhausted {
		message := "No grounded titles matched this request. Try again, or edit its description and constraints."
		if code == FailureBudgetExhausted {
			message = "This request exceeded the bounded discovery budget. Try again with narrower constraints."
		}
		return Failure{
			Code:    code,
			Message: message,
			Trace:   trace,
		}
	}
	return Failure{
		Code:    FailureGenerationFailed,
		Message: "Loomarr couldn't generate this channel. Try again later.",
		Trace:   trace,
	}
}

func validateAttempts(record Record) error {
	if len(record.Attempts) == 0 {
		if record.Status == JobQueued {
			return nil
		}
		return fmt.Errorf("%w: %s job has no Attempt", ErrInvalidState, record.Status)
	}
	for i, attempt := range record.Attempts {
		if attempt.Version != WorkflowVersion1 {
			return fmt.Errorf("%w: Attempt %d has version %d", ErrInvalidState, attempt.Number, attempt.Version)
		}
		if attempt.Number != i+1 {
			return fmt.Errorf("%w: Attempt number %d follows index %d", ErrInvalidState, attempt.Number, i)
		}
		switch attempt.Status {
		case AttemptRunning, AttemptSucceeded, AttemptFailed, AttemptInterrupted:
		default:
			return fmt.Errorf("%w: Attempt %d has status %q", ErrInvalidState, attempt.Number, attempt.Status)
		}
		if i < len(record.Attempts)-1 && attempt.Status == AttemptRunning {
			return fmt.Errorf("%w: non-current Attempt %d is running", ErrInvalidState, attempt.Number)
		}
	}

	latest := record.Attempts[len(record.Attempts)-1]
	want := AttemptStatus("")
	switch record.Status {
	case JobQueued:
		if latest.Status == AttemptRunning {
			return fmt.Errorf("%w: queued job has a running Attempt", ErrInvalidState)
		}
		return nil
	case JobRunning:
		want = AttemptRunning
	case JobDone:
		want = AttemptSucceeded
	case JobFailed:
		want = AttemptFailed
	default:
		return fmt.Errorf("%w: version=%d status=%q", ErrInvalidState, record.Version, record.Status)
	}
	if latest.Status != want {
		return fmt.Errorf("%w: %s job has current Attempt status %q", ErrInvalidState, record.Status, latest.Status)
	}
	return nil
}

func cloneAttempts(attempts []Attempt) []Attempt {
	clones := append([]Attempt(nil), attempts...)
	for i := range clones {
		clones[i].Failure = cloneFailure(clones[i].Failure)
	}
	return clones
}

func approvedMilestone(record Record) (Milestone, error) {
	if record.ReachedLive || record.Channel.Status == schedule.StatusLive {
		return MilestoneLive, nil
	}
	switch record.Channel.Status {
	case schedule.StatusBuilding, schedule.StatusDrifted, schedule.StatusEmpty, schedule.StatusPaused:
		return MilestoneBuilding, nil
	default:
		return "", fmt.Errorf("%w: approved Proposal has Channel status %q before first live", ErrInvalidState, record.Channel.Status)
	}
}

func cloneProposal(proposal *ProposalRef) *ProposalRef {
	if proposal == nil {
		return nil
	}
	clone := *proposal
	return &clone
}

func cloneChannel(channel *ChannelRef) *ChannelRef {
	if channel == nil {
		return nil
	}
	clone := *channel
	return &clone
}

func cloneFailure(failure *Failure) *Failure {
	if failure == nil {
		return nil
	}
	clone := *failure
	return &clone
}
