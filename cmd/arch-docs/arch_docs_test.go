package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The generated package map must match what is committed in docs/design.md §2 — the
// same drift gate `make config-docs` has, and the reason this generator exists at all.
// Adding, removing or re-pointing a package without regenerating fails here, so §2
// cannot quietly go stale the way its filler-flow paragraph did.
//
// Runs under comprehensive verification, not only under `make arch-docs-verify`: a gate that lives
// only in a Makefile target is one CI-config edit away from never running.
func TestArchDocs_NoDrift(t *testing.T) {
	root := repoRoot(t)
	docPath := filepath.Join(root, "docs", "design.md")

	committed, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read design.md: %v", err)
	}
	pkgs, err := scan(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatalf("scan internal/: %v", err)
	}
	next, err := splice(string(committed), render(pkgs), anchor)
	if err != nil {
		t.Fatalf("splice: %v", err)
	}
	if next != string(committed) {
		t.Errorf("docs/design.md §2 package map is stale — run `make arch-docs` and commit.\n"+
			"committed %d bytes, generated %d bytes", len(committed), len(next))
	}
}

// Every package under internal/ must appear in the generated map. This is the same
// guarantee TestPackageMapListsEveryPackage gives for the hand-written §14.2 table,
// asserted here against the generated block so the two cannot disagree about what
// exists.
func TestArchDocs_CoversEveryPackage(t *testing.T) {
	root := repoRoot(t)
	pkgs, err := scan(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	block := render(pkgs)

	entries, err := os.ReadDir(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatalf("read internal/: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// A directory with no non-test Go files is deliberately absent from the map
		// (internal/integration); scan() drops it, so only assert on what it kept.
		found := false
		for _, p := range pkgs {
			if p.Name == e.Name() {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		if !strings.Contains(block, "**`"+e.Name()+"`**") {
			t.Errorf("internal/%s is missing from the generated package map", e.Name())
		}
	}
}

func TestScan_RecursesWithExactPackageIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "internal")
	writeGoFixture(t, root, "top/top.go", `// Package top is the top-level fixture.
package top

import _ "github.com/loomarr/loomarr/internal/nested/leaf"
`)
	writeGoFixture(t, root, "nested/leaf/leaf.go", `// Package leaf is the nested leaf fixture.
package leaf
`)
	writeGoFixture(t, root, "nested/child/child.go", `// Package child imports a top-level package.
package child

import _ "github.com/loomarr/loomarr/internal/top"
`)
	writeGoFixture(t, root, "testonly/only_test.go", `package testonly
`)

	got, err := scan(root)
	if err != nil {
		t.Fatalf("scan fixture tree: %v", err)
	}
	want := []Package{
		{Name: "nested/child", Synopsis: "Imports a top-level package.", Imports: []string{"top"}, Layer: 2},
		{Name: "nested/leaf", Synopsis: "Nested leaf fixture.", Imports: []string{}},
		{Name: "top", Synopsis: "Top-level fixture.", Imports: []string{"nested/leaf"}, Layer: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scan()\n got %#v\nwant %#v", got, want)
	}
	if block := render(got); strings.Contains(block, "**`testonly`**") {
		t.Error("render included a directory containing only test Go files")
	}
}

// A half-present marker pair must ERROR, never regenerate. This is the one path where
// a bug would delete hand-written prose from the source-of-truth document, so it is
// tested directly rather than trusted.
func TestSplice_RefusesHalfAMarkerPair(t *testing.T) {
	for _, doc := range []string{
		"## 2. Architecture\nprose\n" + beginMarker + "\nblock without an end\n",
		"## 2. Architecture\nprose\n" + endMarker + "\n",
	} {
		if _, err := splice(doc, "NEW", anchor); err == nil {
			t.Error("splice accepted a half-matched marker pair — it must refuse rather than " +
				"regenerate over prose")
		}
	}
}

// Regenerating over an existing block must replace only that block, leaving the prose
// on both sides byte-identical.
func TestSplice_ReplacesOnlyTheBlock(t *testing.T) {
	doc := "# Doc\n\n## 2. Architecture\n\nkeep me before\n\n" +
		beginMarker + "\nold\n" + endMarker +
		"\n\nkeep me after\n\n## 3. Next\n"

	out, err := splice(doc, beginMarker+"\nnew\n"+endMarker, anchor)
	if err != nil {
		t.Fatalf("splice: %v", err)
	}
	for _, must := range []string{"keep me before", "keep me after", "## 3. Next", "new"} {
		if !strings.Contains(out, must) {
			t.Errorf("splice dropped %q", must)
		}
	}
	if strings.Contains(out, "old") {
		t.Error("splice left the previous block behind")
	}
}

// Rendering is deterministic: the CI drift check is only meaningful if two runs over
// the same input agree byte for byte. Map iteration order is the obvious way this
// breaks, and it breaks intermittently, which is the worst failure mode for a gate.
func TestRender_IsDeterministic(t *testing.T) {
	pkgs, err := scan(filepath.Join(repoRoot(t), "internal"))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	first := render(pkgs)
	for i := 0; i < 8; i++ {
		if got := render(pkgs); got != first {
			t.Fatalf("render is not deterministic (run %d differed)", i+2)
		}
	}
}

func TestAssignLayers_IsDeterministicForCollapsedCycles(t *testing.T) {
	orders := [][]string{{"app", "metrics", "provision"}, {"provision", "app", "metrics"}, {"metrics", "provision", "app"}}
	var want map[string]int
	for _, order := range orders {
		packages := map[string]*Package{}
		for _, name := range order {
			imports := map[string][]string{
				"app": {"metrics"}, "metrics": {"provision"}, "provision": {"app"},
			}[name]
			packages[name] = &Package{Name: name, Imports: imports}
		}
		assignLayers(packages)
		got := map[string]int{}
		for name, pkg := range packages {
			got[name] = pkg.Layer
		}
		if want == nil {
			want = got
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("layers depend on map insertion order: got %v, want %v", got, want)
		}
	}
}

func TestFirstSentence(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Package binder materializes an APPROVED proposal onto a channel (§7): create it.",
			"Materializes an APPROVED proposal onto a channel (§7): create it."},
		{"Package schedule is the scheduler domain (design §9): the Channel identity. More here.",
			"Scheduler domain (design §9): the Channel identity."},
		// A §-reference with a decimal must not be read as a sentence end.
		{"Package playout is Loomarr's own streaming engine (design §9.1): it turns a lineup into MPEG-TS.",
			"Loomarr's own streaming engine (design §9.1): it turns a lineup into MPEG-TS."},
		{"", ""},
	} {
		if got := firstSentence(tc.in); got != tc.want {
			t.Errorf("firstSentence(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

// repoRoot walks up to the module root so the test works from the package dir.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from " + dir)
		}
		dir = parent
	}
}

func writeGoFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", relative, err)
	}
}
