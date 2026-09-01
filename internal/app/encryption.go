package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/secretprotection"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
)

type encryptionService struct {
	manager      *secretprotection.Manager
	store        store.Store
	destinations *notifications.ProtectedDestinationRepository
}

func buildEncryptionService(st store.Store, manager *secretprotection.Manager) api.EncryptionService {
	if st == nil || manager == nil {
		return nil
	}
	return encryptionService{
		manager: manager, store: st,
		destinations: notifications.NewProtectedDestinationRepository(st, manager),
	}
}

func (s encryptionService) Status(ctx context.Context) (api.EncryptionStatus, error) {
	if s.manager == nil {
		return api.EncryptionStatus{}, nil
	}
	if err := s.manager.Refresh(ctx); err != nil {
		return api.EncryptionStatus{}, err
	}
	return api.EncryptionStatus{
		Enabled: true, InstallationKeyFingerprint: s.manager.Fingerprint(), DataKeyCount: s.manager.DataKeyCount(),
	}, nil
}

func (s encryptionService) RotateDataKey(ctx context.Context) error {
	if err := s.manager.RotateDataKey(ctx); err != nil {
		return err
	}
	if err := reencryptProtectedSettings(ctx, s.store, settings.NewRegistry(), s.manager); err != nil {
		return err
	}
	destinations := s.destinations
	if destinations == nil {
		destinations = notifications.NewProtectedDestinationRepository(s.store, s.manager)
	}
	return destinations.ReencryptAll(ctx)
}
