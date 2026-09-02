package testkit

import (
	"context"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
)

// NotificationRepository is the shared in-memory delivery-work double. It models only the
// repository contract needed by notification service tests; SQL behavior belongs to conformance.
type NotificationRepository struct {
	Destinations map[string]notifications.Destination
	Intents      map[string]notifications.Intent
	ByKey        map[string]string
	Attempts     map[string]notifications.Attempt
	Completions  []notifications.Completion
}

func NewNotificationRepository() *NotificationRepository {
	return &NotificationRepository{
		Destinations: make(map[string]notifications.Destination),
		Intents:      make(map[string]notifications.Intent),
		ByKey:        make(map[string]string),
		Attempts:     make(map[string]notifications.Attempt),
	}
}

func (f *NotificationRepository) SaveNotificationDestination(
	_ context.Context,
	destination notifications.Destination,
) error {
	if err := destination.Validate(); err != nil {
		return err
	}
	if destination.SubscriptionFingerprint != "" {
		for id, current := range f.Destinations {
			if id != destination.ID && current.Means == destination.Means && current.OwnerID == destination.OwnerID &&
				current.SubscriptionFingerprint == destination.SubscriptionFingerprint {
				return notifications.ErrConflict
			}
		}
	}
	if current, ok := f.Destinations[destination.ID]; ok {
		destination.CreatedAt = current.CreatedAt
	}
	f.Destinations[destination.ID] = destination
	return nil
}

func (f *NotificationRepository) ResolveNotificationDestination(
	_ context.Context,
	id string,
) (notifications.Destination, error) {
	destination, ok := f.Destinations[id]
	if !ok {
		return notifications.Destination{}, notifications.ErrNotFound
	}
	return destination, nil
}

func (f *NotificationRepository) OpenNotificationDestinationForUpdate(
	ctx context.Context,
	id string,
) (notifications.Destination, error) {
	return f.ResolveNotificationDestination(ctx, id)
}

func (f *NotificationRepository) GetNotificationDestinationMetadata(
	ctx context.Context,
	id string,
) (notifications.DestinationMetadata, error) {
	destination, err := f.ResolveNotificationDestination(ctx, id)
	if err != nil {
		return notifications.DestinationMetadata{}, err
	}
	return destination.Metadata(), nil
}

func (f *NotificationRepository) ListNotificationDestinationMetadata(
	_ context.Context,
) ([]notifications.DestinationMetadata, error) {
	metadata := make([]notifications.DestinationMetadata, 0, len(f.Destinations))
	for _, destination := range f.Destinations {
		metadata = append(metadata, destination.Metadata())
	}
	sort.Slice(metadata, func(i, j int) bool { return metadata[i].ID < metadata[j].ID })
	return metadata, nil
}

func (f *NotificationRepository) ListNotificationDestinationHealth(
	_ context.Context,
) (map[string]notifications.DestinationHealth, error) {
	health := make(map[string]notifications.DestinationHealth, len(f.Destinations))
	for id := range f.Destinations {
		value := notifications.DestinationHealth{}
		for _, attempt := range f.Attempts {
			if attempt.DestinationRef != id {
				continue
			}
			switch attempt.Status {
			case notifications.StatusQueued:
				value.QueuedCount++
			case notifications.StatusDelivered:
				if attempt.FinishedAt.After(value.LastSuccessAt) {
					value.LastSuccessAt = attempt.FinishedAt
				}
			case notifications.StatusFailed:
				value.TerminalFailureCount++
				if attempt.FinishedAt.After(value.LastFailureAt) {
					value.LastFailureAt = attempt.FinishedAt
					value.LastFailureOutcome = attempt.OutcomeCode
				}
			}
		}
		health[id] = value
	}
	return health, nil
}

func (f *NotificationRepository) DeleteNotificationDestination(_ context.Context, id string) error {
	if _, ok := f.Destinations[id]; !ok {
		return notifications.ErrNotFound
	}
	delete(f.Destinations, id)
	return nil
}

func (f *NotificationRepository) RetireNotificationDestination(_ context.Context, id string) error {
	destination, ok := f.Destinations[id]
	if !ok {
		return notifications.ErrNotFound
	}
	destination.Enabled = false
	f.Destinations[id] = destination
	return nil
}

