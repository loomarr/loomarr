package store

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// The shows waiting for an episode order are read from the partial index on unnumbered episodes,
// so the Library status and Needs you cost what waits, not the size of the library. Matching on
// kind as well led SQLite down the kind index instead: a walk over every episode on each read.
func TestWaitingShowsReadTheirIndex(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	rows, err := s.db.QueryContext(context.Background(), `EXPLAIN QUERY PLAN `+librarystore.WaitingShowsQuery())
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan[0], "idx_library_items_unnumbered") {
		t.Fatalf("plan = %q, want the waiting episodes read from idx_library_items_unnumbered", plan)
	}
}
