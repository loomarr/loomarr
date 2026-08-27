// Command filler-bakeoff-openrouter captures a bounded, label-blind prediction
// ledger from one locked certification corpus. Scoring remains a separate replay.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/loomarr/loomarr/cmd/internal/fillerbakeoffio"
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
	snapshotPath := flags.String("snapshot", "", "locked OpenRouter capability, price, and ZDR snapshot JSON")
	corpusRoot := flags.String("corpus-root", "", "root containing packet media derivatives")
	transcriptsPath := flags.String("transcripts", "", "locked shared transcript JSONL when named by the run")
	predictionsPath := flags.String("predictions", "", "output immutable predictions JSONL")
	baseURL := flags.String("base-url", fillerbakeoff.OpenRouterBaseURL, "OpenRouter API base URL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *manifestPath == "" || *packetsPath == "" || *configPath == "" || *snapshotPath == "" || *corpusRoot == "" || *predictionsPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-bakeoff-openrouter: --manifest, --packets, --config, --snapshot, --corpus-root, and --predictions are required")
		return 2
	}
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		_, _ = fmt.Fprintln(stderr, "filler-bakeoff-openrouter: OPENROUTER_API_KEY is required")
		return 2
	}
	manifest, err := fillerbakeoffio.ReadStrictJSON[fillereval.Manifest](*manifestPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: read manifest: %v\n", err)
		return 1
	}
	configFile, err := fillerbakeoffio.ReadStrictJSON[bakeoffConfigFile](*configPath)
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
	snapshot, err := fillerbakeoffio.ReadStrictJSON[fillerbakeoff.OpenRouterSnapshot](*snapshotPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: read snapshot: %v\n", err)
		return 1
	}
	packets, err := fillerbakeoffio.ReadPackets(*packetsPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: read packets: %v\n", err)
		return 1
	}
	transcripts, err := readRunTranscripts(configFile.Run, *transcriptsPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: read transcripts: %v\n", err)
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
		Run: configFile.Run, Manifest: manifest, Packets: packets, Transcripts: transcripts, CorpusRoot: *corpusRoot,
		Policy: policy, Routes: configFile.Routes, Extractor: extractor, Snapshot: &snapshot,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: run: %v\n", err)
		return 1
	}
	if err := fillerbakeoffio.WritePredictions(*predictionsPath, ".filler-bakeoff-*.jsonl", predictions); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-openrouter: write predictions: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-bakeoff-openrouter: captured %d label-blind predictions in %s\n", len(predictions), *predictionsPath)
	return 0
}

func readRunTranscripts(run fillereval.RunIdentity, path string) ([]fillerbakeoff.TranscriptArtifact, error) {
	if run.TranscriptSetSHA256 == "" {
		if path != "" {
			return nil, fmt.Errorf("transcript file supplied but run names no transcript set")
		}
		return nil, nil
	}
	if path == "" {
		return nil, fmt.Errorf("run names transcript set %s but --transcripts is missing", run.TranscriptSetSHA256)
	}
	return fillerbakeoffio.ReadTranscripts(path)
}
