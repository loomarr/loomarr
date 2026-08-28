// Command filler-corpus-review-ollama completes one independently shuffled
// blind semantic-review package with a digest-pinned local multimodal model.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/loomarr/loomarr/cmd/internal/fillerbakeoffio"
	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-review-ollama", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packageDir := flags.String("package", "", "identity-blind reviewer evidence package")
	transcriptsPath := flags.String("transcripts", "", "content-bound shared transcript JSONL")
	model := flags.String("model", "", "concrete local reviewer model tag")
	modelDigest := flags.String("model-digest", "", "installed model SHA-256 digest")
	reviewerID := flags.String("reviewer-id", "", "identity unique to this independent review")
	outputDir := flags.String("out", "", "new directory for labels and review attestation")
	baseURL := flags.String("base-url", "http://127.0.0.1:11434", "loopback Ollama API base URL")
	expectedCases := flags.Int("expected-cases", 300, "exact blind package case count")
	perCaseTimeout := flags.Duration("per-case-timeout", 5*time.Minute, "hard timeout for each serial review")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *packageDir == "" || *transcriptsPath == "" || *model == "" || *modelDigest == "" || *reviewerID == "" || *outputDir == "" {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-review-ollama: --package, --transcripts, --model, --model-digest, --reviewer-id, and --out are required")
		return 2
	}
	transcripts, err := fillerbakeoffio.ReadTranscripts(*transcriptsPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-ollama: read transcripts: %v\n", err)
		return 1
	}
	runEvidence, submissions, err := fillerreview.RunOllamaReview(context.Background(), fillerreview.OllamaReviewConfig{
		PackageDir: *packageDir, Transcripts: transcripts, BaseURL: *baseURL, Model: *model, ModelDigest: *modelDigest,
		ReviewerID: *reviewerID, ExpectedCases: *expectedCases, PerCaseTimeout: *perCaseTimeout,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-ollama: review: %v\n", err)
		return 1
	}
	if err := fillerreview.PublishReview(*outputDir, runEvidence, submissions); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-ollama: publish: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-review-ollama: completed %d blind labels in %s\n", len(submissions), *outputDir)
	return 0
}
