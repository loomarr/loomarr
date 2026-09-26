package playout

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// The class probe measures what one stream of each cost class costs this host (#1512 G5). It never
// waits for the inventory (G7): at boot it generates small deterministic clips per class with lavfi
// and runs the live pipeline builder's real graphs on them. The HDR clip exercises the tone-map
// stage, so the same run is the startup tone-map self-check (G11).

const (
	// probeClipRecipe names the clip recipe; changing a recipe must change it so old clips regenerate.
	probeClipRecipe  = "v1"
	probeClipSeconds = 2
	// probeWindow is the wall time one (class, rung) measurement encodes for. Clips loop, so a fast
	// class covers more media in the same window.
	probeWindow = 2500 * time.Millisecond
	// probeStatsPeriod is how often ffmpeg reports progress during a measurement.
	probeStatsPeriod = "0.25"
	// SessionProbeBound caps the encoder-session probe (a GeForce opens 12 NVENC sessions at most).
	SessionProbeBound   = 16
	sessionProbeTimeout = 8 * time.Second
	// sessionProbeBatch sessions open in parallel per step: sequential opens (~0.6 s each for a CUDA
	// context) cannot reach the cap in time, and one burst past the cap collapses (see below).
	sessionProbeBatch = 4
	// Before the ramp, sessions the class measurements just closed must be gone: the driver lists
	// them for a moment after ffmpeg exits, and the unsettled ramp read 11 or 12 on the dev GeForce.
	sessionSettlePoll = 200 * time.Millisecond
	sessionSettleMax  = 3 * time.Second
	// probeNice is the probe encoders' scheduling priority: the lowest, below every viewer.
	probeNice = 19
)

// Foreground is live playout as the probe's background work sees it (ResourceBudget): a context
// that ends the moment a live transcode is admitted, and a wait for none to be running.
type Foreground interface {
	BackgroundContext(ctx context.Context) (context.Context, context.CancelFunc)
	WaitIdle(ctx context.Context) error
}

// yielding runs fn as background work: only while no live transcode runs, killed (through its
// context) the moment one is admitted and rerun from scratch once playback is idle again, so a
// half-measured result is never kept. A nil fg runs fn once.
func yielding[T any](ctx context.Context, fg Foreground, fn func(context.Context) (T, error)) (T, error) {
	if fg == nil {
		return fn(ctx)
	}
	for {
		if err := fg.WaitIdle(ctx); err != nil {
			var zero T
			return zero, err
		}
		bctx, cancel := fg.BackgroundContext(ctx)
		v, err := fn(bctx)
		yielded := errors.Is(context.Cause(bctx), ErrYielded)
		cancel()
		if !yielded || ctx.Err() != nil {
			return v, err
		}
	}
}

// probeClip is one synthetic source. Temporal noise gives the clip a realistic bitrate (the 4K HDR
// clip is ~30 Mbit/s): a clean test pattern compresses to almost nothing and under-costs decode.
type probeClip struct {
	class  StreamClass
	format MediaFormat
	filter string
	encode []string
}

const probeNoise = "noise=alls=8:allf=t"

// probeAudio is 5.1 AC-3, so each measurement includes the downmix and AAC encode real items pay.
var probeAudio = []string{"-c:a", "ac3", "-ac", "6", "-ar", "48000"}

