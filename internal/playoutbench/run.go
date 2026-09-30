package playoutbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// Options configures one bench run.
type Options struct {
	FFmpeg, FFprobe string
	// Dir holds the generated corpus and scratch outputs.
	Dir string
	// Family forces a hardware family ("software", "vaapi", "nvenc", "videotoolbox"); empty detects the
	// best working encoder exactly as the app does at boot.
	Family string
	// Height is the output rung (1080 or 720); the bench measures the family's real output profile.
	Height     int
	StartRuns  int
	MaxStreams int
	VMAF       bool
	// Films is a directory of open films (scripts/playout-bench-open-films.sh); empty skips them.
	Films  string
	Commit string
	Log    func(format string, args ...any)
}

// minStreamSpeed is the concurrency bar: every stream must run at least this many times realtime.
const minStreamSpeed = 1.2

// streamLadder is the concurrency steps tried; a run stops at the first step that fails the bar.
var streamLadder = []int{1, 2, 3, 4, 6, 8, 10, 12, 14, 16, 20, 24, 32}

// run is one bench run's state.
type run struct {
	Options
	host   playout.HostProfile
	enc    playout.Encoder
	out    playout.OutputProfile
	prober playout.FormatProber
	rep    *Report
}

// Run measures the corpus on this host and returns the report. Only infrastructure failures (no
// ffmpeg, an unwritable directory) are errors; a clip that fails to encode is a failed Case and a
// missing metric, which Judge turns into a failed threshold.
func Run(ctx context.Context, o Options) (*Report, error) {
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
	if o.FFprobe == "" {
		o.FFprobe = "ffprobe"
		if dir := filepath.Dir(o.FFmpeg); dir != "." {
			o.FFprobe = filepath.Join(dir, "ffprobe")
		}
	}
	o.Dir = CorpusDir(o.Dir)
	r := &run{Options: o}
	r.enc = r.chooseEncoder(ctx)
	r.host = playout.HostFor(r.enc, playout.TonemapperFor(o.FFmpeg)(), playout.GPUFiltersFor(o.FFmpeg)())
	profile := playout.Profile{Width: o.Height * 16 / 9, Height: o.Height, Framerate: 25, Encoder: r.enc, AudioBitrate: 128}
	r.out = playout.ChannelOutput(profile)
	r.prober = playout.FFprobeFormatNextTo(o.FFmpeg)
	version, _, _ := strings.Cut(r.capture(ctx, o.FFmpeg, "-version"), "\n")
	r.rep = NewReport(string(r.host.Family), o.Commit, CorpusName, Host{
		OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), FFmpeg: version,
		Encoder: string(r.enc), Rung: fmt.Sprintf("%dp%d", o.Height, r.out.FPS),
	})

	clips, skipped, err := Generate(ctx, o.FFmpeg, o.Dir)
	if err == nil {
		var films []Clip
		if films, err = Films(o.Films); err == nil {
			clips = append(clips, films...)
		}
	}
	if err != nil {
		return nil, err
	}
	for name, why := range skipped {
		r.rep.Cases = append(r.rep.Cases, Case{Clip: name, Status: "failed", Detail: "corpus generation: " + why})
	}
	pipes := map[string]playout.Pipeline{}
	src := map[string]Clip{}
	for _, c := range clips {
		path := c.Path(o.Dir)
		format, err := r.prober(ctx, path)
		if err != nil {
			r.fail(c, "probe: "+err.Error())
			continue
		}
		pipe, err := playout.Build(r.host, format, r.out)
		if errors.Is(err, playout.ErrRefused) {
			r.rep.Cases = append(r.rep.Cases, Case{Clip: c.Name, Class: c.Class, Status: "refused", Detail: "host cannot transcode in real time; the slate covers the slot"})
			r.skipClass(c, "builder refused this source on this host")
			continue
		} else if err != nil {
			r.fail(c, "build: "+err.Error())
			continue
		}
		pipes[c.Name], src[c.Name] = pipe, c
	}

	var analysed []analysis
	for _, c := range clips {
		pipe, ok := pipes[c.Name]
		if !ok {
			continue
		}
		o.Log("clip %s", c.Name)
		fps := r.out.FPS
		if c.Class != "" {
			r.measureClass(ctx, c, pipe, fps)
		}
		a, err := r.analyse(ctx, c, pipe)
		if err != nil {
			r.fail(c, err.Error())
			continue
		}
		r.rep.Cases = append(r.rep.Cases, Case{Clip: c.Name, Class: c.Class, Status: "ok", Fallbacks: pipe.Fallbacks})
		analysed = append(analysed, a)
	}
	r.reportAnalysis(analysed)
	r.measureConcurrency(ctx, clips, pipes)
	return r.rep, nil
}

func (r *run) fail(c Clip, why string) {
	r.rep.Cases = append(r.rep.Cases, Case{Clip: c.Name, Class: c.Class, Status: "failed", Detail: why})
}

