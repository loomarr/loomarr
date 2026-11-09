package filler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/media/tools"
)

func TestSnapshotSplitCompositeRejectsOversizedSourceBeforeStaging(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, "stage")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "input.mp4")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(source, tools.ConditioningMaxSnapshotBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotSplitComposite(context.Background(), source, stage, ".mp4", "not-used"); err == nil {
		t.Fatal("oversized composite accepted")
	}
	assertNoSnapshotOutputs(t, stage)
	info, err := os.Stat(source)
	if err != nil || info.Size() != tools.ConditioningMaxSnapshotBytes+1 {
		t.Fatalf("source changed: %+v, %v", info, err)
	}
}

func TestSnapshotConditioningArtifactsRejectsOversizedSourceOrParentAtomically(t *testing.T) {
	for _, oversized := range []string{"source", "parent"} {
		t.Run(oversized, func(t *testing.T) {
			dir := t.TempDir()
			stage := filepath.Join(dir, "stage")
			if err := os.Mkdir(stage, 0o700); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(dir, "source.mp4")
			parent := filepath.Join(dir, "parent.mp4")
			if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(parent, []byte("parent"), 0o600); err != nil {
				t.Fatal(err)
			}
			target := source
			if oversized == "parent" {
				target = parent
			}
			if err := os.Truncate(target, tools.ConditioningMaxSnapshotBytes+1); err != nil {
				t.Fatal(err)
			}
			sourceHash, _ := asset.ClipID(source)
			parentHash, _ := asset.ClipID(parent)
			if _, err := snapshotConditioningArtifacts(context.Background(), stage, source, parent, sourceHash, parentHash); err == nil {
				t.Fatal("oversized conditioning artifact accepted")
			}
			assertNoSnapshotOutputs(t, stage)
			for _, path := range []string{source, parent} {
				if _, err := os.Stat(path); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func assertNoSnapshotOutputs(t *testing.T, dir string) {
	t.Helper()
	if matches, _ := filepath.Glob(filepath.Join(dir, ".snapshot-*")); len(matches) != 0 {
		t.Fatalf("leaked temporary snapshots: %v", matches)
	}
	for _, name := range []string{"composite.mp4", "source.mp4", "parent.mp4"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("snapshot output %s exists: %v", name, err)
		}
	}
}
