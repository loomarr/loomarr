// Command filler-corpus-review-package materializes one verified, identity-blind
// evidence package and an intentionally empty submission template.
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
	flags := flag.NewFlagSet("filler-corpus-review-package", flag.ContinueOnError)
	flags.SetOutput(stderr)
	draft := flags.String("draft", "", "unlabeled development or certification draft JSON")
	reviewPacket := flags.String("review-packet", "", "reviewer-visible opaque packet JSON")
	aliasMap := flags.String("alias-map", "", "owner-only alias map JSON")
	evidencePackets := flags.String("evidence-packets", "", "label-blind provider evidence packets JSONL")
	corpusRoot := flags.String("corpus-root", "", "root containing hashed media derivatives")
	output := flags.String("out", "", "new reviewer-visible package directory")
	mode := flags.String("materialize", string(fillerreview.MaterializeHardlink), "media materialization: hardlink or copy")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	result, err := fillerreview.Build(fillerreview.Config{
		DraftPath: *draft, ReviewPacketPath: *reviewPacket, AliasMapPath: *aliasMap,
		EvidencePacketsPath: *evidencePackets, CorpusRoot: *corpusRoot, OutputDir: *output,
		Mode: fillerreview.MaterializationMode(*mode),
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-corpus-review-package:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-corpus-review-package: packaged %d cases, %d files, and %d bytes; manifest sha256 %s; empty label template sha256 %s\n", result.Cases, result.Files, result.Bytes, result.ManifestSHA256, result.LabelTemplateSHA256)
	return 0
}
