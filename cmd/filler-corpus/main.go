// Command filler-corpus locks two blind label batches into a certification
// manifest. It never reads media or contacts a source/provider.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus", flag.ContinueOnError)
	flags.SetOutput(stderr)
	draftPath := flags.String("draft", "", "draft manifest with media provenance and evidence hashes")
	firstPath := flags.String("review-a", "", "first independent label JSONL")
	secondPath := flags.String("review-b", "", "second independent label JSONL")
	adjudicationsPath := flags.String("adjudications", "", "third-party adjudication JSONL for disagreements")
	lockedAtText := flags.String("locked-at", "", "manifest lock time in RFC3339 format")
	outputPath := flags.String("out", "", "locked certification manifest JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *draftPath == "" || *firstPath == "" || *secondPath == "" || *lockedAtText == "" || *outputPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-corpus: --draft, --review-a, --review-b, --locked-at, and --out are required")
		return 2
	}
	lockedAt, err := time.Parse(time.RFC3339, *lockedAtText)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus: parse --locked-at: %v\n", err)
		return 2
	}
	draft, err := readJSON[fillereval.Manifest](*draftPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus: read draft: %v\n", err)
		return 1
	}
	first, err := readJSONL[fillereval.LabelSubmission](*firstPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus: read first review: %v\n", err)
		return 1
	}
	second, err := readJSONL[fillereval.LabelSubmission](*secondPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus: read second review: %v\n", err)
		return 1
	}
	var adjudications []fillereval.AdjudicationSubmission
	if *adjudicationsPath != "" {
		adjudications, err = readJSONL[fillereval.AdjudicationSubmission](*adjudicationsPath)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "filler-corpus: read adjudications: %v\n", err)
			return 1
		}
	}
	locked, failures := fillereval.LockReviewedManifest(draft, first, second, adjudications, lockedAt)
	if len(failures) > 0 {
		for _, failure := range failures {
			_, _ = fmt.Fprintln(stderr, "filler-corpus:", failure)
		}
		return 1
	}
	if err := writeJSON(*outputPath, locked); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus: write manifest: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus: locked %d cases as %s; manifest sha256 %s\n", len(locked.Cases), locked.CorpusVersion, fillereval.ManifestSHA256(locked))
	return 0
}

func readJSON[T any](path string) (T, error) {
	var value T
	file, err := os.Open(path)
	if err != nil {
		return value, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	if err := requireEOF(decoder); err != nil {
		return value, err
	}
	return value, nil
}

func readJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var values []T
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var value T
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if err := requireEOF(decoder); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		values = append(values, value)
	}
	return values, scanner.Err()
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func writeJSON(path string, value any) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".filler-corpus-*.json")
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
