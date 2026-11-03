package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"

	"github.com/loomarr/loomarr/internal/recommend"
)

// recommend-compare compares two or more channel recommendation scorecards without inference.
const recommendCompareName = "eval recommend-compare"

func init() {
	register("recommend-compare", "compare channel-recommendation scorecards; no inference",
		func(args []string) int { return runRecommendCompare(args, os.Stderr) })
}

func runRecommendCompare(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet(recommendCompareName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	outPath := flags.String("out", "", "machine-readable comparison JSON")
	summaryPath := flags.String("summary", "", "human-readable comparison Markdown")
	sharedProfile := flags.String("shared-profile", "", "profile of the shared planner model")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *outPath == "" || *summaryPath == "" || *sharedProfile == "" || flags.NArg() < 2 {
		report(stderr, "%s: --out, --summary, --shared-profile, and at least two scorecards are required\n", recommendCompareName)
		return 2
	}
	cards := make([]recommend.Scorecard, 0, flags.NArg())
	for _, path := range flags.Args() {
		blob, err := os.ReadFile(path)
		if err != nil {
			report(stderr, "%s: read %s: %v\n", recommendCompareName, path, err)
			return 1
		}
		decoder := json.NewDecoder(bytes.NewReader(blob))
		decoder.DisallowUnknownFields()
		var card recommend.Scorecard
		if err := decoder.Decode(&card); err != nil {
			report(stderr, "%s: decode %s: %v\n", recommendCompareName, path, err)
			return 1
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			report(stderr, "%s: decode %s: trailing JSON value\n", recommendCompareName, path)
			return 1
		}
		cards = append(cards, card)
	}
	comparison, err := recommend.Compare(cards, *sharedProfile)
	if err != nil {
		report(stderr, "%s: %v\n", recommendCompareName, err)
		return 1
	}
	blob, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		report(stderr, "%s: encode comparison: %v\n", recommendCompareName, err)
		return 1
	}
	blob = append(blob, '\n')
	if err := os.WriteFile(*outPath, blob, 0o600); err != nil {
		report(stderr, "%s: write %s: %v\n", recommendCompareName, *outPath, err)
		return 1
	}
	if err := os.WriteFile(*summaryPath, []byte(recommend.HumanComparison(comparison)), 0o600); err != nil {
		report(stderr, "%s: write %s: %v\n", recommendCompareName, *summaryPath, err)
		return 1
	}
	if comparison.Decision == recommend.DecisionNoCandidate || comparison.Decision == recommend.DecisionMissingShared {
		report(stderr, "%s: %s\n", recommendCompareName, comparison.Reason)
		return 1
	}
	return 0
}
