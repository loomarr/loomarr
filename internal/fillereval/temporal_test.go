package fillereval

import (
	"strings"
	"testing"
	"time"
)

func TestDecodeTemporalAssessmentSetRejectsUnknownAndTrailingJSON(t *testing.T) {
	valid := `{"schemaVersion":1,"contractVersion":"filler-temporal-unit-role-v1","batchId":"batch","packageSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","assessor":{"id":"a","provider":"ollama","model":"model","modelFamily":"family-a","modelDigest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","promptVersion":"prompt"},"assessments":[]}`
	if _, err := DecodeTemporalAssessmentSet([]byte(strings.Replace(valid, `"assessments":`, `"unknown":true,"assessments":`, 1))); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	if _, err := DecodeTemporalAssessmentSet([]byte(valid + `{}`)); err == nil || !strings.Contains(err.Error(), "trailing JSON value") {
		t.Fatalf("trailing value error = %v", err)
	}
}

func TestValidateTemporalAssessmentSetEnforcesFactoredContract(t *testing.T) {
	cases := []TemporalCaseSignals{{Alias: "one", DurationMS: 30_000, Signals: []TemporalSignal{{ID: "frame-01", Kind: "frame", AtMS: 1000}}}}
	set := temporalSet("family-a", TemporalAssessment{
		Alias:     "one",
		Unit:      &UnitAssessment{Kind: UnitStandalone, DecisiveSignalIDs: []string{"frame-01"}, Reason: "One complete offer."},
		Role:      &RoleAssessment{Kind: TemporalRoleCommercial, DecisiveSignalIDs: []string{"frame-01"}, Reason: "A product is offered."},
		Inference: temporalInference("unit", "role"),
	})
	if err := ValidateTemporalAssessmentSet(set, "batch", strings.Repeat("a", 64), cases); err != nil {
		t.Fatal(err)
	}

	set.Assessments[0].Role = nil
	if err := ValidateTemporalAssessmentSet(set, "batch", strings.Repeat("a", 64), cases); err == nil || !strings.Contains(err.Error(), "standalone") {
		t.Fatalf("missing standalone role error = %v", err)
	}
	set.Assessments[0].Unit = &UnitAssessment{Kind: UnitCompilation, DecisiveSignalIDs: []string{"frame-01"}, Reason: "Several items."}
	set.Assessments[0].Role = &RoleAssessment{Kind: TemporalRoleCommercial, DecisiveSignalIDs: []string{"frame-01"}, Reason: "Offer."}
	if err := ValidateTemporalAssessmentSet(set, "batch", strings.Repeat("a", 64), cases); err == nil || !strings.Contains(err.Error(), "only standalone") {
		t.Fatalf("non-standalone role error = %v", err)
	}
	set.Assessments[0].Role = nil
	set.Assessments[0].Unit.DecisiveSignalIDs = []string{"unknown"}
	if err := ValidateTemporalAssessmentSet(set, "batch", strings.Repeat("a", 64), cases); err == nil || !strings.Contains(err.Error(), "unknown signal") {
		t.Fatalf("unknown signal error = %v", err)
	}
}

