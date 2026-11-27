package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"

	"strconv"
	"strings"
	"sync"

	"github.com/loomarr/loomarr/internal/media/bgexec"
)

const (
	ConditioningMaxStreams        = 8
	ConditioningMaxCuts           = 8
	ConditioningMaxDurationMs     = 120_000
	ConditioningMaxSnapshotBytes  = int64(1 << 30)
	conditioningProbeOutputLimit  = 1 << 20
	conditioningFFmpegOutputLimit = 256 << 10
	conditioningSnapshotCopyChunk = 64 << 10
	conditioningFrameReadInterval = "%+120.000001"
	// ffmpeg and ffprobe render terminal timestamps independently. A detector end
	// may exceed the decoded EOF by a decimal microsecond even when both describe
	// the same final audio sample; accept at most one millisecond, then clamp it.
	conditioningDetectorEndToleranceMs = 1
	conditioningDetectorTail           = "tpad=stop_mode=add:stop=1:color=black,tpad=stop_mode=add:stop=1:color=white,"
)

var (
	ErrConditioningResourceLimit = errors.New("conditioning measurement resource limit exceeded")
	ErrConditioningOutput        = errors.New("conditioning measurement output invalid")
)

// StreamKind is the closed set of presented stream kinds measured by conditioning.
type StreamKind string

const (
	StreamVideo StreamKind = "video"
	StreamAudio StreamKind = "audio"
)

// OptionalMilliseconds distinguishes an observed zero from unavailable evidence.
type OptionalMilliseconds struct {
	Milliseconds int64 `json:"milliseconds"`
	Available    bool  `json:"available"`
}

// Rational preserves an exact ffprobe rational such as 30000/1001.
type Rational struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
}

// ConditioningStream identifies and measures one presented audio or video stream.
type ConditioningStream struct {
	Kind     StreamKind           `json:"kind"`
	Index    int                  `json:"index"`
	Start    OptionalMilliseconds `json:"start"`
	Duration OptionalMilliseconds `json:"duration"`
	Cadence  *Rational            `json:"cadence,omitempty"`
}

// ConditioningSkew is audio minus video. It remains unavailable unless exactly one presented
// audio stream and one presented video stream make the pairing unambiguous.
type ConditioningSkew struct {
	Start OptionalMilliseconds `json:"start"`
	End   OptionalMilliseconds `json:"end"`
}

// ConditioningTruePeakState represents finite true peak, digital silence, or no measurement
// without placing a non-finite float in a future wire payload.
type ConditioningTruePeakState string

const (
	TruePeakUnavailable      ConditioningTruePeakState = "unavailable"
	TruePeakFinite           ConditioningTruePeakState = "finite"
	TruePeakNegativeInfinity ConditioningTruePeakState = "negative_infinity"
)

// ConditioningTruePeak contains a finite dBTP only when State is TruePeakFinite.
type ConditioningTruePeak struct {
	State ConditioningTruePeakState `json:"state"`
	DBTP  float64                   `json:"dbtp,omitempty"`
}

// ConditioningLoudness contains integrated loudness and an explicitly represented true peak.
type ConditioningLoudness struct {
	IntegratedLUFS float64              `json:"integratedLufs"`
	TruePeak       ConditioningTruePeak `json:"truePeak"`
	Available      bool                 `json:"available"`
}

// ConditioningRequest names one local artifact, an optional single parent, and up to eight
// intended intervals in that parent.
type ConditioningRequest struct {
	Path         string
	ParentPath   string
	IntendedCuts []Interval
}

// ConditioningCutMeasurement contains independently matched evidence in request interval order.
type ConditioningCutMeasurement struct {
	Intended Interval                `json:"intended"`
	Streams  []ConditioningCutStream `json:"streams"`
}

// ConditioningCutStream contains actual-minus-intended edge errors for one exact stream identity.
type ConditioningCutStream struct {
	Kind       StreamKind           `json:"kind"`
	Index      int                  `json:"index"`
	StartError OptionalMilliseconds `json:"startError"`
	EndError   OptionalMilliseconds `json:"endError"`
}

