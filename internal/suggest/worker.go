package suggest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

// This file is the job execution model (§8): submissions become persisted jobs;
// an in-process worker pool claims due jobs (ClaimDueJobs, SKIP-LOCKED on
// Postgres) and runs the suggester, bounded by JOB_TIMEOUT so one hung LLM call
// can never starve the queue. Proposals are persisted so the approval queue and
// My proposals survive restarts.

// ProposalStore is the slice of the store the worker/submission path needs.
type ProposalStore interface {
	CreateJob(ctx context.Context, j store.Job) error
	GetJob(ctx context.Context, id string) (store.Job, error)
	UpdateJob(ctx context.Context, j store.Job) error
	ClaimDueJobs(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]store.Job, error)
	FindJobByIntentHash(ctx context.Context, hash string, since time.Time) (store.Job, error)
	CreateProposal(ctx context.Context, p store.Proposal) error
	GetProposal(ctx context.Context, id string) (store.Proposal, error)
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
	newID     func() string
	now       func() time.Time
	emit      ProgressEmitter // optional (§8 SSE progress); nil ⇒ no live frames
	// auto applies the §11 auto-approve grant. nil ⇒ every proposal waits for an admin,
	// which is the correct default: the gate is closed unless something opens it.
	auto *AutoApprover
	// binder materializes an approved proposal onto a channel (§7). nil ⇒ an
	// auto-approval enqueues acquisitions but does not rebind the channel — the
	// composition root always wires one alongside WithAutoApprove so that gap is
	// closed in production; nil stays a safe default for unit tests that don't
	// care about channels.
	binder ChannelBinder
	// autoCurate applies the §8.2 channel-scoped auto-curate grant to a re-curation
	// proposal. nil ⇒ re-curation proposals wait for an admin (the closed default). Distinct
	// from `auto` (per-user quota); this one is authorized by the channel's AutoCurate opt-in.
	autoCurate ChannelAutoCurator
}

// ChannelAutoCurator applies the §8.2 channel-scoped auto-curate grant: it decides whether a
// re-curation proposal may auto-approve because its CHANNEL opted in (AutoCurate), bounded by
// the quality bar + title cap — never a user quota. The suggest package declares the interface
// (dependency inversion); internal/recurate implements it, wired at the composition root, so
// suggest never imports recurate. Returns the same Decision shape the per-user grant does.
type ChannelAutoCurator interface {
	Consider(ctx context.Context, p store.Proposal) (Decision, error)
}

// ChannelBinder materializes an approved proposal onto a channel (§7): create it on
// first approval, patch it (preserving operator-owned fields) on re-approval or
// refine. The suggest package declares this interface itself (dependency
// inversion) — it needs exactly one method, and *binder.Binder satisfies it
// structurally without suggest importing internal/binder.
type ChannelBinder interface {
	BindApprovedChannel(ctx context.Context, p store.Proposal) (channelID string, err error)
}

// WithAutoApprove enables the per-user auto-approve grant (§8, §11). Wired at the
// composition root, where the settings service and the full store are both available.
func (s *Service) WithAutoApprove(a *AutoApprover) *Service {
	s.auto = a
	return s
}

// WithChannelBinder wires the shared channel-binding logic (§7) so an auto-approved
// proposal ALSO rebinds its channel — the fix for the gap where auto-approve
// enqueued acquisitions but never touched the channel a refine was meant to update.
// Mirrors WithAutoApprove's functional-option style.
func (s *Service) WithChannelBinder(b ChannelBinder) *Service {
	s.binder = b
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
// bug, never a correctness bug — GET /v1/suggestions/{id} stays the source of
// truth. The composition root implements it over events.Bus; unit tests leave it
// nil. Kept as a narrow interface so the suggest package doesn't import events.
type ProgressEmitter interface {
	SuggestionPhase(jobID, phase string)
}

// WithProgressEmitter wires the SSE progress emitter and returns the service for
// chaining (matches the runners' WithInterval style). Nil is fine — no frames.
func (s *Service) WithProgressEmitter(e ProgressEmitter) *Service {
	s.emit = e
	return s
}

// emitPhase publishes one phase frame if an emitter is wired.
func (s *Service) emitPhase(jobID string, p Phase) {
	if s.emit != nil {
		s.emit.SuggestionPhase(jobID, string(p))
	}
}

// Config parameterizes the service (from §15).
type Config struct {
	Workers  int
	Timeout  time.Duration
	CacheTTL time.Duration
}

// NewService builds the suggestion service. newID generates job/proposal ids
// (uuid-like); now defaults to time.Now.
func NewService(st ProposalStore, sug *Suggester, cfg Config, newID func() string, now func() time.Time, log *slog.Logger) *Service {
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
		store: st, suggester: sug, log: log,
		workers: cfg.Workers, timeout: cfg.Timeout, cacheTTL: cfg.CacheTTL,
		newID: newID, now: now,
	}
}

