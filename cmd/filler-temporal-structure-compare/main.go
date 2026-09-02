// Command filler-temporal-structure-compare scores at least two independently
// locked full-video assessments against private construction authority.
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

type assessmentPaths []string

func (paths *assessmentPaths) String() string { return fmt.Sprint([]string(*paths)) }

func (paths *assessmentPaths) Set(value string) error {
	if value == "" {
		return fmt.Errorf("assessment path cannot be empty")
	}
	*paths = append(*paths, value)
	return nil
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-temporal-structure-compare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	public := flags.String("public", "", "public temporal-structure manifest JSON")
	authority := flags.String("authority", "", "coordinator-private temporal-structure authority JSON")
	var assessments assessmentPaths
	flags.Var(&assessments, "assessment", "locked full-video assessment JSON; repeat for each independent model")
	cases := flags.Int("cases", 0, "exact expected case count")
	comparedText := flags.String("compared-at", "", "fixed RFC3339 comparison time")
	output := flags.String("out", "", "new immutable private comparison JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	comparedAt, err := time.Parse(time.RFC3339, *comparedText)
	if err != nil || *public == "" || *authority == "" || len(assessments) < 2 || *cases <= 0 || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-compare: challenge authority, at least two --assessment values, positive exact case count, fixed comparison time, and output are required")
		return 2
	}
	report, digest, err := fillerreview.PublishTemporalStructureComparison(fillerreview.TemporalStructureComparisonConfig{
		PublicManifestPath: *public, PrivateAuthorityPath: *authority, AssessmentPaths: assessments,
		ExpectedCases: *cases, ComparedAt: comparedAt.UTC(), OutputPath: *output,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-compare:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-structure-compare: scored %d cases across %d assessors; %d exact consensus cases; %d targeted diagnostics; productionAdmissionAllowed=%t; sha256 %s; %s\n", report.Cases, len(report.Assessors), report.AllAssessorsExactCorrect, len(report.DiagnosticCandidates), report.ProductionAdmissionAllowed, digest, *output)
	return 0
}
