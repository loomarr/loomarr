// Command filler-reference-measure performs full-source technical measurement
// of the bound inspection seed. It does not edit media or assign grades.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerreference"
	"github.com/loomarr/loomarr/internal/mediatools"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-reference-measure", flag.ContinueOnError)
	flags.SetOutput(stderr)
	auditPath := flags.String("audit", "", "bound filler reference audit")
	familyPath := flags.String("families", "", "bound duplicate-family audit")
	seedPath := flags.String("seed", "", "bound 50-source inspection seed")
	sourceRoot := flags.String("source-root", "", "root containing acquired source files")
	ffmpegPath := flags.String("ffmpeg", "", "ffmpeg executable")
	ffprobePath := flags.String("ffprobe", "", "ffprobe executable")
	outputPath := flags.String("output", "", "new immutable measurement JSON")
	generatedText := flags.String("generated-at", "", "fixed RFC3339 measurement time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	generatedAt, err := time.Parse(time.RFC3339, *generatedText)
	if err != nil || *auditPath == "" || *familyPath == "" || *seedPath == "" || *sourceRoot == "" || *ffmpegPath == "" || *ffprobePath == "" || *outputPath == "" || flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "filler-reference-measure: audit, families, seed, source-root, ffmpeg, ffprobe, output, and fixed generated-at are required")
		return 2
	}
	auditRaw, err := os.ReadFile(*auditPath)
	if err != nil {
		return fail(stderr, err)
	}
	familyRaw, err := os.ReadFile(*familyPath)
	if err != nil {
		return fail(stderr, err)
	}
	seedRaw, err := os.ReadFile(*seedPath)
	if err != nil {
		return fail(stderr, err)
	}
	plan, err := fillerreference.BuildInspectionPlan(fillerreference.RawInspectionInputs{Audit: auditRaw, Families: familyRaw, Seed: seedRaw})
	if err != nil {
		return fail(stderr, err)
	}
	if generatedAt.Before(plan.NotBefore) {
		return fail(stderr, fmt.Errorf("generated-at precedes a bound input artifact"))
	}
	tools := mediatools.NewFFmpegTools(*ffmpegPath, *ffprobePath, "", "", "")
	artifact := fillerreference.MeasurementArtifact{SchemaVersion: 1, GeneratedAt: generatedAt.UTC(), Inputs: plan.Inputs, Cases: make([]fillerreference.InspectionMeasurement, 0, len(plan.Cases))}
	for index, item := range plan.Cases {
		path, err := sourcePath(*sourceRoot, item.SourceLocalFile)
		if err != nil {
			return fail(stderr, fmt.Errorf("case %q: %w", item.CaseID, err))
		}
		digest, err := fileSHA256(path)
		if err != nil || digest != item.ContentSHA256 {
			return fail(stderr, fmt.Errorf("case %q source identity mismatch: got %q: %w", item.CaseID, digest, err))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		measurement, err := tools.MeasureConditioning(ctx, mediatools.ConditioningRequest{Path: path})
		cancel()
		if err != nil {
			artifact.Cases = append(artifact.Cases, fillerreference.InspectionMeasurement{
				CaseID: item.CaseID, ContentSHA256: item.ContentSHA256, LocalFile: item.SourceLocalFile,
				Status: "technical_hold", MeasurementError: err.Error(),
			})
			artifact.Summary.TechnicalHolds++
			_, _ = fmt.Fprintf(stdout, "filler-reference-measure: held %d/%d %s: %v\n", index+1, len(plan.Cases), item.CaseID, err)
			continue
		}
		artifact.Cases = append(artifact.Cases, fillerreference.InspectionMeasurement{CaseID: item.CaseID, ContentSHA256: item.ContentSHA256, LocalFile: item.SourceLocalFile, Status: "measured", Measurement: &measurement})
		artifact.Summary.Measured++
		_, _ = fmt.Fprintf(stdout, "filler-reference-measure: measured %d/%d %s\n", index+1, len(plan.Cases), item.CaseID)
	}
	artifact.Summary.Cases = len(artifact.Cases)
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fail(stderr, err)
	}
	if err := publish(*outputPath, append(data, '\n')); err != nil {
		return fail(stderr, err)
	}
	return 0
}

func sourcePath(root, local string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(local)))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local file escapes source root")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve source file: %w", err)
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local file escapes source root through symlink")
	}
	return resolvedPath, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func publish(path string, data []byte) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".filler-reference-measure-*")
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
	if err := temp.Chmod(0o640); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Link(tempName, absolute); err != nil {
		return fmt.Errorf("publish immutable measurement: %w", err)
	}
	if err := os.Remove(tempName); err != nil {
		_ = os.Remove(absolute)
		return err
	}
	ok = true
	return nil
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "filler-reference-measure:", err)
	return 1
}