// ConditioningMeasurement is evidence only. It carries no verdict or policy threshold.
//
// ContainerDurationMs is ffprobe's format.duration, which is measured from ContainerStart
// (format.start_time), not from zero: a re-encoded child typically starts at the AAC priming
// offset. Evidence persisted before ContainerStart existed leaves it unavailable, and its
// container timeline is read as starting at zero (#1540).
type ConditioningMeasurement struct {
	ContainerStart      OptionalMilliseconds         `json:"containerStart"`
	ContainerDurationMs int64                        `json:"containerDurationMs"`
	Streams             []ConditioningStream         `json:"streams"`
	AVSkew              ConditioningSkew             `json:"avSkew"`
	Loudness            ConditioningLoudness         `json:"loudness"`
	Quality             MediaQuality                 `json:"quality"`
	Cuts                []ConditioningCutMeasurement `json:"cuts,omitempty"`
}

type conditioningProbeJSON struct {
	Streams []conditioningProbeStreamJSON `json:"streams"`
	Format  struct {
		StartTime string `json:"start_time"`
		Duration  string `json:"duration"`
	} `json:"format"`
}

type conditioningProbeStreamJSON struct {
	Index        *int   `json:"index"`
	CodecType    string `json:"codec_type"`
	StartTime    string `json:"start_time"`
	Duration     string `json:"duration"`
	AvgFrameRate string `json:"avg_frame_rate"`
	SampleRate   string `json:"sample_rate"`
}

type conditioningProbeFrameJSON struct {
	StreamIndex         *int   `json:"stream_index"`
	PTS                 string `json:"pts_time"`
	BestEffortTimestamp string `json:"best_effort_timestamp_time"`
	Duration            string `json:"duration_time"`
	NumberOfSamples     *int64 `json:"nb_samples"`
}

type conditioningStreamProjection struct {
	stream conditioningProbeStreamJSON
	kind   StreamKind
	index  int
}

// MeasureConditioning performs a bounded, read-only inspection using the binaries already owned by
// FFmpegTools. It does not rewrite media or make an admission decision.
func (t *FFmpegTools) MeasureConditioning(ctx context.Context, req ConditioningRequest) (ConditioningMeasurement, error) {
	if t == nil {
		return ConditioningMeasurement{}, fmt.Errorf("%w: media tools are required", ErrConditioningOutput)
	}
	if len(req.IntendedCuts) > ConditioningMaxCuts {
		return ConditioningMeasurement{}, fmt.Errorf("%w: %d cuts exceeds %d", ErrConditioningResourceLimit, len(req.IntendedCuts), ConditioningMaxCuts)
	}
	if (strings.TrimSpace(req.ParentPath) == "") != (len(req.IntendedCuts) == 0) {
		return ConditioningMeasurement{}, fmt.Errorf("%w: parent and intended cuts must be supplied together", ErrConditioningOutput)
	}
	for _, intended := range req.IntendedCuts {
		if intended.StartMs < 0 || intended.EndMs <= intended.StartMs {
			return ConditioningMeasurement{}, fmt.Errorf("%w: intended cut must be a positive interval", ErrConditioningOutput)
		}
	}

	inputs, err := snapshotConditioningInputs(ctx, req.Path, req.ParentPath)
	if err != nil {
		return ConditioningMeasurement{}, err
	}
	defer inputs.close()
	artifactPath, parentPath := inputs.artifact, inputs.parent

	raw, err := runConditioningCommand(ctx, t.FFprobePath, conditioningProbeOutputLimit, false,
		"-v", "error", "-show_entries", "format=start_time,duration:stream=index,codec_type,start_time,duration,avg_frame_rate,sample_rate",
		"-of", "json", artifactPath)
	if err != nil {
		return ConditioningMeasurement{}, fmt.Errorf("condition media probe: %w", err)
	}
	var probed conditioningProbeJSON
	if err := json.Unmarshal(raw, &probed); err != nil {
		return ConditioningMeasurement{}, fmt.Errorf("%w: parse ffprobe JSON: %v", ErrConditioningOutput, err)
	}
	streams, err := validateConditioningStreamProjection(probed.Streams)
	if err != nil {
		return ConditioningMeasurement{}, err
	}
	duration, err := parseConditioningContainerDuration(probed.Format.Duration)
	if err != nil {
		return ConditioningMeasurement{}, err
	}

	containerStart, err := parseConditioningMilliseconds(probed.Format.StartTime)
	if err != nil {
		return ConditioningMeasurement{}, fmt.Errorf("%w: container start", err)
	}

	measurement := ConditioningMeasurement{ContainerStart: containerStart, ContainerDurationMs: duration.Milliseconds}
	for _, projected := range streams {
		stream, kind, index := projected.stream, projected.kind, projected.index
		measured := ConditioningStream{Kind: kind, Index: index}
		measured.Start, err = parseConditioningMilliseconds(stream.StartTime)
		if err != nil {
			return ConditioningMeasurement{}, fmt.Errorf("%w: stream %d start", err, index)
		}
		measured.Duration, err = parseConditioningDuration(stream.Duration)
		if err != nil {
			return ConditioningMeasurement{}, fmt.Errorf("%w: stream %d duration", err, index)
		}
		if kind == StreamVideo {
			measured.Cadence, err = parseConditioningRational(stream.AvgFrameRate)
			if err != nil {
				return ConditioningMeasurement{}, fmt.Errorf("%w: stream %d cadence", err, index)
			}
		}
		measurement.Streams = append(measurement.Streams, measured)
	}
	measurement.AVSkew, err = conditioningSkew(measurement.Streams)
	if err != nil {
		return ConditioningMeasurement{}, err
	}

	detectorStreams, err := selectConditioningDetectorStreams(measurement.Streams)
	if err != nil {
		return ConditioningMeasurement{}, err
	}
	frames, err := conditioningSelectedFrames(ctx, t.FFprobePath, artifactPath, detectorStreams)
	if err != nil {
		return ConditioningMeasurement{}, fmt.Errorf("condition decoded frame probe: %w", err)
	}
	origin, err := parseConditioningContainerOrigin(probed.Format.StartTime)
	if err != nil {
		return ConditioningMeasurement{}, err
	}
	if err := bindConditioningDetectorEOFs(&detectorStreams, probed.Streams, origin, frames); err != nil {
		return ConditioningMeasurement{}, err
	}
	detectorOutput, err := t.conditioningDetectorOutput(ctx, artifactPath, detectorStreams)
	if err != nil {
		return ConditioningMeasurement{}, err
	}
	measurement.Quality, err = parseConditioningDetectorEvents(detectorOutput, measurement.ContainerDurationMs, detectorStreams)
	if err != nil {
		return ConditioningMeasurement{}, err
	}
	measurement.Loudness, err = parseConditioningLoudness(detectorOutput)
	if err != nil {
		return ConditioningMeasurement{}, err
	}
	if hasConditioningStream(measurement.Streams, StreamAudio) && !measurement.Loudness.Available {
		return ConditioningMeasurement{}, fmt.Errorf("%w: audio loudness summary is incomplete", ErrConditioningOutput)
	}
	for _, intended := range req.IntendedCuts {
		matched, err := t.measureConditioningCut(ctx, artifactPath, parentPath, measurement.ContainerStart.Milliseconds, measurement.ContainerDurationMs, measurement.Streams, intended)
		if err != nil {
			return ConditioningMeasurement{}, err
		}
		matched.Intended = intended
		measurement.Cuts = append(measurement.Cuts, matched)
	}
	return measurement, nil
}

