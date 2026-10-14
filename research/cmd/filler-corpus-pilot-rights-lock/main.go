// Command filler-corpus-pilot-rights-lock validates a completed independent
// pilot review and emits a non-authorizing source-yield qualification report.
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
	flags := flag.NewFlagSet("filler-corpus-pilot-rights-lock", flag.ContinueOnError)
	flags.SetOutput(stderr)
	pilotPath := flags.String("pilot", "", "locked source-yield pilot JSON")
	worksheetPath := flags.String("worksheet", "", "inert pilot review worksheet JSON")
	csvPath := flags.String("completed-csv", "", "completed independent review CSV")
	outputPath := flags.String("out", "", "non-authorizing source-yield qualification report JSON")
	lockedAtText := flags.String("locked-at", "", "review lock time in RFC3339 format")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *pilotPath == "" || *worksheetPath == "" || *csvPath == "" || *outputPath == "" || *lockedAtText == "" {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-lock: pilot, worksheet, completed CSV, output, and lock time are required")
		return 2
	}
	lockedAt, err := time.Parse(time.RFC3339, *lockedAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-lock: parse --locked-at:", err)
		return 2
	}
	pilotRaw, err := os.ReadFile(*pilotPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-lock: read pilot:", err)
		return 1
	}
	worksheetRaw, err := os.ReadFile(*worksheetPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-lock: read worksheet:", err)
		return 1
	}
	csvRaw, err := os.ReadFile(*csvPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-lock: read completed CSV:", err)
		return 1
	}
	result, err := fillercorpus.LockPilotReview(pilotRaw, worksheetRaw, csvRaw, lockedAt)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-lock:", err)
		return 1
	}
	resultRaw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-lock: encode result:", err)
		return 1
	}
	if err := writeAtomic(*outputPath, append(resultRaw, '\n')); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot-rights-lock: write result:", err)
		return 1
	}
	qualified := 0
	for _, lane := range result.Lanes {
		if lane.QualifiedForAdapter {
			qualified++
		}
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-pilot-rights-lock: locked %d decisions; %d of %d lanes qualify; download authority remains false\n", len(result.Decisions), qualified, len(result.Lanes))
	return 0
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-pilot-rights-lock-*")
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
