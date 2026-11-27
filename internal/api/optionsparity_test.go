package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// ⚠ **THE OPTIONS→AREA COPY IS HAND-WRITTEN**: `Router` reads each Options field (top-level, or
// inside an area struct such as `opts.Playout.Observer`) and assigns it to the area package's Deps
// or to Server. A missed line is SILENT: the field takes its zero value, and per the field's
// nil-semantics that may mean 501, 404, a route not mounted, a no-op — or, for the config gates, a
// gate that stops applying.
//
// This asserts every Options field, including every field of every area struct, is read somewhere
// in api.go as a selector chain rooted at `opts`.
//
// ⚠ **It reads the SOURCE, and that is a deliberate choice.** The natural test is to fill each
// Options field with a non-nil value and check it arrives — but most fields are service
// INTERFACES, and reflection cannot synthesise an implementation of an arbitrary method set
// (`reflect.StructOf` does not promote methods of an embedded interface). A value-based version
// would skip precisely the fields most likely to be missed, and checking a fraction while
// reporting success is worse than not testing.
//
// Deliberately NOT solved by embedding Options into Server. That makes the copy one line, but
// makes every dependency settable from `api_test`. Zero tests call handlers directly; all go
// through `Router`, and that discipline is worth more than the copy it would remove.
func TestEveryOptionsFieldIsCopiedToServer(t *testing.T) {
	// Fields that legitimately are never read from Options in Router.
	skip := map[string]string{
		"Log": "Router takes its logger as a parameter, which callers pass alongside Options.Log",
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "api.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Collect every `opts.A` and `opts.A.B` selector chain.
	read := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		var chain []string
		var cur ast.Expr = sel
		for {
			s, ok := cur.(*ast.SelectorExpr)
			if !ok {
				break
			}
			chain = append([]string{s.Sel.Name}, chain...)
			cur = s.X
		}
		if id, ok := cur.(*ast.Ident); ok && id.Name == "opts" {
			read[strings.Join(chain, ".")] = true
		}
		return true
	})
	if len(read) == 0 {
		t.Fatal("found no opts.… reads in api.go — this test can no longer see what it guards")
	}

	var lost []string
	checked, total := 0, 0
	check := func(path string) {
		total++
		if _, ok := skip[path]; ok {
			return
		}
		checked++
		if !read[path] {
			lost = append(lost, path+": never read in Router — the field will be its zero value")
		}
	}
	ot := reflect.TypeOf(Options{})
	for i := 0; i < ot.NumField(); i++ {
		field := ot.Field(i)
		if field.Type.Kind() != reflect.Struct {
			check(field.Name)
			continue
		}
		for j := 0; j < field.Type.NumField(); j++ {
			check(field.Name + "." + field.Type.Field(j).Name)
		}
	}

	// ⚠ Report coverage. A test that skipped everything would pass identically to one that
	// checked it all, and this test skips by design.
	t.Logf("checked %d of %d Options fields", checked, total)
	if checked < total-len(skip) {
		t.Errorf("only %d of %d fields were checkable — the skip list may be stale", checked, total-len(skip))
	}

	sort.Strings(lost)
	if len(lost) > 0 {
		t.Errorf("Options fields that never reach a handler:\n  %s", strings.Join(lost, "\n  "))
	}
}
