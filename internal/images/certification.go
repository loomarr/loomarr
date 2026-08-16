package images

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/images/rustgen"
)

const certificationMaxInputBytes int64 = 8 << 20

// CertificationLimits are intentionally fixed by design rather than exposed as app settings.
type CertificationLimits struct {
	StaticMaxDuration   time.Duration
	AnimatedMaxDuration time.Duration
	MaxPeakRSSBytes     int64
	MaxCases            int
}

// DefaultCertificationLimits returns the §22 V59a certification ceilings.
func DefaultCertificationLimits() CertificationLimits {
	return CertificationLimits{
		StaticMaxDuration: 10 * time.Second, AnimatedMaxDuration: 30 * time.Second,
		MaxPeakRSSBytes: 768 << 20, MaxCases: 10_000,
	}
}

// CertificationOptions selects the read-only corpus and the production renderer seam.
type CertificationOptions struct {
	CorpusDir        string
	Renderer         Renderer
	Limits           CertificationLimits
	ExpectedRefusals map[string]string
	BoundaryCases    []CertificationBoundary
}

// CertificationBoundary exercises one worker budget independently with a small corpus source.
type CertificationBoundary struct {
	Name         string
	Source       string
	Budget       rustgen.Budget
	Targets      []rustgen.Target
	ExpectedCode string
}

// CertificationReport is stable JSON-facing evidence from one corpus run.
type CertificationReport struct {
	Passed  bool                 `json:"passed"`
	Cases   []CertificationCase  `json:"cases"`
	Summary CertificationSummary `json:"summary"`
}

type CertificationCase struct {
	Path         string           `json:"path"`
	Outcome      string           `json:"outcome"`
	ErrorCode    string           `json:"errorCode,omitempty"`
	MIME         string           `json:"mime,omitempty"`
	Width        int              `json:"width,omitempty"`
	Height       int              `json:"height,omitempty"`
	Animated     bool             `json:"animated"`
	FrameCount   int              `json:"frameCount,omitempty"`
	DurationMS   int64            `json:"durationMs,omitempty"`
	LoopCount    *int             `json:"loopCount,omitempty"`
	SourceBytes  int64            `json:"sourceBytes"`
	OutputBytes  int64            `json:"outputBytes"`
	WallTimeMS   int64            `json:"wallTimeMs"`
	PeakRSSBytes int64            `json:"peakRssBytes,omitempty"`
	StagingClean bool             `json:"stagingClean"`
	Outputs      []rustgen.Output `json:"outputs,omitempty"`
}

type CertificationSummary struct {
	Passed          int   `json:"passed"`
	Refused         int   `json:"refused"`
	Failed          int   `json:"failed"`
	Skipped         int   `json:"skipped"`
	SourceBytes     int64 `json:"sourceBytes"`
	OutputBytes     int64 `json:"outputBytes"`
	P50WallTimeMS   int64 `json:"p50WallTimeMs"`
	P95WallTimeMS   int64 `json:"p95WallTimeMs"`
	MaxWallTimeMS   int64 `json:"maxWallTimeMs"`
	MaxPeakRSSBytes int64 `json:"maxPeakRssBytes"`
}

// Certify drives every supported-looking regular file through inspection and a complete ladder.
// It never follows symlinks and never writes beneath CorpusDir.
func Certify(ctx context.Context, opts CertificationOptions) (CertificationReport, error) {
	if opts.Renderer == nil {
		return CertificationReport{}, errors.New("images: certification needs the required renderer")
	}
	root, err := filepath.Abs(opts.CorpusDir)
	if err != nil {
		return CertificationReport{}, fmt.Errorf("images: certification corpus: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return CertificationReport{}, fmt.Errorf("images: certification corpus is not a directory: %s", root)
	}
	limits := opts.Limits
	if limits == (CertificationLimits{}) {
		limits = DefaultCertificationLimits()
	}

	type corpusInput struct {
		path      string
		supported bool
	}
	var inputs []corpusInput
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		inputs = append(inputs, corpusInput{path: path, supported: certificationExtension(filepath.Ext(path))})
		if len(inputs) > limits.MaxCases {
			return fmt.Errorf("images: certification corpus exceeds %d cases", limits.MaxCases)
		}
		return nil
	})
	if err != nil {
		return CertificationReport{}, err
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].path < inputs[j].path })

	if len(inputs) == 0 && len(opts.BoundaryCases) == 0 {
		return CertificationReport{}, errors.New("images: certification corpus contains no regular files")
	}
	report := CertificationReport{Passed: true, Cases: make([]CertificationCase, 0, len(inputs)+len(opts.BoundaryCases))}
	for _, input := range inputs {
		if !input.supported {
			rel, _ := filepath.Rel(root, input.path)
			report.Cases = append(report.Cases, CertificationCase{
				Path: filepath.ToSlash(rel), Outcome: "skipped", StagingClean: true,
			})
			report.Summary.Skipped++
			continue
		}
		result := certifyOne(ctx, opts.Renderer, root, input.path, limits, opts.ExpectedRefusals)
		appendCertificationCase(&report, result)
	}
	for _, boundary := range opts.BoundaryCases {
		appendCertificationCase(&report, certifyBoundary(ctx, opts.Renderer, root, boundary))
	}
	summarizeDurations(&report)
	if !report.Passed {
		return report, fmt.Errorf("images: certification failed %d of %d cases", report.Summary.Failed, len(report.Cases))
	}
	return report, nil
}

