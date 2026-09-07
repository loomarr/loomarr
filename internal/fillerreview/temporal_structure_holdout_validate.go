package fillerreview

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func validateTemporalStructureHoldoutReceipt(receipt TemporalStructureHoldoutReceipt, authoring TemporalStructureChallengeAuthoring, transition *TemporalTransitionAuthority) error {
	if receipt.SchemaVersion != TemporalStructureHoldoutSchemaVersion || receipt.ContractVersion != TemporalStructureHoldoutContractVersion || receipt.PlannedAt.IsZero() || !reviewSHA256(receipt.SeedSHA256) || !reviewSHA256(receipt.AuthoringSHA256) || receipt.Cases != TemporalStructureHoldoutCases || receipt.StandaloneCases != temporalStructureHoldoutClassCases || receipt.CompilationCases != temporalStructureHoldoutClassCases || receipt.MultiCompilationCases != temporalStructureHoldoutClassCases || receipt.ProgrammeExcerptCases != temporalStructureHoldoutClassCases || receipt.ProgrammeSpotCases != temporalStructureHoldoutClassCases || receipt.IndependentSources != temporalStructureHoldoutClassCases || receipt.ProgrammeParents != temporalStructureHoldoutParentSources || len(receipt.SelectedAnchors) != temporalStructureHoldoutClassCases || len(receipt.CompilationConstructions) != temporalStructureHoldoutClassCases || len(receipt.MultiCompilationConstructions) != temporalStructureHoldoutClassCases || len(receipt.ProgrammeConstructions) != temporalStructureHoldoutClassCases || len(receipt.ProgrammeSpotConstructions) != temporalStructureHoldoutClassCases || receipt.BlindHumanAuditRequired == nil || *receipt.BlindHumanAuditRequired || receipt.TrainingAllowed == nil || *receipt.TrainingAllowed || receipt.ProductionAdmissionAllowed == nil || *receipt.ProductionAdmissionAllowed || len(authoring.Cases) != receipt.Cases || len(authoring.Sources) != temporalStructureHoldoutClassCases+temporalStructureHoldoutParentSources {
		return fmt.Errorf("temporal structure holdout receipt counts or disposition are invalid")
	}
	if err := validateTemporalStructureHoldoutLineage(receipt); err != nil {
		return err
	}
	if err := validateTemporalStructureHoldoutInputs(receipt.Inputs, receipt.PlanKind); err != nil {
		return err
	}
	sources, cases, unitCounts, err := indexTemporalStructureHoldoutAuthoring(authoring)
	if err != nil {
		return err
	}
	if unitCounts[fillereval.UnitStandalone] != temporalStructureHoldoutClassCases || unitCounts[fillereval.UnitCompilation] != temporalStructureHoldoutClassCases*2 || unitCounts[fillereval.UnitProgrammeExcerpt] != temporalStructureHoldoutClassCases || unitCounts[fillereval.UnitProgrammeSpots] != temporalStructureHoldoutClassCases || len(unitCounts) != 4 {
		return fmt.Errorf("temporal structure holdout authoring is not balanced by unit")
	}
	anchors, err := validateTemporalStructureHoldoutAnchors(receipt, sources)
	if err != nil {
		return err
	}
	if err := validateTemporalStructureHoldoutTrainingExclusion(receipt, anchors, sources); err != nil {
		return err
	}
	if err := validateTemporalStructureHoldoutStandaloneCases(cases, anchors); err != nil {
		return err
	}
	if err := validateTemporalStructureHoldoutCompilations(receipt.CompilationConstructions, cases, anchors, transition); err != nil {
		return err
	}
	if err := validateTemporalStructureHoldoutMultiCompilations(receipt.MultiCompilationConstructions, cases, anchors); err != nil {
		return err
	}
	if err := validateTemporalStructureHoldoutProgrammeCuts(receipt.ProgrammeConstructions, cases, sources); err != nil {
		return err
	}
	return validateTemporalStructureHoldoutProgrammeSpots(receipt.ProgrammeSpotConstructions, cases, sources, anchors)
}

