// Command filler-suitability-assess-openrouter performs serial, bounded direct-
// video screening for closed prohibited suitability flags. It is diagnostic
// until independently certified and never emits an admission decision.
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

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }
func (values *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("empty alias")
	}
	*values = append(*values, value)
	return nil
}

type suitabilityCapabilities struct {
	getenv func(string) string
	run    func(context.Context, fillerreview.TemporalSuitabilityConfig) (fillerreview.TemporalSuitabilityResult, error)
}

func run(args []string, stdout, stderr io.Writer) int {
	return runSuitability(args, stdout, stderr, suitabilityCapabilities{getenv: os.Getenv, run: fillerreview.RunOpenRouterTemporalSuitability})
}

func runSuitability(args []string, stdout, stderr io.Writer, capabilities suitabilityCapabilities) int {
	flags := flag.NewFlagSet("filler-suitability-assess-openrouter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifest := flags.String("evidence", "", "verified temporal evidence or structure-challenge manifest")
	structureAuthority := flags.String("structure-authority", "", "private structure authority when --evidence is a structure challenge")
	var aliases stringList
	flags.Var(&aliases, "alias", "one evidence alias to screen; repeat; omit for the complete manifest")
	snapshotPath := flags.String("snapshot", "", "fresh immutable OpenRouter capability snapshot")
	model := flags.String("model", "", "concrete video-capable OpenRouter model ID")
	modelFamily := flags.String("model-family", "", "model family identity")
	provider := flags.String("provider", "", "exact upstream provider display name")
	providerSlug := flags.String("provider-slug", "", "exact upstream provider routing slug")
	assessorID := flags.String("assessor-id", "", "identity unique to this screening run")
	reasoningMode := flags.String("reasoning-mode", "", "exact reasoning contract: disabled or provider_required")
	expectedCases := flags.Int("expected-cases", 0, "exact selected case count")
	perCaseTimeout := flags.Duration("per-case-timeout", 10*time.Minute, "hard timeout for each serial video")
	maxRequests := flags.Int("max-requests", 0, "hard paid request ceiling; must equal expected cases")
	maxSpendNanoUSD := flags.Int64("max-spend-nanousd", 0, "hard total paid spend ceiling in nano-USD")
	maxChargeNanoUSD := flags.Int64("max-charge-nanousd", 0, "hard reserved per-request charge ceiling in nano-USD")
	baseURL := flags.String("base-url", fillerbakeoff.OpenRouterBaseURL, "OpenRouter API base URL")
	output := flags.String("out", "", "new immutable result JSON path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	apiKey := capabilities.getenv("OPENROUTER_API_KEY")
	if apiKey == "" || *manifest == "" || *snapshotPath == "" || *model == "" || *modelFamily == "" || *provider == "" || *providerSlug == "" || *assessorID == "" || *reasoningMode == "" || *expectedCases <= 0 || *maxRequests <= 0 || *maxSpendNanoUSD <= 0 || *maxChargeNanoUSD <= 0 || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-suitability-assess-openrouter: credential, evidence, snapshot, exact route/model identity, positive case/request/cost ceilings, assessor, and output are required")
		return 2
	}
	snapshot, err := fillerbakeoffio.ReadStrictJSON[fillerbakeoff.OpenRouterSnapshot](*snapshotPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-suitability-assess-openrouter: read snapshot:", err)
		return 1
	}
	result, err := capabilities.run(context.Background(), fillerreview.TemporalSuitabilityConfig{
		EvidenceManifestPath: *manifest, StructureAuthorityPath: *structureAuthority,
		CaseAliases: aliases, CheckpointDir: *output + ".private",
		BaseURL: *baseURL, APIKey: apiKey, Snapshot: snapshot, Model: *model, ModelFamily: *modelFamily,
		UpstreamProvider: *provider, UpstreamProviderSlug: *providerSlug, AssessorID: *assessorID,
		ReasoningMode: *reasoningMode,
		ExpectedCases: *expectedCases, PerCaseTimeout: *perCaseTimeout, MaxRequests: *maxRequests,
		MaxSpendNanoUSD: *maxSpendNanoUSD, MaxChargeNanoUSD: *maxChargeNanoUSD,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-suitability-assess-openrouter: assess:", err)
		return 1
	}
	if err := fillerbakeoffio.WriteImmutableJSON(*output, ".filler-suitability-openrouter-*", result); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-suitability-assess-openrouter: publish:", err)
		return 1
	}
	prohibited, coverageHolds, noSignal, failures := 0, 0, 0, 0
	for _, assessment := range result.Assessments {
		switch {
		case assessment.OperationalFailure != nil:
			failures++
		case assessment.Outcome == fillerreview.SuitabilityOutcomeProhibitedSignal:
			prohibited++
		case assessment.Outcome == fillerreview.SuitabilityOutcomeCoverageHold:
			coverageHolds++
		case assessment.Outcome == fillerreview.SuitabilityOutcomeNoSignalObserved:
			noSignal++
		}
	}
	_, _ = fmt.Fprintf(stdout, "filler-suitability-assess-openrouter: screened %d cases in %d requests; prohibited=%d coverage-hold=%d no-signal-observed=%d failures=%d; charged %d nano-USD; productionAdmissionAllowed=%t; %s\n", len(result.Assessments), result.Requests, prohibited, coverageHolds, noSignal, failures, result.ChargedNanoUSD, result.ProductionAdmissionAllowed, *output)
	return 0
}
