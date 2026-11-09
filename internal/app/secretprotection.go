package app

import (
	"context"
	"log/slog"

	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/people/keyrotation"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/people/secretprotection"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// buildSecretProtection opens the secret-protection manager over the store, locating the
// installation key from the data directory and the SQLite file's path.
func buildSecretProtection(ctx context.Context, st store.Store, dataDir string, log *slog.Logger) (*secretprotection.Manager, error) {
	return secretprotection.Boot(ctx, st, secretprotection.BootOptions{
		DataDir: dataDir, DatabasePath: store.SQLitePath(st), Log: log,
	})
}

// encryptionAPI implements system.EncryptionService over the key-rotation service, mapping its
// status to the API's.
type encryptionAPI struct{ service keyrotation.Service }

func buildEncryptionService(st store.Store, manager *secretprotection.Manager) system.EncryptionService {
	if st == nil || manager == nil {
		return nil
	}
	return encryptionAPI{service: keyrotation.New(manager, st, notifications.NewProtectedRepository(st, manager))}
}

func (e encryptionAPI) Status(ctx context.Context) (system.EncryptionStatus, error) {
	status, err := e.service.Status(ctx)
	if err != nil {
		return system.EncryptionStatus{}, err
	}
	return system.EncryptionStatus(status), nil
}

func (e encryptionAPI) RotateDataKey(ctx context.Context) error {
	return e.service.RotateDataKey(ctx)
}
