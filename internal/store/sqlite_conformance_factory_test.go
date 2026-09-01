package store

import (
	"context"
	"errors"
	"testing"
)

func TestSQLiteConformanceFactoryClonesMigratedIsolatedStores(t *testing.T) {
	t.Parallel()

	var migrationModes []bool
	newStore := newSQLiteConformanceStoreFactoryWithOpen(t, func(ctx context.Context, dsn string, autoMigrate bool) (Store, error) {
		migrationModes = append(migrationModes, autoMigrate)
		return Open(ctx, dsn, autoMigrate)
	})
	left := newStore(t)
	right := newStore(t)
	ctx := context.Background()
	if len(migrationModes) != 3 || !migrationModes[0] || migrationModes[1] || migrationModes[2] {
		t.Fatalf("auto-migrate modes = %v, want one template migration followed by clone-only opens", migrationModes)
	}

	wantVersion, err := highestMigration("migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got := SchemaVersion(left); got != wantVersion {
		t.Fatalf("cloned schema version = %d, want %d", got, wantVersion)
	}
	taxa, err := right.ListTaxa(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(taxa) == 0 {
		t.Fatal("cloned store lost boot-seeded taxonomy")
	}

	if err := left.SetSetting(ctx, "clone.isolation", "left-only"); err != nil {
		t.Fatal(err)
	}
	if _, err := right.GetSetting(ctx, "clone.isolation"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second clone observed first clone's write: %v", err)
	}
}
