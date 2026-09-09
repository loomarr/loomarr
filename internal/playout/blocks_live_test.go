//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

type muxPacket struct {
	Stream   int         `json:"stream_index"`
	PTS      json.Number `json:"pts"`
	DTS      json.Number `json:"dts"`
	Duration json.Number `json:"duration"`
	Hash     string      `json:"data_hash"`
	Size     json.Number `json:"size"`
}

type muxStream struct {
	Index     int    `json:"index"`
	CodecType string `json:"codec_type"`
	TimeBase  string `json:"time_base"`
}

type muxProbe struct {
	Streams []muxStream `json:"streams"`
	Packets []muxPacket `json:"packets"`
}

func probeMuxPackets(t *testing.T, ctx context.Context, probe, path string) muxProbe {
	t.Helper()
	out, err := exec.CommandContext(ctx, probe, "-v", "error", "-show_packets", "-show_data_hash", "sha256", "-show_entries", "stream=index,codec_type,time_base:packet=stream_index,pts,dts,duration,data_hash,size", "-of", "json", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var got muxProbe
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode ffprobe %s: %v", path, err)
	}
	return got
}

func TestLive_BlockMuxPreservesSourceAVOffset(t *testing.T) {
	t.Run("consistent reordered sources", func(t *testing.T) { verifyMuxAudioReference(t, false) })
	t.Run("session card and return", func(t *testing.T) { verifyMuxAudioReference(t, true) })
}

