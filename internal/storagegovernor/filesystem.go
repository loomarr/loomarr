package storagegovernor

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManagedRoot assigns one filesystem tree to its domain budget. Nested roots in
// the same domain are collapsed so a watch folder beneath filler is counted once.
type ManagedRoot struct {
	Path   string
	Domain Domain
}

// NewFilesystem constructs the production governor for the managed roots. The
// meter counts every regular file on each root's real filesystem, including
// abandoned staging, while refusing unreadable or changing trees.
func NewFilesystem(roots []ManagedRoot, policy func(Domain) Policy) (*Governor, error) {
	normalized, err := normalizeRoots(roots)
	if err != nil {
		return nil, err
	}
	return New(&filesystemMeter{roots: normalized}, policy), nil
}

type filesystemMeter struct {
	roots []ManagedRoot
}

func (m *filesystemMeter) Measure(_ context.Context, path string) (Measurement, error) {
	return platformMeasurement(path)
}

func (m *filesystemMeter) ManagedBytes(ctx context.Context, filesystemID string, domain Domain) (int64, error) {
	if filesystemID == "" {
		return 0, errors.New("filesystem identity is empty")
	}
	var total int64
	for _, configuredRoot := range m.roots {
		if configuredRoot.Domain != domain {
			continue
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		root, err := filepath.EvalSymlinks(configuredRoot.Path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return 0, err
		}
		rootID, err := platformFilesystemID(root)
		if err != nil {
			return 0, err
		}
		if rootID != filesystemID {
			continue
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil // a listed entry vanished mid-walk: private staging is being cleaned up
			}
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() {
				id, err := platformFilesystemID(path)
				if err != nil {
					return err
				}
				if id != filesystemID {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := entryInfo(entry)
			if errors.Is(err, fs.ErrNotExist) {
				return nil // removed after it was listed: another writer's staging cleanup, not a fault
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			total = saturatingAdd(total, info.Size())
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func normalizeRoots(roots []ManagedRoot) ([]ManagedRoot, error) {
	cleaned := make([]ManagedRoot, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root.Path == "" {
			continue
		}
		root.Domain = normalizeDomain(root.Domain)
		if !validDomain(root.Domain) {
			return nil, errors.New("managed storage root has an unknown domain")
		}
		absolute, err := filepath.Abs(root.Path)
		if err != nil {
			return nil, err
		}
		absolute, err = canonicalPath(absolute)
		if err != nil {
			return nil, err
		}
		key := string(root.Domain) + "\x00" + absolute
		if filepath.Separator == '\\' {
			key = strings.ToLower(key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		root.Path = absolute
		cleaned = append(cleaned, root)
	}
	sort.Slice(cleaned, func(left, right int) bool {
		if len(cleaned[left].Path) == len(cleaned[right].Path) {
			return cleaned[left].Path < cleaned[right].Path
		}
		return len(cleaned[left].Path) < len(cleaned[right].Path)
	})
	unique := cleaned[:0]
	for _, candidate := range cleaned {
		nested := false
		for _, parent := range unique {
			relative, err := filepath.Rel(parent.Path, candidate.Path)
			if err == nil && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				if parent.Domain != candidate.Domain {
					return nil, errors.New("managed storage roots overlap across domains")
				}
				nested = true
				break
			}
		}
		if !nested {
			unique = append(unique, candidate)
		}
	}
	return unique, nil
}

func canonicalPath(path string) (string, error) {
	absolute := filepath.Clean(path)
	existing := absolute
	for {
		resolved, err := filepath.EvalSymlinks(existing)
		if err == nil {
			relative, relErr := filepath.Rel(existing, absolute)
			if relErr != nil {
				return "", relErr
			}
			return filepath.Clean(filepath.Join(resolved, relative)), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", err
		}
		existing = parent
	}
}

func nearestExisting(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for {
		_, err := os.Stat(absolute)
		if err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(absolute)
			if resolveErr != nil {
				return "", resolveErr
			}
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(absolute)
		if parent == absolute {
			return "", err
		}
		absolute = parent
	}
}

// entryInfo is a seam so tests can remove an entry between the directory listing and its stat,
// the window in which concurrent staging cleanup races a walk.
var entryInfo = func(entry fs.DirEntry) (fs.FileInfo, error) { return entry.Info() }
