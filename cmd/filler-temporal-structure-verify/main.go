// Command filler-temporal-structure-verify independently re-establishes the
// public/private authority join for a materialized temporal challenge.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-temporal-structure-verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	public := flags.String("public", "", "public temporal-structure manifest JSON")
	authority := flags.String("authority", "", "coordinator-private temporal-structure authority JSON")
	cases := flags.Int("cases", 0, "exact expected case count")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *public == "" || *authority == "" || *cases <= 0 {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-verify: public manifest, private authority, and positive exact case count are required")
		return 2
	}
	manifest, _, publicSHA, authoritySHA, err := fillerreview.LoadTemporalStructureChallenge(*public, *authority, *cases)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-structure-verify:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-structure-verify: verified %d cases; public manifest sha256 %s; private authority sha256 %s\n", len(manifest.Cases), publicSHA, authoritySHA)
	return 0
}
