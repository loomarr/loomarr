//go:build eval

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	eval "github.com/loomarr/loomarr/internal/eval"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("planner-cert-compare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	outPath := flags.String("out", "", "machine-readable comparison JSON")
	summaryPath := flags.String("summary", "", "human-readable comparison Markdown")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *outPath == "" || *summaryPath == "" || flags.NArg() < 2 {
		fmt.Fprintln(stderr, "planner-cert-compare: --out, --summary, and at least two scorecards are required")
		return 2
	}
	cards := make([]eval.Scorecard, 0, flags.NArg())
	for _, path := range flags.Args() {
		blob, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "planner-cert-compare: read %s: %v\n", path, err)
			return 1
		}
		var card eval.Scorecard
		if err := json.Unmarshal(blob, &card); err != nil {
			fmt.Fprintf(stderr, "planner-cert-compare: decode %s: %v\n", path, err)
			return 1
		}
		cards = append(cards, card)
	}
	comparison, err := eval.ComparePlannerModels(cards)
	if err != nil {
		fmt.Fprintf(stderr, "planner-cert-compare: %v\n", err)
		return 1
	}
	blob, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "planner-cert-compare: encode comparison: %v\n", err)
		return 1
	}
	blob = append(blob, '\n')
	if err := os.WriteFile(*outPath, blob, 0o600); err != nil {
		fmt.Fprintf(stderr, "planner-cert-compare: write %s: %v\n", *outPath, err)
		return 1
	}
	if err := os.WriteFile(*summaryPath, []byte(eval.PlannerComparisonSummary(comparison)), 0o600); err != nil {
		fmt.Fprintf(stderr, "planner-cert-compare: write %s: %v\n", *summaryPath, err)
		return 1
	}
	if comparison.DecisionStatus == "no_eligible" {
		fmt.Fprintf(stderr, "planner-cert-compare: %s\n", comparison.DecisionReason)
		return 1
	}
	return 0
}
