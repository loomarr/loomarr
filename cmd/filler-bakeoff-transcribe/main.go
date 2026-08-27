// Command filler-bakeoff-transcribe captures one immutable, shared transcript
// set from exact packet WAVs and a digest-pinned whisper.cpp engine.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/loomarr/loomarr/cmd/internal/fillerbakeoffio"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

const transcriptConfigSchemaVersion = 1

type transcriptConfigFile struct {
	SchemaVersion         int              `json:"schemaVersion"`
	EvaluationSplit       fillereval.Split `json:"evaluationSplit"`
	EvidenceVersion       string           `json:"evidenceVersion"`
	GeneratedAt           time.Time        `json:"generatedAt"`
	PerCaseTimeoutSeconds int              `json:"perCaseTimeoutSeconds"`
	ImplementationVersion string           `json:"implementationVersion"`
	BinarySHA256          string           `json:"binarySha256"`
	ModelSHA256           string           `json:"modelSha256"`
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-bakeoff-transcribe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "prepared manifest or draft JSON")
	packetsPath := flags.String("packets", "", "label-blind evidence packets JSONL")
	configPath := flags.String("config", "", "versioned transcript execution JSON")
	corpusRoot := flags.String("corpus-root", "", "root containing packet media derivatives")
	whisperPath := flags.String("whisper", "", "exact whisper-cli executable")
	modelPath := flags.String("model", "", "exact whisper.cpp model file")
	transcriptsPath := flags.String("transcripts", "", "new immutable transcript JSONL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *manifestPath == "" || *packetsPath == "" || *configPath == "" || *corpusRoot == "" || *whisperPath == "" || *modelPath == "" || *transcriptsPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-bakeoff-transcribe: --manifest, --packets, --config, --corpus-root, --whisper, --model, and --transcripts are required")
		return 2
	}
	manifest, err := fillerbakeoffio.ReadStrictJSON[fillereval.Manifest](*manifestPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-transcribe: read manifest: %v\n", err)
		return 1
	}
	packets, err := fillerbakeoffio.ReadPackets(*packetsPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-transcribe: read packets: %v\n", err)
		return 1
	}
	config, err := fillerbakeoffio.ReadStrictJSON[transcriptConfigFile](*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-transcribe: read config: %v\n", err)
		return 1
	}
	if config.SchemaVersion != transcriptConfigSchemaVersion || config.PerCaseTimeoutSeconds <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-bakeoff-transcribe: config requires schema 1 and a positive per-case timeout")
		return 1
	}
	engine, err := fillerbakeoff.NewWhisperTranscriptEngine(fillerbakeoff.WhisperTranscriptConfig{
		BinaryPath: *whisperPath, BinarySHA256: config.BinarySHA256, ImplementationVersion: config.ImplementationVersion,
		ModelPath: *modelPath, ModelSHA256: config.ModelSHA256,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-transcribe: speech engine: %v\n", err)
		return 1
	}
	transcripts, err := fillerbakeoff.BuildTranscripts(context.Background(), fillerbakeoff.TranscriptConfig{
		Manifest: manifest, Packets: packets, CorpusRoot: *corpusRoot, EvaluationSplit: config.EvaluationSplit,
		EvidenceVersion: config.EvidenceVersion, Engine: engine, GeneratedAt: config.GeneratedAt,
		PerCaseTimeout: time.Duration(config.PerCaseTimeoutSeconds) * time.Second,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-transcribe: build: %v\n", err)
		return 1
	}
	if err := fillerbakeoffio.WriteTranscripts(*transcriptsPath, transcripts); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-bakeoff-transcribe: write transcripts: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-bakeoff-transcribe: captured %d shared transcripts (set sha256 %s) in %s\n", len(transcripts), fillerbakeoff.TranscriptSetSHA256(transcripts), *transcriptsPath)
	return 0
}
