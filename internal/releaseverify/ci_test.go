package releaseverify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCIImageInputs(t *testing.T) {
	workflow := `jobs:
  changes:
    steps:
      - id: filter
        run: |
          changed=$(git diff --name-only HEAD^ HEAD)
          if echo "$changed" | grep -qE '^Dockerfile$|^\.dockerignore$|^LICENSE$|^THIRD_PARTY_NOTICES\.md$|^Cargo\.(toml|lock)$|^rust-toolchain\.toml$|^rust/|^internal/store/migrations/|\.go$|^go\.(mod|sum)$|^docs/help/|^web/|^api/openapi\.yaml$|^scripts/check-fe-bundle\.mjs$'; then
            echo "image=true" >> "$GITHUB_OUTPUT"
          fi
`
	path := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(path, []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIImageInputs(path); err != nil {
		t.Fatalf("complete image filter: %v", err)
	}

	withoutMigrations := strings.Replace(workflow, "|^internal/store/migrations/", "", 1)
	if err := os.WriteFile(path, []byte(withoutMigrations), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIImageInputs(path); err == nil {
		t.Fatal("VerifyCIImageInputs accepted an image filter that skipped embedded migrations")
	}

	commented := strings.ReplaceAll(workflow, "          if echo", "          # if echo")
	commented = strings.ReplaceAll(commented, "            echo \"image=true\"", "            # echo \"image=true\"")
	if err := os.WriteFile(path, []byte(commented), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIImageInputs(path); err == nil {
		t.Fatal("VerifyCIImageInputs accepted a commented-out image filter")
	}
}

func TestVerifyCIAggregate(t *testing.T) {
	workflow := `jobs:
  changes:
    runs-on: ubuntu-latest
  windows-playout:
    runs-on: windows-latest
  ci-ok:
    needs: [changes, windows-playout]
    runs-on: ubuntu-latest
`
	path := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(path, []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIAggregate(path); err != nil {
		t.Fatalf("complete CI aggregate: %v", err)
	}

	withoutWindows := strings.Replace(workflow, "changes, windows-playout", "changes", 1)
	if err := os.WriteFile(path, []byte(withoutWindows), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIAggregate(path); err == nil {
		t.Fatal("VerifyCIAggregate accepted a required check that omitted Windows")
	}

	unknownJob := strings.Replace(workflow, "changes, windows-playout", "changes, windows-playout, phantom", 1)
	if err := os.WriteFile(path, []byte(unknownJob), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIAggregate(path); err == nil {
		t.Fatal("VerifyCIAggregate accepted an unknown dependency")
	}
}

func TestVerifyCIImpactActivation(t *testing.T) {
	workflow := `jobs:
  changes:
    outputs:
      impact_postgres: ${{ steps.impact.outputs.postgres }}
      release_candidate: ${{ steps.filter.outputs.release_candidate }}
  store-postgres:
    needs: changes
    if: needs.changes.outputs.impact_postgres == 'true' || needs.changes.outputs.release_candidate == 'true'
`
	path := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(path, []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIImpactActivation(path); err != nil {
		t.Fatalf("complete impact activation: %v", err)
	}

	mutations := map[string]string{
		"legacy selector":    strings.Replace(workflow, "impact_postgres", "go", 2),
		"missing dependency": strings.Replace(workflow, "    needs: changes\n", "", 1),
		"missing candidate":  strings.Replace(workflow, " || needs.changes.outputs.release_candidate == 'true'", "", 1),
		"detached output":    strings.Replace(workflow, "steps.impact.outputs.postgres", "steps.filter.outputs.go", 1),
	}
	for name, mutated := range mutations {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(mutated), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := VerifyCIImpactActivation(path); err == nil {
				t.Fatal("VerifyCIImpactActivation accepted a broken activated-gate contract")
			}
		})
	}
}

func TestVerifyCIManualScopes(t *testing.T) {
	workflow := `on:
  workflow_dispatch:
    inputs:
      scope:
        default: release-candidate
        type: choice
        options: [release-candidate, full]
jobs:
  changes:
    outputs:
      release_candidate: ${{ steps.filter.outputs.release_candidate }}
    steps:
      - id: filter
        run: ./scripts/ci-dispatch-scope.sh "$DISPATCH_SCOPE"
  release-candidate-scope:
    name: Release candidate — exact main scope
  full-manual-scope:
    name: Manual CI — full scope
  go-contracts:
    if: needs.changes.outputs.release_candidate == 'true'
  image-certification:
    if: needs.changes.outputs.release_candidate == 'true'
`
	path := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(path, []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIManualScopes(path); err != nil {
		t.Fatalf("complete manual CI scopes: %v", err)
	}

	mutations := map[string]string{
		"wrong default":    strings.Replace(workflow, "default: release-candidate", "default: full", 1),
		"missing selector": strings.Replace(workflow, "./scripts/ci-dispatch-scope.sh", "./scripts/other.sh", 1),
		"renamed marker":   strings.Replace(workflow, "Release candidate — exact main scope", "Candidate", 1),
		"missing contract": strings.Replace(workflow, "needs.changes.outputs.release_candidate == 'true'", "false", 1),
	}
	for name, mutated := range mutations {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(mutated), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := VerifyCIManualScopes(path); err == nil {
				t.Fatal("VerifyCIManualScopes accepted a broken manual scope contract")
			}
		})
	}
}
