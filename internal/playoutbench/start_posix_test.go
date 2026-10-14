//go:build !windows

package playoutbench

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/testkit/execfixture"
	"github.com/loomarr/loomarr/internal/testkit/playoutstreamfixture"
)

func startFixture(t *testing.T, stream []byte, after string) *run {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "stream.mp4")
	if err := os.WriteFile(path, stream, 0o600); err != nil {
		t.Fatal(err)
	}
	ffmpeg := execfixture.POSIX(t, "encoder", "cat '"+path+"'\n"+after)
	return &run{Options: Options{Dir: dir, FFmpeg: ffmpeg}}
}

func TestStartSegmentDoesNotClockBlockedEncoderExit(t *testing.T) {
	r := startFixture(t, playoutstreamfixture.FragmentedMP4(t, 25, 25), "exec sleep 10")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	elapsed, err := r.startSegment(ctx, "input", 0, playout.Pipeline{}, 25)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed >= time.Second {
		t.Fatalf("startup %s included encoder exit", elapsed)
	}
	if ctx.Err() != nil {
		t.Fatal("startup only ended when the caller timed out")
	}
}

func TestStartSegmentRejectsIncompleteMediaAndEncoderFailure(t *testing.T) {
	valid := playoutstreamfixture.FragmentedMP4(t, 25, 25)
	for _, tc := range []struct {
		name   string
		stream []byte
		after  string
	}{
		{"short second", playoutstreamfixture.FragmentedMP4(t, 25, 12), ""},
		{"partial mdat", valid[:len(valid)-1], ""},
		{"no media", nil, "echo decoder-failed >&2\nexit 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := startFixture(t, tc.stream, tc.after)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if elapsed, err := r.startSegment(ctx, "input", 0, playout.Pipeline{}, 25); err == nil {
				t.Fatalf("invalid first segment accepted at %s", elapsed)
			} else if strings.Contains(err.Error(), "context deadline exceeded") {
				t.Fatalf("encoder failure was hidden until timeout: %v", err)
			} else if tc.name == "no media" && !strings.Contains(err.Error(), "decoder-failed") {
				t.Fatalf("startup lost the encoder diagnostic: %v", err)
			}
		})
	}
}
