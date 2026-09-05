package fillerdecision

import (
	"context"
	"errors"
	"slices"
	"strings"
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
		if mode == ApplicationModeApplied {
			record.ClipHash = strings.Repeat("a", 64)
			record.ScreeningEvidenceSHA256 = strings.Repeat("b", 64)
			record.ReleaseAuthoritySHA256 = strings.Repeat("c", 64)
		}
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

func TestValidateRecordBindsAppliedReleaseEvidenceOnly(t *testing.T) {
	applied := validRecord()
	applied.ApplicationMode = ApplicationModeApplied
	applied.ClipHash = strings.Repeat("a", 64)
	applied.ScreeningEvidenceSHA256 = strings.Repeat("b", 64)
	applied.ReleaseAuthoritySHA256 = strings.Repeat("c", 64)
	if err := ValidateRecord(applied); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Record){
		"catalog hash": func(record *Record) { record.ClipHash = "clip" },
		"screening":    func(record *Record) { record.ScreeningEvidenceSHA256 = "" },
		"release":      func(record *Record) { record.ReleaseAuthoritySHA256 = strings.Repeat("C", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			record := applied
			mutate(&record)
			if err := ValidateRecord(record); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
	shadow := validRecord()
	shadow.ScreeningEvidenceSHA256 = strings.Repeat("b", 64)
	if err := ValidateRecord(shadow); !errors.Is(err, ErrInvalid) {
		t.Fatalf("shadow release binding = %v, want ErrInvalid", err)
	}
}

type actionRoutingRepository struct {
	Repository
	record fillerdecisionRecordResult
	action Action
}

type fillerdecisionRecordResult struct {
	value Record
	err   error
}

func (r *actionRoutingRepository) GetFillerDecision(context.Context, string) (Record, error) {
	return r.record.value, r.record.err
}

func (r *actionRoutingRepository) CommitFillerDecisionAction(_ context.Context, action Action) error {
	r.action = action
	return nil
}

type appliedActionExecutor struct{ action Action }

func (e *appliedActionExecutor) ActOnAppliedFillerDecision(_ context.Context, _ Record, action Action) error {
	e.action = action
	return nil
}

func TestServiceRoutesActionsByApplicationMode(t *testing.T) {
	action := Action{
		ID: "action-1", DecisionID: "decision-1", ActorID: "admin-1", Kind: ActionAdmit,
		CreatedAt: time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC),
	}
	t.Run("shadow", func(t *testing.T) {
		repo := &actionRoutingRepository{record: fillerdecisionRecordResult{value: validRecord()}}
		service, err := New(repo)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Act(t.Context(), action); err != nil || repo.action != action {
			t.Fatalf("Act = %v, action = %+v", err, repo.action)
		}
	})
	t.Run("applied unavailable", func(t *testing.T) {
		record := validRecord()
		record.ApplicationMode = ApplicationModeApplied
		repo := &actionRoutingRepository{record: fillerdecisionRecordResult{value: record}}
		service, err := New(repo)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Act(t.Context(), action); !errors.Is(err, ErrAppliedUnavailable) {
			t.Fatalf("Act = %v, want ErrAppliedUnavailable", err)
		}
		if repo.action.ID != "" {
			t.Fatal("applied action used the shadow writer")
		}
	})
	t.Run("applied terminal executor", func(t *testing.T) {
		record := validRecord()
		record.ApplicationMode = ApplicationModeApplied
		repo := &actionRoutingRepository{record: fillerdecisionRecordResult{value: record}}
		executor := &appliedActionExecutor{}
		service, err := New(repo)
		if err != nil {
			t.Fatal(err)
		}
		service.WithAppliedActions(executor)
		if err := service.Act(t.Context(), action); err != nil || executor.action != action {
			t.Fatalf("Act = %v, terminal action = %+v", err, executor.action)
		}
		if repo.action.ID != "" {
			t.Fatal("applied action used the shadow writer")
		}
	})
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
