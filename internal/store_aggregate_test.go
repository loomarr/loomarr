package internal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// store.Store is the full union of the persistence roles, held by the composition root and the
// conformance suite. A consumer takes the narrow interface it uses instead, so the union only ever
// shrinks. aggregateStoreRoles is the number of roles it embeds today; lower it when one goes.
const aggregateStoreRoles = 45

func TestAggregateStoreDoesNotGrow(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("platform", "store", "store.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse store.go: %v", err)
	}
	roles, ok := embeddedRoles(file, "Store")
	if !ok {
		t.Fatal("store.go declares no Store interface — the guard would pass over nothing")
	}
	switch {
	case roles > aggregateStoreRoles:
		t.Errorf("store.Store embeds %d roles, above %d: add a narrow interface at the consumer instead",
			roles, aggregateStoreRoles)
	case roles < aggregateStoreRoles:
		t.Errorf("store.Store embeds %d roles, below %d: lower aggregateStoreRoles", roles, aggregateStoreRoles)
	}
}

// embeddedRoles counts the interfaces the named interface embeds; its own methods do not count.
func embeddedRoles(file *ast.File, name string) (count int, found bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != name {
			return true
		}
		iface, ok := spec.Type.(*ast.InterfaceType)
		if !ok {
			return true
		}
		found = true
		for _, member := range iface.Methods.List {
			if len(member.Names) == 0 {
				count++
			}
		}
		return false
	})
	return count, found
}

func TestEmbeddedRolesCountsEmbedsOnly(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package store
type Other interface{ A }
type Store interface {
	TitleStore
	other.ChannelStore
	Close() error
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if count, found := embeddedRoles(file, "Store"); !found || count != 2 {
		t.Errorf("embeddedRoles = %d, %t; want 2, true", count, found)
	}
	if _, found := embeddedRoles(file, "Missing"); found {
		t.Error("embeddedRoles found an interface the file does not declare")
	}
}
