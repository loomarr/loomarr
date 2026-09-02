// Command filler-temporal-model-lock validates, unblinds, and releases one
// complete model-panel result after proving it was run after the human lock.
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
	flags := flag.NewFlagSet("filler-temporal-model-lock", flag.ContinueOnError)
	flags.SetOutput(stderr)
	pack := flags.String("package", "", "completed model-batch public manifest JSON")
	privateMap := flags.String("map", "", "coordinator-private model-batch map JSON")
	result := flags.String("result", "", "complete raw OpenRouter temporal result JSON")
	snapshot := flags.String("snapshot", "", "exact OpenRouter capability snapshot JSON")
	humanAssessment := flags.String("human-assessment", "", "locked canonical human assessment JSON")
	humanAttestation := flags.String("human-attestation", "", "locked human attestation JSON")
	expectedCases := flags.Int("expected-cases", 48, "exact model and human case count")
	releasedText := flags.String("released-at", "", "fixed RFC3339 result-release time")
	output := flags.String("out", "", "new locked model-artifact directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	releasedAt, err := time.Parse(time.RFC3339, *releasedText)
	if err != nil || *pack == "" || *privateMap == "" || *result == "" || *snapshot == "" || *humanAssessment == "" || *humanAttestation == "" || *expectedCases <= 0 || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-model-lock: package, map, result, snapshot, human assessment, human attestation, positive expected cases, fixed release time, and output are required")
		return 2
	}
	locked, err := fillerreview.LockTemporalModelAssessment(fillerreview.TemporalModelAssessmentLockConfig{
		PackagePath: *pack, PrivateMapPath: *privateMap, ResultPath: *result, SnapshotPath: *snapshot,
		HumanAssessmentPath: *humanAssessment, HumanAttestationPath: *humanAttestation,
		ExpectedCases: *expectedCases, ReleasedAt: releasedAt.UTC(), OutputDir: *output,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-model-lock:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-model-lock: locked and released %d assessments to %s; assessment sha256 %s; attestation sha256 %s\n", locked.Assessments, filepath.Clean(*output), locked.AssessmentSetSHA256, locked.AttestationSHA256)
	return 0
}
