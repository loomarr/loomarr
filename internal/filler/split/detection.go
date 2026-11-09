package split

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/media/tools"
)

const legacySplitObservationProducer = "v34-split-shadow-v1"

// AssessDetection projects the V34 detector's durable facts into the V67 contract.
// It deliberately supplies no role claims, so this shadow assessment cannot authorize publication.
func AssessDetection(source SourceAsset, progress DetectionProgress, segments []Segment, assessedAt time.Time) (Assessment, error) {
	observations := splitStructureObservations(source, progress, segments)
	discards, err := splitStructureDiscards(progress.Discarded, observations)
	if err != nil {
		return Assessment{}, err
	}
	return Assess(Input{
		Source: source, Observations: observations, DiscardClaims: discards, AssessedAt: assessedAt,
	})
}

// ChapterEdges returns every distinct declared chapter bound in order, including the bounds of a
// chapter too short to become a segment.
func ChapterEdges(chapters []tools.Chapter) []int64 {
	var edges []int64
	for _, chapter := range chapters {
		edges = append(edges, chapter.StartMs, chapter.EndMs)
	}
	slices.Sort(edges)
	return slices.Compact(edges)
}

func splitStructureObservations(source SourceAsset, progress DetectionProgress, segments []Segment) []Observation {
	var observations []Observation
	appendObservation := func(kind ObservationKind, effect ObservationEffect, startMs, endMs int64, evidence string) {
		if startMs <= 0 || endMs >= source.DurationMs {
			return
		}
		observations = append(observations, Observation{
			Kind: kind, Effect: effect, StartMs: startMs, EndMs: endMs,
			Producer:       legacySplitObservationProducer,
			EvidenceSHA256: structureDerivedEvidenceSHA256(source.SHA256, string(kind), strconv.FormatInt(startMs, 10), strconv.FormatInt(endMs, 10), evidence),
		})
	}
	for _, edge := range progress.ChapterEdges {
		appendObservation(ObservationChapterEdge, ObservationProposesBoundary, edge, edge, "declared-container-edge")
	}
	for _, interval := range progress.Black {
		appendObservation(ObservationBlackInterval, ObservationProposesBoundary, interval.StartMs, interval.EndMs, "ffmpeg-blackdetect")
	}
	for _, interval := range progress.Silence {
		appendObservation(ObservationSilenceInterval, ObservationProposesBoundary, interval.StartMs, interval.EndMs, "ffmpeg-silencedetect")
	}
	seenTranscriptEdges := map[int64]struct{}{}
	for _, segment := range segments {
		for _, edge := range []struct {
			at  int64
			src BoundarySource
		}{{segment.StartMs, segment.StartSrc}, {segment.EndMs, segment.EndSrc}} {
			if edge.src&SrcTranscript == 0 || edge.at <= 0 || edge.at >= source.DurationMs {
				continue
			}
			if _, exists := seenTranscriptEdges[edge.at]; exists {
				continue
			}
			seenTranscriptEdges[edge.at] = struct{}{}
			appendObservation(ObservationTranscriptChange, ObservationProposesBoundary, edge.at, edge.at, segment.Transcript)
		}
	}
	sort.Slice(observations, func(i, j int) bool {
		if observations[i].StartMs != observations[j].StartMs {
			return observations[i].StartMs < observations[j].StartMs
		}
		if observations[i].Kind != observations[j].Kind {
			return observations[i].Kind < observations[j].Kind
		}
		return observations[i].EvidenceSHA256 < observations[j].EvidenceSHA256
	})
	for i := range observations {
		observations[i].ID = fmt.Sprintf("obs-%04d", i+1)
	}
	return observations
}

func splitStructureDiscards(discarded []tools.Interval, observations []Observation) ([]DiscardClaim, error) {
	if len(discarded) == 0 {
		return nil, nil
	}
	var claims []DiscardClaim
	for _, interval := range discarded {
		ids := structureEvidenceAtEdges(interval, observations)
		if len(ids) == 0 {
			return nil, fmt.Errorf("discarded interval %d..%d has no retained boundary evidence", interval.StartMs, interval.EndMs)
		}
		claims = append(claims, DiscardClaim{
			StartMs: interval.StartMs, EndMs: interval.EndMs,
			Reason: DiscardBelowClipFloor, EvidenceIDs: ids,
		})
	}
	return claims, nil
}

func structureEvidenceAtEdges(interval tools.Interval, observations []Observation) []string {
	var ids []string
	for _, observation := range observations {
		midpoint := (observation.StartMs + observation.EndMs) / 2
		if midpoint == interval.StartMs || midpoint == interval.EndMs || observation.StartMs <= interval.StartMs && interval.StartMs <= observation.EndMs || observation.StartMs <= interval.EndMs && interval.EndMs <= observation.EndMs {
			ids = append(ids, observation.ID)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

func structureDerivedEvidenceSHA256(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
