package binder_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

type numSource struct {
	taken map[int]bool
	err   error
}

func (n numSource) TakenChannelNumbers(context.Context) (map[int]bool, error) {
	if n.err != nil {
		return nil, n.err
	}
	return n.taken, nil
}

// seedNumberedChannel writes a channel that exists only to occupy a number. Separate from
// binder_test.go's seedChannel, which fixes Number at 5 and takes a lineup/policy these cases
// have no use for.
func seedNumberedChannel(t *testing.T, st store.Store, id string, number int) {
	t.Helper()
	var ch store.Channel
	ch.ID = id
	ch.Name = id
	ch.Number = number
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusBuilding
	if _, err := st.SaveChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

func TestNumberInUse(t *testing.T) {
	log := slog.New(slog.DiscardHandler)

	tests := []struct {
		name    string
		nums    binder.NumberSource // nil ⇒ no Tunarr wired
		number  int
		want    bool
		wantErr bool
	}{
		{
			name:   "free in both",
			nums:   numSource{taken: map[int]bool{9: true}},
			number: 5, want: false,
		},
		{
			name:   "held by one of Loomarr's own channels",
			nums:   numSource{taken: map[int]bool{}},
			number: 3, want: true,
		},
		{
			// The gap this method was added to close: #258 taught the APPROVE path to union
			// both sources and left the typed-number handlers checking the store alone.
			name:   "held only by Tunarr",
			nums:   numSource{taken: map[int]bool{7: true}},
			number: 7, want: true,
		},
		{
			// ⚠ **A live channel's own number reports IN USE, and that is the documented
			// contract, not a bug.** An earlier cut took an `exceptChannelID` so a no-op
			// renumber would not self-conflict; it cannot be implemented, because Tunarr's
			// list is a bare map[int]bool with no identity — excluding a Loomarr row leaves
			// the same number claimed on the Tunarr side, which is exactly where a pushed
			// channel appears. This case pins the honest answer so nobody re-adds the
			// parameter without confronting the Tunarr half.
			name:   "a live channel's own number is in use (no except escape)",
			nums:   numSource{taken: map[int]bool{3: true}},
			number: 3, want: true,
		},
		{
			// ⚠ Best-effort: an unreachable Tunarr reports NOT-in-use rather than failing the
			// call. A create that refuses because Tunarr is briefly down is a worse failure than
			// the collision this prevents, and the collision is recoverable at push time.
			name:   "unreachable Tunarr falls back to store-only",
			nums:   numSource{err: errors.New("boom")},
			number: 7, want: false,
		},
		{
			name:   "unreachable Tunarr still reports Loomarr's own numbers",
			nums:   numSource{err: errors.New("boom")},
			number: 3, want: true,
		},
		{
			name:   "no Tunarr wired at all",
			nums:   nil,
			number: 7, want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStore(t)
			seedNumberedChannel(t, st, "c1", 3)
			seedNumberedChannel(t, st, "c2", 4)

			b := binder.New(st, nil, nil, log)
			if tc.nums != nil {
				b = b.WithChannelNumbers(tc.nums)
			}

			got, err := b.NumberInUse(context.Background(), tc.number)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("NumberInUse(%d) = %v, want %v", tc.number, got, tc.want)
			}
		})
	}
}
