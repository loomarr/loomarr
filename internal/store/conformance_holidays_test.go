package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

// Holidays (#2030): seeded at boot from the shipped definitions, each owning its seasonal taxon,
// and kept one-to-one with the seasonal taxa through every taxonomy edit.

func testHolidaysSeededOnOpen(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	got, err := s.ListHolidays(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := holidayvocab.Seed(); !reflect.DeepEqual(got, want) {
		t.Fatalf("a fresh store's holidays are not the shipped seed, in order:\n got %+v\nwant %+v", got, want)
	}
	if _, err := holidayvocab.New(got); err != nil {
		t.Fatalf("the stored holidays do not load: %v", err)
	}
}

func testHolidaysFollowSeasonalTaxa(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_900_000_000, 0).UTC()
	harvest := taxonomy.Taxon{Slug: "harvest-fair", Label: "Harvest fair", Axis: taxonomy.AxisSeasonal}

	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Create: true, Taxon: harvest}, at); err != nil {
		t.Fatal(err)
	}
	got := holidayIDs(t, s)
	if got[len(got)-1] != harvest.Slug {
		t.Fatalf("a new seasonal taxon is not the last holiday: %v", got)
	}
	if h := holidayByID(t, s, harvest.Slug); h.Label != harvest.Label || len(h.Kept) > 0 || len(h.Members) > 0 {
		t.Fatalf("a new seasonal taxon's holiday = %+v, want undated with the taxon's label", h)
	}

	moved := harvest
	moved.Axis = taxonomy.AxisFormat
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Taxon: moved}, at); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(holidayIDs(t, s), harvest.Slug) {
		t.Fatal("a taxon moved off the seasonal axis kept its undated holiday")
	}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Taxon: harvest}, at); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(holidayIDs(t, s), harvest.Slug) {
		t.Fatal("a taxon moved onto the seasonal axis got no holiday")
	}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Delete: true, Slug: harvest.Slug}, at); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(holidayIDs(t, s), harvest.Slug) {
		t.Fatal("a deleted seasonal taxon left its undated holiday")
	}

	// A dated holiday holds seasons and calendars: the taxonomy cannot take it away.
	before := holidayIDs(t, s)
	if _, err := s.PreviewTaxonomyEdit(ctx, TaxonomyEdit{Delete: true, Slug: "halloween"}); !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("previewing a dated holiday's taxon deletion = %v, want ErrTaxonConflict", err)
	}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Delete: true, Slug: "halloween"}, at); !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("deleting a dated holiday's taxon = %v, want ErrTaxonConflict", err)
	}
	halloween := taxonomy.Taxon{Slug: "halloween", Label: "Halloween", Axis: taxonomy.AxisFormat}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Taxon: halloween}, at); !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("moving a dated holiday's taxon off seasonal = %v, want ErrTaxonConflict", err)
	}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Delete: true, Slug: holidayvocab.GenericHoliday}, at); !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("deleting a group holiday's taxon = %v, want ErrTaxonConflict", err)
	}
	if got := holidayIDs(t, s); !slices.Equal(got, before) {
		t.Fatalf("a refused edit changed the holidays: %v, want %v", got, before)
	}
	// A seasonal taxon's other edits leave its holiday alone.
	relabelled := taxonomy.Taxon{Slug: "halloween", Label: "All Hallows' Eve", Axis: taxonomy.AxisSeasonal}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Taxon: relabelled}, at); err != nil {
		t.Fatal(err)
	}
	if h := holidayByID(t, s, "halloween"); len(h.Kept) == 0 {
		t.Fatal("relabelling a seasonal taxon lost its holiday's dates")
	}
}

func holidayIDs(t *testing.T, s Store) []string {
	t.Helper()
	holidays, err := s.ListHolidays(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(holidays))
	for _, h := range holidays {
		ids = append(ids, h.ID)
	}
	return ids
}

func holidayByID(t *testing.T, s Store, id string) holidayvocab.Holiday {
	t.Helper()
	holidays, err := s.ListHolidays(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range holidays {
		if h.ID == id {
			return h
		}
	}
	t.Fatalf("no holiday %q", id)
	return holidayvocab.Holiday{}
}
