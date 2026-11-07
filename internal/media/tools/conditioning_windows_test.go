//go:build windows

package tools_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/media/tools"
)

func TestMeasureConditioningRejectsAncestorJunctionBeforeTools(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "fixture.mp4"), []byte("local fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(t.TempDir(), "linked-directory")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, target).CombinedOutput(); err != nil {
		t.Fatalf("create Windows directory junction: %v: %s", err, output)
	}
	missingTool := filepath.Join(t.TempDir(), "must-not-run.exe")
	_, err := tools.NewFFmpegTools(missingTool, missingTool, "", "", "").MeasureConditioning(
		context.Background(), tools.ConditioningRequest{Path: filepath.Join(junction, "fixture.mp4")})
	if !errors.Is(err, tools.ErrConditioningOutput) {
		t.Fatalf("ancestor reparse error = %v, want invalid input before tools", err)
	}
}
