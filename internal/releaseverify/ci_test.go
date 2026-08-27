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
  docs:
    runs-on: ubuntu-latest
  ci-ok:
    needs: [changes, docs]
    runs-on: ubuntu-latest
`
	path := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(path, []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIAggregate(path); err != nil {
		t.Fatalf("complete CI aggregate: %v", err)
	}

	withoutDocs := strings.Replace(workflow, "changes, docs", "changes", 1)
	if err := os.WriteFile(path, []byte(withoutDocs), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIAggregate(path); err == nil {
		t.Fatal("VerifyCIAggregate accepted a required check that omitted docs")
	}

	unknownJob := strings.Replace(workflow, "changes, docs", "changes, docs, phantom", 1)
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
      impact_policy: ${{ steps.impact.outputs.policy }}
      impact_contracts: ${{ steps.impact.outputs.contracts }}
      impact_rust: ${{ steps.impact.outputs.rust }}
      impact_go: ${{ steps.impact.outputs.go }}
      impact_web: ${{ steps.impact.outputs.web }}
      impact_clients: ${{ steps.impact.outputs.clients }}
      impact_image: ${{ steps.impact.outputs.image }}
      impact_docs: ${{ steps.impact.outputs.docs }}
      impact_android: ${{ steps.impact.outputs.android }}
      impact_visual: ${{ steps.impact.outputs.visual }}
      impact_e2e: ${{ steps.impact.outputs.e2e }}
      impact_tuner: ${{ steps.impact.outputs.tuner }}
      impact_apple_mobile: ${{ steps.impact.outputs.apple_mobile }}
      impact_apple_tv: ${{ steps.impact.outputs.apple_tv }}
  ci-policy:
    needs: changes
    if: needs.changes.outputs.impact_policy == 'true'
  rust-contracts:
    needs: changes
    if: needs.changes.outputs.impact_rust == 'true' || needs.changes.outputs.release_candidate == 'true'
  go-contracts:
    needs: changes
    if: needs.changes.outputs.impact_contracts == 'true' || needs.changes.outputs.release_candidate == 'true'
  image-certification:
    needs: changes
    if: needs.changes.outputs.impact_rust == 'true' || needs.changes.outputs.release_candidate == 'true'
  go:
    needs: changes
    if: needs.changes.outputs.impact_go == 'true'
  store-postgres:
    needs: changes
    if: needs.changes.outputs.impact_postgres == 'true'
  playwright:
    needs: changes
    if: needs.changes.outputs.impact_visual == 'true' || needs.changes.outputs.impact_e2e == 'true'
  frontend:
    needs: changes
    if: needs.changes.outputs.impact_web == 'true'
  clients:
    needs: changes
    if: needs.changes.outputs.impact_clients == 'true'
  image:
    needs: changes
    if: needs.changes.outputs.impact_image == 'true'
  docs:
    needs: changes
    if: needs.changes.outputs.impact_docs == 'true'
  android:
    needs: changes
    if: needs.changes.outputs.impact_android == 'true'
  tuner:
    needs: changes
    if: github.event_name != 'pull_request' && needs.changes.outputs.impact_tuner == 'true'
  apple-mobile:
    needs: changes
    if: github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_mobile == 'true'
    steps:
      - run: make client-apple-simulator CLIENT_APP=mobile
  apple-tv:
    needs: changes
    if: github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_tv == 'true'
    steps:
      - run: make client-apple-simulator CLIENT_APP=tv
`
	path := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(path, []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCIImpactActivation(path); err != nil {
		t.Fatalf("complete impact activation: %v", err)
	}

	mutations := map[string]string{
		"legacy Postgres selector": strings.Replace(workflow, "impact_postgres", "go", 2),
		"missing dependency":       strings.Replace(workflow, "    needs: changes\n", "", 1),
		"broadened candidate":      strings.Replace(workflow, " == 'true'", " == 'true' || needs.changes.outputs.release_candidate == 'true'", 1),
		"detached Postgres output": strings.Replace(workflow, "steps.impact.outputs.postgres", "steps.filter.outputs.go", 1),
		"legacy Playwright selector": strings.Replace(
			workflow,
			"needs.changes.outputs.impact_visual == 'true' || needs.changes.outputs.impact_e2e == 'true'",
			"needs.changes.outputs.web == 'true'",
			1,
		),
		"detached visual output": strings.Replace(workflow, "steps.impact.outputs.visual", "steps.filter.outputs.web", 1),
		"lost e2e selector": strings.Replace(
			workflow,
			" || needs.changes.outputs.impact_e2e == 'true'",
			"",
			1,
		),
		"intersected browser selectors": strings.Replace(workflow, "impact_visual == 'true' ||", "impact_visual == 'true' &&", 1),
		"legacy tuner selector": strings.Replace(
			workflow,
			"needs.changes.outputs.impact_tuner == 'true'",
			"needs.changes.outputs.web == 'true'",
			1,
		),
		"detached tuner output": strings.Replace(workflow, "steps.impact.outputs.tuner", "steps.filter.outputs.web", 1),
		"broadened tuner selector": strings.Replace(
			workflow,
			"needs.changes.outputs.impact_tuner == 'true'",
			"needs.changes.outputs.impact_tuner == 'true' || needs.changes.outputs.release_candidate == 'true'",
			1,
		),
		"legacy Apple mobile selector": strings.Replace(
			workflow,
			"needs.changes.outputs.impact_apple_mobile == 'true'",
			"needs.changes.outputs.clients == 'true'",
			1,
		),
		"detached Apple mobile output": strings.Replace(workflow, "steps.impact.outputs.apple_mobile", "steps.filter.outputs.clients", 1),
		"legacy Apple TV selector": strings.Replace(
			workflow,
			"needs.changes.outputs.impact_apple_tv == 'true'",
			"needs.changes.outputs.clients == 'true'",
			1,
		),
		"detached Apple TV output": strings.Replace(workflow, "steps.impact.outputs.apple_tv", "steps.filter.outputs.clients", 1),
		"broadened Apple TV selector": strings.Replace(
			workflow,
			"needs.changes.outputs.impact_apple_tv == 'true'",
			"needs.changes.outputs.impact_apple_tv == 'true' || needs.changes.outputs.release_candidate == 'true'",
			1,
		),
		"Apple mobile runs tvOS": strings.Replace(workflow, "CLIENT_APP=mobile", "CLIENT_APP=tv", 1),
		"Apple tv runs mobile":   strings.Replace(workflow, "CLIENT_APP=tv", "CLIENT_APP=mobile", 1),
		"restored Apple matrix": strings.Replace(
			workflow,
			"  apple-mobile:\n    needs: changes",
			"  apple-mobile:\n    strategy:\n      matrix: {client: [mobile, tv]}\n    needs: changes",
			1,
		),
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

func TestVerifyCINativeAdmission(t *testing.T) {
	workflow := `on:
  pull_request:
  merge_group:
  workflow_dispatch:
jobs:
  changes:
    runs-on: ubuntu-latest
  agent-harness-macos:
    if: github.event_name != 'pull_request' && needs.changes.outputs.impact_agent == 'true'
    runs-on: macos-latest
  apple-mobile:
    if: github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_mobile == 'true'
    runs-on: macos-26
  apple-tv:
    if: github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_tv == 'true'
    runs-on: macos-26
  tuner:
    if: github.event_name != 'pull_request' && needs.changes.outputs.impact_tuner == 'true'
    runs-on: macos-latest
`
	path := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(path, []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCINativeAdmission(path); err != nil {
		t.Fatalf("complete native admission contract: %v", err)
	}

	mutations := map[string]string{
		"missing merge-group trigger": strings.Replace(workflow, "  merge_group:\n", "", 1),
		"ordinary PR consumes Apple capacity": strings.Replace(
			workflow,
			"github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_mobile == 'true'",
			"needs.changes.outputs.impact_apple_mobile == 'true'",
			1,
		),
		"native evidence is queue-only": strings.Replace(
			workflow,
			"github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_tv == 'true'",
			"github.event_name == 'merge_group' && needs.changes.outputs.impact_apple_tv == 'true'",
			1,
		),
		"missing scarce job": strings.Replace(
			workflow,
			"  tuner:\n    if: github.event_name != 'pull_request' && needs.changes.outputs.impact_tuner == 'true'\n    runs-on: macos-latest\n",
			"",
			1,
		),
	}
	for name, mutated := range mutations {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(mutated), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := VerifyCINativeAdmission(path); err == nil {
				t.Fatal("VerifyCINativeAdmission accepted a broken native admission contract")
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
