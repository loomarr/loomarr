package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/app"
	"github.com/loomarr/loomarr/internal/playoutcert"
)

type manifest struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Channels      []playoutcert.Channel `json:"channels"`
}

type repeatedFlag []string

func (f *repeatedFlag) String() string         { return strings.Join(*f, ",") }
func (f *repeatedFlag) Set(value string) error { *f = append(*f, value); return nil }

func main() { os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) }

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("playout-load-cert", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "private ordered Channel manifest")
	outPath := flags.String("out", "", "machine report path under LOOMARR_ARTIFACT_DIR")
	certify := flags.Bool("certify", false, "enforce the 100-Channel certification contract")
	synthetic := flags.Bool("synthetic", false, "start an isolated deterministic Loomarr target")
	syntheticScope := flags.String("synthetic-scope", "isolated-playout-cert", "stable name for the isolated disposable target")
	disposableTarget := flags.String("disposable-target", "", "exact synthetic scope acknowledged for a shutdown drill")
	var faultNames repeatedFlag
	flags.Var(&faultNames, "fault-profile", "selected fault profile (repeatable)")
	syntheticCapacity := flags.Int("synthetic-capacity", 4, "isolated target transcode capacity (1..64)")
	syntheticGrace := flags.Duration("synthetic-grace", 2*time.Second, "isolated target warm-session grace")
	syntheticProgramme := flags.Duration("synthetic-programme-duration", 6*time.Second, "isolated recurring programme duration (2s..30s)")
	remote := flags.Bool("remote-ok", false, "acknowledge that the named origin is not loopback")
	concurrency := flags.Int("concurrency", 12, "bounded HTTP concurrency (1..64)")
	surfRounds := flags.Int("surf-rounds", 1, "complete catalog surf passes")
	fanIn := flags.Int("fan-in", 4, "same-Channel simultaneous viewers")
	requestTimeout := flags.Duration("request-timeout", 15*time.Second, "per-request deadline")
	cleanupTimeout := flags.Duration("cleanup-timeout", 45*time.Second, "cleanup convergence deadline")
	warmGrace := flags.Duration("warm-grace", 30*time.Second, "target warm-session grace")
	programmeBoundaryTimeout := flags.Duration("programme-boundary-timeout", 20*time.Minute, "deadline for observing an actual programme transition (2s..25m)")
	programmeBoundaryLate := flags.Duration("programme-boundary-late-observation", 3*time.Second, "post-transition decoded-media observation (250ms..30s)")
	suiteTimeout := flags.Duration("suite-timeout", 30*time.Minute, "whole-suite deadline (maximum 30m)")
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
	faultProfiles, err := playoutcert.ParseFaultProfiles(faultNames)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: invalid fault profile selection")
		return 2
	}
	controllerScope := ""
	if *synthetic {
		controllerScope = strings.TrimSpace(*syntheticScope)
	}
	if *synthetic && controllerScope == "" {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: synthetic scope is required")
		return 2
	}
	if err := playoutcert.ValidateFaultSelection(faultProfiles, controllerScope, *disposableTarget); err != nil {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: unsafe fault profile selection")
		return 2
	}
	if *suiteTimeout <= 0 || *suiteTimeout > 30*time.Minute || *programmeBoundaryTimeout >= *suiteTimeout || *syntheticCapacity < 1 || *syntheticCapacity > 64 || *syntheticGrace <= 0 || *syntheticGrace > time.Minute || *syntheticProgramme < 2*time.Second || *syntheticProgramme > 30*time.Second {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: resource bounds are invalid")
		return 2
	}
	channels, err := readManifest(*manifestPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: invalid private manifest")
		return 2
	}
	config := playoutcert.Config{
		Channels: channels, Certify: *certify,
		Concurrency: *concurrency, SurfRounds: *surfRounds, FanInViewers: *fanIn,
		RequestTimeout: *requestTimeout, CleanupTimeout: *cleanupTimeout, WarmGrace: *warmGrace, RawCaptureBytes: *rawBytes,
		ProgrammeBoundaryTimeout: *programmeBoundaryTimeout, ProgrammeBoundaryLateObservation: *programmeBoundaryLate,
		FaultProfiles: faultProfiles, DisposableTarget: *disposableTarget,
	}
	if err := config.ValidateInputs(); err != nil {
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
	output, err := openContainedOutput(artifactDir, resolvedOutput)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "playout-load-cert: %v\n", err)
		return 2
	}
	defer func() { _ = output.Close() }()
	runCtx, cancel := context.WithTimeout(ctx, *suiteTimeout)
	defer cancel()
	baseURL := strings.TrimSpace(getenv("LOOMARR_PLAYOUT_CERT_BASE_URL"))
	adminBearer, deviceToken := getenv("LOOMARR_API_TOKEN"), getenv("LOOMARR_PLAYOUT_TOKEN")
	var isolated *app.PlayoutCertificationTarget
	if *synthetic {
		isolated, err = app.NewPlayoutCertificationTarget(runCtx, app.PlayoutCertificationConfig{
			Scope: controllerScope, Channels: channels, FFmpeg: *ffmpeg, Capacity: *syntheticCapacity, Grace: *syntheticGrace, ProgrammeDuration: *syntheticProgramme,
		})
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "playout-load-cert: isolated target setup failed")
			return 1
		}
		baseURL, adminBearer, deviceToken = isolated.BaseURL, isolated.AdminBearer, isolated.DeviceToken
	}
	config.BaseURL, config.AdminBearer, config.DeviceToken = baseURL, adminBearer, deviceToken
	config.RemoteAcknowledged = *remote && !*synthetic
	config.Validator = playoutcert.FFprobeValidator{Path: *ffprobe}
	config.Decoder = playoutcert.FFmpegDecoder{Path: *ffmpeg}
	if isolated != nil {
		config.WarmGrace = *syntheticGrace
		config.ProgrammeBoundaryWitness = isolated.ProgrammeBoundaryWitness()
		config.FaultController = isolated
	}
	var isolatedTarget isolatedCloser
	if isolated != nil {
		isolatedTarget = isolated
	}
	report, err := playoutcert.Run(runCtx, config)
	if err != nil {
		if closeErr := closeIsolated(isolatedTarget, *cleanupTimeout); closeErr != nil {
			_, _ = fmt.Fprintln(stderr, "playout-load-cert: isolated target cleanup failed")
		}
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: run failed during bounded preflight")
		return 1
	}
	return finalizeAfterIsolatedCleanup(output, report, isolatedTarget, *cleanupTimeout, stdout, stderr)
}

