package storeadapter_test

import (
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// catalogStore holds one playable, one held, one removed and one composite clip.
func catalogStore(t *testing.T) records.Store {
	t.Helper()
	st := testkit.MigratedSQLiteStore(t)
	for _, name := range []string{"ready", "held", "removed", "reel"} {
		clip := clipstore.Clip{Clip: clips.Clip{Hash: name, Path: name + ".mp4", Name: name, Role: clips.Commercial, DurationMs: 30_000}}
		if err := st.UpsertClip(t.Context(), clip); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.HoldClips(t.Context(), []string{"held.mp4"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetClipsRemoved(t.Context(), []string{"removed.mp4"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := st.SetClipComposite(t.Context(), "reel", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	return st
}

func hashes(t *testing.T, rows []clipstore.Clip, err error) []string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, row := range rows {
		out = append(out, row.Hash)
	}
	slices.Sort(out)
	return out
}

func sorted(paths []string, err error) []string {
	if err != nil {
		return []string{err.Error()}
	}
	slices.Sort(paths)
	return paths
}

func TestWorkListsTakeHeldClipsOnRequestAndNeverRemovedOnes(t *testing.T) {
	st := catalogStore(t)
	for name, list := range map[string]func(contextT, filler.ClipQuery) ([]clipstore.Clip, error){
		"airing":     storeadapter.NewAiringClips(st).ListClips,
		"language":   storeadapter.NewLanguage(st).ListClips,
		"transcribe": storeadapter.NewTranscribe(st).ListClips,
		"vision":     storeadapter.NewVision(st).ListClips,
	} {
		rows, err := list(t.Context(), filler.ClipQuery{IncludeHeld: true})
		if got := hashes(t, rows, err); !slices.Equal(got, []string{"held", "ready"}) {
			t.Errorf("%s with held = %v", name, got)
		}
		rows, err = list(t.Context(), filler.ClipQuery{})
		if got := hashes(t, rows, err); !slices.Equal(got, []string{"ready"}) {
			t.Errorf("%s default = %v", name, got)
		}
	}
}

func TestAbsentClipIsNotFoundRatherThanAnError(t *testing.T) {
	st := catalogStore(t)
	for name, get := range map[string]func(contextT, string) (clipstore.Clip, bool, error){
		"sync":     storeadapter.NewSync(st).GetClip,
		"pipeline": storeadapter.NewPipelineClips(st).GetClip,
		"defects":  storeadapter.NewDefects(st).GetClip,
		"upkeep":   storeadapter.NewUpkeep(st).GetClip,
	} {
		if _, found, err := get(t.Context(), "absent"); found || err != nil {
			t.Errorf("%s absent: found=%v err=%v", name, found, err)
		}
		if clip, found, err := get(t.Context(), "ready"); !found || err != nil || clip.Hash != "ready" {
			t.Errorf("%s present: clip=%q found=%v err=%v", name, clip.Hash, found, err)
		}
	}
}

func TestCatalogViewsDifferInWhatTheyCount(t *testing.T) {
	st := catalogStore(t)
	ctx := t.Context()
	every := []string{"held.mp4", "ready.mp4", "reel.mp4", "removed.mp4"}
	if got := sorted(storeadapter.NewDefects(st).ListClipPaths(ctx)); !slices.Equal(got, every) {
		t.Errorf("defect paths = %v", got)
	}
	if got := sorted(storeadapter.NewUpkeep(st).ListClipPaths(ctx)); !slices.Equal(got, every) {
		t.Errorf("upkeep paths = %v", got)
	}
	// Auto-fetch must see held and removed clips, or it downloads them again.
	noReels := []string{"held.mp4", "ready.mp4", "removed.mp4"}
	if got := sorted(storeadapter.NewFetch(st, nil, nil).CatalogPaths(ctx)); !slices.Equal(got, noReels) {
		t.Errorf("fetch paths = %v", got)
	}
	ready, err := storeadapter.NewUpkeep(st).ListReadyClips(ctx)
	if got := hashes(t, ready, err); !slices.Equal(got, []string{"ready"}) {
		t.Errorf("ready clips = %v", got)
	}
	// A held clip never enters a pod.
	pods, err := storeadapter.NewCatalog(st).AllClips(ctx)
	if err != nil || len(pods) != 1 || pods[0].Hash != "ready" {
		t.Errorf("pod catalog = %+v err=%v", pods, err)
	}
	identities, err := storeadapter.NewIdentity(st).ListIdentityClips(ctx)
	if err != nil || len(identities) != 4 {
		t.Errorf("identity clips = %d err=%v", len(identities), err)
	}
}
