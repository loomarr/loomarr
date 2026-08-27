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

type laneFiles []string

func (f *laneFiles) String() string { return strings.Join(*f, ",") }
func (f *laneFiles) Set(value string) error {
	if value == "" {
		return fmt.Errorf("lane path cannot be empty")
	}
	*f = append(*f, value)
	return nil
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-pilot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var inputs laneFiles
	flags.Var(&inputs, "lane", "source-neutral lane JSON; repeat once per qualified authority")
	output := flags.String("out", "", "locked pilot JSON")
	snapshotAtText := flags.String("snapshot-at", "", "pilot snapshot time in RFC3339 format")
	lockedAtText := flags.String("locked-at", "", "pilot lock time in RFC3339 format")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(inputs) != len(fillercorpus.PilotAuthorities) || *output == "" || *snapshotAtText == "" || *lockedAtText == "" {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-pilot: exactly %d lanes, output, snapshot time, and lock time are required\n", len(fillercorpus.PilotAuthorities))
		return 2
	}
	snapshotAt, err := time.Parse(time.RFC3339, *snapshotAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot: parse --snapshot-at:", err)
		return 2
	}
	lockedAt, err := time.Parse(time.RFC3339, *lockedAtText)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot: parse --locked-at:", err)
		return 2
	}
	pilot := fillercorpus.Pilot{SchemaVersion: fillercorpus.PilotSchemaVersion, SnapshotAt: snapshotAt.UTC(), LockedAt: lockedAt.UTC()}
	for _, input := range inputs {
		lane, err := readLane(input)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "filler-corpus-pilot: decode %s: %v\n", input, err)
			return 1
		}
		pilot.Lanes = append(pilot.Lanes, lane)
	}
	if failures := fillercorpus.ValidatePilot(pilot); len(failures) > 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot:", strings.Join(failures, "; "))
		return 1
	}
	if err := writeJSON(*output, pilot); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-pilot: write:", err)
		return 1
	}
	var candidates int
	for _, lane := range pilot.Lanes {
		candidates += len(lane.Cases)
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-pilot: locked %d non-authorizing candidates across %d lanes\n", candidates, len(pilot.Lanes))
	return 0
}

func readLane(filename string) (fillercorpus.Lane, error) {
	file, err := os.Open(filename)
	if err != nil {
		return fillercorpus.Lane{}, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, 16<<20))
	decoder.DisallowUnknownFields()
	var lane fillercorpus.Lane
	if err := decoder.Decode(&lane); err != nil {
		return fillercorpus.Lane{}, err
	}
	if err := rejectTrailing(decoder); err != nil {
		return fillercorpus.Lane{}, err
	}
	return lane, nil
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
