package fillerreview

import (
	"fmt"
	"os"
	"reflect"
	"sort"

	"github.com/loomarr/loomarr/internal/fillercandidatepool"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/mediatools"
)

func loadTemporalStructureReplacement(config TemporalStructureHoldoutConfig) (temporalStructureHoldoutLoaded, error) {
	raw, err := os.ReadFile(config.CandidatePoolPath)
	if err != nil {
		return temporalStructureHoldoutLoaded{}, fmt.Errorf("read replacement candidate pool: %w", err)
	}
	pool, err := fillercandidatepool.Decode(raw)
	if err != nil {
		return temporalStructureHoldoutLoaded{}, err
	}
	if pool.GeneratedAt.After(config.PlannedAt) {
		return temporalStructureHoldoutLoaded{}, fmt.Errorf("replacement candidate pool postdates planning")
	}
	prior, err := loadTemporalStructureHoldoutPrior(config)
	if err != nil {
		return temporalStructureHoldoutLoaded{}, err
	}
	if !reflect.DeepEqual(pool.PriorExposure, candidatePoolExposure(prior.exposure)) {
		return temporalStructureHoldoutLoaded{}, fmt.Errorf("replacement candidate pool does not bind the exact prior exposure")
	}
	loaded := temporalStructureHoldoutLoaded{
		prior: prior, sourcePathsRelative: true,
		inputs: []TemporalStructureHoldoutInput{{Name: "replacement_candidate_pool", SHA256: fillercandidatepool.Digest(raw)}},
	}
	loaded.inputs = append(loaded.inputs, prior.inputs...)
	sort.Slice(loaded.inputs, func(i, j int) bool { return loaded.inputs[i].Name < loaded.inputs[j].Name })
	priorSources := stringSet(prior.exposure.SourceSHA256)
	priorFamilies := stringSet(prior.exposure.FamilyIDs)
	priorProgrammes := make(map[string]struct{}, len(prior.exposure.ProgrammeProvenance))
	for _, provenance := range prior.exposure.ProgrammeProvenance {
		priorProgrammes[temporalStructureProgrammeProvenanceKey(provenance)] = struct{}{}
	}
	for _, candidate := range pool.Candidates {
		if candidate.Disposition != fillercandidatepool.DispositionEligible {
			continue
		}
		if _, exposed := priorSources[candidate.Source.SHA256]; exposed {
			return temporalStructureHoldoutLoaded{}, fmt.Errorf("eligible replacement candidate %q repeats prior source bytes", candidate.CaseID)
		}
		if _, exposed := priorFamilies[candidate.FamilyID]; exposed {
			return temporalStructureHoldoutLoaded{}, fmt.Errorf("eligible replacement candidate %q repeats a prior family", candidate.CaseID)
		}
		source := temporalStructureCandidatePoolSource(candidate)
		switch candidate.Kind {
		case fillercandidatepool.KindStandaloneAnchor:
			transition := temporalStructureCandidatePoolTransition(candidate)
			loaded.replacementAnchors = append(loaded.replacementAnchors, temporalStructureHoldoutSelectedAnchor{
				receipt: TemporalStructureHoldoutAnchor{
					EvidenceAlias: transition.EvidenceAlias, CaseID: candidate.CaseID, SourceID: source.ID,
					FamilyID: candidate.FamilyID, Role: fillereval.TemporalRole(candidate.Role), DurationMS: source.DurationMS,
				},
				source: source, transition: transition,
			})
		case fillercandidatepool.KindProgrammeParent:
			key := temporalStructureProgrammeProvenanceKey(TemporalStructureHoldoutProgrammeProvenance{Authority: source.Provenance.Authority, Reference: source.Provenance.Reference})
			if _, exposed := priorProgrammes[key]; exposed {
				return temporalStructureHoldoutLoaded{}, fmt.Errorf("eligible replacement candidate %q repeats prior programme provenance", candidate.CaseID)
			}
			loaded.replacementParents = append(loaded.replacementParents, source)
		}
	}
	return loaded, nil
}

