// Command filler-temporal-panel-compare validates and compares one locked
// human reference with two independently blinded locked model panels.
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
	flags := flag.NewFlagSet("filler-temporal-panel-compare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	humanAssessment := flags.String("human-assessment", "", "locked canonical human assessment JSON")
	humanAttestation := flags.String("human-attestation", "", "locked human attestation JSON")
	firstAssessment := flags.String("first-assessment", "", "first locked model assessment JSON")
	firstAttestation := flags.String("first-attestation", "", "first locked model attestation JSON")
	secondAssessment := flags.String("second-assessment", "", "second locked model assessment JSON")
	secondAttestation := flags.String("second-attestation", "", "second locked model attestation JSON")
	expectedCases := flags.Int("expected-cases", 48, "exact assessment count for every rater")
	comparedText := flags.String("compared-at", "", "fixed RFC3339 comparison time")
	output := flags.String("out", "", "new immutable private comparison JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	comparedAt, err := time.Parse(time.RFC3339, *comparedText)
	if err != nil || *humanAssessment == "" || *humanAttestation == "" || *firstAssessment == "" || *firstAttestation == "" || *secondAssessment == "" || *secondAttestation == "" || *expectedCases <= 0 || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-panel-compare: human lock, two model locks, positive expected cases, fixed comparison time, and output are required")
		return 2
	}
	report, digest, err := fillerreview.PublishTemporalPanelComparison(fillerreview.TemporalPanelComparisonConfig{
		HumanAssessmentPath: *humanAssessment, HumanAttestationPath: *humanAttestation,
		FirstAssessmentPath: *firstAssessment, FirstAttestationPath: *firstAttestation,
		SecondAssessmentPath: *secondAssessment, SecondAttestationPath: *secondAttestation,
		ExpectedCases: *expectedCases, ComparedAt: comparedAt.UTC(), OutputPath: *output,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-panel-compare:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-panel-compare: compared %d cases; %d diagnostic candidates; next=%s; productionAdmissionAllowed=%t; sha256 %s; %s\n", report.Cases, report.Disposition.DiagnosticCandidateCases, report.Disposition.NextAction, report.Disposition.ProductionAdmissionAllowed, digest, *output)
	return 0
}
