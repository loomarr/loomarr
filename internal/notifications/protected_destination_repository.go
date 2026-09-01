package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/secretprotection"
)

// ProtectedDestinationRepository is the sole plaintext credential boundary between notification
// logic and durable storage. Its lower port accepts only authenticated encryption envelopes.
type ProtectedDestinationRepository struct {
	records    DestinationRecordRepository
	protection *secretprotection.Manager
}

func NewProtectedDestinationRepository(
	records DestinationRecordRepository,
	protection *secretprotection.Manager,
) *ProtectedDestinationRepository {
	if records == nil || protection == nil {
		return nil
	}
	return &ProtectedDestinationRepository{records: records, protection: protection}
}

func (r *ProtectedDestinationRepository) SaveNotificationDestination(
	ctx context.Context,
	destination Destination,
) error {
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
	return r.records.SaveNotificationDestinationRecord(ctx, destinationRecord(destination, envelope))
}

func (r *ProtectedDestinationRepository) GetNotificationDestination(
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

func (r *ProtectedDestinationRepository) ListNotificationDestinations(ctx context.Context) ([]Destination, error) {
	if r == nil || r.records == nil || r.protection == nil {
		return nil, errors.New("notification destination credential protection is unavailable")
	}
	if err := r.protection.Refresh(ctx); err != nil {
		return nil, err
	}
	records, err := r.records.ListNotificationDestinationRecords(ctx)
	if err != nil {
		return nil, err
	}
	destinations := make([]Destination, 0, len(records))
	for _, record := range records {
		destination, err := r.open(ctx, record, false)
		if err != nil {
			return nil, err
		}
		destinations = append(destinations, destination)
	}
	return destinations, nil
}

func (r *ProtectedDestinationRepository) ListNotificationDestinationHealth(
	ctx context.Context,
) (map[string]DestinationHealth, error) {
	return r.records.ListNotificationDestinationHealth(ctx)
}

func (r *ProtectedDestinationRepository) DeleteNotificationDestination(ctx context.Context, id string) error {
	return r.records.DeleteNotificationDestination(ctx, id)
}

// ReencryptAll reseals each credential object with the active data key after rotation. Older keys
// remain readable, so an interrupted pass is safe to retry and never requires plaintext storage.
func (r *ProtectedDestinationRepository) ReencryptAll(ctx context.Context) error {
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

func (r *ProtectedDestinationRepository) open(
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
	if err := destination.Validate(); err != nil {
		return Destination{}, fmt.Errorf("validate notification destination %q: %w", record.ID, err)
	}
	return destination, nil
}

func destinationCredentialRecord(id string) secretprotection.Record {
	return secretprotection.Record{Kind: "notification_destination", ID: id, Field: "credentials"}
}

func destinationRecord(destination Destination, envelope string) DestinationRecord {
	return DestinationRecord{
		ID: destination.ID, Means: destination.Means, Label: destination.Label, Scope: destination.Scope,
		OwnerID: destination.OwnerID, Audience: destination.Audience, Topics: append([]Topic(nil), destination.Topics...),
		Enabled: destination.Enabled, Configuration: cloneStringMap(destination.Configuration),
		CredentialsEncrypted: envelope, CreatedAt: destination.CreatedAt, UpdatedAt: destination.UpdatedAt,
	}
}

func destinationFromRecord(record DestinationRecord) Destination {
	return Destination{
		ID: record.ID, Means: record.Means, Label: record.Label, Scope: record.Scope,
		OwnerID: record.OwnerID, Audience: record.Audience, Topics: append([]Topic(nil), record.Topics...),
		Enabled: record.Enabled, Configuration: cloneStringMap(record.Configuration),
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
