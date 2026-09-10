//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestLivePreparedMuxDecodesShortInitialBurstWithoutWaitingForNextAiring(t *testing.T) {
	bin := ffmpegBin(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	// A valid short child tail has both streams, but fewer bytes than the old 256 KiB probe.
	generate := exec.CommandContext(ctx, bin,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "lavfi", "-i", "testsrc2=duration=0.32:size=320x180:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=0.32",
		"-c:v", "libx264", "-preset", "ultrafast", "-bf", "0", "-g", "5",
		"-c:a", "s302m", "-strict", "-2", "-ac", "2", "-f", "mpegts", "pipe:1")
	burst, err := generate.Output()
	if err != nil {
		t.Fatalf("generate private child transport: %v", err)
	}
	if len(burst) <= 32<<10 || len(burst) >= 256<<10 {
		t.Fatalf("fixture transport size=%d, want between 32 and 256 KiB", len(burst))
	}
	args := BlockMuxArgs(BlockProfile{AudioBitrate: 128, PreparedStart: true})
	for i := range args {
		if args[i] == progressPipeArg() {
			args[i] = "pipe:2" // This direct process has no production diagnostic file descriptor.
		}
	}
	// Keep input open throughout the observation window. No successor bytes or EOF may
	// unblock initial analysis; afterwards decode only what the running mux already emitted.
	probeCtx, probeCancel := context.WithTimeout(ctx, 3*time.Second)
	defer probeCancel()
	mux := exec.CommandContext(probeCtx, bin, args...)
	stdin, err := mux.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	var transport, stderr bytes.Buffer
	mux.Stdout, mux.Stderr = &transport, &stderr
	if err := mux.Start(); err != nil {
		t.Fatal(err)
	}
	written := make(chan error, 1)
	go func() { _, err := stdin.Write(burst); written <- err }()
	waitErr := mux.Wait()
	_ = stdin.Close()
	<-written
	if probeCtx.Err() == nil {
		t.Fatalf("continuous mux exited before the observation window: %v\n%s", waitErr, stderr.String())
	}
	decode := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error",
		"-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-frames:v", "1",
		"-f", "rawvideo", "-pix_fmt", "rgb24", "pipe:1")
	decode.Stdin = bytes.NewReader(transport.Bytes())
	frame, err := decode.Output()
	if err != nil || len(frame) != 320*180*3 {
		t.Fatalf("short initial burst did not yield a decoded frame: bytes=%d err=%v", len(frame), err)
	}
}
