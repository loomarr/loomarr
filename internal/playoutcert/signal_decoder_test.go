package playoutcert

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/execfixture"
	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestParseVideoSignalsAcceptsNumericRecords(t *testing.T) {
	var got []DecodedVideoSignal
	err := parseVideoSignals(strings.NewReader("frame:0 pts:-100000 pts_time:-0.1\nlavfi.signalstats.YAVG=16.5\nframe:1 pts:0 pts_time:0\nlavfi.signalstats.YAVG=17\n"), func(signal DecodedVideoSignal) { got = append(got, signal) })
	if err != nil || len(got) != 2 || got[0].PTSUS != -100000 || got[1].Luma != 17 {
		t.Fatalf("parseVideoSignals = %#v, %v", got, err)
	}
}

func TestParseAudioSignalsAcceptsSilenceSentinel(t *testing.T) {
	var got []DecodedAudioSignal
	err := parseAudioSignals(strings.NewReader("frame:0 pts:1 pts_time:0.000001\nlavfi.astats.1.Zero_crossings_rate=0\nlavfi.astats.1.RMS_level=-inf\nlavfi.astats.Overall.Number_of_samples=1024.000000\n"), func(signal DecodedAudioSignal) { got = append(got, signal) })
	if err != nil || len(got) != 1 || !got[0].Silence || got[0].Samples != 1024 {
		t.Fatalf("parseAudioSignals = %#v, %v", got, err)
	}
}

