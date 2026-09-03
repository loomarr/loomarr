// Command filler-temporal-structure-certify publishes the non-authorizing
// certificate for one locked 60-case temporal-structure challenge.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

type capabilities struct {
	publish func(fillerreview.TemporalStructureCertificationConfig) (fillerreview.TemporalStructureCertificationReport, string, error)
}

type assessmentPaths []string

func (paths *assessmentPaths) String() string { return fmt.Sprint([]string(*paths)) }

func (paths *assessmentPaths) Set(value string) error {
	if value == "" {
		return fmt.Errorf("assessment path cannot be empty")
	}
	*paths = append(*paths, value)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, capabilities{publish: fillerreview.PublishTemporalStructureCertification}))
}

func run(args []string, stdout, stderr io.Writer, capability capabilities) int {
	flags := flag.NewFlagSet("filler-temporal-structure-certify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	authoring := flags.String("holdout-authoring", "", "coordinator-private 60-case holdout authoring JSON")
	receipt := flags.String("holdout-receipt", "", "coordinator-private source-family holdout receipt JSON")
	public := flags.String("public", "", "public temporal-structure manifest JSON")
	authority := flags.String("authority", "", "coordinator-private temporal-structure authority JSON")
	var assessments assessmentPaths
	flags.Var(&assessments, "assessment", "locked assessment JSON; repeat for each independent model family")
	comparedRaw := flags.String("compared-at", "", "fixed RFC3339 comparison time")
	certifiedRaw := flags.String("certified-at", "", "fixed RFC3339 certification time")
	output := flags.String("out", "", "new immutable private certification JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *authoring == "" || *receipt == "" || *public == "" || *authority == "" || len(assessments) < 2 || *comparedRaw == "" || *certifiedRaw == "" || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-certify: holdout authoring and receipt, challenge authority, at least two --assessment values, fixed comparison and certification times, and output are required")
		return 2
	}
	comparedAt, err := time.Parse(time.RFC3339Nano, *comparedRaw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-certify: --compared-at must be RFC3339")
		return 2
	}
	certifiedAt, err := time.Parse(time.RFC3339Nano, *certifiedRaw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-certify: --certified-at must be RFC3339")
		return 2
	}
	report, digest, err := capability.publish(fillerreview.TemporalStructureCertificationConfig{
		HoldoutAuthoringPath: *authoring, HoldoutReceiptPath: *receipt,
		PublicManifestPath: *public, PrivateAuthorityPath: *authority,
		AssessmentPaths: assessments, ComparedAt: comparedAt, CertifiedAt: certifiedAt, OutputPath: *output,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-certify:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-structure-certify: %s; %d cases across %d assessors; %d/%d difficult slices certified; productionAdmissionAllowed=%t; sha256 %s; %s\n", report.CertificationStatus, report.Cases, len(report.AssessorIDs), len(report.CertifiedSlices), len(report.Slices), report.ProductionAdmissionAllowed, digest, *output)
	return 0
}
