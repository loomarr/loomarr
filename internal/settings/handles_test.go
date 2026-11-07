package settings

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// keys_gen.go declares exactly one handle per readable setting: named HandleName(key), typed by
// the setting's Kind, holding its key. A setting added, renamed or retyped without
// `make settings-keys` fails here. Read from the syntax tree, so layout is gofmt's business.
func TestKeysGenIsCurrent(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "keys_gen.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			for i, name := range value.Names {
				got[name.Name] = handleLiteral(t, value.Values[i])
			}
		}
	}

	want := map[string]string{}
	for _, s := range NewRegistry().All() {
		if s.MigrationOnly {
			continue
		}
		typ, err := handleType(s.Kind)
		if err != nil {
			t.Fatal(err)
		}
		want[HandleName(s.Key)] = typ + "{" + s.Key + "}"
	}

	for name, w := range want {
		if g, ok := got[name]; !ok || g != w {
			t.Errorf("keys_gen.go: %s = %q, want %q; run `make settings-keys`", name, g, w)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("keys_gen.go declares %s, which no readable setting names; run `make settings-keys`", name)
		}
	}
}

// handleLiteral renders a generated `Type{"key"}` value as `Type{key}`.
func handleLiteral(t *testing.T, expr ast.Expr) string {
	t.Helper()
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || len(lit.Elts) != 1 {
		t.Fatalf("keys_gen.go: a handle is not a one-field literal: %#v", expr)
	}
	typ, okType := lit.Type.(*ast.Ident)
	key, okKey := lit.Elts[0].(*ast.BasicLit)
	if !okType || !okKey {
		t.Fatalf("keys_gen.go: a handle is not Type{\"key\"}: %#v", lit)
	}
	unquoted, err := strconv.Unquote(key.Value)
	if err != nil {
		t.Fatal(err)
	}
	return typ.Name + "{" + unquoted + "}"
}

type mapReader map[string]any

func (m mapReader) Value(key string) any { return m[key] }

func TestHandlesReadTheirKind(t *testing.T) {
	r := mapReader{
		LibraryURL.Key():                "http://media:8096",
		FillerSourceFolderEnabled.Key(): false,
	}
	if got := LibraryURL.Get(r); got != "http://media:8096" {
		t.Errorf("LibraryURL = %q", got)
	}
	// An answered false is false, even for a default-on switch.
	if FillerSourceFolderEnabled.Get(r) {
		t.Error("FillerSourceFolderEnabled = true, want the answered false")
	}
}

// A reader that cannot answer yields the declared default, so a default-on switch stays on.
func TestHandlesFallBackToTheDeclaredDefault(t *testing.T) {
	for _, r := range []Reader{nil, mapReader{}, (*Service)(nil)} {
		if !FillerSourceFolderEnabled.Get(r) {
			t.Errorf("%T: FillerSourceFolderEnabled = false, want its declared default true", r)
		}
		// A duration declared as "6h" is 6h, not zero.
		if got := FillerFetchEvery.Get(r); got != 6*time.Hour {
			t.Errorf("%T: FillerFetchEvery = %v, want 6h", r, got)
		}
	}
}

// The fallback is the value the service resolves for an unset key, for every setting: the same
// default, parsed by the same Kind rules.
func TestHandleFallbackMatchesTheResolvedDefault(t *testing.T) {
	svc := &Service{}
	for _, s := range NewRegistry().All() {
		if s.MigrationOnly || s.DataSubdir != "" {
			continue // a data subdirectory default depends on the data dir, which the handle cannot know
		}
		want := svc.defaultResolved(s, false, false).Value
		if got := fallback(s.Key); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: fallback = %#v, resolved default = %#v", s.Key, got, want)
		}
	}
}

func TestHandleNames(t *testing.T) {
	for key, want := range map[string]string{
		"filler.vision.provider": "FillerVisionProvider",
		"llm.api_key":            "LLMAPIKey",
		"library.url":            "LibraryURL",
	} {
		if got := HandleName(key); got != want {
			t.Errorf("HandleName(%q) = %q, want %q", key, got, want)
		}
	}
}
