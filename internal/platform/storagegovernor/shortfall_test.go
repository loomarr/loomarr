package storagegovernor_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
)

const (
	shortfallRoot = "/filler"
	mib           = int64(1 << 20)
)

func shortfallMeter(managed int64) *meter {
	return &meter{
		byPath:  map[string]storagegovernor.Measurement{shortfallRoot: {ID: "disk-a", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 40 * storagegovernor.GiB}},
		managed: map[string]map[storagegovernor.Domain]int64{"disk-a": {storagegovernor.DomainFiller: managed}},
	}
}

// The shortfall is the room the refused work asked for, plus whatever the library is over its
// allowance (lowering the allowance deletes nothing), so an eviction sheds both.
func TestShortfallIsWhatTheRefusedReservationNeeded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for name, tc := range map[string]struct {
		budget, managed, ask, want int64
	}{
		"room short by part of the ask":      {budget: 100 * mib, managed: 80 * mib, ask: 50 * mib, want: 30 * mib},
		"a full library asks for all of it":  {budget: 100 * mib, managed: 100 * mib, ask: 50 * mib, want: 50 * mib},
		"a library over its allowance sheds": {budget: 100 * mib, managed: 130 * mib, ask: 50 * mib, want: 80 * mib},
	} {
		t.Run(name, func(t *testing.T) {
			g := storagegovernor.New(shortfallMeter(tc.managed), fillerPolicy(tc.budget))
			if lease, d := g.Reserve(ctx, storagegovernor.Request{Path: shortfallRoot, EstimatedBytes: tc.ask}); lease != nil ||
				d.Snapshot.Reason != storagegovernor.ReasonLibraryLimit {
				t.Fatalf("setup: reserve = %v %+v, want library_limit", lease, d)
			}
			if got := g.LibraryShortfall(ctx, shortfallRoot, storagegovernor.DomainFiller); got != tc.want {
				t.Fatalf("shortfall = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestShortfallIsZeroUntilALibraryLimitRefusal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := shortfallMeter(100 * mib)
	g := storagegovernor.New(m, fillerPolicy(100*mib))
	if got := g.LibraryShortfall(ctx, shortfallRoot, storagegovernor.DomainFiller); got != 0 {
		t.Fatalf("shortfall before any refusal = %d, want 0 (a full library nobody asked room of needs nothing)", got)
	}

	// A refusal for the host's reserve is not the library's to fix by evicting.
	m.update(shortfallRoot, storagegovernor.Measurement{ID: "disk-a", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: storagegovernor.GiB}, 0)
	if lease, d := g.Reserve(ctx, storagegovernor.Request{Path: shortfallRoot, EstimatedBytes: mib}); lease != nil ||
		d.Snapshot.Reason != storagegovernor.ReasonHostReserve {
		t.Fatalf("setup: reserve = %v %+v, want host_reserve", lease, d)
	}
	if got := g.LibraryShortfall(ctx, shortfallRoot, storagegovernor.DomainFiller); got != 0 {
		t.Fatalf("shortfall after a host_reserve refusal = %d, want 0", got)
	}
}

// Work that stops asking stops freeing space: a reservation as large as the refused one succeeding
// ends the shortfall. A smaller one does not, since it says nothing about the larger ask.
func TestShortfallEndsWhenTheRefusedWorkIsServed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	budget := 100 * mib
	g := storagegovernor.New(shortfallMeter(80*mib), func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: budget}
	})
	if lease, _ := g.Reserve(ctx, storagegovernor.Request{Path: shortfallRoot, EstimatedBytes: 50 * mib}); lease != nil {
		t.Fatal("setup: a library short of room granted the reservation")
	}

	budget = 135 * mib // room for a small ask, not the refused one
	small, d := g.Reserve(ctx, storagegovernor.Request{Path: shortfallRoot, EstimatedBytes: 10 * mib})
	if small == nil {
		t.Fatalf("setup: small reservation refused: %+v", d)
	}
	small.Release()
	if got := g.LibraryShortfall(ctx, shortfallRoot, storagegovernor.DomainFiller); got != 0 {
		t.Fatalf("shortfall with the budget raised = %d, want 0 (nothing is short of the ask any more)", got)
	}

	budget = 100 * mib
	if got := g.LibraryShortfall(ctx, shortfallRoot, storagegovernor.DomainFiller); got != 30*mib {
		t.Fatalf("a smaller grant ended the shortfall: got %d, want %d", got, 30*mib)
	}

	budget = 200 * mib
	big, d := g.Reserve(ctx, storagegovernor.Request{Path: shortfallRoot, EstimatedBytes: 50 * mib})
	if big == nil {
		t.Fatalf("setup: the refused reservation was still refused: %+v", d)
	}
	big.Release()
	budget = 100 * mib
	if got := g.LibraryShortfall(ctx, shortfallRoot, storagegovernor.DomainFiller); got != 0 {
		t.Fatalf("shortfall after the refused work was served = %d, want 0", got)
	}
}
