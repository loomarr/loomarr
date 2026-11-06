package encode_test

import (
	"io"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/encode"
)

// One sample per block, with out_time_ms read as the MICROSECONDS ffmpeg actually writes, and a
// malformed speed ("N/A" during teardown) leaving the previous value in place.
func TestReadProgress_OneSamplePerBlockInMilliseconds(t *testing.T) {
	stream := "frame=50\nout_time_ms=2000000\nspeed=2.5x\nprogress=continue\n" +
		"frame=100\nout_time_ms=4000000\nspeed=N/A\nprogress=end\n"
	var got []encode.Progress
	encode.ReadProgress(io.NopCloser(strings.NewReader(stream)), func(p encode.Progress) { got = append(got, p) })

	want := []encode.Progress{
		{Frame: 50, Speed: 2.5, OutTimeMS: 2000},
		{Frame: 100, Speed: 2.5, OutTimeMS: 4000},
	}
	if len(got) != len(want) {
		t.Fatalf("samples = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A nil callback still drains the pipe: an undrained pipe fills and stalls ffmpeg's encode.
func TestReadProgress_NilCallbackDrains(t *testing.T) {
	r := strings.NewReader(strings.Repeat("frame=1\nprogress=continue\n", 1000))
	encode.ReadProgress(io.NopCloser(r), nil)
	if r.Len() != 0 {
		t.Fatalf("%d bytes left unread, want the pipe drained", r.Len())
	}
}
