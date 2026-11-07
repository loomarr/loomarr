package suggest

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/proposals"
)

// ErrStaleAttempt is what the workflow returns when an Attempt's terminal write lost its
// compare-and-swap because a newer Attempt owns the Job. The worker drops that result: the
// replacement execution, not this one, decides the Job's outcome.
var ErrStaleAttempt = errors.New("proposal workflow: stale attempt")

// WorkflowWork is the activity input issued by the durable proposal workflow.
// Version plus Attempt form the compare-and-swap token for terminal writes.
type WorkflowWork struct {
	Version   int
	JobID     string
	Attempt   int
	Kind      string
	CreatedBy string
	// ChannelID names the channel a refine or re-curation job re-curates; empty for a request.
	ChannelID string
	Intent    proposals.Intent
}

// WorkflowProposal is the durably committed Proposal returned to the optional
// approval grants after generation succeeds.
type WorkflowProposal struct {
	ID        string
	JobID     string
	CreatedBy string
	Proposal  proposals.Proposal
	CreatedAt time.Time
}

type WorkflowSubmission struct {
	JobID          string
	CachedProposal *WorkflowProposal
}

// DurableWorkflow is the worker-facing deep-module seam. The suggest package
// owns model/catalog activities; it does not own lifecycle transitions.
type DurableWorkflow interface {
	Submit(context.Context, proposals.Intent, string, time.Time) (WorkflowSubmission, error)
	// SubmitForChannel creates a new job (channel id, intent, kind, requester) that re-curates an
	// existing channel and returns its id. It never reuses the intent-hash cache.
	SubmitForChannel(context.Context, string, proposals.Intent, string, string) (string, error)
	Claim(context.Context, time.Time, time.Duration, int) ([]WorkflowWork, error)
	Complete(context.Context, WorkflowWork, proposals.Proposal) (WorkflowProposal, error)
	Fail(context.Context, WorkflowWork, string, string, ...string) error
	Record(context.Context, proposals.Intent, proposals.Proposal, string) (WorkflowProposal, error)
}
