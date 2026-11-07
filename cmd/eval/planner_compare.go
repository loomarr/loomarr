//go:build eval

package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"

	eval "github.com/loomarr/loomarr/internal/eval"
)

func init() {
	register("planner-compare", plannerCompareSummary, func(args []string) int { return runPlannerCompare(args, os.Stderr) })
}

func runPlannerCompare(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet(plannerCompareName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	outPath := flags.String("out", "", "machine-readable comparison JSON")
	summaryPath := flags.String("summary", "", "human-readable comparison Markdown")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *outPath == "" || *summaryPath == "" || flags.NArg() < 2 {
		report(stderr, "%s: --out, --summary, and at least two scorecards are required\n", plannerCompareName)
		return 2
	}
	cards := make([]eval.Scorecard, 0, flags.NArg())
	for _, path := range flags.Args() {
		blob, err := os.ReadFile(path)
		if err != nil {
			report(stderr, "%s: read %s: %v\n", plannerCompareName, path, err)
			return 1
		}
		var card eval.Scorecard
		if err := json.Unmarshal(blob, &card); err != nil {
			report(stderr, "%s: decode %s: %v\n", plannerCompareName, path, err)
			return 1
		}
		cards = append(cards, card)
	}
	comparison, err := eval.ComparePlannerModels(cards)
	if err != nil {
		report(stderr, "%s: %v\n", plannerCompareName, err)
		return 1
	}
	blob, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		report(stderr, "%s: encode comparison: %v\n", plannerCompareName, err)
		return 1
	}
	blob = append(blob, '\n')
	if err := os.WriteFile(*outPath, blob, 0o600); err != nil {
		report(stderr, "%s: write %s: %v\n", plannerCompareName, *outPath, err)
		return 1
	}
	if err := os.WriteFile(*summaryPath, []byte(eval.PlannerComparisonSummary(comparison)), 0o600); err != nil {
		report(stderr, "%s: write %s: %v\n", plannerCompareName, *summaryPath, err)
		return 1
	}
	if comparison.DecisionStatus == "no_eligible" {
		report(stderr, "%s: %s\n", plannerCompareName, comparison.DecisionReason)
		return 1
	}
	return 0
}