var probeClips = []probeClip{
	{
		class: ClassSDR,
		format: MediaFormat{VideoCodec: "h264", Width: 1920, Height: 1080, FrameRate: 24000.0 / 1001,
			PixelFormat: "yuv420p"},
		filter: probeNoise + ",format=yuv420p",
		encode: []string{"-c:v", "libx264", "-preset", "ultrafast", "-b:v", "10M", "-maxrate", "12M", "-bufsize", "12M"},
	},
	{
		class: ClassHEVC10,
		format: MediaFormat{VideoCodec: "hevc", Width: 1920, Height: 1080, FrameRate: 24000.0 / 1001,
			PixelFormat: "yuv420p10le"},
		filter: probeNoise + ",format=yuv420p10le",
		encode: []string{"-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:bitrate=8000"},
	},
	{
		// HDR10: PQ transfer, BT.2020 primaries and matrix, in the stream and the container.
		class: ClassHDR4K,
		format: MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 2160, FrameRate: 24000.0 / 1001,
			PixelFormat: "yuv420p10le", ColorTransfer: "smpte2084"},
		filter: probeNoise + ",zscale=tin=bt709:min=bt709:pin=bt709:rin=tv:t=smpte2084:m=bt2020nc:p=bt2020:r=tv:npl=100,format=yuv420p10le",
		encode: []string{"-c:v", "libx265", "-preset", "ultrafast", "-x265-params",
			"log-level=error:bitrate=30000:vbv-maxrate=40000:vbv-bufsize=40000:hdr10=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:max-cll=1000,400",
			"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc"},
	},
}

func (c probeClip) source() MediaFormat {
	f := c.format
	f.AudioCodec, f.AudioChannels, f.AudioSampleRate = "ac3", 6, 48000
	f.Container, f.Duration = "matroska,webm", probeClipSeconds
	return f
}

func (c probeClip) fileName() string {
	return "probe-" + probeClipRecipe + "-" + string(c.class) + ".mkv"
}

func (c probeClip) generateArgs(out string) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=24000/1001", c.format.Width, c.format.Height),
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", strconv.Itoa(probeClipSeconds), "-vf", c.filter}
	args = append(args, c.encode...)
	args = append(args, probeAudio...)
	return append(args, "-f", "matroska", out)
}

// ClassProbeConfig is one class probe run.
type ClassProbeConfig struct {
	FFmpeg string
	// ClipDir holds the generated clips across restarts (playout.state_dir).
	ClipDir string
	Encoder Encoder
	// CPUTonemap and GPU are the build's tone-mappers, as the live chain passes them to HostFor.
	CPUTonemap bool
	GPU        GPUFilters
	// Outputs are the ladder's rung profiles, best first; each distinct height is measured.
	Outputs []Profile
	// Classes limits the run (nil = every transcode class). A restart that reuses a stored table
	// re-runs only the HDR class: the tone-map self-check runs at every start.
	Classes []StreamClass
	Manager *diagnostics.ProcessManager
	// Foreground makes the probe yield to live playout (the ResourceBudget); nil never yields.
	Foreground Foreground
	// Curve is the tone curve the live chain maps HDR with; the HDR cells are keyed by it. Empty =
	// Hable.
	Curve ToneCurve
}

func (cfg ClassProbeConfig) curve() ToneCurve {
	if cfg.Curve == "" {
		return DefaultToneCurve
	}
	return cfg.Curve
}

// TonemapCheck is the startup tone-map self-check (G11).
type TonemapCheck struct {
	// Ran is false when no HDR measurement was attempted.
	Ran bool `json:"ran"`
	OK  bool `json:"ok"`
	// Stage is the tone-mapper that worked (Pipeline.Tonemapper): opencl, libplacebo or cpu.
	Stage string `json:"stage,omitempty"`
	// Detail is the failure, or why a working tone-map still cannot run in real time.
	Detail string `json:"detail,omitempty"`
	// MinYMax and AvgY are the tone-mapped picture's darkest peak and mean luma over the sampled frames
	// (signalstats, 16–235 scale): the evidence it is not black (#1516).
	MinYMax int     `json:"minYMax,omitempty"`
	AvgY    float64 `json:"avgY,omitempty"`
}

// ClassProbeResult is what one run measured. A class missing from Costs was not measurable here.
type ClassProbeResult struct {
	Costs   map[CostKey]ClassCost
	Tonemap TonemapCheck
	// Failures names each (class, height) that could not be measured, with ffmpeg's reason.
	Failures []string
}

