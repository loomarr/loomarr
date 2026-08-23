//go:build integration

package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestPostgresGeneratedSecretReaderObservesOtherReplicaRotation(t *testing.T) {
	stores := testkit.PostgresStores(t, 2)
	ctx := context.Background()
	noEnv := func(string) (string, bool) { return "", false }
	first, err := settings.NewSecrets(ctx, secretStoreAdapter{stores[0]}, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	second, err := settings.NewSecrets(ctx, secretStoreAdapter{stores[1]}, noEnv)
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
}
