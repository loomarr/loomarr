//go:build !windows

package playout

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/execfixture"
)

func TestCopyStartProbeBoundsFailureAndRetainsValidProof(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		valid        bool
	}{
		{"valid", `printf '%s' '{"streams":[{"index":0,"codec_type":"video","has_b_frames":0}],"packets":[{"stream_index":0,"pts_time":"0","flags":"K_"}],"format":{"start_time":"0"}}'`, true},
		{"deadline", "exec sleep 10", false},
		{"output bound", `printf '%s' '{"streams":[{"index":0,"codec_type":"video","has_b_frames":0}],"packets":[{"stream_index":0,"pts_time":"0","flags":"K_"}],"format":{"start_time":"0"}}'; printf '%1048576s' ' '`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := execfixture.POSIX(t, "ffprobe", tc.script)
			start := time.Now()
			seek, ok := FFprobeCopyStartNextTo(bin)(t.Context(), "owned-fixture", time.Millisecond, time.Second, 25)
			if ok != tc.valid || seek != 0 {
				t.Fatalf("probe=%s,%t expected valid=%t", seek, ok, tc.valid)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("copy proof exceeded its bounded subprocess lifetime")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	bin := execfixture.POSIX(t, "ffprobe", "exec sleep 10")
	if _, ok := FFprobeCopyStartNextTo(bin)(ctx, "owned-fixture", 0, time.Second, 25); ok {
		t.Fatal("cancelled probe granted copy")
	}
}