// conditioningSelectedFrames bounds each selected stream independently. A
// single valid audio stream can contain thousands of decoded frames and exceed
// a shared probe-output cap even though its own bounded evidence is valid.
func conditioningSelectedFrames(ctx context.Context, ffprobePath, artifactPath string, selected conditioningDetectorStreams) ([]conditioningProbeFrameJSON, error) {
	streams := []*ConditioningStream{selected.video, selected.audio}
	frames := make([]conditioningProbeFrameJSON, 0)
	for _, stream := range streams {
		if stream == nil {
			continue
		}
		specifier, err := conditioningStreamSpecifier(stream.Kind)
		if err != nil {
			return nil, err
		}
		frameEntries := "frame=pts_time,best_effort_timestamp_time,duration_time"
		if stream.Kind == StreamAudio {
			frameEntries = "frame=pts_time,best_effort_timestamp_time,nb_samples"
		}
		raw, err := runConditioningCommand(ctx, ffprobePath, conditioningProbeOutputLimit, false,
			"-v", "error", "-read_intervals", conditioningFrameReadInterval, "-select_streams", specifier, "-show_frames",
			"-show_entries", frameEntries,
			"-of", "compact=p=0:nk=1", artifactPath)
		if err != nil {
			return nil, err
		}
		probe, err := parseConditioningCompactFrames(raw, stream.Kind, stream.Index)
		if err != nil {
			return nil, err
		}
		frames = append(frames, probe...)
	}
	return frames, nil
}

