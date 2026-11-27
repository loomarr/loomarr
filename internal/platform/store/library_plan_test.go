package store

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A track filter gathers its titles once, from the track index to each copy and file by key:
// nothing is looked for title by title, no table is read whole, and a language is matched in the
// index, not read from every track of the kind. The statement explained is the list's own.
func TestTrackFiltersReadTheirIndexes(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	query, args := s.ListCountQuery(librarystore.LibraryItemQuery{Kind: "movie", Audio: "EN", Subtitles: "fr"})
	rows, err := s.db.QueryContext(context.Background(), `EXPLAIN QUERY PLAN `+query, args...)
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
	indexed := 0
	for _, step := range plan {
		if strings.Contains(step, "idx_library_streams_language (kind=? AND language=?)") {
			indexed++
		}
		for _, table := range []string{"f", "src", "st"} {
			if step == "SCAN "+table || strings.HasPrefix(step, "SCAN "+table+" ") {
				t.Errorf("a track filter scans a table: %q", step)
			}
		}
	}
	// Two language filters, each a movie's own files and a series' episodes.
	if indexed != 4 {
		t.Errorf("%d of 4 track reads match the language in the index:\n%s", indexed, strings.Join(plan, "\n"))
	}
	// Each filter's titles are one list, not a subquery run again for each title.
	lists := 0
	for _, step := range plan {
		if strings.HasPrefix(step, "LIST SUBQUERY") {
			lists++
		}
	}
	if lists != 2 {
		t.Errorf("%d of 2 track filters gather their titles once:\n%s", lists, strings.Join(plan, "\n"))
	}
}

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