func verifyMuxAudioReference(t *testing.T, card bool) {
	t.Helper()
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	paths := []string{filepath.Join(dir, "aligned.ts"), filepath.Join(dir, "audio-delay.ts")}
	bframes := "3"
	if card {
		paths = append(paths, filepath.Join(dir, "card.ts"), filepath.Join(dir, "return.ts"))
		bframes = "0"
	}
	for i, path := range paths {
		if i == 2 {
			profile := DefaultProfile()
			profile.Encoder, profile.Width, profile.Height, profile.Framerate = EncoderSoftware, 320, 180, 25
			clock := ProgramClock{Origin: time.Unix(1000, 0), StartedAt: time.Unix(1016, 0)}
			args := OfflineCardArgs(profile, "", "", "", 3*time.Second, clock)
			proc, err := Start(ctx, bin, replaceOutput(args, path), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, proc.Stdout); err != nil {
				proc.Stop()
				t.Fatal(err)
			}
			if err := proc.Wait(); err != nil {
				t.Fatalf("private card: %v: %s", err, proc.LastError())
			}
			continue
		}
		offset := "0"
		if i == 1 {
			offset = "0.3"
		}
		args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=duration=3:size=320x180:rate=25", "-itsoffset", offset, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=3", "-map", "0:v:0", "-map", "1:a:0", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-g", "25", "-bf", bframes, "-sc_threshold", "0", "-c:a", "s302m", "-strict", "-2", "-ac", "2", "-ar", "48000", "-t", "3", "-output_ts_offset", strconv.Itoa(10 + i*3), "-muxdelay", "0", "-muxpreload", "0", "-f", "mpegts", "-mpegts_flags", "+initial_discontinuity", path}
		if out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
			t.Fatalf("generate source: %v\n%s", err, out)
		}
	}
	sources := make([]muxProbe, len(paths))
	inputs := make([]*os.File, len(paths))
	for i, path := range paths {
		sources[i] = probeMuxPackets(t, ctx, probe, path)
		input, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		inputs[i] = input
		defer func(input *os.File) { _ = input.Close() }(input)
	}
	outPath := filepath.Join(dir, "joined.ts")
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = outFile.Close() }()
	proc, err := StartPipedObserved(ctx, bin, BlockMuxArgs(BlockProfile{AudioBitrate: 128}), nil, nil, nil, diagnostics.ProcessSpec{})
	if err != nil {
		t.Fatal(err)
	}
	drainDone := make(chan error, 1)
	drained := false
	defer func() {
		cancel()
		_ = proc.Stdin.Close()
		if !drained {
			if err := <-drainDone; err != nil {
				t.Errorf("drain mux stdout: %v", err)
			}
		}
		_ = proc.Wait()
	}()
	go func() { _, err := io.Copy(outFile, proc.Stdout); drainDone <- err }()
	for _, input := range inputs {
		_, copyErr := io.Copy(proc.Stdin, input)
		closeErr := input.Close()
		if copyErr != nil {
			t.Fatal(copyErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if err := proc.Stdin.Close(); err != nil {
		t.Fatal(err)
	}
	drainErr := <-drainDone
	drained = true
	if drainErr != nil {
		t.Fatal(drainErr)
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("mux: %v: %s", err, proc.LastError())
	}
	if err := outFile.Close(); err != nil {
		t.Fatal(err)
	}
	programmes := make([]map[string][]muxPacket, len(sources))
	for i, source := range sources {
		programmes[i] = packetsByCodec(t, source, "source programme "+strconv.Itoa(i))
	}
	output := packetsByCodec(t, probeMuxPackets(t, ctx, probe, outPath), "joined output")
	assertSecondProgrammeAudioDelay(t, programmes[1])
	assertProgrammesPreserved(t, programmes, output, "video")
	assertContinuousAudioReference(t, ctx, bin, paths, outPath)
	// The children share a session clock. Preserve its origin as well as A/V
	// offsets, payloads and spacing; an arbitrary common rebase is insufficient.
	for _, codec := range []string{"video"} {
		cursor := 0
		for index, programme := range programmes {
			first, _, _ := parseMuxPacket(t, programme[codec][0], "source clock")
			got, _, _ := parseMuxPacket(t, output[codec][cursor], "parent clock")
			if got != first {
				t.Fatalf("programme %d %s session origin changed: got %d, want %d", index, codec, got, first)
			}
			cursor += len(programme[codec])
		}
	}
}

func packetsByCodec(t *testing.T, probe muxProbe, label string) map[string][]muxPacket {
	t.Helper()
	streamCodecs := make(map[int]string, 2)
	for _, stream := range probe.Streams {
		if stream.CodecType != "video" && stream.CodecType != "audio" {
			t.Fatalf("%s has unexpected stream %d codec %q", label, stream.Index, stream.CodecType)
		}
		if stream.TimeBase != "1/90000" {
			t.Fatalf("%s stream %d time base=%q, want 1/90000", label, stream.Index, stream.TimeBase)
		}
		if _, duplicate := streamCodecs[stream.Index]; duplicate {
			t.Fatalf("%s has duplicate stream index %d", label, stream.Index)
		}
		if hasCodec(streamCodecs, stream.CodecType) {
			t.Fatalf("%s has multiple %s streams", label, stream.CodecType)
		}
		streamCodecs[stream.Index] = stream.CodecType
	}
	if len(streamCodecs) != 2 || !hasCodec(streamCodecs, "video") || !hasCodec(streamCodecs, "audio") {
		t.Fatalf("%s streams=%v, want exactly one video and one audio stream", label, streamCodecs)
	}
	byCodec := map[string][]muxPacket{"video": nil, "audio": nil}
	for _, packet := range probe.Packets {
		codec, ok := streamCodecs[packet.Stream]
		if !ok {
			t.Fatalf("%s packet has unknown stream %d", label, packet.Stream)
		}
		byCodec[codec] = append(byCodec[codec], packet)
	}
	for _, codec := range []string{"video", "audio"} {
		if len(byCodec[codec]) == 0 {
			t.Fatalf("%s has no %s packets", label, codec)
		}
	}
	return byCodec
}

func hasCodec(streamCodecs map[int]string, want string) bool {
	for _, codec := range streamCodecs {
		if codec == want {
			return true
		}
	}
	return false
}

func parseMuxPacket(t *testing.T, packet muxPacket, label string) (pts, dts, duration int64) {
	t.Helper()
	parse := func(name string, value json.Number) int64 {
		if value == "" {
			t.Fatalf("%s stream %d missing %s", label, packet.Stream, name)
		}
		parsed, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil {
			t.Fatalf("%s stream %d malformed %s %q: %v", label, packet.Stream, name, value, err)
		}
		return parsed
	}
	pts, dts, duration = parse("pts", packet.PTS), parse("dts", packet.DTS), parse("duration", packet.Duration)
	hash := strings.TrimPrefix(packet.Hash, "SHA256:")
	if len(hash) != 64 {
		t.Fatalf("%s stream %d invalid SHA256 hash %q", label, packet.Stream, packet.Hash)
	}
	if _, err := hex.DecodeString(hash); err != nil {
		t.Fatalf("%s stream %d invalid SHA256 hash %q: %v", label, packet.Stream, packet.Hash, err)
	}
	return pts, dts, duration
}

func assertSecondProgrammeAudioDelay(t *testing.T, programme map[string][]muxPacket) {
	t.Helper()
	videoPTS, _, _ := parseMuxPacket(t, programme["video"][0], "second programme video")
	audioPTS, err := strconv.ParseInt(string(programme["audio"][0].PTS), 10, 64)
	if err != nil {
		t.Fatalf("second programme audio PTS: %v", err)
	}
	if delay := audioPTS - videoPTS; abs64(delay-27000) > 4500 {
		t.Fatalf("second programme source A/V delay=%d ticks, want approximately 27000", delay)
	}
}

func assertProgrammesPreserved(t *testing.T, programmes []map[string][]muxPacket, output map[string][]muxPacket, codecs ...string) {
	t.Helper()
	if len(codecs) == 0 {
		codecs = []string{"video", "audio"}
	}
	start := map[string]int{"video": 0, "audio": 0}
	for programmeIndex, programme := range programmes {
		var shift int64
		shiftSet := false
		for _, codec := range codecs {
			sourcePackets, outputPackets := programme[codec], output[codec]
			end := start[codec] + len(sourcePackets)
			if end > len(outputPackets) {
				t.Fatalf("programme %d %s output has %d packets, want at least %d", programmeIndex, codec, len(outputPackets), end)
			}
			for i, source := range sourcePackets {
				out := outputPackets[start[codec]+i]
				sourcePTS, sourceDTS, sourceDuration := parseMuxPacket(t, source, "source")
				outputPTS, outputDTS, outputDuration := parseMuxPacket(t, out, "output")
				if out.Hash != source.Hash {
					t.Fatalf("programme %d %s packet %d payload changed", programmeIndex, codec, i)
				}
				if outputDuration != sourceDuration {
					t.Fatalf("programme %d %s packet %d duration=%d, want %d", programmeIndex, codec, i, outputDuration, sourceDuration)
				}
				if !shiftSet {
					shift, shiftSet = outputPTS-sourcePTS, true
				}
				if abs64((outputPTS-sourcePTS)-shift) > 1 || abs64((outputDTS-sourceDTS)-shift) > 1 {
					t.Fatalf("programme %d %s packet %d timestamp shift pts=%d dts=%d, want common %d", programmeIndex, codec, i, outputPTS-sourcePTS, outputDTS-sourceDTS, shift)
				}
				if i > 0 {
					previousSourcePTS, previousSourceDTS, _ := parseMuxPacket(t, sourcePackets[i-1], "source")
					previousOutputPTS, previousOutputDTS, _ := parseMuxPacket(t, outputPackets[start[codec]+i-1], "output")
					if abs64((outputPTS-previousOutputPTS)-(sourcePTS-previousSourcePTS)) > 1 || abs64((outputDTS-previousOutputDTS)-(sourceDTS-previousSourceDTS)) > 1 {
						t.Fatalf("programme %d %s packet %d timestamp spacing changed", programmeIndex, codec, i)
					}
				}
			}
			start[codec] = end
		}
	}
	for _, codec := range codecs {
		if start[codec] != len(output[codec]) {
			t.Fatalf("%s output packet count=%d, want %d", codec, len(output[codec]), start[codec])
		}
	}
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

// Build the reference from separately decoded private children. Concatenating decoded samples
// before a single independent encode catches AAC restart artifacts, dropped PCM and duplication.
func assertContinuousAudioReference(t *testing.T, ctx context.Context, bin string, sources []string, joined string) {
	t.Helper()
	decode := func(path string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, bin, "-v", "error", "-i", path, "-map", "0:a:0", "-c:a", "pcm_f32le", "-f", "f32le", "pipe:1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		pcm, err := cmd.Output()
		if err != nil {
			t.Fatalf("decode reference audio: %v: %s", err, &stderr)
		}
		return pcm
	}
	var input bytes.Buffer
	var sampleCounts []int64
	for _, source := range sources {
		pcm := decode(source)
		sampleCounts = append(sampleCounts, int64(len(pcm)/8))
		input.Write(pcm)
	}
	if input.Len() == 0 {
		t.Fatal("reference has no PCM samples")
	}
	reference := filepath.Join(t.TempDir(), "continuous-reference.aac")
	cmd := exec.CommandContext(ctx, bin, "-v", "error", "-f", "f32le", "-ar", "48000", "-ac", "2", "-i", "pipe:0", "-c:a", "aac", "-b:a", "128k", "-f", "adts", reference)
	cmd.Stdin = &input
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("encode reference audio: %v: %s", err, out)
	}
	want, got := decode(reference), decode(joined)
	if !bytes.Equal(got, want) {
		t.Fatalf("session decoded audio differs from continuous reference: got %d, want %d bytes", len(got), len(want))
	}
	// Encoder priming is one AAC frame; later programme gaps must survive instead of being
	// concealed by resampling or independent per-programme timestamp rebases.
	probe := ffprobeBin(t)
	output := packetsByCodec(t, probeMuxPackets(t, ctx, probe, joined), "AAC timeline")["audio"]
	var starts []int64
	for _, source := range sources {
		packets := packetsByCodec(t, probeMuxPackets(t, ctx, probe, source), "PCM timeline")["audio"]
		pts, err := strconv.ParseInt(string(packets[0].PTS), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		starts = append(starts, pts)
	}
	programme, consumed := 0, int64(0)
	for i, packet := range output {
		sample := int64(i-1) * 1024
		for programme+1 < len(starts) && sample >= consumed+sampleCounts[programme] {
			consumed += sampleCounts[programme]
			programme++
		}
		wantPTS := starts[programme] + (sample-consumed)*90000/48000
		gotPTS, _, _ := parseMuxPacket(t, packet, "continuous AAC timeline")
		if abs64(gotPTS-wantPTS) > 1 {
			t.Fatalf("AAC packet %d PTS=%d, want %d from source sample %d", i, gotPTS, wantPTS, sample)
		}
	}
	t.Logf("continuous AAC reference: %d stereo samples/channel matched exactly", len(got)/8)
}

// This records FFmpeg CPU, separately from the certification endpoint's Go-process metric.
// The comparison isolates the approved prepared-MPEG-TS audio cost on the same finite source.
func TestLive_PreparedSessionAudioCPU(t *testing.T) {
	bin := ffmpegBin(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	generate := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=duration=20:size=320x180:rate=25", "-f", "lavfi", "-i", "sine=frequency=733:sample_rate=48000:duration=20",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "25", "-bf", "0", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k", "-ac", "2", "-shortest", source}
	if out, err := exec.CommandContext(ctx, bin, generate...).CombinedOutput(); err != nil {
		t.Fatalf("benchmark source: %v: %s", err, out)
	}
	run := func(args []string, input io.Reader) time.Duration {
		t.Helper()
		args = append([]string(nil), args...)
		for i := 1; i < len(args); i++ {
			if args[i-1] == "-progress" {
				args[i] = "pipe:2"
			}
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Stdin = input
		cmd.Stdout = io.Discard
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("benchmark process: %v: %s", err, &stderr)
		}
		return cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()
	}
	decodePCM := func(path string) []byte {
		t.Helper()
		out, err := exec.CommandContext(ctx, bin, "-v", "error", "-i", path, "-map", "0:a:0", "-c:a", "pcm_f32le", "-f", "f32le", "pipe:1").Output()
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	referencePCM := decodePCM(source)
	profile := DefaultProfile()
	profile.Encoder, profile.Width, profile.Height, profile.Framerate = EncoderSoftware, 320, 180, 25
	for _, continuous := range []bool{false, true} {
		child := filepath.Join(t.TempDir(), "child.ts")
		spec := ProgramSpec{SessionAudio: continuous, Profile: profile, Input: source, Limit: 20 * time.Second, Plan: CopyPlan{CopyVideo: true, CopyAudio: true}, UnpacedInput: true,
			Clock: ProgramClock{Origin: time.Unix(1000, 0), StartedAt: time.Unix(1010, 0)}}
		childCPU := run(replaceOutput(ProgramArgs(spec), child), nil)
		input, err := os.Open(child)
		if err != nil {
			t.Fatal(err)
		}
		args := BlockMuxArgs(BlockProfile{AudioBitrate: 128, PreparedStart: true})
		if !continuous {
			// Frozen prior prepared contract: independently encoded AAC enters a copy-only parent.
			args = []string{"-v", "error", "-readrate", "1.0", "-readrate_initial_burst", "2", "-copyts", "-probesize", "256k", "-analyzeduration", "500000", "-f", "mpegts", "-i", "pipe:0",
				"-map", "0:v:0", "-map", "0:a:0", "-c", "copy", "-muxdelay", "0", "-f", "mpegts", "-mpegts_flags", "+initial_discontinuity", "pipe:1"}
		}
		parentOutput := filepath.Join(t.TempDir(), "parent.ts")
		parentCPU := run(replaceOutput(args, parentOutput), input)
		if err := input.Close(); err != nil {
			t.Fatal(err)
		}
		t.Logf("continuous_aac=%t source_seconds=20 child_cpu=%s parent_cpu=%s total_cpu=%s core_fraction=%.4f", continuous, childCPU, parentCPU, childCPU+parentCPU, (childCPU+parentCPU).Seconds()/20)
		pcm := decodePCM(parentOutput)
		// AAC transport retains one 1024-sample priming frame. MP4's input edit removes it;
		// align that codec delay, then compare every available sample over the intended 20s.
		const primingBytes = 1024 * 2 * 4
		if len(pcm) <= primingBytes {
			t.Fatal("CPU sample has no decoded audio")
		}
		pcm = pcm[primingBytes:]
		count := min(len(pcm), len(referencePCM), 20*48000*2*4) / 4
		var energy, noise float64
		for i := range count {
			x := float64(math.Float32frombits(binary.LittleEndian.Uint32(referencePCM[i*4:])))
			y := float64(math.Float32frombits(binary.LittleEndian.Uint32(pcm[i*4:])))
			energy += x * x
			noise += (x - y) * (x - y)
		}
		if count == 0 || energy == 0 {
			t.Fatal("quality reference is empty or silent")
		}
		t.Logf("continuous_aac=%t compared_samples_per_channel=%d snr_db=%.2f", continuous, count/2, 10*math.Log10(energy/noise))

	}
}