func conditioningStreamSpecifier(kind StreamKind) (string, error) {
	switch kind {
	case StreamVideo:
		return "v:0", nil
	case StreamAudio:
		return "a:0", nil
	default:
		return "", fmt.Errorf("%w: selected detector stream has unknown kind %q", ErrConditioningOutput, kind)
	}
}

func parseConditioningCompactFrames(raw []byte, kind StreamKind, index int) ([]conditioningProbeFrameJSON, error) {
	frames := make([]conditioningProbeFrameJSON, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 3 {
			return nil, fmt.Errorf("%w: decoded frame record has %d fields", ErrConditioningOutput, len(fields))
		}
		frame := conditioningProbeFrameJSON{StreamIndex: new(int), PTS: fields[0], BestEffortTimestamp: fields[1], Duration: fields[2]}
		*frame.StreamIndex = index
		if kind == StreamAudio {
			samples, err := strconv.ParseInt(fields[2], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%w: selected audio frame has invalid sample count", ErrConditioningOutput)
			}
			frame.Duration = ""
			frame.NumberOfSamples = &samples
		}
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("%w: selected %s stream %d has no decoded frames", ErrConditioningOutput, kind, index)
	}
	return frames, nil
}

type conditioningInputSnapshots struct {
	dir      string
	artifact string
	parent   string
}

type regularFileSnapshot struct {
	path   string
	sha256 string
	bytes  int64
}

func snapshotConditioningInputs(ctx context.Context, artifact, parent string) (conditioningInputSnapshots, error) {
	if err := ctx.Err(); err != nil {
		return conditioningInputSnapshots{}, err
	}
	dir, err := os.MkdirTemp("", "loomarr-conditioning-")
	if err != nil {
		return conditioningInputSnapshots{}, fmt.Errorf("%w: create private input snapshot: %v", ErrConditioningOutput, err)
	}
	inputs := conditioningInputSnapshots{dir: dir}
	fail := func(err error) (conditioningInputSnapshots, error) {
		inputs.close()
		return conditioningInputSnapshots{}, err
	}
	artifactSnapshot, err := snapshotConditioningRegularFile(ctx, dir, "artifact", artifact)
	if err != nil {
		return fail(err)
	}
	inputs.artifact = artifactSnapshot.path
	if parent != "" {
		parentSnapshot, err := snapshotConditioningRegularFile(ctx, dir, "parent", parent)
		if err != nil {
			return fail(fmt.Errorf("conditioning parent: %w", err))
		}
		inputs.parent = parentSnapshot.path
	}
	return inputs, nil
}

func (s conditioningInputSnapshots) close() {
	if s.dir != "" {
		_ = os.RemoveAll(s.dir)
	}
}

func snapshotConditioningRegularFile(ctx context.Context, dir, name, path string) (regularFileSnapshot, error) {
	if strings.TrimSpace(path) == "" || path == "-" {
		return regularFileSnapshot{}, fmt.Errorf("%w: local regular media path is required", ErrConditioningOutput)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return regularFileSnapshot{}, fmt.Errorf("%w: resolve local media path: %v", ErrConditioningOutput, err)
	}
	file, err := openConditioningRegularFile(abs)
	if err != nil {
		return regularFileSnapshot{}, fmt.Errorf("%w: open local media path: %v", ErrConditioningOutput, err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return regularFileSnapshot{}, fmt.Errorf("%w: media path is not a regular file", ErrConditioningOutput)
	}
	if opened.Size() > ConditioningMaxSnapshotBytes {
		return regularFileSnapshot{}, fmt.Errorf("%w: media snapshot is %d bytes; maximum is %d", ErrConditioningResourceLimit, opened.Size(), ConditioningMaxSnapshotBytes)
	}
	// The opened descriptor is the authority. Independently cap bytes read so a concurrent append or
	// stale size report cannot turn snapshot creation into an unbounded operation.
	// Retain the caller's basename after an internal role prefix. ffmpeg still gets a useful format
	// hint, while artifact and parent cannot collide when both were named identically.
	destination := filepath.Join(dir, name+"-"+filepath.Base(abs))
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return regularFileSnapshot{}, fmt.Errorf("%w: create private media snapshot: %v", ErrConditioningOutput, err)
	}
	digest := sha256.New()
	copyErr := copyConditioningSnapshot(ctx, io.MultiWriter(out, digest), file)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		return regularFileSnapshot{}, errors.Join(copyErr, closeErr)
	}
	snapshotInfo, err := os.Stat(destination)
	if err != nil {
		return regularFileSnapshot{}, fmt.Errorf("%w: stat private media snapshot: %v", ErrConditioningOutput, err)
	}
	return regularFileSnapshot{path: destination, sha256: hex.EncodeToString(digest.Sum(nil)), bytes: snapshotInfo.Size()}, nil
}

