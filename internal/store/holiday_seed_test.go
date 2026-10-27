package store

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

func TestHolidaySeedAppliesOnceAndKeepsOperatorChoices(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	ctx := t.Context()
	at := time.Unix(1_700_000_000, 0).UTC()
	// An install upgrading into holidays: the revision has not run, the operator deleted two
	// shipped holidays' taxa (one a group's member), and moved another's to a different axis.
	for _, stmt := range []string{
		`DELETE FROM holidays`,
		`DELETE FROM holiday_seed_revisions`,
		`DELETE FROM taxa WHERE slug = 'bonfire-night'`,
		`DELETE FROM taxa WHERE slug = 'christmas'`,
		`UPDATE taxa SET axis = 'format' WHERE slug = 'memorial-day'`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.convergeHolidaySeed(ctx, holidayvocab.SeedRevisions(), at); err != nil {
		t.Fatal(err)
	}
	ids := holidayIDs(t, s)
	for _, gone := range []string{"bonfire-night", "memorial-day"} {
		if slices.Contains(ids, gone) {
			t.Fatalf("the seed stored %q, whose taxon the operator removed from the seasonal axis", gone)
		}
	}
	if !slices.Contains(ids, "halloween") {
		t.Fatalf("the seed did not store halloween: %v", ids)
	}
	if got := holidayByID(t, s, holidayvocab.GenericHoliday).Members; !slices.Equal(got, []string{"thanksgiving", "newyear"}) {
		t.Fatalf("the seeded group's members = %v, want the skipped holiday left out", got)
	}
	holidays, err := s.ListHolidays(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holidayvocab.New(holidays); err != nil {
		t.Fatalf("the seeded holidays do not load: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM holidays WHERE id = 'valentines'`); err != nil {
		t.Fatal(err)
	}
	if err := s.convergeHolidaySeed(ctx, holidayvocab.SeedRevisions(), at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(holidayIDs(t, s), "valentines") {
		t.Fatal("an applied revision recreated a holiday the operator retired")
	}
}

func TestHealGivesEveryOrphanSeasonalTaxonAnUndatedHoliday(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	ctx := t.Context()
	at := time.Unix(1_700_000_000, 0).UTC()
	// An operator's seasonal tag from before holidays were data.
	if err := s.upsertTaxon(ctx, taxonomy.Taxon{Slug: "harvest_fair", Label: "Harvest fair", Axis: taxonomy.AxisSeasonal}, at); err != nil {
		t.Fatal(err)
	}
	if err := s.healUndatedHolidays(ctx, at); err != nil {
		t.Fatal(err)
	}
	holidays, err := s.ListHolidays(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := holidays[len(holidays)-1]
	if last.ID != "harvest_fair" || last.Label != "Harvest fair" || last.Kept != nil || last.Members != nil {
		t.Fatalf("the healed holiday = %+v, want undated harvest_fair last", last)
	}
	if _, err := holidayvocab.New(holidays); err != nil {
		t.Fatalf("the healed holidays do not load: %v", err)
	}
	if err := s.healUndatedHolidays(ctx, at); err != nil {
		t.Fatal(err)
	}
	if got := len(holidayIDs(t, s)); got != len(holidays) {
		t.Fatalf("a second heal changed the holidays: %d, want %d", got, len(holidays))
	}
}

func TestRetiringAnUndatedHolidayLeavesNoGroupNamingIt(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	ctx := t.Context()
	at := time.Unix(1_700_000_000, 0).UTC()
	harvest := taxonomy.Taxon{Slug: "harvest-fair", Label: "Harvest fair", Axis: taxonomy.AxisSeasonal}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Create: true, Taxon: harvest}, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE holidays SET members = '["christmas","harvest-fair"]' WHERE id = ?`, holidayvocab.GenericHoliday); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Delete: true, Slug: harvest.Slug}, at); err != nil {
		t.Fatal(err)
	}
	if got := holidayByID(t, s, holidayvocab.GenericHoliday).Members; !slices.Equal(got, []string{"christmas"}) {
		t.Fatalf("the group's members = %v, want the retired holiday dropped", got)
	}
	holidays, err := s.ListHolidays(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holidayvocab.New(holidays); err != nil {
		t.Fatalf("the holidays do not load after a retirement: %v", err)
	}
}

func TestAGroupsLastMemberIsNotRetiredWithItsTaxon(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	ctx := t.Context()
	at := time.Unix(1_700_000_000, 0).UTC()
	harvest := taxonomy.Taxon{Slug: "harvest-fair", Label: "Harvest fair", Axis: taxonomy.AxisSeasonal}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Create: true, Taxon: harvest}, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE holidays SET members = '["harvest-fair"]' WHERE id = ?`, holidayvocab.GenericHoliday); err != nil {
		t.Fatal(err)
	}
	retire := TaxonomyEdit{Delete: true, Slug: harvest.Slug}
	if _, err := s.PreviewTaxonomyEdit(ctx, retire); !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("preview of retiring a group's last member = %v, want ErrTaxonConflict", err)
	}
	if err := s.ApplyTaxonomyEdit(ctx, retire, at); !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("retiring a group's last member = %v, want ErrTaxonConflict", err)
	}
	if got := holidayByID(t, s, holidayvocab.GenericHoliday).Members; !slices.Equal(got, []string{"harvest-fair"}) {
		t.Fatalf("a refused retirement changed the group: %v", got)
	}
}
