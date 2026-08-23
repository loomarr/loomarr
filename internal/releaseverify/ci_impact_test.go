package releaseverify

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCIImpactClassifier(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	cmd := exec.Command("bash", filepath.Join("scripts", "ci-impact-test.sh"))
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ci impact classifier contract: %v\n%s", err, output)
	}
}

func TestGoImpactSelector(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	cmd := exec.Command("bash", filepath.Join("scripts", "go-impact-test.sh"))
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Go impact selector contract: %v\n%s", err, output)
	}
}

func TestCIDispatchScope(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	cmd := exec.Command("bash", filepath.Join("scripts", "ci-dispatch-scope-test.sh"))
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CI dispatch scope contract: %v\n%s", err, output)
	}
}

func TestReleaseSourceEvidence(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	cmd := exec.Command("bash", filepath.Join("scripts", "validate-release-source-test.sh"))
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release source evidence contract: %v\n%s", err, output)
	}
}