// ProbeClassCosts generates any missing clip and measures each class at each rung, alone and in
// sequence. It is bounded: probeWindow per measurement plus clip generation on first boot.
func ProbeClassCosts(ctx context.Context, cfg ClassProbeConfig) ClassProbeResult {
	res := ClassProbeResult{Costs: map[CostKey]ClassCost{}}
	for _, clip := range probeClips {
		if !wantClass(cfg.Classes, clip.class) {
			continue
		}
		hdr := clip.source().HDR()
		path, err := ensureProbeClip(ctx, cfg, clip)
		if err != nil {
			res.Failures = append(res.Failures, string(clip.class)+": "+err.Error())
			if hdr {
				res.Tonemap = TonemapCheck{Ran: true, Detail: "could not generate the HDR test clip: " + err.Error()}
			}
			continue
		}
		gpu := cfg.GPU
		measured := map[int]bool{}
		for _, out := range cfg.Outputs {
			if measured[out.Height] || ctx.Err() != nil {
				continue
			}
			measured[out.Height] = true
			cost, pipe, err := measureWithDemotion(ctx, cfg, clip, path, out, &gpu)
			if hdr && !res.Tonemap.Ran && ctx.Err() == nil {
				stage := pipe.Tonemapper
				if err == nil {
					// A clean, fast exit is not proof (#1516): the picture must not be black. A dark
					// result is not demoted like an error: the live ladder would still air it.
					var luma pictureLuma
					luma, err = checkTonemapPicture(ctx, cfg, path, pipe)
					res.Tonemap.MinYMax, res.Tonemap.AvgY = luma.minYMax, luma.avgY
				}
				res.Tonemap.Ran, res.Tonemap.OK, res.Tonemap.Stage = true, err == nil, stage
				if err != nil {
					res.Tonemap.Detail = err.Error()
				} else if cost.Speed < minStreamSpeed {
					res.Tonemap.Detail = fmt.Sprintf("tone-mapping works but runs at %.2fx, below the %.1fx a live channel needs", cost.Speed, minStreamSpeed)
				}
			}
			if err != nil {
				res.Failures = append(res.Failures, fmt.Sprintf("%s at %dp: %v", clip.class, out.Height, err))
				continue
			}
			res.Costs[HDRKey(clip.class, out.Height, cfg.curve())] = cost
		}
	}
	return res
}

func wantClass(classes []StreamClass, c StreamClass) bool {
	if classes == nil {
		return true
	}
	for _, want := range classes {
		if want == c {
			return true
		}
	}
	return false
}

// measureWithDemotion measures one cell, walking the live ladder's tone-map order on failure
// (GPUFilters.demote) so the self-check reports what a real HDR programme would get.
func measureWithDemotion(
	ctx context.Context, cfg ClassProbeConfig, clip probeClip, path string, out Profile, gpu *GPUFilters,
) (ClassCost, Pipeline, error) {
	src := clip.source()
	for {
		host := HostFor(cfg.Encoder, cfg.CPUTonemap, *gpu)
		// The live curve's own graph: the HDR cell it measures is keyed by that curve.
		output := ChannelOutput(out)
		output.ToneCurve = cfg.curve()
		pipe, err := Build(host, src, output)
		if err != nil {
			return ClassCost{}, pipe, err
		}
		// Yielding sits inside the demotion walk: an interrupted measurement is rerun, never taken
		// for a tone-mapper that failed.
		cost, err := yielding(ctx, cfg.Foreground, func(ctx context.Context) (ClassCost, error) {
			return measureCost(ctx, cfg, clip.class, path, pipe, out.Height)
		})
		if err == nil || ctx.Err() != nil || !src.HDR() || !demoteTonemapper(gpu, pipe.Tonemapper) {
			return cost, pipe, err
		}
	}
}

// The tone-map picture check (#1516): tonemap_vaapi on an Intel Arc exits cleanly at normal speed
// while every frame is black (signalstats YMAX=16). The probe's HDR clip is a bright test pattern, so
// a working tone-map gives every sampled frame real highlights and a mid-range average.
const (
	tonemapCheckFrames = 12
	minTonemapYMAX     = 64
	minTonemapYAVG     = 24
	maxTonemapYAVG     = 220
)

