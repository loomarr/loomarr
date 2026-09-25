package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalRecordsNormalizesJSONNumbers(t *testing.T) {
	t.Parallel()
	fields := []string{"weight"}
	fromGraph := canonicalRecords([]map[string]any{{"weight": json.Number("1.0")}}, fields)
	generated := canonicalRecords([]map[string]any{{"weight": 1.0}}, fields)
	if string(fromGraph) != string(generated) {
		t.Fatalf("canonical records differ: graph=%s generated=%s", fromGraph, generated)
	}
}

func TestWorkflowProjectionAndFreshness(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, root, ".graphifyignore", "graphify-out/\n**/node_modules/\n")
	mustWrite(t, root, "main.go", "package main\n")
	mustWrite(t, root, ".github/workflows/reusable.yml", `name: Reusable
on: workflow_call
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@0123456789012345678901234567890123456789
`)
	mustWrite(t, root, ".github/workflows/ci.yml", `name: CI
on:
  pull_request:
jobs:
  changes:
    runs-on: ubuntu-latest
    steps:
      - name: Classify
        run: make verify
  test:
    needs: changes
    uses: ./.github/workflows/reusable.yml
`)
	mustGitInit(t, root)
	graphPath := filepath.Join(root, "graphify-out", "graph.json")
	mustWrite(t, root, "graphify-out/graph.json", `{"nodes":[],"edges":[],"hyperedges":[]}`)

	if err := inject(root, graphPath); err != nil {
		t.Fatal(err)
	}
	if err := stamp(root, graphPath); err != nil {
		t.Fatal(err)
	}
	if err := verify(root, graphPath); err != nil {
		t.Fatalf("fresh graph rejected: %v", err)
	}

	doc, err := readGraph(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	projection := selectProjection(doc)
	assertRelation(t, projection, "triggers_on")
	assertRelation(t, projection, "contains")
	assertRelation(t, projection, "needs")
	assertRelation(t, projection, "uses")

	mustWrite(t, root, ".github/workflows/ci.yml", strings.ReplaceAll(readFile(t, root, ".github/workflows/ci.yml"), "make verify", "make graphify-verify"))
	if err := verify(root, graphPath); err == nil {
		t.Fatal("workflow change did not make the graph stale")
	}
}

func TestInputScopeMatchesCodeAndIgnoreContract(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, root, ".graphifyignore", "graphify-out/\n**/node_modules/\nweb/generated/\n")
	mustWrite(t, root, "main.go", "package main\n")
	mustWrite(t, root, "web/app.tsx", "export const app = true\n")
	mustWrite(t, root, "web/generated/client.ts", "export const generated = true\n")
	mustWrite(t, root, "web/node_modules/pkg/index.js", "module.exports = {}\n")
	mustWrite(t, root, "docs/readme.md", "not part of the code-only graph\n")
	mustWrite(t, root, ".github/workflows/ci.yml", "name: CI\non: pull_request\njobs: {}\n")
	mustWrite(t, root, "graphify-out/graph.json", "{}\n")
	mustGitInit(t, root)

	metadata, err := currentMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.SourceFiles != 4 {
		t.Fatalf("source files = %d, want .graphifyignore + two code files + workflow", metadata.SourceFiles)
	}
	if metadata.WorkflowFiles != 1 {
		t.Fatalf("workflow files = %d, want 1", metadata.WorkflowFiles)
	}
}

func TestUnsupportedGraphifyIgnorePatternFailsClosed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".graphifyignore")
	if err := os.WriteFile(path, []byte("build/*.json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readGraphifyIgnore(path); err == nil {
		t.Fatal("wildcard pattern was silently accepted")
	}
}

func TestRemoveProjectionDropsLegacyOrigin(t *testing.T) {
	t.Parallel()
	doc := map[string]any{
		"nodes": []any{
			map[string]any{"id": "kept", "_origin": "ast"},
			map[string]any{"id": "legacy", "_origin": origin},
			map[string]any{"id": "current", "_origin": "ast", provenanceKey: origin},
		},
		"edges": []any{
			map[string]any{"source": "kept", "target": "legacy", "_origin": "ast"},
			map[string]any{"source": "kept", "target": "current", provenanceKey: origin},
		},
	}
	removeProjection(doc)
	if got := len(doc["nodes"].([]any)); got != 1 {
		t.Fatalf("nodes after cleanup = %d, want 1", got)
	}
	if got := len(doc["edges"].([]any)); got != 0 {
		t.Fatalf("edges after cleanup = %d, want 0", got)
	}
}

func assertRelation(t *testing.T, p projection, relation string) {
	t.Helper()
	for _, link := range p.Links {
		if link["relation"] == relation {
			return
		}
	}
	t.Fatalf("projection has no %q relation", relation)
}

func mustGitInit(t *testing.T, root string) {
	t.Helper()
	command := func(args ...string) {
		if output, err := run(root, "git", args...); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
		}
	}
	command("init", "-q")
	command("add", ".")
}

func run(dir, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	return string(output), err
}

func mustWrite(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, relative string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMetadataJSONShape(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(graphMetadata{Schema: 1, Graphify: graphifyVersion, InputsSHA256: "abc", SourceFiles: 2, WorkflowFiles: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"inputs_sha256":"abc"`) {
		t.Fatalf("metadata JSON = %s", data)
	}
}
