// Command filler-temporal-truth-prepare materializes the sealed 48-case
// complete-span evidence set. It performs no semantic inference.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-temporal-truth-prepare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	selection := flags.String("selection", "", "coordinator-private 48-case selection JSON")
	draft := flags.String("draft", "", "frozen 300-case draft JSON")
	ledger := flags.String("download-ledger", "", "frozen verified download ledger JSON")
	mediaRoot := flags.String("media-root", "", "root containing original acquired media")
	packets := flags.String("packets", "", "frozen raw evidence packet JSONL")
	packetRoot := flags.String("packet-root", "", "root containing raw packet derivatives")
	transcripts := flags.String("transcripts", "", "content-bound transcript JSONL")
	output := flags.String("out", "", "new evidence-set directory")
	generatedText := flags.String("generated-at", "", "fixed RFC3339 generation time")
	ffmpeg := flags.String("ffmpeg", "ffmpeg", "ffmpeg executable")
	ffprobe := flags.String("ffprobe", "ffprobe", "ffprobe executable")
	sceneThreshold := flags.Float64("scene-threshold", 0.30, "scene-change score threshold")
	perCaseTimeout := flags.Duration("per-case-timeout", 5*time.Minute, "hard timeout for one selected case")
	ocrEngine := flags.String("ocr-engine", "", "optional reviewed local OCR executable")
	ocrSource := flags.String("ocr-source", "", "source file for the optional OCR executable")
	ocrVersion := flags.String("ocr-version", "", "fixed optional OCR implementation version")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	generatedAt, err := time.Parse(time.RFC3339, *generatedText)
	if err != nil || *selection == "" || *draft == "" || *ledger == "" || *mediaRoot == "" || *packets == "" || *packetRoot == "" || *transcripts == "" || *output == "" || *sceneThreshold <= 0 || *sceneThreshold >= 1 || *perCaseTimeout <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-truth-prepare: every evidence path, fixed generation time, positive timeout, and scene threshold between zero and one are required")
		return 2
	}
	if (*ocrEngine == "") != (*ocrSource == "") || (*ocrEngine == "") != (*ocrVersion == "") {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-truth-prepare: --ocr-engine, --ocr-source, and --ocr-version must be supplied together")
		return 2
	}
	ctx := context.Background()
	media, err := fillerreview.NewFFmpegTemporalTruthMedia(ctx, *ffmpeg, *ffprobe)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-truth-prepare: media tools:", err)
		return 1
	}
	var ocr fillerreview.TemporalTruthEvidenceOCR
	if *ocrEngine != "" {
		ocr, err = fillerreview.NewExecTemporalTruthOCR(*ocrEngine, *ocrSource, *ocrVersion)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "filler-temporal-truth-prepare: OCR:", err)
			return 1
		}
	}
	manifest, err := fillerreview.BuildTemporalTruthEvidence(ctx, fillerreview.TemporalTruthEvidenceConfig{
		SelectionPath: *selection, DraftPath: *draft, DownloadLedgerPath: *ledger, MediaRoot: *mediaRoot,
		PacketsPath: *packets, PacketRoot: *packetRoot, TranscriptsPath: *transcripts, OutputDir: *output,
		GeneratedAt: generatedAt.UTC(), SceneThreshold: *sceneThreshold, PerCaseTimeout: *perCaseTimeout,
		Media: media, OCR: ocr,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-truth-prepare:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-truth-prepare: wrote %d cases to %s\n", len(manifest.Cases), filepath.Join(*output, "public"))
	return 0
}
