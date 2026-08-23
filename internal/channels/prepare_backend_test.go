package channels_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestPrepareInheritedBackendUsesSuppliedTarget(t *testing.T) {
	tests := []struct {
		name          string
		target        string
		applied       string
		wantProjected bool
	}{
		{
			name:    "internal target while Tunarr remains applied",
			target:  schedule.PlayoutBackendInternal,
			applied: schedule.PlayoutBackendTunarr,
		},
		{
			name:          "Tunarr target while internal remains applied",
			target:        schedule.PlayoutBackendTunarr,
			applied:       schedule.PlayoutBackendInternal,
			wantProjected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newStore(t)
			tun := testkit.NewTunarr()
			e := newEngineForBackend(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, nil,
				func() string { return tt.applied })
			seedChannel(t, st, "inherited", 10, entry("movie:tmdb:1", "Movie"))

			if err := e.PrepareInheritedBackend(context.Background(), tt.target); err != nil {
				t.Fatal(err)
			}
			got, err := st.GetChannel(context.Background(), "inherited")
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != schedule.StatusLive || len(got.Desired) != 1 {
				t.Fatalf("prepared channel = status %q desired %+v", got.Status, got.Desired)
			}
			if projected := got.TunarrID != ""; projected != tt.wantProjected {
				t.Fatalf("Tunarr projection = %v, want %v (id %q)", projected, tt.wantProjected, got.TunarrID)
			}
			if got := tun.Creates; got != btoi(tt.wantProjected) {
				t.Fatalf("Tunarr creates = %d, want %d", got, btoi(tt.wantProjected))
			}
		})
	}
}

func TestPrepareInheritedBackendSkipsPinsAndInactiveChannels(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	e := newEngineForBackend(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, nil,
		func() string { return schedule.PlayoutBackendInternal })

	type candidate struct {
		id      string
		status  schedule.ChannelStatus
		backend string
	}
	candidates := []candidate{
		{id: "inherited", status: schedule.StatusBuilding},
		{id: "pinned-internal", status: schedule.StatusBuilding, backend: schedule.PlayoutBackendInternal},
		{id: "pinned-tunarr", status: schedule.StatusBuilding, backend: schedule.PlayoutBackendTunarr},
		{id: "paused", status: schedule.StatusPaused},
		{id: "detached", status: schedule.StatusDetached},
	}
	before := make(map[string]int64, len(candidates))
	for i, candidate := range candidates {
		seedChannel(t, st, candidate.id, i+1, entry("movie:tmdb:1", "Movie"))
		ch, err := st.GetChannel(context.Background(), candidate.id)
		if err != nil {
			t.Fatal(err)
		}
		ch.Status = candidate.status
		if candidate.backend != "" {
			ch.Policy.Playout = &schedule.PlayoutPolicy{Backend: candidate.backend}
		}
		ch, err = st.SaveChannel(context.Background(), ch)
		if err != nil {
			t.Fatal(err)
		}
		before[candidate.id] = ch.Revision
	}

	if err := e.PrepareInheritedBackend(context.Background(), schedule.PlayoutBackendTunarr); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		got, err := st.GetChannel(context.Background(), candidate.id)
		if err != nil {
			t.Fatal(err)
		}
		if candidate.id == "inherited" {
			if got.TunarrID == "" || got.Status != schedule.StatusLive {
				t.Fatalf("inherited channel was not prepared: %+v", got)
			}
			continue
		}
		if got.Revision != before[candidate.id] {
			t.Fatalf("%s revision changed from %d to %d", candidate.id, before[candidate.id], got.Revision)
		}
	}
	if tun.Creates != 1 {
		t.Fatalf("Tunarr creates = %d, want only the inherited channel", tun.Creates)
	}
}

func TestPrepareInheritedBackendHonorsPinAddedAfterFleetSnapshot(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	firstProjectionStarted := make(chan struct{})
	releaseFirstProjection := make(chan struct{})
	var once sync.Once
	tun.BeforeSetLineup = func(_ string, _ []schedule.Slot) {
		once.Do(func() {
			close(firstProjectionStarted)
			<-releaseFirstProjection
		})
	}
	e := newEngineForBackend(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, nil,
		func() string { return schedule.PlayoutBackendInternal })
	seedChannel(t, st, "first", 1, entry("movie:tmdb:1", "Movie"))
	seedChannel(t, st, "pin-during-prepare", 2, entry("movie:tmdb:1", "Movie"))

	done := make(chan error, 1)
	go func() {
		done <- e.PrepareInheritedBackend(context.Background(), schedule.PlayoutBackendTunarr)
	}()
	<-firstProjectionStarted
	pinned, err := st.GetChannel(context.Background(), "pin-during-prepare")
	if err != nil {
		t.Fatal(err)
	}
	pinned.Policy.Playout = &schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal}
	pinned, err = st.SaveChannel(context.Background(), pinned)
	if err != nil {
		t.Fatal(err)
	}
	close(releaseFirstProjection)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	got, err := st.GetChannel(context.Background(), pinned.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != pinned.Revision || got.TunarrID != "" || got.Status != schedule.StatusBuilding {
		t.Fatalf("concurrent pin was not an authoritative skip: %+v", got)
	}
	if tun.Creates != 1 {
		t.Fatalf("Tunarr creates = %d, want only the first inherited channel", tun.Creates)
	}
}

