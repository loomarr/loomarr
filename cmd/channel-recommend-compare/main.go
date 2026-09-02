package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/loomarr/loomarr/internal/recommend"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("channel-recommend-compare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	outPath := flags.String("out", "", "machine-readable comparison JSON")
	summaryPath := flags.String("summary", "", "human-readable comparison Markdown")
	sharedProfile := flags.String("shared-profile", "", "profile of the shared planner model")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *outPath == "" || *summaryPath == "" || *sharedProfile == "" || flags.NArg() < 2 {
		report(stderr, "channel-recommend-compare: --out, --summary, --shared-profile, and at least two scorecards are required\n")
		return 2
	}
	cards := make([]recommend.Scorecard, 0, flags.NArg())
	for _, path := range flags.Args() {
		blob, err := os.ReadFile(path)
		if err != nil {
			report(stderr, "channel-recommend-compare: read %s: %v\n", path, err)
			return 1
		}
		decoder := json.NewDecoder(bytes.NewReader(blob))
		decoder.DisallowUnknownFields()
		var card recommend.Scorecard
		if err := decoder.Decode(&card); err != nil {
			report(stderr, "channel-recommend-compare: decode %s: %v\n", path, err)
			return 1
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			report(stderr, "channel-recommend-compare: decode %s: trailing JSON value\n", path)
			return 1
		}
		cards = append(cards, card)
	}
	comparison, err := recommend.Compare(cards, *sharedProfile)
	if err != nil {
		report(stderr, "channel-recommend-compare: %v\n", err)
		return 1
	}
	blob, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		report(stderr, "channel-recommend-compare: encode comparison: %v\n", err)
		return 1
	}
	blob = append(blob, '\n')
	if err := os.WriteFile(*outPath, blob, 0o600); err != nil {
		report(stderr, "channel-recommend-compare: write %s: %v\n", *outPath, err)
		return 1
	}
	if err := os.WriteFile(*summaryPath, []byte(recommend.HumanComparison(comparison)), 0o600); err != nil {
		report(stderr, "channel-recommend-compare: write %s: %v\n", *summaryPath, err)
		return 1
	}
	if comparison.Decision == recommend.DecisionNoCandidate || comparison.Decision == recommend.DecisionMissingShared {
		report(stderr, "channel-recommend-compare: %s\n", comparison.Reason)
		return 1
	}
	return 0
}

func report(writer io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(writer, format, args...)
}
