package main

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// modulePath prefixes every internal import. Used to tell "our package" from a
// third-party one without asking the build system anything.
const modulePath = "github.com/mantonx/loomarr/internal/"

// Package is one row of the generated map: what it is, and what it sits on.
//
// Deliberately carries NO size metric. Line counts change on nearly every commit,
// so emitting them would make `make arch-docs-verify` red on unrelated work — and a
// guard that is red by default gets regenerated blindly, which is worse than not
// having it. §14.1's own gate file rejects size thresholds for the same reason.
// Structure is what this map is for, and structure moves rarely.
type Package struct {
	Name string
	// Synopsis is the first sentence of the package doc, with the Go-idiomatic
	// "Package foo is " prefix stripped so the rendered list reads as prose.
	Synopsis string
	// Imports is the sorted set of OTHER internal packages this one imports.
	// Third-party and stdlib imports are dropped: the question this map answers is
	// how Loomarr's own pieces sit against each other.
	Imports []string
	// Layer is the longest path from this package to a leaf. It is the honest
	// version of "which tier is this" — derived from the graph rather than from a
	// human's idea of which tier a package ought to be in.
	Layer int
}

// scan parses every package under internal/ and returns them sorted by name.
//
// Uses go/parser rather than golang.org/x/tools/go/packages: everything needed here
// (doc comment, import paths) is in the syntax tree, and x/tools is only a transitive
// dependency today. Promoting it to a direct one would be a §14 conversation for a
// capability the standard library already provides.
func scan(root string) ([]Package, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	byName := map[string]*Package{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := scanOne(filepath.Join(root, e.Name()), e.Name())
		if err != nil {
			return nil, err
		}
		// A directory with no non-test Go files (internal/integration is one) has no
		// production surface to describe. Skipping keeps it out of the map rather
		// than rendering an empty row that reads like an omission.
		if p == nil {
			continue
		}
		byName[e.Name()] = p
	}

	// Drop edges to packages that are not in the map (test-only dirs), so a rendered
	// dependency can always be followed to a row that exists.
	for _, p := range byName {
		kept := p.Imports[:0]
		for _, dep := range p.Imports {
			if _, ok := byName[dep]; ok {
				kept = append(kept, dep)
			}
		}
		p.Imports = kept
	}

	assignLayers(byName)

	out := make([]Package, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// scanOne parses one package directory. Returns nil when the directory holds no
// non-test Go files.
//
// Walks the directory itself rather than using parser.ParseDir, which is deprecated
// (SA1019) for a reason that applies here: it ignores build tags when grouping files
// into packages. The pairing of go/parser.ParseFile with doc.NewFromFiles is the
// supported path and needs no dependency beyond the standard library.
func scanOne(dir, name string) (*Package, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var files []*ast.File
	imports := map[string]bool{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		// An external test package (foo_test) in a non-_test.go file would be odd;
		// ignore it rather than let it supply the synopsis.
		if strings.HasSuffix(f.Name.Name, "_test") {
			continue
		}
		files = append(files, f)
		collectInternalImports(f, name, imports)
	}
	if len(files) == 0 {
		return nil, nil
	}

	synopsis := ""
	if d, err := doc.NewFromFiles(fset, files, "./"+name); err == nil && d != nil {
		synopsis = firstSentence(d.Doc)
	}

	deps := make([]string, 0, len(imports))
	for dep := range imports {
		deps = append(deps, dep)
	}
	sort.Strings(deps)

	return &Package{Name: name, Synopsis: synopsis, Imports: deps}, nil
}

// collectInternalImports records every internal/<pkg> this file imports, except
// itself (a subpackage importing its parent would otherwise self-edge).
func collectInternalImports(f *ast.File, self string, into map[string]bool) {
	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		rest, ok := strings.CutPrefix(path, modulePath)
		if !ok {
			continue
		}
		// internal/foo/bar counts as a dependency on foo.
		top, _, _ := strings.Cut(rest, "/")
		if top == "" || top == self {
			continue
		}
		into[top] = true
	}
}

// assignLayers computes each package's longest path to a leaf. Go forbids import
// cycles, so the graph is a DAG and the recursion terminates; the `visiting` guard
// exists only so a malformed graph fails loudly instead of hanging.
func assignLayers(byName map[string]*Package) {
	var depth func(name string, visiting map[string]bool) int
	memo := map[string]int{}

	depth = func(name string, visiting map[string]bool) int {
		if d, ok := memo[name]; ok {
			return d
		}
		if visiting[name] {
			return 0 // defensive: only reachable if Go's own cycle rule were violated
		}
		visiting[name] = true
		p := byName[name]
		max := 0
		for _, dep := range p.Imports {
			if d := depth(dep, visiting) + 1; d > max {
				max = d
			}
		}
		delete(visiting, name)
		memo[name] = max
		return max
	}

	for name, p := range byName {
		p.Layer = depth(name, map[string]bool{})
	}
}

// firstSentence returns the first sentence of a package doc, with the Go-idiomatic
// "Package foo is/are/implements " opener stripped.
//
// Hand-rolled rather than go/doc.Synopsis because that helper does not strip the
// prefix, and because the rendered map reads as a list of descriptions rather than
// a list of sentences each starting with the word it is already labelled by.
func firstSentence(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return ""
	}
	// Collapse the comment's hard wrapping so sentence detection is not fooled by a
	// newline mid-sentence.
	d = strings.Join(strings.Fields(d), " ")

	end := len(d)
	for i, r := range d {
		if r != '.' {
			continue
		}
		// A period inside "§9.1", "v2.39" or "e.g." does not end a sentence.
		if i+1 < len(d) && d[i+1] != ' ' {
			continue
		}
		if i > 0 && isAbbrevBoundary(d[:i]) {
			continue
		}
		end = i + 1
		break
	}
	s := strings.TrimSpace(d[:end])

	for _, prefix := range []string{
		"Package " + firstWord(s) + " is the ",
		"Package " + firstWord(s) + " is ",
		"Package " + firstWord(s) + " are ",
		"Package " + firstWord(s) + " implements ",
		"Package " + firstWord(s) + " provides ",
		"Package " + firstWord(s) + " holds ",
		"Package " + firstWord(s) + " ",
	} {
		if rest, ok := strings.CutPrefix(s, prefix); ok && rest != "" {
			return strings.ToUpper(rest[:1]) + rest[1:]
		}
	}
	return s
}

// firstWord returns the second word of "Package foo …" — i.e. the package name — so
// the prefix table above can be built without knowing the name separately.
func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

// isAbbrevBoundary reports whether the text ending here looks like an abbreviation
// ("e.g", "i.e", "cf") rather than a sentence end.
func isAbbrevBoundary(before string) bool {
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return false
	}
	last := fields[len(fields)-1]
	switch strings.ToLower(last) {
	case "e.g", "i.e", "cf", "vs", "etc", "no":
		return true
	}
	// A single capital letter ("A.") is an initial, not a sentence end.
	return len(last) == 1 && last[0] >= 'A' && last[0] <= 'Z'
}
