package app

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/operator"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestTaxonomyEditorReloadsHolidaysAfterASeasonalEdit(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	snap := holidayvocab.NewSnapshot(st, slog.New(slog.DiscardHandler))
	editor := operator.NewTaxonomyEditor(st, nil, snap.Reload)
	harvest := taxonomy.Taxon{Slug: "harvest-fair", Label: "Harvest fair", Axis: taxonomy.AxisSeasonal}
	if _, err := editor.Apply(t.Context(), clipstore.TaxonomyEdit{Create: true, Taxon: harvest}); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, h := range snap.Current().Holidays() {
		ids = append(ids, h.ID)
	}
	if !slices.Contains(ids, harvest.Slug) {
		t.Fatal("a new seasonal taxon's holiday is not in the snapshot the engine reads")
	}
}
