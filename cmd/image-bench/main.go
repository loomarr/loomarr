// Command image-bench measures complete AVIF ladders through Loomarr's production Go-to-Rust
// process seam. It is comparative evidence, not a timing gate (design §22 V59b).
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"hash/adler32"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/images"
	"github.com/loomarr/loomarr/internal/images/rustgen"
)

const benchmarkRecipe = "loomarr-rendition-v2"

type benchmarkReport struct {
	SchemaVersion int                `json:"schemaVersion"`
	Corpus        string             `json:"corpus"`
	Strategy      string             `json:"strategy"`
	Recipe        string             `json:"recipe"`
	Release       string             `json:"release"`
	GOOS          string             `json:"goos"`
	GOARCH        string             `json:"goarch"`
	LogicalCPUs   int                `json:"logicalCpus"`
	GOMAXPROCS    int                `json:"gomaxprocs"`
	CPUProfile    int                `json:"cpuProfile"`
	Workers       int                `json:"concurrentWorkers"`
	AVIFThreads   int                `json:"avifThreadsPerWorker"`
	Runs          int                `json:"runs"`
	Warmups       int                `json:"warmups"`
	Profiles      []benchmarkProfile `json:"profiles"`
	Summary       benchmarkSummary   `json:"summary"`
}

type benchmarkProfile struct {
	Role         string            `json:"role"`
	SourceWidth  int               `json:"sourceWidth"`
	SourceHeight int               `json:"sourceHeight"`
	SourceBytes  int64             `json:"sourceBytes"`
	SourceSHA256 string            `json:"sourceSha256"`
	Widths       []int             `json:"widths"`
	Samples      []benchmarkSample `json:"samples"`
	Summary      profileSummary    `json:"summary"`
	sourceFormat string
}

type benchmarkSample struct {
	Processes     int     `json:"processes"`
	Renditions    int     `json:"renditions"`
	OutputBytes   int64   `json:"outputBytes"`
	WallTimeMS    int64   `json:"wallTimeMs"`
	PeakRSSBytes  int64   `json:"peakRssBytes,omitempty"`
	WorkerTimesMS []int64 `json:"workerTimesMs"`
}

type profileSummary struct {
	ProcessesPerRun           int     `json:"processesPerRun"`
	RenditionsPerRun          int     `json:"renditionsPerRun"`
	MedianWallTimeMS          int64   `json:"medianWallTimeMs"`
	MedianImagesPerMinute     float64 `json:"medianImagesPerMinute"`
	MedianRenditionsPerMinute float64 `json:"medianRenditionsPerMinute"`
	P50WorkerTimeMS           int64   `json:"p50WorkerTimeMs"`
	P95WorkerTimeMS           int64   `json:"p95WorkerTimeMs"`
	MaxWorkerTimeMS           int64   `json:"maxWorkerTimeMs"`
	MedianOutputBytes         int64   `json:"medianOutputBytes"`
	MaxPeakRSSBytes           int64   `json:"maxPeakRssBytes,omitempty"`
}

