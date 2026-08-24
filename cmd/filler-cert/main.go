// Command filler-cert scores captured filler-admission decisions against a
// versioned corpus. It is a replay tool and never contacts a model or media source.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-cert", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "versioned corpus manifest JSON")
	predictionsPath := flags.String("predictions", "", "captured predictions JSONL")
	reportPath := flags.String("report", "", "output report JSON")
	profile := flags.String("profile", "replay", "evaluation profile identifier")
	evidenceVersion := flags.String("evidence-version", "", "evidence extractor version")
	promptVersion := flags.String("prompt-version", "", "prompt/schema version")
	taxonomyVersion := flags.String("taxonomy-version", "", "taxonomy generation")
	policyVersion := flags.String("policy-version", "", "admission policy version")
	capabilitySnapshot := flags.String("capability-snapshot", "", "model capability snapshot identifier")
	priceSnapshot := flags.String("price-snapshot", "", "price snapshot identifier")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *manifestPath == "" || *predictionsPath == "" || *reportPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-cert: --manifest, --predictions, and --report are required")
		return 2
	}
	manifest, err := readManifest(*manifestPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-cert: read manifest: %v\n", err)
		return 1
	}
	predictions, err := readPredictions(*predictionsPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-cert: read predictions: %v\n", err)
		return 1
	}
	report := fillereval.Score(manifest, predictions, fillereval.RunIdentity{
		Profile: *profile, EvidenceVersion: *evidenceVersion, PromptVersion: *promptVersion,
		TaxonomyVersion: *taxonomyVersion, PolicyVersion: *policyVersion,
		CapabilitySnapshot: *capabilitySnapshot, PriceSnapshot: *priceSnapshot,
	})
	if err := writeJSON(*reportPath, report); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-cert: write report: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-cert: %d cases; admit precision %.4f (lower %.4f); review %.4f; report %s\n",
		report.Metrics.Cases, report.Metrics.AutoAdmitPrecision,
		report.Metrics.AutoAdmitPrecisionLower, report.Metrics.ReviewRate, *reportPath)
	if !report.Certified {
		for _, failure := range report.Failures {
			_, _ = fmt.Fprintln(stderr, "filler-cert:", failure)
		}
		return 1
	}
	return 0
}

func readManifest(path string) (fillereval.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fillereval.Manifest{}, err
	}
	var manifest fillereval.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fillereval.Manifest{}, err
	}
	return manifest, nil
}

func readPredictions(path string) ([]fillereval.Prediction, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var predictions []fillereval.Prediction
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var prediction fillereval.Prediction
		if err := json.Unmarshal(scanner.Bytes(), &prediction); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		predictions = append(predictions, prediction)
	}
	return predictions, scanner.Err()
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
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".filler-cert-*.json")
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