func TestSignalMetadataRejectsMalformedRecords(t *testing.T) {
	cases := []string{
		"frame:0 pts:1\nlavfi.signalstats.YAVG=1\n",
		"frame:0 pts:1 pts_time:0\nlavfi.signalstats.YAVG=1\ntrailing malformed fragment",
		"frame:0 pts:1 pts_time:0\nlavfi.signalstats.YAVG=1\nlavfi.signalstats.YAVG=2\n",
		"frame:1 pts:2 pts_time:0\nlavfi.signalstats.YAVG=1\nframe:1 pts:3 pts_time:0\nlavfi.signalstats.YAVG=1\n",
		"frame:1 pts:2 pts_time:0\nlavfi.signalstats.YAVG=1\nframe:2 pts:1 pts_time:0\nlavfi.signalstats.YAVG=1\n",
		"frame:1 pts:2 pts_time:nan\nlavfi.signalstats.YAVG=1\n",
		"frame:1 pts:2 pts_time:0\nlavfi.signalstats.YAVG=" + strings.Repeat("1", 17<<10) + "\n",
	}
	for _, raw := range cases {
		t.Run("invalid", func(t *testing.T) {
			if err := parseVideoSignals(strings.NewReader(raw), func(DecodedVideoSignal) {}); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
}

func TestSignalMetadataRejectsMalformedAudioRecordAtEOF(t *testing.T) {
	raw := "frame:0 pts:1 pts_time:0\n" +
		"lavfi.astats.Overall.Number_of_samples=1024\n" +
		"lavfi.astats.1.Zero_crossings_rate=0\n" +
		"lavfi.astats.1.RMS_level=-20\n" +
		"trailing malformed fragment"
	if err := parseAudioSignals(strings.NewReader(raw), func(DecodedAudioSignal) {}); err == nil {
		t.Fatal("malformed audio metadata accepted")
	}
}

func TestSignalMetadataRejectsAudioBoundaries(t *testing.T) {
	base := "frame:0 pts:0 pts_time:0\nlavfi.astats.Overall.Number_of_samples=%s\nlavfi.astats.1.Zero_crossings_rate=%s\nlavfi.astats.1.RMS_level=%s\n"
	for _, values := range [][3]string{{"0", "0", "-1"}, {"1048577", "0", "-1"}, {"1", "1.1", "-1"}, {"1", "0", "0.1"}, {"1", "nan", "-1"}} {
		raw := fmt.Sprintf(base, values[0], values[1], values[2])
		if err := parseAudioSignals(strings.NewReader(raw), func(DecodedAudioSignal) {}); err == nil {
			t.Fatalf("invalid audio metadata accepted: %q", raw)
		}
	}
}

func TestSignalMetadataRequiresBothMedia(t *testing.T) {
	if err := parseVideoSignals(strings.NewReader(""), func(DecodedVideoSignal) {}); err == nil {
		t.Fatal("empty video metadata accepted")
	}
	if err := parseAudioSignals(strings.NewReader("frame:0 pts:1 pts_time:0\nlavfi.astats.Overall.Number_of_samples=1\n"), func(DecodedAudioSignal) {}); err == nil {
		t.Fatal("incomplete audio metadata accepted")
	}
}

func TestFFmpegSignalDecoderDeliversBothSignalStreams(t *testing.T) {
	path := execfixture.POSIX(t, "signals", "printf 'frame:0 pts:0 pts_time:0\\nlavfi.signalstats.YAVG=16\\nframe:1 pts:40000 pts_time:0.04\\nlavfi.signalstats.YAVG=235\\n'; printf 'frame:0 pts:0 pts_time:0\\nlavfi.astats.Overall.Number_of_samples=1024\\nlavfi.astats.1.Zero_crossings_rate=0.02\\nlavfi.astats.1.RMS_level=-20\\n' >&2; cat >/dev/null")
	var closes atomic.Int32
	input := &playoutcertfixture.CountingReadCloser{ReadCloser: io.NopCloser(strings.NewReader("media")), Closed: &closes}
	var video []DecodedVideoSignal
	var audio []DecodedAudioSignal
	decoder := testSignalDecoder(path)
	if err := decoder.DecodeSignals(context.Background(), input, func(value DecodedVideoSignal) { video = append(video, value) }, func(value DecodedAudioSignal) { audio = append(audio, value) }); err != nil {
		t.Fatalf("DecodeSignals() error = %v", err)
	}
	if len(video) != 2 || len(audio) != 1 || video[1].PTSUS != 40000 || audio[0].Samples != 1024 || closes.Load() != 1 {
		t.Fatalf("signals = video=%#v audio=%#v closes=%d", video, audio, closes.Load())
	}
}

func TestFFmpegSignalDecoderJoinsBlockedInputAfterChildMetadataExit(t *testing.T) {
	path := execfixture.POSIX(t, "signals-exit", "printf 'frame:0 pts:0 pts_time:0\\nlavfi.signalstats.YAVG=16\\n'; printf 'frame:0 pts:0 pts_time:0\\nlavfi.astats.Overall.Number_of_samples=1024\\nlavfi.astats.1.Zero_crossings_rate=0.02\\nlavfi.astats.1.RMS_level=-20\\n' >&2")
	var closes atomic.Int32
	input := playoutcertfixture.NewBlockingReadCloser(&closes)
	started := make(chan struct{}, 1)
	input.ReadStarted = started
	var callbacks atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- testSignalDecoder(path).DecodeSignals(context.Background(), input,
			func(DecodedVideoSignal) { callbacks.Add(1) },
			func(DecodedAudioSignal) { callbacks.Add(1) })
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("input was not read")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("incomplete input accepted after child metadata exit")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("decoder did not join blocked input after child exit")
	}
	if closes.Load() != 1 || callbacks.Load() != 2 {
		t.Fatalf("closes=%d callbacks=%d", closes.Load(), callbacks.Load())
	}
	time.Sleep(20 * time.Millisecond)
	if callbacks.Load() != 2 {
		t.Fatalf("callback after return: %d", callbacks.Load())
	}
}

func TestFFmpegSignalDecoderRejectsInputFailureAfterChildMetadataExit(t *testing.T) {
	path := execfixture.POSIX(t, "signals-exit", "printf 'frame:0 pts:0 pts_time:0\\nlavfi.signalstats.YAVG=16\\n'; printf 'frame:0 pts:0 pts_time:0\\nlavfi.astats.Overall.Number_of_samples=1024\\nlavfi.astats.1.Zero_crossings_rate=0.02\\nlavfi.astats.1.RMS_level=-20\\n' >&2")
	var closes atomic.Int32
	input := playoutcertfixture.NewBlockingReadCloser(&closes)
	input.Terminal = errors.New("late source failure")
	started := make(chan struct{}, 1)
	input.ReadStarted = started
	done := make(chan error, 1)
	go func() {
		done <- testSignalDecoder(path).DecodeSignals(context.Background(), input, func(DecodedVideoSignal) {}, func(DecodedAudioSignal) {})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("input was not read")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("late input failure accepted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("decoder did not join late input failure")
	}
	if closes.Load() != 1 {
		t.Fatalf("input close count = %d, want 1", closes.Load())
	}
}

func TestFFmpegSignalDecoderClosesBlockedInputOnParserFailure(t *testing.T) {
	path := execfixture.POSIX(t, "bad-signal", "printf 'frame:0 pts:0 pts_time:0\\nlavfi.signalstats.YAVG=bogus\\n'; exec 1>&-; while :; do printf 'frame:0 pts:0 pts_time:0\\nlavfi.astats.Overall.Number_of_samples=1\\nlavfi.astats.1.Zero_crossings_rate=0\\nlavfi.astats.1.RMS_level=-1\\n' >&2; sleep 1; done")
	var closes atomic.Int32
	input := playoutcertfixture.NewBlockingReadCloser(&closes)
	decoder := FFmpegSignalDecoder{Path: path, command: func(ctx context.Context, path string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, path)
	}}
	deadline := time.After(2 * time.Second)
	done := make(chan error, 1)
	go func() {
		done <- decoder.DecodeSignals(context.Background(), input, func(DecodedVideoSignal) {}, func(DecodedAudioSignal) {})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("decoder accepted malformed metadata")
		}
	case <-deadline:
		t.Fatal("decoder did not join after parser failure")
	}
	if closes.Load() != 1 {
		t.Fatalf("input close count = %d, want 1", closes.Load())
	}
}

func TestFFmpegSignalDecoderAbortsOnInputFailure(t *testing.T) {
	path := execfixture.POSIX(t, "wait-input", "cat >/dev/null")
	var closes atomic.Int32
	input := &playoutcertfixture.CountingReadCloser{ReadCloser: io.NopCloser(failingReader{}), Closed: &closes}
	if err := testSignalDecoder(path).DecodeSignals(context.Background(), input, func(DecodedVideoSignal) {}, func(DecodedAudioSignal) {}); err == nil {
		t.Fatal("input failure accepted")
	}
	if closes.Load() != 1 {
		t.Fatalf("input close count = %d, want 1", closes.Load())
	}
}

func TestFFmpegSignalDecoderCancellationClosesBlockedInput(t *testing.T) {
	path := execfixture.POSIX(t, "wait-input", "cat >/dev/null")
	var closes atomic.Int32
	input := playoutcertfixture.NewBlockingReadCloser(&closes)
	started := make(chan struct{}, 1)
	input.ReadStarted = started
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- testSignalDecoder(path).DecodeSignals(ctx, input, func(DecodedVideoSignal) {}, func(DecodedAudioSignal) {})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("input was not read")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("decoder did not return after cancellation")
	}
	if closes.Load() != 1 {
		t.Fatalf("input close count = %d, want 1", closes.Load())
	}
}

