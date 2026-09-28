package fillerstore

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/store/storetest"
)

func TestFillerEnrichmentBackfillCapturesExistingCatalogFactsOnce(t *testing.T) {
	t.Parallel()
	opened := storetest.SQLiteClones(t, openExtended, checkpointSQLite)(t)
	st := opened.(extended).sqlStore
	ctx := t.Context()
	at := time.Unix(1_700_000_000, 0).UTC()
	clip := store.Clip{Clip: filler.Clip{Hash: "existing", Path: "existing.mp4", Name: "Existing",
		Kind: filler.Commercial, Era: 1999, Audience: filler.General, Brand: "HP Sauce",
		GeographicScope: filler.GeographicNational, Country: "GB", Network: "Five", GeoEvidence: "operator"},
		UpdatedAt: at, CreatedAt: at}
	if err := opened.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	if err := opened.SetClipTags(ctx, clip.Hash, []string{"condiments"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, st.ph(`DELETE FROM filler_enrichment_backfills WHERE version = ?`), catalogProjectionBackfillVersion); err != nil {
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
	if byAxis[fillerenrichment.AxisKind].Value.Text != "commercial" ||
		byAxis[fillerenrichment.AxisEra].Value.Year != 1999 || byAxis[fillerenrichment.AxisBrand].Value.Text != "HP Sauce" ||
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

// Open is where the backfill runs now, so a migrating open must leave its marker: without it the
// server's first open would backfill whatever clips a seed tool wrote in between.
func TestOpenRecordsTheEnrichmentBackfill(t *testing.T) {
	t.Parallel()
	opened := storetest.SQLiteClones(t, openExtended, checkpointSQLite)(t)
	var applied int
	if err := store.HandleOf(opened).QueryRowContext(t.Context(), `SELECT COUNT(*) FROM filler_enrichment_backfills WHERE version = ?`,
		catalogProjectionBackfillVersion).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("backfill markers after a migrating open = %d, want 1", applied)
	}
}
