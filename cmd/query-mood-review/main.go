//go:build eval

// Command query-mood-review runs and locks blinded development-only mood reviews.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/eval"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/llm"
)

const maxInputBytes = 8 << 20

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "query-mood-review: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("query-mood-review", flag.ContinueOnError)
	mode := flags.String("mode", "", "run or lock")
	packetPath := flags.String("packet", "", "blinded review packet")
	mapPath := flags.String("private-map", "", "coordinator-only alias map")
	submissionPaths := flags.String("submissions", "", "comma-separated submissions")
	outputPath := flags.String("out", "", "output artifact")
	providerName := flags.String("provider", "", "ollama or openrouter")
	baseURL := flags.String("base-url", "", "provider API base")
	model := flags.String("model", "", "exact requested model")
	reviewerID := flags.String("reviewer-id", "", "stable reviewer identity")
	modelFamily := flags.String("model-family", "", "registered model family")
	modelDigest := flags.String("model-digest", "", "exact Ollama model digest")
	snapshotPath := flags.String("snapshot", "", "fresh OpenRouter capability snapshot")
	upstreamProvider := flags.String("upstream-provider", "", "exact OpenRouter upstream provider")
	providerSlug := flags.String("provider-slug", "", "snapshot endpoint slug")
	maxChargeUSD := flags.String("max-charge-usd", "", "authorized maximum charge for this one call")
	allowProviderRetention := flags.Bool("allow-provider-retention", false, "explicitly permit a public-only packet on a non-ZDR route")
	retentionAuthorization := flags.String("retention-authorization", "", "recorded maintainer authorization for a non-ZDR public packet")
	timeout := flags.Duration("timeout", 5*time.Minute, "single-call timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *packetPath == "" || *outputPath == "" {
		return errors.New("--packet and --out are required")
	}
	packet, err := readBounded(*packetPath)
	if err != nil {
		return err
	}
	switch *mode {
	case "run":
		if *reviewerID == "" || *modelFamily == "" || *model == "" || *timeout <= 0 {
			return errors.New("run mode requires --reviewer-id, --model-family, --model, and a positive --timeout")
		}
		callCtx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		submission, err := runReview(callCtx, packet, runOptions{
			provider: *providerName, baseURL: *baseURL, model: *model, reviewerID: *reviewerID,
			modelFamily: *modelFamily, modelDigest: *modelDigest, snapshotPath: *snapshotPath,
			upstreamProvider: *upstreamProvider, providerSlug: *providerSlug, maxChargeUSD: *maxChargeUSD,
			allowProviderRetention: *allowProviderRetention, retentionAuthorization: *retentionAuthorization,
		})
		if err != nil {
			return err
		}
		return writeJSONAtomic(*outputPath, submission)
	case "lock":
		if *mapPath == "" || *submissionPaths == "" {
			return errors.New("lock mode requires --private-map and --submissions")
		}
		privateMap, err := readBounded(*mapPath)
		if err != nil {
			return err
		}
		paths := strings.Split(*submissionPaths, ",")
		submissions := make([][]byte, 0, len(paths))
		for _, path := range paths {
			blob, err := readBounded(strings.TrimSpace(path))
			if err != nil {
				return err
			}
			submissions = append(submissions, blob)
		}
		authority, err := eval.CompileMoodReviewAuthority(packet, privateMap, submissions...)
		if err != nil {
			return err
		}
		return writeJSONAtomic(*outputPath, authority)
	default:
		return errors.New("--mode must be run or lock")
	}
}

type runOptions struct {
	provider, baseURL, model, reviewerID, modelFamily, modelDigest string
	snapshotPath, upstreamProvider, providerSlug, maxChargeUSD     string
	allowProviderRetention                                         bool
	retentionAuthorization                                         string
}

