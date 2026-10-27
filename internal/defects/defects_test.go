package defects

import (
	"reflect"
	"testing"
)

// The log and metadata are trimmed from a real ffmpeg n9 pass over an upscaled, clipped clip.
const sampleLog = `[Parsed_idet_7 @ 0x55f3d3c69f80] Multi frame detection: TFF:     0 BFF:     0 Progressive:     0 Undetermined:     0
[Parsed_astats_14 @ 0x7f7cd8021980] Overall
[Parsed_astats_14 @ 0x7f7cd8021980] Peak level dB: 0.000265
[Parsed_astats_14 @ 0x7f7cd8021980] Flat factor: 21.016619
[Parsed_astats_14 @ 0x7f7cd8021980] Peak count: 2582.000000
[Parsed_astats_14 @ 0x7f7cd8021980] Noise floor dB: -inf
[Parsed_ssim_16 @ 0x7f7cd8021500] SSIM Y:0.999102 (30.466002) U:0.998001 (26.989700) V:0.998050 (27.099000) All:0.998700 (28.860000)
[Parsed_ssim_12 @ 0x7f7cd8021440] SSIM Y:0.998015 (27.022983) U:0.996313 (24.333041) V:0.996283 (24.298030) All:0.997443 (25.922365)
[Parsed_idet_7 @ 0x7f7cd8006240] Multi frame detection: TFF:    10 BFF:    20 Progressive:    70 Undetermined:     4
[Parsed_blurdetect_3 @ 0x7f7cd8005740] blur mean: 4.5358817
[Parsed_blockdetect_2 @ 0x7f7cd8005500] block mean: 10.5984955
`

const sampleMetadata = `frame:0    pts:0       pts_time:0
lavfi.block=10.681479
lavfi.signalstats.TOUT=0.002
lavfi.signalstats.VREP=0.5
frame:1    pts:6400    pts_time:0.25
lavfi.signalstats.TOUT=0.004
lavfi.signalstats.VREP=0.3
`

// The ssim summaries are logged in the order the filters finish, not graph order: the quarter-size
// one (filter 16) comes first here and must still read as UpscaleSSIM4.
func TestParse_ReadsEverySummaryAndTheFrameLog(t *testing.T) {
	active := ActiveArea{Width: 600, Height: 440, X: 20, Y: 20}
	got := Parse(sampleLog, sampleMetadata, 640, 480, active, true)
	want := Readings{
		Version: MeasurementVersion, Width: 640, Height: 480, Frames: 2,
		BlockMean: 10.5984955, BlurMean: 4.5358817, OutlierShare: 0.003, RepeatedLineShare: 0.4,
		ActiveWidth: 600, ActiveHeight: 440, InterlacedShare: 0.1, UpscaleSSIM: 0.998015, UpscaleSSIM4: 0.999102,
		HasAudio: true, PeakDB: 0.000265, FlatFactor: 21.016619, PeakCount: 2582, NoiseFloorDB: -200,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse =\n %+v\nwant\n %+v", got, want)
	}
	if silent := Parse(sampleLog, sampleMetadata, 640, 480, active, false); silent.FlatFactor != 0 || silent.PeakCount != 0 {
		t.Fatalf("a clip with no audio stream took audio readings: %+v", silent)
	}
}

// cropdetect with reset=0 widens its box as the picture moves, so the last frame's box is the
// union over the clip.
const sampleCrop = `frame:0    pts:0       pts_time:0
lavfi.cropdetect.w=560
lavfi.cropdetect.h=400
lavfi.cropdetect.x=40
lavfi.cropdetect.y=40
frame:1    pts:12800   pts_time:0.5
lavfi.cropdetect.w=600
lavfi.cropdetect.h=440
lavfi.cropdetect.x=20
lavfi.cropdetect.y=20
`

