package internal_test

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// "Which provider is this source from, and what does that mean?" has one answer: the provider
// registered under that ID (internal/filler/source). A comparison of a source's kind against
// "youtube" or "archive" is how a second answer starts: it works for the providers the author
// remembered, and a provider added later is silently treated as the else branch. Ask the registry:
// Registry.Get, Describe().Fetched, or a capability interface (Lister, Searcher, Finder, Addresser).
//
// The guard reads production source with go/parser over loomarrPackages (never `go list`, which the
// test cache cannot track — see importgraph_test.go). It is syntactic and flags a provider-ID
// string literal that is compared (== / !=) with, or is a switch case of, an expression whose name
// says it is a kind or a provider. It cannot see types, so an unrelated "library" compared with
// something named kind is also flagged and belongs in the allowlist with the reason.

const sourceProviderHome = "internal/filler/source/"

// providerIDLiterals are the persisted provider IDs. Constants (source.FolderID,
// archiveprovider.ID, fetch.YouTube) are not flagged: they name a provider where one is
// structurally involved, which a literal does invisibly.
var providerIDLiterals = map[string]bool{"youtube": true, "archive": true, "folder": true, "library": true}

// providerLiteralAllowlist maps a repo-relative file to why it still compares a provider-ID
// literal. It is a ratchet: a file leaves it when it stops, and cannot join it without a reason.
var providerLiteralAllowlist = map[string]string{
	"internal/api/filler/fillerwatch.go":          "drop-folder watch projection; follow-up moves it onto the registry",
	"internal/filler/acquisition/compilations.go": "filler domain rule keyed on the folder kind; follow-up passes the provider's Description instead",
	"internal/filler/curated.go":                  "filler domain rule keyed on fetched kinds; follow-up passes the provider's Description instead",
	"internal/filler/sync.go":                     "scan dispatch by kind; follow-up moves scanning onto the scanned providers",
	"internal/filler/acquisition/fetch.go":        "fetch plan excludes scanned kinds; follow-up asks the registry",
	"internal/filler/research/model.go":           "maps a research model's provider names, which include aliases such as archive.org",
	"internal/filler/records/fillersources.go":    "the store's Fetchable/Scannable split by kind; follow-up asks the registry",
	"cmd/demo-library/seed.go":                    "an HTTP client of the API that seeds the demo; it reads the wire kind and has no registry",
}

// kindishName reports whether an expression is named like a source kind or a provider: the final
// identifier of a selector, call or index, lower-cased, mentions "kind" or "provider".
func kindishName(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return kindishWord(v.Name)
	case *ast.SelectorExpr:
		return kindishWord(v.Sel.Name)
	case *ast.CallExpr:
		// string(x.Kind), strings.ToLower(kind): look through the conversion or the wrapped value.
		for _, arg := range v.Args {
			if kindishName(arg) {
				return true
			}
		}
		return kindishName(v.Fun)
	case *ast.ParenExpr:
		return kindishName(v.X)
	}
	return false
}

func kindishWord(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "kind") || strings.Contains(name, "provider")
}

func providerLiteral(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil && providerIDLiterals[value]
}

// providerLiteralComparisons returns "literal@line" for every provider-ID literal compared with a
// kind-named expression in src.
func providerLiteralComparisons(t *testing.T, filename string, src any) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	var found []string
	flag := func(value string, at token.Pos) {
		found = append(found, value+"@"+strconv.Itoa(fset.Position(at).Line))
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.BinaryExpr:
			if v.Op != token.EQL && v.Op != token.NEQ {
				return true
			}
			if value, ok := providerLiteral(v.Y); ok && kindishName(v.X) {
				flag(value, v.Pos())
			} else if value, ok := providerLiteral(v.X); ok && kindishName(v.Y) {
				flag(value, v.Pos())
			}
		case *ast.SwitchStmt:
			if v.Tag == nil || !kindishName(v.Tag) {
				return true
			}
			for _, stmt := range v.Body.List {
				for _, expr := range stmt.(*ast.CaseClause).List {
					if value, ok := providerLiteral(expr); ok {
						flag(value, expr.Pos())
					}
				}
			}
		}
		return true
	})
	return found
}

// productionProviderLiteralUses maps each repo-relative production file outside
// internal/filler/source to its provider-ID literal comparisons.
func productionProviderLiteralUses(t *testing.T, pkgs map[string]*build.Package) map[string][]string {
	t.Helper()
	uses := map[string][]string{}
	for importPath, pkg := range pkgs {
		rel := strings.TrimPrefix(importPath, modulePath+"/")
		for _, name := range pkg.GoFiles {
			file := filepath.ToSlash(filepath.Join(rel, name))
			if strings.HasPrefix(file, sourceProviderHome) {
				continue
			}
			if found := providerLiteralComparisons(t, filepath.Join(pkg.Dir, name), nil); len(found) > 0 {
				uses[file] = found
			}
		}
	}
	return uses
}

func TestProviderIDsAreComparedOnlyInsideSourceProvider(t *testing.T) {
	uses := productionProviderLiteralUses(t, loomarrPackages(t))
	var files []string
	for file := range uses {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		if _, ok := providerLiteralAllowlist[file]; !ok {
			t.Errorf("%s compares a provider ID literal (%s) — ask the registry (Registry.Get, "+
				"Describe().Fetched, or a capability interface) instead; if this is not a filler source "+
				"provider, add the file to providerLiteralAllowlist with the reason",
				file, strings.Join(uses[file], ", "))
		}
	}
}

func TestProviderLiteralAllowlistIsCurrent(t *testing.T) {
	uses := productionProviderLiteralUses(t, loomarrPackages(t))
	for file, reason := range providerLiteralAllowlist {
		if reason == "" {
			t.Errorf("%s is allowlisted without a reason", file)
		}
		if len(uses[file]) == 0 {
			t.Errorf("%s no longer compares a provider ID literal — drop it from providerLiteralAllowlist", file)
		}
	}
}

func TestProviderLiteralScanFlagsEveryComparisonForm(t *testing.T) {
	src := `package p

func a(src S) bool        { return src.Kind == "youtube" }
func b(kind string) bool  { return "archive" != kind }
func c(row R) bool        { return (row.Kind == "archive" || row.Kind == "youtube") && row.ok }
func d(k K) string {
	switch k.Provider {
	case "folder", "library":
		return "x"
	case "vimeo":
		return "y"
	}
	switch string(k.Kind) {
	case "youtube":
		return "z"
	}
	return ""
}
func e(src S) bool        { return src.Name == "youtube" }
func f(kind Kind) bool    { return kind == YouTube }
func g(m map[string]int) int { return m["archive"] }
func h(in In) bool        { switch in.ID { case "folder": return true }; return false }
`
	got := strings.Join(providerLiteralComparisons(t, "p.go", src), " ")
	want := "youtube@3 archive@4 archive@5 youtube@5 folder@8 library@8 youtube@14"
	if got != want {
		t.Fatalf("flagged %q, want %q", got, want)
	}
}
