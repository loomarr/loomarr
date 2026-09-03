package fillerreview

import (
	"fmt"
	"sort"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func constructTemporalStructureHoldout(config TemporalStructureHoldoutConfig, loaded temporalStructureHoldoutLoaded, anchors []temporalStructureHoldoutSelectedAnchor, parents []TemporalStructureChallengeSource) (TemporalStructureChallengeAuthoring, TemporalStructureHoldoutReceipt, error) {
	if len(anchors) != temporalStructureHoldoutClassCases || len(parents) != temporalStructureHoldoutParentSources {
		return TemporalStructureChallengeAuthoring{}, TemporalStructureHoldoutReceipt{}, fmt.Errorf("temporal structure holdout selection counts are invalid")
	}
	authoring := TemporalStructureChallengeAuthoring{
		SchemaVersion: TemporalStructureChallengeSchemaVersion, ContractVersion: TemporalStructureChallengeContractVersion,
	}
	receipt := TemporalStructureHoldoutReceipt{
		SchemaVersion: TemporalStructureHoldoutSchemaVersion, ContractVersion: TemporalStructureHoldoutContractVersion,
		PlannedAt: config.PlannedAt.UTC(), SeedSHA256: hashBytes([]byte(config.Seed)), Inputs: loaded.inputs,
		Cases: TemporalStructureHoldoutCases, StandaloneCases: temporalStructureHoldoutClassCases,
		CompilationCases: temporalStructureHoldoutClassCases, ProgrammeExcerptCases: temporalStructureHoldoutClassCases,
		ProgrammeSpotCases: temporalStructureHoldoutClassCases, MultiCompilationCases: temporalStructureHoldoutClassCases,
		IndependentSources: temporalStructureHoldoutClassCases, ProgrammeParents: temporalStructureHoldoutParentSources,
		StandaloneRoleCounts: map[fillereval.TemporalRole]int{}, TrainingAllowed: false, ProductionAdmissionAllowed: false,
	}
	for index := range anchors {
		relative, err := temporalStructureHoldoutRelativeEvidencePath(config.SourceRoot, config.EvidenceManifestPath, anchors[index].source.Path)
		if err != nil {
			return TemporalStructureChallengeAuthoring{}, TemporalStructureHoldoutReceipt{}, err
		}
		anchors[index].source.Path = relative
		authoring.Sources = append(authoring.Sources, anchors[index].source)
		receipt.SelectedAnchors = append(receipt.SelectedAnchors, anchors[index].receipt)
		receipt.StandaloneRoleCounts[anchors[index].receipt.Role]++
		authoring.Cases = append(authoring.Cases, TemporalStructureChallengeCase{
			ID:   temporalStructureHoldoutCaseID(config.Seed, "standalone", anchors[index].source.ID),
			Unit: fillereval.UnitStandalone, Role: anchors[index].receipt.Role,
			Segments: []TemporalStructureChallengeSegment{{SourceID: anchors[index].source.ID, DurationMS: anchors[index].source.DurationMS}},
		})
	}
	authoring.Sources = append(authoring.Sources, parents...)
	compilationCases, compilationReceipt, err := constructTemporalStructureHoldoutCompilations(config.Seed, anchors)
	if err != nil {
		return TemporalStructureChallengeAuthoring{}, TemporalStructureHoldoutReceipt{}, err
	}
	authoring.Cases = append(authoring.Cases, compilationCases...)
	receipt.CompilationConstructions = compilationReceipt
	multiCompilationCases, multiCompilationReceipt, err := constructTemporalStructureHoldoutMultiCompilations(config.Seed, anchors)
	if err != nil {
		return TemporalStructureChallengeAuthoring{}, TemporalStructureHoldoutReceipt{}, err
	}
	authoring.Cases = append(authoring.Cases, multiCompilationCases...)
	receipt.MultiCompilationConstructions = multiCompilationReceipt
	programmeCases, programmeReceipt := constructTemporalStructureHoldoutProgrammeCuts(config.Seed, parents)
	authoring.Cases = append(authoring.Cases, programmeCases...)
	receipt.ProgrammeConstructions = programmeReceipt
	programmeSpotCases, programmeSpotReceipt := constructTemporalStructureHoldoutProgrammeSpots(config.Seed, parents, anchors)
	authoring.Cases = append(authoring.Cases, programmeSpotCases...)
	receipt.ProgrammeSpotConstructions = programmeSpotReceipt
	sort.Slice(authoring.Sources, func(i, j int) bool { return authoring.Sources[i].ID < authoring.Sources[j].ID })
	sort.Slice(authoring.Cases, func(i, j int) bool { return authoring.Cases[i].ID < authoring.Cases[j].ID })
	sort.Slice(receipt.SelectedAnchors, func(i, j int) bool { return receipt.SelectedAnchors[i].SourceID < receipt.SelectedAnchors[j].SourceID })
	sort.Slice(receipt.CompilationConstructions, func(i, j int) bool {
		return receipt.CompilationConstructions[i].CaseID < receipt.CompilationConstructions[j].CaseID
	})
	sort.Slice(receipt.MultiCompilationConstructions, func(i, j int) bool {
		return receipt.MultiCompilationConstructions[i].CaseID < receipt.MultiCompilationConstructions[j].CaseID
	})
	sort.Slice(receipt.ProgrammeConstructions, func(i, j int) bool {
		return receipt.ProgrammeConstructions[i].CaseID < receipt.ProgrammeConstructions[j].CaseID
	})
	sort.Slice(receipt.ProgrammeSpotConstructions, func(i, j int) bool {
		return receipt.ProgrammeSpotConstructions[i].CaseID < receipt.ProgrammeSpotConstructions[j].CaseID
	})
	prepared, err := prepareTemporalStructureChallenge(TemporalStructureChallengeConfig{SourceRoot: config.SourceRoot, Seed: config.Seed}, authoring)
	if err != nil {
		return TemporalStructureChallengeAuthoring{}, TemporalStructureHoldoutReceipt{}, err
	}
	if len(prepared) != TemporalStructureHoldoutCases {
		return TemporalStructureChallengeAuthoring{}, TemporalStructureHoldoutReceipt{}, fmt.Errorf("temporal structure holdout authoring did not produce %d unique blinded cases", TemporalStructureHoldoutCases)
	}
	return authoring, receipt, nil
}

func constructTemporalStructureHoldoutProgrammeSpots(seed string, parents []TemporalStructureChallengeSource, anchors []temporalStructureHoldoutSelectedAnchor) ([]TemporalStructureChallengeCase, []TemporalStructureHoldoutProgrammeSpot) {
	const programmePartMS int64 = 20_000
	var cases []TemporalStructureChallengeCase
	var receipts []TemporalStructureHoldoutProgrammeSpot
	anchorIndex := 0
	for _, parent := range parents {
		patterns := []struct {
			name        string
			beforeStart int64
			afterStart  int64
		}{
			{name: "early_insert", beforeStart: 10_000, afterStart: 30_000},
			{name: "late_insert", beforeStart: parent.DurationMS - 50_000, afterStart: parent.DurationMS - 30_000},
		}
		for _, pattern := range patterns {
			anchor := anchors[anchorIndex]
			anchorIndex++
			caseID := temporalStructureHoldoutCaseID(seed, "programme_with_spots", fmt.Sprintf("%s\x00%s\x00%s", parent.ID, anchor.source.ID, pattern.name))
			cases = append(cases, TemporalStructureChallengeCase{
				ID: caseID, Unit: fillereval.UnitProgrammeSpots,
				Segments: []TemporalStructureChallengeSegment{
					{SourceID: parent.ID, StartMS: pattern.beforeStart, DurationMS: programmePartMS},
					{SourceID: anchor.source.ID, DurationMS: anchor.source.DurationMS},
					{SourceID: parent.ID, StartMS: pattern.afterStart, DurationMS: programmePartMS},
				},
			})
			receipts = append(receipts, TemporalStructureHoldoutProgrammeSpot{
				CaseID: caseID, ParentSourceID: parent.ID, FillerSourceID: anchor.source.ID, Pattern: pattern.name,
				ParentDurationMS: parent.DurationMS, BeforeSourceStartMS: pattern.beforeStart, AfterSourceStartMS: pattern.afterStart,
				ProgrammePartMS: programmePartMS, FillerDurationMS: anchor.source.DurationMS, FillerRole: anchor.receipt.Role,
			})
		}
	}
	return cases, receipts
}

func constructTemporalStructureHoldoutCompilations(seed string, selected []temporalStructureHoldoutSelectedAnchor) ([]TemporalStructureChallengeCase, []TemporalStructureHoldoutCompilation, error) {
	pairs, err := selectTemporalStructureHoldoutCompilationPairs(seed, selected)
	if err != nil {
		return nil, nil, err
	}
	cases := make([]TemporalStructureChallengeCase, 0, len(pairs))
	receipts := make([]TemporalStructureHoldoutCompilation, 0, len(pairs))
	for _, item := range pairs {
		first, second := selected[item.first], selected[item.second]
		durationMS := first.source.DurationMS + second.source.DurationMS
		caseID := temporalStructureHoldoutCaseID(seed, "compilation", first.source.ID+"\x00"+second.source.ID)
		cases = append(cases, TemporalStructureChallengeCase{
			ID: caseID, Unit: fillereval.UnitCompilation,
			Segments: []TemporalStructureChallengeSegment{
				{SourceID: first.source.ID, DurationMS: first.source.DurationMS},
				{SourceID: second.source.ID, DurationMS: second.source.DurationMS},
			},
		})
		receipts = append(receipts, TemporalStructureHoldoutCompilation{
			CaseID: caseID, FirstSourceID: first.source.ID, SecondSourceID: second.source.ID,
			JoinBand: item.band, JoinAtMS: first.source.DurationMS, DurationMS: durationMS,
			Roles: []string{string(first.receipt.Role), string(second.receipt.Role)},
		})
	}
	return cases, receipts, nil
}

type temporalStructureHoldoutCompilationPair struct {
	first  int
	second int
	band   string
	rank   string
}

func selectTemporalStructureHoldoutCompilationPairs(seed string, anchors []temporalStructureHoldoutSelectedAnchor) ([]temporalStructureHoldoutCompilationPair, error) {
	var result []temporalStructureHoldoutCompilationPair
	for _, band := range []string{"early", "middle", "late"} {
		var candidates []temporalStructureHoldoutCompilationPair
		for first := range anchors {
			for second := range anchors {
				if first == second || temporalStructureHoldoutJoinBand(anchors[first].source.DurationMS, anchors[first].source.DurationMS+anchors[second].source.DurationMS) != band {
					continue
				}
				identity := anchors[first].source.ID + "\x00" + anchors[second].source.ID
				candidates = append(candidates, temporalStructureHoldoutCompilationPair{
					first: first, second: second, band: band,
					rank: hashBytes([]byte(seed + "\x00compilation-pair\x00" + band + "\x00" + identity)),
				})
			}
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].rank < candidates[j].rank })
		chosen, ok := chooseDisjointTemporalStructureHoldoutPairs(candidates, anchors, nil, map[string]struct{}{}, 4)
		if !ok {
			return nil, fmt.Errorf("temporal structure holdout anchor durations cannot supply four source-disjoint %s joins", band)
		}
		result = append(result, chosen...)
	}
	return result, nil
}