func appendCertificationCase(report *CertificationReport, result CertificationCase) {
	report.Cases = append(report.Cases, result)
	switch result.Outcome {
	case "passed":
		report.Summary.Passed++
	case "refused":
		report.Summary.Refused++
	default:
		report.Passed = false
		report.Summary.Failed++
	}
	report.Summary.SourceBytes += result.SourceBytes
	report.Summary.OutputBytes += result.OutputBytes
	report.Summary.MaxPeakRSSBytes = max(report.Summary.MaxPeakRSSBytes, result.PeakRSSBytes)
}

func certifyOne(ctx context.Context, renderer Renderer, root, source string, limits CertificationLimits, expectedRefusals map[string]string) (result CertificationCase) {
	rel, _ := filepath.Rel(root, source)
	rel = filepath.ToSlash(rel)
	result = CertificationCase{Path: rel, Outcome: "failed"}
	info, err := os.Stat(source)
	if err != nil {
		result.ErrorCode = "io_error"
		return result
	}
	file, err := os.Open(source) //nolint:gosec // source was found beneath the read-only corpus walk
	if err != nil {
		result.ErrorCode = "io_error"
		return result
	}
	data, readErr := io.ReadAll(io.LimitReader(file, certificationMaxInputBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		result.ErrorCode = "io_error"
		return result
	}
	result.SourceBytes = info.Size()
	oversized := info.Size() > certificationMaxInputBytes
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	runRoot, err := os.MkdirTemp("", "loomarr-image-cert-*")
	if err != nil {
		result.ErrorCode = "io_error"
		return result
	}
	defer func() {
		if os.RemoveAll(runRoot) != nil {
			return
		}
		_, statErr := os.Lstat(runRoot)
		result.StagingClean = errors.Is(statErr, os.ErrNotExist)
	}()
	stagedSource := filepath.Join(runRoot, "source")
	if err := os.WriteFile(stagedSource, data, 0o600); err != nil {
		result.ErrorCode = "io_error"
		return result
	}

	var observations []rustgen.Observation
	observed := rustgen.WithObserver(ctx, func(observation rustgen.Observation) {
		observations = append(observations, observation)
	})
	budget := DefaultCertificationBudget()
	inspectDir := filepath.Join(runRoot, "inspect")
	if err := os.Mkdir(inspectDir, 0o700); err != nil {
		result.ErrorCode = "io_error"
		return result
	}
	inspection, err := renderer.Generate(observed, rustgen.Request{
		RequestID: "inspect", Source: rustgen.Source{Path: stagedSource, ExpectedSHA256: digest},
		StagingDir: inspectDir, Targets: []rustgen.Target{}, Budget: budget,
	})
	if err != nil {
		result.ErrorCode = certificationErrorCode(err)
		collectObservations(&result, observations)
		if expectedRefusals[rel] == result.ErrorCode || oversized && result.ErrorCode == "limit_exceeded" {
			result.Outcome = "refused"
		}
		return result
	}
	result.MIME, result.Width, result.Height = inspection.Source.MIME, inspection.Source.Width, inspection.Source.Height
	result.Animated, result.FrameCount, result.DurationMS = inspection.Source.Animated, inspection.Source.FrameCount, inspection.Source.DurationMS
	result.LoopCount = inspection.Source.LoopCount

	renderDir := filepath.Join(runRoot, "render")
	if err := os.Mkdir(renderDir, 0o700); err != nil {
		result.ErrorCode = "io_error"
		return result
	}
	manifest, err := renderer.Generate(observed, rustgen.Request{
		RequestID: "render", Source: rustgen.Source{Path: stagedSource, ExpectedSHA256: digest},
		StagingDir: renderDir,
		Targets: []rustgen.Target{
			{ID: "jpeg-w320", Format: "jpeg", Width: 320, Motion: "first_frame"},
			{ID: "webp-w320", Format: "webp", Width: 320, Motion: "preserve"},
			{ID: "avif-w320", Format: "avif", Width: 320, Motion: "first_frame"},
		},
		Budget: budget,
	})
	collectObservations(&result, observations)
	if err != nil {
		result.ErrorCode = certificationErrorCode(err)
		if expectedRefusals[rel] == result.ErrorCode {
			result.Outcome = "refused"
		}
		return result
	}
	if expectedRefusals[rel] != "" {
		result.ErrorCode = "expected_refusal_missing"
		return result
	}
	result.Outputs = manifest.Outputs
	for _, output := range manifest.Outputs {
		result.OutputBytes += output.Bytes
	}
	ceiling := limits.StaticMaxDuration
	if result.Animated {
		ceiling = limits.AnimatedMaxDuration
	}
	if time.Duration(result.WallTimeMS)*time.Millisecond > ceiling {
		result.ErrorCode = "duration_limit"
		return result
	}
	if result.PeakRSSBytes > limits.MaxPeakRSSBytes {
		result.ErrorCode = "rss_limit"
		return result
	}
	result.Outcome = "passed"
	return result
}

func collectObservations(result *CertificationCase, observations []rustgen.Observation) {
	for _, observation := range observations {
		result.WallTimeMS += max(1, observation.Duration.Milliseconds())
		result.PeakRSSBytes = max(result.PeakRSSBytes, observation.PeakRSSBytes)
	}
}

func certifyBoundary(ctx context.Context, renderer Renderer, root string, boundary CertificationBoundary) (result CertificationCase) {
	result = CertificationCase{Path: "@limits/" + boundary.Name, Outcome: "failed"}
	source := filepath.Join(root, filepath.FromSlash(boundary.Source))
	data, err := os.ReadFile(source)
	if err != nil {
		result.ErrorCode = "io_error"
		return result
	}
	result.SourceBytes = int64(len(data))
	runRoot, err := os.MkdirTemp("", "loomarr-image-cert-limit-*")
	if err != nil {
		result.ErrorCode = "io_error"
		return result
	}
	defer func() {
		if os.RemoveAll(runRoot) != nil {
			return
		}
		_, statErr := os.Lstat(runRoot)
		result.StagingClean = errors.Is(statErr, os.ErrNotExist)
	}()
	var observations []rustgen.Observation
	observed := rustgen.WithObserver(ctx, func(observation rustgen.Observation) {
		observations = append(observations, observation)
	})
	_, err = renderer.Generate(observed, rustgen.Request{
		RequestID:  "limit-" + boundary.Name,
		Source:     rustgen.Source{Path: source, ExpectedSHA256: fmt.Sprintf("%x", sha256.Sum256(data))},
		StagingDir: runRoot, Targets: boundary.Targets, Budget: boundary.Budget,
	})
	collectObservations(&result, observations)
	if err == nil {
		result.ErrorCode = "expected_refusal_missing"
		return result
	}
	result.ErrorCode = certificationErrorCode(err)
	if result.ErrorCode == boundary.ExpectedCode {
		result.Outcome = "refused"
	}
	return result
}

// DefaultCertificationBudget returns the worker ceilings used by the deterministic certification
// corpus. It is exported only for cmd/image-cert, which owns generation of that non-shipping corpus.
func DefaultCertificationBudget() rustgen.Budget {
	return rustgen.Budget{
		MaxInputBytes: certificationMaxInputBytes, MaxWidth: 16_384, MaxHeight: 16_384,
		MaxCanvasPixels: 40_000_000, MaxFrames: 600, MaxTotalFramePixels: 600_000_000,
		MaxDurationMS: 60_000, MaxOutputBytes: 64 << 20,
	}
}

func certificationErrorCode(err error) string {
	var refusal *rustgen.WorkerError
	if errors.As(err, &refusal) {
		return refusal.Code
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "worker_error"
}

func certificationExtension(ext string) bool {
	return slices.Contains([]string{".apng", ".gif", ".jpeg", ".jpg", ".png", ".webp"}, strings.ToLower(ext))
}

func summarizeDurations(report *CertificationReport) {
	if len(report.Cases) == 0 {
		return
	}
	durations := make([]int64, 0, len(report.Cases))
	for _, result := range report.Cases {
		if result.Outcome == "skipped" {
			continue
		}
		durations = append(durations, result.WallTimeMS)
	}
	if len(durations) == 0 {
		return
	}
	slices.Sort(durations)
	percentile := func(p int) int64 {
		index := (len(durations)*p + 99) / 100
		return durations[max(0, index-1)]
	}
	report.Summary.P50WallTimeMS = percentile(50)
	report.Summary.P95WallTimeMS = percentile(95)
	report.Summary.MaxWallTimeMS = durations[len(durations)-1]
}
