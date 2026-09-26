//go:build ffmpeg

package playout

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// EVERY SOFTWARE RUNG IS A PICTURE IN ONE FORMAT (#1517). A rung change restarts the item encoder
// mid-item, so each rung must (a) produce a real picture, not black, and (b) produce exactly the
// output parameters of rung 0: geometry, SAR, pixel format, H.264 profile and level, cadence, and
// the same audio. Any difference is a format change the packager would have to splice.
//
// The sources are the two shapes that break uniformity: a real PQ HDR10 signal (the tone-map runs
// at a different working size per rung) and a 2.40:1 SDR film frame (a two-stage scale rounds its
// SAR, which x264 writes into the SPS).
func TestLive_SoftwareRungsProduceAPictureInOneFormat(t *testing.T) {
	bin := ffmpegBin(t)
	if !TonemapperFor(bin)() {
		t.Skip("this ffmpeg build has no zscale/tonemap")
	}
	sources := []struct {
		name  string
		path  string
		facts MediaFormat
		hdr   bool
	}{
		{"pq-hdr10", makePQSource(t, bin), MediaFormat{VideoCodec: "hevc", Width: 1280, Height: 720, FrameRate: 25,
			PixelFormat: "yuv420p10le", ColorTransfer: "smpte2084", Container: "mpegts"}, true},
		{"scope-sdr", makeScopeSource(t, bin), MediaFormat{VideoCodec: "h264", Width: 1920, Height: 800, FrameRate: 24,
			PixelFormat: "yuv420p", Container: "mpegts"}, false},
	}
	out := OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 22, TargetKbps: 8000, MaxKbps: 12000,
		GOPSeconds: 1, AudioKbps: 160}
	host := HostProfile{Family: FamilySoftware, CPUTonemap: true}
	for _, src := range sources {
		var want outputFacts
		for _, rung := range SoftwareRungs {
			t.Run(src.name+"/"+rung.String(), func(t *testing.T) {
				o := out
				o.SoftwareRung = rung
				pipe, err := Build(host, src.facts, o)
				if err != nil {
					t.Fatal(err)
				}
				dst := t.TempDir() + "/o.ts"
				if errText := encodeTo(t, bin, replaceOutput(pipe.ItemArgs(src.path, 0, 40, 25, 0), dst)); errText != "" {
					t.Fatalf("%s did not encode: %s", rung, errText)
				}
				if src.hdr {
					assertPicture(t, bin, dst)
				} else if avg, ymax := lumaStats(t, bin, dst); ymax <= 16 {
					t.Errorf("black picture (YAVG %.1f, YMAX %d)", avg, ymax)
				}
				got := probeOutputFacts(t, bin, dst)
				t.Logf("%s: %+v", rung, got)
				if rung == RungFull {
					want = got
					if got.video != "1920x1080 sar=1:1 yuv420p High 25/1" {
						t.Errorf("rung 0 is not the channel format: %q", got.video)
					}
					return
				}
				if got.video != want.video || got.level != want.level {
					t.Errorf("output format changed from rung 0: %q level %d, want %q level %d",
						got.video, got.level, want.video, want.level)
				}
				if got.audio != want.audio {
					t.Errorf("audio changed from rung 0: %q, want %q", got.audio, want.audio)
				}
				// Frame counts: a skipped-frame rung must still fill the item (repeated frames), and no
				// rung may shorten the audio.
				if d := got.videoFrames - want.videoFrames; d < -1 || d > 1 {
					t.Errorf("video ends early: %d frames, rung 0 has %d", got.videoFrames, want.videoFrames)
				}
				if got.audioFrames != want.audioFrames || got.audioFrames == 0 {
					t.Errorf("audio degraded: %d AAC frames, rung 0 has %d", got.audioFrames, want.audioFrames)
				}
			})
		}
	}
}

// makeScopeSource is a 2.40:1 SDR film frame (1920x800, 24 fps, 1 s GOP) with a tone, 4 s long.
func makeScopeSource(t *testing.T, bin string) string {
	t.Helper()
	out := t.TempDir() + "/scope.ts"
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1920x800:rate=24:duration=4",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=4",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "24", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ac", "2", "-f", "mpegts", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cannot synthesize the scope source: %v\n%s", err, b)
	}
	return out
}

type outputFacts struct {
	video       string // WxH, SAR, pixel format, profile, frame rate
	level       int
	audio       string // codec, profile, rate, channels
	videoFrames int
	audioFrames int // AAC frames of 1024 samples
}

func probeOutputFacts(t *testing.T, bin, path string) outputFacts {
	t.Helper()
	// key=value, because ffprobe prints entries in its own order, not the requested one.
	probe := func(stream, entries string) map[string]string {
		b, err := exec.Command(ffprobeFor(bin), "-v", "error", "-count_frames", "-select_streams", stream, "-show_entries",
			"stream="+entries+",nb_read_frames", "-of", "default=nw=1", path).Output()
		if err != nil {
			t.Fatalf("ffprobe %s: %v", stream, err)
		}
		m := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if k, v, ok := strings.Cut(line, "="); ok {
				m[k] = v
			}
		}
		return m
	}
	v := probe("v:0", "width,height,sample_aspect_ratio,pix_fmt,profile,r_frame_rate,level")
	a := probe("a:0", "codec_name,profile,sample_rate,channels")
	f := outputFacts{
		video: v["width"] + "x" + v["height"] + " sar=" + v["sample_aspect_ratio"] + " " + v["pix_fmt"] + " " +
			v["profile"] + " " + v["r_frame_rate"],
		audio: a["codec_name"] + " " + a["profile"] + " " + a["sample_rate"] + " " + a["channels"],
	}
	f.level, _ = strconv.Atoi(v["level"])
	f.videoFrames, _ = strconv.Atoi(v["nb_read_frames"])
	f.audioFrames, _ = strconv.Atoi(a["nb_read_frames"])
	return f
}
