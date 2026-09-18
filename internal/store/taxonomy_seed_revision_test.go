package store

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
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

func TestFillerEnrichmentBackfillCapturesExistingCatalogFactsOnce(t *testing.T) {
	st := newSQLiteStore(t).(*sqlStore)
	ctx := t.Context()
	at := time.Unix(1_700_000_000, 0).UTC()
	clip := Clip{Clip: filler.Clip{Hash: "existing", Path: "existing.mp4", Name: "Existing",
		Kind: filler.Commercial, Era: 1999, Audience: filler.General, Brand: "HP Sauce",
		GeographicScope: filler.GeographicNational, Country: "GB", Network: "Five", GeoEvidence: "operator"},
		UpdatedAt: at, CreatedAt: at}
	if err := st.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	if err := st.SetClipTags(ctx, clip.Hash, []string{"condiments"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM filler_enrichment_backfills WHERE version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := st.backfillFillerEnrichment(ctx, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	states, err := st.ListFillerEnrichment(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	byAxis := make(map[fillerenrichment.Axis]fillerenrichment.State, len(states))
	for _, state := range states {
		byAxis[state.Axis] = state
	}
	if byAxis[fillerenrichment.AxisEra].Value.Year != 1999 || byAxis[fillerenrichment.AxisBrand].Value.Text != "HP Sauce" ||
		byAxis[fillerenrichment.AxisGeography].Evidence.Kind != fillerenrichment.EvidenceOperator ||
		len(byAxis[fillerenrichment.AxisProduct].Value.Tags) != 1 {
		t.Fatalf("backfilled states = %+v", states)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM filler_enrichment_axes WHERE clip_hash = 'existing' AND axis = 'brand'`); err != nil {
		t.Fatal(err)
	}
	if err := st.backfillFillerEnrichment(ctx, at.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	states, err = st.ListFillerEnrichment(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Axis == fillerenrichment.AxisBrand {
			t.Fatal("completed backfill recreated a deliberately removed axis row")
		}
	}
}
