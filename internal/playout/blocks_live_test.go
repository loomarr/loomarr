//go:build ffmpeg

package playout

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
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
	out, err := exec.CommandContext(ctx, probe, "-v", "error", "-show_packets", "-show_data_hash", "sha256", "-show_entries", "stream=index,codec_type,time_base:packet=stream_index,pts,dts,duration,data_hash", "-of", "json", path).Output()
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
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	paths := []string{filepath.Join(dir, "aligned.ts"), filepath.Join(dir, "audio-delay.ts")}
	for i, path := range paths {
		offset := "0"
		if i == 1 {
			offset = "0.3"
		}
		args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=duration=3:size=320x180:rate=25", "-itsoffset", offset, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=3", "-map", "0:v:0", "-map", "1:a:0", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-g", "25", "-bf", "3", "-sc_threshold", "0", "-c:a", "aac", "-ac", "2", "-ar", "48000", "-t", "3", "-f", "mpegts", "-mpegts_flags", "+initial_discontinuity", path}
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
	proc, err := StartPipedObserved(ctx, bin, BlockMuxArgs(), nil, nil, nil, diagnostics.ProcessSpec{})
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
	assertProgrammesPreserved(t, programmes, output)
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
		parseMuxPacket(t, packet, label)
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
	audioPTS, _, _ := parseMuxPacket(t, programme["audio"][0], "second programme audio")
	if delay := audioPTS - videoPTS; abs64(delay-27000) > 4500 {
		t.Fatalf("second programme source A/V delay=%d ticks, want approximately 27000", delay)
	}
}

func assertProgrammesPreserved(t *testing.T, programmes []map[string][]muxPacket, output map[string][]muxPacket) {
	t.Helper()
	start := map[string]int{"video": 0, "audio": 0}
	for programmeIndex, programme := range programmes {
		var shift int64
		shiftSet := false
		for _, codec := range []string{"video", "audio"} {
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
	for _, codec := range []string{"video", "audio"} {
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
