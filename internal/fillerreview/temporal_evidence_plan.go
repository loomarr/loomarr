package fillerreview

import (
	"fmt"
	"slices"
	"sort"

	"github.com/loomarr/loomarr/internal/mediatools"
)

const (
	TemporalEvidencePlanSchemaVersion = 1
	TemporalEvidenceTransitionPadMS   = int64(250)
	TemporalEvidenceMinCleanupMS      = int64(300)
	TemporalEvidenceCutContextMS      = int64(150)
	TemporalEvidenceMaxFrames         = 12
	temporalEvidenceSceneCutLimit     = 3
)

type TemporalEvidencePlan struct {
	SchemaVersion   int                     `json:"schemaVersion"`
	SourceStartMS   int64                   `json:"sourceStartMs"`
	SourceEndMS     int64                   `json:"sourceEndMs"`
	EvidenceStartMS int64                   `json:"evidenceStartMs"`
	EvidenceEndMS   int64                   `json:"evidenceEndMs"`
	LeadingCleanup  *TemporalEvidenceRegion `json:"leadingCleanup,omitempty"`
	TrailingCleanup *TemporalEvidenceRegion `json:"trailingCleanup,omitempty"`
	SceneCutsMS     []int64                 `json:"sceneCutsMs"`
	FrameTimesMS    []int64                 `json:"frameTimesMs"`
}

type TemporalEvidenceRegion struct {
	StartMS int64  `json:"startMs"`
	EndMS   int64  `json:"endMs"`
	Reason  string `json:"reason"`
}

func BuildTemporalEvidencePlan(durationMS int64, black, silence []mediatools.Interval, sceneCuts []int64) (TemporalEvidencePlan, error) {
	if durationMS <= 0 {
		return TemporalEvidencePlan{}, fmt.Errorf("duration must be positive, got %d", durationMS)
	}
	if err := validateTemporalEvidenceIntervals(durationMS, black, "black"); err != nil {
		return TemporalEvidencePlan{}, err
	}
	if err := validateTemporalEvidenceIntervals(durationMS, silence, "silence"); err != nil {
		return TemporalEvidencePlan{}, err
	}
	for _, cut := range sceneCuts {
		if cut < 0 || cut >= durationMS {
			return TemporalEvidencePlan{}, fmt.Errorf("scene cut %d is outside 0..%d", cut, durationMS)
		}
	}
	plan := TemporalEvidencePlan{SchemaVersion: TemporalEvidencePlanSchemaVersion, SourceEndMS: durationMS, EvidenceEndMS: durationMS}
	if overlap, ok := temporalEvidenceEdgeOverlap(black, silence, 0, durationMS, true); ok && overlap.EndMS-overlap.StartMS >= TemporalEvidenceMinCleanupMS {
		end := max(int64(0), overlap.EndMS-TemporalEvidenceTransitionPadMS)
		if end > 0 {
			plan.LeadingCleanup = &TemporalEvidenceRegion{StartMS: 0, EndMS: end, Reason: "simultaneous_black_and_silence"}
			plan.EvidenceStartMS = end
		}
	}
	if overlap, ok := temporalEvidenceEdgeOverlap(black, silence, 0, durationMS, false); ok && overlap.EndMS-overlap.StartMS >= TemporalEvidenceMinCleanupMS {
		start := min(durationMS, overlap.StartMS+TemporalEvidenceTransitionPadMS)
		if start < durationMS {
			plan.TrailingCleanup = &TemporalEvidenceRegion{StartMS: start, EndMS: durationMS, Reason: "simultaneous_black_and_silence"}
			plan.EvidenceEndMS = start
		}
	}
	if plan.EvidenceEndMS <= plan.EvidenceStartMS {
		plan.EvidenceStartMS, plan.EvidenceEndMS = 0, durationMS
		plan.LeadingCleanup, plan.TrailingCleanup = nil, nil
	}
	plan.SceneCutsMS = temporalEvidenceUniqueInSpan(sceneCuts, plan.EvidenceStartMS, plan.EvidenceEndMS)
	plan.FrameTimesMS = temporalEvidenceFrameTimes(plan.EvidenceStartMS, plan.EvidenceEndMS, plan.SceneCutsMS)
	return plan, nil
}

func validateTemporalEvidenceIntervals(durationMS int64, intervals []mediatools.Interval, kind string) error {
	for _, interval := range intervals {
		if interval.StartMs < 0 || interval.EndMs <= interval.StartMs || interval.EndMs > durationMS {
			return fmt.Errorf("%s interval %d..%d is outside 0..%d", kind, interval.StartMs, interval.EndMs, durationMS)
		}
	}
	return nil
}

func temporalEvidenceEdgeOverlap(black, silence []mediatools.Interval, start, end int64, leading bool) (TemporalEvidenceRegion, bool) {
	for _, b := range black {
		for _, s := range silence {
			overlap := TemporalEvidenceRegion{StartMS: max(b.StartMs, s.StartMs), EndMS: min(b.EndMs, s.EndMs)}
			if overlap.EndMS <= overlap.StartMS {
				continue
			}
			if leading && overlap.StartMS <= start || !leading && overlap.EndMS >= end {
				return overlap, true
			}
		}
	}
	return TemporalEvidenceRegion{}, false
}

func temporalEvidenceUniqueInSpan(values []int64, start, end int64) []int64 {
	seen := map[int64]struct{}{}
	var result []int64
	for _, value := range values {
		if value < start || value >= end {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func temporalEvidenceFrameTimes(start, end int64, cuts []int64) []int64 {
	span := end - start
	last, closingLead := end-1, start
	if span > 1_000 {
		last, closingLead = end-1_000, max(start, end-2_000)
	}
	anchors := []int64{start, min(last, start+500), start + span/3, start + span*2/3, closingLead, last}
	for _, cut := range temporalEvidenceRepresentativeCuts(cuts, temporalEvidenceSceneCutLimit) {
		anchors = append(anchors, max(start, cut-TemporalEvidenceCutContextMS), min(last, cut+TemporalEvidenceCutContextMS))
	}
	ordered := temporalEvidenceUniqueInSpan(anchors, start, end)
	return ordered[:min(TemporalEvidenceMaxFrames, len(ordered))]
}

func temporalEvidenceRepresentativeCuts(cuts []int64, limit int) []int64 {
	if len(cuts) <= limit {
		return slices.Clone(cuts)
	}
	if limit <= 0 {
		return nil
	}
	if limit == 1 {
		return []int64{cuts[len(cuts)/2]}
	}
	result := make([]int64, 0, limit)
	for index := range limit {
		result = append(result, cuts[index*(len(cuts)-1)/(limit-1)])
	}
	return result
}