type benchmarkSummary struct {
	Processes                 int     `json:"processes"`
	Renditions                int     `json:"renditions"`
	MedianImagesPerMinute     float64 `json:"medianImagesPerMinute"`
	MedianRenditionsPerMinute float64 `json:"medianRenditionsPerMinute"`
	MaxPeakRSSBytes           int64   `json:"maxPeakRssBytes,omitempty"`
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	runsDefault, err := envInt("IMAGE_BENCH_RUNS", 3)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "image-bench: %v\n", err)
		return 2
	}
	warmupsDefault, err := envInt("IMAGE_BENCH_WARMUPS", 1)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "image-bench: %v\n", err)
		return 2
	}
	flags := flag.NewFlagSet("image-bench", flag.ContinueOnError)
	flags.SetOutput(stderr)
	reportPath := flags.String("report", os.Getenv("IMAGE_BENCH_REPORT"), "JSON report path")
	worker := flags.String("worker", os.Getenv("LOOMARR_IMAGE_WORKER"), "loomarr-image executable")
	rolesArg := flags.String("roles", "poster,backdrop,icon", "comma-separated roles: poster, backdrop, icon")
	runs := flags.Int("runs", runsDefault, "measured runs per role")
	warmups := flags.Int("warmups", warmupsDefault, "unreported warm-up runs per role")
	workers := flags.Int("workers", 1, "concurrent worker processes per measured batch")
	avifThreads := flags.Int("avif-threads", 1, "rav1e threads per worker (benchmark only)")
	cpuProfile := flags.Int("cpu-profile", runtime.GOMAXPROCS(0), "logical CPUs made available to this run")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *reportPath == "" {
		_, _ = fmt.Fprintln(stderr, "image-bench: --report or IMAGE_BENCH_REPORT is required")
		return 2
	}
	if *runs < 1 || *runs > 20 || *warmups < 0 || *warmups > 5 {
		_, _ = fmt.Fprintln(stderr, "image-bench: runs must be 1..20 and warmups must be 0..5")
		return 2
	}
	if *workers < 1 || *workers > 8 || *avifThreads < 1 || *avifThreads > 8 {
		_, _ = fmt.Fprintln(stderr, "image-bench: workers and avif-threads must be 1..8")
		return 2
	}
	if *cpuProfile < 1 || *cpuProfile > runtime.NumCPU() || (*workers)*(*avifThreads) > *cpuProfile {
		_, _ = fmt.Fprintln(stderr, "image-bench: cpu-profile must cover workers times avif-threads and not exceed host CPUs")
		return 2
	}
	profiles, err := selectProfiles(*rolesArg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "image-bench: %v\n", err)
		return 2
	}
	if *worker == "" {
		*worker = filepath.Join("bin", "loomarr-image")
	}
	absoluteWorker, err := filepath.Abs(*worker)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "image-bench: worker path: %v\n", err)
		return 2
	}
	release := os.Getenv("LOOMARR_RELEASE")
	if release == "" {
		release = "dev"
	}
	renderer, err := rustgen.OpenBenchmark(absoluteWorker, rustgen.Contract{
		Protocol: 1, Release: release, Recipe: benchmarkRecipe,
		RequiredFormats: []string{"avif", "jpeg", "webp"}, Animation: true,
	}, *avifThreads)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "image-bench: %v\n", err)
		return 1
	}

	benchCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	report, err := benchmark(benchCtx, renderer, release, profiles, *runs, *warmups, *workers, *avifThreads, *cpuProfile)
	if writeErr := writeReport(*reportPath, report); writeErr != nil {
		_, _ = fmt.Fprintf(stderr, "image-bench: write report: %v\n", writeErr)
		return 1
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "image-bench: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "image-bench: %d renditions in %d worker processes; report %s\n",
		report.Summary.Renditions, report.Summary.Processes, *reportPath)
	return 0
}