func (f *NotificationRepository) CreateNotificationIntent(
	_ context.Context,
	intent notifications.Intent,
	attempts []notifications.Attempt,
) (notifications.Intent, bool, error) {
	if id := f.ByKey[intent.IdempotencyKey]; id != "" {
		return f.Intents[id], false, nil
	}
	allTerminal := true
	for _, attempt := range attempts {
		f.Attempts[attempt.ID] = attempt
		if attempt.Status == notifications.StatusQueued || attempt.Status == notifications.StatusSending {
			allTerminal = false
		}
	}
	if allTerminal {
		intent.TerminalAt = intent.CreatedAt
	}
	f.Intents[intent.ID] = intent
	f.ByKey[intent.IdempotencyKey] = intent.ID
	return intent, true, nil
}

func (f *NotificationRepository) GetNotificationIntent(
	_ context.Context,
	id string,
) (notifications.Intent, error) {
	intent, ok := f.Intents[id]
	if !ok {
		return notifications.Intent{}, notifications.ErrNotFound
	}
	return intent, nil
}

func (f *NotificationRepository) ListNotificationIntentsByReference(
	_ context.Context,
	kind notifications.ReferenceKind,
	id string,
) ([]notifications.Intent, error) {
	var intents []notifications.Intent
	for _, intent := range f.Intents {
		if intent.ReferenceKind == kind && intent.ReferenceID == id {
			intents = append(intents, intent)
		}
	}
	sort.Slice(intents, func(i, j int) bool {
		if intents[i].CreatedAt.Equal(intents[j].CreatedAt) {
			return intents[i].ID > intents[j].ID
		}
		return intents[i].CreatedAt.After(intents[j].CreatedAt)
	})
	return intents, nil
}

func (f *NotificationRepository) ListNotificationAttempts(
	_ context.Context,
	intentID string,
) ([]notifications.Attempt, error) {
	var attempts []notifications.Attempt
	for _, attempt := range f.Attempts {
		if attempt.IntentID == intentID {
			attempts = append(attempts, attempt)
		}
	}
	sort.Slice(attempts, func(i, j int) bool {
		if attempts[i].AttemptNumber == attempts[j].AttemptNumber {
			return attempts[i].ID < attempts[j].ID
		}
		return attempts[i].AttemptNumber < attempts[j].AttemptNumber
	})
	return attempts, nil
}

func (f *NotificationRepository) ClaimDueNotificationAttempt(
	_ context.Context,
	owner string,
	now time.Time,
	lease time.Duration,
) (notifications.Attempt, error) {
	ids := make([]string, 0, len(f.Attempts))
	for id := range f.Attempts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		attempt := f.Attempts[id]
		if attempt.Status == notifications.StatusQueued && !attempt.AvailableAt.After(now) {
			attempt.Status = notifications.StatusSending
			attempt.LeaseOwner = owner
			attempt.LeaseExpiresAt = now.Add(lease)
			attempt.StartedAt = now
			f.Attempts[id] = attempt
			return attempt, nil
		}
	}
	return notifications.Attempt{}, notifications.ErrNotFound
}

func (f *NotificationRepository) CompleteNotificationAttempt(
	_ context.Context,
	completion notifications.Completion,
) error {
	attempt, ok := f.Attempts[completion.AttemptID]
	if !ok || attempt.Status != notifications.StatusSending || attempt.LeaseOwner != completion.LeaseOwner {
		return notifications.ErrConflict
	}
	attempt.Status = completion.Status
	attempt.ProviderMessageID = completion.ProviderMessageID
	attempt.FailureClass = completion.FailureClass
	attempt.OutcomeCode = completion.OutcomeCode
	attempt.FinishedAt = completion.FinishedAt
	attempt.LeaseOwner = ""
	attempt.LeaseExpiresAt = time.Time{}
	f.Attempts[attempt.ID] = attempt
	if completion.Next != nil {
		f.Attempts[completion.Next.ID] = *completion.Next
	}
	f.Completions = append(f.Completions, completion)
	return nil
}

type NotificationRouter struct {
	RoutesResult []notifications.Route
	Err          error
}

func (r NotificationRouter) Routes(context.Context, notifications.Intent) ([]notifications.Route, error) {
	return r.RoutesResult, r.Err
}

type NotificationAdapter struct {
	DeliveryMeans notifications.Means
	Result        notifications.Result
	Calls         []notifications.Delivery
}

func (a *NotificationAdapter) Means() notifications.Means { return a.DeliveryMeans }

func (a *NotificationAdapter) Deliver(
	_ context.Context,
	delivery notifications.Delivery,
) notifications.Result {
	a.Calls = append(a.Calls, delivery)
	return a.Result
}
