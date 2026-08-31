package filler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/mediatools"
)

func TestSnapshotOwnedFileRejectsSparseSourceOverBoundBeforeStaging(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("small"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(source, mediatools.ConditioningMaxSnapshotBytes+1); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "destination")
	if err := snapshotOwnedFileBounded(context.Background(), source, destination, mediatools.ConditioningMaxSnapshotBytes, nil); err == nil {
		t.Fatal("oversized source accepted")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exposure: %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".snapshot-*")); len(matches) != 0 {
		t.Fatalf("leaked temporary snapshots: %v", matches)
	}
}

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
	if err := os.Truncate(source, mediatools.ConditioningMaxSnapshotBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotSplitComposite(context.Background(), source, stage, ".mp4", "not-used"); err == nil {
		t.Fatal("oversized composite accepted")
	}
	assertNoSnapshotOutputs(t, stage)
	info, err := os.Stat(source)
	if err != nil || info.Size() != mediatools.ConditioningMaxSnapshotBytes+1 {
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
			if err := os.Truncate(target, mediatools.ConditioningMaxSnapshotBytes+1); err != nil {
				t.Fatal(err)
			}
			sourceHash, _ := ClipID(source)
			parentHash, _ := ClipID(parent)
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

func TestSnapshotOwnedFileExactInjectedBoundAndCancellationAreAtomic(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	if err := os.WriteFile(source, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := snapshotOwnedFileBounded(context.Background(), source, destination, 10, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(destination)
	if string(got) != "0123456789" {
		t.Fatalf("exact-bound output %q", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	err := snapshotOwnedFileBounded(ctx, source, destination, 10, func() { cancel() })
	if err == nil {
		t.Fatal("mid-copy cancellation accepted")
	}
	got, _ = os.ReadFile(destination)
	if string(got) != "0123456789" {
		t.Fatalf("destination changed after cancellation: %q", got)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".snapshot-*")); len(matches) != 0 {
		t.Fatalf("leaked temporary snapshots: %v", matches)
	}
}

func TestSnapshotOwnedFileAlreadyCancelledZeroLengthDoesNotPublish(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "empty")
	destination := filepath.Join(dir, "destination")
	if err := os.WriteFile(source, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := snapshotOwnedFileBounded(ctx, source, destination, 10, nil); err == nil {
		t.Fatal("cancelled empty snapshot published")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exposure: %v", err)
	}
}

func TestSnapshotOwnedFileRejectsShrinkAndGrowthDuringCopy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(string) error
	}{
		{name: "shrink", mutate: func(path string) error { return os.Truncate(path, 2) }},
		{name: "growth", mutate: func(path string) error { return os.Truncate(path, 12) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			destination := filepath.Join(dir, "destination")
			if err := os.WriteFile(source, []byte("0123456789"), 0o600); err != nil {
				t.Fatal(err)
			}
			var mutationErr error
			if err := snapshotOwnedFileBounded(context.Background(), source, destination, 10, func() { mutationErr = tc.mutate(source) }); err == nil {
				t.Fatal("source mutation was accepted")
			}
			if mutationErr != nil {
				t.Fatalf("source mutation: %v", mutationErr)
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("destination exposure: %v", err)
			}
			if matches, _ := filepath.Glob(filepath.Join(dir, ".snapshot-*")); len(matches) != 0 {
				t.Fatalf("leaked temporary snapshots: %v", matches)
			}
		})
	}
}
