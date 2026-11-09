package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/testkit"
)

type conditioningRekeyCrashStore struct {
	st         records.Store
	afterRekey bool
	target     cliplibrary.StoreClip
}

func (s *conditioningRekeyCrashStore) GetClip(ctx context.Context, hash string) (cliplibrary.StoreClip, bool, error) {
	return storeadapter.NewPipelineClips(s.st).GetClip(ctx, hash)
}

func (s *conditioningRekeyCrashStore) ReplaceClipIdentity(ctx context.Context, oldHash string, target cliplibrary.StoreClip) error {
	s.target = target
	if s.afterRekey {
		if err := (storeadapter.NewPipelineClips(s.st)).ReplaceClipIdentity(ctx, oldHash, target); err != nil {
			return err
		}
	}
	return errors.New("injected conditioning transaction failure")
}

func (s *conditioningRekeyCrashStore) CommitConditioningPublication(ctx context.Context, publication asset.ConditioningPublication, target cliplibrary.StoreClip) error {
	s.target = target
	if s.afterRekey {
		if err := (storeadapter.NewPipelineClips(s.st)).CommitConditioningPublication(ctx, publication, target); err != nil {
			return err
		}
	}
	return errors.New("injected conditioning transaction failure")
}

func journeyConditioningMeasurement(lufs float64) tools.ConditioningMeasurement {
	available := func(ms int64) tools.OptionalMilliseconds {
		return tools.OptionalMilliseconds{Milliseconds: ms, Available: true}
	}
	return tools.ConditioningMeasurement{
		ContainerDurationMs: 30_000,
		Streams: []tools.ConditioningStream{
			{Kind: tools.StreamVideo, Index: 0, Start: available(0), Duration: available(30_000),
				Cadence: &tools.Rational{Numerator: 30_000, Denominator: 1001}},
			{Kind: tools.StreamAudio, Index: 1, Start: available(120), Duration: available(29_880)},
		},
		AVSkew: tools.ConditioningSkew{Start: available(120), End: available(0)},
		Loudness: tools.ConditioningLoudness{
			IntegratedLUFS: lufs, Available: true,
			TruePeak: tools.ConditioningTruePeak{State: tools.TruePeakFinite, DBTP: -2.1},
		},
		Quality: tools.MediaQuality{
			EvidenceVersion: tools.MediaQualityEvidenceV1,
			Provenance:      tools.MediaQualityProvenanceFFmpegDetectors,
			DurationMs:      30_000,
		},
		Cuts: []tools.ConditioningCutMeasurement{{Intended: tools.Interval{StartMs: 1_000, EndMs: 31_000}, Streams: []tools.ConditioningCutStream{
			{Kind: tools.StreamVideo, Index: 0, StartError: available(3), EndError: available(-4)},
			{Kind: tools.StreamAudio, Index: 1, StartError: available(5), EndError: available(-2)},
		}}},
	}
}

func TestFillerConditioningJourney_RestartDistinguishesPreAndPostRekeyPublication(t *testing.T) {
	runFillerConditioningRestartJourney(t, func(t *testing.T) records.Store { return testkit.MigratedSQLiteStore(t) })
}

