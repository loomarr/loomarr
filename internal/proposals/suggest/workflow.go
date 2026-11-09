package suggest

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/workflow"
)

// DurableWorkflow is the worker-facing deep-module seam: what the suggest worker needs of the
// proposal workflow (internal/proposals/workflow), which owns the Job lifecycle, its lease token
// (workflow.Work) and every terminal transition. The suggest package owns the model and catalog
// activities that run between Claim and Complete or Fail; it does not own lifecycle transitions.
type DurableWorkflow interface {
	Submit(context.Context, proposals.Intent, string, time.Time) (workflow.Submission, error)
	// SubmitForChannel creates a new job (channel id, intent, kind, requester) that re-curates an
	// existing channel and returns its id. It never reuses the intent-hash cache.
	SubmitForChannel(context.Context, string, proposals.Intent, string, string) (string, error)
	Claim(context.Context, time.Time, time.Duration, int) ([]workflow.Work, error)
	Complete(context.Context, workflow.Work, proposals.Proposal) (workflow.CommittedProposal, error)
	Fail(context.Context, workflow.Work, string, string, ...string) error
	Record(context.Context, proposals.Intent, proposals.Proposal, string) (workflow.CommittedProposal, error)
}