func copyConditioningSnapshot(ctx context.Context, destination io.Writer, source io.Reader) error {
	buffer := make([]byte, conditioningSnapshotCopyChunk)
	var copied int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining := ConditioningMaxSnapshotBytes + 1 - copied
		if remaining <= 0 {
			return fmt.Errorf("%w: media snapshot exceeded %d bytes", ErrConditioningResourceLimit, ConditioningMaxSnapshotBytes)
		}
		chunk := buffer
		if int64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
		}
		n, readErr := source.Read(chunk)
		if n > 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			if copied > ConditioningMaxSnapshotBytes-int64(n) {
				return fmt.Errorf("%w: media snapshot exceeded %d bytes", ErrConditioningResourceLimit, ConditioningMaxSnapshotBytes)
			}
			written, writeErr := destination.Write(chunk[:n])
			copied += int64(written)
			if writeErr != nil {
				return fmt.Errorf("%w: write private media snapshot: %v", ErrConditioningOutput, writeErr)
			}
			if written != n {
				return fmt.Errorf("%w: write private media snapshot: %v", ErrConditioningOutput, io.ErrShortWrite)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return fmt.Errorf("%w: read opened media object: %v", ErrConditioningOutput, readErr)
		}
		if n == 0 {
			return fmt.Errorf("%w: opened media object made no read progress", ErrConditioningOutput)
		}
	}
}

func validateConditioningStreamProjection(raw []conditioningProbeStreamJSON) ([]conditioningStreamProjection, error) {
	if len(raw) > ConditioningMaxStreams {
		return nil, fmt.Errorf("%w: %d streams exceeds %d", ErrConditioningResourceLimit, len(raw), ConditioningMaxStreams)
	}
	projected := make([]conditioningStreamProjection, 0, len(raw))
	seenIndexes := make(map[int]struct{}, len(raw))
	for _, stream := range raw {
		if stream.Index == nil || *stream.Index < 0 {
			return nil, fmt.Errorf("%w: missing or negative stream index", ErrConditioningOutput)
		}
		index := *stream.Index
		if _, duplicate := seenIndexes[index]; duplicate {
			return nil, fmt.Errorf("%w: duplicate stream index %d", ErrConditioningOutput, index)
		}
		seenIndexes[index] = struct{}{}
		if kind, known := conditioningStreamKind(stream.CodecType); known {
			projected = append(projected, conditioningStreamProjection{stream: stream, kind: kind, index: index})
		}
	}
	return projected, nil
}

func hasConditioningStream(streams []ConditioningStream, kind StreamKind) bool {
	for _, stream := range streams {
		if stream.Kind == kind {
			return true
		}
	}
	return false
}

func conditioningStreamKind(raw string) (StreamKind, bool) {
	switch raw {
	case string(StreamVideo):
		return StreamVideo, true
	case string(StreamAudio):
		return StreamAudio, true
	default:
		return "", false
	}
}

func parseConditioningMilliseconds(raw string) (OptionalMilliseconds, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "N/A") {
		return OptionalMilliseconds{}, nil
	}
	seconds, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return OptionalMilliseconds{}, fmt.Errorf("%w: invalid time scalar %q", ErrConditioningOutput, raw)
	}
	scaled := seconds * 1000
	// float64 cannot distinguish MaxInt64 from the next integer. Keep one representable unit of
	// headroom before converting so an out-of-range float can never wrap to MinInt64.
	if math.IsInf(scaled, 0) || scaled >= float64(math.MaxInt64) || scaled <= float64(math.MinInt64) {
		return OptionalMilliseconds{}, fmt.Errorf("%w: time scalar overflows milliseconds", ErrConditioningOutput)
	}
	return OptionalMilliseconds{Milliseconds: int64(math.Round(scaled)), Available: true}, nil
}

func parseConditioningDuration(raw string) (OptionalMilliseconds, error) {
	value, err := parseConditioningMilliseconds(raw)
	if err != nil || !value.Available {
		return value, err
	}
	if value.Milliseconds < 0 {
		return OptionalMilliseconds{}, fmt.Errorf("%w: negative duration", ErrConditioningOutput)
	}
	return value, nil
}