type isolatedCloser interface {
	Close(context.Context) error
}

func finalizeAfterIsolatedCleanup(output artifactOutput, report playoutcert.Report, target isolatedCloser, timeout time.Duration, stdout, stderr io.Writer) int {
	if closeErr := closeIsolated(target, timeout); closeErr != nil {
		if err := playoutcert.DowngradePublication(&report, playoutcert.PublicationDowngradeCleanupFailed); err != nil {
			_, _ = fmt.Fprintln(stderr, "playout-load-cert: report finalization failed")
			return 1
		}
	}
	return publishReport(output, report, stdout, stderr)
}

func publishReport(output artifactOutput, report playoutcert.Report, stdout, stderr io.Writer) int {
	publication, err := playoutcert.FinalizePublication(report)
	if err != nil || !publication.Publishable() {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: report finalization failed")
		return 1
	}
	if err := writeArtifact(output, publication.JSON()); err != nil {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: report write failed")
		return 1
	}
	if written, err := stdout.Write(publication.Summary()); err != nil || written != len(publication.Summary()) {
		_, _ = fmt.Fprintln(stderr, "playout-load-cert: summary write failed")
		return 1
	}
	return publication.ExitStatus()
}

func closeIsolated(target isolatedCloser, timeout time.Duration) error {
	if target == nil {
		return nil
	}
	if synthetic, ok := target.(*app.PlayoutCertificationTarget); ok && synthetic == nil {
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
	const manifestLimit = 1 << 20
	contents, err := io.ReadAll(io.LimitReader(file, manifestLimit+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > manifestLimit {
		return nil, errors.New("manifest exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
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

type artifactOutput struct {
	root *os.Root
	path string
}

func (output artifactOutput) Close() error { return output.root.Close() }

func openContainedOutput(root, output string) (artifactOutput, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return artifactOutput{}, errors.New("artifact directory is invalid")
	}
	outputAbs, err := filepath.Abs(output)
	if err != nil {
		return artifactOutput{}, errors.New("output path is invalid")
	}
	rel, err := filepath.Rel(rootAbs, outputAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return artifactOutput{}, errors.New("output must remain under LOOMARR_ARTIFACT_DIR")
	}
	boundRoot, err := os.OpenRoot(root)
	if err != nil {
		return artifactOutput{}, errors.New("artifact directory is invalid")
	}
	return artifactOutput{root: boundRoot, path: rel}, nil
}

func writeArtifact(output artifactOutput, blob []byte) error {
	dir := filepath.Dir(output.path)
	if err := output.root.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var random [16]byte
	for range 10 {
		if _, err := rand.Read(random[:]); err != nil {
			return err
		}
		temporaryPath := filepath.Join(dir, ".playout-load-cert-"+hex.EncodeToString(random[:]))
		temporary, err := output.root.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		defer func() { _ = output.root.Remove(temporaryPath) }()
		if _, err = temporary.Write(blob); err == nil {
			err = temporary.Close()
		} else {
			_ = temporary.Close()
		}
		if err != nil {
			return err
		}
		return output.root.Rename(temporaryPath, output.path)
	}
	return errors.New("artifact temporary name collision")
}
