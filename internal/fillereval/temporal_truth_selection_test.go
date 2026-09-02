package fillereval

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestBuildTemporalTruthSelectionIsDeterministicAndKeepsBucketsDisjoint(t *testing.T) {
	candidates := temporalTruthCandidatesFixture()
	inputs := []TemporalTruthInputDigest{{Name: "z", SHA256: strings.Repeat("a", 64)}, {Name: "a", SHA256: strings.Repeat("b", 64)}}
	first, err := BuildTemporalTruthSelection("sealed-seed-v1", inputs, candidates)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildTemporalTruthSelection("sealed-seed-v1", inputs, candidates)
	if err != nil {
		t.Fatal(err)
	}
	firstRaw, _ := json.Marshal(first)
	secondRaw, _ := json.Marshal(second)
	if string(firstRaw) != string(secondRaw) {
		t.Fatal("selection is not byte-deterministic")
	}
	if len(first.Cases) != TemporalTruthSelectionCases || first.Inputs[0].Name != "a" {
		t.Fatalf("unexpected selection shape: %+v", first)
	}
	counts := map[string]int{}
	riskCounts := map[string]int{}
	seen := map[string]struct{}{}
	for _, item := range first.Cases {
		counts[item.Bucket]++
		riskCounts[item.RiskClass]++
		if _, duplicate := seen[item.ContentSHA256]; duplicate {
			t.Fatalf("duplicate selected content %s", item.ContentSHA256)
		}
		seen[item.ContentSHA256] = struct{}{}
	}
	for _, bucket := range []string{TemporalTruthBucketAgreement, TemporalTruthBucketDisagreement, TemporalTruthBucketHighRisk} {
		if counts[bucket] != temporalTruthBucketCases {
			t.Fatalf("bucket %s = %d", bucket, counts[bucket])
		}
	}
	for _, riskClass := range []string{TemporalTruthRiskProgrammeExcerpt, TemporalTruthRiskCompilation, TemporalTruthRiskUnusableUnclear, TemporalTruthRiskShortBoundary} {
		if riskCounts[riskClass] != temporalTruthRiskClassCases {
			t.Fatalf("risk class %s = %d", riskClass, riskCounts[riskClass])
		}
	}
}

func TestBuildTemporalTruthSelectionFailsInsteadOfBorrowing(t *testing.T) {
	candidates := temporalTruthCandidatesFixture()
	filtered := candidates[:0]
	short := 0
	for _, candidate := range candidates {
		if temporalTruthRiskClass(candidate) == TemporalTruthRiskShortBoundary {
			short++
			if short > 3 {
				continue
			}
		}
		filtered = append(filtered, candidate)
	}
	_, err := BuildTemporalTruthSelection("seed", []TemporalTruthInputDigest{{Name: "draft", SHA256: strings.Repeat("a", 64)}}, filtered)
	if err == nil || !strings.Contains(err.Error(), "short_boundary") || !strings.Contains(err.Error(), "no fallback borrowing") {
		t.Fatalf("undersized risk stratum error = %v", err)
	}
}

func TestBuildTemporalTruthSelectionRejectsDuplicateContentAndInvalidNormalizedRole(t *testing.T) {
	candidates := temporalTruthCandidatesFixture()
	candidates[1].ContentSHA256 = candidates[0].ContentSHA256
	_, err := BuildTemporalTruthSelection("seed", []TemporalTruthInputDigest{{Name: "draft", SHA256: strings.Repeat("a", 64)}}, candidates)
	if err == nil || !strings.Contains(err.Error(), "repeat content") {
		t.Fatalf("duplicate content error = %v", err)
	}
	candidates = temporalTruthCandidatesFixture()
	candidates[0].Assessments[0].Role = TemporalRoleCommercial
	_, err = BuildTemporalTruthSelection("seed", []TemporalTruthInputDigest{{Name: "draft", SHA256: strings.Repeat("a", 64)}}, candidates)
	if err == nil || !strings.Contains(err.Error(), "invalid normalized assessment") {
		t.Fatalf("invalid role error = %v", err)
	}
}

func temporalTruthCandidatesFixture() []TemporalTruthCandidate {
	var result []TemporalTruthCandidate
	add := func(prefix string, count int, duration int64, assessments ...TemporalTruthCandidateAssessment) {
		for index := range count {
			identity := fmt.Sprintf("%s-%02d", prefix, index)
			digest := temporalTruthRank("fixture", "content", identity)
			copied := append([]TemporalTruthCandidateAssessment(nil), assessments...)
			result = append(result, TemporalTruthCandidate{
				CaseID: identity, ContentSHA256: digest, SourceLane: fmt.Sprintf("source-%d", index%5), DurationMS: duration + int64(index), Assessments: copied,
			})
		}
	}
	assessment := func(assessor string, unit UnitKind, role TemporalRole) TemporalTruthCandidateAssessment {
		return TemporalTruthCandidateAssessment{Assessor: assessor, Unit: unit, Role: role}
	}
	add("programme", 8, 70_000, assessment("a", UnitProgrammeExcerpt, ""), assessment("b", UnitProgrammeExcerpt, ""), assessment("c", UnitStandalone, TemporalRolePromo))
	add("compilation", 8, 70_000, assessment("a", UnitCompilation, ""), assessment("b", UnitCompilation, ""), assessment("c", UnitStandalone, TemporalRoleCommercial))
	add("unclear", 8, 70_000, assessment("a", UnitUnclear, ""), assessment("b", UnitStandalone, TemporalRolePromo), assessment("c", UnitStandalone, TemporalRolePromo))
	add("short", 8, 10_000, assessment("a", UnitStandalone, TemporalRoleBumper), assessment("b", UnitStandalone, TemporalRoleBumper), assessment("c", UnitStandalone, TemporalRoleBumper))
	add("agreement", 24, 30_000, assessment("a", UnitStandalone, TemporalRoleCommercial), assessment("b", UnitStandalone, TemporalRoleCommercial), assessment("c", UnitStandalone, TemporalRoleCommercial))
	add("disagreement", 24, 30_000, assessment("a", UnitStandalone, TemporalRoleCommercial), assessment("b", UnitStandalone, TemporalRolePromo), assessment("c", UnitStandalone, TemporalRoleCommercial))
	return result
}
