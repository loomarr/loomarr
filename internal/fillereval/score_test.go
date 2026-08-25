package fillereval

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSeedCorpusContract(t *testing.T) {
	data, err := os.ReadFile("corpus/seed-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if failures := ValidateManifest(manifest); len(failures) > 0 {
		t.Fatalf("invalid seed corpus: %v", failures)
	}
	if len(manifest.Cases) >= 300 {
		t.Fatalf("seed corpus unexpectedly claims certification scale: %d", len(manifest.Cases))
	}
}

func TestValidateManifestRejectsSimilarityLeakage(t *testing.T) {
	manifest, _ := passingCorpus(2)
	manifest.Cases[0].Split = SplitDevelopment
	manifest.Cases[1].Split = SplitHoldout
	manifest.Cases[1].Cluster = manifest.Cases[0].Cluster
	if failures := ValidateManifest(manifest); !containsFailure(failures, "crosses development and holdout") {
		t.Fatalf("failures = %v", failures)
	}
}

func TestScoreCertifiesOnlyMeasuredSelectiveRiskAndCoverage(t *testing.T) {
	manifest, predictions := passingCorpus(500)
	report := Score(manifest, predictions, completeRun())
	if !report.Certified {
		t.Fatalf("report unexpectedly failed: %v", report.Failures)
	}
	if report.Metrics.AutoAdmitPrecision != 1 || report.Metrics.AutoAdmitPrecisionLower < .99 {
		t.Fatalf("admit precision = %.4f lower %.4f", report.Metrics.AutoAdmitPrecision, report.Metrics.AutoAdmitPrecisionLower)
	}
	if report.Metrics.ReviewRate > .10 || report.Metrics.ReviewAnswerable != 1 {
		t.Fatalf("review rate = %.4f answerable = %.4f", report.Metrics.ReviewRate, report.Metrics.ReviewAnswerable)
	}
}

func TestScoreIsDeterministicForCapturedInputs(t *testing.T) {
	manifest, predictions := passingCorpus(500)
	run := completeRun()
	first := Score(manifest, predictions, run)
	second := Score(manifest, predictions, run)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical captured inputs produced different reports")
	}
	if first.ManifestSHA256 == "" || first.ManifestSHA256 != ManifestSHA256(manifest) {
		t.Fatalf("manifest digest = %q", first.ManifestSHA256)
	}
}

func TestCertificationManifestRequiresLockedProvenanceAndIndependentLabels(t *testing.T) {
	manifest, _ := passingCorpus(500)
	manifest.Cases[0].EvidenceSHA256 = ""
	manifest.Cases[1].Provenance.RightsDecision = ""
	manifest.Cases[2].LabelReviews = manifest.Cases[2].LabelReviews[:1]
	failures := ValidateManifest(manifest)
	for _, term := range []string{"media and evidence hashes", "rights evidence and adjudication", "two independent label reviews"} {
		if !containsFailure(failures, term) {
			t.Errorf("failures %v do not include %q", failures, term)
		}
	}
}

func TestScoreFailsWhenCapturedRunExceedsPredeclaredCeilings(t *testing.T) {
	manifest, predictions := passingCorpus(500)
	run := completeRun()
	run.MaxRequests = 499
	report := Score(manifest, predictions, run)
	if report.Certified || !containsFailure(report.Failures, "request ceiling") {
		t.Fatalf("request ceiling did not fail closed: %v", report.Failures)
	}
}

func TestScoreFailsClosedOnOperationalFailureAndMissingPrediction(t *testing.T) {
	manifest, predictions := passingCorpus(400)
	predictions[0].OperationalFailure = "budget exhausted"
	predictions = predictions[:len(predictions)-1]
	report := Score(manifest, predictions, completeRun())
	if report.Certified {
		t.Fatal("operational and missing results certified")
	}
	want := []string{"operational failure", "missing prediction"}
	for _, term := range want {
		if !containsFailure(report.Failures, term) {
			t.Errorf("failures %v do not include %q", report.Failures, term)
		}
	}
}

func TestScoreTreatsConflictAsReviewNotEvidenceForAdmission(t *testing.T) {
	manifest, predictions := passingCorpus(400)
	manifest.Cases[0].Truth = TruthAmbiguous
	manifest.Cases[0].ReviewQuestion = "Which year describes this recording?"
	predictions[0].Verdict = VerdictAdmit
	predictions[0].Conflicts = []Conflict{{Claim: "recording_year", Values: []string{"1992", "1972"}}}
	report := Score(manifest, predictions, completeRun())
	if report.Cases[0].Correct || report.Certified {
		t.Fatal("a conflicting claim was allowed to admit")
	}
}

