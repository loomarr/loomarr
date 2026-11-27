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
// A track filter gathers its titles once, from the track index to each copy and file by key:
// nothing is looked for title by title, and no table is read whole.
func TestTrackFiltersReadTheirIndexes(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	rows, err := s.db.QueryContext(context.Background(), `EXPLAIN QUERY PLAN `+librarystore.TrackFilterQuery())
	if err != nil {
		t.Fatal(err)
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
	t.Log(strings.Join(plan, "\n"))
	for _, step := range plan {
		for _, table := range []string{"f", "src", "st"} {
			if step == "SCAN "+table || strings.HasPrefix(step, "SCAN "+table+" ") {
				t.Errorf("a track filter scans a table: %q", step)
			}
		}
		if strings.HasPrefix(step, "CORRELATED") {
			t.Errorf("a track filter is looked for title by title: %q", step)
		}
	}
}

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
