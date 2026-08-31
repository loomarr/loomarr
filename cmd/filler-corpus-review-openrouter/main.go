// Command filler-corpus-review-openrouter either completes one independently
// shuffled blind semantic-review package through a capability-snapshotted paid
// route or inspects an existing private Reviewer-B checkpoint offline.
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
	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

type paidRunCapabilities struct {
	getenv              func(string) string
	runOpenRouterReview func(context.Context, fillerreview.OpenRouterReviewConfig) (fillerreview.ReviewRun, []fillereval.LabelSubmission, error)
}

func run(args []string, stdout, stderr io.Writer) int {
	if inspectionRequested(args) {
		return runOfflineInspection(args, stdout, stderr)
	}
	return runPaidReviewOrRecovery(args, stdout, stderr, paidRunCapabilities{
		getenv: os.Getenv, runOpenRouterReview: fillerreview.RunOpenRouterReview,
	})
}

func runPaidReviewOrRecovery(args []string, stdout, stderr io.Writer, capabilities paidRunCapabilities) int {
	flags := flag.NewFlagSet("filler-corpus-review-openrouter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packageDir := flags.String("package", "", "identity-blind reviewer evidence package")
	transcriptsPath := flags.String("transcripts", "", "content-bound shared transcript JSONL")
	snapshotPath := flags.String("snapshot", "", "fresh immutable OpenRouter capability snapshot")
	model := flags.String("model", "", "concrete OpenRouter reviewer model ID")
	provider := flags.String("provider", "", "exact upstream provider display name")
	providerSlug := flags.String("provider-slug", "", "exact upstream provider routing slug")
	reviewerID := flags.String("reviewer-id", "", "identity unique to this independent review")
	outputDir := flags.String("out", "", "new directory for labels and review attestation")
	baseURL := flags.String("base-url", fillerbakeoff.OpenRouterBaseURL, "OpenRouter API base URL")
	expectedCases := flags.Int("expected-cases", 300, "exact blind package case count")
	perCaseTimeout := flags.Duration("per-case-timeout", 5*time.Minute, "hard timeout for each serial review")
	maxRequests := flags.Int("max-requests", 301, "hard paid request ceiling, including one reserved recovery attempt")
	maxSpendNanoUSD := flags.Int64("max-spend-nanousd", 0, "hard total paid spend ceiling in nano-USD")
	maxChargeNanoUSD := flags.Int64("max-charge-nanousd", 0, "hard reserved per-request charge ceiling in nano-USD")
	recoverLockSHA256 := flags.String("recover-lock-sha256", "", "explicitly retire a crash-stale active-run lock matching this exact SHA-256, then exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *recoverLockSHA256 != "" {
		if *outputDir == "" {
			_, _ = fmt.Fprintln(stderr, "filler-corpus-review-openrouter: --out is required for explicit lock recovery")
			return 2
		}
		recovered, err := fillerreview.RecoverOpenRouterReviewLock(*outputDir+".private", *recoverLockSHA256)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "filler-corpus-review-openrouter: recover lock: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "filler-corpus-review-openrouter: recovered crash-stale lock to %s; inspect the audit before starting a new run\n", recovered)
		return 0
	}
	apiKey := capabilities.getenv("OPENROUTER_API_KEY")
	if apiKey == "" || *packageDir == "" || *transcriptsPath == "" || *snapshotPath == "" || *model == "" || *provider == "" || *providerSlug == "" || *reviewerID == "" || *outputDir == "" || *maxSpendNanoUSD <= 0 || *maxChargeNanoUSD <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-review-openrouter: OPENROUTER_API_KEY and --package, --transcripts, --snapshot, --model, --provider, --provider-slug, --reviewer-id, --out, --max-spend-nanousd, and --max-charge-nanousd are required")
		return 2
	}
	transcripts, err := fillerbakeoffio.ReadTranscripts(*transcriptsPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-openrouter: read transcripts: %v\n", err)
		return 1
	}
	snapshot, err := fillerbakeoffio.ReadStrictJSON[fillerbakeoff.OpenRouterSnapshot](*snapshotPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-openrouter: read snapshot: %v\n", err)
		return 1
	}
	runEvidence, submissions, err := capabilities.runOpenRouterReview(context.Background(), fillerreview.OpenRouterReviewConfig{
		PackageDir: *packageDir, CheckpointDir: *outputDir + ".private", Transcripts: transcripts, BaseURL: *baseURL, APIKey: apiKey, Snapshot: snapshot,
		Model: *model, UpstreamProvider: *provider, UpstreamProviderSlug: *providerSlug, ReviewerID: *reviewerID,
		ExpectedCases: *expectedCases, PerCaseTimeout: *perCaseTimeout, MaxRequests: *maxRequests,
		MaxSpendNanoUSD: *maxSpendNanoUSD, MaxChargeNanoUSD: *maxChargeNanoUSD,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-openrouter: review: %v\n", err)
		return 1
	}
	if err := fillerreview.PublishReview(*outputDir, runEvidence, submissions); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-openrouter: publish: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-review-openrouter: completed %d blind labels for %d nano-USD in %s\n", len(submissions), runEvidence.ChargedNanoUSD, *outputDir)
	return 0
}
