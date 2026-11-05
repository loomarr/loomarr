package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The generated package map must match what is committed in docs/design/package-map.md —
// the same drift gate `make config-docs` has, and the reason this generator exists at all.
// Adding, removing or re-pointing a package without regenerating fails here, so the map
// cannot quietly go stale the way design.md §2's filler-flow paragraph did.
//
// Runs under comprehensive verification, not only under `make arch-docs-verify`: a gate that lives
// only in a Makefile target is one CI-config edit away from never running.
func TestArchDocs_NoDrift(t *testing.T) {
	root := repoRoot(t)
	docPath := filepath.Join(root, filepath.FromSlash(defaultDocPath))

	committed, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read package map: %v", err)
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
		t.Errorf("%s is stale — run `make arch-docs` and commit.\n"+
			"committed %d bytes, generated %d bytes", defaultDocPath, len(committed), len(next))
	}
}

// The committed generated block is an exact set inventory: every source package is
// present, including nested packages, and no removed package survives as a stale row.
// Source discovery here is deliberately independent of scan(), so one recursive-walk
// bug cannot make both the generator and its coverage gate agree on the same omission.
func TestArchDocs_CoversEveryPackage(t *testing.T) {
	root := repoRoot(t)
	source, err := goPackageNames(filepath.Join(root, "internal"), false)
	if err != nil {
		t.Fatalf("discover source packages: %v", err)
	}
	doc, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(defaultDocPath)))
	if err != nil {
		t.Fatalf("read package map: %v", err)
	}
	rendered := renderedPackageNames(string(doc))
	missing, stale := packageInventoryDiff(source, rendered)
	if len(missing) > 0 || len(stale) > 0 {
		t.Errorf("generated package inventory differs from source\nmissing: %v\nstale: %v", missing, stale)
	}
}

// The area table names every Go package under internal/ exactly once, test-only packages
// included, and names nothing that is not a package. A new package has to be given an area
// before it merges; a renamed or deleted one has to leave the table. Discovery is the
// test's own walk, not the generator's, for the same reason as TestArchDocs_CoversEveryPackage.
func TestArchDocs_AreasCoverEveryPackage(t *testing.T) {
	source, err := goPackageNames(filepath.Join(repoRoot(t), "internal"), true)
	if err != nil {
		t.Fatalf("discover source packages: %v", err)
	}
	listed, duplicates := areaInventory(areas)
	if len(duplicates) > 0 {
		t.Errorf("packages listed in more than one area in cmd/arch-docs/areas.go: %v", duplicates)
	}
	missing, stale := packageInventoryDiff(source, listed)
	if len(missing) > 0 {
		t.Errorf("packages under internal/ with no area — add each to one area in cmd/arch-docs/areas.go: %v", missing)
	}
	if len(stale) > 0 {
		t.Errorf("cmd/arch-docs/areas.go names packages that do not exist under internal/: %v", stale)
	}
}

// The two traced flows name packages in backticks; each must still be a listed package, so a
// rename that updates the table cannot leave the walkthrough pointing at a package that is gone.
func TestArchDocs_FlowsNameListedPackages(t *testing.T) {
	listed, _ := areaInventory(areas)
	for _, f := range flows {
		parts := strings.Split(f.Text, "`")
		for i := 1; i < len(parts); i += 2 {
			if !listed[parts[i]] {
				t.Errorf("flow %q names `%s`, which no area lists", f.Title, parts[i])
			}
		}
	}
}

func TestAreaInventoryReportsDuplicates(t *testing.T) {
	listed, duplicates := areaInventory([]area{
		{Name: "one", Packages: []string{"a", "b"}},
		{Name: "two", Packages: []string{"b", "c"}},
	})
	if want := map[string]bool{"a": true, "b": true, "c": true}; !reflect.DeepEqual(listed, want) {
		t.Fatalf("listed = %v, want %v", listed, want)
	}
	if !reflect.DeepEqual(duplicates, []string{"b"}) {
		t.Fatalf("duplicates = %v, want [b]", duplicates)
	}
}

