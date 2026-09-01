package fillerreference

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"math/bits"
	"slices"
	"strings"
	"time"
)

const (
	// DuplicateAlgorithm versions the decode cadence, center crop, dHash layout,
	// and ordered-sequence comparison used by filler-reference-families.
	DuplicateAlgorithm = "duplicate-v3-visual-dhash-2fps-fixedoffset-or-audio-rms100ms-corr90"

	duplicateFrameDistance = 12
	duplicateMinFrames     = 12
	duplicateMinCoverage   = 0.70
	duplicateAudioMinBins  = 50
	duplicateAudioCorr     = 0.90
)

// DuplicateComparison is evidence that two captures contain the same moving-
// image unit. Related is deliberately conservative: callers must not collapse
// a pair based on a title, filename, or a handful of matching black frames.
type DuplicateComparison struct {
	Related         bool    `json:"related"`
	MatchedFrames   int     `json:"matchedFrames"`
	ComparedFramesA int     `json:"comparedFramesA"`
	ComparedFramesB int     `json:"comparedFramesB"`
	Coverage        float64 `json:"coverage"`
	MeanDistance    float64 `json:"meanDistance,omitempty"`
	MaximumDistance int     `json:"maximumDistance,omitempty"`
}

type FamilyFingerprint struct {
	CaseID        string   `json:"caseId"`
	ContentSHA256 string   `json:"contentSha256"`
	LocalFile     string   `json:"localFile"`
	FrameHashes   []uint64 `json:"frameHashes"`
	AudioRMS      []uint32 `json:"audioRms100ms"`
}

type FamilyPair struct {
	CaseA      string              `json:"caseA"`
	CaseB      string              `json:"caseB"`
	Basis      []string            `json:"basis,omitempty"`
	Comparison DuplicateComparison `json:"visualComparison"`
	Audio      AudioComparison     `json:"audioComparison"`
}

type AudioComparison struct {
	Related      bool    `json:"related"`
	Correlation  float64 `json:"correlation,omitempty"`
	ComparedBins int     `json:"comparedBins"`
	Coverage     float64 `json:"coverage"`
	OffsetBins   int     `json:"offsetBins"`
}

type DuplicateFamily struct {
	FamilyID       string   `json:"familyId"`
	Members        []string `json:"members"`
	CompleteClique bool     `json:"completeClique"`
	PreferredCase  string   `json:"preferredCase,omitempty"`
}

type FamilySummary struct {
	Cases             int `json:"cases"`
	RelatedPairs      int `json:"relatedPairs"`
	ClosestNonMatches int `json:"closestNonMatches"`
	DuplicateFamilies int `json:"duplicateFamilies"`
	NonCliqueFamilies int `json:"nonCliqueFamilies"`
}

type FamilyAudit struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Algorithm     string              `json:"algorithm"`
	GeneratedAt   time.Time           `json:"generatedAt"`
	SourceAudit   string              `json:"sourceAuditSha256"`
	Summary       FamilySummary       `json:"summary"`
	Fingerprints  []FamilyFingerprint `json:"fingerprints"`
	Pairs         []FamilyPair        `json:"relatedPairs"`
	ClosestPairs  []FamilyPair        `json:"closestNonMatches"`
	Families      []DuplicateFamily   `json:"families"`
}

// BuildFamilyAudit compares already content-verified fingerprints. It reports
// connected components but leaves PreferredCase empty: choosing a rendition
// requires full playback and cannot be inferred from resolution or filenames.
func BuildFamilyAudit(sourceAuditSHA256 string, fingerprints []FamilyFingerprint, generatedAt time.Time) FamilyAudit {
	fingerprints = slices.Clone(fingerprints)
	slices.SortFunc(fingerprints, func(a, b FamilyFingerprint) int { return strings.Compare(a.CaseID, b.CaseID) })
	parents := make([]int, len(fingerprints))
	for i := range parents {
		parents[i] = i
	}
	var find func(int) int
	find = func(index int) int {
		if parents[index] != index {
			parents[index] = find(parents[index])
		}
		return parents[index]
	}
	var pairs, nonMatches []FamilyPair
	pairSet := map[string]struct{}{}
	for i := range fingerprints {
		for j := i + 1; j < len(fingerprints); j++ {
			visual := CompareDuplicateSequences(fingerprints[i].FrameHashes, fingerprints[j].FrameHashes)
			audio := CompareAudioEnvelopes(fingerprints[i].AudioRMS, fingerprints[j].AudioRMS)
			pair := FamilyPair{CaseA: fingerprints[i].CaseID, CaseB: fingerprints[j].CaseID, Comparison: visual, Audio: audio}
			if visual.Related {
				pair.Basis = append(pair.Basis, "visual")
			}
			if audio.Related {
				pair.Basis = append(pair.Basis, "audio")
			}
			if len(pair.Basis) == 0 {
				nonMatches = append(nonMatches, pair)
				continue
			}
			pairs = append(pairs, pair)
			pairSet[fingerprints[i].CaseID+"\x00"+fingerprints[j].CaseID] = struct{}{}
			left, right := find(i), find(j)
			if left != right {
				parents[right] = left
			}
		}
	}
	slices.SortFunc(nonMatches, func(a, b FamilyPair) int {
		if a.Audio.Correlation > b.Audio.Correlation {
			return -1
		}
		if a.Audio.Correlation < b.Audio.Correlation {
			return 1
		}
		if a.Comparison.Coverage > b.Comparison.Coverage {
			return -1
		}
		if a.Comparison.Coverage < b.Comparison.Coverage {
			return 1
		}
		if a.Comparison.MeanDistance < b.Comparison.MeanDistance {
			return -1
		}
		if a.Comparison.MeanDistance > b.Comparison.MeanDistance {
			return 1
		}
		return strings.Compare(a.CaseA+"\x00"+a.CaseB, b.CaseA+"\x00"+b.CaseB)
	})
	if len(nonMatches) > 20 {
		nonMatches = nonMatches[:20]
	}
	components := map[int][]string{}
	for i, fingerprint := range fingerprints {
		components[find(i)] = append(components[find(i)], fingerprint.CaseID)
	}
	var families []DuplicateFamily
	nonClique := 0
	for _, members := range components {
		if len(members) < 2 {
			continue
		}
		slices.Sort(members)
		clique := true
		for i := range members {
			for j := i + 1; j < len(members); j++ {
				if _, ok := pairSet[members[i]+"\x00"+members[j]]; !ok {
					clique = false
				}
			}
		}
		if !clique {
			nonClique++
		}
		digest := sha256.Sum256([]byte(strings.Join(members, "\n")))
		families = append(families, DuplicateFamily{FamilyID: "duplicate-family-" + hex.EncodeToString(digest[:12]), Members: members, CompleteClique: clique})
	}
	slices.SortFunc(families, func(a, b DuplicateFamily) int { return strings.Compare(a.FamilyID, b.FamilyID) })
	return FamilyAudit{
		SchemaVersion: 3, Algorithm: DuplicateAlgorithm, GeneratedAt: generatedAt.UTC(), SourceAudit: sourceAuditSHA256,
		Summary:      FamilySummary{Cases: len(fingerprints), RelatedPairs: len(pairs), ClosestNonMatches: len(nonMatches), DuplicateFamilies: len(families), NonCliqueFamilies: nonClique},
		Fingerprints: fingerprints, Pairs: pairs, ClosestPairs: nonMatches, Families: families,
	}
}

