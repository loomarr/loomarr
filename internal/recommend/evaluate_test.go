package recommend_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/recommend"
)

func TestEvaluateRejectsEvidenceAbsentFromTheSuppliedContext(t *testing.T) {
	snapshot := recommend.Snapshot{
		ID: "sparse-library",
		Signals: []recommend.Signal{
			{ID: "library:genre:science-fiction", Kind: recommend.SignalLibraryGenre, Value: "Science Fiction"},
			{ID: "preference:tone:hopeful", Kind: recommend.SignalPreference, Value: "hopeful"},
		},
	}
	raw := []byte(`{"concepts":[{"name":"Hopeful Futures","intent":{"description":"Hopeful science-fiction adventures"},"evidenceIds":["library:genre:science-fiction","invented:viewer-history"]}]}`)

	assessment := recommend.Evaluate(snapshot, raw)
	if assessment.Passed {
		t.Fatal("assessment passed with unsupported evidence")
	}
	if !hasFailure(assessment.HardFailures, recommend.FailureUnsupportedEvidence) {
		t.Fatalf("hard failures = %v, want %q", assessment.HardFailures, recommend.FailureUnsupportedEvidence)
	}
	if len(assessment.Concepts) != 0 {
		t.Fatalf("unsupported concept escaped evaluation: %+v", assessment.Concepts)
	}
}

func TestEvaluateRejectsConceptsThatClaimEffectAuthority(t *testing.T) {
	snapshot := recommend.Snapshot{ID: "effects", Signals: []recommend.Signal{{
		ID: "library:genre:comedy", Kind: recommend.SignalLibraryGenre, Value: "Comedy",
	}}}
	raw := []byte(`{"concepts":[{"name":"Comedy Loop","intent":{"description":"A comedy channel"},"evidenceIds":["library:genre:comedy"],"channelId":"ch-created","status":"approved"}]}`)

	assessment := recommend.Evaluate(snapshot, raw)
	if assessment.Passed || !hasFailure(assessment.HardFailures, recommend.FailureEffectAuthority) {
		t.Fatalf("assessment = %+v, want effect-authority hard failure", assessment)
	}
	if len(assessment.Concepts) != 0 {
		t.Fatalf("effectful concept escaped evaluation: %+v", assessment.Concepts)
	}
}

func TestEvaluateRejectsAConceptAlreadyRepresentedByAnExistingChannel(t *testing.T) {
	snapshot := recommend.Snapshot{
		ID:      "repetitive",
		Signals: []recommend.Signal{{ID: "library:genre:action", Kind: recommend.SignalLibraryGenre, Value: "Action"}},
		ExistingConcepts: []recommend.ExistingConcept{{
			Name: "Action Heroes", IntentDescription: "High-energy action heroes",
		}},
	}
	raw := []byte(`{"concepts":[{"name":"Action Heroes!","intent":{"description":"high energy action heroes"},"evidenceIds":["library:genre:action"]}]}`)

	assessment := recommend.Evaluate(snapshot, raw)
	if assessment.Passed || !hasFailure(assessment.HardFailures, recommend.FailureDuplicateConcept) {
		t.Fatalf("assessment = %+v, want duplicate hard failure", assessment)
	}
	if len(assessment.Concepts) != 0 {
		t.Fatalf("duplicate concept escaped evaluation: %+v", assessment.Concepts)
	}
}

func TestEvaluateRejectsUnknownOutputFieldsInsteadOfIgnoringThem(t *testing.T) {
	snapshot := recommend.Snapshot{ID: "schema", Signals: []recommend.Signal{{
		ID: "preference:tone:cozy", Kind: recommend.SignalPreference, Value: "cozy",
	}}}
	raw := []byte(`{"concepts":[{"name":"Cozy Evenings","intent":{"description":"Cozy evening television"},"evidenceIds":["preference:tone:cozy"],"confidenceLabel":"definitely"}]}`)

	assessment := recommend.Evaluate(snapshot, raw)
	if assessment.Passed || !hasFailure(assessment.HardFailures, recommend.FailureInvalidSchema) {
		t.Fatalf("assessment = %+v, want invalid-schema hard failure", assessment)
	}
	if len(assessment.Concepts) != 0 {
		t.Fatalf("unknown-field concept escaped evaluation: %+v", assessment.Concepts)
	}
}

func hasFailure(failures []recommend.HardFailure, code string) bool {
	for _, failure := range failures {
		if failure.Code == code {
			return true
		}
	}
	return false
}