func (r *run) skipClass(c Clip, why string) {
	if c.Class == "" {
		return
	}
	for _, m := range []string{"start_p95_ms", "start_p50_ms", "speed_x", "cores_per_stream", "vmaf_mean"} {
		r.rep.Skipped[m+"/"+c.Class] = why
	}
}

// chooseEncoder honours a forced family and otherwise runs the app's own boot detection.
func (r *run) chooseEncoder(ctx context.Context) playout.Encoder {
	switch r.Family {
	case "software":
		return playout.EncoderSoftware
	case "vaapi":
		return playout.EncoderVAAPI
	case "nvenc":
		return playout.EncoderNVENC
	case "videotoolbox":
		return playout.EncoderVideoToolbox
	}
	probe := playout.Profile{Width: r.Height * 16 / 9, Height: r.Height, Framerate: 25, VideoBitrate: 4000, Encoder: playout.EncoderSoftware, AudioBitrate: 128}
	return playout.DetectObserved(ctx, r.FFmpeg, probe, "", nil).Chosen
}

func (r *run) capture(ctx context.Context, bin string, args ...string) string {
	out, _ := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	return string(out)
}

// timed is one ffmpeg process run to completion.
type timed struct {
	wall time.Duration
	cpu  time.Duration
	// firstByte is when the first output byte arrived (only when captured).
	firstByte time.Duration
	stderr    string
}

// firstByteWriter timestamps the first write and forwards the rest.
type firstByteWriter struct {
	w     io.Writer
	start time.Time
	first time.Duration
	once  sync.Once
}

func (f *firstByteWriter) Write(p []byte) (int, error) {
	f.once.Do(func() { f.first = time.Since(f.start) })
	return f.w.Write(p)
}

// exec runs ffmpeg with args, discarding (or capturing to sink) stdout.
func (r *run) exec(ctx context.Context, args []string, sink io.Writer) (timed, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.FFmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	fb := &firstByteWriter{}
	if sink != nil {
		fb.w = sink
		cmd.Stdout = fb
	}
	start := time.Now()
	fb.start = start
	err := cmd.Run()
	t := timed{wall: time.Since(start), firstByte: fb.first, stderr: tail(stderr.String())}
	if cmd.ProcessState != nil {
		t.cpu = cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()
	}
	if err != nil {
		return t, fmt.Errorf("ffmpeg: %w: %s", err, t.stderr)
	}
	return t, nil
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	// Keep the initial decoder/filter error as well as ffmpeg's final encoder failure. The
	// terminal "Could not open encoder before EOF" alone hides the cause of a no-output run.
	if len(s) > 4096 {
		s = s[:2048] + "\n[diagnostics truncated]\n" + s[len(s)-2048:]
	}
	return s
}

// measureClass reports start latency, speed and CPU for one class clip.
func (r *run) measureClass(ctx context.Context, c Clip, pipe playout.Pipeline, fps int) {
	path := c.Path(r.Dir)
	// Start latency: a fresh process per run, seeking to a different second, until the first second
	// of media (one segment, frames = fps) has been produced. Time to media, never bytes.
	var starts []float64
	for i := 0; i < r.StartRuns; i++ {
		seek := time.Duration(i%max(c.Seconds-3, 1)) * time.Second
		t, err := r.exec(ctx, pipe.ItemArgs(path, seek, fps, fps, 0), nil)
		if err != nil {
			r.fail(c, "start run: "+err.Error())
			return
		}
		starts = append(starts, float64(t.wall)/float64(time.Millisecond))
	}
	slices.Sort(starts)
	r.rep.Set("start_p95_ms/"+c.Class, percentile(starts, 0.95), "ms", Lower)
	r.rep.Set("start_p50_ms/"+c.Class, percentile(starts, 0.50), "ms", Lower)

	// Speed and CPU: the whole clip, unpaced. CPU is child rusage per second of media, which is the
	// cores this stream costs at 1x.
	t, err := r.exec(ctx, pipe.ItemArgs(path, 0, c.frames(fps), fps, 0), nil)
	if err != nil {
		r.fail(c, "speed run: "+err.Error())
		return
	}
	r.rep.Set("speed_x/"+c.Class, float64(c.Seconds)/t.wall.Seconds(), "x", Higher)
	r.rep.Set("cores_per_stream/"+c.Class, t.cpu.Seconds()/float64(c.Seconds), "cores", Lower)

	if c.VMAF {
		r.measureVMAF(ctx, c, pipe, fps)
	}
}

// percentile is nearest-rank over a sorted sample.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[max(idx, 0)]
}

