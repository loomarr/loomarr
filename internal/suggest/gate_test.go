package suggest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// THE APPROVAL GATE HAS ONE IMPLEMENTATION (§7 decision D-K, AGENTS.md prime directive 3).
//
// Edit-before-approve is the change most likely to grow a second acquisition path: the moment
// an approver can modify a proposal, it becomes tempting to apply the edit in the handler and
// enqueue from there. That would leave the gate guarding a lineup chosen elsewhere, and the
// auto-approve and re-curation paths running different logic.
//
// So this asserts the property rather than trusting the convention: a `wanted` provisioning
// record — the state that causes an acquisition — is created in exactly ONE place.
//
// ⚠ NOT asserting "only Approve calls UpsertTitle". That is false and would be a brittle test:
// internal/reconcile advances existing titles through their lifecycle, and api/titles.go marks
// a given-up title `unavailable` and re-enqueues a cancelled one. Those are transitions on
// titles that already exist. The invariant is about CREATING an acquisition, not about touching
// the table.
func TestOnlyApproveCreatesWantedTitles(t *testing.T) {
	root := ".."
	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/store/") {
			// Test files construct records freely; the store package is the persistence
			// layer itself and its conformance suite writes every state by definition.
			return nil
		}

		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil // unparseable file is not this test's problem
		}

		ast.Inspect(f, func(n ast.Node) bool {
			// Looking for a composite literal that sets State: provision.Wanted — the shape
			// that turns a title into something the provisioner will go and acquire.
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "State" {
					continue
				}
				sel, ok := kv.Value.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Wanted" {
					continue
				}
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	// Two sanctioned creators, and the distinction between them is the point.
	//
	//   suggest/approve.go — the gate. The ONLY path from a PROPOSAL to an acquisition, which
	//     is what D-K protects: edit-before-approve must not grow a bypass.
	//   api/titles.go     — POST /v1/titles, an admin typing a title id directly. A different
	//     act with its own admin check and its own audit trail, not a proposal being approved
	//     by another name. It is also what the Board's "Try again" uses to re-enqueue a title
	//     the provisioner gave up on.
	//
	// Anything else creating a `wanted` record is a proposal-to-acquisition path that skipped
	// the gate.
	sanctioned := map[string]bool{
		filepath.Join("suggest", "approve.go"): true,
		filepath.Join("api", "titles.go"):      true,
	}
	for _, o := range offenders {
		if sanctioned[o] {
			continue
		}
		t.Errorf("%s creates a `wanted` title — the approval gate must be the ONLY path to an "+
			"acquisition (§7 D-K). If this is a legitimate lifecycle transition rather than a new "+
			"acquisition, it should not be constructing the record from scratch.", o)
	}
	if len(offenders) == 0 {
		t.Fatal("found no `wanted` creation at all — this test has stopped watching anything")
	}
}
