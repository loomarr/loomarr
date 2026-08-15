// Command image-cert certifies the required Rust image worker against either Loomarr's
// deterministic corpus or an operator-supplied read-only raster directory (§22 V59a).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/images/rustgen"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("image-cert", flag.ContinueOnError)
	flags.SetOutput(stderr)
	corpus := flags.String("corpus", os.Getenv("IMAGE_CERT_CORPUS"), "absolute read-only raster corpus directory")
	reportPath := flags.String("report", os.Getenv("IMAGE_CERT_REPORT"), "JSON report path")
	worker := flags.String("worker", os.Getenv("LOOMARR_IMAGE_WORKER"), "loomarr-image executable")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *reportPath == "" {
		_, _ = fmt.Fprintln(stderr, "image-cert: --report or IMAGE_CERT_REPORT is required")
		return 2
	}
	if *worker == "" {
		*worker = filepath.Join("bin", "loomarr-image")
	}
	absoluteWorker, err := filepath.Abs(*worker)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "image-cert: worker path: %v\n", err)
		return 2
	}

	release := os.Getenv("LOOMARR_RELEASE")
	if release == "" {
		release = "dev"
	}
	renderer, err := rustgen.Open(absoluteWorker, rustgen.Contract{
		Protocol: 1, Release: release, Recipe: "loomarr-rendition-v1",
		RequiredFormats: []string{"avif", "jpeg", "webp"}, Animation: true,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "image-cert: %v\n", err)
		return 1
	}

	expected := map[string]string(nil)
	var boundaries []images.CertificationBoundary
	if *corpus == "" {
		generated, tempErr := os.MkdirTemp("", "loomarr-image-cert-corpus-*")
		if tempErr != nil {
			_, _ = fmt.Fprintf(stderr, "image-cert: create repository corpus: %v\n", tempErr)
			return 1
		}
		defer func() { _ = os.RemoveAll(generated) }()
		manifest, writeErr := images.WriteCertificationCorpus(generated)
		if writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "image-cert: generate repository corpus: %v\n", writeErr)
			return 1
		}
		*corpus = generated
		expected = manifest.ExpectedRefusals
		boundaries = manifest.BoundaryCases
	} else if !filepath.IsAbs(*corpus) {
		_, _ = fmt.Fprintln(stderr, "image-cert: --corpus must be an absolute path")
		return 2
	}

	certCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	report, certErr := images.Certify(certCtx, images.CertificationOptions{
		CorpusDir: *corpus, Renderer: renderer, Limits: images.DefaultCertificationLimits(),
		ExpectedRefusals: expected,
		BoundaryCases:    boundaries,
	})
	if err := writeReport(*reportPath, report); err != nil {
		_, _ = fmt.Fprintf(stderr, "image-cert: write report: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "image-cert: %d passed, %d refused, %d failed; report %s\n",
		report.Summary.Passed, report.Summary.Refused, report.Summary.Failed, *reportPath)
	if certErr != nil {
		_, _ = fmt.Fprintf(stderr, "image-cert: %v\n", certErr)
		return 1
	}
	return 0
}

func writeReport(path string, report images.CertificationReport) error {
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
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".image-cert-*.json")
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