func validateTemporalStructureHoldoutTrainingExclusion(receipt TemporalStructureHoldoutReceipt, anchors map[string]TemporalStructureHoldoutAnchor, sources map[string]TemporalStructureChallengeSource) error {
	current := emptyTemporalStructureHoldoutExposure()
	for _, source := range sources {
		current.SourceSHA256 = append(current.SourceSHA256, source.SHA256)
		if source.Provenance.Kind == TemporalStructureSourceProgrammeParent {
			current.ProgrammeProvenance = append(current.ProgrammeProvenance, TemporalStructureHoldoutProgrammeProvenance{Authority: source.Provenance.Authority, Reference: source.Provenance.Reference})
		}
	}
	for _, anchor := range anchors {
		current.FamilyIDs = append(current.FamilyIDs, anchor.FamilyID)
	}
	sort.Strings(current.SourceSHA256)
	sort.Strings(current.FamilyIDs)
	sort.Slice(current.ProgrammeProvenance, func(i, j int) bool {
		return temporalStructureProgrammeProvenanceKey(current.ProgrammeProvenance[i]) < temporalStructureProgrammeProvenanceKey(current.ProgrammeProvenance[j])
	})
	if err := validateTemporalStructureHoldoutExposure(current); err != nil {
		return fmt.Errorf("temporal structure holdout current exposure: %w", err)
	}
	if !disjointTemporalStructureHoldoutExposure(receipt.PriorExposure, current) {
		return fmt.Errorf("temporal structure holdout reuses prior source, family, or programme exposure")
	}
	want := unionTemporalStructureHoldoutExposure(receipt.PriorExposure, current)
	if err := validateTemporalStructureHoldoutExposure(receipt.FutureTrainingExclusion); err != nil {
		return fmt.Errorf("temporal structure holdout future training exclusion is invalid: %w", err)
	}
	if !equalTemporalStructureHoldoutExposure(receipt.FutureTrainingExclusion, want) {
		return fmt.Errorf("temporal structure holdout future training exclusion is not the exact cumulative union")
	}
	return nil
}

func validateTemporalStructureHoldoutLineage(receipt TemporalStructureHoldoutReceipt) error {
	if err := validateTemporalStructureHoldoutExposure(receipt.PriorExposure); err != nil {
		return fmt.Errorf("temporal structure holdout prior exposure is invalid: %w", err)
	}
	if receipt.PlanKind != TemporalStructureHoldoutPlanGenesis && receipt.PlanKind != TemporalStructureHoldoutPlanReplacement {
		return fmt.Errorf("temporal structure holdout plan kind is invalid")
	}
	if receipt.PlanKind == TemporalStructureHoldoutPlanGenesis && (len(receipt.PriorExposure.SourceSHA256) != 0 || len(receipt.PriorExposure.FamilyIDs) != 0 || len(receipt.PriorExposure.ProgrammeProvenance) != 0) {
		return fmt.Errorf("genesis temporal structure holdout carries prior exposure")
	}
	return nil
}

func validateTemporalStructureHoldoutInputs(inputs []TemporalStructureHoldoutInput, planKind string) error {
	want := map[string]struct{}{
		"evidence_manifest": {}, "evidence_private_map": {}, "family_audit": {}, "human_assessment": {},
		"human_attestation": {}, "media_quality": {}, "programme_inventory": {}, "reference_audit": {}, "reference_download_ledger": {},
		"selection": {}, "suitability": {},
		"transition_authority": {},
	}
	if len(inputs) < len(want) || !sort.SliceIsSorted(inputs, func(i, j int) bool { return inputs[i].Name < inputs[j].Name }) {
		return fmt.Errorf("temporal structure holdout receipt input authority is incomplete or unordered")
	}
	priorInputs := 0
	seenInputs := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if !reviewSHA256(input.SHA256) {
			return fmt.Errorf("temporal structure holdout receipt contains an invalid input")
		}
		if _, duplicate := seenInputs[input.Name]; duplicate {
			return fmt.Errorf("temporal structure holdout receipt repeats an input")
		}
		seenInputs[input.Name] = struct{}{}
		if _, exists := want[input.Name]; exists {
			delete(want, input.Name)
			continue
		}
		if !strings.HasPrefix(input.Name, "prior_adjudication:") || strings.TrimPrefix(input.Name, "prior_adjudication:") == "" {
			return fmt.Errorf("temporal structure holdout receipt contains an invalid input")
		}
		priorInputs++
	}
	if len(want) != 0 || planKind == TemporalStructureHoldoutPlanGenesis && priorInputs != 0 || planKind == TemporalStructureHoldoutPlanReplacement && priorInputs == 0 {
		return fmt.Errorf("temporal structure holdout receipt input authority does not match lineage")
	}
	return nil
}

