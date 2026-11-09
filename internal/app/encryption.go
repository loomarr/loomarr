package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/people/secretprotection"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

type encryptionService struct {
	manager      *secretprotection.Manager
	store        store.Store
	destinations *notifications.ProtectedRepository
}

func buildEncryptionService(st store.Store, manager *secretprotection.Manager) system.EncryptionService {
	if st == nil || manager == nil {
		return nil
	}
	return encryptionService{
		manager: manager, store: st,
		destinations: notifications.NewProtectedRepository(st, manager),
	}
}

func (s encryptionService) Status(ctx context.Context) (system.EncryptionStatus, error) {
	if s.manager == nil {
		return system.EncryptionStatus{}, nil
	}
	if err := s.manager.Refresh(ctx); err != nil {
		return system.EncryptionStatus{}, err
	}
	return system.EncryptionStatus{
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
		destinations = notifications.NewProtectedRepository(s.store, s.manager)
	}
	return destinations.ReencryptAll(ctx)
}
