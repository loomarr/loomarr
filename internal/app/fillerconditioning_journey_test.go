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
	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type conditioningJourneySource struct{ dir string }

func (s conditioningJourneySource) EnsureLocalSource(context.Context, string) error { return nil }

func (s conditioningJourneySource) ListLocalClips(ctx context.Context) ([]filler.RawClip, error) {
	clips, _, err := filler.ScanDir(ctx, s.dir, func(context.Context, string) (filler.Probed, error) {
		return filler.Probed{DurationMs: 30_000, Height: 480}, nil
	})
	return clips, err
}

type conditioningRekeyCrashStore struct {
	st         store.Store
	afterRekey bool
	target     filler.StoreClip
}

func (s *conditioningRekeyCrashStore) GetClip(ctx context.Context, hash string) (filler.StoreClip, bool, error) {
	return fillerPipelineClipAdapter{st: s.st}.GetClip(ctx, hash)
}

func (s *conditioningRekeyCrashStore) ReplaceClipIdentity(ctx context.Context, oldHash string, target filler.StoreClip) error {
	s.target = target
	if s.afterRekey {
		if err := (fillerPipelineClipAdapter{st: s.st}).ReplaceClipIdentity(ctx, oldHash, target); err != nil {
			return err
		}
	}
	return errors.New("injected conditioning transaction failure")
}

func (s *conditioningRekeyCrashStore) CommitConditioningPublication(ctx context.Context, publication filler.ConditioningPublication, target filler.StoreClip) error {
	s.target = target
	if s.afterRekey {
		if err := (fillerPipelineClipAdapter{st: s.st}).CommitConditioningPublication(ctx, publication, target); err != nil {
			return err
		}
	}
	return errors.New("injected conditioning transaction failure")
}

func journeyConditioningMeasurement(lufs float64) mediatools.ConditioningMeasurement {
	available := func(ms int64) mediatools.OptionalMilliseconds {
		return mediatools.OptionalMilliseconds{Milliseconds: ms, Available: true}
	}
	return mediatools.ConditioningMeasurement{
		ContainerDurationMs: 30_000,
		Streams: []mediatools.ConditioningStream{
			{Kind: mediatools.StreamVideo, Index: 0, Start: available(0), Duration: available(30_000),
				Cadence: &mediatools.Rational{Numerator: 30_000, Denominator: 1001}},
			{Kind: mediatools.StreamAudio, Index: 1, Start: available(120), Duration: available(29_880)},
		},
		AVSkew: mediatools.ConditioningSkew{Start: available(120), End: available(0)},
		Loudness: mediatools.ConditioningLoudness{
			IntegratedLUFS: lufs, Available: true,
			TruePeak: mediatools.ConditioningTruePeak{State: mediatools.TruePeakFinite, DBTP: -2.1},
		},
		Quality: filler.MediaQuality{
			EvidenceVersion: mediatools.MediaQualityEvidenceV1,
			Provenance:      mediatools.MediaQualityProvenanceFFmpegDetectors,
			DurationMs:      30_000,
		},
		Cuts: []mediatools.ConditioningCutMeasurement{{Intended: mediatools.Interval{StartMs: 1_000, EndMs: 31_000}, Streams: []mediatools.ConditioningCutStream{
			{Kind: mediatools.StreamVideo, Index: 0, StartError: available(3), EndError: available(-4)},
			{Kind: mediatools.StreamAudio, Index: 1, StartError: available(5), EndError: available(-2)},
		}}},
	}
}

func TestFillerConditioningJourney_RestartDistinguishesPreAndPostRekeyPublication(t *testing.T) {
	runFillerConditioningRestartJourney(t, func(t *testing.T) store.Store { return testkit.MigratedSQLiteStore(t) })
}

