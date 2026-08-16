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
