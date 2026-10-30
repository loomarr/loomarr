// Command filler-encode-bench measures the offline derivative encode (#2121) on this host: every
// source master under -masters is encoded at each candidate constant-quality level of one family,
// and each output is scored against its source with VMAF. Today's software evidence recipe
// (libx264 slow, CRF 14) runs beside it as the baseline. The summary is what picks a family's level:
// the smallest output whose VMAF stays at or above the floor (docs/design/filler-pipeline.md).
//
// A measurement tool, run by hand on real hardware: it executes ffmpeg and is never part of a gate.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/bgexec"
	"github.com/loomarr/loomarr/internal/encode"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "filler-encode-bench:", err)
		os.Exit(1)
	}
}

// baseline is the evidence recipe before #2121 (filler-evidence-v1), which the measured levels replace.
var baseline = []string{"-c:v", "libx264", "-preset", "slow", "-crf", "14", "-profile:v", "high",
	"-force_key_frames", "expr:gte(t,n_forced*1)"}

type sample struct {
	clip, setting   string
	bytes           int64
	wall, cpu       time.Duration
	vmafMean, vmafM float64 // pooled mean and minimum
}

func run() error {
	var (
		ffmpeg  = flag.String("ffmpeg", "ffmpeg", "ffmpeg binary with libvmaf")
		masters = flag.String("masters", "", "directory of source masters (searched recursively for .mp4)")
		family  = flag.String("family", "nvenc", "family under test: nvenc, vaapi, videotoolbox or software")
		levels  = flag.String("levels", "", "comma-separated constant-quality levels to try")
		limit   = flag.Int("limit", 30, "most masters to use (the first N by path, for a repeatable corpus)")
		vmafThr = flag.Int("vmaf-threads", 4, "libvmaf threads")
	)
	flag.Parse()
	if *masters == "" || *levels == "" {
		return errors.New("-masters and -levels are required")
	}
	fam := encode.Family(*family)
	switch fam {
	case encode.FamilyNVENC, encode.FamilyVAAPI, encode.FamilyVideoToolbox, encode.FamilySoftware:
	default:
		return fmt.Errorf("family %q is not benchable", fam)
	}
	var qs []int
	for _, s := range strings.Split(*levels, ",") {
		q, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("level %q: %w", s, err)
		}
		qs = append(qs, q)
	}
	clips, err := corpus(*masters, *limit)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	tmp, err := os.MkdirTemp("", "filler-encode-bench-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	settings := []struct {
		name string
		args []string
	}{{"baseline x264 slow crf14", baseline}}
	for _, q := range qs {
		settings = append(settings, struct {
			name string
			args []string
		}{fmt.Sprintf("%s q%d", fam, q), encode.FileVideoArgs(fam, encode.File{Quality: q, KeyframeSeconds: 1})})
	}

	var all []sample
	for i, clip := range clips {
		for _, s := range settings {
			out := filepath.Join(tmp, "out.mp4")
			m, err := measure(ctx, *ffmpeg, clip, out, s.args, *vmafThr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "skip %s / %s: %v\n", filepath.Base(clip), s.name, err)
				continue
			}
			m.clip, m.setting = filepath.Base(clip), s.name
			all = append(all, m)
		}
		fmt.Fprintf(os.Stderr, "%d/%d %s\n", i+1, len(clips), filepath.Base(clip))
	}
	report(settings[0].name, all)
	return nil
}

func corpus(dir string, limit int) ([]string, error) {
	var clips []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".mp4") {
			clips = append(clips, p)
		}
		return err
	})
	slices.Sort(clips)
	if len(clips) > limit {
		clips = clips[:limit]
	}
	if len(clips) == 0 && err == nil {
		err = fmt.Errorf("no .mp4 masters under %s", dir)
	}
	return clips, err
}

// measure encodes one master with args (video only; audio does not affect the picture) and scores
// the output against it. The encode's CPU is the process's user+system time: the cost the
// hardware path exists to remove.
func measure(ctx context.Context, ffmpeg, in, out string, args []string, vmafThreads int) (sample, error) {
	enc := []string{"-hide_banner", "-nostdin", "-v", "error"}
	if args[1] == string(encode.EncoderVAAPI) {
		enc = append(enc, encode.DeviceInitArgs(encode.EncoderVAAPI)...)
	}
	enc = append(enc, "-i", in, "-map", "0:v:0", "-an")
	if args[1] == string(encode.EncoderVAAPI) {
		enc = append(enc, "-vf", encode.UploadFilter(encode.EncoderVAAPI))
	}
	// `-threads 0` is ffmpeg's own choice, the condition the published levels were measured under;
	// an explicit -threads is what makes bgexec.FFmpeg leave the thread count alone.
	enc = append(enc, args...)
	enc = append(enc, "-threads", "0", "-y", out)
	start := time.Now()
	cmd := bgexec.FFmpeg(ctx, ffmpeg, enc...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return sample{}, fmt.Errorf("encode: %v: %s", err, firstLine(string(b)))
	}
	m := sample{wall: time.Since(start), cpu: cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()}
	st, err := os.Stat(out)
	if err != nil {
		return sample{}, err
	}
	m.bytes = st.Size()

	log := out + ".vmaf.json"
	graph := fmt.Sprintf("[0:v]setpts=PTS-STARTPTS[d];[1:v]setpts=PTS-STARTPTS[r];[d][r]libvmaf=log_fmt=json:log_path=%s:n_threads=%d", log, vmafThreads)
	if b, err := bgexec.FFmpeg(ctx, ffmpeg, "-hide_banner", "-nostdin", "-v", "error", "-i", out, "-i", in,
		"-lavfi", graph, "-threads", "0", "-f", "null", "-").CombinedOutput(); err != nil {
		return sample{}, fmt.Errorf("vmaf: %v: %s", err, firstLine(string(b)))
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		return sample{}, err
	}
	var v struct {
		Pooled struct {
			VMAF struct{ Mean, Min float64 } `json:"vmaf"`
		} `json:"pooled_metrics"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return sample{}, fmt.Errorf("vmaf log: %w", err)
	}
	m.vmafMean, m.vmafM = v.Pooled.VMAF.Mean, v.Pooled.VMAF.Min
	return m, nil
}

// report prints one row per setting: median and 10th-percentile per-clip VMAF, the worst frame,
// total size relative to the baseline, and encode cost.
func report(base string, all []sample) {
	bySetting := map[string][]sample{}
	var order []string
	for _, s := range all {
		if _, ok := bySetting[s.setting]; !ok {
			order = append(order, s.setting)
		}
		bySetting[s.setting] = append(bySetting[s.setting], s)
	}
	var baseBytes int64
	for _, s := range bySetting[base] {
		baseBytes += s.bytes
	}
	fmt.Printf("| setting | clips | VMAF median | VMAF p10 | worst frame | size vs baseline | encode wall | encode CPU |\n")
	fmt.Printf("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, name := range order {
		ss := bySetting[name]
		var means []float64
		var bytes int64
		var wall, cpu time.Duration
		worst := math.Inf(1)
		for _, s := range ss {
			means = append(means, s.vmafMean)
			bytes += s.bytes
			wall += s.wall
			cpu += s.cpu
			worst = min(worst, s.vmafM)
		}
		slices.Sort(means)
		fmt.Printf("| %s | %d | %.2f | %.2f | %.1f | %.0f%% | %s | %s |\n", name, len(ss),
			percentile(means, 0.5), percentile(means, 0.1), worst,
			100*float64(bytes)/float64(max(baseBytes, 1)), wall.Round(time.Second), cpu.Round(time.Second))
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[int(p*float64(len(sorted)-1))]
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
