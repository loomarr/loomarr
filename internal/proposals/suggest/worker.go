package suggest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/quality"
	"github.com/loomarr/loomarr/internal/proposals/webreference"
	"github.com/loomarr/loomarr/internal/store"
)

// This file is the job execution model (§8): submissions become persisted jobs;
// an in-process worker pool claims due jobs (ClaimDueJobs, SKIP-LOCKED on
// Postgres) and runs the suggester, bounded by JOB_TIMEOUT so one hung LLM call
// can never starve the queue. Proposals are persisted so the approval queue and
// My proposals survive restarts.

// ProposalStore is the slice of the store the worker needs outside the durable workflow: the
// channel a re-curation job belongs to. Every job lifecycle transition goes through the workflow.
type ProposalStore interface {
	GetChannelForJob(ctx context.Context, jobID string) (store.Channel, error)
}

// Service owns submission + the worker pool (§8). It's the seam the API depends
// on: Submit enqueues a job (respecting the cache), the pool runs it, and the
// resulting proposal lands in the approval queue.
type Service struct {
	store     ProposalStore
	suggester *Suggester
	log       *slog.Logger
	workers   int
	timeout   time.Duration // JOB_TIMEOUT: per-job wall-clock bound (§8)
	cacheTTL  time.Duration
	now       func() time.Time
	emit      ProgressEmitter // optional (§8 SSE progress); nil ⇒ no live frames
	// auto applies the §11 auto-approve grant. nil ⇒ every proposal waits for an admin,
	// which is the correct default: the gate is closed unless something opens it.
	auto *AutoApprover
	// autoCurate applies the §8.2 channel-scoped auto-curate grant to a re-curation
	// proposal. nil ⇒ re-curation proposals wait for an admin (the closed default). Distinct
	// from `auto` (per-user quota); this one is authorized by the channel's AutoCurate opt-in.
	autoCurate ChannelAutoCurator
	workflow   DurableWorkflow
	notify     ProposalNotifier
	quality    QualityRecorder
	// tracked holds the live ProgressTracker of every running job, by job id.
	tracked sync.Map
}

// QualityRecorder is the narrow, best-effort sink for authoritative Proposal
// Job stage outcomes. The worker owns classification; the sink cannot add labels.
type QualityRecorder interface {
	RecordQualityObservation(context.Context, quality.Observation) error
}

func (s *Service) WithQualityRecorder(recorder QualityRecorder) *Service {
	s.quality = recorder
	return s
}

type ProposalNotifier interface {
	ProposalSubmitted(context.Context, store.Proposal)
	ProposalApproved(context.Context, store.Proposal, string)
	ProposalDeclined(context.Context, store.Proposal)
}

func (s *Service) WithProposalNotifier(notifier ProposalNotifier) *Service {
	s.notify = notifier
	return s
}

// ChannelAutoCurator applies the §8.2 channel-scoped auto-curate grant: it decides whether a
// re-curation proposal may auto-approve because its CHANNEL opted in (AutoCurate), bounded by
// the quality bar + title cap — never a user quota. The suggest package declares the interface
// (dependency inversion); internal/channels/recurate implements it, wired at the composition root, so
// suggest never imports recurate. Returns the same Decision shape the per-user grant does.
type ChannelAutoCurator interface {
	Consider(ctx context.Context, p store.Proposal) (Decision, error)
}

const (
	jobKindSuggest  = "suggest"
	jobKindRecurate = "recurate"

	FailureCodeNoGroundedTitles = "no_grounded_titles"
	FailureCodeGenerationFailed = "generation_failed"
)

// WithAutoApprove enables the per-user auto-approve grant (§8, §11). Wired at the
// composition root, where the settings service and the full store are both available.
func (s *Service) WithAutoApprove(a *AutoApprover) *Service {
	s.auto = a
	return s
}

