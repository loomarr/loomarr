package playout

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

var ladderSources = []string{"hevc-4k-hdr-dv", "h264-1080p-sdr-25", "hevc10-1080p", "mpeg2-480i"}

// TestBuild_SoftwareRungGolden pins each degraded rung's argv (rung 0 is the family golden,
// software__*). Regenerate with `go test ./internal/playout -run TestBuild_SoftwareRungGolden
// -update-pipeline`.
func TestBuild_SoftwareRungGolden(t *testing.T) {
	for _, rung := range SoftwareRungs[1:] {
		for _, srcName := range ladderSources {
			out := testOutput
			out.SoftwareRung = rung
			name := "software-" + rung.String() + "__" + srcName
			t.Run(name, func(t *testing.T) {
				checkGolden(t, name, buildGolden(testHosts()["software"], testSources()[srcName], out))
			})
		}
	}
}

// outputTail is the part of a software graph that fixes the output parameters: the last scale
// (to the channel geometry) onwards.
func outputTail(graph string) string {
	i := strings.LastIndex(graph, "scale=")
	if i < 0 {
		return ""
	}
	return graph[i:]
}

// TestBuild_SoftwareRungsKeepTheOutputUniform: a rung change restarts the item encoder mid-item, and
// the packager must never see a format change. Every rung ends in the same geometry, SAR, pixel
// format, cadence and colour labels, and hands the same encoder the same arguments, as rung 0.
func TestBuild_SoftwareRungsKeepTheOutputUniform(t *testing.T) {
	host := testHosts()["software"]
	for _, srcName := range ladderSources {
		src := testSources()[srcName]
		base, err := Build(host, src, testOutput)
		if err != nil {
			t.Fatalf("%s rung 0: %v", srcName, err)
		}
		wantTail := outputTail(base.VideoFilter)
		if !strings.HasPrefix(wantTail, "scale="+"w=1920:h=1080") || !strings.Contains(wantTail, "setsar=1") {
			t.Fatalf("%s: rung 0 must scale to the channel and pin SAR 1:1: %q", srcName, base.VideoFilter)
		}
		for _, rung := range SoftwareRungs[1:] {
			out := testOutput
			out.SoftwareRung = rung
			p, err := Build(host, src, out)
			if err != nil {
				t.Fatalf("%s %s: %v", srcName, rung, err)
			}
			if got := outputTail(p.VideoFilter); got != wantTail {
				t.Errorf("%s %s: output tail %q, rung 0 has %q", srcName, rung, got, wantTail)
			}
			if !slices.Equal(p.VideoEncode, base.VideoEncode) || !slices.Equal(p.AudioEncode, base.AudioEncode) {
				t.Errorf("%s %s: encoder args differ from rung 0: %q %q", srcName, rung, p.VideoEncode, p.AudioEncode)
			}
		}
	}
}

// TestBuild_SoftwareRungDecoderOptions: each rung's decoder shortcut (an input option, so it must
// come before -i) and working size, per the phase 0b measurements and Decision 3.
func TestBuild_SoftwareRungDecoderOptions(t *testing.T) {
	src := testSources()["hevc-4k-hdr-dv"]
	cases := []struct {
		rung    SoftwareRung
		decoder []string
		working string
	}{
		{RungFull, nil, "scale=w=1280:h=720:"},
		{RungLight, []string{"-skip_loop_filter:v", "all"}, "scale=w=1280:h=720:"},
		{RungNoRef, []string{"-skip_loop_filter:v", "all", "-skip_frame:v", "noref"}, "scale=w=1280:h=720:"},
		{RungKeyframes, []string{"-skip_loop_filter:v", "all", "-skip_frame:v", "nokey"}, "scale=w=852:h=480:"},
	}
	for _, c := range cases {
		out := testOutput
		out.SoftwareRung = c.rung
		p, err := Build(testHosts()["software"], src, out)
		if err != nil {
			t.Fatalf("%s: %v", c.rung, err)
		}
		args := p.ItemArgs("/in.mkv", 0, 0, 25, 0)
		input := slices.Index(args, "-i")
		for i := 0; i+1 < len(c.decoder); i += 2 {
			at := slices.Index(args, c.decoder[i])
			if at < 0 || at > input || args[at+1] != c.decoder[i+1] {
				t.Errorf("%s: want %s %s before -i: %q", c.rung, c.decoder[i], c.decoder[i+1], args)
			}
		}
		if c.decoder == nil && (slices.Contains(args, "-skip_frame:v") || slices.Contains(args, "-skip_loop_filter:v")) {
			t.Errorf("%s: rung 0 must decode every frame fully: %q", c.rung, args)
		}
		if !strings.HasPrefix(p.VideoFilter, c.working) {
			t.Errorf("%s: want working size %q first: %q", c.rung, c.working, p.VideoFilter)
		}
	}
}

