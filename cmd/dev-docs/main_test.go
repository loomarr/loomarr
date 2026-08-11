package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
		"      - run: make check\n" +
		"      - name: spec drift\n" +
		"        run: make openapi-verify\n"
	writeWorkflow(t, dir, "ci.yml", workflow)

	got, err := ciTargets(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"check", "openapi-verify"} {
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
	writeWorkflow(t, dir, "ci.yml", "    - run: make check\n")
	writeWorkflow(t, dir, "notes.md", "We should probably wire `make e2e` in here one day.\n")

	got, err := ciTargets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["check"]; !ok {
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
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
