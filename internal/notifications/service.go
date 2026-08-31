package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound = errors.New("notifications: not found")
	ErrConflict = errors.New("notifications: conflict")
)

type Repository interface {
	CreateNotificationIntent(context.Context, Intent, []Attempt) (Intent, bool, error)
	GetNotificationIntent(context.Context, string) (Intent, error)
	ClaimDueNotificationAttempt(context.Context, string, time.Time, time.Duration) (Attempt, error)
	CompleteNotificationAttempt(context.Context, Completion) error
}

// Router owns recipient policy and destination resolution. Domain callers do not select a means.
type Router interface {
	Routes(context.Context, Intent) ([]Route, error)
}

// Adapter executes one means. It must classify every result; arbitrary provider errors never cross
// into persistence. Secure render material may be derived from Delivery's domain references in memory.
type Adapter interface {
	Means() Means
	Deliver(context.Context, Delivery) Result
}

type Delivery struct {
	Intent  Intent
	Attempt Attempt
}

type Result struct {
	Status            Status
	ProviderMessageID string
	FailureClass      FailureClass
	OutcomeCode       OutcomeCode
}

type PublishCommand struct {
	Topic          Topic
	RecipientKind  RecipientKind
	RecipientID    string
	ReferenceKind  ReferenceKind
	ReferenceID    string
	Policy         RecipientPolicy
	Template       TemplateData
	IdempotencyKey string
}

type Service struct {
	repository Repository
	router     Router
	adapters   map[Means]Adapter
	newID      func() string
	now        func() time.Time
	lease      time.Duration
}

func NewService(
	repository Repository,
	router Router,
	adapters []Adapter,
	newID func() string,
	now func() time.Time,
) *Service {
	indexed := make(map[Means]Adapter, len(adapters))
	for _, adapter := range adapters {
		if adapter != nil {
			indexed[adapter.Means()] = adapter
		}
	}
	return &Service{
		repository: repository, router: router, adapters: indexed, newID: newID, now: now,
		lease: 5 * time.Minute,
	}
}

// Publish validates one typed intent, asks module-owned policy for routes, and atomically persists
// the intent plus initial attempts. The idempotency key makes caller retries return the same intent.
func (s *Service) Publish(ctx context.Context, command PublishCommand) (Intent, bool, error) {
	now := s.now().UTC().Truncate(time.Second)
	intent := Intent{
		ID: s.newID(), Topic: command.Topic, RecipientKind: command.RecipientKind,
		RecipientID: command.RecipientID, ReferenceKind: command.ReferenceKind,
		ReferenceID: command.ReferenceID, Policy: command.Policy, Template: command.Template,
		IdempotencyKey: command.IdempotencyKey, CreatedAt: now,
	}
	if err := intent.Validate(); err != nil {
		return Intent{}, false, err
	}
	routes, err := s.router.Routes(ctx, intent)
	if err != nil {
		return Intent{}, false, fmt.Errorf("route notification intent: %w", err)
	}
	if len(routes) == 0 {
		return Intent{}, false, fmt.Errorf("route notification intent: no delivery decision")
	}
	attempts := make([]Attempt, 0, len(routes))
	for _, route := range routes {
		if err := route.Validate(); err != nil {
			return Intent{}, false, fmt.Errorf("route notification intent: %w", err)
		}
		attempt := Attempt{
			ID: s.newID(), IntentID: intent.ID, Means: route.Means,
			DestinationRef: route.DestinationRef, DestinationRedacted: route.DestinationRedacted,
			Status: StatusQueued, AttemptNumber: 1, AvailableAt: now, CreatedAt: now,
		}
		if route.Suppressed != OutcomeNone {
			attempt.Status = StatusSuppressed
			attempt.OutcomeCode = route.Suppressed
			attempt.FinishedAt = now
		}
		if err := attempt.Validate(); err != nil {
			return Intent{}, false, fmt.Errorf("build notification attempt: %w", err)
		}
		attempts = append(attempts, attempt)
	}
	return s.repository.CreateNotificationIntent(ctx, intent, attempts)
}

// RunOne claims and executes at most one due attempt. A retry is created only for a failure known
// to precede remote acceptance; permanent, cancelled, and ambiguous outcomes are terminal.
func (s *Service) RunOne(ctx context.Context, owner string) (bool, error) {
	now := s.now().UTC().Truncate(time.Second)
	attempt, err := s.repository.ClaimDueNotificationAttempt(ctx, owner, now, s.lease)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	intent, err := s.repository.GetNotificationIntent(ctx, attempt.IntentID)
	if err != nil {
		return true, fmt.Errorf("load claimed notification intent: %w", err)
	}

	result := Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeMeansUnavailable}
	if adapter := s.adapters[attempt.Means]; adapter != nil {
		result = adapter.Deliver(ctx, Delivery{Intent: intent, Attempt: attempt})
	}
	if err := validateOutcome(result.Status, result.FailureClass, result.OutcomeCode, result.ProviderMessageID); err != nil {
		result = Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeConfigurationInvalid}
	}

	completion := Completion{
		AttemptID: attempt.ID, LeaseOwner: owner, Status: result.Status,
		ProviderMessageID: result.ProviderMessageID, FailureClass: result.FailureClass,
		OutcomeCode: result.OutcomeCode, FinishedAt: now,
	}
	if result.Status == StatusFailed && result.FailureClass == FailureTransientPreAcceptance {
		if delay, ok := RetryDelay(intent.ID, attempt.AttemptNumber+1); ok {
			next := Attempt{
				ID: s.newID(), IntentID: intent.ID, Means: attempt.Means,
				DestinationRef: attempt.DestinationRef, DestinationRedacted: attempt.DestinationRedacted,
				Status: StatusQueued, AttemptNumber: attempt.AttemptNumber + 1,
				AvailableAt: now.Add(delay), CreatedAt: now,
			}
			completion.Next = &next
		}
	}
	if err := s.repository.CompleteNotificationAttempt(ctx, completion); err != nil {
		return true, err
	}
	return true, nil
}
