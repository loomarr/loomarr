package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

func inspectionRequested(args []string) bool {
	for _, argument := range args {
		if argument == "--" {
			return false
		}
		if argument == "--inspect-checkpoint" || strings.HasPrefix(argument, "--inspect-checkpoint=") {
			return true
		}
	}
	return false
}

func runOfflineInspection(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-corpus-review-openrouter offline inspection", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packageDir := flags.String("package", "", "identity-blind reviewer evidence package")
	transcriptsPath := flags.String("transcripts", "", "content-bound shared transcript JSONL")
	snapshotPath := flags.String("snapshot", "", "historical immutable OpenRouter capability snapshot")
	model := flags.String("model", "", "concrete OpenRouter reviewer model ID")
	provider := flags.String("provider", "", "exact upstream provider display name")
	providerSlug := flags.String("provider-slug", "", "exact upstream provider routing slug")
	reviewerID := flags.String("reviewer-id", "", "identity unique to this independent review")
	expectedCases := flags.Int("expected-cases", 300, "exact blind package case count")
	maxRequests := flags.Int("max-requests", 301, "historical immutable paid request ceiling")
	maxSpendNanoUSD := flags.Int64("max-spend-nanousd", 0, "historical immutable paid spend ceiling in nano-USD")
	maxChargeNanoUSD := flags.Int64("max-charge-nanousd", 0, "historical immutable per-request charge ceiling in nano-USD")
	checkpointDir := flags.String("inspect-checkpoint", "", "offline private checkpoint directory to validate without provider access")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *packageDir == "" || *transcriptsPath == "" || *snapshotPath == "" || *model == "" || *provider == "" || *providerSlug == "" || *reviewerID == "" || *checkpointDir == "" || *maxSpendNanoUSD <= 0 || *maxChargeNanoUSD <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-review-openrouter: offline inspection requires --package, --transcripts, --snapshot, --model, --provider, --provider-slug, --reviewer-id, --inspect-checkpoint, --max-spend-nanousd, and --max-charge-nanousd; paid review and lock recovery are separate operations")
		return 2
	}
	artifacts, err := fillerreview.OpenOpenRouterReviewInspectionArtifacts(fillerreview.OpenRouterReviewInspectionArtifactPaths{
		PackageDir: *packageDir, CheckpointDir: *checkpointDir,
		TranscriptsPath: *transcriptsPath, SnapshotPath: *snapshotPath,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-openrouter: inspect open artifacts: %v\n", err)
		return 1
	}
	defer func() { _ = artifacts.Close() }()
	attestation, err := fillerreview.InspectOpenRouterReviewCheckpoint(fillerreview.OpenRouterReviewInspectionConfig{
		OpenedArtifacts: artifacts,
		Model:           *model, UpstreamProvider: *provider, UpstreamProviderSlug: *providerSlug, ReviewerID: *reviewerID,
		ExpectedCases: *expectedCases, MaxRequests: *maxRequests,
		MaxSpendNanoUSD: *maxSpendNanoUSD, MaxChargeNanoUSD: *maxChargeNanoUSD,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-openrouter: inspect checkpoint: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(attestation); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-corpus-review-openrouter: encode sanitized checkpoint attestation: %v\n", err)
		return 1
	}
	return 0
}
