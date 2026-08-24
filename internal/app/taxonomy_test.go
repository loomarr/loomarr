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
	"github.com/loomarr/loomarr/internal/taxonomy"
	"github.com/loomarr/loomarr/internal/testkit"
)

type taxonomyWakeRecorder struct {
	snapshots []filler.Clip
}

func (*taxonomyWakeRecorder) Reconcile(context.Context, string) error { return nil }

func (r *taxonomyWakeRecorder) ReconcileFillerChange(_ context.Context, snapshots []filler.Clip) error {
	r.snapshots = append(r.snapshots, snapshots...)
	return nil
}

func TestTaxonomyEditor_PreviewsSavedSelectionsAndConvergesCommittedEligibility(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	clip := filler.Clip{
		Hash: "beer-clip", Path: "beer-clip.mp4", Name: "Beer clip", Kind: filler.Commercial,
		DurationMs: 30_000, Era: 1994, Audience: filler.General,
	}
	if err := st.UpsertClip(t.Context(), store.Clip{Clip: clip, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetClipTags(t.Context(), clip.Hash, []string{"beer"}); err != nil {
		t.Fatal(err)
	}
	for _, channel := range []store.Channel{
		{
			Channel: schedule.Channel{ID: "direct", Name: "Direct", Number: 20, Status: schedule.StatusLive},
			Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{
				Filler: &schedule.FillerSelection{Categories: []string{"beer"}},
			}},
		},
		{
			Channel: schedule.Channel{ID: "other", Name: "Other", Number: 10, Status: schedule.StatusLive},
			Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{
				Filler: &schedule.FillerSelection{Categories: []string{"cereal"}},
			}},
		},
	} {
		if _, err := st.SaveChannel(t.Context(), channel); err != nil {
			t.Fatal(err)
		}
	}

	recorder := &taxonomyWakeRecorder{}
	editor := taxonomyEditor{
		store: st,
		wake:  &fillerChannelWake{st: st, channels: recorder, log: slog.New(slog.DiscardHandler)},
		now:   func() time.Time { return now },
	}
	beer, ok := taxonomy.New(mustTaxa(t, st)).Get("beer")
	if !ok {
		t.Fatal("seed taxonomy has no beer")
	}
	beer.Parent = "food"
	edit := store.TaxonomyEdit{Slug: beer.Slug, Taxon: beer}

	preview, err := editor.Preview(t.Context(), edit)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Channels) != 1 || preview.Channels[0].ID != "direct" {
		t.Fatalf("saved selection impact = %+v, want only direct", preview.Channels)
	}
	if len(preview.Store.PlayableClipHashes) != 1 || preview.Store.PlayableClipHashes[0] != clip.Hash {
		t.Fatalf("playable impact = %v, want [%s]", preview.Store.PlayableClipHashes, clip.Hash)
	}
	if len(recorder.snapshots) != 0 {
		t.Fatal("preview woke channels")
	}

	if _, err := editor.Apply(t.Context(), edit); err != nil {
		t.Fatal(err)
	}
	if len(recorder.snapshots) != 2 {
		t.Fatalf("committed edit wake snapshots = %d, want before + after", len(recorder.snapshots))
	}
	if !containsString(recorder.snapshots[0].Tags, "alcohol") || containsString(recorder.snapshots[1].Tags, "alcohol") || !containsString(recorder.snapshots[1].Tags, "food") {
		t.Fatalf("wake snapshots did not carry old/new eligibility: before=%v after=%v", recorder.snapshots[0].Tags, recorder.snapshots[1].Tags)
	}
}

func TestTaxonomyEditor_CommittedLineageChangeReconcilesTheRealChannelPool(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	clip := filler.Clip{
		Hash: "beer-clip", Path: "beer-clip.mp4", Name: "Beer clip", Kind: filler.Commercial,
		DurationMs: 30_000, Era: 1994, Audience: filler.General,
	}
	if err := st.UpsertClip(t.Context(), store.Clip{Clip: clip, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetClipTags(t.Context(), clip.Hash, []string{"beer"}); err != nil {
		t.Fatal(err)
	}
	channel := store.Channel{
		Channel: schedule.Channel{
			ID: "alcohol-channel", Name: "Alcohol channel", Number: 31,
			Strategy: schedule.Sequential, Status: schedule.StatusBuilding,
		},
		Lineup: []schedule.LineupEntry{
			{Key: provision.Key("movie:tmdb:1"), Title: "One", DurationMs: time.Hour.Milliseconds()},
			{Key: provision.Key("movie:tmdb:2"), Title: "Two", DurationMs: time.Hour.Milliseconds()},
		},
		Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{
			Filler: &schedule.FillerSelection{Categories: []string{"alcohol"}},
		}},
	}
	if _, err := st.SaveChannel(t.Context(), channel); err != nil {
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
	}, func() time.Time { return now }, slog.New(slog.DiscardHandler)).WithPods(pods)
	if err := engine.Reconcile(t.Context(), channel.ID); err != nil {
		t.Fatal(err)
	}
	assertChannelHasFiller(t, st, channel.ID, true)

	beer, ok := taxonomy.New(mustTaxa(t, st)).Get("beer")
	if !ok {
		t.Fatal("seed taxonomy has no beer")
	}
	beer.Parent = "food"
	editor := taxonomyEditor{
		store: st,
		wake:  &fillerChannelWake{st: st, channels: engine, log: slog.New(slog.DiscardHandler)},
		now:   func() time.Time { return now.Add(time.Minute) },
	}
	if _, err := editor.Apply(t.Context(), store.TaxonomyEdit{Slug: beer.Slug, Taxon: beer}); err != nil {
		t.Fatal(err)
	}
	assertChannelHasFiller(t, st, channel.ID, false)
}

func TestTaxonomyEditor_ResolverOnlyEditDoesNotReconcileChannels(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	beer, ok := taxonomy.New(mustTaxa(t, st)).Get("beer")
	if !ok {
		t.Fatal("seed taxonomy has no beer")
	}
	beer.Synonyms = append(beer.Synonyms, "pint")
	recorder := &taxonomyWakeRecorder{}
	editor := taxonomyEditor{
		store: st,
		wake:  &fillerChannelWake{st: st, channels: recorder, log: slog.New(slog.DiscardHandler)},
	}
	if _, err := editor.Apply(t.Context(), store.TaxonomyEdit{Slug: beer.Slug, Taxon: beer}); err != nil {
		t.Fatal(err)
	}
	if len(recorder.snapshots) != 0 {
		t.Fatalf("resolver-only edit woke channel eligibility with snapshots %+v", recorder.snapshots)
	}
}

func assertChannelHasFiller(t *testing.T, st store.Store, channelID string, want bool) {
	t.Helper()
	channel, err := st.GetChannel(t.Context(), channelID)
	if err != nil {
		t.Fatal(err)
	}
	has := false
	for _, slot := range channel.Desired {
		has = has || slot.Kind == schedule.SlotFiller
	}
	if has != want {
		t.Fatalf("channel %s has filler = %v, want %v; desired=%+v", channelID, has, want, channel.Desired)
	}
}

func mustTaxa(t *testing.T, st store.Store) []taxonomy.Taxon {
	t.Helper()
	taxa, err := st.ListTaxa(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return taxa
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
