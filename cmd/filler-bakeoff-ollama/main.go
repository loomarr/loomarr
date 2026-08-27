// Command filler-bakeoff-ollama captures a bounded, label-blind prediction
// ledger from one digest-pinned local model. Scoring remains a separate replay.
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
	ModelDigest   string                 `json:"modelDigest"`
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-bakeoff-ollama", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "locked certification manifest JSON")
	packetsPath := flags.String("packets", "", "label-blind evidence packets JSONL")
	configPath := flags.String("config", "", "versioned local route and policy JSON")
	corpusRoot := flags.String("corpus-root", "", "root containing packet media derivatives")
	transcriptsPath := flags.String("transcripts", "", "locked shared transcript JSONL when named by the run")
	predictionsPath := flags.String("predictions", "", "output immutable predictions JSONL")
	baseURL := flags.String("base-url", "http://127.0.0.1:11434", "loopback Ollama API base URL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *manifestPath == "" || *packetsPath == "" || *configPath == "" || *corpusRoot == "" || *predictionsPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-bakeoff-ollama: --manifest, --packets, --config, --corpus-root, and --predictions are required")
		return 2
	}
	manifest, err := fillerbakeoffio.ReadStrictJSON[fillereval.Manifest](*manifestPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-ollama: read manifest: %v\n", err)
		return 1
	}
	configFile, err := fillerbakeoffio.ReadStrictJSON[bakeoffConfigFile](*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-ollama: read config: %v\n", err)
		return 1
	}
	if configFile.SchemaVersion != bakeoffConfigSchemaVersion || configFile.Run.PromptVersion != fillerbakeoff.OllamaPromptVersion || len(configFile.Routes) != 1 {
		_, _ = fmt.Fprintln(stderr, "filler-bakeoff-ollama: config requires schema 1, the exact Ollama prompt version, and one route")
		return 1
	}
	packets, err := fillerbakeoffio.ReadPackets(*packetsPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-ollama: read packets: %v\n", err)
		return 1
	}
	transcripts, err := readRunTranscripts(configFile.Run, *transcriptsPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-ollama: read transcripts: %v\n", err)
		return 1
	}
	policy, err := filleradmission.New(configFile.Policy)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-ollama: admission policy: %v\n", err)
		return 1
	}
	extractor, err := fillerbakeoff.NewOllamaExtractor(context.Background(), fillerbakeoff.OllamaConfig{
		BaseURL: *baseURL, Model: configFile.Routes[0].Model, ModelDigest: configFile.ModelDigest,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-ollama: provider adapter: %v\n", err)
		return 1
	}
	predictions, err := fillerbakeoff.Run(context.Background(), fillerbakeoff.Config{
		Run: configFile.Run, Manifest: manifest, Packets: packets, Transcripts: transcripts, CorpusRoot: *corpusRoot,
		Policy: policy, Routes: configFile.Routes, Extractor: extractor,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-ollama: run: %v\n", err)
		return 1
	}
	if err := fillerbakeoffio.WritePredictions(*predictionsPath, ".filler-bakeoff-ollama-*", predictions); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-ollama: write predictions: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-bakeoff-ollama: captured %d label-blind predictions in %s\n", len(predictions), *predictionsPath)
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
