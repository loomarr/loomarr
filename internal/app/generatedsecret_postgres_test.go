//go:build integration

package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/secretprotection"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestPostgresGeneratedSecretReaderObservesOtherReplicaRotation(t *testing.T) {
	stores := testkit.PostgresStores(t, 2)
	ctx := context.Background()
	noEnv := func(string) (string, bool) { return "", false }
	firstProtection := testSecretProtection(t, stores[0])
	secondProtection := testSecretProtection(t, stores[1])
	first, err := settings.NewSecrets(ctx, secretStoreAdapter{st: stores[0], protection: firstProtection}, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	second, err := settings.NewSecrets(ctx, secretStoreAdapter{st: stores[1], protection: secondProtection}, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	old := second.Value(settings.SecretPlayout)
	fresh, err := first.Regenerate(ctx, settings.SecretPlayout)
	if err != nil {
		t.Fatal(err)
	}
	got, err := currentGeneratedSecret(ctx, store.DialectPostgres, second,
		settings.SecretPlayout, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != fresh || got == old {
		t.Fatalf("replica token = %q, want rotated durable %q (old %q)", got, fresh, old)
	}
	stored, err := stores[1].GetSetting(ctx, "secret.playout_token")
	if err != nil || stored == fresh || !secretprotection.IsEnvelope(stored) {
		t.Fatalf("stored replica token = (%q, %v), want envelope", stored, err)
	}
}
