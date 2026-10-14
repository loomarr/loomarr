package filler

import (
	"sort"
	"time"
)

// AcquisitionTrigger says who started a filler download. It is intentionally about the
// initiating policy, not the transport: a scheduled source refresh and an operator's explicit
// fetch may both use the same downloader while remaining different operational stories.
type AcquisitionTrigger string

const (
	AcquisitionScheduled AcquisitionTrigger = "scheduled"
	AcquisitionSource    AcquisitionTrigger = "source"
	AcquisitionPull      AcquisitionTrigger = "pull"
	AcquisitionManual    AcquisitionTrigger = "manual"
)

// AcquisitionStatus is the durable execution state of one bounded download run.
type AcquisitionStatus string

const (
	AcquisitionQueued  AcquisitionStatus = "queued"
	AcquisitionRunning AcquisitionStatus = "running"
	AcquisitionSuccess AcquisitionStatus = "success"
	AcquisitionError   AcquisitionStatus = "error"
)

// AcquisitionRun connects a source/pull decision to downloader facts and the pipeline rows that
// resulted from it. Download counts and pipeline outcomes are deliberately separate: one fetched
// compilation may produce many clips, while a skipped remote item produces no pipeline row.
type AcquisitionRun struct {
	ID       string
	Trigger  AcquisitionTrigger
	SourceID string
	PullID   string
	Status   AcquisitionStatus

	Requested int
	Fetched   int
	Skipped   int
	Failed    int
	Empty     int
	Error     string

	StartedAt   time.Time
	CompletedAt time.Time
	UpdatedAt   time.Time

	Outcome   AcquisitionOutcome
	Artifacts AcquisitionArtifactOutcome
	// GapYield splits Outcome by the coverage gap each download was for (#749); nil when no
	// download in the run was steered by a gap.
	GapYield []AcquisitionGapYield
}

// AcquisitionTarget is one URL inside an approved acquisition plan. SourceID stays per-target
// because one approved pull may deliberately draw from several registered sources.
type AcquisitionTarget struct {
	SourceID string
	// RemoteID is the provider's stable item id for candidate-level pulls. Empty is allowed for
	// historical source-level pulls and deliberate one-off URLs.
	RemoteID string
	// Kind is the registered source provider. Empty is reserved for a one-off URL an admin typed,
	// where the ingest boundary must infer the downloader because no source policy exists.
	Kind string
	URL  string
	// DurationMS and Height are what discovery already knew about the item (0 = unknown). They
	// size the storage reservation when the provider cannot report a size at download time.
	DurationMS int64
	Height     int
	// Gap is the channel coverage gap this target was selected for (EraGapKey, #749), recorded
	// on its downloaded artifact. "" when selection was not steered by a gap.
	Gap string
}

// AcquisitionOutcome is the current lifecycle distribution of every clip enrolled from one run.
// Enrolled is the non-overlapping total; the remaining fields partition it by operator ownership.
type AcquisitionOutcome struct {
	Enrolled      int
	Preparing     int
	NeedsDecision int
	Ready         int
	Complete      int
	Rejected      int
	Dismissed     int
}

// AcquisitionArtifactOutcome is the bounded operator-facing projection of manifest state. Only
// the newest repair reason is retained; the durable rows remain the full audit.
type AcquisitionArtifactOutcome struct {
	Staged       int
	Published    int
	Consumed     int
	Repair       int
	RepairReason string
}

func AcquisitionArtifactOutcomeFrom(artifacts []AcquisitionArtifact) AcquisitionArtifactOutcome {
	var out AcquisitionArtifactOutcome
	for _, artifact := range artifacts {
		switch artifact.State {
		case ArtifactStaged:
			out.Staged++
		case ArtifactPublished:
			out.Published++
		case ArtifactConsumed:
			out.Consumed++
		case ArtifactRepair:
			out.Repair++
			if out.RepairReason == "" {
				out.RepairReason = artifact.RepairReason
			}
		}
	}
	return out
}

// AcquisitionGapYield is what one run's downloads for one channel coverage gap became (#749):
// the files acquired for it and the lifecycle of every clip they enrolled.
type AcquisitionGapYield struct {
	Gap       string
	Downloads int
	Outcome   AcquisitionOutcome
}

// AcquisitionGapYieldFrom attributes a run's clips to the gap their download was for, sorted by
// gap. A clip is a download's when its hash is the artifact's clip hash or when it was split out
// of that clip (parents maps a split child's hash to its compilation's). Unsteered downloads and
// clips with no artifact (dropped by hand, pre-manifest) yield no row.
func AcquisitionGapYieldFrom(artifacts []AcquisitionArtifact, rows []ClipPipeline, parents map[string]string, at time.Time) []AcquisitionGapYield {
	byGap := map[string]*AcquisitionGapYield{}
	gapOfClip := map[string]string{}
	for _, artifact := range artifacts {
		if artifact.Gap == "" {
			continue
		}
		y := byGap[artifact.Gap]
		if y == nil {
			y = &AcquisitionGapYield{Gap: artifact.Gap}
			byGap[artifact.Gap] = y
		}
		y.Downloads++
		if artifact.ClipHash != "" {
			gapOfClip[artifact.ClipHash] = artifact.Gap
		}
	}
	if len(byGap) == 0 {
		return nil
	}
	attributed := map[string][]ClipPipeline{}
	for _, row := range rows {
		gap, ok := gapOfClip[row.ClipHash]
		if !ok {
			gap, ok = gapOfClip[parents[row.ClipHash]]
		}
		if ok {
			attributed[gap] = append(attributed[gap], row)
		}
	}
	out := make([]AcquisitionGapYield, 0, len(byGap))
	for gap, y := range byGap {
		y.Outcome = AcquisitionOutcomeFrom(attributed[gap], at)
		out = append(out, *y)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Gap < out[j].Gap })
	return out
}

// AcquisitionOutcomeFrom projects pipeline rows through the same Lifecycle classifier used by
// Incoming and run telemetry. This is the only ownership mapping for acquisition results.
func AcquisitionOutcomeFrom(rows []ClipPipeline, at time.Time) AcquisitionOutcome {
	var out AcquisitionOutcome
	for _, row := range rows {
		out.Enrolled++
		switch row.Lifecycle(at).State {
		case LifecycleRunnable, LifecycleInProgress, LifecycleScheduled:
			out.Preparing++
		case LifecycleNeedsDecision:
			out.NeedsDecision++
		case LifecycleReady:
			out.Ready++
		case LifecycleComplete:
			out.Complete++
		case LifecycleRejected:
			out.Rejected++
		case LifecycleDismissed:
			out.Dismissed++
		}
	}
	return out
}