func parseConditioningContainerDuration(raw string) (OptionalMilliseconds, error) {
	seconds, ok := new(big.Rat).SetString(strings.TrimSpace(raw))
	if !ok || seconds.Sign() <= 0 {
		return OptionalMilliseconds{}, fmt.Errorf("%w: positive container duration is required", ErrConditioningOutput)
	}
	if seconds.Cmp(big.NewRat(ConditioningMaxDurationMs, 1_000)) > 0 {
		return OptionalMilliseconds{}, fmt.Errorf("%w: container duration exceeds %dms", ErrConditioningResourceLimit, ConditioningMaxDurationMs)
	}
	milliseconds, ok := roundPositiveConditioningRational(new(big.Rat).Mul(seconds, big.NewRat(1_000, 1)))
	if !ok || milliseconds <= 0 {
		return OptionalMilliseconds{}, fmt.Errorf("%w: positive container duration is required", ErrConditioningOutput)
	}
	return OptionalMilliseconds{Milliseconds: milliseconds, Available: true}, nil
}

func parseConditioningRational(raw string) (*Rational, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "N/A") || trimmed == "0/0" {
		return nil, nil
	}
	numerator, denominator, ok := strings.Cut(trimmed, "/")
	if !ok {
		return nil, fmt.Errorf("%w: invalid cadence %q", ErrConditioningOutput, raw)
	}
	n, nErr := strconv.ParseInt(numerator, 10, 64)
	d, dErr := strconv.ParseInt(denominator, 10, 64)
	if nErr != nil || dErr != nil || n <= 0 || d <= 0 {
		return nil, fmt.Errorf("%w: invalid cadence %q", ErrConditioningOutput, raw)
	}
	return &Rational{Numerator: n, Denominator: d}, nil
}

func conditioningSkew(streams []ConditioningStream) (ConditioningSkew, error) {
	var audio, video *ConditioningStream
	for i := range streams {
		switch streams[i].Kind {
		case StreamAudio:
			if audio != nil {
				return ConditioningSkew{}, nil
			}
			audio = &streams[i]
		case StreamVideo:
			if video != nil {
				return ConditioningSkew{}, nil
			}
			video = &streams[i]
		}
	}
	var skew ConditioningSkew
	if audio == nil || video == nil {
		return skew, nil
	}
	var err error
	if audio.Start.Available && video.Start.Available {
		skew.Start.Milliseconds, err = checkedConditioningSub(audio.Start.Milliseconds, video.Start.Milliseconds)
		if err != nil {
			return ConditioningSkew{}, err
		}
		skew.Start.Available = true
	}
	if audio.Start.Available && audio.Duration.Available && video.Start.Available && video.Duration.Available {
		audioEnd, audioErr := checkedConditioningAdd(audio.Start.Milliseconds, audio.Duration.Milliseconds)
		videoEnd, videoErr := checkedConditioningAdd(video.Start.Milliseconds, video.Duration.Milliseconds)
		if audioErr != nil || videoErr != nil {
			return ConditioningSkew{}, fmt.Errorf("%w: stream end overflows", ErrConditioningOutput)
		}
		skew.End.Milliseconds, err = checkedConditioningSub(audioEnd, videoEnd)
		if err != nil {
			return ConditioningSkew{}, err
		}
		skew.End.Available = true
	}
	return skew, nil
}

func checkedConditioningAdd(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, fmt.Errorf("%w: millisecond arithmetic overflow", ErrConditioningOutput)
	}
	return a + b, nil
}

func checkedConditioningSub(a, b int64) (int64, error) {
	if b == math.MinInt64 {
		if a >= 0 {
			return 0, fmt.Errorf("%w: millisecond arithmetic overflow", ErrConditioningOutput)
		}
		return a - b, nil
	}
	return checkedConditioningAdd(a, -b)
}

type conditioningDetectorStreams struct {
	video       *ConditioningStream
	audio       *ConditioningStream
	videoEOF    conditioningDetectorScalar
	audioEOF    conditioningDetectorScalar
	videoTailMs int64
}

func selectConditioningDetectorStreams(streams []ConditioningStream) (conditioningDetectorStreams, error) {
	var selected conditioningDetectorStreams
	for i := range streams {
		stream := &streams[i]
		switch stream.Kind {
		case StreamVideo:
			if selected.video == nil || stream.Index < selected.video.Index {
				selected.video = stream
			}
		case StreamAudio:
			if selected.audio == nil || stream.Index < selected.audio.Index {
				selected.audio = stream
			}
		}
	}
	for _, stream := range []*ConditioningStream{selected.video, selected.audio} {
		if stream != nil && (!stream.Duration.Available || stream.Duration.Milliseconds <= 0) {
			return conditioningDetectorStreams{}, fmt.Errorf("%w: selected %s stream %d has no positive EOF", ErrConditioningOutput, stream.Kind, stream.Index)
		}
	}
	return selected, nil
}