// checkTonemapPicture encodes a few frames of the HDR clip through pipe, exactly as a live channel
// would, then decodes them and reads their luma: nil when every frame shows a picture.
func checkTonemapPicture(ctx context.Context, cfg ClassProbeConfig, path string, pipe Pipeline) (pictureLuma, error) {
	sample := filepath.Join(cfg.ClipDir, "tonemap-check.ts")
	defer func() { _ = os.Remove(sample) }()
	encode := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}
	encode = append(encode, pipe.PreInput...)
	encode = append(encode, "-i", path, "-map", "0:v:0", "-vf", pipe.VideoFilter)
	encode = append(encode, pipe.VideoEncode...)
	encode = append(encode, "-an", "-frames:v", strconv.Itoa(tonemapCheckFrames), "-f", "mpegts", sample)
	if _, err := yielding(ctx, cfg.Foreground, func(ctx context.Context) (string, error) {
		return runProbeCommand(ctx, cfg, "tonemap_check", "encode", encode)
	}); err != nil {
		return pictureLuma{}, fmt.Errorf("could not encode the tone-map check: %w", err)
	}
	stats := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-i", sample,
		"-vf", "signalstats,metadata=print:file=-", "-f", "null", "-"}
	out, err := yielding(ctx, cfg.Foreground, func(ctx context.Context) (string, error) {
		return runProbeCommand(ctx, cfg, "tonemap_check", "signalstats", stats)
	})
	if err != nil {
		return pictureLuma{}, fmt.Errorf("could not read the tone-mapped picture: %w", err)
	}
	return judgeTonemapPicture(out)
}

// pictureLuma is what the picture check saw: the darkest frame's peak and the mean luma.
type pictureLuma struct {
	minYMax int
	avgY    float64
}

// judgeTonemapPicture reads signalstats' per-frame metadata and fails on a black, washed-out or
// missing picture, naming the numbers it saw.
func judgeTonemapPicture(metadata string) (pictureLuma, error) {
	frames, minMax := 0, -1
	sumAvg := 0.0
	for _, line := range strings.Split(metadata, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		switch key {
		case "lavfi.signalstats.YMAX":
			if frames++; minMax < 0 || int(v) < minMax {
				minMax = int(v)
			}
		case "lavfi.signalstats.YAVG":
			sumAvg += v
		}
	}
	if frames == 0 {
		return pictureLuma{}, errors.New("the tone-mapped output has no frames")
	}
	l := pictureLuma{minYMax: minMax, avgY: round2(sumAvg / float64(frames))}
	switch {
	case l.minYMax <= minTonemapYMAX:
		return l, fmt.Errorf("the tone-mapped picture is black (brightest pixel %d, average %.0f on a 16–235 scale)", l.minYMax, l.avgY)
	case l.avgY < minTonemapYAVG || l.avgY > maxTonemapYAVG:
		return l, fmt.Errorf("the tone-mapped picture is unwatchable (average brightness %.0f on a 16–235 scale)", l.avgY)
	}
	return l, nil
}

// runProbeCommand runs one short probe ffmpeg at background priority and returns its stdout.
func runProbeCommand(ctx context.Context, cfg ClassProbeConfig, purpose, target string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, cfg.FFmpeg, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &limitedWriter{w: &stdout, n: 1 << 20}
	cmd.Stderr = &limitedWriter{w: &stderr, n: 16 << 10}
	run := cfg.Manager.Begin(diagnostics.ProcessSpec{Purpose: purpose, Target: target, Executable: cfg.FFmpeg, Args: args})
	err := startLowPriority(cmd)
	if err == nil {
		err = cmd.Wait()
	}
	if run != nil {
		run.Finish(diagnostics.ProcessResult{Err: err, Cancelled: ctx.Err() != nil})
	}
	if err != nil {
		if line := firstLine(strings.TrimSpace(stderr.String())); line != "" {
			return "", errors.New(line)
		}
		return "", err
	}
	return stdout.String(), nil
}