// WithAutoCurate wires the §8.2 channel-scoped auto-curate grant, so a re-curation proposal
// for an auto-curate-opted-in channel auto-approves within its quality bar + title cap.
// Mirrors WithAutoApprove; nil (unset) keeps re-curation proposals in the admin queue.
func (s *Service) WithAutoCurate(c ChannelAutoCurator) *Service {
	s.autoCurate = c
	return s
}

// ProgressEmitter publishes a job's generation-progress frames to the SSE bus
// (§7, type=suggestion). Optional and best-effort: a dropped frame is a latency
// bug, never a correctness bug — GET /v1/proposals/{id} stays the source of
// truth. The composition root implements it over events.Bus; unit tests leave it
// nil. Kept as a narrow interface so the suggest package doesn't import events.
// round is the 1-based tool-loop iteration (0 outside the loop) — it lets the UI
// show a long run PROGRESSING rather than looking hung, since the same phase
// legitimately repeats as the model alternates thinking and searching.
type ProgressEmitter interface {
	SuggestionPhase(jobID, phase string, round int)
}

// WithProgressEmitter wires the SSE progress emitter and returns the service for
// chaining (matches the runners' WithInterval style). Nil is fine — no frames.
func (s *Service) WithProgressEmitter(e ProgressEmitter) *Service {
	s.emit = e
	return s
}

// emitPhase publishes one phase frame if an emitter is wired. round is 0 for the
// phases emitted around the pipeline (done/failed) rather than inside its tool loop.
func (s *Service) emitPhase(jobID string, p Phase, round int) {
	if s.emit != nil {
		s.emit.SuggestionPhase(jobID, string(p), round)
	}
}

// Config parameterizes the service (from §15).
type Config struct {
	Workers  int
	Timeout  time.Duration
	CacheTTL time.Duration
}

// NewService builds the suggestion service. The durable workflow owns every job and proposal
// lifecycle transition (submit, claim, complete, fail), so it is required; now defaults to
// time.Now.
func NewService(st ProposalStore, workflow DurableWorkflow, sug *Suggester, cfg Config, now func() time.Time, log *slog.Logger) *Service {
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Minute
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 24 * time.Hour
	}
	if now == nil {
		now = time.Now
	}
	return &Service{
		store: st, workflow: workflow, suggester: sug, log: log,
		workers: cfg.Workers, timeout: cfg.Timeout, cacheTTL: cfg.CacheTTL,
		now: now,
	}
}

// Submit creates a caller-owned suggestion job for an intent (§8). A successful
// cache hit copies only proposal content into a fresh job/proposal lifecycle; it
// never reuses another request's id, owner, or approval state. Returns the job id.
func (s *Service) Submit(ctx context.Context, intent proposals.Intent, createdBy string) (string, error) {
	// The workflow serves the cache only for a recent SUCCESSFUL job, so a failed job (no
	// grounded titles, a timeout) never wedges a re-submit.
	submission, err := s.workflow.Submit(ctx, intent, createdBy, s.now().Add(-s.cacheTTL))
	if err != nil {
		return "", err
	}
	if submission.CachedProposal != nil {
		s.settleSubmitted(ctx, store.Job{
			ID: submission.JobID, Kind: jobKindSuggest, CreatedBy: createdBy,
		}, store.Proposal{
			ID: submission.CachedProposal.ID, JobID: submission.JobID, Status: "submitted",
			CreatedBy: createdBy, Body: submission.CachedProposal.Proposal,
			CreatedAt: submission.CachedProposal.CreatedAt, UpdatedAt: submission.CachedProposal.CreatedAt,
		})
	}
	return submission.JobID, nil
}

// Refine submits a NEW job that re-curates an existing channel with a refine-flavored intent
// (§7 refine). The job names the channel, so approving its proposal patches that channel in
// place (no duplicate channel). It is the admin's own request, listed under them, and it never
// touches the request the channel was made from. Bypasses the intent-hash cache deliberately:
// a refine always re-generates (the operator asked for a change; a cached identical run would be
// surprising). Returns the new job id for the caller to poll for the proposal.
func (s *Service) Refine(ctx context.Context, channelID string, intent proposals.Intent, requestedBy string) (string, error) {
	return s.submitForChannel(ctx, channelID, intent, jobKindSuggest, requestedBy, "refine")
}

