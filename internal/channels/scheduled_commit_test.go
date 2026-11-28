package channels_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/timeline"
	"github.com/loomarr/loomarr/internal/testkit"
)

// rivalRedeal is a store on which another replica commits a re-deal of the channel just before
// this reconcile's first save, so that save loses the CAS.
type rivalRedeal struct {
	store.Store
	done bool
}

func (r *rivalRedeal) SaveChannel(ctx context.Context, ch channelstore.Channel) (channelstore.Channel, error) {
	if !r.done {
		r.done = true
		winner, err := r.GetChannel(ctx, ch.ID)
		if err != nil {
			return channelstore.Channel{}, err
		}
		winner.Desired = slices.Clone(winner.Desired)
		slices.Reverse(winner.Desired)
		if _, err := r.Store.SaveChannel(ctx, winner); err != nil {
			return channelstore.Channel{}, err
		}
	}
	return r.Store.SaveChannel(ctx, ch)
}

func ledgerChannel(t *testing.T, st store.Store, avail channels.Availability, now *time.Time) *channels.Engine {
	t.Helper()
	e := channels.New(st, nil, avail, nil, channels.Config{
		ReconcileTTL: 10 * time.Minute,
		ResolvePlayoutBackendContext: func(context.Context) (string, error) {
			return schedule.PlayoutBackendInternal, nil
		},
	}, func() time.Time { return *now }, testkit.Logger())
	return e
}

func ledgerFilms() []schedule.LineupEntry {
	return []schedule.LineupEntry{
		entry("movie:tmdb:90000001", "A"), entry("movie:tmdb:90000002", "B"),
		entry("movie:tmdb:90000003", "C"), entry("movie:tmdb:90000004", "D"),
	}
}

// startedPrograms is the ledger a cycle owes at now: each programme started by then, at its start.
func startedPrograms(ch channelstore.Channel, now time.Time) map[string]time.Time {
	want := map[string]time.Time{}
	for _, b := range timeline.BroadcastsBetween(ch.Desired, ch.PlayoutAnchor, ch.PlayoutAnchor, now.Add(time.Nanosecond)) {
		if b.Kind == schedule.SlotProgram && b.LibraryItemID != "" {
			want[b.LibraryItemID] = b.Start
		}
	}
	return want
}

// A reconcile that loses its commit to another writer records nothing from the cycle it planned
// against, which the commit did not confirm. Its retry records from the winning row.
func TestReconcileThatLosesItsCommitRecordsFromTheWinningRow(t *testing.T) {
	ctx := context.Background()
	inner := newStore(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	avail := mapAvail{
		"movie:tmdb:90000001": "lib-1", "movie:tmdb:90000002": "lib-2",
		"movie:tmdb:90000003": "lib-3", "movie:tmdb:90000004": "lib-4",
	}
	seedChannel(t, inner, "c1", 5, ledgerFilms()...)
	if err := ledgerChannel(t, inner, avail, &now).Reconcile(ctx, "c1"); err != nil {
		t.Fatal(err)
	}

	now = now.Add(150 * time.Minute)
	rival := &rivalRedeal{Store: inner}
	if err := ledgerChannel(t, rival, avail, &now).Reconcile(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if !rival.done {
		t.Fatal("fixture: the rival never re-dealt")
	}
	winner, err := inner.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}

	got, err := inner.LastScheduledByChannel(ctx, "c1", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := startedPrograms(winner, now)
	if len(want) == 0 {
		t.Fatal("fixture: the winning cycle has started nothing")
	}
	for id, at := range got {
		if w, ok := want[id]; !ok || !w.Equal(at) {
			t.Errorf("ledger has %s at %v, which the winning cycle did not air then (want %v, present %v)", id, at, w, ok)
		}
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("the winning cycle aired %s but the ledger lacks it", id)
		}
	}
}

// A reconcile that stops on a failed read records nothing; the next one that commits records the
// same rows, stamped at each programme's start, so recency is unchanged.
func TestReconcileThatAbortsRecordsNothing(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	for i, key := range []provision.Key{"movie:tmdb:90000001", "movie:tmdb:90000002", "movie:tmdb:90000003", "movie:tmdb:90000004"} {
		seedAvailableTitle(t, st, key, provision.Title{MediaType: provision.Movie, TMDBID: 90000001 + i}, "lib-"+string(rune('1'+i)))
	}
	seedChannel(t, st, "c1", 5, ledgerFilms()...)
	titles := &failingTitles{AvailabilityStore: st}
	fresh := func() *channels.Engine {
		return ledgerChannel(t, st, channels.NewStoreAvailability(ctx, titles, nil, nil), &now)
	}
	if err := fresh().Reconcile(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	accepted, err := st.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(150 * time.Minute)
	titles.fail.Store(true)
	if err := fresh().Reconcile(ctx, "c1"); err == nil {
		t.Fatal("fixture: the reconcile did not stop on the failed read")
	}
	if got, err := st.LastScheduledByChannel(ctx, "c1", now.Add(time.Hour)); err != nil || len(got) != 0 {
		t.Fatalf("an aborted reconcile recorded %v (err %v), want nothing", got, err)
	}

	titles.fail.Store(false)
	if err := fresh().Reconcile(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	got, err := st.LastScheduledByChannel(ctx, "c1", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := startedPrograms(accepted, now)
	if len(want) == 0 || len(got) != len(want) {
		t.Fatalf("ledger = %v, want %v", got, want)
	}
	for id, at := range want {
		if !got[id].Equal(at) {
			t.Errorf("%s recorded at %v, want its start %v", id, got[id], at)
		}
	}
}