func disjointTemporalStructureHoldoutExposure(left, right TemporalStructureHoldoutTrainingExclusion) bool {
	for _, pair := range [][2][]string{{left.SourceSHA256, right.SourceSHA256}, {left.FamilyIDs, right.FamilyIDs}} {
		seen := stringSet(pair[0])
		for _, value := range pair[1] {
			if _, exists := seen[value]; exists {
				return false
			}
		}
	}
	seenProvenance := make(map[string]struct{}, len(left.ProgrammeProvenance))
	for _, value := range left.ProgrammeProvenance {
		seenProvenance[temporalStructureProgrammeProvenanceKey(value)] = struct{}{}
	}
	for _, value := range right.ProgrammeProvenance {
		if _, exists := seenProvenance[temporalStructureProgrammeProvenanceKey(value)]; exists {
			return false
		}
	}
	return true
}

func equalTemporalStructureHoldoutExposure(left, right TemporalStructureHoldoutTrainingExclusion) bool {
	return left.Split == right.Split && slices.Equal(left.SourceSHA256, right.SourceSHA256) && slices.Equal(left.FamilyIDs, right.FamilyIDs) && slices.Equal(left.ProgrammeProvenance, right.ProgrammeProvenance)
}

func indexTemporalStructureHoldoutAuthoring(authoring TemporalStructureChallengeAuthoring) (map[string]TemporalStructureChallengeSource, map[string]TemporalStructureChallengeCase, map[fillereval.UnitKind]int, error) {
	sources := make(map[string]TemporalStructureChallengeSource, len(authoring.Sources))
	for _, source := range authoring.Sources {
		if _, duplicate := sources[source.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("temporal structure holdout repeats source %q", source.ID)
		}
		sources[source.ID] = source
	}
	cases := make(map[string]TemporalStructureChallengeCase, len(authoring.Cases))
	unitCounts := make(map[fillereval.UnitKind]int)
	for _, item := range authoring.Cases {
		if _, duplicate := cases[item.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("temporal structure holdout repeats case %q", item.ID)
		}
		cases[item.ID] = item
		unitCounts[item.Unit]++
	}
	return sources, cases, unitCounts, nil
}

func validateTemporalStructureHoldoutAnchors(receipt TemporalStructureHoldoutReceipt, sources map[string]TemporalStructureChallengeSource) (map[string]TemporalStructureHoldoutAnchor, error) {
	anchors := make(map[string]TemporalStructureHoldoutAnchor, len(receipt.SelectedAnchors))
	seenAliases, seenCases, seenFamilies, seenRanks, seenSourceSHA := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	roleCounts := make(map[fillereval.TemporalRole]int)
	for _, item := range receipt.SelectedAnchors {
		source, exists := sources[item.SourceID]
		if !exists || source.Provenance.Kind != TemporalStructureSourceBoundedItem || source.DurationMS != item.DurationMS || source.StandaloneRole != item.Role || item.DurationMS <= 0 || strings.TrimSpace(item.EvidenceAlias) == "" || strings.TrimSpace(item.CaseID) == "" || strings.TrimSpace(item.FamilyID) == "" || !reviewSHA256(item.RankSHA256) {
			return nil, fmt.Errorf("temporal structure holdout contains an invalid selected anchor")
		}
		if _, duplicate := anchors[item.SourceID]; duplicate {
			return nil, fmt.Errorf("temporal structure holdout repeats an anchor source")
		}
		if _, duplicate := seenSourceSHA[source.SHA256]; duplicate {
			return nil, fmt.Errorf("temporal structure holdout repeats anchor source bytes")
		}
		seenSourceSHA[source.SHA256] = struct{}{}
		for label, seen := range map[string]map[string]struct{}{"alias": seenAliases, "case": seenCases, "family": seenFamilies, "rank": seenRanks} {
			value := map[string]string{"alias": item.EvidenceAlias, "case": item.CaseID, "family": item.FamilyID, "rank": item.RankSHA256}[label]
			if _, duplicate := seen[value]; duplicate {
				return nil, fmt.Errorf("temporal structure holdout repeats an anchor %s", label)
			}
			seen[value] = struct{}{}
		}
		anchors[item.SourceID] = item
		roleCounts[item.Role]++
	}
	for role, quota := range temporalStructureHoldoutRoleQuotas {
		if roleCounts[role] != quota || receipt.StandaloneRoleCounts[role] != quota {
			return nil, fmt.Errorf("temporal structure holdout standalone role quota drift for %s", role)
		}
	}
	if len(roleCounts) != len(temporalStructureHoldoutRoleQuotas) || len(receipt.StandaloneRoleCounts) != len(temporalStructureHoldoutRoleQuotas) {
		return nil, fmt.Errorf("temporal structure holdout contains an unexpected standalone role")
	}
	return anchors, nil
}