// SubmitBuilt puts a proposal that needed no generation (a library channel idea, #1720) in the
// approval queue as the requester's own request, and returns its job id. The model never runs, so
// it works with the LLM off. Afterwards it does what a generated proposal gets: admins are told a
// request is waiting, and the requester's auto-approve grant, if any, applies within its cap.
func (s *Service) SubmitBuilt(ctx context.Context, intent proposals.Intent, built proposals.Proposal, createdBy string) (string, error) {
	recorded, err := s.workflow.Record(ctx, intent, built, createdBy)
	if err != nil {
		return "", err
	}
	row := store.Proposal{
		ID: recorded.ID, JobID: recorded.JobID, Status: "submitted", CreatedBy: createdBy,
		Body: recorded.Proposal, CreatedAt: recorded.CreatedAt, UpdatedAt: recorded.CreatedAt,
	}
	s.settleSubmitted(ctx, store.Job{ID: recorded.JobID, Kind: jobKindSuggest, CreatedBy: createdBy, Attempts: 1}, row)
	return recorded.JobID, nil
}

// Recurate submits a NEW job that re-curates a channel under the channel-scoped unattended
// grant. The distinct durable kind is the authorization discriminator the worker reads: a
// scheduled refresh must never fall through a requester's auto-approve grant, and a human refine
// must never be mistaken for unattended curation merely because its channel is opted in. The job
// has no requester: it is system work, so it never appears in anyone's request list.
func (s *Service) Recurate(ctx context.Context, channelID string, intent proposals.Intent) (string, error) {
	return s.submitForChannel(ctx, channelID, intent, jobKindRecurate, "", "recurate")
}

func (s *Service) submitForChannel(ctx context.Context, channelID string, intent proposals.Intent, kind, createdBy, operation string) (string, error) {
	jobID, err := s.workflow.SubmitForChannel(ctx, channelID, intent, kind, createdBy)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	return jobID, nil
}

// Run starts the worker pool and blocks until ctx is cancelled (§8). Each worker
// polls for due jobs and runs them; the pool shares the ClaimDueJobs lease so no
// job runs twice.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	s.log.Info("suggestion worker pool started", "workers", s.workers, "timeout", s.timeout)
	sem := make(chan struct{}, s.workers)
	// A service start or restart should recover already-due work immediately. Waiting
	// for the first polling tick adds two seconds to every queued Proposal Job and to
	// every independent worker lifecycle exercised by the test suite.
	s.drainOnce(ctx, sem)
	for {
		select {
		case <-ctx.Done():
			s.log.Info("suggestion worker pool stopped")
			return
		case <-ticker.C:
			s.drainOnce(ctx, sem)
		}
	}
}

// drainOnce claims up to `workers` due jobs and runs each in a bounded goroutine.
func (s *Service) drainOnce(ctx context.Context, sem chan struct{}) {
	available := cap(sem) - len(sem)
	if available <= 0 {
		return
	}
	// Lease horizon must exceed JOB_TIMEOUT so a running job isn't re-claimed.
	works, err := s.workflow.Claim(ctx, s.now(), s.timeout+time.Minute, available)
	if err != nil {
		s.log.Error("claim Proposal Jobs", "err", err)
		return
	}
	for _, work := range works {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		go func() {
			defer func() { <-sem }()
			s.runWorkflow(ctx, work)
		}()
	}
}