// Submit enqueues a suggestion job for an intent (§8). If an identical intent was
// generated within the cache TTL, it returns that job's id instead of enqueuing
// a duplicate (§8 cache by hash(normalized intent)). Returns the job id.
func (s *Service) Submit(ctx context.Context, intent Intent, createdBy string) (string, error) {
	hash := IntentHash(intent)
	// Cache hit ONLY on a recent SUCCESSFUL job. A failed job — e.g. one that found
	// no grounded titles (ErrNoGroundedTitles) or hit a timeout — must NOT wedge
	// re-submits of the same intent for the cache TTL; the operator retrying is
	// exactly how they recover, so a re-submit re-runs generation.
	if cached, err := s.store.FindJobByIntentHash(ctx, hash, s.now().Add(-s.cacheTTL)); err == nil && cached.Status == "done" {
		return cached.ID, nil // cache hit — reuse the recent successful job
	}
	blob, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("marshal intent: %w", err)
	}
	now := s.now()
	job := store.Job{
		ID: s.newID(), Kind: "suggest", Status: "queued",
		IntentJSON: string(blob), IntentHash: hash, CreatedBy: createdBy,
		Deadline: now, CreatedAt: now, UpdatedAt: now, // due immediately
	}
	if err := s.store.CreateJob(ctx, job); err != nil {
		return "", err
	}
	return job.ID, nil
}

// Refine re-runs an EXISTING suggestion job with a refine-flavored intent (§7 refine).
// It re-queues the channel's own `IntentRef` job rather than minting a new one, so the
// refined proposal lands on that job — and approving it patches the SAME channel via the
// existing `channelForIntent`/newest-approved-proposal path (no new schema, no duplicate
// channel). Bypasses the intent-hash cache deliberately: a refine always re-generates
// (the operator asked for a change; a cached identical run would be surprising). Returns
// the job id (== the channel's IntentRef) for the caller to poll for the new proposal.
func (s *Service) Refine(ctx context.Context, jobID string, intent Intent) (string, error) {
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return "", fmt.Errorf("refine: load job %q: %w", jobID, err)
	}
	blob, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("refine: marshal intent: %w", err)
	}
	now := s.now()
	// Re-queue in place: swap in the refine intent and make it due immediately. The worker
	// picks it up, generates a fresh proposal on this job, and the newest-approved wins.
	job.IntentJSON = string(blob)
	job.IntentHash = IntentHash(intent)
	job.Status = "queued"
	job.LastError = ""
	job.Deadline = now
	job.UpdatedAt = now
	if err := s.store.UpdateJob(ctx, job); err != nil {
		return "", fmt.Errorf("refine: requeue job: %w", err)
	}
	return job.ID, nil
}

// Run starts the worker pool and blocks until ctx is cancelled (§8). Each worker
// polls for due jobs and runs them; the pool shares the ClaimDueJobs lease so no
// job runs twice.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	s.log.Info("suggestion worker pool started", "workers", s.workers, "timeout", s.timeout)
	sem := make(chan struct{}, s.workers)
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
	// Lease horizon must exceed JOB_TIMEOUT so a running job isn't re-claimed.
	jobs, err := s.store.ClaimDueJobs(ctx, s.now(), s.timeout+time.Minute, s.workers)
	if err != nil {
		s.log.Error("claim due jobs", "err", err)
		return
	}
	for _, job := range jobs {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		go func(j store.Job) {
			defer func() { <-sem }()
			s.runJob(ctx, j)
		}(job)
	}
}

