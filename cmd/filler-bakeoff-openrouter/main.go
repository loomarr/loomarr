// Command filler-bakeoff-openrouter captures a bounded, label-blind prediction
// ledger from one locked certification corpus. Scoring remains a separate replay.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

const bakeoffConfigSchemaVersion = 1

type bakeoffConfigFile struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Run           fillereval.RunIdentity `json:"run"`
	Policy        filleradmission.Policy `json:"policy"`
	Routes        []fillerbakeoff.Route  `json:"routes"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-bakeoff-openrouter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "locked certification manifest JSON")
	packetsPath := flags.String("packets", "", "label-blind evidence packets JSONL")
	configPath := flags.String("config", "", "versioned bakeoff route and policy JSON")
	corpusRoot := flags.String("corpus-root", "", "root containing packet media derivatives")
	predictionsPath := flags.String("predictions", "", "output immutable predictions JSONL")
	baseURL := flags.String("base-url", fillerbakeoff.OpenRouterBaseURL, "OpenRouter API base URL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *manifestPath == "" || *packetsPath == "" || *configPath == "" || *corpusRoot == "" || *predictionsPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-bakeoff-openrouter: --manifest, --packets, --config, --corpus-root, and --predictions are required")
		return 2
	}
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		_, _ = fmt.Fprintln(stderr, "filler-bakeoff-openrouter: OPENROUTER_API_KEY is required")
		return 2
	}
	manifest, err := readStrictJSON[fillereval.Manifest](*manifestPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: read manifest: %v\n", err)
		return 1
	}
	configFile, err := readStrictJSON[bakeoffConfigFile](*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: read config: %v\n", err)
		return 1
	}
	if configFile.SchemaVersion != bakeoffConfigSchemaVersion {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: config schema %d, want %d\n", configFile.SchemaVersion, bakeoffConfigSchemaVersion)
		return 1
	}
	if configFile.Run.PromptVersion != fillerbakeoff.OpenRouterPromptVersion {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: prompt version %q, want %q\n", configFile.Run.PromptVersion, fillerbakeoff.OpenRouterPromptVersion)
		return 1
	}
	packets, err := readPackets(*packetsPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: read packets: %v\n", err)
		return 1
	}
	policy, err := filleradmission.New(configFile.Policy)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: admission policy: %v\n", err)
		return 1
	}
	extractor, err := fillerbakeoff.NewOpenRouterExtractor(fillerbakeoff.OpenRouterConfig{BaseURL: *baseURL, APIKey: apiKey})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: provider adapter: %v\n", err)
		return 1
	}
	predictions, err := fillerbakeoff.Run(context.Background(), fillerbakeoff.Config{
		Run: configFile.Run, Manifest: manifest, Packets: packets, CorpusRoot: *corpusRoot,
		Policy: policy, Routes: configFile.Routes, Extractor: extractor,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: run: %v\n", err)
		return 1
	}
	if err := writePredictions(*predictionsPath, predictions); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: write predictions: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-bakeoff-openrouter: captured %d label-blind predictions in %s\n", len(predictions), *predictionsPath)
	return 0
}

func readStrictJSON[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return value, fmt.Errorf("trailing JSON value")
		}
		return value, err
	}
	return value, nil
}

func readPackets(path string) (map[string]fillerbakeoff.Packet, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	packets := make(map[string]fillerbakeoff.Packet)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		packet, err := fillerbakeoff.DecodePacket(bytes.NewReader(scanner.Bytes()))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if _, exists := packets[packet.CaseID]; exists {
			return nil, fmt.Errorf("line %d: duplicate case %q", line, packet.CaseID)
		}
		packets[packet.CaseID] = packet
	}
	return packets, scanner.Err()
}

func writePredictions(path string, predictions []fillereval.Prediction) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".filler-bakeoff-*.jsonl")
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
	encoder := json.NewEncoder(temp)
	encoder.SetEscapeHTML(false)
	for _, prediction := range predictions {
		if err := encoder.Encode(prediction); err != nil {
			return err
		}
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Link(tempName, absolute); err != nil {
		return fmt.Errorf("publish immutable prediction ledger: %w", err)
	}
	if err := os.Remove(tempName); err != nil {
		_ = os.Remove(absolute)
		return err
	}
	ok = true
	return nil
}