// CompareAudioEnvelopes aligns 100ms RMS bins at one fixed offset and reports
// normalized correlation. Constant/near-silent tracks have no useful variance
// and cannot establish a duplicate relationship.
func CompareAudioEnvelopes(a, b []uint32) AudioComparison {
	result := AudioComparison{}
	if len(a) < duplicateAudioMinBins || len(b) < duplicateAudioMinBins {
		return result
	}
	minimum := min(len(a), len(b))
	minimumOverlap := int(float64(minimum) * duplicateMinCoverage)
	best := -2.0
	for offset := -len(a) + minimumOverlap; offset <= len(b)-minimumOverlap; offset++ {
		start := max(0, -offset)
		end := min(len(a), len(b)-offset)
		n := end - start
		if n < minimumOverlap {
			continue
		}
		var sumA, sumB float64
		for i := start; i < end; i++ {
			sumA += float64(a[i])
			sumB += float64(b[i+offset])
		}
		meanA, meanB := sumA/float64(n), sumB/float64(n)
		var numerator, varianceA, varianceB float64
		for i := start; i < end; i++ {
			deltaA := float64(a[i]) - meanA
			deltaB := float64(b[i+offset]) - meanB
			numerator += deltaA * deltaB
			varianceA += deltaA * deltaA
			varianceB += deltaB * deltaB
		}
		if varianceA == 0 || varianceB == 0 {
			continue
		}
		correlation := numerator / (math.Sqrt(varianceA) * math.Sqrt(varianceB))
		if correlation > best {
			best = correlation
			result.Correlation = correlation
			result.ComparedBins = n
			result.Coverage = float64(n) / float64(minimum)
			result.OffsetBins = offset
		}
	}
	result.Related = result.ComparedBins >= duplicateAudioMinBins && result.Correlation >= duplicateAudioCorr
	return result
}

// CompareDuplicateSequences performs an order-preserving match over useful
// dHash frames. Unlike the retired four-relative-frame comparison, it tolerates
// short leaders, trailers, and sample-phase shifts. A match must use one fixed
// time offset: it cannot collect convenient frames scattered across a long
// compilation. Near-flat frames do not count because black and white cards are
// common across unrelated adverts, but they retain their timeline position.
func CompareDuplicateSequences(a, b []uint64) DuplicateComparison {
	usefulA := informativeCount(a)
	usefulB := informativeCount(b)
	result := DuplicateComparison{ComparedFramesA: usefulA, ComparedFramesB: usefulB}
	if usefulA < duplicateMinFrames || usefulB < duplicateMinFrames {
		return result
	}
	type alignment struct{ matches, distance, maximum int }
	best := alignment{}
	// offset is b's frame index minus a's. Every comparison along one
	// diagonal therefore represents one continuous temporal alignment.
	for offset := -len(a) + 1; offset < len(b); offset++ {
		candidate := alignment{}
		for i := max(0, -offset); i < len(a) && i+offset < len(b); i++ {
			j := i + offset
			if !informativeHash(a[i]) || !informativeHash(b[j]) {
				continue
			}
			distance := bits.OnesCount64(a[i] ^ b[j])
			if distance <= duplicateFrameDistance {
				candidate.matches++
				candidate.distance += distance
				candidate.maximum = max(candidate.maximum, distance)
			}
		}
		if candidate.matches > best.matches || (candidate.matches == best.matches && candidate.distance < best.distance) {
			best = candidate
		}
	}
	result.MatchedFrames = best.matches
	result.Coverage = float64(best.matches) / float64(min(usefulA, usefulB))
	if best.matches > 0 {
		result.MeanDistance = float64(best.distance) / float64(best.matches)
		result.MaximumDistance = best.maximum
	}
	result.Related = result.Coverage >= duplicateMinCoverage
	return result
}

func informativeCount(in []uint64) int {
	count := 0
	for _, hash := range in {
		if informativeHash(hash) {
			count++
		}
	}
	return count
}

func informativeHash(hash uint64) bool {
	ones := bits.OnesCount64(hash)
	return ones >= 4 && ones <= 60
}
