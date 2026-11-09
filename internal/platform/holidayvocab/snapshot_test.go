package holidayvocab

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
)

type holidayRows struct {
	rows []Holiday
	err  error
}

func (h *holidayRows) ListHolidays(context.Context) ([]Holiday, error) {
	return h.rows, h.err
}

func holidayIDsOf(v Vocabulary) []string {
	var ids []string
	for _, h := range v.Holidays() {
		ids = append(ids, h.ID)
	}
	return ids
}

func TestSnapshotKeepsTheLastGoodHolidays(t *testing.T) {
	stored := &holidayRows{}
	snap := NewSnapshot(stored, slog.New(slog.DiscardHandler))
	if got, want := holidayIDsOf(snap.Current()), holidayIDsOf(Default()); !slices.Equal(got, want) {
		t.Fatalf("before a load the snapshot = %v, want the shipped holidays", got)
	}

	stored.rows = []Holiday{{ID: "harvest-fair", Label: "Harvest fair"}}
	snap.Reload(t.Context())
	if got := holidayIDsOf(snap.Current()); !slices.Equal(got, []string{"harvest-fair"}) {
		t.Fatalf("after a load the snapshot = %v, want the stored holidays", got)
	}

	stored.rows = []Holiday{{ID: "Not A Slug", Label: "Broken"}}
	snap.Reload(t.Context())
	if got := holidayIDsOf(snap.Current()); !slices.Equal(got, []string{"harvest-fair"}) {
		t.Fatalf("an invalid stored set replaced the snapshot: %v", got)
	}
	stored.err = errors.New("database unreachable")
	snap.Reload(t.Context())
	if got := holidayIDsOf(snap.Current()); !slices.Equal(got, []string{"harvest-fair"}) {
		t.Fatalf("a failed read replaced the snapshot: %v", got)
	}
}
