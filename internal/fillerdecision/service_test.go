package fillerdecision

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/testkit/recordfixture"
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
	record   fillerdecisionRecordResult
	actions  recordfixture.Recorder[Action, struct{}]
	existing Action
}

type fillerdecisionRecordResult struct {
	value Record
	err   error
}

func (r *actionRoutingRepository) GetFillerDecision(context.Context, string) (Record, error) {
	return r.record.value, r.record.err
}

func (r *actionRoutingRepository) CommitFillerDecisionAction(_ context.Context, action Action) error {
	_, err := r.actions.Call(action)
	return err
}

func (r *actionRoutingRepository) FindFillerDecisionAction(_ context.Context, id string) (Action, bool, error) {
	return r.existing, r.existing.ID == id, nil
}

type appliedActionExecutor struct {
	actions recordfixture.Recorder[Action, struct{}]
}

func (e *appliedActionExecutor) ActOnAppliedFillerDecision(_ context.Context, _ Record, action Action) error {
	_, err := e.actions.Call(action)
	return err
}

type attentionRepository struct {
	Repository
	page DecisionPage
}

func (r *attentionRepository) ListFillerDecisions(context.Context, DecisionFilter) (DecisionPage, error) {
	return r.page, nil
}

type diagnosticRepository struct {
	Repository
	page     DecisionPage
	record   Record
	existing DiagnosticRecoveryRequest
	commits  recordfixture.Recorder[DiagnosticRecoveryRequest, struct{}]
}

func (r *diagnosticRepository) ListFillerDecisions(context.Context, DecisionFilter) (DecisionPage, error) {
	return r.page, nil
}

func (r *diagnosticRepository) GetFillerDecision(context.Context, string) (Record, error) {
	return r.record, nil
}

func (r *diagnosticRepository) FindFillerDiagnosticRecovery(_ context.Context, id string) (DiagnosticRecoveryRequest, bool, error) {
	return r.existing, r.existing.ID == id, nil
}

func (r *diagnosticRepository) CommitFillerDiagnosticRecovery(_ context.Context, request DiagnosticRecoveryRequest) error {
	_, err := r.commits.Call(request)
	return err
}

type diagnosticExecutor struct {
	status    DiagnosticRetryStatus
	statusErr error
	retries   recordfixture.Recorder[string, struct{}]
}

func (e *diagnosticExecutor) DiagnosticRetryStatus(context.Context, string) (DiagnosticRetryStatus, error) {
	return e.status, e.statusErr
}

func (e *diagnosticExecutor) RetryDiagnostic(_ context.Context, hash string) error {
	_, err := e.retries.Call(hash)
	return err
}

