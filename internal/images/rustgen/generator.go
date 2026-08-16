// Package rustgen is the concrete adapter for Loomarr's required Rust image worker (§22).
// Product policy stays in package images; this package owns only the executable contract.
package rustgen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Contract is the release-time handshake the worker must satisfy before Loomarr is ready.
type Contract struct {
	Protocol        int
	Release         string
	Recipe          string
	RequiredFormats []string
	Animation       bool
}

type capabilities struct {
	Protocol  int      `json:"protocol"`
	Release   string   `json:"release"`
	Recipe    string   `json:"recipe"`
	Formats   []string `json:"formats"`
	Animation bool     `json:"animation"`
	SelfTest  bool     `json:"selfTest"`
}

// WorkerError is a stable worker refusal. Callers can distinguish invalid input and exhausted
// budgets from crashes without parsing stderr; implementation diagnostics remain private.
type WorkerError struct {
	Code    string
	Message string
}

func (e *WorkerError) Error() string { return "images: worker " + e.Code + ": " + e.Message }

type errorManifest struct {
	Protocol  int    `json:"protocol"`
	Status    string `json:"status"`
	RequestID string `json:"requestId"`
	Error     struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Generator is a verified handle to the required worker executable.
type Generator struct {
	executable   string
	contract     Contract
	generateArgs []string
}

// Request is one source inspection plus zero or more Renditions. Product policy chooses targets;
// the worker owns every pixel operation needed to satisfy them.
type Request struct {
	Protocol   int      `json:"protocol"`
	RequestID  string   `json:"requestId"`
	Source     Source   `json:"source"`
	StagingDir string   `json:"stagingDir"`
	Targets    []Target `json:"targets"`
	Budget     Budget   `json:"budget"`
}

type Source struct {
	Path           string `json:"path"`
	ExpectedSHA256 string `json:"expectedSha256"`
}

type Target struct {
	ID     string `json:"id"`
	Format string `json:"format"`
	Width  int    `json:"width"`
	Motion string `json:"motion"`
}

type Budget struct {
	MaxInputBytes       int64 `json:"maxInputBytes"`
	MaxWidth            int   `json:"maxWidth"`
	MaxHeight           int   `json:"maxHeight"`
	MaxCanvasPixels     int64 `json:"maxCanvasPixels"`
	MaxFrames           int   `json:"maxFrames"`
	MaxTotalFramePixels int64 `json:"maxTotalFramePixels"`
	MaxDurationMS       int64 `json:"maxDurationMs"`
	MaxOutputBytes      int64 `json:"maxOutputBytes"`
}

type Manifest struct {
	Protocol  int          `json:"protocol"`
	Status    string       `json:"status"`
	RequestID string       `json:"requestId"`
	Source    SourceResult `json:"source"`
	Outputs   []Output     `json:"outputs"`
}

type SourceResult struct {
	SHA256      string `json:"sha256"`
	MIME        string `json:"mime"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Bytes       int64  `json:"bytes"`
	Animated    bool   `json:"animated"`
	FrameCount  int    `json:"frameCount"`
	DurationMS  int64  `json:"durationMs"`
	LoopCount   *int   `json:"loopCount"`
	Placeholder string `json:"placeholder"`
	DominantHex string `json:"dominantHex"`
}

type Output struct {
	TargetID       string `json:"targetId"`
	RelativePath   string `json:"relativePath"`
	RecipeID       string `json:"recipeId"`
	Format         string `json:"format"`
	MIME           string `json:"mime"`
	RequestedWidth int    `json:"requestedWidth"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Bytes          int64  `json:"bytes"`
	SHA256         string `json:"sha256"`
	Animated       bool   `json:"animated"`
}

// Open verifies the executable synchronously. A caller must not make the application ready when
// this fails: there is intentionally no second production implementation.
func Open(executable string, expected Contract) (*Generator, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, executable,
		"capabilities", "--protocol", strconv.Itoa(expected.Protocol), "--self-test")
	stdout := newLimitBuffer(64 << 10)
	stderr := newLimitBuffer(64 << 10)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if stdout.exceeded || stderr.exceeded {
		return nil, fmt.Errorf("images: worker capabilities exceeded protocol limit")
	}
	if err != nil {
		return nil, fmt.Errorf("images: required worker capabilities: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var got capabilities
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		return nil, fmt.Errorf("images: required worker capabilities JSON: %w", err)
	}
	if got.Protocol != expected.Protocol {
		return nil, fmt.Errorf("images: worker protocol %d, need %d", got.Protocol, expected.Protocol)
	}
	if got.Release != expected.Release {
		return nil, fmt.Errorf("images: worker release %q, need %q", got.Release, expected.Release)
	}
	if got.Recipe != expected.Recipe {
		return nil, fmt.Errorf("images: worker recipe %q, need %q", got.Recipe, expected.Recipe)
	}
	if !got.SelfTest {
		return nil, fmt.Errorf("images: worker self-test did not pass")
	}
	if expected.Animation && !got.Animation {
		return nil, fmt.Errorf("images: worker lacks required animation capability")
	}
	for _, format := range expected.RequiredFormats {
		if !slices.Contains(got.Formats, format) {
			return nil, fmt.Errorf("images: worker lacks required %s format", format)
		}
	}
	return &Generator{executable: executable, contract: expected}, nil
}

// OpenBenchmark verifies the production worker, then opts Generate into the worker's explicit
// measurement-only AVIF thread override. Production composition must use Open.
func OpenBenchmark(executable string, expected Contract, avifThreads int) (*Generator, error) {
	if avifThreads < 1 || avifThreads > 8 {
		return nil, fmt.Errorf("images: benchmark AVIF threads must be 1..8")
	}
	gen, err := Open(executable, expected)
	if err != nil {
		return nil, err
	}
	gen.generateArgs = []string{"--benchmark-avif-threads", strconv.Itoa(avifThreads)}
	return gen, nil
}

// Generate runs exactly one bounded worker process and accepts only the complete requested target
// set. Files remain unpublished in StagingDir; package images is the sole publication authority.
func (g *Generator) Generate(ctx context.Context, req Request) (manifest Manifest, err error) {
	kind := "render"
	if len(req.Targets) == 0 {
		kind = "inspect"
	}
	observation := Observation{Kind: kind, Result: "protocol_error"}
	if info, statErr := os.Stat(req.Source.Path); statErr == nil {
		observation.InputBytes = info.Size()
	}
	started := time.Now()
	if observe := observerFrom(ctx); observe != nil {
		defer func() {
			observation.Duration = time.Since(started)
			observe(observation)
		}()
	}

	req.Protocol = g.contract.Protocol
	payload, err := json.Marshal(req)
	if err != nil {
		return Manifest{}, fmt.Errorf("images: worker request JSON: %w", err)
	}

	args := []string{"generate", "--protocol", strconv.Itoa(g.contract.Protocol)}
	args = append(args, g.generateArgs...)
	cmd := exec.CommandContext(ctx, g.executable, args...)
	cmd.Stdin = bytes.NewReader(payload)
	stdout := newLimitBuffer(1 << 20)
	stderr := newLimitBuffer(64 << 10)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	observation.PeakRSSBytes = processPeakRSSBytes(cmd.ProcessState)
	if stdout.exceeded || stderr.exceeded {
		return Manifest{}, fmt.Errorf("images: worker output exceeded protocol limit")
	}
	if runErr != nil {
		var failure errorManifest
		if json.Unmarshal(stdout.Bytes(), &failure) == nil && failure.Protocol == g.contract.Protocol &&
			failure.Status == "error" && failure.RequestID == req.RequestID && failure.Error.Code != "" {
			observation.Result = "refused"
			return Manifest{}, &WorkerError{Code: failure.Error.Code, Message: failure.Error.Message}
		}
		if ctx.Err() != nil {
			observation.Result = "canceled"
		} else {
			observation.Result = "process_error"
		}
		return Manifest{}, fmt.Errorf("images: worker generate: %w: %s", runErr, strings.TrimSpace(stderr.String()))
	}

	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		return Manifest{}, fmt.Errorf("images: worker result JSON: %w", err)
	}
	if err := g.validateManifest(req, manifest); err != nil {
		return Manifest{}, err
	}
	for _, output := range manifest.Outputs {
		observation.OutputBytes += output.Bytes
	}
	observation.Result = "success"
	return manifest, nil
}

