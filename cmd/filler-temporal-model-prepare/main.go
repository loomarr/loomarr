// Command filler-temporal-model-prepare creates one fresh, identity-blind
// frames/OCR/transcript package for a model-panel slot.
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
	flags := flag.NewFlagSet("filler-temporal-model-prepare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	evidence := flags.String("evidence", "", "sealed public temporal evidence manifest JSON")
	evidenceMap := flags.String("evidence-map", "", "coordinator-private temporal evidence map JSON")
	selection := flags.String("selection", "", "coordinator-private temporal evidence selection JSON")
	panelSlot := flags.String("panel-slot", "", "opaque panel slot such as panel-a")
	batch := flags.String("batch", "", "unique model batch identifier")
	seed := flags.String("seed", "", "declared deterministic blinding seed")
	preparedText := flags.String("prepared-at", "", "fixed RFC3339 preparation time")
	output := flags.String("out", "", "new model-batch directory")
	mode := flags.String("materialization", string(fillerreview.TemporalHumanReviewHardlink), "frame materialization: hardlink or copy")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	preparedAt, err := time.Parse(time.RFC3339, *preparedText)
	if err != nil || *evidence == "" || *evidenceMap == "" || *selection == "" || *panelSlot == "" || *batch == "" || *seed == "" || *output == "" {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-model-prepare: evidence, evidence map, selection, panel slot, batch, seed, fixed preparation time, and output are required")
		return 2
	}
	result, err := fillerreview.BuildTemporalModelReviewPackage(fillerreview.TemporalModelReviewPackageConfig{
		EvidenceManifestPath: *evidence, EvidencePrivateMapPath: *evidenceMap, SelectionPath: *selection,
		PanelSlot: *panelSlot, BatchID: *batch, Seed: *seed, PreparedAt: preparedAt.UTC(), OutputDir: *output,
		Materialization: fillerreview.TemporalHumanReviewMaterialization(*mode),
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-model-prepare:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-model-prepare: wrote %d blinded cases (%d frame files, %d bytes) to %s; package sha256 %s; map sha256 %s\n", result.Cases, result.Files, result.Bytes, filepath.Join(*output, "public"), result.PackageSHA256, result.MapSHA256)
	return 0
}