// ensureProbeClip returns the clip's path, generating it (atomically) when missing.
func ensureProbeClip(ctx context.Context, cfg ClassProbeConfig, clip probeClip) (string, error) {
	if strings.TrimSpace(cfg.ClipDir) == "" {
		return "", errors.New("no playout state directory")
	}
	path := filepath.Join(cfg.ClipDir, clip.fileName())
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return path, nil
	}
	if err := os.MkdirAll(cfg.ClipDir, 0o750); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	// Generation is the first boot's heaviest CPU work (a 4K HEVC encode); it yields to a viewer
	// and restarts from nothing, like a measurement.
	_, err := yielding(ctx, cfg.Foreground, func(ctx context.Context) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		return runProbeCommand(ctx, cfg, "capacity_probe_clip", string(clip.class), clip.generateArgs(tmp))
	})
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// probeSample is one progress report: media produced by a wall-clock instant, and the process CPU
// time then when the platform exposes it.
type probeSample struct {
	wall time.Duration
	out  time.Duration
	cpu  time.Duration
	hasC bool
}

// measureCost encodes the looping clip through pipe for probeWindow. Speed and CPU are steady-state:
// taken between the first and last progress samples, so device and decoder start-up (a CUDA context
// alone costs a few hundred ms of CPU) do not inflate a fast class's per-stream cost.
func measureCost(ctx context.Context, cfg ClassProbeConfig, class StreamClass, path string, pipe Pipeline, height int) (ClassCost, error) {
	ctx, cancel := context.WithTimeout(ctx, probeWindow)
	defer cancel()
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-nostats",
		"-progress", "pipe:1", "-stats_period", probeStatsPeriod}
	args = append(args, pipe.PreInput...)
	args = append(args, "-stream_loop", "-1", "-i", path, "-map", "0:v:0", "-map", "0:a:0", "-vf", pipe.VideoFilter)
	args = append(args, pipe.VideoEncode...)
	args = append(args, pipe.AudioEncode...)
	args = append(args, "-f", "mpegts", os.DevNull)

	cmd := exec.CommandContext(ctx, cfg.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ClassCost{}, err
	}
	var stderr strings.Builder
	cmd.Stderr = &limitedWriter{w: &stderr, n: 16 << 10}
	run := cfg.Manager.Begin(diagnostics.ProcessSpec{
		Purpose: "capacity_probe", Target: fmt.Sprintf("%s@%dp", class, height), Executable: cfg.FFmpeg, Args: args,
	})
	start := time.Now()
	if err := startLowPriority(cmd); err != nil {
		if run != nil {
			run.Finish(diagnostics.ProcessResult{Err: err})
		}
		return ClassCost{}, err
	}
	samples := readProbeSamples(stdout, start, cmd.Process.Pid)
	waitErr := cmd.Wait()
	windowEnded := errors.Is(ctx.Err(), context.DeadlineExceeded)
	if run != nil {
		run.Finish(diagnostics.ProcessResult{Err: waitErr, Cancelled: windowEnded, TerminationReason: "measurement window ended"})
	}
	cost, ok := costFromSamples(samples, cmd.ProcessState)
	if !ok {
		msg := firstLine(strings.TrimSpace(stderr.String()))
		if msg == "" && waitErr != nil {
			msg = waitErr.Error()
		}
		if msg == "" {
			msg = "produced no media"
		}
		return ClassCost{}, errors.New(msg)
	}
	return cost, nil
}

func readProbeSamples(r io.Reader, start time.Time, pid int) []probeSample {
	var samples []probeSample
	var out time.Duration
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "out_time_us":
			if us, err := strconv.ParseInt(value, 10, 64); err == nil && us > 0 {
				out = time.Duration(us) * time.Microsecond
			}
		case "progress":
			if out <= 0 {
				continue
			}
			s := probeSample{wall: time.Since(start), out: out}
			s.cpu, s.hasC = processCPUTime(pid)
			samples = append(samples, s)
		}
	}
	return samples
}

