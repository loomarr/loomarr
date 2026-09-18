package filler

import (
	"sort"
	"strings"
	"time"
)

const (
	MinimumPreparationSamples  = 3
	MaximumEstimatedQueueAhead = 25
	PreparationEvidenceWindow  = 30 * 24 * time.Hour
	PreparationStaleAfter      = 30 * time.Minute
)

// PreparationWork joins one pipeline fact with the coarse media duration needed for local
// calibration. It contains no transcript, provider response, credential, or private path.
type PreparationWork struct {
	Pipeline   ClipPipeline
	DurationMs int64
}

type ReadyEstimate struct {
	Lower time.Duration
	Upper time.Duration
}

// ProjectPreparationProgress calculates the conservative whole-attempt percentage from the
// server's canonical work plan. It never mutates legacy rows whose attempt identity is unknown.
func ProjectPreparationProgress(row ClipPipeline) int {
	if row.PreparationAttempt <= 0 {
		return -1
	}
	if row.Disposition == DispositionReady {
		return 100
	}
	completed := 0
	seen := make(map[StageID]struct{}, len(row.Stages))
	for _, record := range row.Stages {
		if _, duplicate := seen[record.Stage]; duplicate {
			continue
		}
		if StageIndex(record.Stage) < 0 {
			continue
		}
		if record.Status == StatusDone || record.Status == StatusSkipped {
			completed++
			seen[record.Stage] = struct{}{}
		}
	}
	units := completed * 100
	if row.Disposition == DispositionRunning && row.Status == StatusRunning && row.Progress >= 0 && row.Progress <= 100 {
		if _, alreadyComplete := seen[row.Stage]; !alreadyComplete && StageIndex(row.Stage) >= 0 {
			units += row.Progress
		}
	}
	projected := units / len(StageOrder)
	if projected > 99 {
		projected = 99
	}
	if row.PreparationProgress > projected {
		return row.PreparationProgress
	}
	return projected
}

func applyPreparationProgress(row *ClipPipeline) {
	row.PreparationProgress = ProjectPreparationProgress(*row)
}

// EstimatePreparationReady derives a range from comparable Ready attempts. QueueAhead must be the
// bounded scheduler-owned work ahead of current; if any item lacks evidence, the answer is absent.
func EstimatePreparationReady(current PreparationWork, history, queueAhead []PreparationWork, at time.Time) (ReadyEstimate, bool) {
	row := current.Pipeline
	if current.DurationMs <= 0 || row.PreparationAttempt <= 0 || row.PreparationProgress < 0 || row.Disposition != DispositionRunning ||
		row.Status == StatusFailed || row.NextRun.After(at) || row.UpdatedAt.IsZero() || at.Sub(row.UpdatedAt) > PreparationStaleAfter ||
		(row.PreparationStartReason == PreparationStartedByRestart && row.PreparationProgress == 0) ||
		len(queueAhead) > MaximumEstimatedQueueAhead {
		return ReadyEstimate{}, false
	}
	lower, upper, ok := comparableRemaining(current, history)
	if !ok {
		return ReadyEstimate{}, false
	}
	for _, ahead := range queueAhead {
		lo, hi, found := comparableRemaining(ahead, history)
		if !found {
			return ReadyEstimate{}, false
		}
		lower += lo
		upper += hi
	}
	if upper < lower {
		upper = lower
	}
	return ReadyEstimate{Lower: lower, Upper: upper}, true
}

func comparableRemaining(work PreparationWork, history []PreparationWork) (time.Duration, time.Duration, bool) {
	bySignature := make(map[string][]time.Duration)
	for _, sample := range history {
		if sample.DurationMs <= 0 || durationBucket(sample.DurationMs) != durationBucket(work.DurationMs) || !matchesKnownPlan(work.Pipeline, sample.Pipeline) {
			continue
		}
		remaining, ok := sampleRemaining(work.Pipeline, sample.Pipeline)
		if ok {
			signature := PreparationPlanSignature(sample.Pipeline)
			bySignature[signature] = append(bySignature[signature], remaining)
		}
	}
	var selected string
	for signature, values := range bySignature {
		if len(values) > len(bySignature[selected]) || len(values) == len(bySignature[selected]) && signature < selected {
			selected = signature
		}
	}
	values := bySignature[selected]
	if len(values) < MinimumPreparationSamples {
		return 0, 0, false
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values[0], values[len(values)-1], true
}

func durationBucket(ms int64) string {
	switch {
	case ms <= 30_000:
		return "short"
	case ms <= 90_000:
		return "medium"
	default:
		return "long"
	}
}

func matchesKnownPlan(current, sample ClipPipeline) bool {
	sampleByStage := make(map[StageID]StageRecord, len(sample.Stages))
	for _, record := range sample.Stages {
		sampleByStage[record.Stage] = record
	}
	for _, record := range current.Stages {
		if record.Status != StatusDone && record.Status != StatusSkipped {
			continue
		}
		observed, ok := sampleByStage[record.Stage]
		if !ok || observed.Status != record.Status {
			return false
		}
	}
	return true
}

func sampleRemaining(current, sample ClipPipeline) (time.Duration, bool) {
	if sample.Disposition != DispositionReady || sample.PreparationAttempt <= 0 || sample.PreparationProgress != 100 ||
		sample.PreparationStartedAt.IsZero() || len(sample.Stages) != len(StageOrder) {
		return 0, false
	}
	records := make(map[StageID]StageRecord, len(sample.Stages))
	for _, record := range sample.Stages {
		if _, duplicate := records[record.Stage]; duplicate || record.Attempts > 1 {
			return 0, false
		}
		if record.Status == StatusDone {
			if record.StartedAt.IsZero() || record.At.Before(record.StartedAt) {
				return 0, false
			}
		} else if record.Status != StatusSkipped {
			return 0, false
		}
		records[record.Stage] = record
	}
	remaining := time.Duration(0)
	currentIndex := StageIndex(current.Stage)
	if currentIndex < 0 {
		currentIndex = 0
	}
	for index, stage := range StageOrder {
		record, ok := records[stage]
		if !ok {
			return 0, false
		}
		if index < currentIndex || record.Status == StatusSkipped {
			continue
		}
		duration := record.At.Sub(record.StartedAt)
		if index == currentIndex && current.Status == StatusRunning && current.Progress >= 0 && current.Progress <= 100 {
			duration = duration * time.Duration(100-current.Progress) / 100
		}
		remaining += duration
	}
	return remaining, true
}

// PreparationPlanSignature is persisted nowhere; it is a coarse grouping key for tests and
// diagnostics and deliberately exposes only done/not-needed stage applicability.
func PreparationPlanSignature(row ClipPipeline) string {
	byStage := make(map[StageID]StageStatus, len(row.Stages))
	for _, record := range row.Stages {
		byStage[record.Stage] = record.Status
	}
	parts := make([]string, 0, len(StageOrder))
	for _, stage := range StageOrder {
		switch byStage[stage] {
		case StatusDone:
			parts = append(parts, string(stage)+":done")
		case StatusSkipped:
			parts = append(parts, string(stage)+":skipped")
		default:
			parts = append(parts, string(stage)+":unknown")
		}
	}
	return strings.Join(parts, "|")
}
