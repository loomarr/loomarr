// Command filler-temporal-assess-openrouter performs one bounded, paid
// stronger-model calibration over an immutable temporal selection. Lock
// recovery is an explicit offline operation and never constructs paid-run
// capabilities.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/loomarr/loomarr/cmd/internal/fillerbakeoffio"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

type paidTemporalCapabilities struct {
	getenv func(string) string
	run    func(context.Context, fillerreview.OpenRouterTemporalConfig) (fillerreview.OpenRouterTemporalResult, error)
}

func run(args []string, stdout, stderr io.Writer) int {
	if recoveryRequested(args) {
		return runLockRecovery(args, stdout, stderr)
	}
	return runPaidAssessment(args, stdout, stderr, paidTemporalCapabilities{
		getenv: os.Getenv,
		run:    fillerreview.RunOpenRouterTemporalAssessment,
	})
}

func runPaidAssessment(args []string, stdout, stderr io.Writer, capabilities paidTemporalCapabilities) int {
	flags := newFlagSet(stderr)
	options := bindOptions(flags)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	apiKey := capabilities.getenv("OPENROUTER_API_KEY")
	if apiKey == "" || options.packagePath == "" || options.selectionPath == "" || options.snapshotPath == "" || options.model == "" || options.modelFamily == "" || options.provider == "" || options.providerSlug == "" || options.assessorID == "" || options.outputPath == "" || options.maxRequests <= 0 || options.maxSpendNanoUSD <= 0 || options.maxChargeNanoUSD <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-assess-openrouter: OPENROUTER_API_KEY and --package, --selection, --snapshot, --model, --model-family, --provider, --provider-slug, --assessor-id, --out, --max-requests, --max-spend-nanousd, and --max-charge-nanousd are required")
		return 2
	}
	snapshot, err := fillerbakeoffio.ReadStrictJSON[fillerbakeoff.OpenRouterSnapshot](options.snapshotPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-assess-openrouter: read snapshot:", err)
		return 1
	}
	result, err := capabilities.run(context.Background(), fillerreview.OpenRouterTemporalConfig{
		PackagePath: options.packagePath, SelectionPath: options.selectionPath, CheckpointDir: options.outputPath + ".private",
		BaseURL: options.baseURL, APIKey: apiKey, Snapshot: snapshot, Model: options.model, ModelFamily: options.modelFamily,
		UpstreamProvider: options.provider, UpstreamProviderSlug: options.providerSlug, AssessorID: options.assessorID,
		ExpectedPackageCases: options.expectedPackageCases, ExpectedCalibrationCases: options.expectedCalibrationCases,
		PerCaseTimeout: options.perCaseTimeout, MaxRequests: options.maxRequests,
		MaxSpendNanoUSD: options.maxSpendNanoUSD, MaxChargeNanoUSD: options.maxChargeNanoUSD,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-assess-openrouter: assess:", err)
		return 1
	}
	if err := fillerbakeoffio.WriteImmutableJSON(options.outputPath, ".filler-temporal-openrouter-*", result); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-assess-openrouter: publish:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-assess-openrouter: assessed %d cases in %d requests; charged %d and consumed %d nano-USD; %s\n", len(result.AssessmentSet.Assessments), result.Requests, result.ChargedNanoUSD, result.ConsumedNanoUSD, options.outputPath)
	return 0
}

func runLockRecovery(args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet(stderr)
	options := bindOptions(flags)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if options.outputPath == "" || options.recoverLockSHA256 == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-assess-openrouter: --out and --recover-lock-sha256 are required for explicit lock recovery")
		return 2
	}
	recovered, err := fillerreview.RecoverOpenRouterReviewLock(options.outputPath+".private", options.recoverLockSHA256)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-assess-openrouter: recover lock:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-assess-openrouter: recovered crash-stale lock to %s; inspect the checkpoint before any new paid run\n", recovered)
	return 0
}

type commandOptions struct {
	packagePath, selectionPath, snapshotPath, model, modelFamily string
	provider, providerSlug, assessorID, outputPath, baseURL      string
	recoverLockSHA256                                            string
	expectedPackageCases, expectedCalibrationCases, maxRequests  int
	perCaseTimeout                                               time.Duration
	maxSpendNanoUSD, maxChargeNanoUSD                            int64
}

func newFlagSet(stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet("filler-temporal-assess-openrouter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func bindOptions(flags *flag.FlagSet) *commandOptions {
	options := &commandOptions{}
	flags.StringVar(&options.packagePath, "package", "", "sealed identity-blind temporal package JSON")
	flags.StringVar(&options.selectionPath, "selection", "", "immutable calibration-selection JSON")
	flags.StringVar(&options.snapshotPath, "snapshot", "", "fresh immutable OpenRouter capability snapshot")
	flags.StringVar(&options.model, "model", "", "concrete OpenRouter model ID")
	flags.StringVar(&options.modelFamily, "model-family", "", "independent model family identity")
	flags.StringVar(&options.provider, "provider", "", "exact upstream provider display name")
	flags.StringVar(&options.providerSlug, "provider-slug", "", "exact upstream provider routing slug")
	flags.StringVar(&options.assessorID, "assessor-id", "", "identity unique to this calibration run")
	flags.StringVar(&options.outputPath, "out", "", "new immutable result JSON path")
	flags.StringVar(&options.baseURL, "base-url", fillerbakeoff.OpenRouterBaseURL, "OpenRouter API base URL")
	flags.StringVar(&options.recoverLockSHA256, "recover-lock-sha256", "", "retire one exact crash-stale active-run lock, then exit")
	flags.IntVar(&options.expectedPackageCases, "expected-package-cases", 32, "exact sealed package case count")
	flags.IntVar(&options.expectedCalibrationCases, "expected-calibration-cases", 15, "exact selected calibration case count")
	flags.DurationVar(&options.perCaseTimeout, "per-case-timeout", 5*time.Minute, "hard timeout for each serial case")
	flags.IntVar(&options.maxRequests, "max-requests", 0, "hard paid request ceiling")
	flags.Int64Var(&options.maxSpendNanoUSD, "max-spend-nanousd", 0, "hard total paid spend ceiling in nano-USD")
	flags.Int64Var(&options.maxChargeNanoUSD, "max-charge-nanousd", 0, "hard reserved per-request charge ceiling in nano-USD")
	return options
}

func recoveryRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--recover-lock-sha256" || strings.HasPrefix(arg, "--recover-lock-sha256=") {
			return true
		}
	}
	return false
}
