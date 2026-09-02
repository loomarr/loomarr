// Command filler-temporal-media-quality measures the production decoder-quality policy against
// the exact locked temporal human panel without requesting another human review.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-temporal-media-quality", flag.ContinueOnError)
	flags.SetOutput(stderr)
	humanPackage := flags.String("human-package", "", "reviewer public manifest JSON")
	humanMap := flags.String("human-map", "", "reviewer private map JSON")
	humanAssessment := flags.String("human-assessment", "", "locked canonical human assessment JSON")
	humanAttestation := flags.String("human-attestation", "", "locked human attestation JSON")
	ffmpeg := flags.String("ffmpeg", "ffmpeg", "ffmpeg binary path")
	expectedCases := flags.Int("expected-cases", 48, "exact panel case count")
	measuredText := flags.String("measured-at", "", "fixed RFC3339 measurement time")
	output := flags.String("out", "", "new immutable private quality report JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	measuredAt, err := time.Parse(time.RFC3339, *measuredText)
	if err != nil || *humanPackage == "" || *humanMap == "" || *humanAssessment == "" || *humanAttestation == "" || *ffmpeg == "" || *expectedCases <= 0 || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-media-quality: human package, map, lock, ffmpeg, positive expected cases, fixed measurement time, and output are required")
		return 2
	}
	report, digest, err := fillerreview.PublishTemporalMediaQuality(context.Background(), fillerreview.TemporalMediaQualityConfig{
		HumanPackagePath: *humanPackage, HumanPrivateMapPath: *humanMap,
		HumanAssessmentPath: *humanAssessment, HumanAttestationPath: *humanAttestation,
		FFmpegPath: *ffmpeg, ExpectedCases: *expectedCases, MeasuredAt: measuredAt.UTC(), OutputPath: *output,
		Progress: func(done, total int, _ string) {
			_, _ = fmt.Fprintf(stderr, "filler-temporal-media-quality: measured %d/%d\n", done, total)
		},
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-media-quality:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-media-quality: measured %d cases; human unusable held=%d continued=%d; other labels held=%d; failures=%d; productionAdmissionAllowed=%t; sha256 %s; %s\n", report.Cases, report.HumanUnusableHeld, report.HumanUnusableContinued, report.OtherHumanLabelsHeld, report.OperationalFailures, report.ProductionAdmissionAllowed, digest, *output)
	return 0
}
