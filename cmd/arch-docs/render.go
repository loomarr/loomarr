package main

import (
	"fmt"
	"sort"
	"strings"
)

// beginMarker/endMarker delimit the generated block inside docs/design.md §2.
//
// A marker pair rather than whole-file generation (which is how docs/configuration.md
// works): design.md is hand-written and authoritative for behaviour, so this tool owns
// one block inside it and must not be able to touch a word outside that block.
const (
	beginMarker = "<!-- BEGIN GENERATED: package-map — `make arch-docs`. DO NOT EDIT BY HAND. -->"
	endMarker   = "<!-- END GENERATED: package-map -->"
)

// spineFanIn is the importer count at which a package is considered part of the
// load-bearing spine and appears in the diagram.
//
// The diagram deliberately shows the SPINE, not the whole graph: 36 nodes and ~90
// edges renders as a hairball that nobody reads, and an unreadable diagram in an
// orientation document is worse than none — it looks like the answer while conveying
// nothing. The full edge set is in the list below it, which is greppable.
const spineFanIn = 5

// render produces the generated block for design.md §2.
//
// Deterministic by construction — every list is sorted — so the CI drift check is
// stable and a regeneration with no source change produces a byte-identical file.
func render(pkgs []Package) string {
	var b strings.Builder

	b.WriteString(beginMarker + "\n\n")
	b.WriteString("#### Package map\n\n")
	b.WriteString("Generated recursively from each package's own doc comment and exact internal imports, ")
	b.WriteString("using full paths below `internal/`, so it cannot drift from the code the way a ")
	b.WriteString("hand-maintained list does. **Layer** is derived: the longest ")
	b.WriteString("path from that package to one with no internal dependencies. It is the measured ")
	b.WriteString("layering, not an aspirational one.\n\n")
	b.WriteString("Sizes are deliberately absent — they change on nearly every commit, which would make ")
	b.WriteString("the drift check red by default and train everyone to regenerate without reading.\n\n")

	fanIn := fanInCounts(pkgs)

	b.WriteString(renderSpine(pkgs, fanIn))
	b.WriteString(renderLayers(pkgs, fanIn))

	b.WriteString("\n" + endMarker)
	return b.String()
}

// renderSpine lists the packages that most of the tree depends on.
func renderSpine(pkgs []Package, fanIn map[string]int) string {
	spine := map[string]bool{}
	for _, p := range pkgs {
		if fanIn[p.Name] >= spineFanIn {
			spine[p.Name] = true
		}
	}
	if len(spine) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("##### Dependency spine\n\n")
	fmt.Fprintf(&b, "Packages imported by %d or more others, and their dependencies within the spine. ",
		spineFanIn)
	b.WriteString("Everything else in the tree sits on top of these packages.\n\n")
	b.WriteString("| Package | Direct importers | Depends on |\n")
	b.WriteString("| --- | ---: | --- |\n")

	byName := make(map[string]Package, len(pkgs))
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	names := sortedKeys(spine)
	for _, n := range names {
		var deps []string
		for _, dep := range byName[n].Imports {
			if spine[dep] {
				deps = append(deps, dep)
			}
		}
		dependencyList := "—"
		if len(deps) > 0 {
			dependencyList = codeList(deps)
		}
		fmt.Fprintf(&b, "| `%s` | %d | %s |\n", n, fanIn[n], dependencyList)
	}
	b.WriteString("\n")
	return b.String()
}

// renderLayers lists every package grouped by its derived layer.
func renderLayers(pkgs []Package, fanIn map[string]int) string {
	byLayer := map[int][]Package{}
	maxLayer := 0
	for _, p := range pkgs {
		byLayer[p.Layer] = append(byLayer[p.Layer], p)
		if p.Layer > maxLayer {
			maxLayer = p.Layer
		}
	}

	var b strings.Builder
	b.WriteString("##### Every package, by layer\n\n")

	for layer := 0; layer <= maxLayer; layer++ {
		group := byLayer[layer]
		if len(group) == 0 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].Name < group[j].Name })

		if layer == 0 {
			b.WriteString("**Layer 0** — no internal dependencies. These are the vocabulary the rest agrees on.\n\n")
		} else {
			fmt.Fprintf(&b, "**Layer %d**\n\n", layer)
		}
		for _, p := range group {
			fmt.Fprintf(&b, "- **`%s`**", p.Name)
			if n := fanIn[p.Name]; n > 0 {
				fmt.Fprintf(&b, " · %d %s", n, plural(n, "importer"))
			}
			if len(p.Imports) > 0 {
				fmt.Fprintf(&b, " · → %s", codeList(p.Imports))
			}
			b.WriteString("\n")
			if p.Synopsis != "" {
				fmt.Fprintf(&b, "  %s\n", p.Synopsis)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// fanInCounts counts, for each package, how many others import it directly.
func fanInCounts(pkgs []Package) map[string]int {
	counts := map[string]int{}
	for _, p := range pkgs {
		for _, dep := range p.Imports {
			counts[dep]++
		}
	}
	return counts
}

// splice replaces the generated block in doc, or appends it to the named section
// when no block exists yet. Returns an error rather than guessing if the markers are
// malformed — a half-matched marker pair would silently eat hand-written prose.
func splice(doc, block, anchor string) (string, error) {
	start := strings.Index(doc, beginMarker)
	end := strings.Index(doc, endMarker)

	switch {
	case start >= 0 && end > start:
		return doc[:start] + block + doc[end+len(endMarker):], nil
	case start >= 0 || end >= 0:
		return "", fmt.Errorf("design.md has one half of the generated-block marker pair but not the other; " +
			"repair it by hand — regenerating over a malformed pair would delete prose")
	}

	// First run: insert after the anchor heading's section.
	idx := strings.Index(doc, anchor)
	if idx < 0 {
		return "", fmt.Errorf("anchor %q not found in design.md", anchor)
	}
	// Insert immediately before the next top-level heading after the anchor, so the
	// block lands at the end of §2 rather than in the middle of its prose.
	rest := doc[idx+len(anchor):]
	next := strings.Index(rest, "\n## ")
	if next < 0 {
		return strings.TrimRight(doc, "\n") + "\n\n" + block + "\n", nil
	}
	at := idx + len(anchor) + next
	// Trim the section's trailing blank lines before splicing, so the block lands with
	// exactly one blank line on each side however the hand-written prose above it ends.
	// doc[at:] already begins with the newline that starts the next heading.
	return strings.TrimRight(doc[:at], "\n") + "\n\n" + block + "\n" + doc[at:], nil
}

// plural is the tiny grammar helper the map needs so "1 importers" never ships in a
// document whose whole job is to be read.
func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func codeList(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "`" + n + "`"
	}
	return strings.Join(out, ", ")
}
