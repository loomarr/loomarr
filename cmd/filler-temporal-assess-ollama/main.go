// Command filler-temporal-assess-ollama runs the sealed temporal diagnostic
// through one digest-pinned local multimodal model.
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
	flags := flag.NewFlagSet("filler-temporal-assess-ollama", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packagePath := flags.String("package", "", "sealed identity-blind temporal package JSON")
	model := flags.String("model", "", "concrete local model tag")
	modelFamily := flags.String("model-family", "", "independent model family identity")
	modelDigest := flags.String("model-digest", "", "installed model SHA-256 digest")
	assessorID := flags.String("assessor-id", "", "identity unique to this assessment run")
	outputPath := flags.String("out", "", "new assessment-set JSON path")
	baseURL := flags.String("base-url", "http://127.0.0.1:11434", "loopback Ollama API base URL")
	expectedCases := flags.Int("expected-cases", 32, "exact sealed package case count")
	perCaseTimeout := flags.Duration("per-case-timeout", 10*time.Minute, "hard timeout for each serial assessment")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *packagePath == "" || *model == "" || *modelFamily == "" || *modelDigest == "" || *assessorID == "" || *outputPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-assess-ollama: --package, --model, --model-family, --model-digest, --assessor-id, and --out are required")
		return 2
	}
	set, err := fillerreview.RunOllamaTemporalAssessment(context.Background(), fillerreview.OllamaTemporalConfig{
		PackagePath: *packagePath, BaseURL: *baseURL, Model: *model, ModelFamily: *modelFamily,
		ModelDigest: *modelDigest, AssessorID: *assessorID, ExpectedCases: *expectedCases, PerCaseTimeout: *perCaseTimeout,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-assess-ollama:", err)
		return 1
	}
	if err := fillerbakeoffio.WriteImmutableJSON(*outputPath, ".filler-temporal-assessment-*", set); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-assess-ollama: publish:", err)
		return 1
	}
	failures := 0
	for _, assessment := range set.Assessments {
		if assessment.OperationalFailure != nil {
			failures++
		}
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-assess-ollama: assessed %d cases with %d operational failures in %s\n", len(set.Assessments), failures, *outputPath)
	return 0
}
