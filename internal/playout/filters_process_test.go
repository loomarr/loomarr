//go:build !windows

package playout

import (
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/execfixture"
)

func TestGPUFiltersFor_VideoToolboxDeinterlacerComesFromConfiguredBinary(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		want         bool
	}{
		{"present", "printf '%s\\n' ' ... yadif_videotoolbox V->V Deinterlace video.'", true},
		{"absent", "printf '%s\\n' ' TS. yadif V->V CPU deinterlacer; alternative to yadif_videotoolbox.'", false},
		{"unreadable", "exit 1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := execfixture.POSIX(t, "ffmpeg", tc.script)
			if got := GPUFiltersFor(bin)().VideoToolboxDeinterlace; got != tc.want {
				t.Fatalf("configured binary deinterlacer capability=%t, want %t", got, tc.want)
			}
		})
	}
}