func validateTemporalStructureHoldoutStandaloneCases(cases map[string]TemporalStructureChallengeCase, anchors map[string]TemporalStructureHoldoutAnchor) error {
	seen := map[string]struct{}{}
	for _, item := range cases {
		if item.Unit != fillereval.UnitStandalone {
			continue
		}
		if len(item.Segments) != 1 {
			return fmt.Errorf("temporal structure holdout standalone case must have one segment")
		}
		anchor, exists := anchors[item.Segments[0].SourceID]
		if !exists || item.Role != anchor.Role || item.Segments[0].StartMS != 0 || item.Segments[0].DurationMS != anchor.DurationMS {
			return fmt.Errorf("temporal structure holdout standalone case drifts from its anchor")
		}
		seen[anchor.SourceID] = struct{}{}
	}
	if len(seen) != len(anchors) {
		return fmt.Errorf("temporal structure holdout does not use each selected anchor exactly once as standalone")
	}
	return nil
}

func validateTemporalStructureHoldoutCompilations(items []TemporalStructureHoldoutCompilation, cases map[string]TemporalStructureChallengeCase, anchors map[string]TemporalStructureHoldoutAnchor, transition *TemporalTransitionAuthority) error {
	bands, seenCases, seenPairs := map[string]int{}, map[string]struct{}{}, map[string]struct{}{}
	sameRoleByBand := map[string]int{}
	strataByBand := map[string]map[TemporalTransitionStratum]int{"early": {}, "middle": {}, "late": {}}
	usedByBand := map[string]map[string]struct{}{"early": {}, "middle": {}, "late": {}}
	transitionByAlias := map[string]TemporalTransitionAuthorityCase{}
	if transition != nil {
		transitionByAlias = make(map[string]TemporalTransitionAuthorityCase, len(transition.Cases))
		for _, item := range transition.Cases {
			transitionByAlias[item.EvidenceAlias] = item
		}
	}
	for _, item := range items {
		challenge, exists := cases[item.CaseID]
		first, firstExists := anchors[item.FirstSourceID]
		second, secondExists := anchors[item.SecondSourceID]
		pairID := item.FirstSourceID + "\x00" + item.SecondSourceID
		roles := []fillereval.TemporalRole{first.Role, second.Role}
		if !exists || challenge.Unit != fillereval.UnitCompilation || !firstExists || !secondExists || item.FirstSourceID == item.SecondSourceID || !slices.Equal(challenge.Slices, []string{temporalStructureMultiTrait(roles), TemporalStructureSliceTwoItemCompilation}) || len(challenge.Segments) != 2 || item.JoinAtMS != first.DurationMS || item.DurationMS != first.DurationMS+second.DurationMS || item.JoinBand != temporalStructureHoldoutJoinBand(item.JoinAtMS, item.DurationMS) || len(item.Roles) != 2 || item.Roles[0] != string(first.Role) || item.Roles[1] != string(second.Role) || challenge.Segments[0].SourceID != item.FirstSourceID || challenge.Segments[0].DurationMS != first.DurationMS || challenge.Segments[1].SourceID != item.SecondSourceID || challenge.Segments[1].DurationMS != second.DurationMS {
			return fmt.Errorf("temporal structure holdout contains an invalid compilation construction")
		}
		if transition != nil {
			firstTransition, firstTransitionExists := transitionByAlias[first.EvidenceAlias]
			secondTransition, secondTransitionExists := transitionByAlias[second.EvidenceAlias]
			wantStratum, resolved := temporalTransitionStratum(firstTransition, secondTransition)
			if !firstTransitionExists || !secondTransitionExists || !resolved || item.TransitionStratum != wantStratum {
				return fmt.Errorf("temporal structure holdout compilation transition authority drift")
			}
		}
		if _, duplicate := seenCases[item.CaseID]; duplicate {
			return fmt.Errorf("temporal structure holdout repeats a compilation case")
		}
		if _, duplicate := seenPairs[pairID]; duplicate {
			return fmt.Errorf("temporal structure holdout repeats a compilation pair")
		}
		usedSources, validBand := usedByBand[item.JoinBand]
		if !validBand {
			return fmt.Errorf("temporal structure holdout contains an unknown compilation join band")
		}
		if _, duplicate := usedSources[item.FirstSourceID]; duplicate {
			return fmt.Errorf("temporal structure holdout compilation band reuses a source")
		}
		if _, duplicate := usedSources[item.SecondSourceID]; duplicate {
			return fmt.Errorf("temporal structure holdout compilation band reuses a source")
		}
		usedSources[item.FirstSourceID], usedSources[item.SecondSourceID] = struct{}{}, struct{}{}
		seenCases[item.CaseID], seenPairs[pairID] = struct{}{}, struct{}{}
		bands[item.JoinBand]++
		switch item.TransitionStratum {
		case TemporalTransitionBlackBoundary, TemporalTransitionAudibleNonblackCut, TemporalTransitionSilenceTouchedNonblackCut:
			strataByBand[item.JoinBand][item.TransitionStratum]++
		default:
			return fmt.Errorf("temporal structure holdout contains an unknown transition stratum")
		}
		if first.Role == second.Role {
			sameRoleByBand[item.JoinBand]++
		}
	}
	if bands["early"] != 4 || bands["middle"] != 4 || bands["late"] != 4 || len(bands) != 3 {
		return fmt.Errorf("temporal structure holdout compilation join bands are unbalanced")
	}
	if sameRoleByBand["early"] != 2 || sameRoleByBand["middle"] != 2 || sameRoleByBand["late"] != 2 {
		return fmt.Errorf("temporal structure holdout compilation roles are unbalanced")
	}
	for _, band := range []string{"early", "middle", "late"} {
		if strataByBand[band][TemporalTransitionBlackBoundary] == 0 || strataByBand[band][TemporalTransitionAudibleNonblackCut] == 0 || strataByBand[band][TemporalTransitionSilenceTouchedNonblackCut] == 0 {
			return fmt.Errorf("temporal structure holdout transition strata are incomplete in %s", band)
		}
	}
	return nil
}

