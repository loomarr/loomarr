// Command filler-corpus-pilot locks a source-neutral, metadata-only rights-yield
// pilot. It performs no media download and grants no rights authority.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-pilot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("in", "", "source-neutral pilot draft JSON")
	output := flags.String("out", "", "locked pilot JSON")
	lockedAtText := flags.String("locked-at", "", "pilot lock time in RFC3339 format")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *input == "" || *output == "" || *lockedAtText == "" {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot: input, output, and lock time are required")
		return 2
	}
	lockedAt, err := time.Parse(time.RFC3339, *lockedAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot: parse --locked-at:", err)
		return 2
	}
	file, err := os.Open(*input)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot:", err)
		return 1
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, 16<<20))
	decoder.DisallowUnknownFields()
	var pilot fillercorpus.Pilot
	if err := decoder.Decode(&pilot); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot: decode:", err)
		return 1
	}
	if err := rejectTrailing(decoder); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot:", err)
		return 1
	}
	if !pilot.LockedAt.IsZero() {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot: draft cannot predeclare lock authority")
		return 1
	}
	pilot.LockedAt = lockedAt.UTC()
	if failures := fillercorpus.ValidatePilot(pilot); len(failures) > 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot:", strings.Join(failures, "; "))
		return 1
	}
	if err := writeJSON(*output, pilot); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot: write:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-pilot: locked %d non-authorizing candidates across %d lanes\n", 60, len(pilot.Lanes))
	return 0
}

func rejectTrailing(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	}
	return fmt.Errorf("input contains trailing JSON")
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-pilot-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temp)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
