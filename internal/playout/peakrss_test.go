package playout

import "testing"

func TestMaxRSSBytesUnits(t *testing.T) {
	type rusage struct{ Maxrss int64 }
	for _, tc := range []struct {
		name  string
		usage any
		goos  string
		want  int64
	}{
		{"linux reports KiB", &rusage{Maxrss: 2048}, "linux", 2048 * 1024},
		{"darwin reports bytes", &rusage{Maxrss: 2048}, "darwin", 2048},
		{"unknown platform is unmeasured", &rusage{Maxrss: 2048}, "plan9", 0},
		{"no Maxrss field", &struct{ UserTime int64 }{UserTime: 5}, "linux", 0},
		{"nil usage", (*rusage)(nil), "linux", 0},
		{"non-struct usage", 42, "linux", 0},
	} {
		if got := maxRSSBytes(tc.usage, tc.goos); got != tc.want {
			t.Errorf("%s: maxRSSBytes = %d, want %d", tc.name, got, tc.want)
		}
	}
}