func (s *Service) runWorkflow(ctx context.Context, work WorkflowWork) {
	jobCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	jobCtx = WithProgress(jobCtx, func(p Phase, round int) { s.emitPhase(work.JobID, p, round) })
	jobCtx, untrack := s.trackJob(jobCtx, work.JobID)
	defer untrack()

	intent, err := s.executionIntent(jobCtx, work.Kind, work.JobID, work.Intent)
	if err != nil {
		s.failWorkflow(ctx, work, err, 0, false)
		return
	}
	started := s.now()
	suggested, err := s.suggester.Suggest(jobCtx, intent)
	duration := s.now().Sub(started)
	if err != nil {
		s.failWorkflow(ctx, work, err, duration, true)
		return
	}
	completed, err := s.workflow.Complete(ctx, work, suggested)
	if errors.Is(err, ErrStaleAttempt) {
		s.log.Info("discarding stale suggestion result", "job", work.JobID, "attempt", work.Attempt, "err", err)
		return
	}
	if err != nil {
		s.failWorkflow(ctx, work, fmt.Errorf("persist proposal: %w", err), 0, false)
		return
	}
	s.recordSuggestionQuality(ctx, work.JobID, work.Attempt, duration, completed.Proposal, nil)
	row := store.Proposal{
		ID: completed.ID, JobID: completed.JobID, Status: "submitted", CreatedBy: completed.CreatedBy,
		Body: completed.Proposal, CreatedAt: completed.CreatedAt, UpdatedAt: completed.CreatedAt,
	}
	s.settleSubmitted(ctx, store.Job{
		ID: work.JobID, Kind: work.Kind, CreatedBy: work.CreatedBy, ChannelID: work.ChannelID, Attempts: work.Attempt,
	}, row)
	s.emitPhase(work.JobID, PhaseDone, 0)
}

func (s *Service) failWorkflow(ctx context.Context, work WorkflowWork, cause error, measuredDuration time.Duration, measured bool) {
	traceJSON, cause := persistedFailure(cause)
	if err := s.workflow.Fail(ctx, work, classifyFailure(cause), persistedMessage(cause), traceJSON); err != nil {
		s.log.Info("discarding stale or undurable Proposal Job failure",
			"job", work.JobID, "attempt", work.Attempt, "cause", cause, "err", err)
		return
	}
	s.log.Error("suggestion job failed", "job", work.JobID, "attempt", work.Attempt, "err", cause, "cause", unwrapFailure(cause))
	if measured {
		s.recordSuggestionQuality(ctx, work.JobID, work.Attempt, measuredDuration, proposals.Proposal{}, cause)
	}
	s.emitPhase(work.JobID, PhaseFailed, 0)
}

// executionIntent derives channel feedback scope from durable server-owned work.
// DiscoveryScopeID is intentionally not serialized, so no caller-controlled Intent
// can select another channel's feedback at execution time.
func (s *Service) executionIntent(ctx context.Context, kind, jobID string, intent proposals.Intent) (proposals.Intent, error) {
	intent.DiscoveryScopeID = ""
	if kind != jobKindRecurate {
		return intent, nil
	}
	channel, err := s.store.GetChannelForJob(ctx, jobID)
	if err != nil {
		return proposals.Intent{}, fmt.Errorf("resolve recurate channel for Proposal Job %q: %w", jobID, err)
	}
	if strings.TrimSpace(channel.ID) == "" || channel.Status == schedule.StatusDetached {
		return proposals.Intent{}, fmt.Errorf("resolve recurate channel for Proposal Job %q: invalid owner id=%q status=%q",
			jobID, channel.ID, channel.Status)
	}
	intent.DiscoveryScopeID = channel.ID
	return intent, nil
}

// settleSubmitted runs the automatic grants on a freshly committed proposal and tells the
// approvers a request is waiting only if a person still has to act on it. An auto-approved or
// auto-curated proposal never sends the waiting notice, so no one is told about a proposal and
// then told it was approved.
func (s *Service) settleSubmitted(ctx context.Context, job store.Job, p store.Proposal) {
	if s.considerAutomaticApproval(ctx, job, p) {
		return
	}
	if s.notify != nil {
		s.notify.ProposalSubmitted(ctx, p)
	}
}