func chooseDisjointTemporalStructureHoldoutPairs(candidates []temporalStructureHoldoutCompilationPair, anchors []temporalStructureHoldoutSelectedAnchor, chosen []temporalStructureHoldoutCompilationPair, used map[string]struct{}, want int) ([]temporalStructureHoldoutCompilationPair, bool) {
	if len(chosen) == want {
		return chosen, true
	}
	for index, candidate := range candidates {
		firstID, secondID := anchors[candidate.first].source.ID, anchors[candidate.second].source.ID
		if _, exists := used[firstID]; exists {
			continue
		}
		if _, exists := used[secondID]; exists {
			continue
		}
		used[firstID], used[secondID] = struct{}{}, struct{}{}
		if result, ok := chooseDisjointTemporalStructureHoldoutPairs(candidates[index+1:], anchors, append(chosen, candidate), used, want); ok {
			return result, true
		}
		delete(used, firstID)
		delete(used, secondID)
	}
	return nil, false
}

func temporalStructureHoldoutJoinBand(joinAtMS, durationMS int64) string {
	switch {
	case joinAtMS*5 <= durationMS*2:
		return "early"
	case joinAtMS*5 >= durationMS*3:
		return "late"
	default:
		return "middle"
	}
}

func constructTemporalStructureHoldoutProgrammeCuts(seed string, parents []TemporalStructureChallengeSource) ([]TemporalStructureChallengeCase, []TemporalStructureHoldoutProgrammeCut) {
	var cases []TemporalStructureChallengeCase
	var receipts []TemporalStructureHoldoutProgrammeCut
	for _, parent := range parents {
		cuts := []struct {
			pattern    string
			start      int64
			durationMS int64
		}{
			{pattern: "near_parent_start", start: 10_000, durationMS: 30_000},
			{pattern: "near_parent_end", start: parent.DurationMS - 55_000, durationMS: 45_000},
		}
		for _, cut := range cuts {
			caseID := temporalStructureHoldoutCaseID(seed, "programme_excerpt", fmt.Sprintf("%s\x00%s\x00%d\x00%d", parent.ID, cut.pattern, cut.start, cut.durationMS))
			cases = append(cases, TemporalStructureChallengeCase{
				ID: caseID, Unit: fillereval.UnitProgrammeExcerpt,
				Segments: []TemporalStructureChallengeSegment{{SourceID: parent.ID, StartMS: cut.start, DurationMS: cut.durationMS}},
			})
			receipts = append(receipts, TemporalStructureHoldoutProgrammeCut{
				CaseID: caseID, SourceID: parent.ID, Pattern: cut.pattern, StartMS: cut.start,
				DurationMS: cut.durationMS, ParentEndMS: parent.DurationMS,
			})
		}
	}
	return cases, receipts
}

func temporalStructureHoldoutCaseID(seed, kind, identity string) string {
	return "holdout-" + hashBytes([]byte(seed + "\x00" + kind + "\x00" + identity))[:24]
}
