package playout

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/media/encode"
)

// A failed trial names why ffmpeg failed even when the reason is the last thing it printed after a
// long stderr: the pipe is read to the end before the process is reaped, so cmd.Wait never closes it
// under the reader and drops the tail.
func TestTrialEncode_FailureCarriesTheTailOfStderr(t *testing.T) {
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\ni=0\nwhile [ $i -lt 400 ]; do echo 'noise noise noise noise noise noise noise noise noise noise noise noise noise noise noise noise noise noise noise noise' >&2; i=$((i+1)); done\n" +
		"echo 'Driver does not support the required nvenc API version. Required: 13.0 The minimum required Nvidia driver for nvenc is 570' >&2\nexit 1\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	const runs = 50
	lost := 0
	for range runs {
		c := trialEncodeObserved(context.Background(), ffmpeg, encode.EncoderSoftware, DefaultProfile(), 1, nil)
		if c.Works {
			t.Fatal("a failing ffmpeg reported Works")
		}
		if !strings.Contains(c.Err, "minimum required Nvidia driver") {
			lost++
		}
	}
	if lost > 0 {
		t.Errorf("%d of %d failed trials lost the tail of ffmpeg's stderr", lost, runs)
	}
}
