// Command filler-temporal-calibration-report verifies and compares a hosted
// temporal result against the two local assessment sets that produced its
// immutable selection. It performs no inference and treats no model as truth.
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
	flags := flag.NewFlagSet("filler-temporal-calibration-report", flag.ContinueOnError)
	flags.SetOutput(stderr)
	packagePath := flags.String("package", "", "sealed identity-blind temporal package JSON")
	selectionPath := flags.String("selection", "", "immutable calibration-selection JSON")
	firstPath := flags.String("first", "", "first complete local temporal assessment set")
	secondPath := flags.String("second", "", "second complete local temporal assessment set")
	hostedResultPath := flags.String("hosted-result", "", "verified hosted temporal result JSON")
	outputPath := flags.String("out", "", "new immutable calibration-report JSON path")
	expectedPackageCases := flags.Int("expected-package-cases", 32, "exact sealed package case count")
	expectedCalibrationCases := flags.Int("expected-calibration-cases", 15, "exact selected calibration case count")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *packagePath == "" || *selectionPath == "" || *firstPath == "" || *secondPath == "" || *hostedResultPath == "" || *outputPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: --package, --selection, --first, --second, --hosted-result, and --out are required")
		return 2
	}
	loaded, err := fillerreview.LoadTemporalCalibrationPackage(*packagePath, *selectionPath, *expectedPackageCases, *expectedCalibrationCases)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: calibration package:", err)
		return 1
	}
	_, allSignals, _, err := fillerreview.LoadTemporalReviewPackage(*packagePath, *expectedPackageCases)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: package:", err)
		return 1
	}
	first, err := fillerbakeoffio.ReadStrictJSON[fillereval.TemporalAssessmentSet](*firstPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: first assessment:", err)
		return 1
	}
	second, err := fillerbakeoffio.ReadStrictJSON[fillereval.TemporalAssessmentSet](*secondPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: second assessment:", err)
		return 1
	}
	hostedRaw, err := os.ReadFile(*hostedResultPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: hosted result:", err)
		return 1
	}
	hosted, err := fillerreview.DecodeOpenRouterTemporalResult(hostedRaw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: hosted result:", err)
		return 1
	}
	if err := fillerreview.ValidateOpenRouterTemporalResult(hosted, loaded); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: hosted result:", err)
		return 1
	}
	report, err := fillereval.AnalyzeTemporalCalibration(loaded.Selection, loaded.SelectionSHA256, first, second, hosted.AssessmentSet, allSignals)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: analyze:", err)
		return 1
	}
	if err := fillerbakeoffio.WriteImmutableJSON(*outputPath, ".filler-temporal-calibration-report-*", report); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-calibration-report: publish:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-calibration-report: %d cases, %d operational failures, %d/%d agreement controls preserved; %s\n", report.Cases, report.OperationalFailures, report.AgreementControlsPreserved, report.AgreementControls, *outputPath)
	return 0
}