// runJob executes one suggestion job under JOB_TIMEOUT (§8). A hung LLM hits the
// deadline and the job fails cleanly; the pool keeps draining others.
func (s *Service) runJob(ctx context.Context, job store.Job) {
	jobCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Thread live progress (§8) into the pipeline: the suggester reports
	// searching→reasoning→scoring off this context; done/failed are emitted here.
	jobCtx = WithProgress(jobCtx, func(p Phase) { s.emitPhase(job.ID, p) })

	var intent Intent
	if err := json.Unmarshal([]byte(job.IntentJSON), &intent); err != nil {
		s.failJob(ctx, job, fmt.Errorf("bad intent json: %w", err))
		return
	}

	job.Status = "running"
	job.UpdatedAt = s.now()
	_ = s.store.UpdateJob(ctx, job)

	prop, err := s.suggester.Suggest(jobCtx, intent)
	if err != nil {
		s.failJob(ctx, job, err)
		return
	}
	propBlob, err := json.Marshal(prop)
	if err != nil {
		s.failJob(ctx, job, fmt.Errorf("marshal proposal: %w", err))
		return
	}
	// Persist the proposal into the approval queue (submitted). created_by carries
	// from the job (§8 My proposals). NOTHING auto-executes (§8 human-in-the-loop).
	now := s.now()
	p := store.Proposal{
		ID: s.newID(), JobID: job.ID, Status: "submitted", CreatedBy: job.CreatedBy,
		ProposalJSON: string(propBlob), CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.CreateProposal(ctx, p); err != nil {
		s.failJob(ctx, job, fmt.Errorf("persist proposal: %w", err))
		return
	}
	// The auto-approve grant (§8, §11), if the requester holds one and stays within
	// their cap. Over quota it stays `submitted` for an admin — never denied, because a
	// cap bounds unattended spending rather than judging the request.
	approved := false
	if s.auto != nil {
		if d, aerr := s.auto.Consider(ctx, p); aerr != nil {
			// A failed auto-approval must not fail the JOB: the proposal exists and an
			// admin can still act on it. Log and move on.
			s.log.Warn("auto-approve failed; proposal stays in the queue",
				"proposal", p.ID, "user", p.CreatedBy, "err", aerr)
		} else if d.Approved {
			approved = true
			s.log.Info("auto-approved within quota",
				"proposal", p.ID, "user", p.CreatedBy, "enqueued", d.Enqueued)
		} else {
			s.log.Debug("not auto-approved", "proposal", p.ID, "reason", d.Reason)
		}
	}
	// The channel-scoped auto-CURATE grant (§8.2): distinct from the per-user grant above.
	// A re-curation job's proposal is authorized by the CHANNEL's AutoCurate opt-in, not a
	// user's grant, and is bounded by the quality bar + title cap rather than the user quota.
	// Only tried when the per-user path didn't already approve (a proposal is approved once).
	// Same "never fail the job" contract.
	if !approved && s.autoCurate != nil {
		if d, aerr := s.autoCurate.Consider(ctx, p); aerr != nil {
			s.log.Warn("auto-curate failed; proposal stays in the queue",
				"proposal", p.ID, "job", job.ID, "err", aerr)
		} else if d.Approved {
			approved = true
			s.log.Info("auto-curated within thresholds",
				"proposal", p.ID, "job", job.ID, "enqueued", d.Enqueued)
		} else if d.Reason != "" {
			s.log.Debug("not auto-curated", "proposal", p.ID, "reason", d.Reason)
		}
	}
	// Rebind the channel (§7/§8/§11) after ANY auto-approval — per-user or auto-curate. The
	// manual-approve HTTP handler always did this; auto-approve previously stopped at
	// enqueueing acquisitions, leaving the channel's lineup stale. Same non-fatal handling as
	// the manual path: a bind failure must not undo or fail an approval that already stands.
	if approved && s.binder != nil {
		if _, berr := s.binder.BindApprovedChannel(ctx, p); berr != nil {
			s.log.Warn("auto-approved, but the channel could not be bound",
				"proposal", p.ID, "job", job.ID, "err", berr)
		}
	}

	job.Status = "done"
	job.UpdatedAt = now
	_ = s.store.UpdateJob(ctx, job)
	s.emitPhase(job.ID, PhaseDone)
}

func (s *Service) failJob(ctx context.Context, job store.Job, cause error) {
	s.log.Error("suggestion job failed", "job", job.ID, "err", cause)
	job.Status = "failed"
	job.LastError = cause.Error()
	job.Attempts++
	job.UpdatedAt = s.now()
	_ = s.store.UpdateJob(ctx, job)
	s.emitPhase(job.ID, PhaseFailed)
}

// IntentHash is the cache key: a stable hash of the normalized intent (§8). Field
// order + case are normalized so semantically-identical intents collide.
func IntentHash(i Intent) string {
	norm := struct {
		Desc, Era, Tone  string
		Rt, Max          int
		Include, Exclude []string
		// Refine inputs are part of the identity so a refine ("drop the slow ones") never
		// collides in the 24h cache with the original suggestion or a prior refine of the
		// same channel. The current lineup is folded in via its keys (order-independent).
		Refine     string
		LineupKeys []string
	}{
		Desc: strings.ToLower(strings.TrimSpace(i.Description)),
		Era:  strings.ToLower(strings.TrimSpace(i.Era)),
		Tone: strings.ToLower(strings.TrimSpace(i.Tone)),
		Rt:   i.RuntimeTgt, Max: i.MaxAcquire,
		Include: normSlice(i.MustInclude), Exclude: normSlice(i.MustExclude),
		Refine:     strings.ToLower(strings.TrimSpace(i.RefineText)),
		LineupKeys: lineupKeys(i.CurrentLineup),
	}
	blob, _ := json.Marshal(norm)
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

// lineupKeys extracts + sorts the current-lineup keys for a stable, order-independent
// contribution to the intent hash.
func lineupKeys(ctx []LineupContext) []string {
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