// TestBuild_SoftwareNeverRefusesHDRForSpeed (#1517): a CPU that is too slow steps down the ladder;
// it never refuses. Only a build with no tone-mapper at all refuses HDR.
func TestBuild_SoftwareNeverRefusesHDRForSpeed(t *testing.T) {
	src := testSources()["hevc-4k-hdr-dv"]
	for _, rung := range SoftwareRungs {
		out := testOutput
		out.SoftwareRung = rung
		if _, err := Build(HostProfile{Family: FamilySoftware, CPUTonemap: true}, src, out); err != nil {
			t.Errorf("%s: a software host must not refuse HDR for speed: %v", rung, err)
		}
		if _, err := Build(HostProfile{Family: FamilySoftware}, src, out); !errors.Is(err, ErrRefused) {
			t.Errorf("%s: a build without a tone-mapper must refuse HDR, got %v", rung, err)
		}
	}
}

// TestBuild_RungIsSoftwareOnly: the ladder is the CPU's; a GPU family's argv ignores it.
func TestBuild_RungIsSoftwareOnly(t *testing.T) {
	for hostName, host := range testHosts() {
		if host.Family == FamilySoftware {
			continue
		}
		for srcName, src := range testSources() {
			want := buildGolden(host, src, testOutput)
			out := testOutput
			out.SoftwareRung = RungKeyframes
			if got := buildGolden(host, src, out); got != want {
				t.Errorf("%s/%s: a rung changed a GPU family's argv:\n%s\nvs\n%s", hostName, srcName, got, want)
			}
		}
	}
}

// TestStartRung: the rung a new item starts on, from the measured software cost of its rung 0.
func TestStartRung(t *testing.T) {
	hdr, sdr := testSources()["hevc-4k-hdr-dv"], testSources()["h264-1080p-sdr-25"]
	cases := []struct {
		name string
		src  MediaFormat
		cost RungCost
		want SoftwareRung
	}{
		// Unmeasured: SDR up to 1080p starts at full quality; 4K or HDR, which phase 0b measured
		// at 2.3–3.7 cores, starts on the rung that always fits and steps up with headroom.
		{"unmeasured sdr", sdr, RungCost{}, RungFull},
		{"unmeasured hdr", hdr, RungCost{}, RungKeyframes},
		{"fast", hdr, RungCost{Speed: 1.6, CPUCores: 2.5}, RungFull},
		// H1 alone at 1.06x: rung 1 saves ~5%, not enough for 1.2x; noref is never a start-time promise.
		{"phase 0b H1 at 720p", hdr, RungCost{Speed: 1.06, CPUCores: 3.66}, RungKeyframes},
		{"just short", sdr, RungCost{Speed: 1.16, CPUCores: 3}, RungLight},
		{"hopeless", hdr, RungCost{Speed: 0.1, CPUCores: 4}, RungKeyframes},
	}
	for _, c := range cases {
		if got := StartRung(c.src, c.cost); got != c.want {
			t.Errorf("%s: StartRung = %s, want %s", c.name, got, c.want)
		}
	}
}

// TestBuild_SoftwareRungsDownscaleOnlyHeavySources (supervisor decision, #1517): rungs 1–2 take a
// 720-line working size only for HDR or above-1080p sources. SDR up to 1080p decodes at its own
// size with the decoder shortcuts; a 1080→720→1080 round trip measured slower than rung 0.
func TestBuild_SoftwareRungsDownscaleOnlyHeavySources(t *testing.T) {
	uhdSDR := MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, FrameRate: 24, PixelFormat: "yuv420p10le",
		AudioCodec: "eac3", AudioChannels: 6, AudioSampleRate: 48000, Container: "matroska,webm"}
	cases := []struct {
		name    string
		src     MediaFormat
		rung    SoftwareRung
		working string
	}{
		{"1080p SDR rung 1", testSources()["h264-1080p-sdr-25"], RungLight, "scale=w=1920:h=1080:"},
		{"1080p SDR rung 2", testSources()["hevc10-1080p"], RungNoRef, "tpad=stop_mode=clone:stop_duration=10,scale=w=1920:h=1080:"},
		{"1080p SDR rung 3", testSources()["hevc10-1080p"], RungKeyframes, "scale=w=852:h=480:"},
		{"4K SDR rung 1", uhdSDR, RungLight, "scale=w=1280:h=720:"},
		{"4K HDR rung 2", testSources()["hevc-4k-hdr-dv"], RungNoRef, "scale=w=1280:h=720:"},
	}
	for _, c := range cases {
		out := testOutput
		out.SoftwareRung = c.rung
		p, err := Build(testHosts()["software"], c.src, out)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.HasPrefix(p.VideoFilter, c.working) {
			t.Errorf("%s: want the graph to start %q: %q", c.name, c.working, p.VideoFilter)
		}
	}
}
