package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("notifications: not found")
	ErrConflict = errors.New("notifications: conflict")
)

type Repository interface {
	GetNotificationDestination(context.Context, string) (Destination, error)
	CreateNotificationIntent(context.Context, Intent, []Attempt) (Intent, bool, error)
	GetNotificationIntent(context.Context, string) (Intent, error)
	ListNotificationIntentsByReference(context.Context, ReferenceKind, string) ([]Intent, error)
	ListNotificationAttempts(context.Context, string) ([]Attempt, error)
	ClaimDueNotificationAttempt(context.Context, string, time.Time, time.Duration) (Attempt, error)
	CompleteNotificationAttempt(context.Context, Completion) error
}

type DestinationRetirer interface {
	RetireNotificationDestination(context.Context, string) error
}

// LatestDelivery returns the newest explicit delivery request and its newest attempt. Invitation
// lifecycle remains owned by invitation; this read model describes only the effort to convey it.
func (s *Service) LatestDelivery(
	ctx context.Context,
	referenceKind ReferenceKind,
	referenceID string,
) (DeliverySummary, error) {
	intents, err := s.repository.ListNotificationIntentsByReference(ctx, referenceKind, referenceID)
	if err != nil {
		return DeliverySummary{}, err
	}
	if len(intents) == 0 {
		return DeliverySummary{}, ErrNotFound
	}
	attempts, err := s.repository.ListNotificationAttempts(ctx, intents[0].ID)
	if err != nil {
		return DeliverySummary{}, err
	}
	if len(attempts) == 0 {
		return DeliverySummary{}, ErrNotFound
	}
	latest := attempts[len(attempts)-1]
	updatedAt := latest.CreatedAt
	if !latest.StartedAt.IsZero() {
		updatedAt = latest.StartedAt
	}
	if !latest.FinishedAt.IsZero() {
		updatedAt = latest.FinishedAt
	}
	return DeliverySummary{
		Status: latest.Status, AttemptNumber: latest.AttemptNumber,
		OutcomeCode: latest.OutcomeCode, ProviderMessageID: latest.ProviderMessageID,
		UpdatedAt: updatedAt,
	}, nil
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
	Intent      Intent
	Attempt     Attempt
	Destination *Destination
}

type Result struct {
	Status            Status
	ProviderMessageID string
	FailureClass      FailureClass
	OutcomeCode       OutcomeCode
	// RetryAfter is an adapter-supplied lower bound for the next attempt. It is meaningful only
	// for definitely pre-acceptance transient failures and is clamped by the service policy.
	RetryAfter time.Duration
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
	return s.publishWithRoutes(ctx, intent, routes, now)
}

// PublishDestinationTest records a test as its own intent and routes it directly to the selected
// destination. It never fabricates a proposal, acquisition, or channel transition and its return
// value means only that the durable handoff was accepted.
func (s *Service) PublishDestinationTest(
	ctx context.Context,
	destination Destination,
	requestID string,
) (DestinationTestResult, error) {
	if err := destination.Validate(); err != nil {
		return DestinationTestResult{}, err
	}
	if !destination.Enabled {
		return DestinationTestResult{}, fmt.Errorf("notification destination must be enabled before testing")
	}
	if err := identifier("destination test request id", requestID); err != nil {
		return DestinationTestResult{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	intent := Intent{
		ID: s.newID(), Topic: TopicDeliveryTest, RecipientKind: destination.Audience,
		RecipientID: destination.ID, ReferenceKind: ReferenceDestination, ReferenceID: destination.ID,
		Policy: PolicyConfigurable, Template: TemplateData{SubjectName: "Test notification", Summary: "Loomarr notification destination test."},
		IdempotencyKey: "notification:destination-test:" + destination.ID + ":" + requestID, CreatedAt: now,
	}
	if err := intent.Validate(); err != nil {
		return DestinationTestResult{}, err
	}
	stored, created, err := s.publishWithRoutes(ctx, intent, []Route{{
		Means: destination.Means, DestinationRef: destination.ID, DestinationRedacted: destination.Label,
	}}, now)
	if err != nil {
		return DestinationTestResult{}, err
	}
	return DestinationTestResult{IntentID: stored.ID, Created: created}, nil
}

func (s *Service) publishWithRoutes(ctx context.Context, intent Intent, routes []Route, now time.Time) (Intent, bool, error) {
	if len(routes) == 0 {
		if intent.Policy != PolicyConfigurable {
			return Intent{}, false, fmt.Errorf("route notification intent: no delivery decision")
		}
		return s.repository.CreateNotificationIntent(ctx, intent, nil)
	}
	attempts := make([]Attempt, 0, len(routes))
	for _, route := range routes {
		if err := route.Validate(); err != nil {
			return Intent{}, false, fmt.Errorf("route notification intent: %w", err)
		}
		if intent.Policy == PolicyMandatoryAccount && route.Means != MeansEmail {
			return Intent{}, false, fmt.Errorf("route notification intent: mandatory account delivery requires email")
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

	delivery := Delivery{Intent: intent, Attempt: attempt}
	result := Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeMeansUnavailable}
	destinationID := attempt.DestinationRef
	usesConfiguredDestination := intent.Policy == PolicyConfigurable
	if intent.Policy == PolicyMandatoryAccount && strings.HasPrefix(destinationID, "provider:") {
		usesConfiguredDestination = true
		destinationID = strings.TrimPrefix(destinationID, "provider:")
	}
	if usesConfiguredDestination {
		destination, destinationErr := s.repository.GetNotificationDestination(ctx, destinationID)
		if errors.Is(destinationErr, ErrNotFound) || (destinationErr == nil && !destination.Enabled) {
			result = Result{Status: StatusSuppressed, OutcomeCode: OutcomeDestinationUnavailable}
		} else if destinationErr != nil {
			return true, fmt.Errorf("load notification destination: %w", destinationErr)
		} else if destination.Means != attempt.Means {
			result = Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeConfigurationInvalid}
		} else {
			delivery.Destination = &destination
			if adapter := s.adapters[attempt.Means]; adapter != nil {
				result = adapter.Deliver(ctx, delivery)
			}
		}
	} else if adapter := s.adapters[attempt.Means]; adapter != nil {
		result = adapter.Deliver(ctx, delivery)
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
			if hint := boundedProviderRetryAfter(result.RetryAfter); hint > delay {
				delay = hint
			}
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
	if attempt.Means == MeansWebPush && result.OutcomeCode == OutcomeDestinationUnavailable {
		if retirer, ok := s.repository.(DestinationRetirer); ok {
			if err := retirer.RetireNotificationDestination(ctx, destinationID); err != nil && !errors.Is(err, ErrNotFound) {
				return true, fmt.Errorf("retire unavailable Web Push destination: %w", err)
			}
		}
	}
	return true, nil
}

func boundedProviderRetryAfter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	const maximum = 2 * time.Hour
	if delay > maximum {
		return maximum
	}
	return delay
}