func runReview(ctx context.Context, packet []byte, options runOptions) (eval.MoodReviewSubmission, error) {
	config := eval.MoodReviewRunConfig{ReviewerID: options.reviewerID, ModelFamily: options.modelFamily}
	var provider llm.Provider
	switch options.provider {
	case "ollama":
		if options.baseURL == "" {
			options.baseURL = "http://127.0.0.1:11434"
		}
		if err := verifyOllamaDigest(ctx, options.baseURL, options.model, options.modelDigest); err != nil {
			return eval.MoodReviewSubmission{}, err
		}
		provider = llm.NewOllama(options.baseURL, options.model)
		config.IdentityKind, config.IdentitySHA256 = "ollama-model-digest", options.modelDigest
	case "openrouter":
		if options.baseURL == "" {
			options.baseURL = fillerbakeoff.OpenRouterBaseURL
		}
		budget, ok := new(big.Rat).SetString(options.maxChargeUSD)
		if !ok || budget.Sign() <= 0 {
			return eval.MoodReviewSubmission{}, errors.New("OpenRouter review requires an explicit positive --max-charge-usd")
		}
		if options.allowProviderRetention == (strings.TrimSpace(options.retentionAuthorization) == "") {
			return eval.MoodReviewSubmission{}, errors.New("--allow-provider-retention requires a nonempty --retention-authorization, and ZDR runs must omit it")
		}
		snapshotBlob, err := readBounded(options.snapshotPath)
		if err != nil {
			return eval.MoodReviewSubmission{}, err
		}
		var snapshot fillerbakeoff.OpenRouterSnapshot
		if err := decodeStrict(snapshotBlob, &snapshot); err != nil {
			return eval.MoodReviewSubmission{}, fmt.Errorf("decode OpenRouter snapshot: %w", err)
		}
		age := time.Since(snapshot.RetrievedAt)
		if age < 0 || age > 24*time.Hour || snapshot.SourceBaseURL != options.baseURL {
			return eval.MoodReviewSubmission{}, errors.New("OpenRouter snapshot is stale, future-dated, or from another API base")
		}
		_, capabilityDigest, err := fillerbakeoff.OpenRouterAssessorIdentity(snapshot, options.model, options.upstreamProvider, options.providerSlug, "disabled")
		if err != nil {
			return eval.MoodReviewSubmission{}, fmt.Errorf("bind OpenRouter reviewer identity: %w", err)
		}
		provider, err = llm.NewOpenRouterChat(llm.OpenRouterChatConfig{
			BaseURL: options.baseURL, Model: options.model, APIKey: os.Getenv("OPENROUTER_API_KEY"),
			UpstreamProvider: options.upstreamProvider, AllowProviderRetention: options.allowProviderRetention,
		})
		if err != nil {
			return eval.MoodReviewSubmission{}, err
		}
		if os.Getenv("OPENROUTER_API_KEY") == "" {
			return eval.MoodReviewSubmission{}, errors.New("OPENROUTER_API_KEY is required")
		}
		config.IdentityKind, config.IdentitySHA256 = "openrouter-route-snapshot", capabilityDigest
		config.ExpectedResolvedProvider, config.RouteSlug = options.upstreamProvider, options.providerSlug
		config.SnapshotSHA256 = sha256Hex(snapshotBlob)
		config.ZeroDataRetention = !options.allowProviderRetention
		if options.allowProviderRetention {
			config.RetentionAuthorization = options.retentionAuthorization
		}
		submission, err := eval.RunMoodReview(ctx, provider, packet, config)
		if err != nil {
			return eval.MoodReviewSubmission{}, err
		}
		charge, ok := new(big.Rat).SetString(submission.Inference.ChargeAmount)
		if !ok || charge.Sign() < 0 || charge.Cmp(budget) > 0 {
			return eval.MoodReviewSubmission{}, fmt.Errorf("OpenRouter reported charge %q exceeds or cannot be checked against authorized USD %s", submission.Inference.ChargeAmount, budget.FloatString(6))
		}
		return submission, nil
	default:
		return eval.MoodReviewSubmission{}, errors.New("run mode --provider must be ollama or openrouter")
	}
	return eval.RunMoodReview(ctx, provider, packet, config)
}

func verifyOllamaDigest(ctx context.Context, baseURL, model, expected string) error {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" && parsed.Hostname() != "::1") || len(expected) != sha256.Size*2 {
		return errors.New("ollama review requires a loopback API and exact SHA-256 model digest")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String()+"/api/tags", nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama tags returned status %d", response.StatusCode)
	}
	var tags struct {
		Models []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxInputBytes)).Decode(&tags); err != nil {
		return err
	}
	for _, item := range tags.Models {
		if item.Name == model && item.Digest == expected {
			return nil
		}
	}
	return fmt.Errorf("ollama model %q does not match digest %q", model, expected)
}

func readBounded(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("artifact path is empty")
	}
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	blob, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(blob) > maxInputBytes {
		return nil, fmt.Errorf("artifact %q exceeds %d bytes", path, maxInputBytes)
	}
	return blob, nil
}

func writeJSONAtomic(path string, value any) error {
	blob, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	blob = append(blob, '\n')
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(absolute), ".query-mood-review-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(blob); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, absolute)
}

func decodeStrict(blob []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(blob))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing data")
	}
	return nil
}

func sha256Hex(blob []byte) string {
	digest := sha256.Sum256(blob)
	return hex.EncodeToString(digest[:])
}
