package playoutbench

import (
	"bytes"
	"testing"
)

func TestOffGrid(t *testing.T) {
	const step = 3600
	clean := []int64{0, 3600, 7200, 10800}
	if n := offGrid(clean, step); n != 0 {
		t.Errorf("a clean grid has no gaps, got %d", n)
	}
	// One dropped frame (a 2-frame delta) and one duplicate: both are off-grid.
	torn := []int64{0, 3600, 10800, 10800, 14400}
	if n := offGrid(torn, step); n != 2 {
		t.Errorf("gap + duplicate = 2 off-grid deltas, got %d", n)
	}
	// One tick of rounding is tolerated; two is not.
	if n := offGrid([]int64{0, 3601, 7201}, step); n != 0 {
		t.Errorf("a one-tick jitter is on the grid, got %d", n)
	}
	if n := offGrid([]int64{0, 3603}, step); n != 1 {
		t.Errorf("a three-tick error is off the grid, got %d", n)
	}
}

func TestSPSNAL(t *testing.T) {
	sps := []byte{0x67, 0x64, 0x00, 0x28, 0xac}
	stream := bytes.Join([][]byte{
		{0, 0, 0, 1, 0x09, 0xf0}, // access unit delimiter
		{0, 0, 0, 1}, sps,
		{0, 0, 0, 1, 0x68, 0xee}, // PPS
	}, nil)
	got := spsNAL(stream)
	if !bytes.Equal(got, sps) {
		t.Fatalf("sps = % x, want % x", got, sps)
	}
	if spsNAL([]byte{0, 0, 1, 0x65, 1, 2, 3}) != nil {
		t.Error("a stream with no SPS must return nil, not an IDR slice")
	}
}

func TestParseIntegratedLoudness(t *testing.T) {
	log := `[Parsed_ebur128_0 @ 0x1] t: 5.9 TARGET:-23 LUFS M: -22.9 S: -23.0 I: -23.1 LUFS LRA: 0.0 LU
[Parsed_ebur128_0 @ 0x1] Summary:

  Integrated loudness:
    I:         -23.2 LUFS
    Threshold: -33.2 LUFS
`
	got, err := parseIntegratedLoudness(log)
	if err != nil || got != -23.2 {
		t.Fatalf("got %v, %v; want -23.2 (the summary, not the running value)", got, err)
	}
	silent := "Summary:\n\n  Integrated loudness:\n    I:         -inf LUFS\n"
	if _, err := parseIntegratedLoudness(silent); err == nil {
		t.Error("silent output must be an error, not a pass")
	}
	if _, err := parseIntegratedLoudness("nothing"); err == nil {
		t.Error("no summary must be an error")
	}
}

func TestCorpusShape(t *testing.T) {
	seen := map[string]bool{}
	classes := map[string]int{}
	spots := 0
	for _, c := range Corpus() {
		if seen[c.Name] {
			t.Errorf("duplicate clip name %s", c.Name)
		}
		seen[c.Name] = true
		classes[c.Class]++
		if c.Break {
			spots++
		}
		args := c.Args("/x/" + c.Name + ".mkv")
		// Encode options must follow BOTH inputs or ffmpeg binds them to the second input.
		last, enc := -1, -1
		for i, a := range args {
			if a == "-i" {
				last = i
			}
			if a == "-c:v" {
				enc = i
			}
		}
		if enc < last {
			t.Errorf("%s: -c:v precedes the last -i, so it would configure the audio input", c.Name)
		}
	}
	for _, class := range []string{"h264-1080p", "hevc-1080p", "hdr-4k"} {
		if classes[class] != 1 {
			t.Errorf("class %s has %d clips, want exactly 1", class, classes[class])
		}
	}
	if spots < 3 {
		t.Errorf("the break sequence needs several spots, got %d", spots)
	}
}
