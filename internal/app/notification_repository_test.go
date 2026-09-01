package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/store"
)

func notificationRepositoryForTest(t *testing.T, st store.Store) notifications.Repository {
	t.Helper()
	protection, err := buildSecretProtection(t.Context(), st, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return notificationServiceRepository{
		Store:        st,
		destinations: notifications.NewProtectedDestinationRepository(st, protection),
	}
}
