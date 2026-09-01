// Command filler-temporal-compare deterministically compares two independent
// factored assessment sets over the same sealed package.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/loomarr/loomarr/cmd/internal/fillerbakeoffio"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-temporal-compare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packagePath := flags.String("package", "", "sealed identity-blind temporal package JSON")
	firstPath := flags.String("first", "", "first temporal assessment-set JSON")
	secondPath := flags.String("second", "", "second temporal assessment-set JSON")
	outputPath := flags.String("out", "", "new deterministic comparison JSON path")
	expectedCases := flags.Int("expected-cases", 32, "exact sealed package case count")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *packagePath == "" || *firstPath == "" || *secondPath == "" || *outputPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-compare: --package, --first, --second, and --out are required")
		return 2
	}
	_, cases, _, err := fillerreview.LoadTemporalReviewPackage(*packagePath, *expectedCases)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-compare: package:", err)
		return 1
	}
	firstRaw, err := os.ReadFile(*firstPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-compare: first:", err)
		return 1
	}
	first, err := fillereval.DecodeTemporalAssessmentSet(firstRaw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-compare: first:", err)
		return 1
	}
	secondRaw, err := os.ReadFile(*secondPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-compare: second:", err)
		return 1
	}
	second, err := fillereval.DecodeTemporalAssessmentSet(secondRaw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-compare: second:", err)
		return 1
	}
	report, err := fillereval.CompareTemporalAssessmentSets(first, second, cases)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-compare:", err)
		return 1
	}
	if err := fillerbakeoffio.WriteImmutableJSON(*outputPath, ".filler-temporal-comparison-*", report); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-compare: publish:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-compare: %d/%d exact agreements; %d require calibration; systemicFailure=%t\n", report.ExactAgreement, report.Cases, report.AdjudicationRequired, report.SystemicFailure)
	return 0
}