// parseConditioningContainerOrigin is format.start_time as an exact rational. Quality evidence is on
// the container timeline, like every other conditioning fact (#1540): the detector decode runs
// without -copyts, so ffmpeg already reports each detector time as pts minus this origin, and the
// decoded stream ends are put on the same timeline here (#1719). An unavailable start leaves
// ffmpeg's timeline unshifted, so the origin is zero.
func parseConditioningContainerOrigin(raw string) (*big.Rat, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "N/A") {
		return new(big.Rat), nil
	}
	origin, ok := new(big.Rat).SetString(trimmed)
	if !ok {
		return nil, fmt.Errorf("%w: invalid container start %q", ErrConditioningOutput, raw)
	}
	return origin, nil
}

func bindConditioningDetectorEOFs(selected *conditioningDetectorStreams, probedStreams []conditioningProbeStreamJSON, origin *big.Rat, frames []conditioningProbeFrameJSON) error {
	if selected.video != nil {
		eof, err := conditioningDecodedStreamEOF(StreamVideo, selected.video.Index, "", origin, frames)
		if err != nil {
			return err
		}
		selected.videoEOF = eof
		tailMs, err := conditioningVideoDetectorTailAllowance(selected.video.Index, frames, *selected.video)
		if err != nil {
			return err
		}
		selected.videoTailMs = tailMs
	}
	if selected.audio != nil {
		sampleRate := ""
		for _, stream := range probedStreams {
			if stream.Index != nil && *stream.Index == selected.audio.Index {
				sampleRate = stream.SampleRate
				break
			}
		}
		eof, err := conditioningDecodedStreamEOF(StreamAudio, selected.audio.Index, sampleRate, origin, frames)
		if err != nil {
			return err
		}
		selected.audioEOF = eof
	}
	return nil
}

func conditioningDecodedStreamEOF(kind StreamKind, index int, sampleRateRaw string, origin *big.Rat, frames []conditioningProbeFrameJSON) (conditioningDetectorScalar, error) {
	var first, last *big.Rat
	var sampleRate int64
	if kind == StreamAudio {
		var err error
		sampleRate, err = strconv.ParseInt(sampleRateRaw, 10, 64)
		if err != nil || sampleRate <= 0 {
			return conditioningDetectorScalar{}, fmt.Errorf("%w: selected audio stream %d has invalid sample rate", ErrConditioningOutput, index)
		}
	}
	for _, frame := range frames {
		if frame.StreamIndex == nil || *frame.StreamIndex != index {
			continue
		}
		ptsRaw := frame.BestEffortTimestamp
		if ptsRaw == "" || strings.EqualFold(ptsRaw, "N/A") {
			ptsRaw = frame.PTS
		}
		pts, ok := new(big.Rat).SetString(ptsRaw)
		if !ok {
			return conditioningDetectorScalar{}, fmt.Errorf("%w: selected %s stream %d has invalid frame timestamp", ErrConditioningOutput, kind, index)
		}
		var span *big.Rat
		if kind == StreamAudio {
			if frame.NumberOfSamples == nil || *frame.NumberOfSamples <= 0 {
				return conditioningDetectorScalar{}, fmt.Errorf("%w: selected audio stream %d has invalid decoded sample count", ErrConditioningOutput, index)
			}
			span = big.NewRat(*frame.NumberOfSamples, sampleRate)
		} else {
			span, ok = new(big.Rat).SetString(frame.Duration)
			if !ok || span.Sign() <= 0 {
				return conditioningDetectorScalar{}, fmt.Errorf("%w: selected video stream %d has invalid frame duration", ErrConditioningOutput, index)
			}
		}
		end := new(big.Rat).Add(pts, span)
		if first == nil || pts.Cmp(first) < 0 {
			first = new(big.Rat).Set(pts)
		}
		if last == nil || end.Cmp(last) > 0 {
			last = end
		}
	}
	if first == nil || last == nil || last.Cmp(first) <= 0 {
		return conditioningDetectorScalar{}, fmt.Errorf("%w: selected %s stream %d has no decoded EOF", ErrConditioningOutput, kind, index)
	}
	spanSeconds := new(big.Rat).Sub(last, first)
	if spanSeconds.Cmp(big.NewRat(ConditioningMaxDurationMs, 1_000)) > 0 {
		return conditioningDetectorScalar{}, fmt.Errorf("%w: selected %s stream %d decoded EOF exceeds %dms", ErrConditioningResourceLimit, kind, index, ConditioningMaxDurationMs)
	}
	// The decoded end on the container timeline, where the detectors report (#1719).
	endSeconds := new(big.Rat).Sub(last, origin)
	if endSeconds.Sign() <= 0 {
		return conditioningDetectorScalar{}, fmt.Errorf("%w: selected %s stream %d decoded EOF is out of range", ErrConditioningOutput, kind, index)
	}
	endMs := new(big.Rat).Mul(endSeconds, big.NewRat(1_000, 1))
	eof, ok := roundPositiveConditioningRational(endMs)
	if !ok || eof <= 0 || eof > ConditioningMaxDurationMs {
		return conditioningDetectorScalar{}, fmt.Errorf("%w: selected %s stream %d decoded EOF is out of range", ErrConditioningOutput, kind, index)
	}
	return conditioningDetectorScalar{seconds: endSeconds, ms: eof}, nil
}

