// Command filler-media-integrity-prepare builds the label-free public package and private authority
// map for the purpose-built media-integrity challenge. It performs no inference or media access.
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
	flags := flag.NewFlagSet("filler-media-integrity-prepare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	authority := flags.String("authority", "", "private closed-label authority JSON")
	quality := flags.String("media-quality", "", "exact full-decode media-quality report JSON")
	seed := flags.String("seed", "", "fresh private HMAC seed")
	preparedText := flags.String("prepared-at", "", "fixed RFC3339 preparation time")
	output := flags.String("out", "", "new challenge output directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	preparedAt, err := time.Parse(time.RFC3339, *preparedText)
	if err != nil || *authority == "" || *quality == "" || *seed == "" || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-media-integrity-prepare: authority, media quality, private seed, fixed preparation time, and output are required")
		return 2
	}
	result, err := fillerreview.BuildMediaIntegrityChallenge(fillerreview.MediaIntegrityChallengeBuildConfig{AuthoringPath: *authority, MediaQualityPath: *quality, Seed: *seed, PreparedAt: preparedAt.UTC(), OutputDir: *output})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-media-integrity-prepare:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-media-integrity-prepare: prepared %d cases; package sha256 %s; private map sha256 %s; production admission false\n", result.Cases, result.PackageSHA256, result.MapSHA256)
	return 0
}
