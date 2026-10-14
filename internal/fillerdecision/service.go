package fillerdecision

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/loomarr/loomarr/internal/filleradmission"
)

type Service struct {
	repo       Repository
	diagnostic DiagnosticRecoveryExecutor
}

func New(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalid)
	}
	return &Service{repo: repo}, nil
}

// WithDiagnosticRecovery attaches the pipeline-owned status and retry seam. Configuration and
// inspection destinations remain available without it; executable retries fail closed.
func (s *Service) WithDiagnosticRecovery(executor DiagnosticRecoveryExecutor) *Service {
	if s != nil {
		s.diagnostic = executor
	}
	return s
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	counts, err := s.repo.FillerDecisionCounts(ctx)
	if err != nil {
		return Overview{}, err
	}
	overview := Overview{Healthy: true, Next: NextNone, Counts: counts}
	switch {
	case counts.Operational > counts.Retryable:
		overview.Healthy, overview.Next = false, NextRepair
		overview.ActionCount = counts.Operational - counts.Retryable
	case counts.Retryable > 0:
		overview.Healthy, overview.Next = false, NextRetry
		overview.ActionCount = counts.Retryable
	case counts.UnresolvedReviews > 0:
		overview.Healthy, overview.Next = false, NextReview
		overview.ActionCount = counts.UnresolvedReviews
	}
	return overview, nil
}