func benchmark(ctx context.Context, renderer images.Renderer, release string, profiles []benchmarkProfile, runs, warmups, workers, avifThreads, cpuProfile int) (benchmarkReport, error) {
	report := benchmarkReport{
		SchemaVersion: 2, Corpus: "synthetic-role-v1", Strategy: "concurrent-stepped-ladders-v3",
		Recipe: benchmarkRecipe, Release: release,
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(),
		GOMAXPROCS: runtime.GOMAXPROCS(0), CPUProfile: cpuProfile,
		Workers: workers, AVIFThreads: avifThreads,
		Runs: runs, Warmups: warmups, Profiles: profiles,
	}
	root, err := os.MkdirTemp("", "loomarr-image-bench-*")
	if err != nil {
		return report, fmt.Errorf("create workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(root) }()

	for profileIndex := range report.Profiles {
		profile := &report.Profiles[profileIndex]
		source := filepath.Join(root, profile.Role+"."+profile.sourceFormat)
		if err := writeBenchmarkSource(source, *profile); err != nil {
			return report, fmt.Errorf("create %s source: %w", profile.Role, err)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return report, fmt.Errorf("read %s source: %w", profile.Role, err)
		}
		profile.SourceBytes = int64(len(data))
		digest := fmt.Sprintf("%x", sha256.Sum256(data))
		profile.SourceSHA256 = digest
		for warmup := 0; warmup < warmups; warmup++ {
			if _, err := measureConcurrentLadders(ctx, renderer, root, source, digest, *profile, fmt.Sprintf("warmup-%d", warmup), workers); err != nil {
				return report, fmt.Errorf("%s warm-up %d: %w", profile.Role, warmup+1, err)
			}
		}
		for measured := 0; measured < runs; measured++ {
			sample, err := measureConcurrentLadders(ctx, renderer, root, source, digest, *profile, fmt.Sprintf("run-%d", measured), workers)
			if err != nil {
				return report, fmt.Errorf("%s run %d: %w", profile.Role, measured+1, err)
			}
			profile.Samples = append(profile.Samples, sample)
			report.Summary.Processes += sample.Processes
			report.Summary.Renditions += sample.Renditions
			report.Summary.MaxPeakRSSBytes = max(report.Summary.MaxPeakRSSBytes, sample.PeakRSSBytes)
		}
		profile.Summary = summarizeProfile(*profile)
	}
	summarizeReport(&report)
	return report, nil
}

func measureConcurrentLadders(ctx context.Context, renderer images.Renderer, root, source, digest string, profile benchmarkProfile, runID string, workers int) (benchmarkSample, error) {
	type result struct {
		sample benchmarkSample
		err    error
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan result, workers)
	var started sync.WaitGroup
	started.Add(workers)
	batchStarted := time.Now()
	for worker := range workers {
		go func() {
			started.Done()
			started.Wait()
			sample, err := measureLadder(ctx, renderer, root, source, digest, profile, fmt.Sprintf("%s-worker-%d", runID, worker))
			results <- result{sample: sample, err: err}
		}()
	}
	sample := benchmarkSample{}
	var firstErr error
	for range workers {
		got := <-results
		if got.err != nil {
			if firstErr == nil {
				firstErr = got.err
				cancel()
			}
			continue
		}
		sample.Processes += got.sample.Processes
		sample.Renditions += got.sample.Renditions
		sample.OutputBytes += got.sample.OutputBytes
		sample.PeakRSSBytes += got.sample.PeakRSSBytes
		sample.WorkerTimesMS = append(sample.WorkerTimesMS, got.sample.WorkerTimesMS...)
	}
	if firstErr != nil {
		return sample, firstErr
	}
	sample.WallTimeMS = max(1, time.Since(batchStarted).Milliseconds())
	return sample, nil
}

func measureLadder(ctx context.Context, renderer images.Renderer, root, source, digest string, profile benchmarkProfile, runID string) (benchmarkSample, error) {
	sample := benchmarkSample{WorkerTimesMS: make([]int64, 0, 1)}
	started := time.Now()
	staging := filepath.Join(root, "staging", profile.Role, runID)
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return sample, err
	}
	targets := make([]rustgen.Target, 0, len(profile.Widths))
	for _, width := range profile.Widths {
		targets = append(targets, rustgen.Target{
			ID: fmt.Sprintf("avif-w%d", width), Format: "avif", Width: width, Motion: "first_frame",
		})
	}
	var observations []rustgen.Observation
	observed := rustgen.WithObserver(ctx, func(observation rustgen.Observation) {
		observations = append(observations, observation)
	})
	manifest, err := renderer.Generate(observed, rustgen.Request{
		RequestID: fmt.Sprintf("bench-%s-%s", profile.Role, runID),
		Source:    rustgen.Source{Path: source, ExpectedSHA256: digest}, StagingDir: staging,
		Targets: targets, Budget: benchmarkBudget(),
	})
	if err != nil {
		_ = os.RemoveAll(staging)
		return sample, err
	}
	if len(observations) != 1 || len(manifest.Outputs) != len(profile.Widths) {
		_ = os.RemoveAll(staging)
		return sample, fmt.Errorf("ladder returned %d observations and %d outputs", len(observations), len(manifest.Outputs))
	}
	observation := observations[0]
	sample.Processes = 1
	sample.Renditions = len(manifest.Outputs)
	for _, output := range manifest.Outputs {
		sample.OutputBytes += output.Bytes
	}
	sample.PeakRSSBytes = observation.PeakRSSBytes
	sample.WorkerTimesMS = append(sample.WorkerTimesMS, max(1, observation.Duration.Milliseconds()))
	if err := os.RemoveAll(staging); err != nil {
		return sample, err
	}
	sample.WallTimeMS = max(1, time.Since(started).Milliseconds())
	return sample, nil
}

func selectProfiles(raw string) ([]benchmarkProfile, error) {
	available := map[string]benchmarkProfile{
		"poster": {
			Role: "poster", SourceWidth: 1560, SourceHeight: 2340,
			Widths: images.RolePoster.Widths(), sourceFormat: "jpg",
		},
		"backdrop": {
			Role: "backdrop", SourceWidth: 2560, SourceHeight: 1440,
			Widths: images.RoleBackdrop.Widths(), sourceFormat: "jpg",
		},
		"icon": {
			Role: "icon", SourceWidth: 1000, SourceHeight: 1000,
			Widths: images.RoleIcon.Widths(), sourceFormat: "png",
		},
	}
	var profiles []benchmarkProfile
	seen := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		role := strings.TrimSpace(part)
		profile, ok := available[role]
		if !ok || role == "" {
			return nil, fmt.Errorf("unknown role %q (want poster, backdrop, or icon)", role)
		}
		if seen[role] {
			return nil, fmt.Errorf("role %q is repeated", role)
		}
		seen[role] = true
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func writeBenchmarkSource(path string, profile benchmarkProfile) error {
	img := image.NewNRGBA(image.Rect(0, 0, profile.SourceWidth, profile.SourceHeight))
	for y := range profile.SourceHeight {
		for x := range profile.SourceWidth {
			block := uint8(((x / 47) + (y / 31)) % 7)
			alpha := uint8(255)
			if profile.Role == "icon" {
				alpha = uint8(96 + (x+y)%160)
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x*3+y)/max(1, profile.SourceWidth/2)) + block*11,
				G: uint8((y*5+x)/max(1, profile.SourceHeight/2)) + block*7,
				B: uint8((x+y*2)/max(1, (profile.SourceWidth+profile.SourceHeight)/3)) + block*13,
				A: alpha,
			})
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encoded := false
	defer func() {
		_ = file.Close()
		if !encoded {
			_ = os.Remove(path)
		}
	}()
	if profile.sourceFormat == "png" {
		err = encodeStableBenchmarkPNG(file, img)
	} else {
		err = jpeg.Encode(file, img, &jpeg.Options{Quality: 90})
	}
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	encoded = true
	return nil
}

func encodeStableBenchmarkPNG(out io.Writer, img *image.NRGBA) error {
	if _, err := out.Write([]byte("\x89PNG\r\n\x1a\n")); err != nil {
		return err
	}
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header[0:4], uint32(img.Bounds().Dx()))
	binary.BigEndian.PutUint32(header[4:8], uint32(img.Bounds().Dy()))
	header[8] = 8 // bit depth
	header[9] = 6 // RGBA
	if err := writeBenchmarkPNGChunk(out, "IHDR", header); err != nil {
		return err
	}
	var raw bytes.Buffer
	for y := range img.Bounds().Dy() {
		if err := raw.WriteByte(0); err != nil { // fixed None row filter
			return err
		}
		start := y * img.Stride
		if _, err := raw.Write(img.Pix[start : start+img.Bounds().Dx()*4]); err != nil {
			return err
		}
	}
	if err := writeBenchmarkPNGChunk(out, "IDAT", storedBenchmarkZlib(raw.Bytes())); err != nil {
		return err
	}
	return writeBenchmarkPNGChunk(out, "IEND", nil)
}

