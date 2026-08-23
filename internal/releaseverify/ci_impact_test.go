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
