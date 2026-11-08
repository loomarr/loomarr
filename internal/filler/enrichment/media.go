package enrichment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/filler/taxonomy"
)

// Capability identifies one optional media signal that can improve descriptive understanding.
// It is deliberately smaller than the readiness pipeline's stage vocabulary: these capabilities
// may add facts to a Ready clip but can never decide whether it is playable.
type Capability string

const (
	CapabilityTranscript Capability = "transcript"
	CapabilityVision     Capability = "vision"
)

func (c Capability) valid() bool {
	return c == CapabilityTranscript || c == CapabilityVision
}

// MediaCapabilities are the capabilities currently available to the household. Availability is
// live policy, not clip state: a capability that appears later makes unresolved clips eligible.
type MediaCapabilities struct {
	Transcript bool
	Vision     bool
}

// MediaWork is the coordinator's public answer to "what useful work remains for this clip?".
type MediaWork struct {
	Transcript bool
	Vision     bool
}

var transcriptAxes = map[Axis]bool{
	AxisRole: true, AxisAudience: true, AxisBrand: true, AxisProduct: true,
	AxisFormat: true, AxisSeasonal: true, AxisAudienceCue: true, AxisPresentation: true,
}

var visionAxes = map[Axis]bool{
	AxisRole: true, AxisEra: true, AxisAudience: true, AxisBrand: true, AxisProduct: true,
	AxisFormat: true, AxisSeasonal: true, AxisAudienceCue: true, AxisPresentation: true,
}

func taxonomyAxis(axis Axis) bool {
	switch axis {
	case AxisProduct, AxisFormat, AxisSeasonal, AxisAudienceCue, AxisPresentation:
		return true
	default:
		return false
	}
}

// PlanMediaWork returns only media work that can still improve an unresolved descriptive axis.
// The runner has already established that this exact capability identity has not completed, so an
// older transcript or frame pass does not suppress a newly selected model/prompt. Operator answers,
// including intentional empty answers, remain terminal. Grounded frames may also improve weaker
// inference or source defaults; transcription only opens axes that still lack a useful value.
func PlanMediaWork(_ Candidate, states []State, available MediaCapabilities) MediaWork {
	current := make(map[Axis]State, len(states))
	for _, state := range states {
		current[state.Axis] = state
	}
	unresolved := func(capabilityAxes map[Axis]bool, replaceBelow EvidenceRank) bool {
		for axis := range capabilityAxes {
			state, found := current[axis]
			if !found || state.Status == StatusMissing || state.Status == StatusUnsupported || state.Status == StatusStale {
				return true
			}
			if state.Evidence.Kind == EvidenceOperator {
				continue
			}
			if state.Value.empty() {
				return true
			}
			if rank, ok := state.Evidence.Kind.Rank(); ok && rank < replaceBelow {
				return true
			}
		}
		return false
	}
	return MediaWork{
		Transcript: available.Transcript && unresolved(transcriptAxes, RankInference),
		Vision:     available.Vision && unresolved(visionAxes, RankContent),
	}
}

// CapabilitySelection binds one optional pass to the exact capability identity whose cost was
// paid. An unavailable selection stays completely quiet: it neither queries nor stamps work.
type CapabilitySelection struct {
	Available       bool
	Producer        string
	ProducerVersion string
}

type capabilityRepository interface {
	Repository
	ListCapabilityCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]Candidate, error)
	ListStates(ctx context.Context, clipHash string) ([]State, error)
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
}

type CapabilityResult struct {
	States      []State
	Observation *MediaObservation
}

type CapabilityExecutor func(context.Context, Candidate) (CapabilityResult, error)

// CapabilityRunner performs one bounded optional media pass. The executor is the external media
// boundary; this module owns eligibility, identity, completion, and failure isolation.
type CapabilityRunner struct {
	kind       Capability
	repository capabilityRepository
	selection  func() CapabilitySelection
	execute    CapabilityExecutor
	limit      func() int
	now        func() time.Time

	mu      sync.Mutex
	backoff map[string]capabilityRetry // clip hash → when it may run again after failing
}

// capabilityRetry is one clip's failure streak. A failed clip is stamped with nothing (its
// evidence was never paid for), so without this it stayed the first candidate and was retried
// every pass. On the household install five clips whose frames ffmpeg refused filled every slot of
// every pass for days: 1,726 failures, no other clip enriched (#2456). Kept in memory on purpose:
// a restart costs one retry per failing clip, and a schema change for that isn't worth it.
type capabilityRetry struct {
	failures int
	next     time.Time
}

// Capability retry backoff: 10 minutes after the first failure, doubling to a day. A transient
// fault (model server down) clears within the first steps; a clip that always fails costs one
// attempt a day.
const (
	capabilityRetryBase = 10 * time.Minute
	capabilityRetryMax  = 24 * time.Hour
)