func storedBenchmarkZlib(raw []byte) []byte {
	// A hand-written stored-block stream keeps corpus bytes independent of changes to Go's
	// flate implementation. 0x7801 is the zlib header for a 32 KiB window and fastest level.
	checksum := adler32.Checksum(raw)
	encoded := make([]byte, 0, len(raw)+(len(raw)/65_535+1)*5+6)
	encoded = append(encoded, 0x78, 0x01)
	for len(raw) > 0 {
		blockSize := min(len(raw), 65_535)
		if blockSize == len(raw) {
			encoded = append(encoded, 0x01)
		} else {
			encoded = append(encoded, 0x00)
		}
		length := uint16(blockSize)
		encoded = binary.LittleEndian.AppendUint16(encoded, length)
		encoded = binary.LittleEndian.AppendUint16(encoded, ^length)
		encoded = append(encoded, raw[:blockSize]...)
		raw = raw[blockSize:]
	}
	return binary.BigEndian.AppendUint32(encoded, checksum)
}

func writeBenchmarkPNGChunk(out io.Writer, kind string, data []byte) error {
	if err := binary.Write(out, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	if _, err := io.WriteString(out, kind); err != nil {
		return err
	}
	if _, err := out.Write(data); err != nil {
		return err
	}
	digest := crc32.NewIEEE()
	_, _ = io.WriteString(digest, kind)
	_, _ = digest.Write(data)
	return binary.Write(out, binary.BigEndian, digest.Sum32())
}

func summarizeProfile(profile benchmarkProfile) profileSummary {
	var wallTimes, outputBytes, workerTimes []int64
	summary := profileSummary{}
	for _, sample := range profile.Samples {
		wallTimes = append(wallTimes, sample.WallTimeMS)
		outputBytes = append(outputBytes, sample.OutputBytes)
		workerTimes = append(workerTimes, sample.WorkerTimesMS...)
		summary.MaxPeakRSSBytes = max(summary.MaxPeakRSSBytes, sample.PeakRSSBytes)
	}
	if len(profile.Samples) == 0 {
		return summary
	}
	summary.ProcessesPerRun = profile.Samples[0].Processes
	summary.RenditionsPerRun = profile.Samples[0].Renditions
	summary.MedianWallTimeMS = percentile(wallTimes, 50)
	summary.MedianOutputBytes = percentile(outputBytes, 50)
	summary.P50WorkerTimeMS = percentile(workerTimes, 50)
	summary.P95WorkerTimeMS = percentile(workerTimes, 95)
	summary.MaxWorkerTimeMS = percentile(workerTimes, 100)
	summary.MedianImagesPerMinute = perMinute(summary.ProcessesPerRun, summary.MedianWallTimeMS)
	summary.MedianRenditionsPerMinute = perMinute(summary.RenditionsPerRun, summary.MedianWallTimeMS)
	return summary
}

func summarizeReport(report *benchmarkReport) {
	var medianWall int64
	var images, renditions int
	for _, profile := range report.Profiles {
		medianWall += profile.Summary.MedianWallTimeMS
		images += profile.Summary.ProcessesPerRun
		renditions += profile.Summary.RenditionsPerRun
	}
	report.Summary.MedianImagesPerMinute = perMinute(images, medianWall)
	report.Summary.MedianRenditionsPerMinute = perMinute(renditions, medianWall)
}

func percentile(values []int64, percent int) int64 {
	if len(values) == 0 {
		return 0
	}
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	index := (len(ordered)*percent + 99) / 100
	return ordered[max(0, index-1)]
}

func perMinute(count int, durationMS int64) float64 {
	if durationMS <= 0 {
		return 0
	}
	return float64(count) * 60_000 / float64(durationMS)
}

func benchmarkBudget() rustgen.Budget {
	return rustgen.Budget{
		MaxInputBytes: 8 << 20, MaxWidth: 16_384, MaxHeight: 16_384,
		MaxCanvasPixels: 40_000_000, MaxFrames: 600, MaxTotalFramePixels: 600_000_000,
		MaxDurationMS: 60_000, MaxOutputBytes: 64 << 20,
	}
}

func writeReport(path string, report benchmarkReport) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".image-bench-*.json")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, absolute); err != nil {
		return err
	}
	ok = true
	return nil
}

func envInt(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}
