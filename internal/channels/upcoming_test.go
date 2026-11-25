package channels_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/tunarr/tunarrtest"
)

// upcomingCall is one IndexUpcoming call: the committed channel's revision and the horizon.
type upcomingCall struct {
	channelID string
	revision  int64
	from, to  time.Time
}

type recordingUpcoming struct {
	calls []upcomingCall
	err   error
}

func (r *recordingUpcoming) IndexUpcoming(_ context.Context, ch channelstore.Channel, from, to time.Time) error {
	r.calls = append(r.calls, upcomingCall{channelID: ch.ID, revision: ch.Revision, from: from, to: to})
	return r.err
}

// After a committed reconcile of an internal-playout channel, the upcoming index is asked for the
// next PremeasureHorizon at the committed revision (#4476).
func TestReconcile_IndexesUpcomingAfterTheCommit(t *testing.T) {
	st := newStore(t)
	e := newEngineForBackend(st, nil, mapAvail{"movie:tmdb:90000001": "lib-1"}, &fakeGuide{},
		func() string { return schedule.PlayoutBackendInternal })
	upcoming := &recordingUpcoming{}
	e.WithUpcomingIndex(upcoming)
	seedChannel(t, st, "up-internal", 6, entry("movie:tmdb:90000001", "A"))

	if err := e.Reconcile(context.Background(), "up-internal"); err != nil {
		t.Fatal(err)
	}
	ch, err := st.GetChannel(context.Background(), "up-internal")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	want := upcomingCall{channelID: ch.ID, revision: ch.Revision, from: now, to: now.Add(channels.PremeasureHorizon)}
	if len(upcoming.calls) != 1 || upcoming.calls[0].channelID != want.channelID || upcoming.calls[0].revision != want.revision ||
		!upcoming.calls[0].from.Equal(want.from) || !upcoming.calls[0].to.Equal(want.to) {
		t.Fatalf("upcoming index calls = %+v, want one %+v", upcoming.calls, want)
	}
}

// A failing upcoming index is logged; the reconcile still commits.
func TestReconcile_UpcomingIndexFailureDoesNotFailTheReconcile(t *testing.T) {
	st := newStore(t)
	e := newEngineForBackend(st, nil, mapAvail{"movie:tmdb:90000001": "lib-1"}, &fakeGuide{},
		func() string { return schedule.PlayoutBackendInternal })
	upcoming := &recordingUpcoming{err: errors.New("store unavailable")}
	e.WithUpcomingIndex(upcoming)
	seedChannel(t, st, "up-fails", 6, entry("movie:tmdb:90000001", "A"))

	if err := e.Reconcile(context.Background(), "up-fails"); err != nil {
		t.Fatalf("reconcile failed with the upcoming index: %v", err)
	}
	ch, err := st.GetChannel(context.Background(), "up-fails")
	if err != nil {
		t.Fatal(err)
	}
	if len(upcoming.calls) != 1 || len(ch.Desired) == 0 || ch.Status != schedule.StatusLive {
		t.Fatalf("after a failing index: %d calls, status %s, %d slots; want the reconcile committed live",
			len(upcoming.calls), ch.Status, len(ch.Desired))
	}
}

// A paused or detached channel is not reconciled, so nothing is indexed.
func TestReconcile_UpcomingIndexSkipsPausedAndDetached(t *testing.T) {
	for _, status := range []schedule.ChannelStatus{schedule.StatusPaused, schedule.StatusDetached} {
		t.Run(string(status), func(t *testing.T) {
			st := newStore(t)
			seedChannel(t, st, "up-inactive", 5, entry("movie:tmdb:90000001", "A"))
			ch, err := st.GetChannel(context.Background(), "up-inactive")
			if err != nil {
				t.Fatal(err)
			}
			ch.Status = status
			if _, err := st.SaveChannel(context.Background(), ch); err != nil {
				t.Fatal(err)
			}
			e := newEngineForBackend(st, nil, mapAvail{"movie:tmdb:90000001": "lib-1"}, &fakeGuide{},
				func() string { return schedule.PlayoutBackendInternal })
			upcoming := &recordingUpcoming{}
			e.WithUpcomingIndex(upcoming)
			if err := e.Reconcile(context.Background(), "up-inactive"); err != nil {
				t.Fatal(err)
			}
			if len(upcoming.calls) != 0 {
				t.Fatalf("%s channel indexed: %+v", status, upcoming.calls)
			}
		})
	}
}

// A Tunarr channel stores no upcoming airings, the same as the scheduled ledger.
func TestReconcile_UpcomingIndexSkipsTunarrChannels(t *testing.T) {
	st := newStore(t)
	e := newEngineForBackend(st, tunarrtest.NewTunarr(), mapAvail{"movie:tmdb:90000001": "lib-1"}, &fakeGuide{},
		func() string { return "tunarr" })
	upcoming := &recordingUpcoming{}
	e.WithUpcomingIndex(upcoming)
	seedChannel(t, st, "up-tunarr", 6, entry("movie:tmdb:90000001", "A"))

	if err := e.Reconcile(context.Background(), "up-tunarr"); err != nil {
		t.Fatal(err)
	}
	if len(upcoming.calls) != 0 {
		t.Fatalf("Tunarr channel indexed: %+v", upcoming.calls)
	}
}

// staleOnceStore commits a competing edit just before reconcile's first save, so that attempt
// loses its compare-and-swap and reconcile retries from the newer revision.
type staleOnceStore struct {
	store.Store
	raced bool
}

func (s *staleOnceStore) SaveChannel(ctx context.Context, ch channelstore.Channel) (channelstore.Channel, error) {
	if !s.raced && ch.Revision > 0 {
		s.raced = true
		edited, err := s.GetChannel(ctx, ch.ID)
		if err != nil {
			return channelstore.Channel{}, err
		}
		edited.Name = "Operator edit"
		if _, err := s.Store.SaveChannel(ctx, edited); err != nil {
			return channelstore.Channel{}, err
		}
	}
	return s.Store.SaveChannel(ctx, ch)
}

// An attempt whose save loses to a concurrent edit indexes nothing; only the winning retry does,
// at the revision it committed.
func TestReconcile_StaleAttemptDoesNotIndexUpcoming(t *testing.T) {
	inner := newStore(t)
	seedChannel(t, inner, "up-race", 6, entry("movie:tmdb:90000001", "A"))
	st := &staleOnceStore{Store: inner}
	e := newEngineForBackend(st, nil, mapAvail{"movie:tmdb:90000001": "lib-1"}, &fakeGuide{},
		func() string { return schedule.PlayoutBackendInternal })
	upcoming := &recordingUpcoming{}
	e.WithUpcomingIndex(upcoming)

	if err := e.Reconcile(context.Background(), "up-race"); err != nil {
		t.Fatal(err)
	}
	if !st.raced {
		t.Fatal("the competing edit never ran; the test proves nothing")
	}
	ch, err := inner.GetChannel(context.Background(), "up-race")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Name != "Operator edit" {
		t.Fatalf("the competing edit was lost: name %q", ch.Name)
	}
	if len(upcoming.calls) != 1 || upcoming.calls[0].revision != ch.Revision {
		t.Fatalf("upcoming index calls = %+v, want one at the winning revision %d", upcoming.calls, ch.Revision)
	}
}
