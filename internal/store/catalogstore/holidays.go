package catalogstore

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/store/storedb"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

// The holiday store (#2030): one row per holiday, keyed by the seasonal taxon it owns. Rows are
// stored as written; holidayvocab.New validates them into the snapshot the engine reads.

// ListHolidays returns every holiday definition in display order.
func (s *Store) ListHolidays(ctx context.Context) ([]holidayvocab.Holiday, error) {
	return listHolidaysFrom(ctx, s.h)
}

func listHolidaysFrom(ctx context.Context, q taxonomyQueryer) ([]holidayvocab.Holiday, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, label, words, kept, air_window, members, name_needs_corroboration FROM holidays ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("list holidays: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []holidayvocab.Holiday
	for rows.Next() {
		var h holidayvocab.Holiday
		var words, kept, airWindow, members string
		if err := rows.Scan(&h.ID, &h.Label, &words, &kept, &airWindow, &members, &h.NameNeedsCorroboration); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(words), &h.Words); err != nil {
			return nil, fmt.Errorf("holiday %s: words: %w", h.ID, err)
		}
		if err := json.Unmarshal([]byte(kept), &h.Kept); err != nil {
			return nil, fmt.Errorf("holiday %s: kept: %w", h.ID, err)
		}
		if airWindow != "" {
			h.AirWindow = &holidayvocab.Rule{}
			if err := json.Unmarshal([]byte(airWindow), h.AirWindow); err != nil {
				return nil, fmt.Errorf("holiday %s: air window: %w", h.ID, err)
			}
		}
		if err := json.Unmarshal([]byte(members), &h.Members); err != nil {
			return nil, fmt.Errorf("holiday %s: members: %w", h.ID, err)
		}
		// None is nil, as in a definition written in Go.
		if len(h.Kept) == 0 {
			h.Kept = nil
		}
		if len(h.Members) == 0 {
			h.Members = nil
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// insertHolidayTx adds h at the end of the display order. An id already stored is left as it is.
func (s *Store) insertHolidayTx(ctx context.Context, tx storedb.Tx, h holidayvocab.Holiday, at time.Time) error {
	words, _ := json.Marshal(h.Words)
	kept, _ := json.Marshal(nonNilKept(h.Kept))
	members, _ := json.Marshal(nonNil(h.Members))
	airWindow := ""
	if h.AirWindow != nil {
		raw, _ := json.Marshal(h.AirWindow)
		airWindow = string(raw)
	}
	var position int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM holidays`).Scan(&position); err != nil {
		return fmt.Errorf("insert holiday %s: position: %w", h.ID, err)
	}
	_, err := tx.ExecContext(ctx, s.h.Rebind(
		`INSERT INTO holidays (id, label, position, words, kept, air_window, members, name_needs_corroboration, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`),
		h.ID, h.Label, position, string(words), string(kept), airWindow, string(members), h.NameNeedsCorroboration, epoch(at))
	if err != nil {
		return fmt.Errorf("insert holiday %s: %w", h.ID, err)
	}
	return nil
}

func nonNilKept(k []holidayvocab.Keeping) []holidayvocab.Keeping {
	if k == nil {
		return []holidayvocab.Keeping{}
	}
	return k
}

// ConvergeHolidaySeed applies each shipped holiday revision once (#2030 decision 28), after the
// taxonomy seed has made the holidays' seasonal taxa. A holiday whose id is already stored keeps the
// operator's version; one whose taxon the operator deleted, or moved to another axis, is skipped.
// The revision is recorded even then, so an operator's choice is never undone by boot.
func (s *Store) ConvergeHolidaySeed(ctx context.Context, revisions []holidayvocab.SeedRevision, at time.Time) error {
	for _, revision := range revisions {
		if err := s.applyHolidaySeedRevision(ctx, revision, at); err != nil {
			return fmt.Errorf("converge holiday seed v%d: %w", revision.Version, err)
		}
	}
	return nil
}

func (s *Store) applyHolidaySeedRevision(ctx context.Context, revision holidayvocab.SeedRevision, at time.Time) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.lockTaxonomyTx(ctx, tx); err != nil {
		return err
	}
	var applied int
	if err := tx.QueryRowContext(ctx, s.h.Rebind(`SELECT COUNT(*) FROM holiday_seed_revisions WHERE version = ?`), revision.Version).Scan(&applied); err != nil {
		return fmt.Errorf("read marker: %w", err)
	}
	if applied > 0 {
		return nil
	}
	taxa, err := listTaxaFrom(ctx, tx)
	if err != nil {
		return err
	}
	stored, err := listHolidaysFrom(ctx, tx)
	if err != nil {
		return err
	}
	// dated is every stored holiday a group may list: one with no members of its own.
	dated := map[string]bool{}
	for _, h := range stored {
		dated[h.ID] = len(h.Members) == 0
	}
	for _, h := range revision.Holidays {
		if taxon, ok := findTaxon(taxa, h.ID); !ok || taxon.Axis != taxonomy.AxisSeasonal {
			continue
		}
		if len(h.Members) > 0 {
			// A member skipped above would dangle and make the whole stored set unloadable.
			h.Members = slices.DeleteFunc(slices.Clone(h.Members), func(m string) bool { return !dated[m] })
			if len(h.Members) == 0 {
				continue
			}
		}
		if err := s.insertHolidayTx(ctx, tx, h, at); err != nil {
			return err
		}
		if _, ok := dated[h.ID]; !ok {
			dated[h.ID] = len(h.Members) == 0
		}
	}
	if _, err := tx.ExecContext(ctx, s.h.Rebind(`INSERT INTO holiday_seed_revisions (version, applied_at) VALUES (?, ?)`), revision.Version, epoch(at)); err != nil {
		return fmt.Errorf("record marker: %w", err)
	}
	return tx.Commit()
}

// HealUndatedHolidays gives every seasonal taxon without a holiday an undated one (#2030 decision
// 30): a seasonal tag an operator made before holidays were data. Its clips air year-round until
// the operator dates it.
func (s *Store) HealUndatedHolidays(ctx context.Context, at time.Time) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("heal holidays: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.lockTaxonomyTx(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, s.h.Rebind(`SELECT t.slug, t.label FROM taxa t LEFT JOIN holidays h ON h.id = t.slug WHERE t.axis = ? AND h.id IS NULL ORDER BY t.slug`), string(taxonomy.AxisSeasonal))
	if err != nil {
		return fmt.Errorf("heal holidays: list: %w", err)
	}
	var orphans []holidayvocab.Holiday
	for rows.Next() {
		var h holidayvocab.Holiday
		if err := rows.Scan(&h.ID, &h.Label); err != nil {
			_ = rows.Close()
			return err
		}
		orphans = append(orphans, h)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("heal holidays: list: %w", err)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, h := range orphans {
		if err := s.insertHolidayTx(ctx, tx, h, at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// syncTaxonHolidayTx keeps a taxonomy edit's seasonal taxa and their holidays one-to-one, inside the
// edit's transaction: a taxon that becomes seasonal gets an undated holiday, and one that stops
// being seasonal takes its holiday with it, but only an undated one. A dated or group holiday holds
// clips' seasons and channels' calendars, so it is retired through the holidays, not the taxonomy.
// before runs ahead of the taxon write (the holiday row references it); after runs once it exists.
func (s *Store) syncTaxonHolidayTx(ctx context.Context, tx storedb.Tx, current taxonomy.Taxon, edit TaxonomyEdit, at time.Time) (after func() error, err error) {
	wasSeasonal := !edit.Create && current.Axis == taxonomy.AxisSeasonal
	isSeasonal := !edit.Delete && edit.Taxon.Axis == taxonomy.AxisSeasonal
	after = func() error { return nil }
	switch {
	case wasSeasonal && !isSeasonal:
		holidays, err := listHolidaysFrom(ctx, tx)
		if err != nil {
			return nil, err
		}
		if err := refuseHolidayRetirement(current.Slug, holidays); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, s.h.Rebind(`DELETE FROM holidays WHERE id = ?`), current.Slug); err != nil {
			return nil, fmt.Errorf("retire holiday %s: %w", current.Slug, err)
		}
		if err := s.dropGroupMembershipTx(ctx, tx, current.Slug, holidays, at); err != nil {
			return nil, err
		}
	case !wasSeasonal && isSeasonal:
		after = func() error {
			return s.insertHolidayTx(ctx, tx, holidayvocab.Holiday{ID: edit.Taxon.Slug, Label: edit.Taxon.Label}, at)
		}
	}
	return after, nil
}

// refuseHolidayRetirement is why a taxonomy edit may not retire holiday id along with its taxon:
// it has dates or members (it holds clips' seasons and channels' calendars, so it is retired
// through the holidays), or it is a group's last member (the group would silently become a plain
// undated holiday). Nil when id is undated, or is not a holiday at all.
func refuseHolidayRetirement(id string, holidays []holidayvocab.Holiday) error {
	for _, h := range holidays {
		if h.ID == id && (len(h.Kept) > 0 || len(h.Members) > 0) {
			return fmt.Errorf("%w: %q is a holiday with dates; retire it from the holidays instead", ErrTaxonConflict, id)
		}
		if len(h.Members) == 1 && h.Members[0] == id {
			return fmt.Errorf("%w: %q is the only holiday in group %q; change the group first", ErrTaxonConflict, id, h.ID)
		}
	}
	return nil
}

// dropGroupMembershipTx removes a retired holiday from every group that lists it: New rejects a
// member that names no holiday, so a dangling one would make the whole stored set unloadable.
func (s *Store) dropGroupMembershipTx(ctx context.Context, tx storedb.Tx, id string, holidays []holidayvocab.Holiday, at time.Time) error {
	for _, h := range holidays {
		kept := make([]string, 0, len(h.Members))
		for _, m := range h.Members {
			if m != id {
				kept = append(kept, m)
			}
		}
		if len(kept) == len(h.Members) {
			continue
		}
		members, _ := json.Marshal(kept)
		if _, err := tx.ExecContext(ctx, s.h.Rebind(`UPDATE holidays SET members = ?, updated_at = ? WHERE id = ?`), string(members), epoch(at), h.ID); err != nil {
			return fmt.Errorf("drop %s from group %s: %w", id, h.ID, err)
		}
	}
	return nil
}

func findTaxon(taxa []taxonomy.Taxon, slug string) (taxonomy.Taxon, bool) {
	for _, t := range taxa {
		if t.Slug == slug {
			return t, true
		}
	}
	return taxonomy.Taxon{}, false
}
