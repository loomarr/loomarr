// Command filler-suitability-compare validates and compares two independent
// direct-video suitability results without turning observed absence into an
// admission decision.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-suitability-compare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	evidence := flags.String("evidence", "", "verified temporal evidence manifest")
	first := flags.String("first", "", "first immutable suitability result")
	second := flags.String("second", "", "second immutable suitability result")
	expectedCases := flags.Int("expected-cases", 48, "exact case count in both results")
	comparedText := flags.String("compared-at", "", "fixed RFC3339 comparison time")
	output := flags.String("out", "", "new immutable private comparison JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	comparedAt, err := time.Parse(time.RFC3339, *comparedText)
	if err != nil || *evidence == "" || *first == "" || *second == "" || *expectedCases <= 0 || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-suitability-compare: evidence, two results, positive expected cases, fixed comparison time, and output are required")
		return 2
	}
	report, digest, err := fillerreview.PublishTemporalSuitabilityComparison(fillerreview.TemporalSuitabilityComparisonConfig{
		EvidenceManifestPath: *evidence, FirstResultPath: *first, SecondResultPath: *second,
		ComparedAt: comparedAt.UTC(), ExpectedCases: *expectedCases, OutputPath: *output,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-suitability-compare:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-suitability-compare: %d cases; corroborated-prohibited=%d uncorroborated-prohibited=%d operational-hold=%d coverage-hold=%d candidate-no-signal=%d; productionAdmissionAllowed=%t; sha256 %s; %s\n", report.Cases, report.CorroboratedProhibitedCases, report.UncorroboratedProhibitedCases, report.OperationalHoldCases, report.CoverageHoldCases, report.CandidateNoSignalCases, report.ProductionAdmissionAllowed, digest, *output)
	return 0
}
