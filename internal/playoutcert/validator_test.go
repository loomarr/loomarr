package playoutcert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestParseProbeRequiresExactlyOneVideoAndAudio(t *testing.T) {
	shape, err := parseProbe([]byte(`{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"aac"}]}`))
	if err != nil || shape.VideoStreams != 1 || shape.AudioStreams != 1 || shape.VideoCodec != "h264" || shape.AudioCodec != "aac" {
		t.Fatalf("parseProbe = %+v, %v", shape, err)
	}
	if _, err := parseProbe([]byte(`{"streams":[{"codec_type":"video","codec_name":"h264"}]}`)); err == nil {
		t.Fatal("video-only media passed validation")
	}
	if _, err := parseProbe([]byte(`{"streams":[{"codec_type":"video"},{"codec_type":"video"},{"codec_type":"audio"}]}`)); err == nil {
		t.Fatal("two-video media passed validation")
	}
}

func TestFFmpegDecoderRetainsInputPumpErrorAndClosesInput(t *testing.T) {
	input := &errorReadCloser{}
	decoder := FFmpegDecoder{Path: "helper", command: helperDecoderCommand("progress")}
	if err := decoder.Decode(context.Background(), input, func(int64) {}); err == nil {
		t.Fatal("input pump error was discarded")
	}
	if !input.closed {
		t.Fatal("decoder input was not closed")
	}
}

func TestFFmpegDecoderRetainsProgressScannerError(t *testing.T) {
	decoder := FFmpegDecoder{Path: "helper", command: helperDecoderCommand("oversized_progress")}
	if err := decoder.Decode(context.Background(), io.NopCloser(strings.NewReader("media")), func(int64) {}); err == nil {
		t.Fatal("progress scanner error was discarded")
	}
}

func TestFFmpegDecoderHelperProcess(t *testing.T) {
	if os.Getenv("LOOMARR_DECODER_HELPER") != "1" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	if os.Getenv("LOOMARR_DECODER_HELPER_MODE") == "oversized_progress" {
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", 70<<10))
	} else {
		_, _ = fmt.Fprintln(os.Stdout, "frame=1")
	}
	os.Exit(0)
}

func helperDecoderCommand(mode string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFFmpegDecoderHelperProcess$")
		command.Env = append(os.Environ(), "LOOMARR_DECODER_HELPER=1", "LOOMARR_DECODER_HELPER_MODE="+mode)
		return command
	}
}

type errorReadCloser struct {
	closed bool
}

func (*errorReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("controlled input failure")
}

func (r *errorReadCloser) Close() error {
	r.closed = true
	return nil
}