func validateTemporalStructureHoldoutProgrammeCuts(items []TemporalStructureHoldoutProgrammeCut, cases map[string]TemporalStructureChallengeCase, sources map[string]TemporalStructureChallengeSource) error {
	parents, seenCases, seenPatterns := map[string]int{}, map[string]struct{}{}, map[string]struct{}{}
	patternCounts := map[string]int{}
	for _, item := range items {
		challenge, caseExists := cases[item.CaseID]
		source, sourceExists := sources[item.SourceID]
		patternID := item.SourceID + "\x00" + item.Pattern
		wantSlice := map[string]string{"near_parent_start": TemporalStructureSliceProgrammeNearStart, "near_parent_end": TemporalStructureSliceProgrammeNearEnd}[item.Pattern]
		if !caseExists || challenge.Unit != fillereval.UnitProgrammeExcerpt || !sourceExists || source.Provenance.Kind != TemporalStructureSourceProgrammeParent || source.DurationMS != item.ParentEndMS || !slices.Equal(challenge.Slices, []string{wantSlice}) || len(challenge.Segments) != 1 || challenge.Segments[0].SourceID != item.SourceID || challenge.Segments[0].StartMS != item.StartMS || challenge.Segments[0].DurationMS != item.DurationMS || item.StartMS < 10_000 || item.StartMS+item.DurationMS > item.ParentEndMS-10_000 {
			return fmt.Errorf("temporal structure holdout contains an invalid programme cut")
		}
		switch item.Pattern {
		case "near_parent_start":
			if item.StartMS != 10_000 || item.DurationMS != 30_000 {
				return fmt.Errorf("temporal structure holdout start cut drift")
			}
		case "near_parent_end":
			if item.StartMS != item.ParentEndMS-55_000 || item.DurationMS != 45_000 {
				return fmt.Errorf("temporal structure holdout end cut drift")
			}
		default:
			return fmt.Errorf("temporal structure holdout contains an unknown programme cut pattern")
		}
		if _, duplicate := seenCases[item.CaseID]; duplicate {
			return fmt.Errorf("temporal structure holdout repeats a programme case")
		}
		if _, duplicate := seenPatterns[patternID]; duplicate {
			return fmt.Errorf("temporal structure holdout repeats a programme cut pattern")
		}
		seenCases[item.CaseID], seenPatterns[patternID] = struct{}{}, struct{}{}
		parents[item.SourceID]++
		patternCounts[item.Pattern]++
	}
	if len(parents) != temporalStructureHoldoutParentSources {
		return fmt.Errorf("temporal structure holdout does not use six distinct programme parents")
	}
	for _, count := range parents {
		if count != 2 {
			return fmt.Errorf("temporal structure holdout must make two cuts per programme parent")
		}
	}
	if patternCounts["near_parent_start"] != temporalStructureHoldoutParentSources || patternCounts["near_parent_end"] != temporalStructureHoldoutParentSources || len(patternCounts) != 2 {
		return fmt.Errorf("temporal structure holdout programme patterns are unbalanced")
	}
	return nil
}