func (g *Generator) validateManifest(req Request, manifest Manifest) error {
	if manifest.Protocol != g.contract.Protocol || manifest.Status != "ok" || manifest.RequestID != req.RequestID {
		return fmt.Errorf("images: worker result envelope mismatch")
	}
	if manifest.Source.SHA256 != req.Source.ExpectedSHA256 {
		return fmt.Errorf("images: worker source hash mismatch")
	}
	sourceInfo, err := os.Lstat(req.Source.Path)
	if err != nil || !sourceInfo.Mode().IsRegular() || sourceInfo.Size() != manifest.Source.Bytes ||
		manifest.Source.Bytes > req.Budget.MaxInputBytes {
		return fmt.Errorf("images: worker source does not match the declared regular file")
	}
	sourceFile, err := os.Open(req.Source.Path) //nolint:gosec // caller supplies a private staged/content-addressed path
	if err != nil {
		return fmt.Errorf("images: open worker source: %w", err)
	}
	sourceHash := sha256.New()
	sourceBytes, readErr := io.Copy(sourceHash, io.LimitReader(sourceFile, req.Budget.MaxInputBytes+1))
	closeErr := sourceFile.Close()
	if readErr != nil || closeErr != nil || sourceBytes != manifest.Source.Bytes ||
		fmt.Sprintf("%x", sourceHash.Sum(nil)) != req.Source.ExpectedSHA256 {
		return fmt.Errorf("images: worker source bytes changed during generation")
	}
	if manifest.Source.Width <= 0 || manifest.Source.Height <= 0 || manifest.Source.Bytes <= 0 ||
		manifest.Source.Width > req.Budget.MaxWidth || manifest.Source.Height > req.Budget.MaxHeight ||
		int64(manifest.Source.Width)*int64(manifest.Source.Height) > req.Budget.MaxCanvasPixels ||
		!slices.Contains([]string{"image/gif", "image/jpeg", "image/png", "image/webp"}, manifest.Source.MIME) ||
		manifest.Source.Placeholder == "" || len(manifest.Source.DominantHex) != 7 || manifest.Source.DominantHex[0] != '#' {
		return fmt.Errorf("images: worker returned invalid source metadata")
	}
	if manifest.Source.Animated {
		if manifest.Source.FrameCount < 2 || manifest.Source.FrameCount > req.Budget.MaxFrames ||
			manifest.Source.DurationMS <= 0 || manifest.Source.DurationMS > req.Budget.MaxDurationMS ||
			manifest.Source.LoopCount == nil || *manifest.Source.LoopCount < 0 ||
			int64(manifest.Source.Width)*int64(manifest.Source.Height)*int64(manifest.Source.FrameCount) > req.Budget.MaxTotalFramePixels {
			return fmt.Errorf("images: worker returned invalid animation metadata")
		}
	} else if manifest.Source.FrameCount != 1 || manifest.Source.DurationMS != 0 || manifest.Source.LoopCount != nil {
		return fmt.Errorf("images: worker returned timeline metadata for a still")
	}
	if len(manifest.Outputs) != len(req.Targets) {
		return fmt.Errorf("images: worker returned %d outputs for %d targets", len(manifest.Outputs), len(req.Targets))
	}
	wanted := make(map[string]Target, len(req.Targets))
	for _, target := range req.Targets {
		if _, duplicate := wanted[target.ID]; duplicate {
			return fmt.Errorf("images: duplicate requested target %q", target.ID)
		}
		wanted[target.ID] = target
	}
	seen := make(map[string]bool, len(manifest.Outputs))
	var totalOutputBytes int64
	for _, output := range manifest.Outputs {
		target, ok := wanted[output.TargetID]
		if !ok || seen[output.TargetID] {
			return fmt.Errorf("images: worker returned unexpected target %q", output.TargetID)
		}
		seen[output.TargetID] = true
		if output.RecipeID != g.contract.Recipe || output.Format != target.Format || output.RequestedWidth != target.Width {
			return fmt.Errorf("images: worker target %q does not match request", output.TargetID)
		}
		expectedWidth := min(target.Width, manifest.Source.Width)
		expectedHeight := max(1, int(int64(manifest.Source.Height)*int64(expectedWidth)/int64(manifest.Source.Width)))
		expectedAnimated := manifest.Source.Animated && target.Motion == "preserve"
		if output.Width != expectedWidth || output.Height != expectedHeight || output.Bytes <= 0 ||
			output.MIME != mimeFor(target.Format) || output.Animated != expectedAnimated ||
			(expectedAnimated && target.Format != "webp") {
			return fmt.Errorf("images: worker target %q has invalid output metadata", output.TargetID)
		}
		totalOutputBytes += output.Bytes
		if totalOutputBytes > req.Budget.MaxOutputBytes {
			return fmt.Errorf("images: worker outputs exceed declared budget")
		}
		if output.RelativePath == "" || filepath.Base(output.RelativePath) != output.RelativePath || filepath.Clean(output.RelativePath) != output.RelativePath {
			return fmt.Errorf("images: worker returned unsafe output path %q", output.RelativePath)
		}
		path := filepath.Join(req.StagingDir, output.RelativePath)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("images: worker output %q: %w", output.TargetID, err)
		}
		if !info.Mode().IsRegular() || info.Size() != output.Bytes {
			return fmt.Errorf("images: worker output %q is not the declared regular file", output.TargetID)
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("images: open worker output %q: %w", output.TargetID, err)
		}
		hash := sha256.New()
		prefix := make([]byte, 12)
		_, prefixErr := io.ReadFull(file, prefix)
		var seekErr error
		var readErr error
		if _, seekErr = file.Seek(0, 0); seekErr == nil {
			_, readErr = io.Copy(hash, file)
		}
		closeErr := file.Close()
		if prefixErr != nil || seekErr != nil || readErr != nil || closeErr != nil {
			return fmt.Errorf("images: read worker output %q", output.TargetID)
		}
		if fmt.Sprintf("%x", hash.Sum(nil)) != output.SHA256 {
			return fmt.Errorf("images: worker output %q hash mismatch", output.TargetID)
		}
		if output.Format == "webp" && (string(prefix[:4]) != "RIFF" || string(prefix[8:12]) != "WEBP") {
			return fmt.Errorf("images: worker output %q is not WebP", output.TargetID)
		}
		if output.Format == "jpeg" && (prefix[0] != 0xff || prefix[1] != 0xd8) {
			return fmt.Errorf("images: worker output %q is not JPEG", output.TargetID)
		}
		if output.Format == "avif" && string(prefix[4:8]) != "ftyp" {
			return fmt.Errorf("images: worker output %q is not AVIF", output.TargetID)
		}
	}
	return nil
}

func mimeFor(format string) string {
	switch format {
	case "avif":
		return "image/avif"
	case "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

type limitBuffer struct {
	bytes.Buffer
	remaining int
	exceeded  bool
}

func newLimitBuffer(limit int) *limitBuffer { return &limitBuffer{remaining: limit} }

func (b *limitBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if len(p) > b.remaining {
		b.exceeded = true
		p = p[:b.remaining]
	}
	if len(p) > 0 {
		_, _ = b.Buffer.Write(p)
		b.remaining -= len(p)
	}
	return original, nil
}
