package prepared

import (
	"strings"
	"testing"
)

func TestPreparedRandomAccessObservesKeyframesAndTerminalTail(t *testing.T) {
	t.Parallel()
	const first = "pts_time=0.000000|duration_time=0.040000|flags=K_\n"
	for _, tc := range []struct {
		name, packets string
		valid         bool
	}{
		{"single frame", first, true},
		{"bounded tail", first + "pts_time=0.160000|duration_time=0.040000|flags=__\n", true},
		{"next access", first + "pts_time=0.200000|duration_time=0.040000|flags=K_\n", true},
		{"rounded bound", first + "pts_time=0.200001|duration_time=0.040000|flags=K_\n", true},
		{"late next access", first + "pts_time=0.240000|duration_time=0.040000|flags=K_\n", false},
		{"uncovered tail", first + "pts_time=0.180000|duration_time=0.040000|flags=__\n", false},
		{"duplicate timestamps", first + first, false},
		{"reordered timestamps", first + "pts_time=0.160000|duration_time=0.040000|flags=K_\n" + first, false},
		{"no initial access", "pts_time=0|duration_time=0.04|flags=__\n", false},
		{"missing", "", false},
		{"unknown timestamp", "pts_time=N/A|duration_time=0.04|flags=K_\n", false},
		{"negative timestamp", "pts_time=-1|duration_time=0.04|flags=K_\n", false},
		{"unknown duration", "pts_time=0|duration_time=N/A|flags=K_\n", false},
		{"zero duration", "pts_time=0|duration_time=0|flags=K_\n", false},
		{"missing flags", "pts_time=0|duration_time=0.04\n", false},
		{"duplicate field", "pts_time=0|pts_time=0|duration_time=0.04|flags=K_\n", false},
		{"oversized line", strings.Repeat("x", 4097), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateRandomAccess(strings.NewReader(tc.packets)); (err == nil) != tc.valid {
				t.Fatalf("accepted=%v, want %v: %v", err == nil, tc.valid, err)
			}
		})
	}
}