func runFillerConditioningRestartJourney(t *testing.T, openStore func(*testing.T) store.Store) {
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
			parentHash, err := filler.ClipID(parentFull)
			if err != nil {
				t.Fatal(err)
			}
			sourceHash, err := filler.ClipID(sourceFull)
			if err != nil {
				t.Fatal(err)
			}
			lineage := &filler.ConditioningLineage{
				ChildHash: sourceHash, ParentHash: parentHash,
				IntendedStartMs: 1_000, IntendedEndMs: 31_000,
			}
			if err := filler.WriteSidecarTags(sourceFull, filler.SidecarTags{ConditioningLineage: lineage}, false); err != nil {
				t.Fatal(err)
			}
			now := time.Unix(1_900_634_000, 0).UTC()
			for _, clip := range []filler.Clip{
				{Hash: parentHash, Path: filepath.Base(parentFull), Name: "Retained parent", IsComposite: true},
				{Hash: sourceHash, Path: filepath.Base(sourceFull), Name: "Reviewed child", Kind: filler.Commercial, ParentHash: parentHash},
			} {
				if err := st.UpsertClip(ctx, store.Clip{Clip: clip, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.UpsertClipPipeline(ctx, filler.ClipPipeline{
				ClipHash: sourceHash, Stage: filler.StageTranscode, Status: filler.StatusRunning,
				Disposition: filler.DispositionRunning, EnrolledAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}

			before := journeyConditioningMeasurement(-28.4)
			after := journeyConditioningMeasurement(-23.1)
			for i := range after.Cuts[0].Streams {
				after.Cuts[0].Streams[i].StartError = mediatools.OptionalMilliseconds{}
				after.Cuts[0].Streams[i].EndError = mediatools.OptionalMilliseconds{}
			}
			ffmpeg := testkit.CopyingMediaExecutable(t, "ffmpeg", "conditioned target bytes "+fixtureSuffix,
				"loudnorm=I=-23:TP=-1:LRA=11")
			crashStore := &conditioningRekeyCrashStore{st: st, afterRekey: tc.afterRekey}
			newStage := func(clipStore filler.TranscodeClipStore) *filler.TranscodeStage {
				stage := filler.NewTranscodeStage(clipStore, func(context.Context, string) (filler.Probed, error) {
					return filler.Probed{DurationMs: 30_000, Height: 480}, nil
				}, dir, mediatools.DefaultMezzanine(), func() string { return ffmpeg }, func() float64 { return -23 }, func() time.Time { return now })
				stage.WithConditioning(func(_ context.Context, req mediatools.ConditioningRequest) (mediatools.ConditioningMeasurement, error) {
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
			out, err := first.Run(ctx, filler.StoreClip{Clip: filler.Clip{
				Hash: sourceHash, Path: filepath.Base(sourceFull), Name: "Reviewed child",
				Kind: filler.Commercial, ParentHash: parentHash,
			}})
			if err == nil || crashStore.target.Hash == "" {
				t.Fatalf("crash result = %+v, target=%+v, err=%v; want retryable infrastructure error", out, crashStore.target, err)
			}
			targetFull := filepath.Join(dir, filepath.FromSlash(crashStore.target.Path))
			tags, ok := filler.ReadSidecarTags(targetFull)
			if !ok || tags.ConditioningPublication == nil {
				t.Fatalf("crash lost pending publication evidence: %+v, ok=%v", tags, ok)
			}
			if !tc.afterRekey {
				if source, err := st.GetClip(ctx, sourceHash); err != nil || source.Hash != sourceHash {
					t.Fatalf("transaction failure lost source evidence: %+v, %v", source, err)
				}
				if pipeline, found, err := st.GetClipPipeline(ctx, sourceHash); err != nil || !found || pipeline.Stage != filler.StageTranscode {
					t.Fatalf("transaction failure lost source reference: %+v, found=%v, err=%v", pipeline, found, err)
				}
			}

			if !tc.afterRekey {
				layout, err := filler.NewLayout(dir, "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := filler.NewSyncer(conditioningJourneySource{dir: dir}, fillerStoreAdapter{st}, layout, time.Now, nil).Sync(ctx); err != nil {
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
			restarted := newStage(fillerPipelineClipAdapter{st})
			result, err := restarted.Run(ctx, filler.StoreClip{Clip: restartClip.Clip, UpdatedAt: restartClip.UpdatedAt})
			if err != nil || result.Verdict != filler.VerdictContinue || result.Clip.Hash != crashStore.target.Hash {
				t.Fatalf("restart result = %+v, %v; want committed target %s", result, err, crashStore.target.Hash)
			}
			if _, err := st.GetClip(ctx, sourceHash); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("source catalog identity survived recovery: %v", err)
			}
			committedTags, ok := filler.ReadSidecarTags(targetFull)
			if !ok || committedTags.ConditioningPublication != nil {
				t.Fatalf("restart retained pending publication: %+v, ok=%v", committedTags.ConditioningPublication, ok)
			}
		})
	}
}

func TestFillerConditioningJourney_ConfirmedChildBecomesTheIdentifiedAiring(t *testing.T) {
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
	if err := st.UpsertClip(ctx, store.Clip{Clip: filler.Clip{
		Hash: compHash, Path: compPath, Name: "Compilation", Kind: filler.Commercial,
	}, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	ffmpeg := testkit.CopyingMediaExecutable(t, "ffmpeg", "mezzanine", "loudnorm=I=-23:TP=-1:LRA=11")
	parentProbeCalls := 0
	probe := func(_ context.Context, path string) (filler.Probed, error) {
		if filepath.Clean(path) == filepath.Clean(parentFull) {
			parentProbeCalls++
			return filler.Probed{DurationMs: 61_000, Height: 480}, nil
		}
		return filler.Probed{DurationMs: 30_000, Height: 480}, nil
	}
	parentTranscodeCalls := 0
	parentProbe := filler.NewProbeStage(probe, fillerPipelineClipAdapter{st}, drop, nil,
		func() time.Duration { return 45 * time.Second }, time.Now)
	parentTranscode := filler.NewTranscodeStage(fillerPipelineClipAdapter{st}, probe, drop,
		mediatools.DefaultMezzanine(), func() string { parentTranscodeCalls++; return ffmpeg },
		func() float64 { return -23 }, time.Now)
	parentPipeline := filler.NewPipeline(st, fillerPipelineClipAdapter{st}, []filler.Stage{parentProbe, parentTranscode},
		filler.Budget{}, nil, time.Now, slog.New(slog.DiscardHandler))
	if err := st.UpsertClipPipeline(ctx, filler.ClipPipeline{
		ClipHash: compHash, Stage: filler.StageProbe, Status: filler.StatusQueued,
		Disposition: filler.DispositionRunning,
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

	splitter := filler.NewSplitter(fillerSplitStoreAdapter{st: st}, splitFakeTools{chapters: []filler.Chapter{
		{StartMs: 10_000, EndMs: 40_000, Title: "Reviewed advert"},
	}}, nil, drop, func() time.Duration { return 10 * time.Second },
		func() string { return "conditioning-proposal" }, time.Now, nil)
	proposal, err := splitter.Propose(ctx, compHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetClipsHeld(ctx, []string{compPath}, true, false, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertClipPipeline(ctx, filler.ClipPipeline{
		ClipHash: compHash, Stage: filler.StageSplit, Status: filler.StatusQueued,
		Disposition: filler.DispositionReview, NextRun: time.Now().UTC(),
		EnrolledAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	transcode := filler.NewTranscodeStage(
		fillerPipelineClipAdapter{st}, probe, drop, mediatools.DefaultMezzanine(),
		func() string { return ffmpeg }, func() float64 { return -23 }, time.Now,
	)
	var measured []mediatools.ConditioningRequest
	var measuredParentBound bool
	var beforeSourceHash string
	transcode.WithConditioning(func(_ context.Context, req mediatools.ConditioningRequest) (mediatools.ConditioningMeasurement, error) {
		measured = append(measured, req)
		if len(measured) == 1 {
			parentID, parentErr := filler.ClipID(req.ParentPath)
			beforeSourceHash, _ = filler.ClipID(req.Path)
			measuredParentBound = parentErr == nil && parentID == compHash
			measurement := journeyConditioningMeasurement(-28.4)
			measurement.Cuts[0].Intended = req.IntendedCuts[0]
			return measurement, nil
		}
		measurement := journeyConditioningMeasurement(-23.1)
		measurement.Cuts[0].Intended = req.IntendedCuts[0]
		return measurement, nil
	})
	pipeline := filler.NewPipeline(st, fillerPipelineClipAdapter{st}, []filler.Stage{
		filler.NewProbeStage(probe, fillerPipelineClipAdapter{st}, drop, nil,
			func() time.Duration { return 45 * time.Second }, time.Now),
		transcode,
		filler.NewScoreStage(fillerTagStoreAdapter{st: st}, &filler.AutoFilePolicy{
			Enabled: func() bool { return true }, MinConfidence: func() int { return 50 },
		}, func() bool { return false }, time.Now),
	}, filler.Budget{}, nil, time.Now, slog.New(slog.DiscardHandler))
	app := fillerServiceAdapter{
		splitter: splitter, splitClips: fillerSplitStoreAdapter{st: st}, pipeline: pipeline,
	}
	segments := append([]filler.SplitSegment(nil), proposal.Segments...)
	segments[0].Era = 1994
	segments[0].Tags = []string{"toys"}
	if err := app.ConfirmSplit(ctx, proposal.ID, segments); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	clips, err := st.ListClips(ctx, store.ClipFilter{IncludeComposites: true, IncludeHeld: true})
	if err != nil {
		t.Fatal(err)
	}
	var child store.Clip
	for _, clip := range clips {
		if clip.ParentHash == compHash {
			child = clip
		}
	}
	if child.Hash == "" || child.Hash == proposal.ClipHash || child.ParentHash != compHash {
		t.Fatalf("conditioned child = %+v", child)
	}
	row, found, err := st.GetClipPipeline(ctx, child.Hash)
	if err != nil || !found || row.Disposition != filler.DispositionFiled {
		t.Fatalf("child pipeline = (%+v, %v, %v), want filed", row, found, err)
	}
	if len(measured) != 2 || !measuredParentBound || len(measured[0].IntendedCuts) != 1 ||
		measured[0].IntendedCuts[0] != (mediatools.Interval{StartMs: 10_000, EndMs: 40_000}) {
		t.Fatalf("conditioning requests = %+v", measured)
	}
	tags, ok := filler.ReadSidecarTags(filepath.Join(drop, filepath.FromSlash(child.Path)))
	if !ok || tags.Conditioning == nil || tags.Conditioning.BeforeRewriteHash != beforeSourceHash ||
		tags.Conditioning.BeforeRewrite.Loudness.IntegratedLUFS != -28.4 ||
		tags.Conditioning.AfterRewrite.Loudness.IntegratedLUFS != -23.1 ||
		tags.Conditioning.BeforeRewrite.Loudness.TruePeak.State != mediatools.TruePeakFinite ||
		tags.Conditioning.AfterRewrite.Loudness.TruePeak.State != mediatools.TruePeakFinite ||
		tags.NormalizedLUFS != -23 ||
		len(tags.Conditioning.DerivedParentEdgesAfterRewrite.Streams) != 2 {
		t.Fatalf("conditioned sidecar = %+v, ok=%v", tags.Conditioning, ok)
	}

	channel := store.Channel{Channel: schedule.Channel{
		ID: "conditioning-channel", Name: "Conditioning", Number: 63, Status: schedule.StatusLive,
	}}
	if _, err := st.SaveChannel(ctx, channel); err != nil {
		t.Fatal(err)
	}
	pods := filler.NewPodAdapter(clipCatalogAdapter{st}, nil, func() filler.Policy {
		return filler.Policy{PodMax: 1, BreakDurationMs: 30_000}
	}, slog.New(slog.DiscardHandler))
	previewer := podPreviewAdapter{store: st, pods: pods}
	breakStart := time.Unix(1_900_000_000, 0).UTC()
	pod, err := previewer.PreviewAt(ctx, channel.ID, breakStart.UnixMilli())
	if err != nil || len(pod.Entries) == 0 || pod.Entries[0].Hash != child.Hash {
		t.Fatalf("deterministic pod = %+v, %v", pod, err)
	}
	resolver := &playoutResolver{pods: previewer, fillerDir: drop}
	gap := playout.Airing{
		StartedAt: breakStart, ScheduleBlockID: "break-634", Kind: schedule.SlotFiller,
		Offset: 5 * time.Second, Remaining: 25 * time.Second,
	}
	airing, source, err := resolver.airingFiller(ctx, channel.ID, gap, breakStart.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if airing.Identity != child.Hash || airing.Kind != schedule.SlotFiller ||
		airing.ScheduleBlockID != "break-634" || airing.Offset != 5*time.Second ||
		airing.Remaining != 25*time.Second || source != filepath.Join(drop, filepath.FromSlash(child.Path)) {
		t.Fatalf("filler Airing.Identity = %+v, source=%q, child=%s", airing, source, child.Hash)
	}
}