func TestFFmpegSignalDecoderSetupFailuresCloseInputOnce(t *testing.T) {
	cases := []struct {
		name    string
		command func(context.Context, string, ...string) *exec.Cmd
	}{
		{"stdin", func(context.Context, string, ...string) *exec.Cmd {
			cmd := exec.Command("true")
			cmd.Stdin = strings.NewReader("")
			return cmd
		}},
		{"video", func(context.Context, string, ...string) *exec.Cmd {
			cmd := exec.Command("true")
			cmd.Stdout = &bytes.Buffer{}
			return cmd
		}},
		{"start", func(context.Context, string, ...string) *exec.Cmd { return exec.Command("") }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var closes atomic.Int32
			input := &playoutcertfixture.CountingReadCloser{ReadCloser: io.NopCloser(strings.NewReader("media")), Closed: &closes}
			if err := (FFmpegSignalDecoder{command: test.command}).DecodeSignals(context.Background(), input, func(DecodedVideoSignal) {}, func(DecodedAudioSignal) {}); err == nil {
				t.Fatal("setup failure accepted")
			}
			if closes.Load() != 1 {
				t.Fatalf("input close count = %d, want 1", closes.Load())
			}
		})
	}
}

func testSignalDecoder(path string) FFmpegSignalDecoder {
	return FFmpegSignalDecoder{Path: path, command: func(ctx context.Context, path string, _ ...string) *exec.Cmd { return exec.CommandContext(ctx, path) }}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("input failed") }
