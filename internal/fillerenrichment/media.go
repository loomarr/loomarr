package fillerenrichment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/taxonomy"
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
	AxisKind: true, AxisAudience: true, AxisBrand: true, AxisProduct: true,
	AxisFormat: true, AxisSeasonal: true, AxisAudienceCue: true, AxisPresentation: true,
}

var visionAxes = map[Axis]bool{
	AxisKind: true, AxisEra: true, AxisAudience: true, AxisBrand: true, AxisProduct: true,
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
// Existing media signals and operator answers, including intentional empty answers, are terminal
// for automation. A completed empty answer from a weaker automated pass is not: a richer signal is
// allowed one versioned attempt to replace that honest absence.
func PlanMediaWork(candidate Candidate, states []State, available MediaCapabilities) MediaWork {
	current := make(map[Axis]State, len(states))
	for _, state := range states {
		current[state.Axis] = state
	}
	unresolved := func(capabilityAxes map[Axis]bool) bool {
		for axis := range capabilityAxes {
			state, found := current[axis]
			if !found || state.Status == StatusMissing || state.Status == StatusUnsupported || state.Status == StatusStale {
				return true
			}
			if state.Evidence.Kind == EvidenceOperator || !state.Value.empty() {
				continue
			}
			return true
		}
		return false
	}
	return MediaWork{
		Transcript: available.Transcript && strings.TrimSpace(candidate.Transcript) == "" && unresolved(transcriptAxes),
		Vision:     available.Vision && !candidate.VisionTagged && unresolved(visionAxes),
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
	candidates, err := r.repository.ListCandidates(ctx, selection.Producer, selection.ProducerVersion, taxonomyVersion, limit)
	if err != nil {
		return result, fmt.Errorf("list %s enrichment candidates: %w", r.kind, err)
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
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
				capabilityErrors = append(capabilityErrors,
					fmt.Errorf("run %s enrichment for %s: %w", r.kind, candidate.ClipHash, err))
				continue
			}
		}
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
