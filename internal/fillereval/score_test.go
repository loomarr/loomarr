package fillereval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
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

func passingCorpus(total int) (Manifest, []Prediction) {
	manifest := Manifest{SchemaVersion: SchemaVersion, CorpusVersion: "test-v1", SliceGates: []SliceGate{{Slice: "contract", MinCases: total, MinAccuracy: .99}}}
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
		manifest.Cases = append(manifest.Cases, Case{ID: id, Split: SplitHoldout, Cluster: id, Source: "synthetic", License: "CC0", Truth: truth, RejectClass: class, ContentRole: "commercial", Slices: []string{"contract"}, ReviewQuestion: question})
		predictions = append(predictions, Prediction{CaseID: id, Verdict: verdict, RejectClass: class, ContentRole: "commercial", ReviewQuestion: question, RequestedModel: "fixture", ResolvedModel: "fixture", ResolvedProvider: "fixture", Modalities: []string{"text"}, LatencyMS: int64(i)})
	}
	return manifest, predictions
}

func completeRun() RunIdentity {
	return RunIdentity{Profile: "contract", EvidenceVersion: "e1", PromptVersion: "p1", TaxonomyVersion: "t1", PolicyVersion: "a1", CapabilitySnapshot: "c1", PriceSnapshot: "price1"}
}

func containsFailure(failures []string, term string) bool {
	for _, failure := range failures {
		if strings.Contains(failure, term) {
			return true
		}
	}
	return false
}