// costFromSamples derives speed and CPU per stream at 1x. Without a per-sample CPU clock it falls
// back to the whole process's CPU over all media, which overstates: the safe direction.
func costFromSamples(samples []probeSample, state *os.ProcessState) (ClassCost, bool) {
	if len(samples) < 2 {
		return ClassCost{}, false
	}
	first, last := samples[0], samples[len(samples)-1]
	media := last.out - first.out
	wall := last.wall - first.wall
	if media <= 0 || wall <= 0 {
		return ClassCost{}, false
	}
	c := ClassCost{Speed: round2(media.Seconds() / wall.Seconds())}
	switch {
	case first.hasC && last.hasC && last.cpu >= first.cpu:
		c.CPUCores = (last.cpu - first.cpu).Seconds() / media.Seconds()
	case state != nil:
		c.CPUCores = (state.UserTime() + state.SystemTime()).Seconds() / last.out.Seconds()
	}
	c.CPUCores = round3(c.CPUCores)
	return c, true
}

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }
func round3(v float64) float64 { return float64(int64(v*1000+0.5)) / 1000 }

// SessionProbeConfig is one encoder-session probe.
type SessionProbeConfig struct {
	FFmpeg  string
	Encoder Encoder
	// Bound caps the ramp (SessionProbeBound).
	Bound   int
	Manager *diagnostics.ProcessManager
	// InUse counts the encoder sessions open on the device right now, every process's
	// (EncoderSessionsInUse); nil or not ok when the driver does not say.
	InUse func(context.Context) (int, bool)
	// Foreground makes the probe yield to live playout; nil never yields.
	Foreground Foreground
}

// ProbeSessionLimit returns how many encoder sessions this host holds at once (#1512 G5: the
// encoder session term): its capacity, not what happened to be free. It first waits for the in-use
// count to settle (sessions closed by the class measurements linger briefly), then ramps, and counts
// the sessions still in use, another application's included, alongside the ones it opened.
// Software hosts have no such limit: 0.
func ProbeSessionLimit(ctx context.Context, cfg SessionProbeConfig) (int, error) {
	if IsSoftwareEncoder(cfg.Encoder) || cfg.Bound <= 0 {
		return 0, nil
	}
	return yielding(ctx, cfg.Foreground, func(ctx context.Context) (int, error) {
		inUse := settleSessions(ctx, cfg.InUse)
		opened, err := rampSessions(ctx, cfg.FFmpeg, cfg.Encoder, max(cfg.Bound-inUse, 1), cfg.Manager)
		if opened == 0 {
			return 0, err
		}
		return opened + inUse, nil
	})
}

// settleSessions polls the in-use count until two readings agree (or sessionSettleMax passes) and
// returns it; 0 when the driver does not say.
func settleSessions(ctx context.Context, inUse func(context.Context) (int, bool)) int {
	if inUse == nil {
		return 0
	}
	prev, ok := inUse(ctx)
	if !ok {
		return 0
	}
	for deadline := time.Now().Add(sessionSettleMax); time.Now().Before(deadline); {
		select {
		case <-ctx.Done():
			return prev
		case <-time.After(sessionSettlePoll):
		}
		n, ok := inUse(ctx)
		if !ok || n == prev {
			return prev
		}
		prev = n
	}
	return prev
}

// EncoderSessionsInUse is the in-use session counter for enc's family, or nil when its driver has
// none: NVIDIA's reports every process's NVENC sessions, which is what a GeForce's cap counts.
func EncoderSessionsInUse(enc Encoder) func(context.Context) (int, bool) {
	if engineOf(enc) != EncoderNVENC {
		return nil
	}
	return func(ctx context.Context) (int, bool) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=encoder.stats.sessionCount",
			"--format=csv,noheader,nounits").Output()
		if err != nil {
			return 0, false
		}
		total, seen := 0, false
		for _, line := range strings.Fields(string(out)) {
			n, err := strconv.Atoi(line)
			if err != nil {
				return 0, false
			}
			total, seen = total+n, true
		}
		return total, seen
	}
}

