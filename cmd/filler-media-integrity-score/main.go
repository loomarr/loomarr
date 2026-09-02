// Command filler-media-integrity-score locks the private media-integrity comparison. It performs no
// inference or media access and never grants production admission.
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
	flags := flag.NewFlagSet("filler-media-integrity-score", flag.ContinueOnError)
	flags.SetOutput(stderr)
	pack := flags.String("package", "", "label-free public package JSON")
	mapping := flags.String("map", "", "private authority map JSON")
	quality := flags.String("media-quality", "", "exact full-decode media-quality report JSON")
	lockedText := flags.String("locked-at", "", "fixed RFC3339 lock time")
	output := flags.String("out", "", "new private comparison report JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	lockedAt, err := time.Parse(time.RFC3339, *lockedText)
	if err != nil || *pack == "" || *mapping == "" || *quality == "" || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-media-integrity-score: package, private map, media quality, fixed lock time, and output are required")
		return 2
	}
	report, digest, err := fillerreview.ScoreMediaIntegrityChallenge(fillerreview.MediaIntegrityChallengeScoreConfig{PackagePath: *pack, MapPath: *mapping, MediaQualityPath: *quality, LockedAt: lockedAt.UTC(), OutputPath: *output})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-media-integrity-score:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-media-integrity-score: scored %d cases; correct %d; operational holds %d; measured gaps %d; production admission %t; sha256 %s\n", report.Cases, report.Correct, report.OperationalHolds, report.MeasuredGaps, report.ProductionAdmissionAllowed, digest)
	return 0
}