func (r *run) measureVMAF(ctx context.Context, c Clip, pipe playout.Pipeline, fps int) {
	name := "vmaf_mean/" + c.Class
	if !r.VMAF {
		r.rep.Skipped[name] = "disabled"
		return
	}
	if !strings.Contains(r.capture(ctx, r.FFmpeg, "-hide_banner", "-filters"), " libvmaf ") {
		r.rep.Skipped[name] = "this ffmpeg has no libvmaf"
		return
	}
	encoded := filepath.Join(r.Dir, c.Name+".vmaf.ts")
	logPath := filepath.Join(r.Dir, c.Name+".vmaf.json")
	defer func() { _ = os.Remove(encoded) }()
	defer func() { _ = os.Remove(logPath) }()
	if _, err := r.encodeTo(ctx, c, pipe, encoded); err != nil {
		r.rep.Skipped[name] = "encode: " + err.Error()
		return
	}
	// The reference is the source scaled to the output rung and cadence: VMAF compares frames.
	graph := fmt.Sprintf("[0:v]format=yuv420p,setpts=PTS-STARTPTS[d];[1:v]fps=%d,scale=%d:%d:flags=bicubic,format=yuv420p,setpts=PTS-STARTPTS[r];[d][r]libvmaf=log_fmt=json:log_path=%s:n_threads=4",
		fps, r.out.Width, r.out.Height, logPath)
	if _, err := r.exec(ctx, []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-i", encoded, "-i", c.Path(r.Dir), "-lavfi", graph, "-f", "null", "-"}, nil); err != nil {
		r.rep.Skipped[name] = err.Error()
		return
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		r.rep.Skipped[name] = err.Error()
		return
	}
	var log struct {
		Pooled struct {
			VMAF struct {
				Mean float64 `json:"mean"`
			} `json:"vmaf"`
		} `json:"pooled_metrics"`
	}
	if err := json.Unmarshal(raw, &log); err != nil {
		r.rep.Skipped[name] = err.Error()
		return
	}
	r.rep.Set(name, log.Pooled.VMAF.Mean, "vmaf", Higher)
}

// encodeTo runs the clip through the pipeline to a file.
func (r *run) encodeTo(ctx context.Context, c Clip, pipe playout.Pipeline, path string) (t timed, err error) {
	f, err := os.Create(path)
	if err != nil {
		return timed{}, err
	}
	// A failed close can mean a truncated capture, which would be analysed as if it were the output.
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	// ItemArgs writes MPEG-TS to stdout, which the sink captures.
	return r.exec(ctx, pipe.ItemArgs(c.Path(r.Dir), 0, c.frames(r.out.FPS), r.out.FPS, 0), f)
}

// measureConcurrency finds how many simultaneous unpaced streams of the 1080p H.264 class hold
// minStreamSpeed each, and the total CPU those streams cost at 1x.
func (r *run) measureConcurrency(ctx context.Context, clips []Clip, pipes map[string]playout.Pipeline) {
	var c Clip
	for _, cand := range clips {
		if cand.Class == "h264-1080p" {
			c = cand
		}
	}
	pipe, ok := pipes[c.Name]
	if !ok {
		for _, n := range []string{"concurrency/max_streams", "concurrency/total_cores"} {
			r.rep.Skipped[n] = "no 1080p H.264 pipeline on this host"
		}
		return
	}
	best, bestSpeed, bestCores := 0, 0.0, 0.0
	for _, n := range streamLadder {
		if n > r.MaxStreams {
			break
		}
		speeds, cpus, err := r.parallel(ctx, c, pipe, n)
		if err != nil {
			r.rep.Cases = append(r.rep.Cases, Case{Clip: c.Name, Class: "concurrency", Status: "failed", Detail: fmt.Sprintf("%d streams: %v", n, err)})
			break
		}
		lo := slices.Min(speeds)
		r.Log("%d streams: slowest %.2fx", n, lo)
		if lo < minStreamSpeed {
			break
		}
		best, bestSpeed = n, lo
		var sum float64
		for _, cpu := range cpus {
			sum += cpu
		}
		bestCores = sum / float64(c.Seconds)
	}
	r.rep.Set("concurrency/max_streams", float64(best), "streams", Higher)
	r.rep.Set("concurrency/min_speed_x", bestSpeed, "x", Higher)
	r.rep.Set("concurrency/total_cores", bestCores, "cores", Lower)
	if best == r.MaxStreams || (best > 0 && best == streamLadder[len(streamLadder)-1]) {
		r.rep.Cases = append(r.rep.Cases, Case{Clip: c.Name, Class: "concurrency", Status: "ok", Detail: fmt.Sprintf("capped at %d streams, not the host's limit", best)})
	}
}

func (r *run) parallel(ctx context.Context, c Clip, pipe playout.Pipeline, n int) (speeds, cpus []float64, err error) {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t, e := r.exec(ctx, pipe.ItemArgs(c.Path(r.Dir), 0, c.frames(r.out.FPS), r.out.FPS, 0), nil)
			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				errs = append(errs, e)
				return
			}
			speeds = append(speeds, float64(c.Seconds)/t.wall.Seconds())
			cpus = append(cpus, t.cpu.Seconds())
		}()
	}
	wg.Wait()
	return speeds, cpus, errors.Join(errs...)
}
