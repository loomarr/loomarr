// Command filler-temporal-structure-lock joins a public-only paid model result
// to private construction authority after inference and publishes one lock.
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
	flags := flag.NewFlagSet("filler-temporal-structure-lock", flag.ContinueOnError)
	flags.SetOutput(stderr)
	public := flags.String("public", "", "public temporal-structure challenge manifest")
	authority := flags.String("authority", "", "coordinator-private construction authority")
	result := flags.String("result", "", "immutable raw OpenRouter structure result")
	snapshot := flags.String("snapshot", "", "exact capability snapshot used by the result")
	cases := flags.Int("cases", 0, "exact complete challenge case count")
	lockedText := flags.String("locked-at", "", "fixed RFC3339 lock time after result completion")
	output := flags.String("out", "", "new immutable canonical assessment-set JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	lockedAt, err := time.Parse(time.RFC3339, *lockedText)
	if err != nil || *public == "" || *authority == "" || *result == "" || *snapshot == "" || *cases <= 0 || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-lock: challenge authority, raw result, snapshot, positive exact case count, fixed lock time, and output are required")
		return 2
	}
	locked, err := fillerreview.LockTemporalStructureAssessment(fillerreview.TemporalStructureAssessmentLockConfig{
		PublicManifestPath: *public, PrivateAuthorityPath: *authority, ResultPath: *result, SnapshotPath: *snapshot,
		ExpectedCases: *cases, LockedAt: lockedAt.UTC(), OutputPath: *output,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-lock:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-structure-lock: locked %d assessments; assessment sha256 %s; raw result sha256 %s; snapshot file sha256 %s; %s\n", locked.Assessments, locked.AssessmentSHA256, locked.RawResultSHA256, locked.SnapshotFileSHA256, *output)
	return 0
}