func TestScoreRejectsSmallPerfectCorpus(t *testing.T) {
	manifest, predictions := passingCorpus(40)
	report := Score(manifest, predictions, completeRun())
	if report.Certified || !containsFailure(report.Failures, "at least 300") {
		t.Fatalf("small corpus should not certify: %v", report.Failures)
	}
}

func TestFalseSemanticRejectCountsAgainstRejectPrecision(t *testing.T) {
	manifest, predictions := passingCorpus(500)
	predictions[0].Verdict = VerdictReject
	predictions[0].RejectClass = RejectSemantic
	report := Score(manifest, predictions, completeRun())
	if report.Metrics.AutoRejectPrecision >= 1 || report.Metrics.SemanticRejectPrecision >= 1 {
		t.Fatalf("false reject was hidden: overall %.4f semantic %.4f", report.Metrics.AutoRejectPrecision, report.Metrics.SemanticRejectPrecision)
	}
}

func TestScoreUsesExactNanodollarAccountingByRungAndSlice(t *testing.T) {
	manifest, predictions := passingCorpus(500)
	predictions[0].ChargedAmount = "0.0000012300"
	predictions[0].ChargedCurrency = "USD"
	predictions[0].ChargedNanoUSD = 1230
	predictions[0].Rung = "frames"
	report := Score(manifest, predictions, completeRun())
	if !report.Certified {
		t.Fatalf("exact accounting failed certification: %v", report.Failures)
	}
	if report.Metrics.TotalChargedNanoUSD != 1230 || report.Metrics.CostPerCorrectAutomationNanoUSD != 3 {
		t.Fatalf("cost metrics = %+v", report.Metrics)
	}
	if len(report.Metrics.Rungs) != 2 || report.Metrics.Rungs[0].Rung != "frames" || report.Metrics.Rungs[0].ChargedNanoUSD != 1230 {
		t.Fatalf("rung metrics = %+v", report.Metrics.Rungs)
	}
	predictions[0].ChargedNanoUSD = 1229
	if report := Score(manifest, predictions, completeRun()); report.Certified || !containsFailure(report.Failures, "projects to 1230") {
		t.Fatalf("mismatched cost projection certified: %v", report.Failures)
	}
}

func TestUSDToNanoCeilRoundsSubNanodollarSpendUp(t *testing.T) {
	got, err := USDToNanoCeil("0.0000000005")
	if err != nil || got != 1 {
		t.Fatalf("USDToNanoCeil = %d, %v", got, err)
	}
}

func TestValidateAccountingPreservesNonUSDCurrencyWithoutInventingFX(t *testing.T) {
	t.Parallel()
	if err := validateAccounting(Prediction{ChargedAmount: "0.25", ChargedCurrency: "EUR"}); err != nil {
		t.Fatalf("provider-reported non-USD charge = %v", err)
	}
	if err := validateAccounting(Prediction{ChargedAmount: "0.25", ChargedCurrency: "EUR", ChargedNanoUSD: 250_000_000}); err == nil {
		t.Fatal("non-USD charge accepted an invented USD projection")
	}
}