// considerAutomaticApproval applies the two optional grants only after the
// proposal is durably visible. Generation completion and proposal decision are
// distinct lifecycles: done means inference produced a persisted proposal;
// approval may move that proposal from submitted to approved immediately after.
// It reports whether a grant approved the proposal.
func (s *Service) considerAutomaticApproval(ctx context.Context, job store.Job, p store.Proposal) (approved bool) {
	// The auto-approve grant (§8, §11), if the requester holds one and stays within
	// their cap. Over quota it stays `submitted` for an admin — never denied, because a
	// cap bounds unattended spending rather than judging the request.
	//
	// A refine of a channel (a suggest job that names its channel) never takes it: an admin
	// reviews it in the refine panel, and its acquisitions are charged to the refining admin when
	// they approve. Unattended lineup changes belong to the channel's auto-curate grant.
	if job.Kind != jobKindRecurate && job.ChannelID == "" && s.auto != nil {
		if d, aerr := s.auto.Consider(ctx, p); aerr != nil {
			// A failed auto-approval must not fail the JOB: the proposal exists and an
			// admin can still act on it. Log and move on.
			s.log.Warn("auto-approve failed; proposal stays in the queue",
				"proposal", p.ID, "user", p.CreatedBy, "err", aerr)
		} else if d.Approved {
			s.log.Info("auto-approved within quota",
				"proposal", p.ID, "user", p.CreatedBy, "enqueued", d.Enqueued)
			approved = true
		} else {
			s.log.Debug("not auto-approved", "proposal", p.ID, "reason", d.Reason)
		}
	}
	// The channel-scoped auto-CURATE grant (§8.2): distinct from the per-user grant above.
	// A re-curation job's proposal is authorized by the CHANNEL's AutoCurate opt-in, not a
	// user's grant, and is bounded by the quality bar + title cap rather than the user quota.
	// The durable job kind makes the two grants mutually exclusive; this path cannot follow a
	// requester auto-approval. Same "never fail the job" contract.
	if job.Kind == jobKindRecurate && s.autoCurate != nil {
		if d, aerr := s.autoCurate.Consider(ctx, p); aerr != nil {
			s.log.Warn("auto-curate failed; proposal stays in the queue",
				"proposal", p.ID, "job", job.ID, "err", aerr)
		} else if d.Approved {
			s.log.Info("auto-curated within thresholds",
				"proposal", p.ID, "job", job.ID, "enqueued", d.Enqueued)
			approved = true
		} else if d.Reason != "" {
			s.log.Debug("not auto-curated", "proposal", p.ID, "reason", d.Reason)
		}
	}
	return approved
}

func (s *Service) recordSuggestionQuality(
	ctx context.Context,
	jobID string,
	attempt int,
	duration time.Duration,
	result proposals.Proposal,
	cause error,
) {
	if s.quality == nil {
		return
	}
	at := s.now()
	if duration < 0 {
		duration = 0
	} else if duration > quality.MaxObservationDuration {
		duration = quality.MaxObservationDuration
	}
	trace := result.Trace
	var failure *Failure
	if errors.As(cause, &failure) {
		trace = failure.Trace
	}
	observations := classifySuggestionQuality(trace, duration, cause == nil, failure)
	for _, observation := range observations {
		observation.IdempotencyKey = qualityObservationKey(jobID, attempt, observation.Stage)
		observation.At = at
		if err := s.quality.RecordQualityObservation(ctx, observation); err != nil {
			s.log.Warn("record Proposal Job quality observation", "job", jobID, "attempt", attempt,
				"stage", observation.Stage, "err", err)
		}
	}
}

