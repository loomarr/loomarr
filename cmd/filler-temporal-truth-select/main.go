// Command filler-temporal-truth-select creates the coordinator-private 48-case
// sampling ledger from the frozen legacy review history. It performs no model
// inference and emits no final truth labels.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/loomarr/loomarr/cmd/internal/fillerbakeoffio"
	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/fillerreview"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("filler-temporal-truth-select", flag.ContinueOnError)
	flags.SetOutput(stderr)
	draft := flags.String("draft", "", "frozen 300-case unlabeled draft JSON")
	seed := flags.String("seed", "", "declared deterministic selection seed")
	output := flags.String("out", "", "new coordinator-private selection JSON")
	aPackage := flags.String("a-package", "", "legacy reviewer A package manifest")
	aMap := flags.String("a-map", "", "legacy reviewer A private alias map")
	aLabels := flags.String("a-labels", "", "legacy reviewer A labels JSONL")
	bPackage := flags.String("b-package", "", "legacy reviewer B package manifest")
	bMap := flags.String("b-map", "", "legacy reviewer B private alias map")
	bLabels := flags.String("b-labels", "", "legacy reviewer B labels JSONL")
	cPackage := flags.String("c-package", "", "legacy adjudicator package manifest")
	cMap := flags.String("c-map", "", "legacy adjudicator private alias map")
	cAdjudications := flags.String("c-adjudications", "", "legacy adjudications JSONL")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	required := []*string{draft, seed, output, aPackage, aMap, aLabels, bPackage, bMap, bLabels, cPackage, cMap, cAdjudications}
	for _, value := range required {
		if *value == "" {
			_, _ = fmt.Fprintln(stderr, "filler-temporal-truth-select: --draft, --seed, --out, and all A/B/C package, map, and submission flags are required")
			return 2
		}
	}
	inputs, candidates, err := fillerreview.LoadTemporalTruthCandidateHistory(fillerreview.TemporalTruthHistoryConfig{
		DraftPath: *draft,
		Artifacts: []fillerreview.TemporalTruthHistoryArtifact{
			{Name: "legacy-a", Kind: fillerreview.TemporalTruthHistoryLabels, PackagePath: *aPackage, AliasMapPath: *aMap, SubmissionPath: *aLabels},
			{Name: "legacy-b", Kind: fillerreview.TemporalTruthHistoryLabels, PackagePath: *bPackage, AliasMapPath: *bMap, SubmissionPath: *bLabels},
			{Name: "legacy-c", Kind: fillerreview.TemporalTruthHistoryAdjudications, PackagePath: *cPackage, AliasMapPath: *cMap, SubmissionPath: *cAdjudications},
		},
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-truth-select: load history:", err)
		return 1
	}
	selection, err := fillereval.BuildTemporalTruthSelection(*seed, inputs, candidates)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-truth-select: select:", err)
		return 1
	}
	if err := fillerbakeoffio.WriteImmutableJSON(*output, ".filler-temporal-truth-selection-*", selection); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-temporal-truth-select: publish:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-temporal-truth-select: selected %d cases from %d frozen candidates in %s\n", len(selection.Cases), len(candidates), *output)
	return 0
}