func TestParseActiveArea_TakesTheClipsUnionAndRefusesAKeyhole(t *testing.T) {
	cases := []struct {
		name     string
		metadata string
		want     ActiveArea
	}{
		{"the last box is the union", sampleCrop, ActiveArea{Width: 600, Height: 440, X: 20, Y: 20}},
		{"no cropdetect output", "", ActiveArea{Width: 640, Height: 480}},
		{"a nearly black clip", "lavfi.cropdetect.w=16\nlavfi.cropdetect.h=480\n", ActiveArea{Width: 640, Height: 480}},
		{"a box outside the frame", "lavfi.cropdetect.w=640\nlavfi.cropdetect.h=480\nlavfi.cropdetect.x=8\n", ActiveArea{Width: 640, Height: 480}},
	}
	for _, tc := range cases {
		if got := ParseActiveArea(tc.metadata, 640, 480); got != tc.want {
			t.Errorf("%s: ParseActiveArea = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func clean() Readings {
	return Readings{Version: MeasurementVersion, Width: 640, Height: 480, Frames: 120,
		BlockMean: 1.1, BlurMean: 5.7, OutlierShare: 0.0001, ActiveWidth: 640, ActiveHeight: 480,
		UpscaleSSIM: 0.969, UpscaleSSIM4: 0.946, HasAudio: true, FlatFactor: 0}
}

func findings(a Assessment) map[Check]Outcome {
	out := map[Check]Outcome{}
	for _, f := range a.Findings {
		out[f.Check] = f.Outcome
	}
	return out
}

func TestAssess_TheBarIsSetByTheEra(t *testing.T) {
	soft := clean()
	soft.BlurMean = 8.5 // tape-era softness
	slightlySoft := clean()
	slightlySoft.BlurMean = 6.7
	// A 2x enlargement: nothing beyond half size, detail left at a quarter.
	doubled := clean()
	doubled.UpscaleSSIM, doubled.UpscaleSSIM4 = 0.998, 0.972
	// A 4x enlargement: nothing beyond a quarter size either.
	quadrupled := clean()
	quadrupled.UpscaleSSIM, quadrupled.UpscaleSSIM4 = 0.998, 0.995

	cases := []struct {
		name  string
		r     Readings
		era   int
		check Check
		want  Outcome
	}{
		{"soft picture is authentic tape", soft, 1985, CheckBlur, Borderline},
		{"the same softness fails an HD ad", soft, 2015, CheckBlur, Fail},
		{"a 2x enlargement is good tape", doubled, 1985, CheckUpscale, Pass},
		{"a 2x enlargement is good SD", doubled, 1999, CheckUpscale, Pass},
		{"a 2x enlargement fails an HD ad", doubled, 2015, CheckUpscale, Fail},
		{"a 4x enlargement fails even tape", quadrupled, 1985, CheckUpscale, Fail},
		{"slight softness passes tape", slightlySoft, 1985, CheckBlur, Pass},
		{"an unknown era is held to the SD bar", slightlySoft, 0, CheckBlur, Borderline},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := findings(Assess(c.r, c.era))[c.check]; got != c.want {
				t.Fatalf("%s at %d = %q, want %q", c.check, c.era, got, c.want)
			}
		})
	}
}

func TestAssess_OverallIsTheWorstFinding(t *testing.T) {
	if a := Assess(clean(), 1992); a.Outcome != Pass || a.Version != FloorVersion || a.Band != BandTape {
		t.Fatalf("clean tape-era clip = %+v, want a pass under %s", a, FloorVersion)
	}
	blocky := clean()
	blocky.BlockMean = 60
	blocky.InterlacedShare = 0.4
	a := Assess(blocky, 2003)
	if a.Outcome != Fail || findings(a)[CheckBlocks] != Fail || findings(a)[CheckInterlace] != Borderline {
		t.Fatalf("blocky, combed SD clip = %+v, want blocks fail, interlace borderline, overall fail", a)
	}
	clipped := clean()
	clipped.FlatFactor = 21
	if a := Assess(clipped, 1980); findings(a)[CheckClipping] != Fail {
		t.Fatalf("clipping is rip damage in every era: %+v", a)
	}
	clipped.HasAudio = false
	if _, judged := findings(Assess(clipped, 1980))[CheckClipping]; judged {
		t.Fatal("a clip with no sound was judged for clipping")
	}
}

func TestAssess_TooLittlePictureIsUndecidedNotPassed(t *testing.T) {
	short := clean()
	short.Frames = 3
	if a := Assess(short, 2001); a.Outcome != Undecided || findings(a)[CheckBlur] != Undecided {
		t.Fatalf("three frames = %+v, want undecided", a)
	}
}

// Letterbox and pillarbox are an aspect ratio; only a picture shrunk on both axes is windowboxed.
func TestAssess_WindowboxNeedsABorderOnBothAxes(t *testing.T) {
	letterbox := clean()
	letterbox.ActiveHeight = 360
	if got := findings(Assess(letterbox, 2005))[CheckWindowbox]; got != Pass {
		t.Fatalf("letterbox = %q, want pass", got)
	}
	boxed := clean()
	boxed.ActiveWidth, boxed.ActiveHeight = 480, 360
	if got := findings(Assess(boxed, 2005))[CheckWindowbox]; got != Fail {
		t.Fatalf("480x360 inside 640x480 = %q, want fail", got)
	}
}
