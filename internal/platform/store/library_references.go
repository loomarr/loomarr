package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/proposals"
)

// LibraryReferences are the distinct item and collection ids stored records name (#2151 step 15).
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

// LibraryReferenceStore lists the item and collection ids stored records name.
type LibraryReferenceStore interface {
	// LibraryReferences lists every distinct id stored records name, sorted.
	LibraryReferences(ctx context.Context) (LibraryReferences, error)
}

// visitChannelReferences calls item for each item id a channel names and collection for each
// collection id.
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
// of the run that made it and is not read.
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

// openProposalStatus is the status of a proposal still waiting on a decision: the only ones whose
// references count. A decided proposal is history.
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
	airings, err := s.distinctStrings(ctx, s.db, `SELECT DISTINCT library_item_id FROM airings`)
	if err != nil {
		return LibraryReferences{}, fmt.Errorf("library references: %w", err)
	}
	for _, id := range airings {
		items[id] = true
	}
	rows, err := s.db.QueryContext(ctx, `SELECT library_id, key FROM titles WHERE library_id IS NOT NULL`)
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