func (t *FFmpegTools) conditioningDetectorOutput(ctx context.Context, path string, streams conditioningDetectorStreams) (string, error) {
	// No -copyts and no per-stream rebase: ffmpeg subtracts format.start_time from every input
	// timestamp, so each detector reports on the container timeline. Rebasing each stream to its own
	// first frame put a primed mezzanine's audio (and a delayed stream's intervals) on a different
	// timeline from the container it is bounded by (#1719).
	args := []string{"-nostdin", "-threads", strconv.Itoa(BackgroundThreads), "-hide_banner", "-nostats", "-v", "info", "-i", path}
	if streams.video != nil {
		// One black frame followed by one white frame guarantees a change regardless of the final
		// artifact pixels. Evidence is clamped to the selected video's exact decoded EOF, so these
		// syntax-closing frames cannot become conditioning facts.
		args = append(args, "-map", fmt.Sprintf("0:%d", streams.video.Index), "-vf", conditioningDetectorTail+qualityVideoFilters)
	} else {
		args = append(args, "-vn")
	}
	if streams.audio != nil {
		args = append(args, "-map", fmt.Sprintf("0:%d", streams.audio.Index), "-af", qualityAudioFilter+",ebur128=peak=true:framelog=quiet")
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-f", "null", "-")
	raw, err := runConditioningCommand(ctx, FFmpegOr(t.FFmpegPath), conditioningFFmpegOutputLimit, true, args...)
	if err != nil {
		return "", fmt.Errorf("condition media decode: %w", err)
	}
	return string(raw), nil
}

type conditioningOutput struct {
	mu       sync.Mutex
	limit    int
	buf      bytes.Buffer
	overflow bool
	cancel   context.CancelFunc
}

func (w *conditioningOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remaining := w.limit - w.buf.Len()
	if remaining > 0 {
		_, _ = w.buf.Write(p[:min(remaining, len(p))])
	}
	if len(p) > remaining && !w.overflow {
		w.overflow = true
		w.cancel()
	}
	return len(p), nil
}

func (w *conditioningOutput) exceeded() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.overflow
}

func (w *conditioningOutput) bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.buf.Bytes())
}

// runConditioningCommand is the single subprocess boundary for conditioning. Either stream hitting
// its execution cap cancels the process immediately; caller cancellation remains discoverable with
// errors.Is.
func runConditioningCommand(ctx context.Context, executable string, limit int, combined bool, args ...string) ([]byte, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdout := &conditioningOutput{limit: limit, cancel: cancel}
	stderr := &conditioningOutput{limit: limit, cancel: cancel}
	if combined {
		stderr = stdout
	}
	cmd := bgexec.Tool(runCtx, executable, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("conditioning tool canceled: %w", ctxErr)
	}
	if stdout.exceeded() || stderr.exceeded() {
		return nil, fmt.Errorf("%w: tool output exceeds %d bytes", ErrConditioningResourceLimit, limit)
	}
	if runErr != nil {
		return nil, fmt.Errorf("conditioning tool: %w: %s", runErr, stderr.bytes())
	}
	return stdout.bytes(), nil
}
