package clipfetch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

const acquisitionCleanupGrace = 24 * time.Hour

type AcquisitionCleanupStore interface {
	GetAcquisitionRun(context.Context, string, time.Time) (filler.AcquisitionRun, error)
	ListRecoverableAcquisitionArtifactsAfter(context.Context, filler.AcquisitionArtifactCursor, int) ([]filler.AcquisitionArtifact, error)
}

// AcquisitionCleaner owns the only user-triggered storage cleanup in Filler. It removes stale
// numbered attempt directories only after their durable acquisition run is terminal and no staged
// or repair artifact still depends on them. Unknown paths and symlinked trees are never candidates.
type AcquisitionCleaner struct {
	watchDir string
	store    AcquisitionCleanupStore
	now      func() time.Time
	remove   func(string) error
}

type acquisitionCleanupCandidate struct {
	path  string
	bytes int64
}

func NewAcquisitionCleaner(watchDir string, store AcquisitionCleanupStore, now func() time.Time) *AcquisitionCleaner {
	if now == nil {
		now = time.Now
	}
	return &AcquisitionCleaner{watchDir: watchDir, store: store, now: now, remove: os.RemoveAll}
}

func (c *AcquisitionCleaner) Preview(ctx context.Context) (filler.StorageCleanupPreview, error) {
	candidates, err := c.candidates(ctx)
	if err != nil {
		return filler.StorageCleanupPreview{}, err
	}
	return cleanupPreview(candidates), nil
}

func (c *AcquisitionCleaner) Clean(ctx context.Context) (filler.StorageCleanupResult, error) {
	var result filler.StorageCleanupResult
	candidates, err := c.candidates(ctx)
	if err != nil {
		return result, err
	}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		// Re-read immediately before removal. A path that became young, symlinked, or otherwise
		// non-private after preview fails closed and remains on disk.
		bytes, newest, safe, statErr := privateTree(candidate.path)
		if statErr != nil || !safe || newest.After(c.now().UTC().Add(-acquisitionCleanupGrace)) {
			result.FailedItems++
			continue
		}
		if err := c.remove(candidate.path); err != nil {
			result.FailedItems++
			continue
		}
		result.RemovedItems++
		result.RemovedBytes += bytes
		// Remove the acquisition-id directory only when it became empty. Failure is harmless: an
		// empty private parent consumes no meaningful capacity and can be retried later.
		_ = os.Remove(filepath.Dir(candidate.path))
	}
	remaining, previewErr := c.Preview(ctx)
	result.Remaining = remaining
	return result, previewErr
}

func (c *AcquisitionCleaner) candidates(ctx context.Context) ([]acquisitionCleanupCandidate, error) {
	if c == nil || c.store == nil || strings.TrimSpace(c.watchDir) == "" {
		return nil, nil
	}
	root := filepath.Join(c.watchDir, ".loomarr-acquisitions")
	rootInfo, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect acquisition staging: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, errors.New("acquisition staging root is not a private directory")
	}
	protected, err := c.protectedStaging(ctx, root)
	if err != nil {
		return nil, err
	}
	acquisitions, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read acquisition staging: %w", err)
	}
	cutoff := c.now().UTC().Add(-acquisitionCleanupGrace)
	candidates := make([]acquisitionCleanupCandidate, 0)
	for _, acquisition := range acquisitions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if acquisition.Type()&os.ModeSymlink != 0 || !acquisition.IsDir() || acquisition.Name() == "" {
			continue
		}
		run, runErr := c.store.GetAcquisitionRun(ctx, acquisition.Name(), c.now().UTC())
		if runErr != nil || (run.Status != filler.AcquisitionSuccess && run.Status != filler.AcquisitionError) {
			continue
		}
		acquisitionDir := filepath.Join(root, acquisition.Name())
		attempts, readErr := os.ReadDir(acquisitionDir)
		if readErr != nil {
			continue
		}
		for _, attempt := range attempts {
			if attempt.Type()&os.ModeSymlink != 0 || !attempt.IsDir() || !numberedAttempt(attempt.Name()) {
				continue
			}
			path := filepath.Join(acquisitionDir, attempt.Name())
			if protected[path] {
				continue
			}
			bytes, newest, safe, statErr := privateTree(path)
			if statErr != nil || !safe || newest.After(cutoff) {
				continue
			}
			candidates = append(candidates, acquisitionCleanupCandidate{path: path, bytes: bytes})
		}
	}
	return candidates, nil
}

func (c *AcquisitionCleaner) protectedStaging(ctx context.Context, stagingRoot string) (map[string]bool, error) {
	protected := make(map[string]bool)
	var cursor filler.AcquisitionArtifactCursor
	for {
		artifacts, err := c.store.ListRecoverableAcquisitionArtifactsAfter(ctx, cursor, 500)
		if err != nil {
			return nil, fmt.Errorf("read acquisition cleanup protection: %w", err)
		}
		if len(artifacts) == 0 {
			return protected, nil
		}
		for _, artifact := range artifacts {
			cursor = filler.AcquisitionArtifactCursor{UpdatedAt: artifact.UpdatedAt, ID: artifact.ID}
			if artifact.State != filler.ArtifactStaged && artifact.State != filler.ArtifactRepair {
				continue
			}
			path := filepath.Clean(filepath.Join(c.watchDir, filepath.FromSlash(artifact.StagingPath)))
			rel, relErr := filepath.Rel(stagingRoot, path)
			if relErr != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) >= 2 && numberedAttempt(parts[1]) {
				protected[filepath.Join(stagingRoot, parts[0], parts[1])] = true
			}
		}
		if len(artifacts) < 500 {
			return protected, nil
		}
	}
}

func numberedAttempt(name string) bool {
	if len(name) != 3 {
		return false
	}
	_, err := strconv.Atoi(name)
	return err == nil
}

func privateTree(root string) (bytes int64, newest time.Time, safe bool, err error) {
	safe = true
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, infoErr := os.Lstat(path)
		if infoErr != nil {
			return infoErr
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			safe = false
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		if info.Mode().IsRegular() {
			bytes += info.Size()
		}
		return nil
	})
	return bytes, newest, safe, err
}

func cleanupPreview(candidates []acquisitionCleanupCandidate) filler.StorageCleanupPreview {
	preview := filler.StorageCleanupPreview{Items: len(candidates)}
	for _, candidate := range candidates {
		preview.Bytes += candidate.bytes
	}
	return preview
}
