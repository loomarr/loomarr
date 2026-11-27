package store

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The fetch queue is read from idx_images_fetch_queue in queue order (priority, then queued time,
// then hash), so the every-minute pass reads the front of the queue rather than sorting every
// waiting row.
func TestImageFetchQueueReadsItsIndex(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	rows, err := s.db.QueryContext(context.Background(), `EXPLAIN QUERY PLAN `+clipstore.AwaitingFetchQuery(), 0, 64)
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
	joined := strings.Join(plan, " | ")
	if !strings.Contains(joined, "idx_images_fetch_queue") || strings.Contains(joined, "TEMP B-TREE") {
		t.Fatalf("plan = %q, want the queue read from idx_images_fetch_queue with no sort", plan)
	}
}
