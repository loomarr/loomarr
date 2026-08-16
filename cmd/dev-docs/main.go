// Command dev-docs generates docs/dev/commands.md from the Makefile's own `## ` doc
// comments and the CI workflow's `make` invocations. `make dev-docs` runs it;
// `make dev-docs-verify` diffs the result so the tree goes red when they drift — the
// same committed-artifact discipline as api/openapi.yaml and docs/configuration.md.
//
// WHY THIS EXISTS. The command contract was restated by hand in four files — README.md,
// contributor and agent entrypoints — and they disagreed with each other and with
// the tree: the Go version (1.22 vs 1.26), the Node version (20 vs 22.5), what `make fe`
// runs ("Storybook browser tests" that do not exist), and the visual-suite size, stated
// three ways with none correct. Twenty-one targets appeared in none of them, including
// `make fe-install`, without which the documented clean-clone path simply fails.
//
// Prose could not hold this. Every one of those facts already existed in machine-readable
// form; the only reason they drifted is that a human had to copy them. So they are read:
// the Makefile is the source for what a target does, and ci.yml is the source for whether
// CI runs it — which retires the hand-written "CI mirrors: …" list that was itself wrong.
package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// target is one documented Makefile entry.
type target struct {
	Name    string
	Deps    []string // prerequisites, so `check: fmt vet vet-tags lint test` shows what it runs
	Doc     string   // the text after `## `
	Section string
	InCI    bool
}

var (
	// A target line: name, optional prerequisites, then `## ` and the doc. Anchored at
	// column 0 so the `help` recipe's own grep pattern (which is tab-indented and
	// contains a literal `## `) is not mistaken for a target.
	targetLine = regexp.MustCompile(`^([a-zA-Z0-9_-]+):([^#]*)##\s*(.+)$`)
	// A section banner: `## ---- the default gate ----------------------------------`
	sectionLine = regexp.MustCompile(`^##\s*-+\s*(.+?)\s*-+\s*$`)
	// `make check`, `make fe-visual PW_SHARD=…`, `make test-pg` inside a workflow.
	makeInvocation = regexp.MustCompile(`\bmake\s+([a-z][a-z0-9-]*)`)
)

func main() {
	out := "docs/dev/commands.md"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	targets, err := parseMakefile("Makefile")
	if err != nil {
		fail("parse Makefile", err)
	}
	ci, err := ciTargets(".github/workflows")
	if err != nil {
		fail("scan workflows", err)
	}
	for i := range targets {
		if _, ok := ci[targets[i].Name]; ok {
			targets[i].InCI = true
		}
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fail("create output dir", err)
	}
	if err := os.WriteFile(out, render(targets), 0o644); err != nil {
		fail("write dev docs", err)
	}
}

func fail(what string, err error) {
	slog.Error(what, "err", err)
	os.Exit(1)
}

func parseMakefile(path string) ([]target, error) {
	f, err := os.Open(path) //nolint:gosec // a fixed path in the repo root
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var (
		out     []target
		section = "General"
	)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := sectionLine.FindStringSubmatch(line); m != nil {
			section = strings.TrimSpace(m[1])
			continue
		}
		m := targetLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out = append(out, target{
			Name:    m[1],
			Deps:    strings.Fields(m[2]),
			Doc:     strings.TrimSpace(m[3]),
			Section: section,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		// A parser that silently matches nothing would generate an empty page and a
		// clean `dev-docs-verify` forever after — the vacuous-green failure this repo
		// keeps rediscovering. Refuse instead.
		return nil, fmt.Errorf("no documented targets found in %s; the parser is broken, not the Makefile", path)
	}
	return out, nil
}

// ciTargets collects every `make <target>` any workflow invokes. This is what makes the
// "runs in CI" column impossible to drift: the previous hand-written list ("CI mirrors:
// check + openapi-verify + test-pg + fe + e2e") omitted six real gates.
func ciTargets(dir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]struct{}{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") && !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // workflow dir
		if err != nil {
			return nil, err
		}
		for _, m := range makeInvocation.FindAllStringSubmatch(stripYAMLComments(string(body)), -1) {
			out[m[1]] = struct{}{}
		}
	}
	return out, nil
}