func TestAttentionProjectsTaskKindAndOnlyCurrentlyAllowedActions(t *testing.T) {
	at := time.Date(2026, 9, 12, 13, 0, 0, 0, time.UTC)
	review := func(id string, mode ApplicationMode, reasons ...filleradmission.ReasonCode) Record {
		record := validRecord()
		record.ID = id
		record.ApplicationMode = mode
		record.CreatedAt = at
		record.Result.Decision.Verdict = filleradmission.VerdictReview
		record.Result.Decision.ReviewQuestion = "What should Loomarr record?"
		record.Result.Decision.ReasonCodes = reasons
		return record
	}
	repo := &attentionRepository{page: DecisionPage{Rows: []Record{
		review("identity", ApplicationModeShadow, filleradmission.ReasonConflictContentRole),
		review("rights", ApplicationModeShadow, filleradmission.ReasonMissingSourceLicense),
		review("suitability", ApplicationModeShadow, filleradmission.ReasonInsufficientSensitiveEvidence),
		review("applied-unavailable", ApplicationModeApplied, filleradmission.ReasonMissingCommercialIdentity),
	}, Total: 4}}
	service, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}

	page, err := service.Attention(t.Context(), Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []AttentionTask{
		{ID: "identity", Kind: AttentionIdentityRole, AllowedActions: []ActionKind{ActionAdmit, ActionReject, ActionCorrect, ActionAbandon}},
		{ID: "rights", Kind: AttentionRightsProvenance, AllowedActions: []ActionKind{ActionAbandon}},
		{ID: "suitability", Kind: AttentionSuitabilityException, AllowedActions: []ActionKind{ActionAbandon}},
		{ID: "applied-unavailable", Kind: AttentionIdentityRole, AllowedActions: []ActionKind{}},
	}
	if len(page.Tasks) != len(want) || page.Total != len(want) {
		t.Fatalf("Attention = %+v, want %d tasks", page, len(want))
	}
	for i := range want {
		if page.Tasks[i].ID != want[i].ID || page.Tasks[i].Kind != want[i].Kind ||
			!slices.Equal(page.Tasks[i].AllowedActions, want[i].AllowedActions) {
			t.Errorf("task %d = %+v, want identity/kind/actions %+v", i, page.Tasks[i], want[i])
		}
	}
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
		if err := service.Act(t.Context(), action); err != nil {
			t.Fatalf("Act = %v, action = %+v", err, repo.actions.Inputs())
		}
		if actions := repo.actions.Inputs(); len(actions) != 1 || actions[0] != action {
			t.Fatalf("recorded actions = %+v", actions)
		}
	})
	t.Run("action absent from attention projection", func(t *testing.T) {
		record := validRecord()
		record.Result.Decision.ReasonCodes = []filleradmission.ReasonCode{filleradmission.ReasonMissingSourceLicense}
		repo := &actionRoutingRepository{record: fillerdecisionRecordResult{value: record}}
		service, err := New(repo)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.ActOnAttention(t.Context(), action); !errors.Is(err, ErrActionNotAllowed) {
			t.Fatalf("ActOnAttention = %v, want ErrActionNotAllowed", err)
		}
		if repo.actions.Calls() != 0 {
			t.Fatal("an action absent from Attention reached the writer")
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
		if err := service.ActOnAttention(t.Context(), action); !errors.Is(err, ErrActionNotAllowed) {
			t.Fatalf("ActOnAttention = %v, want ErrActionNotAllowed", err)
		}
		if repo.actions.Calls() != 0 {
			t.Fatal("applied action used the shadow writer")
		}
	})
	t.Run("applied exact retry without terminal executor", func(t *testing.T) {
		record := validRecord()
		record.ApplicationMode = ApplicationModeApplied
		repo := &actionRoutingRepository{record: fillerdecisionRecordResult{value: record}, existing: action}
		service, err := New(repo)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.ActOnAttention(t.Context(), action); err != nil {
			t.Fatalf("ActOnAttention = %v, want recorded result", err)
		}
		if repo.actions.Calls() != 0 {
			t.Fatal("exact retry used the shadow writer")
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
		if err := service.Act(t.Context(), action); err != nil {
			t.Fatalf("Act = %v, terminal action = %+v", err, executor.actions.Inputs())
		}
		if actions := executor.actions.Inputs(); len(actions) != 1 || actions[0] != action {
			t.Fatalf("terminal action = %+v", actions)
		}
		if repo.actions.Calls() != 0 {
			t.Fatal("applied action used the shadow writer")
		}
	})
}

func TestSameActionUsesEveryRequestIdentityFieldExceptCreatedAt(t *testing.T) {
	base := Action{
		ID: "action-1", DecisionID: "decision-1", ActorID: "admin-1", Kind: ActionCorrect,
		Reason: "operator correction", Answer: "The closing card identifies soda.",
		CorrectedVerdict: filleradmission.VerdictAdmit, SupersedesID: "previous-action",
		CreatedAt: time.Date(2026, 8, 25, 5, 0, 0, 0, time.UTC),
	}
	if changedAt := base; func() bool {
		changedAt.CreatedAt = changedAt.CreatedAt.Add(time.Hour)
		return !SameAction(base, changedAt)
	}() {
		t.Fatal("server-assigned CreatedAt changed retry identity")
	}
	for name, mutate := range map[string]func(*Action){
		"decision":   func(a *Action) { a.DecisionID = "decision-2" },
		"actor":      func(a *Action) { a.ActorID = "admin-2" },
		"kind":       func(a *Action) { a.Kind = ActionReject },
		"reason":     func(a *Action) { a.Reason = "other correction" },
		"answer":     func(a *Action) { a.Answer = "other answer" },
		"verdict":    func(a *Action) { a.CorrectedVerdict = filleradmission.VerdictReject },
		"supersedes": func(a *Action) { a.SupersedesID = "other-action" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if SameAction(base, changed) {
				t.Fatalf("%s was accepted as the same request", name)
			}
		})
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

func TestDiagnosticsProjectCompletableServerOwnedRecoveryPlans(t *testing.T) {
	at := time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC)
	records := []Record{
		operationalRecord("provider", "provider-clip", filleradmission.HoldProviderUnavailable, true),
		operationalRecord("budget", "budget-clip", filleradmission.HoldBudgetExhausted, false),
		operationalRecord("retry", "retry-clip", filleradmission.HoldExtractionFailed, true),
		operationalRecord("inspect", "inspect-clip", filleradmission.HoldExtractionFailed, false),
		operationalRecord("policy", "policy-clip", filleradmission.HoldSchemaInvalid, false),
	}
	for index := range records {
		records[index].CreatedAt = at.Add(time.Duration(index) * time.Second)
	}
	repo := &diagnosticRepository{page: DecisionPage{Rows: records, Total: len(records)}}
	service, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.Diagnostics(t.Context(), Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []RecoveryPlan{
		{Action: RecoveryConfigureProvider, Mode: RecoveryModeConfiguration, Destination: "/settings/ai"},
		{Action: RecoveryAdjustBudget, Mode: RecoveryModeConfiguration, Destination: "/filler/settings"},
		{Action: RecoveryRetryExtraction, Mode: RecoveryModeManualRetry},
		{Action: RecoveryInspectMedia, Mode: RecoveryModeInspection, Destination: "/v1/filler/media/inspect-clip"},
		{Action: RecoveryUpdatePolicy, Mode: RecoveryModeConfiguration, Destination: "/filler/settings"},
	}
	if len(page.Rows) != len(want) {
		t.Fatalf("Diagnostics rows = %d, want %d", len(page.Rows), len(want))
	}
	for index := range want {
		if page.Rows[index].Recovery != want[index] {
			t.Errorf("recovery %d = %+v, want %+v", index, page.Rows[index].Recovery, want[index])
		}
	}
}

func TestDiagnosticsProjectAutomaticRetryWithExactSchedule(t *testing.T) {
	retryAt := time.Date(2026, 9, 12, 15, 30, 0, 0, time.UTC)
	record := operationalRecord("retry", "retry-clip", filleradmission.HoldExtractionFailed, true)
	repo := &diagnosticRepository{page: DecisionPage{Rows: []Record{record}, Total: 1}}
	executor := &diagnosticExecutor{status: DiagnosticRetryStatus{Automatic: true, RetryAt: retryAt}}
	service, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	service.WithDiagnosticRecovery(executor)
	page, err := service.Diagnostics(t.Context(), Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := RecoveryPlan{Action: RecoveryRetryExtraction, Mode: RecoveryModeAutomaticRetry, RetryAt: retryAt}
	if len(page.Rows) != 1 || page.Rows[0].Recovery != want {
		t.Fatalf("automatic recovery = %+v, want %+v", page.Rows, want)
	}
}

func TestRetryDiagnosticExecutesAndRecordsOneIdempotentRequest(t *testing.T) {
	record := operationalRecord("retry", "retry-clip", filleradmission.HoldExtractionFailed, true)
	repo := &diagnosticRepository{record: record, page: DecisionPage{Rows: []Record{record}, Total: 1}}
	executor := &diagnosticExecutor{}
	retryAt := time.Date(2026, 9, 12, 15, 30, 0, 0, time.UTC)
	executor.retries.Respond = func(string) (struct{}, error) {
		executor.status = DiagnosticRetryStatus{Automatic: true, RetryAt: retryAt}
		return struct{}{}, nil
	}
	service, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	service.WithDiagnosticRecovery(executor)
	request := DiagnosticRecoveryRequest{
		ID: "recovery-1", DecisionID: record.ID, ActorID: "admin-1", Action: DiagnosticRecoveryRetry,
		CreatedAt: time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC),
	}
	if err := service.RetryDiagnostic(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if executor.retries.Calls() != 1 || repo.commits.Calls() != 1 {
		t.Fatalf("retry calls = %d, commits = %d", executor.retries.Calls(), repo.commits.Calls())
	}
	page, err := service.Diagnostics(t.Context(), Cursor{}, 10)
	if err != nil || len(page.Rows) != 1 || page.Rows[0].Recovery.Mode != RecoveryModeAutomaticRetry ||
		!page.Rows[0].Recovery.RetryAt.Equal(retryAt) {
		t.Fatalf("recovery after retry = %+v, %v", page, err)
	}
	repo.existing = request
	if err := service.RetryDiagnostic(t.Context(), request); err != nil {
		t.Fatalf("idempotent retry = %v", err)
	}
	if executor.retries.Calls() != 1 || repo.commits.Calls() != 1 {
		t.Fatal("idempotent request executed or committed twice")
	}
}

func TestRetryDiagnosticLeavesFailedAndDisallowedHoldsUntouched(t *testing.T) {
	record := operationalRecord("retry", "retry-clip", filleradmission.HoldExtractionFailed, true)
	repo := &diagnosticRepository{record: record}
	executor := &diagnosticExecutor{}
	executor.retries.Respond = func(string) (struct{}, error) { return struct{}{}, errors.New("rewind failed") }
	service, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	service.WithDiagnosticRecovery(executor)
	request := DiagnosticRecoveryRequest{
		ID: "recovery-1", DecisionID: record.ID, ActorID: "admin-1", Action: DiagnosticRecoveryRetry,
		CreatedAt: time.Date(2026, 9, 12, 15, 0, 0, 0, time.UTC),
	}
	if err := service.RetryDiagnostic(t.Context(), request); err == nil {
		t.Fatal("failed retry returned success")
	}
	if repo.commits.Calls() != 0 {
		t.Fatal("failed executor recorded a recovery action")
	}

	repo.record = operationalRecord("provider", "provider-clip", filleradmission.HoldProviderUnavailable, true)
	executor.retries.Respond = nil
	request.DecisionID = repo.record.ID
	if err := service.RetryDiagnostic(t.Context(), request); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatalf("provider retry = %v, want ErrActionNotAllowed", err)
	}
}

func operationalRecord(id, hash string, code filleradmission.OperationalCode, retryable bool) Record {
	record := validRecord()
	record.ID, record.ClipHash = id, hash
	record.Result = filleradmission.Result{Hold: &filleradmission.Hold{Code: code, Retryable: retryable}}
	return record
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
