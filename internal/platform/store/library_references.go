package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/provision"
)

// LibraryReferences are the distinct item and collection ids stored records name (#2151 step 15):
// what the cutover rewrites from the media server's ids to the Library's.
type LibraryReferences struct {
	// Items are movie, series and episode ids: titles, airings, channels' slots, and open
	// proposals' picks.
	Items []string
	// Collections are the collection ids channels' scopes and rules, their lineups' membership
	// stamps, and open proposals' policies name.
	Collections []string
	// Keys is the title an item id was recorded for, where the record says (a title's, a
	// proposal pick's): a movie or series id the media server no longer knows still names its
	// title by its provider ids.
	Keys map[string]provision.Key
}

// LibraryReferenceStore lists and rewrites the item and collection ids stored records name.
type LibraryReferenceStore interface {
	// LibraryReferences lists every distinct id stored records name, sorted.
	LibraryReferences(ctx context.Context) (LibraryReferences, error)
	// RewriteLibraryReferences points every stored reference named in items or collections at the
	// id it maps to, in one transaction, and drops the media server's episode and path caches. An
	// id in neither map is left as it is. It returns how many records changed.
	RewriteLibraryReferences(ctx context.Context, items, collections map[string]string) (int, error)
}

// visitChannelReferences calls item for each item id a channel names and collection for each
// collection id, with a pointer the callback may rewrite through.
func visitChannelReferences(ch *Channel, item, collection func(*string)) {
	for i := range ch.Desired {
		item(&ch.Desired[i].LibraryItemID)
	}
	for i := range ch.Lineup {
		for j := range ch.Lineup[i].BoxSetIDs {
			collection(&ch.Lineup[i].BoxSetIDs[j])
		}
	}
	for i := range ch.Policy.Scope.Collections {
		collection(&ch.Policy.Scope.Collections[i])
	}
	for _, r := range ch.Policy.Rules {
		if r.What == nil {
			continue
		}
		for i := range r.What.Collections {
			collection(&r.What.Collections[i])
		}
	}
}

// visitProposalReferences does the same for a proposal's picks and policy. Its trace is evidence
// of the run that made it and is never rewritten.
func visitProposalReferences(p *proposals.Proposal, item, collection func(*string)) {
	for _, list := range [][]proposals.ProposalItem{p.Lineup, p.Acquisitions, p.Alternates} {
		for i := range list {
			item(&list[i].LibraryItemID)
		}
	}
	for i := range p.Refused {
		item(&p.Refused[i].Item.LibraryItemID)
	}
	for i := range p.Policy.Scope.Collections {
		collection(&p.Policy.Scope.Collections[i])
	}
	for _, r := range p.Policy.Rules {
		if r.What == nil {
			continue
		}
		for i := range r.What.Collections {
			collection(&r.What.Collections[i])
		}
	}
}

// openProposalStatus is the status of a proposal still waiting on a decision: the only ones the
// cutover rewrites. A decided proposal is history.
const openProposalStatus = "submitted"

