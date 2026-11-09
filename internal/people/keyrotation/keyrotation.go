// Package keyrotation is the operator's view of database secret encryption: whether it is active,
// and rotating the data key, which reseals every protected setting and every stored notification
// destination under the new key.
package keyrotation

import (
	"context"

	"github.com/loomarr/loomarr/internal/people/protectedsettings"
	"github.com/loomarr/loomarr/internal/people/secretprotection"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// Status is safe to render in diagnostics and the Security settings page.
type Status struct {
	Enabled                    bool
	InstallationKeyFingerprint string // non-secret
	DataKeyCount               int    // active and retained readable data keys
}

// Reencrypter reseals the secrets it owns under the current data key.
type Reencrypter interface {
	ReencryptAll(ctx context.Context) error
}

// Service reports encryption status and rotates the data key.
type Service struct {
	manager      *secretprotection.Manager
	settings     protectedsettings.Store
	destinations Reencrypter
}

// New returns the service over the manager, the settings store and the notification destinations
// that hold encrypted credentials.
func New(manager *secretprotection.Manager, settingsStore protectedsettings.Store, destinations Reencrypter) Service {
	return Service{manager: manager, settings: settingsStore, destinations: destinations}
}

// Status reports the installation-key fingerprint and the retained data-key count.
func (s Service) Status(ctx context.Context) (Status, error) {
	if s.manager == nil {
		return Status{}, nil
	}
	if err := s.manager.Refresh(ctx); err != nil {
		return Status{}, err
	}
	return Status{
		Enabled: true, InstallationKeyFingerprint: s.manager.Fingerprint(), DataKeyCount: s.manager.DataKeyCount(),
	}, nil
}

// RotateDataKey adds a fresh data key, then reseals the protected settings and the notification
// destinations under it.
func (s Service) RotateDataKey(ctx context.Context) error {
	if err := s.manager.RotateDataKey(ctx); err != nil {
		return err
	}
	if err := protectedsettings.ReencryptAll(ctx, s.settings, settings.NewRegistry(), s.manager); err != nil {
		return err
	}
	return s.destinations.ReencryptAll(ctx)
}
