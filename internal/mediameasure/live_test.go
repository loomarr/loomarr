package mediameasure

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// procRchar reads the bytes a process has asked the kernel to read (rchar counts network-mount
// reads, which read_bytes does not). It returns 0 where /proc is unavailable.
func procRchar(pid int) int64 {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/io", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(line, "rchar: "); ok {
			n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			return n
		}
	}
	return 0
}

// TestLive_MeasureFile times each measurement step on a real file and reports the bytes each step
// read. It is skipped unless MEDIAMEASURE_LIVE_FILE names one, so it can be pointed at library
// files (for example inside a container with the media mounted read-only):
//
//	MEDIAMEASURE_LIVE_FILE=/path/to/film.mkv go test ./internal/mediameasure -run Live -v
func TestLive_MeasureFile(t *testing.T) {
	path := os.Getenv("MEDIAMEASURE_LIVE_FILE")
	if path == "" {
		t.Skip("set MEDIAMEASURE_LIVE_FILE to measure a real file")
	}
	ctx := context.Background()
	tools := DefaultTools(os.Getenv("MEDIAMEASURE_FFMPEG"), os.Getenv("MEDIAMEASURE_FFPROBE"))
	var childBytes atomic.Int64
	tools.Run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		var stdout, stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := lowPriority(cmd); err != nil {
			return nil, nil, err
		}
		done := make(chan struct{})
		go func() {
			var last int64
			for {
				if n := procRchar(cmd.Process.Pid); n > last {
					last = n
					childBytes.Store(n)
				}
				select {
				case <-done:
					return
				case <-time.After(20 * time.Millisecond):
				}
			}
		}()
		err := cmd.Wait()
		close(done)
		return stdout.Bytes(), stderr.Bytes(), err
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("file: %.2f GiB", float64(info.Size())/(1<<30))

	childBytes.Store(0)
	start := time.Now()
	out, _, err := tools.Run(ctx, tools.FFprobe, "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ffprobe format: %s, %d KiB read", time.Since(start).Round(time.Millisecond), childBytes.Load()>>10)
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Fatal(err)
	}
	durationMs := int64(seconds * 1000)

	before, start := procRchar(os.Getpid()), time.Now()
	childBytes.Store(0)
	frames, err := tools.Keyframes(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	read := procRchar(os.Getpid()) - before + childBytes.Load()
	t.Logf("keyframes: %d in %s, %d KiB read (%.4f%% of the file)", len(frames),
		time.Since(start).Round(time.Millisecond), read>>10, 100*float64(read)/float64(info.Size()))

	if os.Getenv("MEDIAMEASURE_SKIP_DECODE") != "" {
		return
	}
	childBytes.Store(0)
	start = time.Now()
	quality, loudness, err := tools.Decode(ctx, path, durationMs, true, true)
	if err != nil {
		t.Fatal(err)
	}
	breaks := BreakCandidates(quality.Black, quality.Silence, frames, durationMs)
	t.Logf("decode pass: %s for %.0f s of media, %d MiB read; black %d, silence %d; %.1f LUFS, true peak %+v",
		time.Since(start).Round(time.Millisecond), seconds, childBytes.Load()>>20,
		len(quality.Black), len(quality.Silence), loudness.IntegratedLUFS, loudness.TruePeak)
	for _, b := range breaks {
		t.Logf("break candidate: at %d ms, keyframe %d ms, overlap %d ms, confidence %.2f", b.AtMs, b.KeyframeMs, b.OverlapMs, b.Confidence)
	}
}
