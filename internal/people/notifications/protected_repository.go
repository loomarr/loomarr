package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/people/secretprotection"
)

// ProtectedRepository implements Repository and is the sole plaintext credential boundary
// between notification logic and durable storage. Its lower port accepts only authenticated
// encryption envelopes; intents and attempts carry no credentials and pass straight through.
type ProtectedRepository struct {
	records    RecordRepository
	protection *secretprotection.Manager
	redactor   CredentialRedactor
}

// CredentialRedactor is the systemic sink for plaintext provider credentials. The protected
// repository is the only component that can register these values because it is the only durable
// boundary permitted to open their encryption envelopes.
type CredentialRedactor interface {
	SetSource(string, []string)
}

const notificationCredentialRedactionSource = "notification-destination:"

func (r *ProtectedRepository) ListNotificationDestinationMetadata(
	ctx context.Context,
) ([]DestinationMetadata, error) {
	if r == nil || r.records == nil {
		return nil, errors.New("notification destination repository is unavailable")
	}
	records, err := r.records.ListNotificationDestinationRecords(ctx)
	if err != nil {
		return nil, err
	}
	metadata := make([]DestinationMetadata, 0, len(records))
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return nil, fmt.Errorf("validate notification destination metadata %q: %w", record.ID, err)
		}
		metadata = append(metadata, destinationMetadataFromRecord(record))
	}
	return metadata, nil
}

func NewProtectedRepository(
	records RecordRepository,
	protection *secretprotection.Manager,
) *ProtectedRepository {
	if records == nil || protection == nil {
		return nil
	}
	return &ProtectedRepository{records: records, protection: protection}
}

// WithCredentialRedactor registers the systemic sink that must observe every plaintext value.
func (r *ProtectedRepository) WithCredentialRedactor(
	redactor CredentialRedactor,
) *ProtectedRepository {
	if r != nil {
		r.redactor = redactor
	}
	return r
}