func (s *Service) Diagnostics(ctx context.Context, cursor Cursor, limit int) (DiagnosticPage, error) {
	limit, err := validLimit(limit)
	if err != nil {
		return DiagnosticPage{}, err
	}
	page, err := s.repo.ListFillerDecisions(ctx, DecisionFilter{
		Kind: OutcomeOperational, CurrentOnly: true, Cursor: cursor, Limit: limit,
	})
	if err != nil {
		return DiagnosticPage{}, err
	}
	out := DiagnosticPage{Rows: make([]DiagnosticItem, 0, len(page.Rows)), Total: page.Total}
	for _, record := range page.Rows {
		hold := record.Result.Hold
		recovery, err := s.diagnosticRecovery(ctx, record)
		if err != nil {
			return DiagnosticPage{}, err
		}
		out.Rows = append(out.Rows, DiagnosticItem{
			ID: record.ID, ClipHash: record.ClipHash, Code: hold.Code,
			Retryable: hold.Retryable, Recovery: recovery, CreatedAt: record.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) diagnosticRecovery(ctx context.Context, record Record) (RecoveryPlan, error) {
	hold := record.Result.Hold
	if hold == nil || record.Result.Decision != nil {
		return RecoveryPlan{}, ErrActionNotAllowed
	}
	action := recoveryFor(hold.Code, hold.Retryable)
	plan := RecoveryPlan{Action: action}
	switch action {
	case RecoveryConfigureProvider:
		plan.Mode, plan.Destination = RecoveryModeConfiguration, "/settings/ai"
	case RecoveryAdjustBudget:
		plan.Mode, plan.Destination = RecoveryModeConfiguration, "/filler/settings/limits"
	case RecoveryUpdatePolicy:
		plan.Mode, plan.Destination = RecoveryModeConfiguration, "/filler/settings/review"
	case RecoveryInspectMedia:
		plan.Mode, plan.Destination = RecoveryModeInspection, "/v1/filler/media/"+url.PathEscape(record.ClipHash)
	case RecoveryRetryExtraction:
		plan.Mode = RecoveryModeManualRetry
		if s.diagnostic == nil {
			return plan, nil
		}
		status, err := s.diagnostic.DiagnosticRetryStatus(ctx, record.ClipHash)
		if err != nil {
			return RecoveryPlan{}, err
		}
		if status.Automatic {
			if status.RetryAt.IsZero() {
				return RecoveryPlan{}, fmt.Errorf("%w: automatic diagnostic retry has no schedule", ErrInvalid)
			}
			plan.Mode, plan.RetryAt = RecoveryModeAutomaticRetry, status.RetryAt.UTC()
		}
	}
	return plan, nil
}

// RetryDiagnostic accepts only the retry command projected for the latest retryable extraction
// hold. An already automatic retry needs no second pipeline mutation, but still completes the
// caller's durable idempotency record after a process interruption.
func (s *Service) RetryDiagnostic(ctx context.Context, request DiagnosticRecoveryRequest) error {
	if err := ValidateDiagnosticRecovery(request); err != nil {
		return err
	}
	existing, found, err := s.repo.FindFillerDiagnosticRecovery(ctx, request.ID)
	if err != nil {
		return err
	}
	if found {
		if SameDiagnosticRecovery(existing, request) {
			return nil
		}
		return ErrConflict
	}
	record, err := s.repo.GetFillerDecision(ctx, request.DecisionID)
	if err != nil {
		return err
	}
	plan, err := s.diagnosticRecovery(ctx, record)
	if err != nil {
		return err
	}
	if request.Action != DiagnosticRecoveryRetry || plan.Action != RecoveryRetryExtraction ||
		(plan.Mode != RecoveryModeManualRetry && plan.Mode != RecoveryModeAutomaticRetry) {
		return ErrActionNotAllowed
	}
	if s.diagnostic == nil {
		return ErrRecoveryUnavailable
	}
	if plan.Mode == RecoveryModeManualRetry {
		if err := s.diagnostic.RetryDiagnostic(ctx, record.ClipHash); err != nil {
			return err
		}
	}
	return s.repo.CommitFillerDiagnosticRecovery(ctx, request)
}

func (s *Service) Activity(ctx context.Context, cursor Cursor, limit int) (ActivityPage, error) {
	limit, err := validLimit(limit)
	if err != nil {
		return ActivityPage{}, err
	}
	return s.repo.ListFillerDecisionActivity(ctx, cursor, limit)
}

func validLimit(limit int) (int, error) {
	if limit == 0 {
		return MaxPageSize, nil
	}
	if limit < 1 || limit > MaxPageSize {
		return 0, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalid, MaxPageSize)
	}
	return limit, nil
}

func ValidateRecord(record Record) error {
	if record.ApplicationMode != ApplicationModeShadow && record.ApplicationMode != ApplicationModeApplied {
		return fmt.Errorf("%w: application mode must be shadow or applied", ErrInvalid)
	}
	switch record.ApplicationMode {
	case ApplicationModeShadow:
		if record.ScreeningEvidenceSHA256 != "" || record.ReleaseAuthoritySHA256 != "" {
			return fmt.Errorf("%w: shadow decisions cannot carry release bindings", ErrInvalid)
		}
	case ApplicationModeApplied:
		if !contentSHA256(record.ClipHash) || !contentSHA256(record.ScreeningEvidenceSHA256) ||
			!contentSHA256(record.ReleaseAuthoritySHA256) {
			return fmt.Errorf("%w: applied decisions require exact catalog, screening, and release identities", ErrInvalid)
		}
	}
	for name, value := range map[string]string{
		"id": record.ID, "clip hash": record.ClipHash, "evidence hash": record.EvidenceHash,
		"evidence version": record.EvidenceVersion, "policy version": record.PolicyVersion,
		"taxonomy version": record.TaxonomyVersion,
	} {
		if !boundedRequired(value, MaxIDBytes) {
			return fmt.Errorf("%w: %s is required and bounded", ErrInvalid, name)
		}
	}
	if record.SchemaVersion < 1 || record.CreatedAt.IsZero() || record.Kind() == "" {
		return fmt.Errorf("%w: schema, timestamp, and exactly one outcome are required", ErrInvalid)
	}
	payload, err := json.Marshal(record.Result)
	if err != nil || len(payload) > MaxResultBytes {
		return fmt.Errorf("%w: canonical result exceeds its bound", ErrInvalid)
	}
	if decision := record.Result.Decision; decision != nil {
		if len(decision.ReasonCodes) == 0 || !knownVerdict(decision.Verdict) {
			return fmt.Errorf("%w: semantic decision is incomplete", ErrInvalid)
		}
		question := strings.TrimSpace(decision.ReviewQuestion)
		if (decision.Verdict == filleradmission.VerdictReview) != (question != "") || !bounded(decision.ReviewQuestion, MaxTextBytes) {
			return fmt.Errorf("%w: review decisions require exactly one bounded question", ErrInvalid)
		}
	}
	if hold := record.Result.Hold; hold != nil {
		if hold.Code == "" || !bounded(hold.Detail, MaxTextBytes) {
			return fmt.Errorf("%w: operational hold is incomplete", ErrInvalid)
		}
	}
	return nil
}

func contentSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func recoveryFor(code filleradmission.OperationalCode, retryable bool) RecoveryAction {
	switch code {
	case filleradmission.HoldProviderUnavailable, filleradmission.HoldRouteUnavailable:
		return RecoveryConfigureProvider
	case filleradmission.HoldBudgetExhausted:
		return RecoveryAdjustBudget
	case filleradmission.HoldExtractionFailed:
		if retryable {
			return RecoveryRetryExtraction
		}
		return RecoveryInspectMedia
	default:
		return RecoveryUpdatePolicy
	}
}

func ValidateDiagnosticRecovery(request DiagnosticRecoveryRequest) error {
	if !boundedRequired(request.ID, MaxIDBytes) || !boundedRequired(request.DecisionID, MaxIDBytes) ||
		!boundedRequired(request.ActorID, MaxIDBytes) || request.Action != DiagnosticRecoveryRetry || request.CreatedAt.IsZero() {
		return fmt.Errorf("%w: diagnostic recovery request is incomplete", ErrInvalid)
	}
	return nil
}

func ValidateAction(action Action) error {
	for name, value := range map[string]string{
		"id": action.ID, "decision id": action.DecisionID, "actor id": action.ActorID,
	} {
		if !boundedRequired(value, MaxIDBytes) {
			return fmt.Errorf("%w: %s is required and bounded", ErrInvalid, name)
		}
	}
	if action.CreatedAt.IsZero() || !knownAction(action.Kind) ||
		!bounded(action.Reason, MaxTextBytes) || !bounded(action.Answer, MaxTextBytes) ||
		!bounded(action.SupersedesID, MaxIDBytes) {
		return fmt.Errorf("%w: action is incomplete or outside its bounds", ErrInvalid)
	}
	if action.Kind == ActionCorrect {
		if strings.TrimSpace(action.Answer) == "" ||
			(action.CorrectedVerdict != filleradmission.VerdictAdmit && action.CorrectedVerdict != filleradmission.VerdictReject) {
			return fmt.Errorf("%w: correction requires an answer and corrected verdict", ErrInvalid)
		}
	} else if action.CorrectedVerdict != "" {
		return fmt.Errorf("%w: only a correction carries a corrected verdict", ErrInvalid)
	}
	if (action.Kind == ActionRestore || action.Kind == ActionReverse) && strings.TrimSpace(action.Reason) == "" {
		return fmt.Errorf("%w: restore and reverse require an audit reason", ErrInvalid)
	}
	return nil
}

func knownVerdict(verdict filleradmission.Verdict) bool {
	return verdict == filleradmission.VerdictAdmit || verdict == filleradmission.VerdictReject || verdict == filleradmission.VerdictReview
}

func knownAction(kind ActionKind) bool {
	return kind == ActionAdmit || kind == ActionReject || kind == ActionCorrect ||
		kind == ActionAbandon || kind == ActionRestore || kind == ActionReverse
}

func boundedRequired(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && bounded(value, limit)
}

func bounded(value string, limit int) bool {
	return len(value) <= limit && utf8.ValidString(value)
}
