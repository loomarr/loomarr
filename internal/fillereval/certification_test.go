package fillereval

import "testing"

func TestCertificationContractRejectsDependentHoldoutAndConcentration(t *testing.T) {
	manifest, _ := passingCorpus(CertificationMinHoldout)
	manifest.Cases[1].Cluster = manifest.Cases[0].Cluster
	manifest.Cases[1].Provenance.SourceFamily = manifest.Cases[0].Provenance.SourceFamily
	for i := 0; i < 9; i++ {
		manifest.Cases[i].Provenance.Creator = "one-creator"
		manifest.Cases[i].Provenance.Campaign = "one-campaign"
	}
	failures := append(ValidateManifest(manifest), ValidateCertificationContract(manifest)...)
	for _, term := range []string{"holdout similarity cluster", "holdout source family", "creator \"one-creator\"", "holdout campaign \"one-campaign\""} {
		if !containsFailure(failures, term) {
			t.Errorf("failures %v do not include %q", failures, term)
		}
	}
}

func TestCertificationContractRejectsUndersizedDenominator(t *testing.T) {
	manifest, _ := passingCorpus(CertificationMinHoldout)
	manifest.Cases[892].Truth = TruthAmbiguous
	manifest.Cases[892].RejectClass = ""
	manifest.Cases[892].ReviewQuestion = "Is this usable filler?"
	if failures := ValidateCertificationContract(manifest); !containsFailure(failures, "semantic-invalid holdout has 146 cases") {
		t.Fatalf("undersized semantic denominator accepted: %v", failures)
	}
}

func TestCertificationConfidenceAllowsOneErrorButNotTwo(t *testing.T) {
	manifest, predictions := passingCorpus(CertificationMinHoldout)
	predictions[0].Verdict = VerdictReject
	predictions[0].RejectClass = RejectDeterministic
	if report := Score(manifest, predictions, completeRun()); !report.Certified {
		t.Fatalf("one deterministic precision error should fit the declared margin: %v", report.Failures)
	}
	predictions[1].Verdict = VerdictReject
	predictions[1].RejectClass = RejectDeterministic
	if report := Score(manifest, predictions, completeRun()); report.Certified || !containsFailure(report.Failures, "deterministic reject precision") {
		t.Fatalf("two deterministic precision errors crossed the bound without failing: %v", report.Failures)
	}
}

func TestCertificationRequiresAutomationConfidenceNotOnlyPointEstimate(t *testing.T) {
	manifest, predictions := passingCorpus(CertificationMinHoldout)
	for i := 0; i < 44; i++ {
		predictions[i].Verdict = VerdictReview
		predictions[i].ReviewQuestion = "Is this eligible filler?"
	}
	report := Score(manifest, predictions, completeRun())
	if report.Metrics.ValidAutomation < .90 || report.Certified || !containsFailure(report.Failures, "valid filler automation") {
		t.Fatalf("point-only automation certified: metric %.4f lower %.4f failures %v", report.Metrics.ValidAutomation, report.Metrics.ValidAutomationLower, report.Failures)
	}
}
