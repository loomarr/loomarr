package scan

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// fakeNeedsStore holds titles that need the household and which of them a notice counted.
type fakeNeedsStore struct {
	needs   []string
	noticed map[string]bool
}

func (f *fakeNeedsStore) LibraryNeeds(context.Context) ([]store.LibraryNeed, error) {
	out := make([]store.LibraryNeed, 0, len(f.needs))
	for _, id := range f.needs {
		var n store.LibraryNeed
		n.ID = id
		out = append(out, n)
	}
	return out, nil
}

func (f *fakeNeedsStore) UnnoticedLibraryNeeds(context.Context) ([]string, error) {
	var out []string
	for _, id := range f.needs {
		if !f.noticed[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fakeNeedsStore) NoticeLibraryNeeds(_ context.Context, ids []string) error {
	for _, id := range ids {
		f.noticed[id] = true
	}
	return nil
}

// The notice waits for the import to settle, counts every title that needs the household when any
// is new, says nothing again about the same titles, and lets a day pass between notices.
func TestNeedsNoticeIsDailyAndOnlyForNewTitles(t *testing.T) {
	ctx := context.Background()
	st := &fakeNeedsStore{needs: []string{"90000001", "90000002"}, noticed: map[string]bool{}}
	now := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	settled := false
	var sent []int
	n := &NeedsNotice{
		Store: st, Notify: func(_ context.Context, total int) { sent = append(sent, total) },
		Settled: func() bool { return settled }, Now: func() time.Time { return now },
	}
	tick := func() {
		t.Helper()
		if err := n.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}

	tick()
	if len(sent) != 0 {
		t.Fatalf("sent %v while the import runs", sent)
	}
	settled = true
	tick()
	if !slices.Equal(sent, []int{2}) {
		t.Fatalf("sent %v, want both titles counted", sent)
	}

	// Nothing new: nothing said, however long it has been.
	now = now.Add(48 * time.Hour)
	tick()
	if len(sent) != 1 {
		t.Fatalf("sent %v about titles already counted", sent)
	}

	// A new title is said at once when a day has passed, with every title counted; another within
	// the day waits for the next one.
	st.needs = append(st.needs, "90000003")
	tick()
	st.needs = append(st.needs, "90000004")
	now = now.Add(time.Hour)
	tick()
	if !slices.Equal(sent, []int{2, 3}) {
		t.Fatalf("sent %v, want the new title said once within the day", sent)
	}
	now = now.Add(needsNoticeGap)
	tick()
	if !slices.Equal(sent, []int{2, 3, 4}) {
		t.Fatalf("sent %v, want the next day's new title said", sent)
	}

	// Off is off.
	st.needs = append(st.needs, "90000005")
	now = now.Add(needsNoticeGap)
	n.Enabled = func() bool { return false }
	tick()
	if len(sent) != 3 {
		t.Fatalf("sent %v with the scanner off", sent)
	}
}
