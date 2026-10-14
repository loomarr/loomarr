package clipfetch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

type cleanupStore struct {
	runs      map[string]filler.AcquisitionRun
	artifacts []filler.AcquisitionArtifact
}

func (s cleanupStore) GetAcquisitionRun(_ context.Context, id string, _ time.Time) (filler.AcquisitionRun, error) {
	run, ok := s.runs[id]
	if !ok {
		return filler.AcquisitionRun{}, errors.New("not found")
	}
	return run, nil
}

func (s cleanupStore) ListRecoverableAcquisitionArtifactsAfter(
	_ context.Context, after filler.AcquisitionArtifactCursor, limit int,
) ([]filler.AcquisitionArtifact, error) {
	rows := make([]filler.AcquisitionArtifact, 0, limit)
	for _, artifact := range s.artifacts {
		if artifact.UpdatedAt.Before(after.UpdatedAt) ||
			(artifact.UpdatedAt.Equal(after.UpdatedAt) && artifact.ID <= after.ID) {
			continue
		}
		rows = append(rows, artifact)
		if len(rows) == limit {
			break
		}
	}
	return rows, nil
}

func TestAcquisitionCleanerRemovesOnlyProvenDisposableTerminalStaging(t *testing.T) {
	now := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)
	watch := t.TempDir()
	root := filepath.Join(watch, ".loomarr-acquisitions")
	writeAttempt := func(acquisition, attempt string, bytes int) string {
		t.Helper()
		dir := filepath.Join(root, acquisition, attempt)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		media := filepath.Join(dir, "download.mp4")
		if err := os.WriteFile(media, make([]byte, bytes), 0o600); err != nil {
			t.Fatal(err)
		}
		old := now.Add(-48 * time.Hour)
		if err := os.Chtimes(media, old, old); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	safe := writeAttempt("terminal", "000", 100)
	active := writeAttempt("active", "000", 200)
	repair := writeAttempt("repair", "000", 300)
	symlinked := writeAttempt("symlinked", "000", 400)
	if err := os.Symlink(filepath.Join(symlinked, "download.mp4"), filepath.Join(symlinked, "linked.mp4")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	unknown := writeAttempt("unknown", "custom", 500)

	cleaner := NewAcquisitionCleaner(watch, cleanupStore{
		runs: map[string]filler.AcquisitionRun{
			"terminal":  {Status: filler.AcquisitionSuccess},
			"active":    {Status: filler.AcquisitionRunning},
			"repair":    {Status: filler.AcquisitionError},
			"symlinked": {Status: filler.AcquisitionError},
			"unknown":   {Status: filler.AcquisitionError},
		},
		artifacts: []filler.AcquisitionArtifact{{
			ID: "repair-artifact", State: filler.ArtifactRepair,
			StagingPath: filepath.ToSlash(filepath.Join(".loomarr-acquisitions", "repair", "000", "download.mp4")),
			UpdatedAt:   now.Add(-time.Hour),
		}},
	}, func() time.Time { return now })

	preview, err := cleaner.Preview(t.Context())
	if err != nil || preview.Items != 1 || preview.Bytes != 100 {
		t.Fatalf("preview = %+v, err=%v", preview, err)
	}
	result, err := cleaner.Clean(t.Context())
	if err != nil || result.RemovedItems != 1 || result.RemovedBytes != 100 || result.FailedItems != 0 || result.Remaining.Items != 0 {
		t.Fatalf("cleanup = %+v, err=%v", result, err)
	}
	if _, err := os.Stat(safe); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("safe terminal staging remains: %v", err)
	}
	for _, path := range []string{active, repair, symlinked, unknown} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("protected path %q was removed: %v", path, err)
		}
	}
}

func TestAcquisitionCleanerReportsRemovalFailureAndFreshRemainingPreview(t *testing.T) {
	now := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)
	watch := t.TempDir()
	attempt := filepath.Join(watch, ".loomarr-acquisitions", "terminal", "000")
	if err := os.MkdirAll(attempt, 0o750); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(attempt, "download.mp4")
	if err := os.WriteFile(media, make([]byte, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-48 * time.Hour)
	if err := os.Chtimes(media, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(attempt, old, old); err != nil {
		t.Fatal(err)
	}
	cleaner := NewAcquisitionCleaner(watch, cleanupStore{
		runs: map[string]filler.AcquisitionRun{"terminal": {Status: filler.AcquisitionError}},
	}, func() time.Time { return now })
	cleaner.remove = func(string) error { return errors.New("read-only filesystem") }

	result, err := cleaner.Clean(t.Context())
	if err != nil || result.RemovedItems != 0 || result.FailedItems != 1 ||
		result.Remaining.Items != 1 || result.Remaining.Bytes != 64 {
		t.Fatalf("cleanup = %+v, err=%v", result, err)
	}
}
