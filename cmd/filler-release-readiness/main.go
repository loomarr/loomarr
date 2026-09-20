// Command filler-release-readiness evaluates one local beta release evidence bundle.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/fillerrelease"
)

const (
	exitGo    = 0
	exitError = 1
	exitHold  = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now))
}

func run(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	flags := flag.NewFlagSet("filler-release-readiness", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "root of the immutable evidence bundle")
	manifestPath := flags.String("manifest", "", "path to the release manifest JSON")
	outPath := flags.String("out", "", "path to write the canonical report JSON")
	generatedAtText := flags.String("generated-at", "", "report time in RFC3339 form (defaults to now)")
	if err := flags.Parse(args); err != nil {
		return exitError
	}
	if *root == "" || *manifestPath == "" || *outPath == "" {
		_, _ = fmt.Fprintln(stderr, "filler-release-readiness: -root, -manifest, and -out are required")
		return exitError
	}

	generatedAt := now().UTC()
	if *generatedAtText != "" {
		parsed, err := time.Parse(time.RFC3339, *generatedAtText)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "filler-release-readiness: parse -generated-at: %v\n", err)
			return exitError
		}
		generatedAt = parsed.UTC()
	}
	manifestBytes, err := os.ReadFile(*manifestPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-release-readiness: read manifest: %v\n", err)
		return exitError
	}

	var evidenceFS fs.FS
	evidenceRoot, rootErr := os.OpenRoot(*root)
	if rootErr == nil {
		defer func() { _ = evidenceRoot.Close() }()
		evidenceFS = evidenceRoot.FS()
	}

	report := fillerrelease.Evaluate(evidenceFS, manifestBytes, generatedAt)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-release-readiness: encode report: %v\n", err)
		return exitError
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(*outPath, encoded, 0o644); err != nil {
		_, _ = fmt.Fprintf(stderr, "filler-release-readiness: write report: %v\n", err)
		return exitError
	}

	if report.Verdict == fillerrelease.VerdictHold {
		_, _ = fmt.Fprintf(stdout, "HOLD: %d release-readiness reason(s); report: %s\n", len(report.Holds), *outPath)
		return exitHold
	}
	_, _ = fmt.Fprintf(stdout, "GO: filler release evidence is complete; report: %s\n", *outPath)
	return exitGo
}
