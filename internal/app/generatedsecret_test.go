package app

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
)

type generatedSecretProbe struct {
	value        string
	durable      string
	currentReads int
}

func (p *generatedSecretProbe) Value(settings.GeneratedSecret) string { return p.value }

func (p *generatedSecretProbe) Current(context.Context, settings.GeneratedSecret) (string, error) {
	p.currentReads++
	p.value = p.durable
	return p.durable, nil
}

func TestCurrentGeneratedSecretKeepsSQLiteRequestPathCacheOnly(t *testing.T) {
	probe := &generatedSecretProbe{value: "sqlite-cached", durable: "should-not-read"}
	got, err := currentGeneratedSecret(context.Background(), store.DialectSQLite, probe,
		settings.SecretPlayout, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sqlite-cached" || probe.currentReads != 0 {
		t.Fatalf("SQLite read = %q with %d durable reads, want cached value and zero reads", got, probe.currentReads)
	}
}

func TestCurrentGeneratedSecretRefreshesPostgresReplicaAndRedactor(t *testing.T) {
	probe := &generatedSecretProbe{value: "stale-replica", durable: "rotated-durable"}
	redactorRefreshes := 0
	got, err := currentGeneratedSecret(context.Background(), store.DialectPostgres, probe,
		settings.SecretPlayout, func() { redactorRefreshes++ })
	if err != nil {
		t.Fatal(err)
	}
	if got != "rotated-durable" || probe.currentReads != 1 || redactorRefreshes != 1 {
		t.Fatalf("Postgres read = %q, reads %d, redactor refreshes %d", got, probe.currentReads, redactorRefreshes)
	}
}