// SeedCredentialRedaction opens every stored credential envelope during startup so the systemic
// logger is safe before provider delivery begins. Normal metadata reads remain credential-free.
func (r *ProtectedRepository) SeedCredentialRedaction(ctx context.Context) error {
	if r == nil || r.records == nil || r.protection == nil {
		return errors.New("notification destination credential protection is unavailable")
	}
	if r.redactor == nil {
		return errors.New("notification destination credential redactor is unavailable")
	}
	if err := r.protection.Refresh(ctx); err != nil {
		return err
	}
	records, err := r.records.ListNotificationDestinationRecords(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if _, err := r.open(ctx, record, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *ProtectedRepository) SaveNotificationDestination(
	ctx context.Context,
	destination Destination,
) error {
	setWebPushSubscriptionFingerprint(&destination)
	if err := destination.Validate(); err != nil {
		return fmt.Errorf("validate notification destination: %w", err)
	}
	if r == nil || r.records == nil || r.protection == nil {
		return errors.New("notification destination credential protection is unavailable")
	}
	if err := r.protection.Refresh(ctx); err != nil {
		return err
	}
	credentials := destination.Credentials
	if credentials == nil {
		credentials = map[string]string{}
	}
	plain, err := json.Marshal(credentials)
	if err != nil {
		return fmt.Errorf("encode notification destination credentials: %w", err)
	}
	envelope, err := r.protection.Seal(destinationCredentialRecord(destination.ID), plain)
	if err != nil {
		return fmt.Errorf("protect notification destination credentials: %w", err)
	}
	if err := r.records.SaveNotificationDestinationRecord(ctx, destinationRecord(destination, envelope)); err != nil {
		return err
	}
	r.registerCredentials(destination)
	return nil
}

// ResolveNotificationDestination opens the complete destination for a claimed delivery attempt.
func (r *ProtectedRepository) ResolveNotificationDestination(
	ctx context.Context,
	id string,
) (Destination, error) {
	if r == nil || r.records == nil || r.protection == nil {
		return Destination{}, errors.New("notification destination credential protection is unavailable")
	}
	record, err := r.records.GetNotificationDestinationRecord(ctx, id)
	if err != nil {
		return Destination{}, err
	}
	return r.open(ctx, record, true)
}

// OpenNotificationDestinationForUpdate is the explicit management-mutation boundary. Updating a
// partial settings object must merge it with the existing sealed credential object before resealing.
func (r *ProtectedRepository) OpenNotificationDestinationForUpdate(
	ctx context.Context,
	id string,
) (Destination, error) {
	return r.ResolveNotificationDestination(ctx, id)
}

func (r *ProtectedRepository) GetNotificationDestinationMetadata(
	ctx context.Context,
	id string,
) (DestinationMetadata, error) {
	if r == nil || r.records == nil {
		return DestinationMetadata{}, errors.New("notification destination repository is unavailable")
	}
	record, err := r.records.GetNotificationDestinationRecord(ctx, id)
	if err != nil {
		return DestinationMetadata{}, err
	}
	if err := record.Validate(); err != nil {
		return DestinationMetadata{}, fmt.Errorf("validate notification destination metadata %q: %w", record.ID, err)
	}
	return destinationMetadataFromRecord(record), nil
}

func (r *ProtectedRepository) ListNotificationDestinationHealth(
	ctx context.Context,
) (map[string]DestinationHealth, error) {
	return r.records.ListNotificationDestinationHealth(ctx)
}

func (r *ProtectedRepository) DeleteNotificationDestination(ctx context.Context, id string) error {
	if err := r.records.DeleteNotificationDestination(ctx, id); err != nil {
		return err
	}
	if r.redactor != nil {
		r.redactor.SetSource(notificationCredentialRedactionSource+id, nil)
	}
	return nil
}

func (r *ProtectedRepository) RetireNotificationDestination(ctx context.Context, id string) error {
	if r == nil || r.records == nil {
		return errors.New("notification destination repository is unavailable")
	}
	record, err := r.records.GetNotificationDestinationRecord(ctx, id)
	if err != nil {
		return err
	}
	if !record.Enabled {
		return nil
	}
	record.Enabled = false
	record.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if record.UpdatedAt.Before(record.CreatedAt) {
		record.UpdatedAt = record.CreatedAt
	}
	return r.records.SaveNotificationDestinationRecord(ctx, record)
}

var _ Repository = (*ProtectedRepository)(nil)

// Intents and attempts reference destinations by ID and redacted label only, so they pass through
// to storage without touching credential protection.
func (r *ProtectedRepository) CreateNotificationIntent(
	ctx context.Context,
	intent Intent,
	attempts []Attempt,
) (Intent, bool, error) {
	return r.records.CreateNotificationIntent(ctx, intent, attempts)
}

func (r *ProtectedRepository) GetNotificationIntent(ctx context.Context, id string) (Intent, error) {
	return r.records.GetNotificationIntent(ctx, id)
}

func (r *ProtectedRepository) ListNotificationIntentsByReference(
	ctx context.Context,
	kind ReferenceKind,
	id string,
) ([]Intent, error) {
	return r.records.ListNotificationIntentsByReference(ctx, kind, id)
}

func (r *ProtectedRepository) ListNotificationAttempts(ctx context.Context, intentID string) ([]Attempt, error) {
	return r.records.ListNotificationAttempts(ctx, intentID)
}

func (r *ProtectedRepository) ClaimDueNotificationAttempt(
	ctx context.Context,
	owner string,
	now time.Time,
	lease time.Duration,
) (Attempt, error) {
	return r.records.ClaimDueNotificationAttempt(ctx, owner, now, lease)
}

func (r *ProtectedRepository) CompleteNotificationAttempt(ctx context.Context, completion Completion) error {
	return r.records.CompleteNotificationAttempt(ctx, completion)
}

// ReencryptAll reseals each credential object with the active data key after rotation. Older keys
// remain readable, so an interrupted pass is safe to retry and never requires plaintext storage.
func (r *ProtectedRepository) ReencryptAll(ctx context.Context) error {
	if r == nil || r.records == nil || r.protection == nil {
		return errors.New("notification destination credential protection is unavailable")
	}
	if err := r.protection.Refresh(ctx); err != nil {
		return err
	}
	records, err := r.records.ListNotificationDestinationRecords(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		plain, err := r.protection.Open(destinationCredentialRecord(record.ID), record.CredentialsEncrypted)
		if err != nil {
			return fmt.Errorf("open notification destination %q for data-key rotation: %w", record.ID, err)
		}
		envelope, err := r.protection.Seal(destinationCredentialRecord(record.ID), plain)
		if err != nil {
			return fmt.Errorf("reseal notification destination %q after data-key rotation: %w", record.ID, err)
		}
		record.CredentialsEncrypted = envelope
		if err := r.records.SaveNotificationDestinationRecord(ctx, record); err != nil {
			return fmt.Errorf("commit data-key rotation for notification destination %q: %w", record.ID, err)
		}
	}
	return nil
}

func (r *ProtectedRepository) open(
	ctx context.Context,
	record DestinationRecord,
	refreshOnUnknownKey bool,
) (Destination, error) {
	var (
		plain []byte
		err   error
	)
	if refreshOnUnknownKey {
		plain, err = r.protection.OpenLatest(ctx, destinationCredentialRecord(record.ID), record.CredentialsEncrypted)
	} else {
		plain, err = r.protection.Open(destinationCredentialRecord(record.ID), record.CredentialsEncrypted)
	}
	if err != nil {
		return Destination{}, fmt.Errorf("open notification destination credentials %q: %w", record.ID, err)
	}
	destination := destinationFromRecord(record)
	if err := json.Unmarshal(plain, &destination.Credentials); err != nil {
		return Destination{}, fmt.Errorf("decode notification destination credentials %q: %w", record.ID, err)
	}
	if destination.Means == MeansWebPush && destination.SubscriptionFingerprint != "" && !sameFingerprint(
		destination.SubscriptionFingerprint,
		webPushSubscriptionFingerprint(destination.ProviderSettings().secret(keyEndpoint)),
	) {
		return Destination{}, fmt.Errorf("validate notification destination %q: subscription fingerprint mismatch", record.ID)
	}
	if err := destination.Validate(); err != nil {
		return Destination{}, fmt.Errorf("validate notification destination %q: %w", record.ID, err)
	}
	r.registerCredentials(destination)
	return destination, nil
}

func (r *ProtectedRepository) registerCredentials(destination Destination) {
	if r == nil || r.redactor == nil {
		return
	}
	values := make([]string, 0, len(destination.Credentials))
	for _, value := range destination.Credentials {
		if value != "" {
			values = append(values, value)
		}
	}
	r.redactor.SetSource(notificationCredentialRedactionSource+destination.ID, values)
}

func destinationCredentialRecord(id string) secretprotection.Record {
	return secretprotection.Record{Kind: "notification_destination", ID: id, Field: "credentials"}
}

func destinationRecord(destination Destination, envelope string) DestinationRecord {
	credentialKeys := make([]string, 0, len(destination.Credentials))
	for key, value := range destination.Credentials {
		if value != "" {
			credentialKeys = append(credentialKeys, key)
		}
	}
	sort.Strings(credentialKeys)
	return DestinationRecord{
		ID: destination.ID, Means: destination.Means, Label: destination.Label, Scope: destination.Scope,
		OwnerID: destination.OwnerID, Audience: destination.Audience, Topics: append([]Topic(nil), destination.Topics...),
		Enabled: destination.Enabled, Configuration: cloneStringMap(destination.Configuration), CredentialKeys: credentialKeys,
		CredentialsEncrypted: envelope, SubscriptionFingerprint: destination.SubscriptionFingerprint,
		CreatedAt: destination.CreatedAt, UpdatedAt: destination.UpdatedAt,
	}
}

func destinationMetadataFromRecord(record DestinationRecord) DestinationMetadata {
	credentialKeys := append([]string(nil), record.CredentialKeys...)
	if record.CredentialKeys == nil {
		// Rows written before credential metadata was added cannot reveal which optional secrets
		// were present without opening the envelope. Report all defined secret fields as configured
		// until the next explicit update rewrites exact metadata; never decrypt during a read.
		if definition, ok := ProviderDefinitionFor(record.Means); ok {
			for _, field := range definition.Fields {
				if field.Sensitive {
					credentialKeys = append(credentialKeys, field.Key)
				}
			}
		}
	}
	return DestinationMetadata{
		ID: record.ID, Means: record.Means, Label: record.Label, Scope: record.Scope,
		OwnerID: record.OwnerID, Audience: record.Audience, Topics: append([]Topic(nil), record.Topics...),
		Enabled: record.Enabled, Configuration: cloneStringMap(record.Configuration),
		CredentialKeys: credentialKeys, SubscriptionFingerprint: record.SubscriptionFingerprint,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func destinationFromRecord(record DestinationRecord) Destination {
	return Destination{
		ID: record.ID, Means: record.Means, Label: record.Label, Scope: record.Scope,
		OwnerID: record.OwnerID, Audience: record.Audience, Topics: append([]Topic(nil), record.Topics...),
		Enabled: record.Enabled, Configuration: cloneStringMap(record.Configuration),
		SubscriptionFingerprint: record.SubscriptionFingerprint,
		CreatedAt:               record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
