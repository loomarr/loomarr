//go:build ffmpeg

package playout

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

// THE LADDER ON REAL TITLES (#1517). Set PLAYOUT_TEST_LADDER_SOURCES to comma-separated local
// source files (stream-copy excerpts of at least 2 minutes, so the network is not what is measured)
// and run inside a CPU-limited container with no GPU devices. For each source it reports:
//
//   - speed and CPU per stream at 1x for every rung, 30 s of content unpaced;
//   - a paced run where the real RungMonitor drives real encoder restarts on the real -progress
//     stream (as the phase 2 packager will): its decisions, and at every rung change the segment's
//     video and audio length, so a gap or overlap in the audio shows up as a sample count.
//
// PLAYOUT_TEST_LADDER_CPUS is the cores the container allows (default 4).
func TestLive_SoftwareLadderRealTitles(t *testing.T) {
	list := os.Getenv("PLAYOUT_TEST_LADDER_SOURCES")
	if list == "" {
		t.Skip("set PLAYOUT_TEST_LADDER_SOURCES to local real source files to run this")
	}
	bin := ffmpegBin(t)
	cpus := 4.0
	if s := os.Getenv("PLAYOUT_TEST_LADDER_CPUS"); s != "" {
		cpus, _ = strconv.ParseFloat(s, 64)
	}
	probe := FFprobeFormatNextTo(bin)
	out := ChannelOutput(Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: EncoderSoftware, AudioBitrate: 160})
	host := HostProfile{Family: FamilySoftware, CPUTonemap: TonemapperFor(bin)()}
	for _, path := range strings.Split(list, ",") {
		facts, err := probe(context.Background(), path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		name := filepath.Base(path)
		for _, rung := range SoftwareRungs {
			o := out
			o.SoftwareRung = rung
			pipe, err := Build(host, facts, o)
			if err != nil {
				t.Fatal(err)
			}
			args := replaceOutput(pipe.ItemArgs(path, 20*time.Second, 30*25, 25, 0), os.DevNull)
			cmd := exec.Command(bin, args...)
			start := time.Now()
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s %s: %v %s", name, rung, err, b)
			}
			wall := time.Since(start).Seconds()
			cpu := (cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()).Seconds()
			t.Logf("RUNG %s %s: speed %.2fx, %.2f cores at 1x (%.2f cores busy)", name, rung, 30/wall, cpu/30, cpu/wall)
		}
		for _, start := range []SoftwareRung{RungFull, RungKeyframes} {
			driveLadder(t, bin, path, name, facts, host, out, start, cpus, 100*time.Second)
		}
	}
}

// driveLadder airs the source paced at 1x from 5 s in, starting on start, for dur of wall time. The
// monitor's step stops the encoder (SIGINT, so the segment is finalized) and a new one resumes at
// the next frame the previous segment did not produce, on the new rung.
func driveLadder(t *testing.T, bin, path, name string, facts MediaFormat, host HostProfile, out OutputProfile,
	start SoftwareRung, cpus float64, dur time.Duration) {
	t.Helper()
	m := NewRungMonitor(start, RungMonitorConfig{CPUAllowance: cpus})
	pos, rung := 5*time.Second, start
	deadline := time.Now().Add(dur)
	var totalV, totalA float64
	for seg := 0; time.Now().Before(deadline); seg++ {
		o := out
		o.SoftwareRung = rung
		pipe, err := Build(host, facts, o)
		if err != nil {
			t.Fatal(err)
		}
		dst := t.TempDir() + "/seg.ts"
		args := append([]string{"-readrate", "1.0", "-progress", "pipe:3"}, replaceOutput(pipe.ItemArgs(path, pos, 0, 25, 0), dst)...)
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, args...)
		cmd.ExtraFiles = []*os.File{pw}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		_ = pw.Close()
		var once sync.Once
		stop := func() { once.Do(func() { _ = cmd.Process.Signal(os.Interrupt) }) }
		timer := time.AfterFunc(time.Until(deadline), stop)
		next, why := rung, "deadline"
		ReadProgress(pr, func(p Progress) {
			d := m.Observe(SpeedSample{At: time.Now(), OutTime: time.Duration(p.OutTimeMS) * time.Millisecond, CPU: procCPU(cmd.Process.Pid)})
			if d.Step {
				next, why = d.Rung, d.Reason
				stop()
			}
		})
		_ = cmd.Wait()
		timer.Stop()
		v, a := segmentLengths(t, bin, dst)
		totalV += v
		totalA += a
		t.Logf("LADDER %s start=%s seg %d at %s on %s: video %.3f s, audio %.3f s (audio-video %+.0f ms); then %s (%s)",
			name, start, seg, pos, rung, v, a, (a-v)*1000, next, why)
		pos += time.Duration(v * float64(time.Second))
		rung = next
	}
	t.Logf("LADDER %s start=%s total: video %.3f s, audio %.3f s, drift %+.0f ms", name, start, totalV, totalA, (totalA-totalV)*1000)
}

// procCPU is a process's CPU time (user+sys, all threads) from /proc; 0 when unreadable.
func procCPU(pid int) time.Duration {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	s := string(b)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	if len(fields) < 13 {
		return 0
	}
	u, _ := strconv.ParseInt(fields[11], 10, 64)
	k, _ := strconv.ParseInt(fields[12], 10, 64)
	return time.Duration(u+k) * time.Second / 100 // USER_HZ
}

// segmentLengths is a segment's video length (frames at 25 fps) and decoded audio length (samples
// at 48 kHz).
func segmentLengths(t *testing.T, bin, path string) (float64, float64) {
	t.Helper()
	b, err := exec.Command(ffprobeFor(bin), "-v", "error", "-count_frames", "-select_streams", "v:0",
		"-show_entries", "stream=nb_read_frames", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("probe %s: %v", path, err)
	}
	frames, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	pcm, err := exec.Command(bin, "-v", "error", "-i", path, "-map", "0:a:0", "-f", "s16le", "-ac", "1", "-ar", "48000", "-").Output()
	if err != nil {
		t.Fatalf("decode audio %s: %v", path, err)
	}
	return float64(frames) / 25, float64(len(pcm)/2) / 48000
}
