package prepared

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	preparedUseGrace     = 15 * time.Minute
	preparedStartupGrace = 30 * time.Minute
	preparedStagingGrace = 24 * time.Hour
)

var ErrUnknownEntry = errors.New("prepared: unknown library entry")

// PruneResult describes the prepared store after one retention pass. RemainingBytes may exceed
// BudgetBytes when recently served publications are protected; the budget is deliberately soft.
type PruneResult struct {
	BudgetBytes         int64
	TotalBytes          int64
	RemainingBytes      int64
	ProtectedBytes      int64
	PublicationsEvicted int
	BytesEvicted        int64
	StagingRemoved      int
}

type pruneCandidate struct {
	key      string
	dir      string
	bytes    int64
	lastUsed time.Time
	protect  bool
}

// Prune evicts complete cold publications until the logical-byte soft cap is met. It never removes
// an unknown root entry, an invalid publication, or a recently used publication. Per-key locking
// keeps deletion atomic with Lookup/Open/Publish without blocking unrelated channel tunes.
func (l *Library) Prune(
	ctx context.Context, budgetBytes int64, protected []Specification,
) (PruneResult, error) {
	result := PruneResult{BudgetBytes: budgetBytes}
	if l == nil || budgetBytes <= 0 {
		return result, nil
	}
	entries, err := os.ReadDir(l.root)
	if err != nil {
		return result, fmt.Errorf("prepared: read library root: %w", err)
	}
	now := l.now()
	startupProtected := now.Before(l.startedAt.Add(preparedStartupGrace))
	candidates := make([]pruneCandidate, 0, len(entries))
	var errs []error
	protectedKeys := make(map[string]struct{}, len(protected))
	for _, specification := range protected {
		key, keyErr := keyFor(specification)
		if keyErr != nil {
			errs = append(errs, keyErr)
			continue
		}
		protectedKeys[key] = struct{}{}
	}
	for _, dirEntry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		name := dirEntry.Name()
		path := filepath.Join(l.root, name)
		info, statErr := dirEntry.Info()
		if statErr != nil {
			errs = append(errs, fmt.Errorf("prepared: stat %q: %w", name, statErr))
			continue
		}
		if name == readinessMetadata {
			if !info.Mode().IsRegular() {
				errs = append(errs, fmt.Errorf("%w: %q", ErrUnknownEntry, name))
			}
			continue
		}
		if isReadinessTemporary(name) {
			if !info.Mode().IsRegular() {
				errs = append(errs, fmt.Errorf("%w: %q", ErrUnknownEntry, name))
				continue
			}
			if now.Sub(info.ModTime()) > preparedStagingGrace {
				if removeErr := os.Remove(path); removeErr != nil {
					errs = append(errs, fmt.Errorf("prepared: remove abandoned readiness workspace %q: %w", name, removeErr))
				} else {
					result.StagingRemoved++
				}
			}
			continue
		}
		if isOwnedStaging(name) {
			if info.IsDir() && now.Sub(info.ModTime()) > preparedStagingGrace {
				if removeErr := os.RemoveAll(path); removeErr != nil {
					errs = append(errs, fmt.Errorf("prepared: remove abandoned staging %q: %w", name, removeErr))
				} else {
					result.StagingRemoved++
				}
			}
			continue
		}
		if !info.IsDir() || !validPublicationKey(name) {
			errs = append(errs, fmt.Errorf("%w: %q", ErrUnknownEntry, name))
			continue
		}
		entry, ok, readErr := l.readEntry(name)
		if readErr != nil || !ok {
			if readErr == nil {
				readErr = ErrIncomplete
			}
			errs = append(errs, fmt.Errorf("prepared: inspect publication %q: %w", name, readErr))
			continue
		}
		bytes, sizeErr := publicationBytes(entry.publication)
		if sizeErr != nil {
			errs = append(errs, fmt.Errorf("prepared: size publication %q: %w", name, sizeErr))
			continue
		}
		lastUsed := info.ModTime()
		if cached, hit := l.cached(name, false); hit && cached.lastUsed.After(lastUsed) {
			lastUsed = cached.lastUsed
		}
		_, scheduled := protectedKeys[name]
		protect := scheduled || startupProtected || now.Sub(lastUsed) < preparedUseGrace
		result.TotalBytes += bytes
		if protect {
			result.ProtectedBytes += bytes
		}
		candidates = append(candidates, pruneCandidate{
			key: name, dir: path, bytes: bytes, lastUsed: lastUsed, protect: protect,
		})
	}
	result.RemainingBytes = result.TotalBytes
	if result.RemainingBytes <= budgetBytes {
		return result, errors.Join(errs...)
	}

	slices.SortStableFunc(candidates, func(a, b pruneCandidate) int { return a.lastUsed.Compare(b.lastUsed) })
	removed := false
	for _, candidate := range candidates {
		if result.RemainingBytes <= budgetBytes {
			break
		}
		if candidate.protect {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		unlock := l.lock(candidate.key)
		cached, hit := l.cached(candidate.key, false)
		if hit && now.Sub(cached.lastUsed) < preparedUseGrace {
			result.ProtectedBytes += candidate.bytes
			unlock()
			continue
		}
		removeErr := os.RemoveAll(candidate.dir)
		if removeErr == nil {
			l.mu.Lock()
			delete(l.catalog, candidate.key)
			l.mu.Unlock()
		}
		unlock()
		if removeErr != nil {
			errs = append(errs, fmt.Errorf("prepared: evict publication %q: %w", candidate.key, removeErr))
			continue
		}
		removed = true
		result.PublicationsEvicted++
		result.BytesEvicted += candidate.bytes
		result.RemainingBytes -= candidate.bytes
	}
	if removed {
		if syncErr := syncDir(l.root); syncErr != nil {
			errs = append(errs, syncErr)
		}
	}
	return result, errors.Join(errs...)
}

func publicationBytes(publication Publication) (int64, error) {
	info, err := os.Stat(filepath.Join(publication.Directory, publicationMetadata))
	if err != nil || !info.Mode().IsRegular() {
		return 0, ErrIncomplete
	}
	total := info.Size()
	for _, name := range publication.Files {
		info, err = regularFileInside(publication.Directory, name)
		if err != nil {
			return 0, ErrIncomplete
		}
		total += info.Size()
	}
	return total, nil
}

func validPublicationKey(key string) bool {
	if len(key) != 64 {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

func isOwnedStaging(name string) bool {
	remainder := strings.TrimPrefix(name, ".staging-")
	if remainder == name {
		return false
	}
	key, suffix, ok := strings.Cut(remainder, "-")
	return ok && suffix != "" && validPublicationKey(key)
}

func isReadinessTemporary(name string) bool {
	remainder := strings.TrimPrefix(name, ".readiness-")
	return remainder != name && remainder != ""
}