func TestCompareTemporalAssessmentSetsSeparatesUnitAndRoleDisagreement(t *testing.T) {
	cases := []TemporalCaseSignals{
		temporalCase("agree"), temporalCase("unit-dispute"), temporalCase("role-dispute"), temporalCase("failure"),
	}
	first := temporalSet("family-a",
		standaloneAssessment("agree", TemporalRoleCommercial),
		standaloneAssessment("unit-dispute", TemporalRoleCommercial),
		standaloneAssessment("role-dispute", TemporalRoleCommercial),
		TemporalAssessment{Alias: "failure", OperationalFailure: &TemporalOperationalFailure{Code: TemporalFailureTimeout, Detail: "deadline", Retryable: true}, Inference: failedTemporalInference(TemporalFailureTimeout)},
	)
	second := temporalSet("family-b",
		standaloneAssessment("agree", TemporalRoleCommercial),
		TemporalAssessment{Alias: "unit-dispute", Unit: &UnitAssessment{Kind: UnitCompilation, DecisiveSignalIDs: []string{"frame-01"}, Reason: "Several items."}, Inference: temporalInference("unit")},
		standaloneAssessment("role-dispute", TemporalRolePromo),
		TemporalAssessment{Alias: "failure", Unit: &UnitAssessment{Kind: UnitProgrammeExcerpt, DecisiveSignalIDs: []string{"frame-01"}, Reason: "Ordinary programme scene."}, Inference: temporalInference("unit")},
	)

	report, err := CompareTemporalAssessmentSets(first, second, cases)
	if err != nil {
		t.Fatal(err)
	}
	if report.Cases != 4 || report.UnitComparable != 3 || report.UnitAgreement != 2 || report.RoleComparable != 2 || report.RoleAgreement != 1 || report.ExactAgreement != 1 {
		t.Fatalf("agreement counts = %+v", report)
	}
	if report.AdjudicationRequired != 3 || !report.SystemicFailure || len(report.CalibrationCandidates) != 4 {
		t.Fatalf("routing = %+v", report)
	}
	if got := report.Confusions; len(got) != 2 || got[0].Axis != "role" || got[1].Axis != "unit" {
		t.Fatalf("confusions = %+v", got)
	}
}

func TestCompareTemporalAssessmentSetsRejectsSameModelFamily(t *testing.T) {
	cases := []TemporalCaseSignals{temporalCase("one")}
	first := temporalSet("family-a", standaloneAssessment("one", TemporalRoleCommercial))
	second := temporalSet("family-a", standaloneAssessment("one", TemporalRoleCommercial))
	second.Assessor.ID = "second"
	if _, err := CompareTemporalAssessmentSets(first, second, cases); err == nil || !strings.Contains(err.Error(), "distinct model families") {
		t.Fatalf("same-family error = %v", err)
	}
}

func temporalCase(alias string) TemporalCaseSignals {
	return TemporalCaseSignals{Alias: alias, DurationMS: 30_000, Signals: []TemporalSignal{{ID: "frame-01", Kind: "frame", AtMS: 1000}}}
}

func standaloneAssessment(alias string, role TemporalRole) TemporalAssessment {
	return TemporalAssessment{
		Alias:     alias,
		Unit:      &UnitAssessment{Kind: UnitStandalone, DecisiveSignalIDs: []string{"frame-01"}, Reason: "One complete item."},
		Role:      &RoleAssessment{Kind: role, DecisiveSignalIDs: []string{"frame-01"}, Reason: "Observed role."},
		Inference: temporalInference("unit", "role"),
	}
}

func temporalInference(axes ...string) TemporalInference {
	calls := make([]TemporalInferenceCall, 0, len(axes))
	for index, axis := range axes {
		calls = append(calls, TemporalInferenceCall{Axis: axis, Attempt: index + 1, ResponseSHA256: strings.Repeat("c", 64)})
	}
	return TemporalInference{AssessedAt: time.Unix(1, 0).UTC(), Attempts: len(calls), Calls: calls}
}

func failedTemporalInference(code TemporalFailureCode) TemporalInference {
	return TemporalInference{AssessedAt: time.Unix(1, 0).UTC(), Attempts: 1, Calls: []TemporalInferenceCall{{Axis: "unit", Attempt: 1, OperationalFailure: code}}}
}

func temporalSet(family string, assessments ...TemporalAssessment) TemporalAssessmentSet {
	return TemporalAssessmentSet{
		SchemaVersion:   TemporalAssessmentSchemaVersion,
		ContractVersion: TemporalAssessmentContractVersion,
		BatchID:         "batch",
		PackageSHA256:   strings.Repeat("a", 64),
		Assessor: TemporalAssessorIdentity{
			ID: "assessor-" + family, Provider: "ollama", Model: "model", ModelFamily: family,
			ModelDigest: strings.Repeat("b", 64), PromptVersion: "prompt",
		},
		Assessments: assessments,
	}
}