func passingCorpus(total int) (Manifest, []Prediction) {
	lockedAt := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	manifest := Manifest{SchemaVersion: SchemaVersion, Kind: CorpusCertification, CorpusVersion: "test-v1", LockedAt: lockedAt, SliceGates: []SliceGate{{Slice: "contract", MinCases: total, MinAccuracy: .99, MinAccuracyLower: .99}}}
	predictions := make([]Prediction, 0, total)
	for i := 0; i < total; i++ {
		truth, verdict, class := TruthEligible, VerdictAdmit, RejectClass("")
		question := ""
		switch {
		case i >= total*60/100 && i < total*80/100:
			truth, verdict, class = TruthInvalid, VerdictReject, RejectDeterministic
		case i >= total*80/100 && i < total*95/100:
			truth, verdict, class = TruthInvalid, VerdictReject, RejectSemantic
		case i >= total*95/100:
			truth, verdict, question = TruthAmbiguous, VerdictReview, "Is this a commercial or a programme excerpt?"
		}
		id := fmt.Sprintf("case-%03d", i)
		c := Case{
			ID: id, Split: SplitHoldout, Cluster: id,
			ContentSHA256: fmt.Sprintf("%064x", i+1), EvidenceSHA256: fmt.Sprintf("%064x", total+i+1),
			Source: "fixture", License: "CC0-1.0", Truth: truth, RejectClass: class,
			ContentRole: "commercial", Slices: []string{"contract"}, ReviewQuestion: question,
			Evidence: []Evidence{{ID: "truth", Kind: "fixture", Claim: "content_role", Value: "commercial", Provenance: "blind annotation"}},
			Provenance: MediaProvenance{
				Authority: "fixture", ItemID: id, ItemURL: "https://example.invalid/items/" + id,
				MetadataRetrievedAt: lockedAt, MetadataSHA256: strings.Repeat("c", 64), EvidenceURL: "https://example.invalid/metadata/" + id,
				RightsStatement: "CC0 fixture", RightsDecision: "allowed", RightsReviewerID: "rights-reviewer", RightsReviewedAt: lockedAt,
				Redistributable: true, SourceFilename: id + ".mp4", SourceURL: "https://example.invalid/media/" + id + ".mp4",
				SourceBytes: 1024, SegmentDurationMS: 30_000,
			},
		}
		labelHash := LabelSHA256(c)
		c.LabelReviews = []LabelReview{
			{ReviewerID: "reviewer-a", BatchID: "blind-a", ReviewedAt: lockedAt, Independent: true, LabelSHA256: labelHash},
			{ReviewerID: "reviewer-b", BatchID: "blind-b", ReviewedAt: lockedAt, Independent: true, LabelSHA256: labelHash},
		}
		manifest.Cases = append(manifest.Cases, c)
		predictions = append(predictions, Prediction{
			CaseID: id, Verdict: verdict, RejectClass: class, ContentRole: "commercial", ReviewQuestion: question,
			Role: "filler_text", Rung: "text", RequestedProvider: "fixture", RequestedModel: "fixture",
			ResolvedModel: "fixture", ResolvedProvider: "fixture", Modalities: []string{"text"}, Attempts: 1, LatencyMS: int64(i),
		})
	}
	for i := 0; i < total/4; i++ {
		id := fmt.Sprintf("development-%03d", i)
		c := Case{
			ID: id, Split: SplitDevelopment, Cluster: id,
			ContentSHA256: fmt.Sprintf("%064x", total*2+i+1), EvidenceSHA256: fmt.Sprintf("%064x", total*3+i+1),
			Source: "fixture", License: "CC0-1.0", Truth: TruthEligible, ContentRole: "commercial", Slices: []string{"contract"},
			Evidence: []Evidence{{ID: "truth", Kind: "fixture", Claim: "content_role", Value: "commercial", Provenance: "blind annotation"}},
			Provenance: MediaProvenance{
				Authority: "fixture", ItemID: id, ItemURL: "https://example.invalid/items/" + id,
				MetadataRetrievedAt: lockedAt, MetadataSHA256: strings.Repeat("d", 64), EvidenceURL: "https://example.invalid/metadata/" + id,
				RightsStatement: "CC0 fixture", RightsDecision: "allowed", RightsReviewerID: "rights-reviewer", RightsReviewedAt: lockedAt,
				Redistributable: true, SourceFilename: id + ".mp4", SourceURL: "https://example.invalid/media/" + id + ".mp4",
				SourceBytes: 1024, SegmentDurationMS: 30_000,
			},
		}
		labelHash := LabelSHA256(c)
		c.LabelReviews = []LabelReview{
			{ReviewerID: "reviewer-a", BatchID: "blind-a", ReviewedAt: lockedAt, Independent: true, LabelSHA256: labelHash},
			{ReviewerID: "reviewer-b", BatchID: "blind-b", ReviewedAt: lockedAt, Independent: true, LabelSHA256: labelHash},
		}
		manifest.Cases = append(manifest.Cases, c)
	}
	return manifest, predictions
}

func completeRun() RunIdentity {
	return RunIdentity{
		Profile: "contract", EvaluationSplit: SplitHoldout, EvidenceVersion: "e1", PromptVersion: "p1", TaxonomyVersion: "t1", PolicyVersion: "a1", RolePolicyVersion: "r1",
		CapabilitySnapshot: "c1", PriceSnapshot: "price1", GeneratedAt: time.Date(2026, time.August, 25, 13, 0, 0, 0, time.UTC),
		MaxRequests: 1000, MaxSpendNanoUSD: 1_000_000_000, MaxConcurrency: 1,
	}
}

func containsFailure(failures []string, term string) bool {
	for _, failure := range failures {
		if strings.Contains(failure, term) {
			return true
		}
	}
	return false
}
