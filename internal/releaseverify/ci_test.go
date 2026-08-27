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
      impact_visual: ${{ steps.impact.outputs.visual }}
      impact_e2e: ${{ steps.impact.outputs.e2e }}
      impact_tuner: ${{ steps.impact.outputs.tuner }}
      impact_apple_mobile: ${{ steps.impact.outputs.apple_mobile }}
      impact_apple_tv: ${{ steps.impact.outputs.apple_tv }}
      impact_expo_android_mobile: ${{ steps.impact.outputs.expo_android_mobile }}
  store-postgres:
    needs: changes
    if: needs.changes.outputs.impact_postgres == 'true'
  playwright:
    needs: changes
    if: needs.changes.outputs.impact_visual == 'true' || needs.changes.outputs.impact_e2e == 'true'
  tuner:
    needs: changes
    if: needs.changes.outputs.impact_tuner == 'true'
  apple-mobile:
    needs: changes
    if: needs.changes.outputs.impact_apple_mobile == 'true'
    steps:
      - run: make client-apple-simulator CLIENT_APP=mobile
  apple-tv:
    needs: changes
    if: needs.changes.outputs.impact_apple_tv == 'true'
    steps:
      - run: make client-apple-simulator CLIENT_APP=tv
  expo-android-mobile:
    needs: changes
    if: needs.changes.outputs.impact_expo_android_mobile == 'true'
    steps:
      - run: make client-android-debug CLIENT_APP=mobile
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
		"legacy Expo Android mobile selector": strings.Replace(
			workflow,
			"needs.changes.outputs.impact_expo_android_mobile == 'true'",
			"needs.changes.outputs.clients == 'true'",
			1,
		),
		"detached Expo Android mobile output": strings.Replace(workflow, "steps.impact.outputs.expo_android_mobile", "steps.filter.outputs.clients", 1),
		"Expo Android mobile runs TV":         strings.Replace(workflow, "client-android-debug CLIENT_APP=mobile", "client-android-debug CLIENT_APP=tv", 1),
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