func runFillerConditioningRestartJourney(t *testing.T, openStore func(*testing.T) records.Store) {
	for _, tc := range []struct {
		name       string
		afterRekey bool
	}{
		{name: "pre-rekey Sync target is safely reused"},
		{name: "post-rekey target clears its pending publication", afterRekey: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st := openStore(t)
			dir := t.TempDir()
			fixtureSuffix := "pre-rekey"
			if tc.afterRekey {
				fixtureSuffix = "post-rekey"
			}
			parentFull := filepath.Join(dir, "retained-parent.mp4")
			sourceFull := filepath.Join(dir, "reviewed-child.mkv")
			if err := os.WriteFile(parentFull, []byte("retained parent bytes "+fixtureSuffix), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(sourceFull, []byte("reviewed child source bytes "+fixtureSuffix), 0o600); err != nil {
				t.Fatal(err)
			}
			parentHash, err := asset.ClipID(parentFull)
			if err != nil {
				t.Fatal(err)
			}
			sourceHash, err := asset.ClipID(sourceFull)
			if err != nil {
				t.Fatal(err)
			}
			lineage := &asset.ConditioningLineage{
				ChildHash: sourceHash, ParentHash: parentHash,
				IntendedStartMs: 1_000, IntendedEndMs: 31_000,
			}
			if err := asset.WriteSidecarTags(sourceFull, asset.SidecarTags{ConditioningLineage: lineage}, false); err != nil {
				t.Fatal(err)
			}
			now := time.Unix(1_900_634_000, 0).UTC()
			for _, clip := range []clips.Clip{
				{Hash: parentHash, Path: filepath.Base(parentFull), Name: "Retained parent", IsComposite: true},
				{Hash: sourceHash, Path: filepath.Base(sourceFull), Name: "Reviewed child", Role: clips.Commercial, ParentHash: parentHash},
			} {
				if err := st.UpsertClip(ctx, clipstore.Clip{Clip: clip, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
				ClipHash: sourceHash, Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusRunning,
				Disposition: pipelinestate.DispositionRunning, EnrolledAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}

			before := journeyConditioningMeasurement(-28.4)
			after := journeyConditioningMeasurement(-23.1)
			for i := range after.Cuts[0].Streams {
				after.Cuts[0].Streams[i].StartError = tools.OptionalMilliseconds{}
				after.Cuts[0].Streams[i].EndError = tools.OptionalMilliseconds{}
			}
			ffmpeg := testkit.CopyingMediaExecutable(t, "ffmpeg", "conditioned target bytes "+fixtureSuffix)
			crashStore := &conditioningRekeyCrashStore{st: st, afterRekey: tc.afterRekey}
			newStage := func(clipStore filler.TranscodeClipStore) *filler.TranscodeStage {
				stage := filler.NewTranscodeStage(clipStore, func(context.Context, string) (tools.Probed, error) {
					return tools.Probed{DurationMs: 30_000, Height: 480}, nil
				}, dir, tools.DefaultMezzanine(), func() string { return ffmpeg }, func() float64 { return -23 }, func() time.Time { return now })
				stage.WithConditioning(func(_ context.Context, req tools.ConditioningRequest) (tools.ConditioningMeasurement, error) {
					if filepath.Base(req.Path) == filepath.Base(req.ParentPath) {
						t.Fatal("conditioning source and parent snapshots collapsed")
					}
					if filepath.Ext(req.Path) == ".mkv" {
						return before, nil
					}
					return after, nil
				})
				return stage
			}
			first := newStage(crashStore)
			out, err := first.Run(ctx, cliplibrary.StoreClip{Clip: clips.Clip{
				Hash: sourceHash, Path: filepath.Base(sourceFull), Name: "Reviewed child",
				Role: clips.Commercial, ParentHash: parentHash,
			}})
			if err == nil || crashStore.target.Hash == "" {
				t.Fatalf("crash result = %+v, target=%+v, err=%v; want retryable infrastructure error", out, crashStore.target, err)
			}
			targetFull := filepath.Join(dir, filepath.FromSlash(crashStore.target.Path))
			tags, ok := asset.ReadSidecarTags(targetFull)
			if !ok || tags.ConditioningPublication == nil {
				t.Fatalf("crash lost pending publication evidence: %+v, ok=%v", tags, ok)
			}
			if !tc.afterRekey {
				if source, err := st.GetClip(ctx, sourceHash); err != nil || source.Hash != sourceHash {
					t.Fatalf("transaction failure lost source evidence: %+v, %v", source, err)
				}
				if pipeline, found, err := st.GetClipPipeline(ctx, sourceHash); err != nil || !found || pipeline.Stage != pipelinestate.StageTranscode {
					t.Fatalf("transaction failure lost source reference: %+v, found=%v, err=%v", pipeline, found, err)
				}
			}

			if !tc.afterRekey {
				layout, err := cliplibrary.NewLayout(dir, "")
				if err != nil {
					t.Fatal(err)
				}
				source := cliplibrary.DirSource{Layout: layout,
					Probe: func(context.Context, string) (tools.Probed, error) {
						return tools.Probed{DurationMs: 30_000, Height: 480}, nil
					},
					Artwork: func(context.Context, string, string, string, float64) error { return nil },
				}
				if _, err := filler.NewSyncer(source, storeadapter.NewSync(st), layout, time.Now, nil).Sync(ctx); err != nil {
					t.Fatal(err)
				}
				if target, err := st.GetClip(ctx, crashStore.target.Hash); err != nil || !target.Held {
					t.Fatalf("Sync target = %+v, %v; want held reconstructed row", target, err)
				}
			}

			restartClip, err := st.GetClip(ctx, sourceHash)
			if tc.afterRekey {
				restartClip, err = st.GetClip(ctx, crashStore.target.Hash)
			}
			if err != nil {
				t.Fatal(err)
			}
			restarted := newStage(storeadapter.NewPipelineClips(st))
			result, err := restarted.Run(ctx, cliplibrary.StoreClip{Clip: restartClip.Clip, UpdatedAt: restartClip.UpdatedAt})
			if err != nil || result.Verdict != pipelinestate.VerdictContinue || result.Clip.Hash != crashStore.target.Hash {
				t.Fatalf("restart result = %+v, %v; want committed target %s", result, err, crashStore.target.Hash)
			}
			if _, err := st.GetClip(ctx, sourceHash); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("source catalog identity survived recovery: %v", err)
			}
			committedTags, ok := asset.ReadSidecarTags(targetFull)
			if !ok || committedTags.ConditioningPublication != nil {
				t.Fatalf("restart retained pending publication: %+v, ok=%v", committedTags.ConditioningPublication, ok)
			}
		})
	}
}

func TestFillerConditioningJourney_ConfirmedChildBecomesAdmissionReady(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	drop := t.TempDir()
	parentFull := filepath.Join(drop, filepath.FromSlash(compPath))
	if err := os.MkdirAll(filepath.Dir(parentFull), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parentFull, []byte("compilation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertClip(ctx, clipstore.Clip{Clip: clips.Clip{
		Hash: compHash, Path: compPath, Name: "Compilation", Role: clips.Commercial,
		Source: "test-folder",
	}, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	ffmpeg := testkit.CopyingMediaExecutable(t, "ffmpeg", "mezzanine")
	parentProbeCalls := 0
	probe := func(_ context.Context, path string) (tools.Probed, error) {
		if filepath.Clean(path) == filepath.Clean(parentFull) {
			parentProbeCalls++
			return tools.Probed{DurationMs: 61_000, Height: 480}, nil
		}
		return tools.Probed{DurationMs: 30_000, Height: 480}, nil
	}
	parentTranscodeCalls := 0
	parentProbe := filler.NewProbeStage(probe, storeadapter.NewPipelineClips(st), drop, nil,
		func() time.Duration { return 45 * time.Second }, time.Now)
	parentTranscode := filler.NewTranscodeStage(storeadapter.NewPipelineClips(st), probe, drop,
		tools.DefaultMezzanine(), func() string { parentTranscodeCalls++; return ffmpeg },
		func() float64 { return -23 }, time.Now)
	parentPipeline := filler.NewPipeline(st, storeadapter.NewPipelineClips(st), []filler.Stage{parentProbe, parentTranscode},
		filler.Budget{}, nil, time.Now, slog.New(slog.DiscardHandler))
	if err := st.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
		ClipHash: compHash, Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
		Disposition: pipelinestate.DispositionRunning,
		NextRun:     time.Now().UTC(), EnrolledAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := parentPipeline.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	parent, err := st.GetClip(ctx, compHash)
	if err != nil || !parent.IsComposite || parent.DurationMs != 61_000 || parentProbeCalls == 0 || parentTranscodeCalls != 0 {
		t.Fatalf("parent probe/composite skip = %+v, probes=%d transcodes=%d, err=%v",
			parent, parentProbeCalls, parentTranscodeCalls, err)
	}
	if parentBytes, err := os.ReadFile(parentFull); err != nil || string(parentBytes) != "compilation" {
		t.Fatalf("composite source changed before split: %q, %v", parentBytes, err)
	}

	splitter := filler.NewSplitter(storeadapter.NewSplit(st, nil), splitFakeTools{chapters: []tools.Chapter{
		{StartMs: 10_000, EndMs: 40_000, Title: "Reviewed advert"},
	}}, nil, drop, func() time.Duration { return 10 * time.Second },
		func() string { return "conditioning-proposal" }, time.Now, nil)
	proposal, err := splitter.Propose(ctx, compHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.HoldClips(ctx, []string{compPath}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
		ClipHash: compHash, Stage: pipelinestate.StageSplit, Status: pipelinestate.StatusQueued,
		Disposition: pipelinestate.DispositionReview, NextRun: time.Now().UTC(),
		EnrolledAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	transcode := filler.NewTranscodeStage(
		storeadapter.NewPipelineClips(st), probe, drop, tools.DefaultMezzanine(),
		func() string { return ffmpeg }, func() float64 { return -23 }, time.Now,
	)
	var measured []tools.ConditioningRequest
	var measuredParentBound bool
	var beforeSourceHash string
	transcode.WithConditioning(func(_ context.Context, req tools.ConditioningRequest) (tools.ConditioningMeasurement, error) {
		measured = append(measured, req)
		if len(measured) == 1 {
			parentID, parentErr := asset.ClipID(req.ParentPath)
			beforeSourceHash, _ = asset.ClipID(req.Path)
			measuredParentBound = parentErr == nil && parentID == compHash
			measurement := journeyConditioningMeasurement(-28.4)
			measurement.Cuts[0].Intended = req.IntendedCuts[0]
			return measurement, nil
		}
		measurement := journeyConditioningMeasurement(-23.1)
		measurement.Cuts[0].Intended = req.IntendedCuts[0]
		return measurement, nil
	})
	pipeline := filler.NewPipeline(st, storeadapter.NewPipelineClips(st), []filler.Stage{
		filler.NewProbeStage(probe, storeadapter.NewPipelineClips(st), drop, nil,
			func() time.Duration { return 45 * time.Second }, time.Now),
		transcode,
		filler.NewFinishStage(),
	}, filler.Budget{}, nil, time.Now, slog.New(slog.DiscardHandler))
	app := fillerServiceAdapter{
		splitter: splitter, splitClips: storeadapter.NewSplit(st, nil), pipeline: pipeline,
	}
	segments := append([]split.Segment(nil), proposal.Segments...)
	segments[0].Era = 1994
	segments[0].Tags = []string{"toys"}
	if err := app.ConfirmSplit(ctx, proposal.ID, segments); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	listed, err := st.ListClips(ctx, clipstore.ClipFilter{IncludeComposites: true, IncludeHeld: true})
	if err != nil {
		t.Fatal(err)
	}
	var child clipstore.Clip
	for _, clip := range listed {
		if clip.ParentHash == compHash {
			child = clip
		}
	}
	if child.Hash == "" || child.Hash == proposal.ClipHash || child.ParentHash != compHash {
		t.Fatalf("conditioned child = %+v", child)
	}
	row, found, err := st.GetClipPipeline(ctx, child.Hash)
	if err != nil || !found || row.Disposition != pipelinestate.DispositionReady || child.Held ||
		child.Placement != clips.PlacementBreakBody {
		t.Fatalf("child readiness = row:%+v found:%v held:%v placement:%q err:%v, want Ready break body",
			row, found, child.Held, child.Placement, err)
	}
	if len(measured) != 2 || !measuredParentBound || len(measured[0].IntendedCuts) != 1 ||
		measured[0].IntendedCuts[0] != (tools.Interval{StartMs: 10_000, EndMs: 40_000}) {
		t.Fatalf("conditioning requests = %+v", measured)
	}
	tags, ok := asset.ReadSidecarTags(filepath.Join(drop, filepath.FromSlash(child.Path)))
	if !ok || tags.Conditioning == nil || tags.Conditioning.BeforeRewriteHash != beforeSourceHash ||
		tags.Conditioning.BeforeRewrite.Loudness.IntegratedLUFS != -28.4 ||
		tags.Conditioning.AfterRewrite.Loudness.IntegratedLUFS != -23.1 ||
		tags.Conditioning.BeforeRewrite.Loudness.TruePeak.State != tools.TruePeakFinite ||
		tags.Conditioning.AfterRewrite.Loudness.TruePeak.State != tools.TruePeakFinite ||
		len(tags.Conditioning.DerivedParentEdgesAfterRewrite.Streams) != 2 {
		t.Fatalf("conditioned sidecar = %+v, ok=%v", tags.Conditioning, ok)
	}

}
