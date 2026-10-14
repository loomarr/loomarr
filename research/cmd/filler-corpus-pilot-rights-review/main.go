// Command filler-corpus-pilot-rights-review prepares an inert, deterministic
// independent-review packet from the exact locked source-yield pilot.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-pilot-rights-review", flag.ContinueOnError)
	flags.SetOutput(stderr)
	pilotPath := flags.String("pilot", "", "locked source-yield pilot JSON")
	outputPath := flags.String("out", "", "inert review worksheet JSON")
	csvPath := flags.String("csv-out", "", "spreadsheet-safe inert review worksheet CSV")
	preparedAtText := flags.String("prepared-at", "", "worksheet preparation time in RFC3339 format")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *pilotPath == "" || *outputPath == "" || *csvPath == "" || *preparedAtText == "" {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-review: pilot, JSON output, CSV output, and preparation time are required")
		return 2
	}
	preparedAt, err := time.Parse(time.RFC3339, *preparedAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-review: parse --prepared-at:", err)
		return 2
	}
	pilotRaw, err := os.ReadFile(*pilotPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-review: read pilot:", err)
		return 1
	}
	sheet, err := fillercorpus.PreparePilotReview(pilotRaw, preparedAt)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-review:", err)
		return 1
	}
	jsonRaw, err := json.MarshalIndent(sheet, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-review: encode worksheet:", err)
		return 1
	}
	csvRaw, err := fillercorpus.PilotReviewCSV(sheet)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-review: encode CSV:", err)
		return 1
	}
	if err := writeAtomic(*outputPath, append(jsonRaw, '\n')); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-review: write worksheet:", err)
		return 1
	}
	if err := writeAtomic(*csvPath, csvRaw); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-review: write CSV:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-pilot-rights-review: prepared %d inert rows for pilot %s\n", len(sheet.Cases), sheet.PilotSHA256)
	return 0
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-pilot-rights-review-*")
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
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	ok = true
	return nil
}
