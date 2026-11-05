package fillerscreening

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const testDurationMS = int64(60_000)

func TestEvaluatorPassesOnlyCompleteCertifiedNegativeCoverage(t *testing.T) {
	t.Parallel()
	for _, profile := range []AirworthinessProfile{ProfileAllAges, ProfileGeneralAudience} {
		profile := profile
		t.Run(string(profile), func(t *testing.T) {
			t.Parallel()
			evaluator, profiles := testEvaluator(t, profile)
			decision := evaluator.Evaluate(testDocument(profiles))
			if decision.Verdict != VerdictPass || !reflect.DeepEqual(decision.ReasonCodes, []AirworthinessReason{ReasonEvidenceSatisfied}) {
				t.Fatalf("decision = %#v", decision)
			}
			if err := ValidateAirworthinessDecision(decision); err != nil {
				t.Fatalf("validate decision: %v", err)
			}
		})
	}
}

func TestRestrictedArchiveNeverAuthorizesUnattendedPlayout(t *testing.T) {
	t.Parallel()
	evaluator, profiles := testEvaluator(t, ProfileRestrictedArchive)
	decision := evaluator.Evaluate(testDocument(profiles))
	if decision.Verdict != VerdictHold || !slices.Contains(decision.ReasonCodes, ReasonRestrictedArchive) {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestRejectingObservationWinsOverReviewConflictAndMissingCoverage(t *testing.T) {
	t.Parallel()
	evaluator, profiles := testEvaluator(t, ProfileAllAges)
	document := testDocument(profiles)
	document.Axes[0].Observations = []AirworthinessObservation{
		testObservation("adult-nudity", FlagAdultNudity, SeverityLow, ContextDepiction),
		testObservation("frightening", FlagFrighteningOrDisturbing, SeverityLow, ContextDepiction),
	}
	document.Axes[1].Coverage = CoverageConflict
	document.Axes[2].Coverage = CoverageIncomplete

	decision := evaluator.Evaluate(document)
	if decision.Verdict != VerdictReject || len(decision.Triggers) != 1 ||
		decision.Triggers[0].Flag != FlagAdultNudity || decision.Triggers[0].Effect != EffectReject {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestReviewObservationHolds(t *testing.T) {
	t.Parallel()
	evaluator, profiles := testEvaluator(t, ProfileGeneralAudience)
	document := testDocument(profiles)
	document.Axes[1].Observations = []AirworthinessObservation{
		testObservation("moderate-profanity", FlagProfanity, SeverityModerate, ContextDepiction),
	}
	decision := evaluator.Evaluate(document)
	if decision.Verdict != VerdictHold || !slices.Contains(decision.ReasonCodes, ReasonObservationRequiresReview) ||
		len(decision.Triggers) != 1 || decision.Triggers[0].Effect != EffectReview {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestContextualObservationsDoNotBecomeProhibitions(t *testing.T) {
	t.Parallel()
	evaluator, profiles := testEvaluator(t, ProfileAllAges)
	document := testDocument(profiles)
	document.Axes[0].Observations = []AirworthinessObservation{
		testObservation("age-ambiguous", FlagAgeAmbiguous, SeverityHigh, ContextPresence),
		testObservation("minor-present", FlagMinorPresent, SeverityHigh, ContextPresence),
		testObservation("commercial", FlagCommercialOrBrand, SeverityHigh, ContextPromotion),
	}
	document.Axes[1].Observations = []AirworthinessObservation{
		testObservation("religion", FlagReligiousSuffering, SeverityHigh, ContextDepiction),
		testObservation("war", FlagWarOrMilitary, SeverityHigh, ContextDepiction),
	}
	decision := evaluator.Evaluate(document)
	if decision.Verdict != VerdictPass || len(decision.ObservedFlags) != 5 {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestIncompleteCertificationAndCoverageHold(t *testing.T) {
	t.Parallel()
	profiles := testProfiles()
	profiles[0].CertifiedFlags = slices.DeleteFunc(profiles[0].CertifiedFlags, func(flag AirworthinessFlag) bool {
		return flag == FlagAdultNudity
	})
	evaluator, err := NewAirworthinessEvaluator(ProfileAllAges, profiles)
	if err != nil {
		t.Fatalf("new evaluator: %v", err)
	}
	document := testDocument(profiles)
	document.Axes[1].Coverage = CoverageFailed
	decision := evaluator.Evaluate(document)
	if decision.Verdict != VerdictHold ||
		!slices.Contains(decision.ReasonCodes, ReasonCertificationIncomplete) ||
		!slices.Contains(decision.ReasonCodes, ReasonCoverageIncomplete) ||
		!reflect.DeepEqual(decision.HeldAxes, []AirworthinessAxis{AxisVisual, AxisSpoken}) {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestMalformedOrDriftedEvidenceHolds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*AirworthinessDocument)
	}{
		{"unknown flag", func(document *AirworthinessDocument) {
			document.Axes[0].Observations = []AirworthinessObservation{testObservation("unknown", AirworthinessFlag("unknown"), SeverityLow, ContextPresence)}
		}},
		{"unsupported severity", func(document *AirworthinessDocument) {
			document.Axes[0].Observations = []AirworthinessObservation{testObservation("severity", FlagAdultNudity, AirworthinessSeverity("unknown"), ContextDepiction)}
		}},
		{"unsupported context", func(document *AirworthinessDocument) {
			document.Axes[0].Observations = []AirworthinessObservation{testObservation("context", FlagAdultNudity, SeverityLow, AirworthinessContext("unknown"))}
		}},
		{"invalid interval", func(document *AirworthinessDocument) {
			observation := testObservation("interval", FlagAdultNudity, SeverityLow, ContextDepiction)
			observation.EndMS = testDurationMS + 1
			document.Axes[0].Observations = []AirworthinessObservation{observation}
		}},
		{"profile drift", func(document *AirworthinessDocument) { document.Axes[0].Profile.PolicySHA256 = testSHA('f') }},
		{"subject drift", func(document *AirworthinessDocument) { document.Axes[0].SubjectSHA256 = testSHA('e') }},
		{"invalid evidence digest", func(document *AirworthinessDocument) { document.Axes[0].EvidenceSHA256 = "invalid" }},
		{"duplicate axis", func(document *AirworthinessDocument) { document.Axes[2] = document.Axes[0] }},
		{"duplicate observation id", func(document *AirworthinessDocument) {
			document.Axes[1].Observations = []AirworthinessObservation{testObservation("duplicate", FlagProfanity, SeverityLow, ContextDepiction)}
			document.Axes[2].Observations = []AirworthinessObservation{testObservation("duplicate", FlagProfanity, SeverityLow, ContextDepiction)}
		}},
		{"observations on failed coverage", func(document *AirworthinessDocument) {
			document.Axes[0].Coverage = CoverageFailed
			document.Axes[0].Observations = []AirworthinessObservation{testObservation("failed-positive", FlagAdultNudity, SeverityLow, ContextDepiction)}
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			evaluator, profiles := testEvaluator(t, ProfileAllAges)
			document := testDocument(profiles)
			test.mutate(&document)
			decision := evaluator.Evaluate(document)
			if decision.Verdict != VerdictHold || !slices.Contains(decision.ReasonCodes, ReasonEvidenceInvalid) {
				t.Fatalf("decision = %#v", decision)
			}
		})
	}
}

func TestEvaluationIsOrderIndependentAndSelfAddressed(t *testing.T) {
	t.Parallel()
	evaluator, profiles := testEvaluator(t, ProfileGeneralAudience)
	document := testDocument(profiles)
	document.Axes[0].Observations = []AirworthinessObservation{
		testObservation("weapon", FlagWeaponDepiction, SeverityLow, ContextDepiction),
		testObservation("graphic", FlagGraphicViolenceOrGore, SeverityHigh, ContextDepiction),
	}
	document.Axes[1].Observations = []AirworthinessObservation{
		testObservation("profanity", FlagProfanity, SeverityModerate, ContextDepiction),
	}
	first := evaluator.Evaluate(document)
	slices.Reverse(document.Axes)
	for index := range document.Axes {
		slices.Reverse(document.Axes[index].Observations)
	}
	second := evaluator.Evaluate(document)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reordered decision changed:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if err := ValidateAirworthinessDecision(first); err != nil {
		t.Fatalf("validate decision: %v", err)
	}
	mutated := first
	mutated.Triggers = slices.Clone(first.Triggers)
	mutated.Triggers[0].Severity = SeverityLow
	if err := ValidateAirworthinessDecision(mutated); err == nil {
		t.Fatal("mutated decision validated")
	}
}

func TestPublicDecisionCannotCarryRawProse(t *testing.T) {
	t.Parallel()
	evaluator, profiles := testEvaluator(t, ProfileAllAges)
	document := testDocument(profiles)
	document.Axes[0].Observations = []AirworthinessObservation{
		testObservation("opaque-observation-1", FlagAdultNudity, SeverityHigh, ContextDepiction),
	}
	raw, err := json.Marshal(evaluator.Evaluate(document))
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	for _, forbidden := range []string{"description", "transcript", "reasoning", "prompt", "path"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("public decision contains forbidden field %q: %s", forbidden, raw)
		}
	}
}

func testEvaluator(t *testing.T, profile AirworthinessProfile) (*AirworthinessEvaluator, []AirworthinessAxisProfile) {
	t.Helper()
	profiles := testProfiles()
	evaluator, err := NewAirworthinessEvaluator(profile, profiles)
	if err != nil {
		t.Fatalf("new evaluator: %v", err)
	}
	return evaluator, profiles
}

func testProfiles() []AirworthinessAxisProfile {
	profiles := make([]AirworthinessAxisProfile, 0, len(axisOrder))
	for index, axis := range axisOrder {
		flags := make([]AirworthinessFlag, 0, len(vocabulary))
		for _, flag := range vocabulary {
			if axisOwnsFlag(axis, flag) {
				flags = append(flags, flag)
			}
		}
		profiles = append(profiles, AirworthinessAxisProfile{
			Axis: axis, EvidenceContract: "axis-evidence-v1", PolicySHA256: testSHA(byte('1' + index)),
			CertificationSHA256: testSHA(byte('4' + index)), ImplementationSHA256: testSHA(byte('7' + index)),
			CertifiedFlags: flags,
		})
	}
	return profiles
}

func testDocument(profiles []AirworthinessAxisProfile) AirworthinessDocument {
	document := AirworthinessDocument{
		SchemaVersion: AirworthinessEvidenceSchemaVersion, ContractVersion: AirworthinessEvidenceContractVersion,
		SubjectSHA256: testSHA('a'), DurationMS: testDurationMS,
	}
	for index, profile := range profiles {
		document.Axes = append(document.Axes, AirworthinessAxisEvidence{
			SubjectSHA256: document.SubjectSHA256, Profile: profile, Coverage: CoverageComplete,
			EvidenceSHA256: testSHA(byte('b' + index)), Observations: []AirworthinessObservation{},
		})
	}
	return document
}

func testObservation(id string, flag AirworthinessFlag, severity AirworthinessSeverity, context AirworthinessContext) AirworthinessObservation {
	return AirworthinessObservation{ID: id, Flag: flag, Severity: severity, Context: context, StartMS: 1_000, EndMS: 2_000}
}

func testSHA(character byte) string { return strings.Repeat(string(character), 64) }
