package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type recordingFillerChannelReconciler struct{ ids []string }

func (r *recordingFillerChannelReconciler) Reconcile(_ context.Context, id string) error {
	r.ids = append(r.ids, id)
	return nil
}

type fillerAdmissionAvailability map[provision.Key]string

func (a fillerAdmissionAvailability) Resolve(key provision.Key) (string, int64, bool) {
	id, ok := a[key]
	return id, time.Hour.Milliseconds(), ok
}

func (fillerAdmissionAvailability) ResolveEpisodes(provision.Key) ([]schedule.ResolvedProgram, bool) {
	return nil, false
}

// The score rung and the legacy tag sweep both file through fillerTagStoreAdapter. That admission
// must have the same scheduling consequence as POST /v1/filler/file or a fully-grounded automatic
// download can become eligible while every internal channel keeps its old no-break cycle.
func TestAutomaticFillerAdmission_ReconcilesActiveChannelsImmediately(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	clip := filler.Clip{
		Hash: "clip", Path: "clip.mp4", Name: "Clip", Kind: filler.Commercial,
		DurationMs: 30_000, Era: 1990, Audience: filler.Kids, Category: "toys", Held: true,
	}
	if err := st.UpsertClip(t.Context(), store.Clip{Clip: clip, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	for _, ch := range []store.Channel{
		{Channel: schedule.Channel{ID: "live", Name: "Live", Number: 1, Status: schedule.StatusLive}},
		{Channel: schedule.Channel{ID: "paused", Name: "Paused", Number: 2, Status: schedule.StatusPaused}},
	} {
		if _, err := st.SaveChannel(t.Context(), ch); err != nil {
			t.Fatal(err)
		}
	}

	reconciler := &recordingFillerChannelReconciler{}
	adapter := fillerTagStoreAdapter{
		st: st,
		wake: &fillerChannelWake{
			st: st, channels: reconciler,
		},
	}
	if _, err := adapter.SetClipsHeld(t.Context(), []string{clip.Path}, false, true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if len(reconciler.ids) != 1 || reconciler.ids[0] != "live" {
		t.Fatalf("reconciled = %v, want only the active channel", reconciler.ids)
	}
}

func TestFillerSourceAutoAdmit_UsesExactPolicyAndFailsClosed(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	ctx := t.Context()
	trusted := store.NewFillerSource("archive:trusted", "archive", "trusted", "Trusted", time.Now().UTC())
	review := store.NewFillerSource("archive:review", "archive", "review", "Review", time.Now().UTC())
	for _, src := range []store.FillerSource{trusted, review} {
		if err := st.UpsertFillerSource(ctx, src); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetFillerSourceAutoAdmit(ctx, review.ID, false); err != nil {
		t.Fatal(err)
	}
	allows := fillerSourceAutoAdmit(st)
	for source, want := range map[string]bool{
		trusted.ID: true,
		review.ID:  false,
		"missing":  false,
		"":         true, // manual URL/legacy fetched clips resolve through the seeded folder policy
	} {
		got, err := allows(ctx, source)
		if err != nil {
			t.Fatalf("source %q: %v", source, err)
		}
		if got != want {
			t.Errorf("source %q allowed = %v, want %v", source, got, want)
		}
	}
}

// Full internal-playout proof of the beta symptom: the first reconcile has no eligible pool and
// therefore no break slots; filing the downloaded clip through the production adapter wakes the
// real channel engine, whose persisted desired cycle immediately gains a filler transition.
func TestAutomaticFillerAdmission_InsertsBreakIntoInternalSchedule(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	clip := filler.Clip{
		Hash: "clip", Path: "clip.mp4", Name: "Clip", Kind: filler.Commercial,
		DurationMs: 30_000, Era: 1990, Audience: filler.Kids, Category: "toys", Held: true,
	}
	if err := st.UpsertClip(t.Context(), store.Clip{Clip: clip, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	ch := store.Channel{
		Channel: schedule.Channel{
			ID: "internal", Name: "Internal", Number: 1, Strategy: schedule.Sequential,
			Status: schedule.StatusBuilding,
		},
		Lineup: []schedule.LineupEntry{
			{Key: provision.Key("movie:tmdb:1"), Title: "One", DurationMs: time.Hour.Milliseconds()},
			{Key: provision.Key("movie:tmdb:2"), Title: "Two", DurationMs: time.Hour.Milliseconds()},
		},
	}
	if _, err := st.SaveChannel(t.Context(), ch); err != nil {
		t.Fatal(err)
	}

	pods := filler.NewPodAdapter(clipCatalogAdapter{st: st}, nil, func() filler.Policy {
		return filler.Policy{PodMax: 4, BreakDurationMs: 30_000}
	}, slog.New(slog.DiscardHandler))
	engine := channels.New(st, nil, fillerAdmissionAvailability{
		provision.Key("movie:tmdb:1"): "lib-1",
		provision.Key("movie:tmdb:2"): "lib-2",
	}, nil, channels.Config{
		ReconcileTTL: 10 * time.Minute, BreaksPerHour: 30,
		ResolvePlayoutBackendContext: func(context.Context) (string, error) {
			return schedule.PlayoutBackendInternal, nil
		},
	}, func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, slog.New(slog.DiscardHandler)).WithPods(pods)

	if err := engine.Reconcile(t.Context(), ch.ID); err != nil {
		t.Fatal(err)
	}
	assertBreaks := func(want bool) {
		t.Helper()
		got, err := st.GetChannel(t.Context(), ch.ID)
		if err != nil {
			t.Fatal(err)
		}
		has := false
		for _, slot := range got.Desired {
			has = has || slot.Kind == schedule.SlotFiller
		}
		if has != want {
			t.Fatalf("has filler break = %v, want %v; desired = %+v", has, want, got.Desired)
		}
	}
	assertBreaks(false)

	adapter := fillerTagStoreAdapter{
		st: st,
		wake: &fillerChannelWake{
			st: st, channels: engine,
		},
	}
	if _, err := adapter.SetClipsHeld(t.Context(), []string{clip.Path}, false, true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	assertBreaks(true)
}
