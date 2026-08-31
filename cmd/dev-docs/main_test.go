package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMakefileFollowsOrderedIncludes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "root: ## root target\ninclude mk/go.mk\ninclude mk/docs.mk\n")
	writeFile(t, dir, "mk/go.mk", "## ---- go gate ----\ncheck: fmt ## check it\n")
	writeFile(t, dir, "mk/docs.mk", "## ---- docs gate ----\ndocs: ## render docs\n")

	got, err := parseMakefile(filepath.Join(dir, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Name != "root" || got[1].Name != "check" || got[2].Name != "docs" {
		t.Fatalf("targets = %#v; want root, check, docs in include order", got)
	}
	if got[1].Section != "go gate" || got[2].Section != "docs gate" {
		t.Fatalf("included sections were not preserved: %#v", got)
	}
}

func TestParseMakefileRejectsUnsafeOrAmbiguousIncludes(t *testing.T) {
	tests := map[string]struct {
		root  string
		files map[string]string
		want  string
	}{
		"missing":  {root: "include mk/missing.mk\n", want: "open Make interface"},
		"escaping": {root: "include ../outside.mk\n", want: "escapes repository root"},
		"variable": {root: "include $(MODULES)\n", want: "must be a literal"},
		"cycle": {
			root:  "include mk/one.mk\n",
			files: map[string]string{"mk/one.mk": "include Makefile\n"},
			want:  "cyclic Make include",
		},
		"duplicate target": {
			root:  "same: ## first\ninclude mk/one.mk\n",
			files: map[string]string{"mk/one.mk": "same: ## second\n"},
			want:  "duplicate documented target",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "Makefile", tc.root)
			for path, body := range tc.files {
				writeFile(t, dir, path, body)
			}
			_, err := parseMakefile(filepath.Join(dir, "Makefile"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want substring %q", err, tc.want)
			}
		})
	}
}

// The CI column is a claim about what CI actually runs, and it once overstated itself: these
// workflows are heavily commented, and a comment mentioning a target made that target report as
// a gate while no step ran it. A generated page that overstates coverage is worse than a
// hand-written one, because it is trusted.
//
// This is the test `stripYAMLComments` never had. The bug it guards was found by reading the
// generator's own output, not by a failing gate.
func TestCITargetsIgnoresCommentedInvocations(t *testing.T) {
	dir := t.TempDir()
	workflow := "" +
		"jobs:\n" +
		"  go:\n" +
		"    steps:\n" +
		"      # Drift is already covered by `make dev-docs-verify`, so there is no need to\n" +
		"      # run `make tags-verify` as a separate step here.\n" +
		"      - run: make verify SCOPE=all\n" +
		"      - name: spec drift\n" +
		"        run: make openapi-verify\n"
	writeWorkflow(t, dir, "ci.yml", workflow)

	got, err := ciTargets(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"verify", "openapi-verify"} {
		if _, ok := got[want]; !ok {
			t.Errorf("%q is invoked by a real step but went undetected — the CI column would understate coverage", want)
		}
	}
	for _, commented := range []string{"dev-docs-verify", "tags-verify"} {
		if _, ok := got[commented]; ok {
			t.Errorf("%q appears only inside a YAML comment, but the CI column would mark it ✅ — the exact lie this strip exists to prevent", commented)
		}
	}
}

// Only workflow files are scanned. A README or a JSON config sitting in the same directory
// mentioning `make something` must not manufacture a CI claim.
func TestCITargetsReadsOnlyYAML(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "ci.yml", "    - run: make verify SCOPE=all\n")
	writeWorkflow(t, dir, "notes.md", "We should probably wire `make e2e` in here one day.\n")

	got, err := ciTargets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["verify"]; !ok {
		t.Error("ci.yml step was not detected")
	}
	if _, ok := got["e2e"]; ok {
		t.Error("a non-workflow file contributed a target, so the CI column would claim a gate that does not exist")
	}
}

// .yaml is as valid as .yml, and a workflow named either way gates real jobs.
func TestCITargetsAcceptsBothYAMLExtensions(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "pages.yaml", "    - run: make docs-lint\n")

	got, err := ciTargets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["docs-lint"]; !ok {
		t.Error(".yaml workflow was skipped, so its targets would report as not run in CI")
	}
}

func writeWorkflow(t *testing.T, dir, name, body string) {
	t.Helper()
	writeFile(t, dir, name, body)
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
