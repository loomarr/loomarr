// Command filler-corpus-review prepares one independently randomized semantic
// review batch and its private alias map. It never reads labels or calls a provider.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-review", flag.ContinueOnError)
	flags.SetOutput(stderr)
	draftPath := flags.String("draft", "", "unlabeled certification draft JSON")
	batchID := flags.String("batch-id", "", "unique opaque review batch identity")
	packetPath := flags.String("packet-out", "", "reviewer-visible blind packet JSON")
	mapPath := flags.String("map-out", "", "private coordinator alias map JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *draftPath == "" || *batchID == "" || *packetPath == "" || *mapPath == "" || *packetPath == *mapPath {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-review: distinct --draft, --batch-id, --packet-out, and --map-out are required")
		return 2
	}
	for _, output := range []string{*packetPath, *mapPath} {
		if _, err := os.Lstat(output); err == nil || !os.IsNotExist(err) {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-review: output already exists:", output)
			return 1
		}
	}
	draft, err := readDraft(*draftPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-review: read draft:", err)
		return 1
	}
	packet, mapping, failures := fillereval.PrepareBlindReview(draft, *batchID, nil)
	if len(failures) > 0 {
		for _, failure := range failures {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-review:", failure)
		}
		return 1
	}
	if err := writeJSON(*mapPath, mapping, 0o600); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-review: write private map:", err)
		return 1
	}
	if err := writeJSON(*packetPath, packet, 0o640); err != nil {
		_ = os.Remove(*mapPath)
		_, _ = fmt.Fprintln(stderr, "filler-corpus-review: write packet:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-review: prepared %d opaque cases for batch %s\n", len(packet.Cases), packet.BatchID)
	return 0
}

func readDraft(path string) (fillereval.Manifest, error) {
	var draft fillereval.Manifest
	file, err := os.Open(path)
	if err != nil {
		return draft, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil {
		return draft, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return draft, fmt.Errorf("trailing JSON value")
		}
		return draft, err
	}
	return draft, nil
}

func writeJSON(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".filler-corpus-review-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer func() { _ = temp.Close(); _ = os.Remove(name) }()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
