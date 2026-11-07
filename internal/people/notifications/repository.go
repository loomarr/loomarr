package notifications

import (
	"context"
	"time"
)

// Repository is the one durable port the notifications module works through: the destinations a
// person configures, and the intents and attempts that deliver to them. The Service and the
// DestinationManager both depend on it. ProtectedRepository is its implementation: destination
// credentials are sealed and opened there, and nothing below it sees plaintext.
type Repository interface {
	SaveNotificationDestination(context.Context, Destination) error
	ResolveNotificationDestination(context.Context, string) (Destination, error)
	OpenNotificationDestinationForUpdate(context.Context, string) (Destination, error)
	GetNotificationDestinationMetadata(context.Context, string) (DestinationMetadata, error)
	ListNotificationDestinationMetadata(context.Context) ([]DestinationMetadata, error)
	ListNotificationDestinationHealth(context.Context) (map[string]DestinationHealth, error)
	DeleteNotificationDestination(context.Context, string) error
	RetireNotificationDestination(context.Context, string) error

	CreateNotificationIntent(context.Context, Intent, []Attempt) (Intent, bool, error)
	GetNotificationIntent(context.Context, string) (Intent, error)
	ListNotificationIntentsByReference(context.Context, ReferenceKind, string) ([]Intent, error)
	ListNotificationAttempts(context.Context, string) ([]Attempt, error)
	ClaimDueNotificationAttempt(context.Context, string, time.Time, time.Duration) (Attempt, error)
	CompleteNotificationAttempt(context.Context, Completion) error
}

// RecordRepository is the storage port below credential protection: destination records holding
// sealed credential envelopes, and the intents and attempts, which carry no credentials.
// Implementations must never receive or return plaintext provider credentials.
type RecordRepository interface {
	SaveNotificationDestinationRecord(context.Context, DestinationRecord) error
	GetNotificationDestinationRecord(context.Context, string) (DestinationRecord, error)
	ListNotificationDestinationRecords(context.Context) ([]DestinationRecord, error)
	ListNotificationDestinationHealth(context.Context) (map[string]DestinationHealth, error)
	DeleteNotificationDestination(context.Context, string) error

	CreateNotificationIntent(context.Context, Intent, []Attempt) (Intent, bool, error)
	GetNotificationIntent(context.Context, string) (Intent, error)
	ListNotificationIntentsByReference(context.Context, ReferenceKind, string) ([]Intent, error)
	ListNotificationAttempts(context.Context, string) ([]Attempt, error)
	ClaimDueNotificationAttempt(context.Context, string, time.Time, time.Duration) (Attempt, error)
	CompleteNotificationAttempt(context.Context, Completion) error
}
