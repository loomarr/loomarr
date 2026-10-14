package app

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/store"
)

func notificationRepositoryForTest(t *testing.T, st store.Store) notificationServiceRepository {
	t.Helper()
	protection, err := buildSecretProtection(t.Context(), st, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	destinations := notifications.NewProtectedDestinationRepository(st, protection)
	now := time.Unix(1_900_000_000, 0).UTC()
	if err := destinations.SaveNotificationDestination(t.Context(), notifications.Destination{
		ID: "smtp-test", Means: notifications.MeansEmail, Label: "SMTP",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Enabled: true,
		Configuration: map[string]string{
			"host": "smtp.example.com", "port": "587", "security": "starttls",
			"fromAddress": "loomarr@example.com", "fromName": "Loomarr",
		},
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return notificationServiceRepository{
		Store:        st,
		destinations: destinations,
	}
}
