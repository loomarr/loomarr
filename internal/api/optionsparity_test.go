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

// ⚠ **THE OPTIONS→SERVER COPY IS 36 HAND-WRITTEN PAIRS**, assigned in `Router` ~400 lines
// from where either struct is declared. A missed line is SILENT: the field takes its zero
// value, and per `Server`'s nil-semantics that may mean 501, 404, a route not mounted, a
// no-op — or, for the two config gates, a gate that stops applying.
//
// This asserts every `Options` field is assigned somewhere in the `Server` literal.
//
// ⚠ **It reads the SOURCE, and that is a deliberate second choice.** The natural test is to
// fill each Options field with a non-nil value and check it arrives — but 29 of the 41 are
// service INTERFACES, and reflection cannot synthesise an implementation of an arbitrary
// method set (`reflect.StructOf` does not promote methods of an embedded interface). A
// value-based version therefore covered 11 of 41, skipping precisely the fields most likely
// to be missed. Checking 11 while reporting success would have been worse than not testing.
//
// Deliberately NOT solved by embedding Options into Server. That makes the copy one line,
// but renames 308 call sites (`s.store` → `s.Store`) and — the reason it is refused — makes
// every dependency settable from `api_test`. Zero tests call handlers directly today; all 26
// go through `Router`, and that discipline is worth more than the copy it would remove.
func TestEveryOptionsFieldIsCopiedToServer(t *testing.T) {
	// Fields that legitimately never become a same-named Server field.
	skip := map[string]string{
		"Pprof":         "mounts a route; not stored on Server",
		"HWEncodeSlots": "consumed to CONSTRUCT srv.hwEncodeGate (playoutadmission.go), not copied to a same-named field",
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "api.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Collect the field names assigned in the `&Server{…}` composite literal.
	assigned := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		id, ok := lit.Type.(*ast.Ident)
		if !ok || id.Name != "Server" {
			return true
		}
		for _, el := range lit.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if k, ok := kv.Key.(*ast.Ident); ok {
				assigned[k.Name] = true
			}
		}
		return true
	})
	if len(assigned) == 0 {
		t.Fatal("found no &Server{…} literal in api.go — this test can no longer see what it guards")
	}

	rename := map[string]string{"LiveTV": "livetv", "SSO": "sso"}
	ot := reflect.TypeOf(Options{})
	st := reflect.TypeOf(Server{})

	var lost []string
	checked := 0
	for i := 0; i < ot.NumField(); i++ {
		name := ot.Field(i).Name
		if _, ok := skip[name]; ok {
			continue
		}
		target, ok := rename[name]
		if !ok {
			target = strings.ToLower(name[:1]) + name[1:]
		}
		if _, ok := st.FieldByName(target); !ok {
			lost = append(lost, name+": no Server field named "+target)
			continue
		}
		checked++
		if !assigned[target] {
			lost = append(lost, name+": Server."+target+" is never assigned in Router — "+
				"the field will be its zero value")
		}
	}

	// ⚠ Report coverage. A test that skipped everything would pass identically to one that
	// checked it all, and this test skips by design.
	t.Logf("checked %d of %d Options fields", checked, ot.NumField())
	if checked < ot.NumField()-len(skip) {
		t.Errorf("only %d of %d fields were checkable — the skip list may be stale", checked, ot.NumField()-len(skip))
	}

	sort.Strings(lost)
	if len(lost) > 0 {
		t.Errorf("Options fields that never reach Server:\n  %s", strings.Join(lost, "\n  "))
	}
}
