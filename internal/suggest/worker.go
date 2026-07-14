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
	job.Status = "done"
	job.UpdatedAt = now
	_ = s.store.UpdateJob(ctx, job)
}

func (s *Service) failJob(ctx context.Context, job store.Job, cause error) {
	s.log.Error("suggestion job failed", "job", job.ID, "err", cause)
	job.Status = "failed"
	job.LastError = cause.Error()
	job.Attempts++
	job.UpdatedAt = s.now()
	_ = s.store.UpdateJob(ctx, job)
}

// IntentHash is the cache key: a stable hash of the normalized intent (§8). Field
// order + case are normalized so semantically-identical intents collide.
func IntentHash(i Intent) string {
	norm := struct {
		Desc, Era, Tone  string
		Rt, Max          int
		Include, Exclude []string
	}{
		Desc: strings.ToLower(strings.TrimSpace(i.Description)),
		Era:  strings.ToLower(strings.TrimSpace(i.Era)),
		Tone: strings.ToLower(strings.TrimSpace(i.Tone)),
		Rt:   i.RuntimeTgt, Max: i.MaxAcquire,
		Include: normSlice(i.MustInclude), Exclude: normSlice(i.MustExclude),
	}
	blob, _ := json.Marshal(norm)
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

func normSlice(s []string) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		out = append(out, strings.ToLower(strings.TrimSpace(v)))
	}
	sort.Strings(out)
	return out
}
