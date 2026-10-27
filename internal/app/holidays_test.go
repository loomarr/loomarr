package app

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/taxonomy"
	"github.com/loomarr/loomarr/internal/testkit"
)

type holidayRows struct {
	rows []holidayvocab.Holiday
	err  error
}

func (h *holidayRows) ListHolidays(context.Context) ([]holidayvocab.Holiday, error) {
	return h.rows, h.err
}

func holidayIDsOf(v holidayvocab.Vocabulary) []string {
	var ids []string
	for _, h := range v.Holidays() {
		ids = append(ids, h.ID)
	}
	return ids
}

func TestHolidaySnapshotKeepsTheLastGoodHolidays(t *testing.T) {
	stored := &holidayRows{}
	snap := newHolidaySnapshot(stored, slog.New(slog.DiscardHandler))
	if got, want := holidayIDsOf(snap.Current()), holidayIDsOf(holidayvocab.Default()); !slices.Equal(got, want) {
		t.Fatalf("before a load the snapshot = %v, want the shipped holidays", got)
	}

	stored.rows = []holidayvocab.Holiday{{ID: "harvest-fair", Label: "Harvest fair"}}
	snap.Reload(t.Context())
	if got := holidayIDsOf(snap.Current()); !slices.Equal(got, []string{"harvest-fair"}) {
		t.Fatalf("after a load the snapshot = %v, want the stored holidays", got)
	}

	stored.rows = []holidayvocab.Holiday{{ID: "Not A Slug", Label: "Broken"}}
	snap.Reload(t.Context())
	if got := holidayIDsOf(snap.Current()); !slices.Equal(got, []string{"harvest-fair"}) {
		t.Fatalf("an invalid stored set replaced the snapshot: %v", got)
	}
	stored.err = errors.New("database unreachable")
	snap.Reload(t.Context())
	if got := holidayIDsOf(snap.Current()); !slices.Equal(got, []string{"harvest-fair"}) {
		t.Fatalf("a failed read replaced the snapshot: %v", got)
	}
}

func TestTaxonomyEditorReloadsHolidaysAfterASeasonalEdit(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	snap := newHolidaySnapshot(st, slog.New(slog.DiscardHandler))
	editor := taxonomyEditor{store: st, holidaysChanged: snap.Reload}
	harvest := taxonomy.Taxon{Slug: "harvest-fair", Label: "Harvest fair", Axis: taxonomy.AxisSeasonal}
	if _, err := editor.Apply(t.Context(), store.TaxonomyEdit{Create: true, Taxon: harvest}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(holidayIDsOf(snap.Current()), harvest.Slug) {
		t.Fatal("a new seasonal taxon's holiday is not in the snapshot the engine reads")
	}
}