// stripYAMLComments drops whole-line comments before the `make` scan.
//
// ⚠ Without this the CI column LIES, and it did: these workflows are heavily commented, and a
// comment reading "…already verified by `make dev-docs-verify`" made that target report as a
// CI gate while no step ran it. A generated page that overstates coverage is worse than a
// hand-written one, because it is trusted — so the scan reads steps, not prose about steps.
//
// Whole-line only, deliberately. A trailing `# comment` after real YAML is rare in these files,
// and stripping mid-line would risk cutting a legitimate `#` inside a quoted shell string.
func stripYAMLComments(body string) string {
	lines := strings.Split(body, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func render(targets []target) []byte {
	var b strings.Builder
	b.WriteString("# Command reference\n\n")
	b.WriteString("<!-- GENERATED by `make dev-docs` from the Makefile and .github/workflows. DO NOT EDIT. -->\n\n")
	b.WriteString("Every `make` target, its description, and whether CI runs it.\n")
	b.WriteString("Both columns are read from source — the descriptions from the Makefile's own `##`\n")
	b.WriteString("comments, the CI column from `make` invocations in the workflows — so this page\n")
	b.WriteString("cannot drift from either. `make dev-docs-verify` fails the build if it does.\n\n")
	// The CI column is derived from workflow invocations BY NAME, so `test` and `lint` are
	// blank despite running on every PR. Without this line a reader concludes CI does not run
	// the unit tests — a false claim, generated, and therefore trusted.
	b.WriteString("**✅ means a workflow invokes that target by name.** A blank cell is not\n")
	b.WriteString("\"never runs in CI\" — `fmt`, `vet`, `lint` and `test` all run as prerequisites\n")
	b.WriteString("of `make check`. The *runs:* note on a row lists what it pulls in.\n\n")
	b.WriteString("**The default gate is `make check`.** Run it before every push.\n\n")

	// Preserve Makefile order for sections; a stable order keeps the diff readable when
	// a target is added, which is the whole point of committing generated output.
	var sections []string
	bySection := map[string][]target{}
	for _, t := range targets {
		if _, seen := bySection[t.Section]; !seen {
			sections = append(sections, t.Section)
		}
		bySection[t.Section] = append(bySection[t.Section], t)
	}

	for _, s := range sections {
		fmt.Fprintf(&b, "## %s\n\n", strings.ToUpper(s[:1])+s[1:])
		b.WriteString("| Target | CI | What it does |\n| --- | --- | --- |\n")
		for _, t := range bySection[s] {
			ci := ""
			if t.InCI {
				ci = "✅"
			}
			doc := t.Doc
			if len(t.Deps) > 0 {
				doc += fmt.Sprintf(" <br>*runs:* `%s`", strings.Join(t.Deps, "` `"))
			}
			fmt.Fprintf(&b, "| `make %s` | %s | %s |\n", t.Name, ci, doc)
		}
		b.WriteString("\n")
	}

	var ciNames []string
	for _, t := range targets {
		if t.InCI {
			ciNames = append(ciNames, "`"+t.Name+"`")
		}
	}
	sort.Strings(ciNames)
	fmt.Fprintf(&b, "## What CI runs\n\n%s\n", strings.Join(ciNames, " · "))
	b.WriteString("\nThese are the targets a workflow step invokes DIRECTLY. Their prerequisites run too —\n")
	b.WriteString("`fmt`, `vet`, `vet-tags`, `lint` and `test` are all covered by `check` — so read the\n")
	b.WriteString("*runs:* line in each table above for the full picture.\n\n")
	b.WriteString("A target absent from both is yours to run deliberately: the maintainer smoke suites\n")
	b.WriteString("and the tagged builds are not gates and never run unattended.\n")
	return []byte(b.String())
}