func classifySuggestionQuality(
	trace proposals.DecisionTrace,
	duration time.Duration,
	succeeded bool,
	failure *Failure,
) []quality.Observation {
	observation := func(stage quality.Stage, outcome quality.Outcome) quality.Observation {
		return quality.Observation{Stage: stage, Outcome: outcome}
	}
	if succeeded {
		retrieval := observation(quality.StageRetrieval, quality.OutcomeSucceeded)
		retrieval.CandidateCount = trace.SurfacedTotal
		generation := observation(quality.StageGeneration, quality.OutcomeSucceeded)
		generation.Duration = duration
		return []quality.Observation{
			retrieval,
			generation,
			observation(quality.StageGrounding, quality.OutcomeAccepted),
		}
	}

	if failure != nil && failure.Code == FailureCodeNoGroundedTitles {
		retrievalOutcome := quality.OutcomeEmpty
		if trace.SurfacedTotal > 0 {
			retrievalOutcome = quality.OutcomeSucceeded
		} else if trace.Terminal == TerminalRetrievalFailure {
			retrievalOutcome = quality.OutcomeFailed
		}
		retrieval := observation(quality.StageRetrieval, retrievalOutcome)
		retrieval.CandidateCount = trace.SurfacedTotal
		generation := observation(quality.StageGeneration, quality.OutcomeAbstained)
		generation.Duration = duration
		return []quality.Observation{
			retrieval,
			generation,
			observation(quality.StageGrounding, quality.OutcomeRejected),
		}
	}

	result := make([]quality.Observation, 0, 2)
	if trace.SurfacedTotal > 0 {
		retrieval := observation(quality.StageRetrieval, quality.OutcomeSucceeded)
		retrieval.CandidateCount = trace.SurfacedTotal
		result = append(result, retrieval)
	} else if trace.Terminal == ReasonRetrievalEmpty || trace.Terminal == TerminalRetrievalFailure {
		outcome := quality.OutcomeEmpty
		if trace.Terminal == TerminalRetrievalFailure {
			outcome = quality.OutcomeFailed
		}
		result = append(result, observation(quality.StageRetrieval, outcome))
	}
	generation := observation(quality.StageGeneration, quality.OutcomeFailed)
	generation.Duration = duration
	return append(result, generation)
}

func qualityObservationKey(jobID string, attempt int, stage quality.Stage) string {
	sum := sha256.Sum256([]byte(jobID + "\x00" + strconv.Itoa(attempt) + "\x00" + string(stage)))
	return hex.EncodeToString(sum[:])
}

func persistedFailure(cause error) (string, error) {
	cause = normalizeContextFailure(cause)
	var failure *Failure
	if !errors.As(cause, &failure) {
		return "", cause
	}
	traceJSON, err := failure.TraceJSON()
	if err != nil {
		return "", fmt.Errorf("safe failure trace unavailable: %w", err)
	}
	return traceJSON, cause
}

// normalizeContextFailure closes raw and provider-wrapped cancellations at the
// worker boundary. The public failure stays generic while any already-bounded
// candidate facts survive with the terminal outcome that actually ended the run.
func normalizeContextFailure(cause error) error {
	if !errors.Is(cause, context.DeadlineExceeded) && !errors.Is(cause, context.Canceled) {
		return cause
	}
	trace := proposals.DecisionTrace{Version: DecisionTraceVersion}
	var failure *Failure
	if errors.As(cause, &failure) {
		trace = failure.Trace.Clone()
		if trace.Version == 0 {
			trace.Version = DecisionTraceVersion
		}
	}
	trace.Terminal = TerminalGenerationFailure
	return NewFailure(FailureCodeGenerationFailed, trace, cause)
}

// persistedMessage is the text stored in jobs.last_error. A Failure's own Error() is
// deliberately generic ("suggestion failed: provider failure"), which left a failed job
// undiagnosable; for provider-caused failures the bounded, sanitized llm cause is appended
// (status + provider message, or the fired timeout budget; never prompt/response content).
func persistedMessage(cause error) string {
	message := cause.Error()
	var failure *Failure
	if !errors.As(cause, &failure) || failure.Cause == nil {
		return message
	}
	if failure.Code != FailureProvider && failure.Code != FailureCodeGenerationFailed {
		return message
	}
	if detail := llm.SafeCause(failure.Cause); detail != "" {
		return message + ": " + detail
	}
	return message
}

