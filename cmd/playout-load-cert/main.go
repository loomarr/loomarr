package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/playoutcert"
)

type manifest struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Channels      []playoutcert.Channel `json:"channels"`
}

func main() { os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) }

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("playout-load-cert", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "private ordered Channel manifest")
	outPath := flags.String("out", "", "machine report path under LOOMARR_ARTIFACT_DIR")
	certify := flags.Bool("certify", false, "enforce the 100-Channel certification contract")
	synthetic := flags.Bool("synthetic", false, "start an isolated deterministic Loomarr target")
	syntheticCapacity := flags.Int("synthetic-capacity", 4, "isolated target transcode capacity (1..64)")
	syntheticGrace := flags.Duration("synthetic-grace", 2*time.Second, "isolated target warm-session grace")
	remote := flags.Bool("remote-ok", false, "acknowledge that the named origin is not loopback")
	concurrency := flags.Int("concurrency", 12, "bounded HTTP concurrency (1..64)")
	surfRounds := flags.Int("surf-rounds", 1, "complete catalog surf passes")
	fanIn := flags.Int("fan-in", 4, "same-Channel simultaneous viewers")
	requestTimeout := flags.Duration("request-timeout", 15*time.Second, "per-request deadline")
	cleanupTimeout := flags.Duration("cleanup-timeout", 45*time.Second, "cleanup convergence deadline")
	warmGrace := flags.Duration("warm-grace", 30*time.Second, "target warm-session grace")
	suiteTimeout := flags.Duration("suite-timeout", 10*time.Minute, "whole-suite deadline (maximum 30m)")
	rawBytes := flags.Int("raw-capture-bytes", 2<<20, "bounded bytes retained in memory per raw stream")
	ffprobe := flags.String("ffprobe", "ffprobe", "ffprobe executable")
	ffmpeg := flags.String("ffmpeg", "ffmpeg", "ffmpeg executable used for first-frame decode")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(*manifestPath) == "" {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: --manifest is required and positional arguments are refused")
		return 2
	}
	if *concurrency < 1 || *concurrency > 64 || *surfRounds < 1 || *surfRounds > 100 || *fanIn < 1 || *fanIn > 64 || *requestTimeout <= 0 || *cleanupTimeout <= 0 || *warmGrace <= 0 || *warmGrace > time.Minute || *suiteTimeout <= 0 || *suiteTimeout > 30*time.Minute || *syntheticCapacity < 1 || *syntheticCapacity > 64 || *syntheticGrace <= 0 || *syntheticGrace > time.Minute || *rawBytes < 188 || *rawBytes > 16<<20 {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: resource bounds are invalid")
		return 2
	}
	artifactDir := strings.TrimSpace(getenv("LOOMARR_ARTIFACT_DIR"))
	if artifactDir == "" {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: LOOMARR_ARTIFACT_DIR is required")
		return 2
	}
	resolvedOutput := *outPath
	if resolvedOutput == "" {
		resolvedOutput = filepath.Join(artifactDir, "playout-load-cert.json")
	}
	if err := requireContainedOutput(artifactDir, resolvedOutput); err != nil {
		_, _ = fmt.Fprintf(stderr, "playout-load-cert: %v\n", err)
		return 2
	}
	channels, err := readManifest(*manifestPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: invalid private manifest")
		return 2
	}
	runCtx, cancel := context.WithTimeout(ctx, *suiteTimeout)
	defer cancel()
	baseURL := strings.TrimSpace(getenv("LOOMARR_PLAYOUT_CERT_BASE_URL"))
	adminBearer, deviceToken := getenv("LOOMARR_API_TOKEN"), getenv("LOOMARR_PLAYOUT_TOKEN")
	var isolated *playoutcert.SyntheticTarget
	if *synthetic {
		isolated, err = playoutcert.NewSyntheticTarget(runCtx, playoutcert.SyntheticConfig{
			Channels: channels, FFmpeg: *ffmpeg, Capacity: *syntheticCapacity, Grace: *syntheticGrace,
		})
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "playout-load-cert: isolated target setup failed")
			return 1
		}
		baseURL, adminBearer, deviceToken = isolated.BaseURL, isolated.AdminBearer, isolated.DeviceToken
	}
	config := playoutcert.Config{
		BaseURL: baseURL, AdminBearer: adminBearer, DeviceToken: deviceToken,
		Channels: channels, Certify: *certify, RemoteAcknowledged: *remote && !*synthetic,
		Concurrency: *concurrency, SurfRounds: *surfRounds, FanInViewers: *fanIn,
		RequestTimeout: *requestTimeout, CleanupTimeout: *cleanupTimeout, WarmGrace: *warmGrace, RawCaptureBytes: *rawBytes,
		Validator: playoutcert.FFprobeValidator{Path: *ffprobe},
		Decoder:   playoutcert.FFmpegDecoder{Path: *ffmpeg},
	}
	if isolated != nil {
		config.WarmGrace = *syntheticGrace
	}
	report, err := playoutcert.Run(runCtx, config)
	if err != nil {
		if closeErr := closeIsolated(isolated, *cleanupTimeout); closeErr != nil {
			_, _ = fmt.Fprintln(stderr, "playout-load-cert: isolated target cleanup failed")
		}
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: run failed during bounded preflight")
		return 1
	}
	if isolated != nil {
		started := time.Now()
		closeErr := closeIsolated(isolated, *cleanupTimeout)
		elapsedMS := float64(time.Since(started).Microseconds()) / 1000
		phase := playoutcert.Phase{Name: "shutdown", HTTPClasses: map[string]int{"ok": 1}}
		phase.Attempts, phase.Successes = 1, 1
		phase.P50MS, phase.P95MS, phase.P99MS = elapsedMS, elapsedMS, elapsedMS
		if closeErr != nil {
			phase.Successes, phase.Failures = 0, 1
			phase.HTTPClasses = map[string]int{"shutdown_failed": 1}
			report.Failures = append(report.Failures, "shutdown_failed")
		}
		report.Phases = append(report.Phases, phase)
		report.CompletedAt = time.Now()
		report.Certified = *certify && len(report.Failures) == 0
	}
	return publishReport(resolvedOutput, report, *certify, stdout, stderr)
}

func publishReport(output string, report playoutcert.Report, certify bool, stdout, stderr io.Writer) int {
	blob, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: report encoding failed")
		return 1
	}
	if err := writeArtifact(output, append(blob, '\n')); err != nil {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: report write failed")
		return 1
	}
	_, _ = io.WriteString(stdout, playoutcert.HumanSummary(report))
	if len(report.Failures) != 0 || (certify && !report.Certified) {
		return 1
	}
	return 0
}

func closeIsolated(target *playoutcert.SyntheticTarget, timeout time.Duration) error {
	if target == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return target.Close(ctx)
}

func readManifest(path string) ([]playoutcert.Channel, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var value manifest
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if value.SchemaVersion != 1 {
		return nil, errors.New("unsupported manifest schema")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("trailing manifest content")
	}
	return value.Channels, nil
}

func requireContainedOutput(root, output string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return errors.New("artifact directory is invalid")
	}
	outputAbs, err := filepath.Abs(output)
	if err != nil {
		return errors.New("output path is invalid")
	}
	rel, err := filepath.Rel(rootAbs, outputAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("output must remain under LOOMARR_ARTIFACT_DIR")
	}
	return nil
}

func writeArtifact(path string, blob []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".playout-load-cert-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err = temporary.Write(blob); err == nil {
		err = temporary.Chmod(0o600)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