func validateTemporalStructureHoldoutProgrammeSpots(items []TemporalStructureHoldoutProgrammeSpot, cases map[string]TemporalStructureChallengeCase, sources map[string]TemporalStructureChallengeSource, anchors map[string]TemporalStructureHoldoutAnchor) error {
	parents, fillers, patterns, seenCases := map[string]int{}, map[string]int{}, map[string]int{}, map[string]struct{}{}
	for _, item := range items {
		challenge, caseExists := cases[item.CaseID]
		parent, parentExists := sources[item.ParentSourceID]
		anchor, fillerExists := anchors[item.FillerSourceID]
		wantSlice := map[string]string{"early_insert": TemporalStructureSliceSpotEarly, "late_insert": TemporalStructureSliceSpotLate}[item.Pattern]
		if !caseExists || challenge.Unit != fillereval.UnitProgrammeSpots || !parentExists || parent.Provenance.Kind != TemporalStructureSourceProgrammeParent || !fillerExists || !slices.Equal(challenge.Slices, []string{wantSlice}) || len(challenge.Segments) != 3 || item.ParentDurationMS != parent.DurationMS || item.ProgrammePartMS != 20_000 || item.FillerDurationMS != anchor.DurationMS || item.FillerRole != anchor.Role || challenge.Segments[0] != (TemporalStructureChallengeSegment{SourceID: parent.ID, StartMS: item.BeforeSourceStartMS, DurationMS: item.ProgrammePartMS}) || challenge.Segments[1] != (TemporalStructureChallengeSegment{SourceID: anchor.SourceID, DurationMS: anchor.DurationMS}) || challenge.Segments[2] != (TemporalStructureChallengeSegment{SourceID: parent.ID, StartMS: item.AfterSourceStartMS, DurationMS: item.ProgrammePartMS}) {
			return fmt.Errorf("temporal structure holdout contains an invalid programme-with-spots construction")
		}
		switch item.Pattern {
		case "early_insert":
			if item.BeforeSourceStartMS != 10_000 || item.AfterSourceStartMS != 30_000 {
				return fmt.Errorf("temporal structure holdout early inserted-spot pattern drift")
			}
		case "late_insert":
			if item.BeforeSourceStartMS != parent.DurationMS-50_000 || item.AfterSourceStartMS != parent.DurationMS-30_000 {
				return fmt.Errorf("temporal structure holdout late inserted-spot pattern drift")
			}
		default:
			return fmt.Errorf("temporal structure holdout contains an unknown inserted-spot pattern")
		}
		if _, duplicate := seenCases[item.CaseID]; duplicate {
			return fmt.Errorf("temporal structure holdout repeats a programme-with-spots case")
		}
		seenCases[item.CaseID] = struct{}{}
		parents[item.ParentSourceID]++
		fillers[item.FillerSourceID]++
		patterns[item.Pattern]++
	}
	if len(parents) != temporalStructureHoldoutParentSources || len(fillers) != temporalStructureHoldoutClassCases || patterns["early_insert"] != temporalStructureHoldoutParentSources || patterns["late_insert"] != temporalStructureHoldoutParentSources || len(patterns) != 2 {
		return fmt.Errorf("temporal structure holdout programme-with-spots coverage is incomplete")
	}
	for _, count := range parents {
		if count != 2 {
			return fmt.Errorf("temporal structure holdout must make two inserted-spot cases per programme parent")
		}
	}
	for _, count := range fillers {
		if count != 1 {
			return fmt.Errorf("temporal structure holdout must use each filler once in an inserted-spot case")
		}
	}
	return nil
}
