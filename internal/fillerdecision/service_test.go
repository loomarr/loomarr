package fillerdecision

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
)

func TestValidateRecordKeepsSemanticAndOperationalStatesDisjoint(t *testing.T) {
	valid := validRecord()
	if err := ValidateRecord(valid); err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(*Record){
		"both outcomes": func(r *Record) {
			r.Result.Hold = &filleradmission.Hold{Code: filleradmission.HoldBudgetExhausted}
		},
		"neither outcome": func(r *Record) { r.Result = filleradmission.Result{} },
		"review without one question": func(r *Record) {
			r.Result.Decision.Verdict = filleradmission.VerdictReview
			r.Result.Decision.ReviewQuestion = ""
		},
		"non-review with a question": func(r *Record) {
			r.Result.Decision.ReviewQuestion = "Should this be admitted?"
		},
		"invalid utf8": func(r *Record) { r.EvidenceHash = string([]byte{0xff}) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			record := validRecord()
			mutate(&record)
			if err := ValidateRecord(record); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateRecordRequiresClosedApplicationMode(t *testing.T) {
	for _, mode := range []ApplicationMode{ApplicationModeShadow, ApplicationModeApplied} {
		record := validRecord()
		record.ApplicationMode = mode
		if err := ValidateRecord(record); err != nil {
			t.Fatalf("ValidateRecord(application mode %q) = %v", mode, err)
		}
	}

	for _, mode := range []ApplicationMode{"", "automatic"} {
		record := validRecord()
		record.ApplicationMode = mode
		if err := ValidateRecord(record); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ValidateRecord(application mode %q) = %v, want ErrInvalid", mode, err)
		}
	}
}

func TestValidateCorrectionRequiresAnAnswerAndClosedVerdict(t *testing.T) {
	action := Action{
		ID: "action-1", DecisionID: "decision-1", ActorID: "admin-1", Kind: ActionCorrect,
		Answer: "The end card identifies soda.", CorrectedVerdict: filleradmission.VerdictAdmit,
		CreatedAt: time.Date(2026, 8, 25, 5, 0, 0, 0, time.UTC),
	}
	if err := ValidateAction(action); err != nil {
		t.Fatal(err)
	}
	action.Answer = ""
	if err := ValidateAction(action); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing answer = %v, want ErrInvalid", err)
	}
	action.Answer = "known"
	action.CorrectedVerdict = filleradmission.VerdictReview
	if err := ValidateAction(action); !errors.Is(err, ErrInvalid) {
		t.Fatalf("review correction = %v, want ErrInvalid", err)
	}
}

func TestValidateAbandonCarriesNoInferredVerdict(t *testing.T) {
	action := Action{
		ID: "action-skip", DecisionID: "decision-1", ActorID: "admin-1", Kind: ActionAbandon,
		Reason: "skip for now", CreatedAt: time.Date(2026, 8, 25, 5, 0, 0, 0, time.UTC),
	}
	if err := ValidateAction(action); err != nil {
		t.Fatal(err)
	}
	action.CorrectedVerdict = filleradmission.VerdictReject
	if err := ValidateAction(action); !errors.Is(err, ErrInvalid) {
		t.Fatalf("abandon with inferred verdict = %v, want ErrInvalid", err)
	}
}

func TestValidateRecordBoundsCanonicalPayload(t *testing.T) {
	record := validRecord()
	record.Result.Decision.ReasonCodes = slices.Repeat(
		[]filleradmission.ReasonCode{filleradmission.ReasonEvidenceSatisfied}, 20_000,
	)
	if err := ValidateRecord(record); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized result = %v, want ErrInvalid", err)
	}
}

func TestRecoveryActionsAreServerOwned(t *testing.T) {
	cases := map[filleradmission.OperationalCode]RecoveryAction{
		filleradmission.HoldProviderUnavailable: RecoveryConfigureProvider,
		filleradmission.HoldRouteUnavailable:    RecoveryConfigureProvider,
		filleradmission.HoldBudgetExhausted:     RecoveryAdjustBudget,
		filleradmission.HoldSchemaInvalid:       RecoveryUpdatePolicy,
	}
	for code, want := range cases {
		if got := recoveryFor(code, false); got != want {
			t.Errorf("recoveryFor(%q) = %q, want %q", code, got, want)
		}
	}
	if got := recoveryFor(filleradmission.HoldExtractionFailed, true); got != RecoveryRetryExtraction {
		t.Errorf("retryable extraction recovery = %q", got)
	}
	if got := recoveryFor(filleradmission.HoldExtractionFailed, false); got != RecoveryInspectMedia {
		t.Errorf("terminal extraction recovery = %q", got)
	}
}

func validRecord() Record {
	return Record{
		ID: "decision-1", ClipHash: "clip-1", EvidenceHash: "evidence-1",
		EvidenceVersion: "e1", SchemaVersion: 1, PolicyVersion: "p1", TaxonomyVersion: "t1",
		ApplicationMode: ApplicationModeShadow,
		CreatedAt:       time.Date(2026, 8, 25, 5, 0, 0, 0, time.UTC),
		Result: filleradmission.Result{Decision: &filleradmission.Decision{
			Verdict:     filleradmission.VerdictAdmit,
			ReasonCodes: []filleradmission.ReasonCode{filleradmission.ReasonEvidenceSatisfied},
		}},
	}
}
