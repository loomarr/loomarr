package store

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/taxonomy"
)

func TestTaxonomySeedRevisionConvergesOnceWithoutOverwritingOperatorChoice(t *testing.T) {
	store := newSQLiteStore(t).(*sqlStore)
	ctx := t.Context()
	if _, err := store.db.ExecContext(ctx, `DELETE FROM taxonomy_seed_revisions WHERE version = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM taxa WHERE slug IN ('animated', 'live-action', 'condiments')`); err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := store.convergeTaxonomySeed(ctx, taxonomy.SeedRevisions(), at); err != nil {
		t.Fatal(err)
	}
	taxa, err := store.ListTaxa(ctx)
	if err != nil {
		t.Fatal(err)
	}
	forest := taxonomy.New(taxa)
	for _, slug := range []string{"animated", "live-action", "condiments"} {
		if _, ok := forest.Get(slug); !ok {
			t.Fatalf("revision did not add %q", slug)
		}
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM taxa WHERE slug = 'animated'`); err != nil {
		t.Fatal(err)
	}
	if err := store.convergeTaxonomySeed(ctx, taxonomy.SeedRevisions(), at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	taxa, err = store.ListTaxa(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := taxonomy.New(taxa).Get("animated"); ok {
		t.Fatal("applied revision recreated a term the operator removed")
	}
}