// rampSessions opens sessions up to bound and reports how many held at once. Sessions open in
// batches on tiny real-time inputs and each stays open, so the count is of overlapping sessions; the
// first refusal ends the ramp. Opening them all at once undercounts: on a GeForce, 16 simultaneous
// opens past the 12-session cap left just one open. Bounded by sessionProbeTimeout.
func rampSessions(ctx context.Context, ffmpeg string, enc Encoder, bound int, manager *diagnostics.ProcessManager) (int, error) {
	if bound <= 0 {
		return 0, nil
	}
	tiny := Profile{Width: 320, Height: 180, Framerate: 25, AudioBitrate: 64, Encoder: enc}
	pipe, err := Build(HostFor(enc, false, GPUFilters{}), MediaFormat{}, ChannelOutput(tiny))
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, sessionProbeTimeout)
	defer cancel()
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-nostats", "-progress", "pipe:1", "-stats_period", probeStatsPeriod}
	args = append(args, pipe.PreInput...)
	args = append(args, "-re", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25:duration=60", "-vf", pipe.VideoFilter)
	args = append(args, pipe.VideoEncode...)
	args = append(args, "-an", "-f", "null", "-")

	var held sync.WaitGroup
	defer func() { cancel(); held.Wait() }()
	opened := 0
	for opened < bound {
		n := min(sessionProbeBatch, bound-opened)
		var batch sync.WaitGroup
		var mu sync.Mutex
		got, reason := 0, ""
		for i := range n {
			batch.Add(1)
			go func() {
				defer batch.Done()
				ok, why := openHeldSession(ctx, ffmpeg, args, fmt.Sprintf("%s#%d", enc, opened+i+1), manager, &held)
				mu.Lock()
				defer mu.Unlock()
				if ok {
					got++
				} else {
					reason = why
				}
			}()
		}
		batch.Wait()
		opened += got
		if got < n {
			// The batch crossed the cap, or met a session the driver had not yet released: settle
			// the last few one at a time, stopping at the first refusal.
			for opened < bound {
				ok, why := openHeldSession(ctx, ffmpeg, args, fmt.Sprintf("%s#%d", enc, opened+1), manager, &held)
				if !ok {
					reason = why
					break
				}
				opened++
			}
			if opened == 0 {
				return 0, errors.New(reason)
			}
			break
		}
	}
	return opened, nil
}

// openHeldSession starts one session and reports once it has encoded a frame (ok) or exited or run
// out of time (not ok, with ffmpeg's reason). An opened session keeps running until ctx ends.
func openHeldSession(
	ctx context.Context, ffmpeg string, args []string, target string, manager *diagnostics.ProcessManager, held *sync.WaitGroup,
) (bool, string) {
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	var stderr strings.Builder
	cmd.Stderr = &limitedWriter{w: &stderr, n: 4 << 10}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, err.Error()
	}
	run := manager.Begin(diagnostics.ProcessSpec{Purpose: "encoder_session_probe", Target: target, Executable: ffmpeg, Args: args})
	if err := startLowPriority(cmd); err != nil {
		if run != nil {
			run.Finish(diagnostics.ProcessResult{Err: err})
		}
		return false, err.Error()
	}
	encoding := make(chan bool, 1)
	held.Add(1)
	go func() {
		defer held.Done()
		scanner := bufio.NewScanner(stdout)
		signalled := false
		for scanner.Scan() {
			if n, ok := strings.CutPrefix(scanner.Text(), "frame="); ok && n != "0" && !signalled {
				signalled = true
				encoding <- true
			}
		}
		err := cmd.Wait()
		if run != nil {
			run.Finish(diagnostics.ProcessResult{Err: err, Cancelled: ctx.Err() != nil, TerminationReason: "session probe ended"})
		}
		if !signalled {
			encoding <- false
		}
	}()
	if opened := <-encoding; opened {
		if ctx.Err() == nil {
			return true, ""
		}
		return false, "no encoder session opened in time" // still running: its stderr is not ours to read
	}
	// Exited before a frame, and reaped (false is sent after Wait): this session did not open. Only
	// this session is waited for; the held ones run until the ramp ends.
	reason := firstLine(strings.TrimSpace(stderr.String()))
	if reason == "" {
		reason = "no encoder session opened in time"
	}
	return false, reason
}
