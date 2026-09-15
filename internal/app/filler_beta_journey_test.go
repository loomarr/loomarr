package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// This is the household-beta journey at the application composition seam. The only substituted
// boundaries are yt-dlp, ffprobe, and ffmpeg artwork; acquisition manifests, intake, catalog sync,
// enrollment, the conveyor, terminal readiness, SQLite persistence, and Pod visibility are real.
func TestFillerBetaJourney_QueuedSourceItemResumesAfterRestartAndBecomesPlayable(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	clipDir := filepath.Join(root, "clips")
	watchDir := filepath.Join(root, "watch")
	for _, dir := range []string{clipDir, watchDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	layout, err := filler.NewLayout(clipDir, watchDir)
	if err != nil {
		t.Fatal(err)
	}
	dsn := "sqlite://" + filepath.Join(root, "loomarr.db")
	st, err := store.Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 14, 20, 0, 0, 0, time.UTC)
	clock := now
	if err := st.UpsertFillerSource(ctx, store.FillerSource{
		ID: "youtube:retro-toys", Kind: "youtube", URI: "https://youtube.com/@retro-toys/videos",
		Label: "Retro toy commercials", Enabled: true, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	ytdlp := testkit.Executable(t, "yt-dlp", `#!/bin/sh
archive=""; result=""
while test "$#" -gt 0; do
  case "$1" in
    --download-archive) archive="$2"; shift 2 ;;
    --print-to-file) result="$3"; shift 3 ;;
    *) shift ;;
  esac
done
stage=$(dirname "$result")
printf 'beta journey video bytes' > "$stage/retro-toy.mp4"
printf '%s\n' '{"id":"retro-toy","title":"Retro Toy Spot","license":"https://example.invalid/provider-license"}' > "$stage/retro-toy.info.json"
printf '%s\n' 'youtube retro-toy' > "$archive"
printf 'retro-toy\t"%s"\n' "$stage/retro-toy.mp4" >> "$result"
`)
	scanProbe := func(context.Context, string) (filler.Probed, error) {
		return filler.Probed{DurationMs: 30_000, Height: 480}, nil
	}
	artwork := func(_ context.Context, _, stillDst, animDst string, _ float64) error {
		if err := os.WriteFile(stillDst, []byte("still"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(animDst, []byte("animation"), 0o600)
	}

	probeCalls := 0
	interruptedProbe := func(context.Context, string) (filler.Probed, error) {
		probeCalls++
		if probeCalls == 1 {
			return filler.Probed{}, errors.New("decoder temporarily unavailable")
		}
		return filler.Probed{DurationMs: 30_000, Height: 480}, nil
	}
	newPipeline := func(database store.Store, probe filler.Prober) *filler.Pipeline {
		clipStore := fillerPipelineClipAdapter{st: database}
		return filler.NewPipeline(database, clipStore, []filler.Stage{
			filler.NewProbeStage(probe, clipStore, layout.ClipDir(), func() int64 { return 10_000 }, nil, func() time.Time { return clock }),
			filler.NewScoreStage(fillerTagStoreAdapter{st: database}, nil, func() time.Time { return clock }),
		}, filler.DefaultBudget(), nil, func() time.Time { return clock }, nil).
			WithRewind(fillerRewindAdapter{st: database}, layout.ClipDir())
	}
	pipeline := newPipeline(st, interruptedProbe)
	syncer := filler.NewSyncer(filler.DirSource{
		Layout: layout, Probe: scanProbe, Artwork: artwork,
	}, fillerStoreAdapter{st: st}, layout, func() time.Time { return now }, nil).
		WithAcquisitionManifests(st)
	fetcher := clipfetch.New(clipfetch.NewYtDlpDownloader(ytdlp, "ffmpeg"), nil, layout.WatchDir(), nil).
		WithArtifactWriter(st)
	adapter := fillerServiceAdapter{
		fetcher: fetcher, acquisitions: st, sources: st,
		newID:   func() string { return "acq-beta-journey" },
		now:     func() time.Time { return now },
		timeout: time.Minute,
		start:   testInteractiveOperationLauncher,
		afterIngest: func(operationCtx context.Context) error {
			if _, syncErr := syncer.Sync(operationCtx); syncErr != nil {
				return syncErr
			}
			_, enrolErr := pipeline.EnrolMissing(operationCtx)
			return enrolErr
		},
	}

	jobID, err := adapter.IngestSourceItems(ctx, "youtube:retro-toys", "youtube", []filler.DiscoveredRef{{
		ID: "retro-toy", URL: "https://youtube.com/watch?v=retro-toy",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if jobID != "acq-beta-journey" {
		t.Fatalf("job id = %q, want acq-beta-journey", jobID)
	}
	waitForAcquisition(t, st, jobID, now, filler.AcquisitionSuccess)

	held, err := st.ListClips(ctx, store.ClipFilter{IncludeHeld: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || !held[0].Held {
		t.Fatalf("catalog after acquisition = %+v, want one held clip", held)
	}
	if held[0].Kind != filler.Unclassified {
		t.Fatalf("downloaded clip kind = %q, want descriptively unclassified", held[0].Kind)
	}
	if playable, err := (clipCatalogAdapter{st: st}).AllClips(ctx); err != nil || len(playable) != 0 {
		t.Fatalf("playable catalog before readiness = %+v, %v; want empty", playable, err)
	}
	row, found, err := st.GetClipPipeline(ctx, held[0].Hash)
	if err != nil || !found || row.AcquisitionID != jobID || row.Disposition != filler.DispositionRunning {
		t.Fatalf("enrolled conveyor = %+v, found=%v, err=%v", row, found, err)
	}
	failed, err := pipeline.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Advanced != 1 || failed.Completed != 0 || failed.Overview.Scheduled != 1 {
		t.Fatalf("interrupted first pass = %+v, want one durable retry", failed)
	}
	row, found, err = st.GetClipPipeline(ctx, held[0].Hash)
	if err != nil || !found || row.Status != filler.StatusFailed || row.Attempts != 1 || !row.NextRun.After(clock) {
		t.Fatalf("retry state = %+v, found=%v, err=%v", row, found, err)
	}

	// Stop after a failed required check, then reopen the same database after its backoff. The new
	// application generation must discover the durable retry rather than relying on memory.
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	clock = now.Add(6 * time.Minute)
	reopened, err := store.Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := newPipeline(reopened, scanProbe)
	result, err := restarted.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Completed != 1 || result.Failed != 0 || result.Rejected != 0 {
		t.Fatalf("restarted pipeline = %+v, want one automatic Ready completion", result)
	}

	playable, err := (clipCatalogAdapter{st: reopened}).AllClips(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(playable) != 1 || playable[0].Held || playable[0].Kind != filler.Unclassified || playable[0].Placement != filler.PlacementBreakBody {
		t.Fatalf("playable catalog = %+v, want one unclassified break-body Ready clip", playable)
	}
	event, found, err := reopened.GetFillerReadyEvent(ctx, playable[0].Hash)
	if err != nil || !found || event.AcquisitionID != jobID || event.Enrollment.Kind != filler.EnrollmentAcquisition {
		t.Fatalf("Ready event = %+v, found=%v, err=%v", event, found, err)
	}
	pool, err := filler.NewPodAdapter(clipCatalogAdapter{st: reopened}, nil, nil, nil).PoolCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pool.Clips != 1 || pool.BreakBody != 1 || pool.Eligible != 1 {
		t.Fatalf("Pod pool = %+v, want the Ready clip to be eligible", pool)
	}
	run, err := reopened.GetAcquisitionRun(ctx, jobID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if run.Outcome.Enrolled != 1 || run.Outcome.Ready != 1 || run.Outcome.Preparing != 0 || run.Outcome.NeedsDecision != 0 {
		t.Fatalf("acquisition outcome = %+v, want hands-off Ready", run.Outcome)
	}

	// A later scheduled pass is idempotent: no duplicate Ready event and no reopened work.
	if again, err := restarted.RunOnce(ctx); err != nil || again.Completed != 0 || again.Advanced != 0 {
		t.Fatalf("second pass = %+v, err=%v; want settled conveyor", again, err)
	}
	again, found, err := reopened.GetFillerReadyEvent(ctx, playable[0].Hash)
	if err != nil || !found || again != event {
		t.Fatalf("Ready event changed on retry: before=%+v after=%+v found=%v err=%v", event, again, found, err)
	}
}

func waitForAcquisition(t *testing.T, st store.Store, id string, at time.Time, want filler.AcquisitionStatus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		run, err := st.GetAcquisitionRun(t.Context(), id, at)
		if err == nil && run.Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("acquisition %s did not reach %s: last=%+v err=%v", id, want, run, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
