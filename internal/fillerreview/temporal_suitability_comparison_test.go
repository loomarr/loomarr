package fillerreview

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestCompareTemporalSuitabilityResultsCorroboratesOnlyOverlappingRanges(t *testing.T) {
	aliases := []string{"corroborated", "first-only", "failure", "coverage", "candidate"}
	first := temporalSuitabilityComparisonSet("first", "family-a", aliases)
	second := temporalSuitabilityComparisonSet("second", "family-b", aliases)
	first.Assessments[0].Flags = []TemporalSuitabilityObservation{{Kind: SuitabilityExplicitNudity, StartMS: 100, EndMS: 300, Modality: SuitabilityModalityVideo}}
	first.Assessments[0].Outcome = SuitabilityOutcomeProhibitedSignal
	second.Assessments[0].Flags = []TemporalSuitabilityObservation{{Kind: SuitabilityExplicitNudity, StartMS: 200, EndMS: 400, Modality: SuitabilityModalityVideo}}
	second.Assessments[0].Outcome = SuitabilityOutcomeProhibitedSignal
	first.Assessments[1].Flags = []TemporalSuitabilityObservation{{Kind: SuitabilityHatefulOrDegradingSlur, StartMS: 10, EndMS: 20, Modality: SuitabilityModalityAudio}}
	first.Assessments[1].Outcome = SuitabilityOutcomeProhibitedSignal
	failure := fillereval.TemporalFailureInvalidResponse
	second.Assessments[2].OperationalFailure = &fillereval.TemporalOperationalFailure{Code: failure}
	second.Assessments[2].VisualAssessment, second.Assessments[2].SpokenLanguageAssessment, second.Assessments[2].Outcome = "", "", ""
	second.Assessments[3].SpokenLanguageAssessment = suitabilityLanguageInsufficient
	second.Assessments[3].Outcome = SuitabilityOutcomeCoverageHold

	evidenceSHA, selectionSHA := strings.Repeat("a", 64), temporalTruthJSONSHA(aliases)
	first.EvidenceManifestSHA256, second.EvidenceManifestSHA256 = evidenceSHA, evidenceSHA
	first.SelectionSHA256, second.SelectionSHA256 = selectionSHA, selectionSHA
	report, err := CompareTemporalSuitabilityResults(first, second, evidenceSHA, selectionSHA, strings.Repeat("b", 64), strings.Repeat("c", 64), time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if report.CorroboratedProhibitedCases != 1 || report.UncorroboratedProhibitedCases != 1 || report.OperationalHoldCases != 1 || report.CoverageHoldCases != 1 || report.CandidateNoSignalCases != 1 || report.ProductionAdmissionAllowed {
		t.Fatalf("report = %+v", report)
	}
	var corroborated TemporalSuitabilityCaseComparison
	for _, comparison := range report.CaseComparisons {
		if comparison.EvidenceAlias == "corroborated" {
			corroborated = comparison
		}
	}
	if len(corroborated.CorroboratedFlags) != 1 || corroborated.Disposition != "prohibited_hold" {
		t.Fatalf("corroborated comparison = %+v", corroborated)
	}
}

func temporalSuitabilityComparisonSet(id, family string, aliases []string) TemporalSuitabilityResult {
	set := TemporalSuitabilityResult{
		Assessor:         fillereval.TemporalAssessorIdentity{ID: id, Provider: "openrouter", ModelFamily: family},
		SelectionAliases: append([]string(nil), aliases...), ProductionAdmissionAllowed: false,
	}
	for _, alias := range aliases {
		set.Assessments = append(set.Assessments, TemporalSuitabilityAssessment{
			EvidenceAlias: alias, VisualAssessment: suitabilityVisualCompleted, SpokenLanguageAssessment: suitabilityLanguageCompleted,
			Outcome: SuitabilityOutcomeNoSignalObserved,
		})
	}
	return set
}
