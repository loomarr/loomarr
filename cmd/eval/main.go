// Command eval runs the on-demand evaluation tools: model certification, scorecard comparison,
// protocol diagnostics and provider-free evidence publication. Each tool is one subcommand with its
// own flags. They are run by hand through mk/eval.mk; none of them is part of a gate.
//
// planner-compare reads internal/eval, which builds only with -tags=eval; without that tag the
// subcommand exists but refuses to run.
package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// planner-compare compares two or more frozen planner scorecards without inference. Both its
// tagged implementation (planner_compare.go) and its untagged refusal use these.
const (
	plannerCompareName    = "eval planner-compare"
	plannerCompareSummary = "compare frozen planner scorecards; no inference (needs -tags=eval)"
)

type subcommand struct {
	summary string
	run     func(args []string) int
}

// subcommands is filled by each tool's file so a build-tagged tool registers only when it compiles.
var subcommands = map[string]subcommand{}

func register(name, summary string, run func(args []string) int) {
	if _, exists := subcommands[name]; exists {
		panic("eval: subcommand registered twice: " + name)
	}
	subcommands[name] = subcommand{summary: summary, run: run}
}

func main() { os.Exit(dispatch(os.Args[1:], os.Stderr)) }

func dispatch(args []string, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "help", "-h", "-help", "--help":
		usage(stderr)
		return 0
	}
	command, ok := subcommands[args[0]]
	if !ok {
		report(stderr, "eval: unknown subcommand %q\n", args[0])
		usage(stderr)
		return 2
	}
	return command.run(args[1:])
}

func usage(w io.Writer) {
	names := make([]string, 0, len(subcommands))
	width := 0
	for name := range subcommands {
		names = append(names, name)
		width = max(width, len(name))
	}
	slices.Sort(names)
	var b strings.Builder
	b.WriteString("usage: eval <subcommand> [flags]\n\nsubcommands:\n")
	for _, name := range names {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, name, subcommands[name].summary)
	}
	b.WriteString("\nRun eval <subcommand> -h for its flags.\n")
	_, _ = io.WriteString(w, b.String())
}

func report(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
