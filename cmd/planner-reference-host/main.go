package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/plannerreference"
)

const (
	maxInputFileBytes = 16 << 20
	maxEvidenceFiles  = 16
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("planner-reference-host", flag.ContinueOnError)
	flags.SetOutput(stderr)
	scorecardPath := flags.String("scorecard", "", "exact planner scorecard JSON")
	capturePath := flags.String("capture", "", "normalized reference-host capture JSON")
	evidenceDir := flags.String("evidence-dir", "", "directory containing the fixed raw evidence files")
	generatedAtRaw := flags.String("generated-at", "", "publication time in RFC3339")
	outPath := flags.String("out", "", "new immutable manifest path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *scorecardPath == "" || *capturePath == "" || *evidenceDir == "" ||
		*generatedAtRaw == "" || *outPath == "" {
		_, _ = fmt.Fprintln(stderr, "planner-reference-host: --scorecard, --capture, --evidence-dir, --generated-at, and --out are required")
		return 2
	}
	generatedAt, err := time.Parse(time.RFC3339Nano, *generatedAtRaw)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "planner-reference-host: parse generated-at: %v\n", err)
		return 2
	}
	scorecard, err := readBoundedFile(*scorecardPath, maxInputFileBytes)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "planner-reference-host: read scorecard: %v\n", err)
		return 1
	}
	capture, err := readBoundedFile(*capturePath, maxInputFileBytes)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "planner-reference-host: read capture: %v\n", err)
		return 1
	}
	evidence, err := readEvidenceDirectory(*evidenceDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "planner-reference-host: read evidence: %v\n", err)
		return 1
	}
	artifact, err := plannerreference.BuildManifest(plannerreference.RawInputs{
		Scorecard: scorecard, Capture: capture, Evidence: evidence, GeneratedAt: generatedAt,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "planner-reference-host: build manifest: %v\n", err)
		return 1
	}
	if err := publishImmutable(*outPath, artifact.JSON); err != nil {
		_, _ = fmt.Fprintf(stderr, "planner-reference-host: publish: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "planner-reference-host: wrote %s sha256=%s\n", *outPath, artifact.SHA256)
	return 0
}

func readEvidenceDirectory(root string) (map[string][]byte, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 || len(entries) > maxEvidenceFiles {
		return nil, fmt.Errorf("evidence file count = %d, want 1..%d", len(entries), maxEvidenceFiles)
	}
	result := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil, fmt.Errorf("%q is not a regular non-symlink file", entry.Name())
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("inspect %q: %w", entry.Name(), infoErr)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%q is not a regular file", entry.Name())
		}
		raw, readErr := readBoundedFile(filepath.Join(root, entry.Name()), maxInputFileBytes)
		if readErr != nil {
			return nil, fmt.Errorf("read %q: %w", entry.Name(), readErr)
		}
		result[entry.Name()] = raw
	}
	return result, nil
}

func readBoundedFile(path string, limit int64) (raw []byte, resultErr error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("close input: %w", closeErr)
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > limit {
		return nil, fmt.Errorf("file must be regular and 1..%d bytes", limit)
	}
	raw, err = io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("file exceeds %d-byte ceiling", limit)
	}
	return raw, nil
}

func publishImmutable(path string, raw []byte) error {
	if path == "" || len(raw) == 0 {
		return errors.New("output path and bytes are required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	cleanup := func(cause error) error {
		_ = file.Close()
		_ = os.Remove(path)
		return cause
	}
	if _, err := file.Write(raw); err != nil {
		return cleanup(err)
	}
	if err := file.Sync(); err != nil {
		return cleanup(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
