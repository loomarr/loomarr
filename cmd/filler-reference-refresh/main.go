// Command filler-reference-refresh performs the one-way, offline v3/v1 to
// v4/v2 refresh for the retained 300-case reference corpus.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/fillerreference"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-reference-refresh", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "locked v3 development manifest")
	packetsPath := flags.String("packets", "", "v1 evidence packet JSONL")
	mappingPath := flags.String("mapping", "", "manifest-bound product mapping")
	reviewPath := flags.String("content-review", "", "v3 negative content review")
	outputPath := flags.String("output", "", "new output directory")
	refreshedText := flags.String("refreshed-at", "", "fixed RFC3339 refresh time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	refreshedAt, err := time.Parse(time.RFC3339, *refreshedText)
	if err != nil || *manifestPath == "" || *packetsPath == "" || *mappingPath == "" || *reviewPath == "" || *outputPath == "" || flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "filler-reference-refresh: manifest, packets, mapping, content-review, output, and fixed refreshed-at are required")
		return 2
	}
	manifest, err := os.ReadFile(*manifestPath)
	if err != nil {
		return fail(stderr, err)
	}
	packets, err := os.ReadFile(*packetsPath)
	if err != nil {
		return fail(stderr, err)
	}
	mapping, err := os.ReadFile(*mappingPath)
	if err != nil {
		return fail(stderr, err)
	}
	review, err := os.ReadFile(*reviewPath)
	if err != nil {
		return fail(stderr, err)
	}
	result, err := fillerreference.Refresh(fillerreference.RawRefreshInputs{Manifest: manifest, Packets: packets, Mapping: mapping, ContentReview: review}, refreshedAt)
	if err != nil {
		return fail(stderr, err)
	}
	report, err := json.MarshalIndent(result.Report, "", "  ")
	if err != nil {
		return fail(stderr, err)
	}
	files := map[string][]byte{
		"manifest.json": result.Manifest, "packets.jsonl": result.Packets,
		"mapping.json": result.Mapping, "content-review.json": result.ContentReview,
		"refresh-report.json": append(report, '\n'),
	}
	if err := publishBundle(*outputPath, files); err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "filler-reference-refresh: refreshed %d cases and removed %d retired licensing facts; report sha256 %s\n", result.Report.Denominators.Cases, result.Report.Denominators.RemovedSourceLicenseFacts, fillerreference.SHA256(files["refresh-report.json"]))
	return 0
}

func publishBundle(path string, files map[string][]byte) error {
	target, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("output already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".filler-reference-refresh-*")
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := os.Chmod(stage, 0o700); err != nil {
		return err
	}
	for name, raw := range files {
		file, err := os.OpenFile(filepath.Join(stage, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := file.Write(raw); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, target); err != nil {
		return err
	}
	published = true
	return nil
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "filler-reference-refresh:", err)
	return 1
}