func (s *sqlStore) LibraryReferences(ctx context.Context) (LibraryReferences, error) {
	items, collections := map[string]bool{}, map[string]bool{}
	addItem := func(id *string) {
		if *id != "" {
			items[*id] = true
		}
	}
	addCollection := func(id *string) {
		if *id != "" {
			collections[*id] = true
		}
	}
	keys := map[string]provision.Key{}
	airings, err := s.distinctStrings(ctx, s.db, `SELECT DISTINCT library_item_id FROM airings WHERE library_item_id <> ''`)
	if err != nil {
		return LibraryReferences{}, fmt.Errorf("library references: %w", err)
	}
	for _, id := range airings {
		items[id] = true
	}
	rows, err := s.db.QueryContext(ctx, `SELECT library_id, key FROM titles WHERE library_id <> ''`)
	if err != nil {
		return LibraryReferences{}, fmt.Errorf("library references: %w", err)
	}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			_ = rows.Close()
			return LibraryReferences{}, fmt.Errorf("library references: %w", err)
		}
		items[id], keys[id] = true, provision.Key(key)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return LibraryReferences{}, fmt.Errorf("library references: %w", err)
	}
	channels, err := s.ListChannels(ctx)
	if err != nil {
		return LibraryReferences{}, fmt.Errorf("library references: %w", err)
	}
	for i := range channels {
		visitChannelReferences(&channels[i], addItem, addCollection)
	}
	open, err := s.ListProposalsByStatus(ctx, openProposalStatus)
	if err != nil {
		return LibraryReferences{}, fmt.Errorf("library references: %w", err)
	}
	for i := range open {
		visitProposalReferences(&open[i].Body, addItem, addCollection)
		for _, list := range [][]proposals.ProposalItem{open[i].Body.Lineup, open[i].Body.Acquisitions, open[i].Body.Alternates} {
			for _, pick := range list {
				if key, err := pick.Key(); err == nil && pick.LibraryItemID != "" {
					if _, known := keys[pick.LibraryItemID]; !known {
						keys[pick.LibraryItemID] = key
					}
				}
			}
		}
	}
	return LibraryReferences{Items: sortedKeys(items), Collections: sortedKeys(collections), Keys: keys}, nil
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *sqlStore) distinctStrings(ctx context.Context, q queryer, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, s.ph(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *sqlStore) RewriteLibraryReferences(ctx context.Context, items, collections map[string]string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("rewrite library references: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := s.rewriteLibraryReferences(ctx, tx, items, collections)
	if err != nil {
		return 0, fmt.Errorf("rewrite library references: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("rewrite library references: %w", err)
	}
	return changed, nil
}

func (s *sqlStore) rewriteLibraryReferences(ctx context.Context, tx *sql.Tx, items, collections map[string]string) (int, error) {
	changed := 0
	titleIDs, err := s.distinctStrings(ctx, tx, `SELECT DISTINCT library_id FROM titles WHERE library_id <> ''`)
	if err != nil {
		return 0, err
	}
	for _, old := range titleIDs {
		next, ok := items[old]
		if !ok || next == old {
			continue
		}
		res, err := tx.ExecContext(ctx, s.ph(`UPDATE titles SET library_id = ? WHERE library_id = ?`), next, old)
		if err != nil {
			return 0, fmt.Errorf("titles: %w", err)
		}
		n, _ := res.RowsAffected()
		changed += int(n)
	}
	airingIDs, err := s.distinctStrings(ctx, tx, `SELECT DISTINCT library_item_id FROM airings WHERE library_item_id <> ''`)
	if err != nil {
		return 0, err
	}
	for _, old := range airingIDs {
		next, ok := items[old]
		if !ok || next == old {
			continue
		}
		// Two ids that name one item keep one row per airing: the key includes the item id.
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at, key)
			SELECT channel_id, ?, aired_at, recorded_at, key FROM airings WHERE library_item_id = ?
			ON CONFLICT DO NOTHING`), next, old); err != nil {
			return 0, fmt.Errorf("airings: %w", err)
		}
		res, err := tx.ExecContext(ctx, s.ph(`DELETE FROM airings WHERE library_item_id = ?`), old)
		if err != nil {
			return 0, fmt.Errorf("airings: %w", err)
		}
		n, _ := res.RowsAffected()
		changed += int(n)
	}

	var dirty bool
	remap := func(m map[string]string) func(*string) {
		return func(id *string) {
			if next, ok := m[*id]; ok && next != *id {
				*id, dirty = next, true
			}
		}
	}
	rows, err := tx.QueryContext(ctx, s.ph(s.channelSelect()+` ORDER BY number`))
	if err != nil {
		return 0, fmt.Errorf("channels: %w", err)
	}
	channels, err := scanChannels(rows)
	_ = rows.Close()
	if err != nil {
		return 0, fmt.Errorf("channels: %w", err)
	}
	for i := range channels {
		dirty = false
		visitChannelReferences(&channels[i], remap(items), remap(collections))
		if !dirty {
			continue
		}
		if _, err := s.saveChannel(ctx, tx, channels[i]); err != nil {
			return 0, fmt.Errorf("channels: %w", err)
		}
		changed++
	}
	prows, err := tx.QueryContext(ctx, s.ph(proposalSelect+` WHERE status = ?`), openProposalStatus)
	if err != nil {
		return 0, fmt.Errorf("proposals: %w", err)
	}
	open, err := scanProposals(prows)
	_ = prows.Close()
	if err != nil {
		return 0, fmt.Errorf("proposals: %w", err)
	}
	for i := range open {
		dirty = false
		visitProposalReferences(&open[i].Body, remap(items), remap(collections))
		if !dirty {
			continue
		}
		body, err := encodeProposalBody(open[i])
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, s.ph(`UPDATE proposals SET proposal_json = ? WHERE id = ?`), body, open[i].ID); err != nil {
			return 0, fmt.Errorf("proposals: %w", err)
		}
		changed++
	}
	return changed, nil
}