func TestPrepareInheritedBackendContinuesAfterFailureAndRetryIsIdempotent(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	tun.SetLineupErrByChannel = map[string]error{"srv-1": errors.New("scripted lineup failure")}
	e := newEngineForBackend(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, nil,
		func() string { return schedule.PlayoutBackendInternal })
	seedChannel(t, st, "fails", 1, entry("movie:tmdb:1", "Movie"))
	seedChannel(t, st, "succeeds", 2, entry("movie:tmdb:1", "Movie"))

	err := e.PrepareInheritedBackend(context.Background(), schedule.PlayoutBackendTunarr)
	if err == nil || !strings.Contains(err.Error(), "prepare inherited channel fails") {
		t.Fatalf("partial prepare error = %v", err)
	}
	good, err := st.GetChannel(context.Background(), "succeeds")
	if err != nil {
		t.Fatal(err)
	}
	if good.Status != schedule.StatusLive || good.TunarrID == "" {
		t.Fatalf("later channel did not prepare after peer failure: %+v", good)
	}
	if tun.Creates != 2 || tun.Pushes != 1 {
		t.Fatalf("partial prepare effects = %d creates/%d pushes, want 2/1", tun.Creates, tun.Pushes)
	}

	delete(tun.SetLineupErrByChannel, "srv-1")
	if err := e.PrepareInheritedBackend(context.Background(), schedule.PlayoutBackendTunarr); err != nil {
		t.Fatal(err)
	}
	if tun.Creates != 2 || tun.Pushes != 2 {
		t.Fatalf("retry effects = %d creates/%d pushes, want 2/2", tun.Creates, tun.Pushes)
	}
	if err := e.PrepareInheritedBackend(context.Background(), schedule.PlayoutBackendTunarr); err != nil {
		t.Fatal(err)
	}
	if tun.Creates != 2 || tun.Pushes != 2 {
		t.Fatalf("idempotent pass effects = %d creates/%d pushes, want 2/2", tun.Creates, tun.Pushes)
	}
}

func TestPrepareInheritedBackendSerializesWithOrdinaryReconcile(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	projectionStarted := make(chan struct{})
	releaseProjection := make(chan struct{})
	var once sync.Once
	tun.BeforeSetLineup = func(_ string, _ []schedule.Slot) {
		once.Do(func() {
			close(projectionStarted)
			<-releaseProjection
		})
	}
	resolverEntered := make(chan struct{})
	var resolverOnce sync.Once
	e := newEngineForBackend(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, nil, func() string {
		resolverOnce.Do(func() { close(resolverEntered) })
		return schedule.PlayoutBackendInternal
	})
	seedChannel(t, st, "concurrent", 10, entry("movie:tmdb:1", "Movie"))

	prepareDone := make(chan error, 1)
	go func() {
		prepareDone <- e.PrepareInheritedBackend(context.Background(), schedule.PlayoutBackendTunarr)
	}()
	<-projectionStarted
	reconcileDone := make(chan error, 1)
	go func() { reconcileDone <- e.Reconcile(context.Background(), "concurrent") }()

	select {
	case <-resolverEntered:
		t.Fatal("ordinary reconcile crossed the per-channel lock during target preparation")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseProjection)
	if err := <-prepareDone; err != nil {
		t.Fatal(err)
	}
	if err := <-reconcileDone; err != nil {
		t.Fatal(err)
	}
	got, err := st.GetChannel(context.Background(), "concurrent")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != schedule.StatusLive || len(got.Desired) != 1 || got.TunarrID == "" {
		t.Fatalf("concurrent operations left an invalid channel: %+v", got)
	}
	if tun.Creates != 1 || tun.Pushes != 1 {
		t.Fatalf("concurrent effects = %d creates/%d pushes, want 1/1", tun.Creates, tun.Pushes)
	}
}

func TestPrepareInheritedBackendRejectsUnknownTarget(t *testing.T) {
	st := newStore(t)
	e := newEngineForBackend(st, testkit.NewTunarr(), mapAvail{}, nil, func() string {
		return schedule.PlayoutBackendInternal
	})
	if err := e.PrepareInheritedBackend(context.Background(), "external"); err == nil {
		t.Fatal("unknown backend was accepted")
	}
}

func btoi(v bool) int {
	if v {
		return 1
	}
	return 0
}