// unwrapFailure returns the underlying cause of a Failure for the log line, which the
// generic Failure.Error() otherwise hides.
func unwrapFailure(cause error) error {
	var failure *Failure
	if errors.As(cause, &failure) && failure.Cause != nil {
		return failure.Cause
	}
	return cause
}

func classifyFailure(cause error) string {
	var failure *Failure
	if errors.As(cause, &failure) {
		if errors.Is(failure.Cause, context.DeadlineExceeded) || errors.Is(failure.Cause, context.Canceled) {
			return FailureCodeGenerationFailed
		}
		return failure.Code
	}
	if errors.Is(cause, ErrNoGroundedTitles) {
		return FailureCodeNoGroundedTitles
	}
	return FailureCodeGenerationFailed
}

// IntentHash is the cache key: a stable hash of the normalized intent (§8). Field
// order + case are normalized so semantically-identical intents collide.
func IntentHash(request proposals.Intent) string {
	i := Intent{Intent: request}
	referenceIdentity, _ := webreference.URL(referenceIntentText(i))
	ceiling, excludeUnrated := intentExplicitAudienceCeiling(i), intentExcludesUnrated(i)
	audienceConstraint := ""
	if ceiling != "" || excludeUnrated {
		audienceConstraint = string(ceiling)
		if excludeUnrated {
			audienceConstraint += ":exclude_unrated"
		}
	}
	norm := struct {
		AudienceConstraint string `json:",omitempty"`
		NamedBlockLabel    string `json:",omitempty"`
		PlannerContract    string
		MembershipPolicy   bool
		ReferenceIdentity  string
		Desc, Era, Tone    string
		Rt, Max            int
		Include, Exclude   []string
		// Refine inputs are part of the identity so a refine ("drop the slow ones") never
		// collides in the 24h cache with the original suggestion or a prior refine of the
		// same channel. The current lineup is folded in via its keys (order-independent).
		Refine     string
		LineupKeys []string
		// A fixed name lands verbatim on the proposal, so a cached one with another name must
		// not answer for it. Omitted when empty, leaving unnamed hashes unchanged.
		Name string `json:",omitempty"`
	}{
		AudienceConstraint: audienceConstraint,
		NamedBlockLabel:    strings.ToLower(namedBlockLabel(i)),
		PlannerContract:    PlannerPromptVersion + ":" + PlannerToolSchemaVersion + ":" + PlannerSourceVersion,
		MembershipPolicy:   requiresMembershipEvidence(i),
		ReferenceIdentity:  referenceIdentity,
		Desc:               strings.ToLower(strings.TrimSpace(i.Description)),
		Era:                strings.ToLower(strings.TrimSpace(i.Era)),
		Tone:               strings.ToLower(strings.TrimSpace(i.Tone)),
		Rt:                 i.RuntimeTgt, Max: i.MaxAcquire,
		Include: normSlice(i.MustInclude), Exclude: normSlice(i.MustExclude),
		Refine:     strings.ToLower(strings.TrimSpace(i.RefineText)),
		LineupKeys: lineupKeys(i.CurrentLineup),
		Name:       strings.TrimSpace(i.ChannelName),
	}
	blob, _ := json.Marshal(norm)
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

// lineupKeys extracts + sorts the current-lineup keys for a stable, order-independent
// contribution to the intent hash.
func lineupKeys(ctx []proposals.LineupContext) []string {
	out := make([]string, 0, len(ctx))
	for _, e := range ctx {
		if e.Key != "" {
			out = append(out, e.Key)
		}
	}
	sort.Strings(out)
	return out
}

func normSlice(s []string) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		out = append(out, strings.ToLower(strings.TrimSpace(v)))
	}
	sort.Strings(out)
	return out
}
