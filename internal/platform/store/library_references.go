package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/loomarr/loomarr/internal/proposals"
)

// openProposalStatus is the status of a proposal still waiting on a decision: the only ones whose
// picks count. A decided proposal is history.
const openProposalStatus = "submitted"

// openProposalItems lists, sorted, the library items open proposals' picks name: the lineup,
// acquisitions, alternates and refused picks. Every other record that names a library item does it
// through a foreign key (titles, airings, scheduled_airings, channel_planned_items, #2793), which
// the catalog purge checks in SQL; a proposal still holds its picks only in its document.
func (s *sqlStore) openProposalItems(ctx context.Context) ([]string, error) {
	open, err := s.ListProposalsByStatus(ctx, openProposalStatus)
	if err != nil {
		return nil, fmt.Errorf("open proposal items: %w", err)
	}
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for i := range open {
		body := &open[i].Body
		for _, list := range [][]proposals.ProposalItem{body.Lineup, body.Acquisitions, body.Alternates} {
			for _, pick := range list {
				add(pick.LibraryItemID)
			}
		}
		for _, refused := range body.Refused {
			add(refused.Item.LibraryItemID)
		}
	}
	slices.Sort(out)
	return out, nil
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
