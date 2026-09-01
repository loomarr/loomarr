// Command filler-temporal-select derives one immutable, label-free calibration
// selection from two complete temporal assessment sets.
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
	flags := flag.NewFlagSet("filler-temporal-select", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packagePath := flags.String("package", "", "sealed identity-blind temporal package JSON")
	firstPath := flags.String("first", "", "first temporal assessment-set JSON")
	secondPath := flags.String("second", "", "second temporal assessment-set JSON")
	outputPath := flags.String("out", "", "new deterministic calibration-selection JSON path")
	expectedCases := flags.Int("expected-cases", 32, "exact sealed package case count")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *packagePath == "" || *firstPath == "" || *secondPath == "" || *outputPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-select: --package, --first, --second, and --out are required")
		return 2
	}
	_, cases, _, err := fillerreview.LoadTemporalReviewPackage(*packagePath, *expectedCases)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-select: package:", err)
		return 1
	}
	first, err := readAssessmentSet(*firstPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-select: first:", err)
		return 1
	}
	second, err := readAssessmentSet(*secondPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-select: second:", err)
		return 1
	}
	report, err := fillereval.CompareTemporalAssessmentSets(first, second, cases)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-select: compare:", err)
		return 1
	}
	selection, err := fillereval.BuildTemporalCalibrationSelection(report)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-select:", err)
		return 1
	}
	if err := fillerbakeoffio.WriteImmutableJSON(*outputPath, ".filler-temporal-selection-*", selection); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-select: publish:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-select: selected %d/%d cases in %s\n", len(selection.Cases), report.Cases, *outputPath)
	return 0
}

func readAssessmentSet(path string) (fillereval.TemporalAssessmentSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fillereval.TemporalAssessmentSet{}, err
	}
	return fillereval.DecodeTemporalAssessmentSet(raw)
}