// Each area's import count is the number of package-to-package imports that cross into
// another area, grouped by that area and listed busiest first.
func TestRenderStartHere_CountsCrossAreaImports(t *testing.T) {
	fixture := []area{
		{Name: "channels", Title: "Channels", Description: "Channel things.", Packages: []string{"schedule", "binder"}},
		{Name: "platform", Title: "Platform", Description: "Shared ground.", Packages: []string{"store", "httpx"}},
		{Name: "library", Title: "Library", Description: "What is owned.", Packages: []string{"library", "itest"}},
	}
	pkgs := []Package{
		{Name: "binder", Synopsis: "Plans a channel.", Imports: []string{"httpx", "library", "schedule", "store"}},
		{Name: "httpx", Synopsis: "HTTP client."},
		{Name: "library", Synopsis: "Library port.", Imports: []string{"httpx"}},
		{Name: "schedule", Synopsis: "Scheduler.", Imports: []string{"store"}},
		{Name: "store", Synopsis: "Persistence."},
	}

	out := renderStartHere(fixture, pkgs)
	for _, want := range []string{
		"**Imports from other areas:** platform (3), library (1)",
		"**Imports from other areas:** none",
		"**Imports from other areas:** platform (1)",
		"| `binder` | Plans a channel. |",
		"| `itest` | Tests only. |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("start-here section missing %q\n%s", want, out)
		}
	}
	// The coverage gate parses "- **`name`**" rows as the layer inventory; the area tables
	// must not look like one, or a test-only package would read as a stale layer row.
	if strings.Contains(out, "- **`") {
		t.Error("start-here section uses the layer-row prefix the inventory gate parses")
	}
}

func TestPackageInventoryDiffReportsMissingAndStale(t *testing.T) {
	missing, stale := packageInventoryDiff(
		map[string]bool{"images": true, "images/rustgen": true},
		map[string]bool{"images": true, "removed": true},
	)
	if !reflect.DeepEqual(missing, []string{"images/rustgen"}) {
		t.Fatalf("missing = %v, want [images/rustgen]", missing)
	}
	if !reflect.DeepEqual(stale, []string{"removed"}) {
		t.Fatalf("stale = %v, want [removed]", stale)
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
	writeGoFixture(t, root, "nested/nested.go", `// Package nested is the parent fixture.
package nested
`)
	writeGoFixture(t, root, "nested/child/child.go", `// Package child imports its parent package.
package child

import _ "github.com/loomarr/loomarr/internal/nested"
`)
	writeGoFixture(t, root, "testonly/only_test.go", `package testonly
`)

	got, err := scan(root)
	if err != nil {
		t.Fatalf("scan fixture tree: %v", err)
	}
	want := []Package{
		{Name: "nested", Synopsis: "Parent fixture.", Imports: []string{}},
		{Name: "nested/child", Synopsis: "Imports its parent package.", Imports: []string{"nested"}, Layer: 1},
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

func TestAssignLayers_IsDeterministicForMalformedCycles(t *testing.T) {
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

// goPackageNames lists every directory below root that holds Go files. With includeTests
// false only non-test files count, which is the set the layer listing renders; with it true a
// directory of tests alone counts too, which is the set the area table must cover.
func goPackageNames(root string, includeTests bool) (map[string]bool, error) {
	names := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && (entry.Name() == "testdata" || entry.Name() == "vendor" ||
			strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_")) {
			return filepath.SkipDir
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, child := range entries {
			if child.IsDir() || !strings.HasSuffix(child.Name(), ".go") ||
				(!includeTests && strings.HasSuffix(child.Name(), "_test.go")) {
				continue
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if relative != "." {
				names[filepath.ToSlash(relative)] = true
			}
			break
		}
		return nil
	})
	return names, err
}

func renderedPackageNames(doc string) map[string]bool {
	names := map[string]bool{}
	start := strings.Index(doc, beginMarker)
	end := strings.Index(doc, endMarker)
	if start < 0 || end <= start {
		return names
	}
	for _, line := range strings.Split(doc[start:end], "\n") {
		const prefix = "- **`"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		name, _, ok := strings.Cut(strings.TrimPrefix(line, prefix), "`**")
		if ok && name != "" {
			names[name] = true
		}
	}
	return names
}

func packageInventoryDiff(source, rendered map[string]bool) (missing, stale []string) {
	for name := range source {
		if !rendered[name] {
			missing = append(missing, name)
		}
	}
	for name := range rendered {
		if !source[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	return missing, stale
}
