// Command filler-temporal-review-lock validates and unblinds one complete
// offline human submission into immutable canonical artifacts.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-temporal-review-lock", flag.ContinueOnError)
	flags.SetOutput(stderr)
	pack := flags.String("package", "", "completed batch public manifest JSON")
	privateMap := flags.String("map", "", "coordinator-private batch map JSON")
	submission := flags.String("submission", "", "reviewer-exported submission JSON")
	reviewer := flags.String("reviewer", "", "expected reviewer identifier")
	lockedText := flags.String("locked-at", "", "fixed RFC3339 lock time")
	maximumAge := flags.Duration("maximum-age", 7*24*time.Hour, "maximum interval from preparation through lock")
	output := flags.String("out", "", "new locked-artifact directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	lockedAt, err := time.Parse(time.RFC3339, *lockedText)
	if err != nil || *pack == "" || *privateMap == "" || *submission == "" || *reviewer == "" || *maximumAge <= 0 || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-review-lock: package, map, submission, reviewer, fixed lock time, positive maximum age, and output are required")
		return 2
	}
	result, err := fillerreview.LockTemporalHumanReview(fillerreview.TemporalHumanReviewLockConfig{
		PackagePath: *pack, PrivateMapPath: *privateMap, SubmissionPath: *submission,
		ExpectedReviewerID: *reviewer, LockedAt: lockedAt.UTC(), MaximumAge: *maximumAge, OutputDir: *output,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-review-lock:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-review-lock: locked %d assessments to %s; assessment sha256 %s; attestation sha256 %s\n", result.Assessments, filepath.Clean(*output), result.AssessmentSetSHA256, result.AttestationSHA256)
	return 0
}