func selectTemporalStructureReplacementAnchors(seed string, loaded temporalStructureHoldoutLoaded) ([]temporalStructureHoldoutSelectedAnchor, error) {
	byRole := make(map[fillereval.TemporalRole]int)
	for index := range loaded.replacementAnchors {
		item := &loaded.replacementAnchors[index]
		item.receipt.RankSHA256 = hashBytes([]byte(seed + "\x00anchor\x00" + item.receipt.CaseID))
		byRole[item.receipt.Role]++
	}
	for role, want := range temporalStructureHoldoutRoleQuotas {
		if byRole[role] < want {
			return nil, fmt.Errorf("replacement candidate pool has insufficient eligible %s anchors", role)
		}
	}
	selected, ok := solveTemporalStructureHoldoutAnchors(seed, loaded.replacementAnchors)
	if !ok {
		return nil, fmt.Errorf("replacement candidate pool cannot jointly satisfy role, family, source, timing, role-pair, and transition quotas")
	}
	return selected, nil
}

func selectTemporalStructureReplacementParents(seed string, loaded temporalStructureHoldoutLoaded) ([]TemporalStructureChallengeSource, error) {
	parents := append([]TemporalStructureChallengeSource(nil), loaded.replacementParents...)
	sort.Slice(parents, func(i, j int) bool {
		left := hashBytes([]byte(seed + "\x00programme-parent\x00" + parents[i].ID))
		right := hashBytes([]byte(seed + "\x00programme-parent\x00" + parents[j].ID))
		return left < right || left == right && parents[i].ID < parents[j].ID
	})
	if len(parents) < temporalStructureHoldoutParentSources {
		return nil, fmt.Errorf("replacement candidate pool needs six eligible programme parents")
	}
	return parents[:temporalStructureHoldoutParentSources], nil
}

func temporalStructureCandidatePoolSource(candidate fillercandidatepool.Candidate) TemporalStructureChallengeSource {
	kind := TemporalStructureSourceBoundedItem
	if candidate.Kind == fillercandidatepool.KindProgrammeParent {
		kind = TemporalStructureSourceProgrammeParent
	}
	return TemporalStructureChallengeSource{
		ID: candidate.Source.ID, Path: candidate.Source.Path, SHA256: candidate.Source.SHA256,
		DurationMS: candidate.Source.DurationMS, StandaloneRole: fillereval.TemporalRole(candidate.Role),
		Provenance: TemporalStructureSourceProvenance{
			Kind: kind, Authority: candidate.Source.Authority, ItemID: candidate.Source.ItemID,
			Reference: candidate.Source.ItemURL, MetadataSHA256: candidate.Source.MetadataSHA256,
			RetrievedAt: candidate.Source.MetadataRetrievedAt,
		},
	}
}

func temporalStructureCandidatePoolTransition(candidate fillercandidatepool.Candidate) TemporalTransitionAuthorityCase {
	transition := candidate.Transition
	return TemporalTransitionAuthorityCase{
		EvidenceAlias: transition.EvidenceAlias, CaseID: candidate.CaseID, SourceSHA256: candidate.Source.SHA256,
		DurationMS: candidate.Source.DurationMS, Head: candidatePoolEdge(transition.Head), Tail: candidatePoolEdge(transition.Tail),
	}
}

func candidatePoolEdge(edge fillercandidatepool.Edge) TemporalTransitionEdge {
	return TemporalTransitionEdge{
		StartMS: edge.StartMS, EndMS: edge.EndMS, Black: candidatePoolIntervals(edge.Black),
		Silence: candidatePoolIntervals(edge.Silence), RMSMilliDBFS: edge.RMSMilliDBFS, PeakMilliDBFS: edge.PeakMilliDBFS,
	}
}

func candidatePoolIntervals(values []fillercandidatepool.Interval) []mediatools.Interval {
	result := make([]mediatools.Interval, 0, len(values))
	for _, value := range values {
		result = append(result, mediatools.Interval{StartMs: value.StartMS, EndMs: value.EndMS})
	}
	return result
}

func candidatePoolExposure(value TemporalStructureHoldoutTrainingExclusion) fillercandidatepool.Exposure {
	result := fillercandidatepool.Exposure{
		SourceSHA256: append([]string(nil), value.SourceSHA256...), FamilyIDs: append([]string(nil), value.FamilyIDs...),
		ProgrammeProvenance: make([]fillercandidatepool.ProgrammeProvenance, 0, len(value.ProgrammeProvenance)),
	}
	for _, provenance := range value.ProgrammeProvenance {
		result.ProgrammeProvenance = append(result.ProgrammeProvenance, fillercandidatepool.ProgrammeProvenance{
			Authority: provenance.Authority, Reference: provenance.Reference,
		})
	}
	return result
}