func capabilityRetryDelay(failures int) time.Duration {
	delay := capabilityRetryBase
	for i := 1; i < failures && delay < capabilityRetryMax; i++ {
		delay *= 2
	}
	return min(delay, capabilityRetryMax)
}

// waiting reports the clips still backing off at now. A clip whose wait is over keeps its streak,
// so failing again doubles the delay; only a success clears it.
func (r *CapabilityRunner) waiting(now time.Time) map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	for hash, retry := range r.backoff {
		if now.Before(retry.next) {
			out[hash] = true
		}
	}
	return out
}

func (r *CapabilityRunner) failed(hash string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.backoff == nil {
		r.backoff = map[string]capabilityRetry{}
	}
	retry := r.backoff[hash]
	retry.failures++
	retry.next = now.Add(capabilityRetryDelay(retry.failures))
	r.backoff[hash] = retry
}

func (r *CapabilityRunner) succeeded(hash string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.backoff, hash)
}

func NewCapabilityRunner(kind Capability, repository capabilityRepository,
	selection func() CapabilitySelection, execute CapabilityExecutor,
	limit func() int, now func() time.Time) *CapabilityRunner {
	if limit == nil {
		limit = func() int { return 10 }
	}
	if now == nil {
		now = time.Now
	}
	return &CapabilityRunner{kind: kind, repository: repository, selection: selection,
		execute: execute, limit: limit, now: now}
}

func (r *CapabilityRunner) Run(ctx context.Context) (RunResult, error) {
	var result RunResult
	if r == nil || !r.kind.valid() || r.repository == nil || r.selection == nil || r.execute == nil {
		return result, nil
	}
	limit := r.limit()
	selection := r.selection()
	if limit <= 0 || !selection.Available || strings.TrimSpace(selection.Producer) == "" ||
		strings.TrimSpace(selection.ProducerVersion) == "" {
		return result, nil
	}
	taxonomyVersion := ""
	if r.kind == CapabilityVision {
		taxa, err := r.repository.ListTaxa(ctx)
		if err != nil {
			return result, fmt.Errorf("load vision enrichment taxonomy: %w", err)
		}
		taxonomyVersion, err = taxonomyIdentity(taxonomy.New(taxa))
		if err != nil {
			return result, err
		}
	}
	// Ask for enough extra rows that clips still backing off can't take every slot.
	waiting := r.waiting(r.now())
	listed, err := r.repository.ListCapabilityCandidates(ctx, selection.Producer, selection.ProducerVersion, taxonomyVersion, limit+len(waiting))
	if err != nil {
		return result, fmt.Errorf("list %s enrichment candidates: %w", r.kind, err)
	}
	candidates := make([]Candidate, 0, limit)
	for _, candidate := range listed {
		if !waiting[candidate.ClipHash] && len(candidates) < limit {
			candidates = append(candidates, candidate)
		}
	}
	result.Considered = len(candidates)
	var capabilityErrors []error
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		states, err := r.repository.ListStates(ctx, candidate.ClipHash)
		if err != nil {
			return result, fmt.Errorf("load enrichment state for %s: %w", candidate.ClipHash, err)
		}
		availability := MediaCapabilities{}
		if r.kind == CapabilityTranscript {
			availability.Transcript = true
		} else {
			availability.Vision = true
		}
		work := PlanMediaWork(candidate, states, availability)
		needed := work.Transcript || work.Vision
		var capabilityResult CapabilityResult
		if needed {
			capabilityResult, err = r.execute(ctx, candidate)
			if err != nil {
				result.Failed++
				r.failed(candidate.ClipHash, r.now())
				capabilityErrors = append(capabilityErrors,
					fmt.Errorf("run %s enrichment for %s: %w", r.kind, candidate.ClipHash, err))
				continue
			}
		}
		r.succeeded(candidate.ClipHash)
		completedAt := r.now().UTC()
		for i := range capabilityResult.States {
			capabilityResult.States[i].ClipHash = candidate.ClipHash
			capabilityResult.States[i].Evidence.Producer = selection.Producer
			capabilityResult.States[i].Evidence.ProducerVersion = selection.ProducerVersion
			capabilityResult.States[i].Evidence.ObservedAt = completedAt
			if taxonomyAxis(capabilityResult.States[i].Axis) {
				capabilityResult.States[i].Evidence.TaxonomyVersion = taxonomyVersion
			}
		}
		if _, err := r.repository.ApplyPass(ctx, Pass{
			ClipHash: candidate.ClipHash, Producer: selection.Producer,
			ProducerVersion: selection.ProducerVersion, TaxonomyVersion: taxonomyVersion,
			CompletedAt: completedAt, States: capabilityResult.States,
			Observation: capabilityResult.Observation,
		}); err != nil {
			return result, fmt.Errorf("record %s enrichment for %s: %w", r.kind, candidate.ClipHash, err)
		}
	}
	return result, errors.Join(capabilityErrors...)
}
