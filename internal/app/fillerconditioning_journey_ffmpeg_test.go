//go:build ffmpeg

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/packager"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type realReviewedSplitTools struct {
	splitFakeTools
	ffmpeg *mediatools.FFmpegTools
}

func (t realReviewedSplitTools) Cut(ctx context.Context, in string, start, end int64, out string) error {
	return t.ffmpeg.Cut(ctx, in, start, end, out)
}

func (t realReviewedSplitTools) Chapters(ctx context.Context, file string) ([]filler.Chapter, error) {
	return t.ffmpeg.Chapters(ctx, file)
}

func (t realReviewedSplitTools) Boundaries(ctx context.Context, file string, startMs, endMs int64) ([]filler.Interval, []filler.Interval, error) {
	return t.ffmpeg.Boundaries(ctx, file, startMs, endMs)
}

func TestFillerConditioningJourneyFFmpeg_MidBreakClipReturnsToDecodableProgram(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dir := t.TempDir()
	fixtures := testkit.FillerConditioningMedia(t, dir)
	tools := mediatools.NewFFmpegTools(ffmpeg, ffprobe, "", "", "")

	parentHash, err := filler.ClipID(fixtures.Compilation)
	if err != nil {
		t.Fatal(err)
	}
	probe := filler.FFprobeNextTo(ffmpeg)
	st := testkit.MigratedSQLiteStore(t)
	parentRel := filepath.Base(fixtures.Compilation)
	parentProbe, err := probe(ctx, fixtures.Compilation)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertClip(ctx, store.Clip{Clip: filler.Clip{Hash: parentHash, Path: parentRel, Name: "Compilation", Kind: filler.Commercial, DurationMs: parentProbe.DurationMs, Source: "test-folder"}, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.HoldClips(ctx, []string{parentRel}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertClipPipeline(ctx, filler.ClipPipeline{ClipHash: parentHash, Stage: filler.StageSplit, Status: filler.StatusQueued, Disposition: filler.DispositionReview, EnrolledAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	splitter := filler.NewSplitter(fillerSplitStoreAdapter{st: st}, realReviewedSplitTools{
		splitFakeTools: splitFakeTools{}, ffmpeg: tools,
	}, nil, dir, func() time.Duration { return 0 }, func() string { return "real-ffmpeg-proposal" }, time.Now, nil)
	proposal, err := splitter.Propose(ctx, parentHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Segments) < 2 || proposal.Segments[0].StartMs != 0 {
		t.Fatalf("production compilation detection proposal = %+v", proposal.Segments)
	}
	// Detection produced the candidate; the reviewed operator edit trims its transition tail to the
	// measured content interval used by this fixture. No chapter is supplied to Propose.
	reviewed := append([]filler.SplitSegment(nil), proposal.Segments[0])
	reviewed[0].EndMs = 12_000
	childHashes, err := splitter.Confirm(ctx, proposal.ID, reviewed)
	if err != nil || len(childHashes) != 1 {
		t.Fatalf("reviewed real split = %v, %v", childHashes, err)
	}
	childHash := childHashes[0]
	reviewedStart, reviewedEnd := reviewed[0].StartMs, reviewed[0].EndMs
	stage := filler.NewTranscodeStage(fillerPipelineClipAdapter{st}, probe, dir,
		mediatools.DefaultMezzanine(), func() string { return ffmpeg }, func() float64 { return -23 }, time.Now).
		WithConditioning(func(ctx context.Context, req mediatools.ConditioningRequest) (mediatools.ConditioningMeasurement, error) {
			measurement, err := tools.MeasureConditioning(ctx, req)
			t.Logf("conditioning %s: %+v err=%v", filepath.Base(req.Path), measurement, err)
			return measurement, err
		})
	pipeline := filler.NewPipeline(st, fillerPipelineClipAdapter{st}, []filler.Stage{
		filler.NewProbeStage(probe, fillerPipelineClipAdapter{st}, dir, nil,
			func() time.Duration { return 45 * time.Second }, time.Now), stage,
		filler.NewScoreStage(fillerTagStoreAdapter{st: st}, func() bool { return false }, time.Now),
	}, filler.Budget{}, nil, time.Now, nil)
	var passes []filler.PipelineResult
	for i := 0; i < 3; i++ {
		result, err := pipeline.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		passes = append(passes, result)
		if result.Failed != 0 || result.Rejected != 0 || result.Deferred != 0 {
			t.Fatalf("pipeline pass %d was not clean: %+v", i, result)
		}
	}
	productive := false
	for _, result := range passes {
		productive = productive || result.Advanced > 0
	}
	if !productive {
		t.Fatalf("pipeline passes were not productive: %+v", passes)
	}
	clips, err := st.ListClips(ctx, store.ClipFilter{IncludeComposites: true, IncludeHeld: true})
	if err != nil {
		t.Fatal(err)
	}
	var conditioned store.Clip
	for _, candidate := range clips {
		if candidate.ParentHash == parentHash && candidate.Hash != childHash {
			conditioned = candidate
		}
	}
	if conditioned.Hash == "" {
		t.Fatalf("conditioning pipeline did not prepare a child: %+v", clips)
	}
	// Since #1255 an enrolled clip whose ladder finishes clean is published automatically: the
	// conditioned child settles Ready, released from hold, with a playable placement.
	pipelineRow, found, err := st.GetClipPipeline(ctx, conditioned.Hash)
	if err != nil || !found || pipelineRow.Status != filler.StatusDone ||
		pipelineRow.Disposition != filler.DispositionReady || conditioned.Held ||
		conditioned.Placement == filler.PlacementNotPlayable {
		t.Fatalf("durable conditioned publication = %+v, held=%v placement=%q found=%v err=%v",
			pipelineRow, conditioned.Held, conditioned.Placement, found, err)
	}
	if conditioned.Kind != filler.Commercial {
		t.Fatalf("conditioned child kind = %q, want inherited commercial", conditioned.Kind)
	}
	if conditioned.Hash == childHash || conditioned.DurationMs <= 0 {
		t.Fatalf("real conditioned transcode = %+v", conditioned)
	}
	mezzanine := filepath.Join(dir, filepath.FromSlash(conditioned.Path))
	tags, ok := filler.ReadSidecarTags(mezzanine)
	if !ok || tags.Conditioning == nil || tags.MediaQuality == nil ||
		tags.ConditioningLineage == nil || tags.ConditioningLineage.ParentHash != parentHash ||
		tags.ConditioningLineage.ChildHash != childHash || tags.ConditioningLineage.IntendedStartMs != reviewedStart ||
		tags.ConditioningLineage.IntendedEndMs != reviewedEnd ||
		len(tags.Conditioning.BeforeRewrite.Cuts) != 1 ||
		tags.Conditioning.BeforeRewrite.Quality.DurationMs != tags.Conditioning.BeforeRewrite.ContainerDurationMs ||
		tags.Conditioning.BeforeRewrite.Cuts[0].Intended != (mediatools.Interval{StartMs: reviewedStart, EndMs: reviewedEnd}) ||
		tags.Conditioning.AfterRewrite.Cuts[0].Intended != (mediatools.Interval{StartMs: reviewedStart, EndMs: reviewedEnd}) ||
		tags.NormalizedLUFS != -23 ||
		!tags.Conditioning.BeforeRewrite.Loudness.Available ||
		!tags.Conditioning.AfterRewrite.Loudness.Available ||
		math.Abs(tags.Conditioning.BeforeRewrite.Loudness.IntegratedLUFS-tags.Conditioning.AfterRewrite.Loudness.IntegratedLUFS) < 0.5 ||
		math.Abs(tags.Conditioning.AfterRewrite.Loudness.IntegratedLUFS-(-23)) > 1.0 ||
		tags.Conditioning.BeforeRewrite.Loudness.TruePeak.State != mediatools.TruePeakFinite ||
		tags.Conditioning.AfterRewrite.Loudness.TruePeak.State != mediatools.TruePeakFinite ||
		len(tags.Conditioning.AfterRewrite.Streams) != 2 ||
		tags.Conditioning.AfterRewrite.Streams[0].Cadence == nil ||
		len(tags.Conditioning.AfterRewrite.Cuts) != 1 ||
		len(tags.Conditioning.AfterRewrite.Cuts[0].Streams) != 2 ||
		tags.Conditioning.AfterRewrite.Cuts[0].Streams[0].StartError.Available ||
		tags.Conditioning.AfterRewrite.Cuts[0].Streams[0].EndError.Available ||
		tags.Conditioning.AfterRewrite.Cuts[0].Streams[1].StartError.Available ||
		tags.Conditioning.AfterRewrite.Cuts[0].Streams[1].EndError.Available ||
		len(tags.Conditioning.DerivedParentEdgesAfterRewrite.Streams) != 2 ||
		!tags.Conditioning.DerivedParentEdgesAfterRewrite.Streams[0].StartError.Available ||
		!tags.Conditioning.DerivedParentEdgesAfterRewrite.Streams[0].EndError.Available {
		t.Fatalf("real conditioning sidecar = %+v, ok=%v", tags, ok)
	}

	// Exercise the conditioned bytes through the production encoder/mux boundary. Terminal
	// admission is independently covered by its transaction suite; this test owns media
	// compatibility, not release authority.
	epoch := time.Unix(1_900_000_000, 0).UTC()
	first := playout.Airing{StartedAt: epoch, Kind: schedule.SlotProgram, Remaining: 2 * time.Second}
	fillerAiring := playout.Airing{
		StartedAt: epoch.Add(2 * time.Second), Kind: schedule.SlotFiller,
		Identity: conditioned.Hash, Offset: 5 * time.Second, Remaining: 3 * time.Second,
	}
	last := playout.Airing{StartedAt: epoch.Add(5 * time.Second), Kind: schedule.SlotProgram, Remaining: 2 * time.Second}
	fillerSource := mezzanine

	airings := []struct {
		airing playout.Airing
		input  string
	}{
		{airing: first, input: fixtures.WhiteFrozen},
		{airing: fillerAiring, input: fillerSource},
		{airing: last, input: fixtures.WhiteFrozen},
	}
	// Air the three through the channel packager, as a live channel does: each item's encoder is
	// the production pipeline (Build, FragmentArgs) and the packager joins them into one fMP4
	// timeline. After them the schedule has a card, whose slate this fixture cannot load, so the
	// packager stops there with everything aired on disk.
	host := playout.HostFor(playout.EncoderSoftware, false, playout.GPUFilters{})
	out := playout.OutputProfile{Width: 320, Height: 180, FPS: 30, Quality: 23, TargetKbps: 600, MaxKbps: 900, GOPSeconds: 1, AudioKbps: 128}
	formatOf := playout.FFprobeFormatNextTo(ffmpeg)
	airDir := t.TempDir()
	onAir := time.Now()
	sched := func(ctx context.Context, at time.Time) (packager.Item, error) {
		elapsed := at.Sub(onAir)
		for _, a := range airings {
			slotStart := a.airing.StartedAt.Sub(epoch)
			if elapsed >= slotStart+a.airing.Remaining {
				continue
			}
			into := max(elapsed-slotStart, 0)
			input, seek := a.input, a.airing.Offset+into
			facts, err := formatOf(ctx, input)
			if err != nil {
				return packager.Item{}, err
			}
			pl, err := playout.Build(host, facts, out)
			if err != nil {
				return packager.Item{}, err
			}
			return packager.Item{Label: string(a.airing.Kind), Duration: a.airing.Remaining - into,
				Open: func(ctx context.Context, slot packager.Slot) (io.ReadCloser, error) {
					cmd := exec.CommandContext(ctx, ffmpeg, pl.FragmentArgs(input, seek, slot.Offset, slot.Frames, slot.AudioFrames, out.FPS, 0)...)
					stdout, err := cmd.StdoutPipe()
					if err != nil {
						return nil, err
					}
					if err := cmd.Start(); err != nil {
						return nil, err
					}
					return itemEncoder{ReadCloser: stdout, cmd: cmd}, nil
				}}, nil
		}
		return packager.Item{Label: "card", Duration: time.Hour}, nil
	}
	noSlate := func(context.Context) (*packager.Slate, error) { return nil, errors.New("no slate in this fixture") }
	pk, err := packager.New(packager.Config{FPS: out.FPS, Dir: airDir}, sched, noSlate)
	if err != nil {
		t.Fatal(err)
	}
	if err := pk.Run(ctx); err == nil || !strings.Contains(err.Error(), "no slate in this fixture") {
		t.Fatalf("packager stopped with %v, want it to stop at the card after the three airings", err)
	}
	segments, _ := filepath.Glob(filepath.Join(airDir, "seg*.m4s"))
	var joinedBytes []byte
	for _, name := range append([]string{filepath.Join(airDir, packager.InitName)}, segments...) {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		joinedBytes = append(joinedBytes, b...)
	}
	joined := filepath.Join(dir, "program-filler-program.mp4")
	if err := os.WriteFile(joined, joinedBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	probeRaw, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_entries",
		"format=duration:stream=codec_type,start_time,duration,r_frame_rate", "-of", "json", joined).Output()
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Streams []struct {
			Kind      string `json:"codec_type"`
			Start     string `json:"start_time"`
			Duration  string `json:"duration"`
			FrameRate string `json:"r_frame_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(probeRaw, &decoded); err != nil {
		t.Fatal(err)
	}
	containerSeconds, err := strconv.ParseFloat(decoded.Format.Duration, 64)
	if err != nil || containerSeconds < 6.95 || containerSeconds > 7.05 || len(decoded.Streams) != 2 {
		t.Fatalf("joined probe = %+v, duration error=%v", decoded, err)
	}
	var starts, ends []float64
	for _, stream := range decoded.Streams {
		start, startErr := strconv.ParseFloat(stream.Start, 64)
		duration, durationErr := strconv.ParseFloat(stream.Duration, 64)
		if startErr != nil || durationErr != nil || duration <= 0 {
			t.Fatalf("joined stream timing = %+v, start=%v duration=%v", stream, startErr, durationErr)
		}
		if stream.Kind == "video" && stream.FrameRate != "30/1" {
			t.Fatalf("joined video cadence = %q, want production profile 30/1", stream.FrameRate)
		}
		starts = append(starts, start)
		ends = append(ends, start+duration)
	}
	// This fixture encodes each finite production block independently. Bound the mux envelope by
	// one 1024-sample AAC-LC frame per block, one measured output-video frame, and the inspector's
	// measured post-rewrite skew. This is 97.3ms for three 48kHz blocks at 30fps with zero skew;
	// it replaces the unexplained rounded 100ms allowance with the actual cadence carried here.
	postSkew := tags.Conditioning.AfterRewrite.AVSkew
	if !postSkew.Start.Available || !postSkew.End.Available {
		t.Fatalf("post-rewrite A/V skew unavailable: %+v", postSkew)
	}
	videoCadence := tags.Conditioning.AfterRewrite.Streams[0].Cadence
	if videoCadence == nil || videoCadence.Numerator <= 0 || videoCadence.Denominator <= 0 {
		t.Fatalf("post-rewrite video cadence unavailable: %+v", videoCadence)
	}
	const (
		aacFrameSamples = 1024
		audioSampleRate = 48_000
	)
	measuredSkewSeconds := math.Max(math.Abs(float64(postSkew.Start.Milliseconds)), math.Abs(float64(postSkew.End.Milliseconds))) / 1000
	aacEnvelopeSeconds := float64(len(airings)*aacFrameSamples) / audioSampleRate
	videoFrameSeconds := float64(videoCadence.Denominator) / float64(videoCadence.Numerator)
	avTimingEnvelope := measuredSkewSeconds + aacEnvelopeSeconds + videoFrameSeconds
	if math.Abs(starts[0]-starts[1]) > avTimingEnvelope || math.Abs(ends[0]-ends[1]) > avTimingEnvelope {
		t.Fatalf("joined A/V timing starts=%v ends=%v envelope=%.6fs (skew=%.6fs AAC=%.6fs video=%.6fs)",
			starts, ends, avTimingEnvelope, measuredSkewSeconds, aacEnvelopeSeconds, videoFrameSeconds)
	}

	frameMean := func(path, at string) float64 {
		t.Helper()
		out, err := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-v", "error", "-i", path, "-ss", at,
			"-frames:v", "1", "-pix_fmt", "gray", "-f", "rawvideo", "-").Output()
		if err != nil {
			t.Fatalf("decode frame %s at %s: %v", path, at, err)
		}
		if len(out) == 0 {
			t.Fatalf("no decoded frame from %s at %s", path, at)
		}
		var total int64
		for _, sample := range out {
			total += int64(sample)
		}
		return float64(total) / float64(len(out))
	}
	programBefore := frameMean(joined, "1.000")
	programAfter := frameMean(joined, "6.000")
	fillerAtMidBreak := frameMean(joined, "3.000")
	fillerAtSourceOffset := frameMean(mezzanine, "6.000")
	if math.Abs(programBefore-programAfter) > 1 {
		t.Fatalf("program did not return at 5s boundary: before %.3f after %.3f", programBefore, programAfter)
	}
	if math.Abs(fillerAtMidBreak-fillerAtSourceOffset) > 2 || math.Abs(fillerAtMidBreak-programBefore) < 20 {
		t.Fatalf("mid-break frame did not come from child offset 5s: joined %.3f source %.3f program %.3f",
			fillerAtMidBreak, fillerAtSourceOffset, programBefore)
	}
}

// itemEncoder is one aired item's ffmpeg: the packager reads its stdout and closes it when the
// item is done, which reaps the process.
type itemEncoder struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (e itemEncoder) Close() error {
	_ = e.ReadCloser.Close()
	_ = e.cmd.Process.Kill()
	_ = e.cmd.Wait()
	return nil
}
