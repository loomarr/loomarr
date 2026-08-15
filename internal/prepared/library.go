// Package prepared owns immutable, reusable playout publications.
//
// A publication is addressed by its source fingerprint and rendition contract. Builders may write
// only into a private workspace; readers cannot observe those bytes until Library has validated and
// atomically renamed the complete directory into place.
package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const publicationMetadata = ".publication.json"

var (
	// ErrInvalidSpecification means the source identity or rendition contract is incomplete.
	ErrInvalidSpecification = errors.New("prepared: invalid specification")
	// ErrIncomplete means a builder did not produce every non-empty regular file it declared.
	ErrIncomplete = errors.New("prepared: incomplete publication")
)

// RenditionContract is the transport-independent identity of prepared media. New transport
// adapters select a compatible contract; they do not add platform names to publication identity.
type RenditionContract struct {
	VideoCodec        string `json:"videoCodec"`
	VideoProfile      string `json:"videoProfile,omitempty"`
	VideoLevel        string `json:"videoLevel,omitempty"`
	PixelFormat       string `json:"pixelFormat,omitempty"`
	HDR               string `json:"hdr,omitempty"`
	AudioCodec        string `json:"audioCodec"`
	AudioLayout       string `json:"audioLayout,omitempty"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	FrameRate         int    `json:"frameRate,omitempty"`
	VideoBitrateKbps  int    `json:"videoBitrateKbps,omitempty"`
	AudioBitrateKbps  int    `json:"audioBitrateKbps,omitempty"`
	SegmentDurationMS int    `json:"segmentDurationMs"`
	PackagingVersion  int    `json:"packagingVersion"`
}

// Specification identifies one immutable publication. SourceFingerprint must change whenever the
// source bytes or selected tracks change; doing so makes stale output unreachable by construction.
type Specification struct {
	SourceFingerprint string            `json:"sourceFingerprint"`
	Rendition         RenditionContract `json:"rendition"`
}

// Output is the builder's declaration of the files that form a complete publication. Paths are
// relative to its private workspace. Library validates and durably syncs them before publication.
type Output struct {
	Files []string
}

// Builder prepares one rendition inside workspace. The path is private and on the same filesystem
// as the final publication, which lets Library make the result visible with one atomic rename.
type Builder func(ctx context.Context, workspace string) (Output, error)

// Publication is a complete immutable directory. Files contains safe paths relative to Directory.
type Publication struct {
	Key       string
	Directory string
	Files     []string
}

// Asset is one opened immutable file from a complete publication. Library keeps publication
// metadata and paths private; delivery code receives only the opened bytes and their timestamp.
type Asset struct {
	Content  *os.File
	Modified time.Time
}

type metadata struct {
	Version       int           `json:"version"`
	Specification Specification `json:"specification"`
	Files         []string      `json:"files"`
}

// Library hides workspace cleanup, validation, durability, reuse, and atomic publication behind
// Publish and Lookup. Its root must be dedicated to prepared media.
type Library struct {
	root string

	mu      sync.Mutex
	locks   map[string]*keyLock
	catalog map[string]catalogEntry
}

type keyLock struct {
	mu   sync.Mutex
	refs int
}

type catalogEntry struct {
	specification Specification
	publication   Publication
	files         map[string]struct{}
}

// NewLibrary creates (or opens) a prepared-media root.
func NewLibrary(root string) (*Library, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || root == "" {
		return nil, fmt.Errorf("%w: empty library root", ErrInvalidSpecification)
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("prepared: create library root: %w", err)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("prepared: resolve library root: %w", err)
	}
	return &Library{
		root: abs, locks: make(map[string]*keyLock), catalog: make(map[string]catalogEntry),
	}, nil
}

// Publish returns an existing publication for spec or runs build exactly once per process and key.
// Failed and incomplete builds are removed without affecting any other immutable publication.
func (l *Library) Publish(ctx context.Context, spec Specification, build Builder) (Publication, error) {
	key, err := keyFor(spec)
	if err != nil {
		return Publication{}, err
	}
	if build == nil {
		return Publication{}, fmt.Errorf("%w: nil builder", ErrInvalidSpecification)
	}
	unlock := l.lock(key)
	defer unlock()

	if pub, ok, err := l.Lookup(spec); err != nil || ok {
		return pub, err
	}
	if err := ctx.Err(); err != nil {
		return Publication{}, err
	}

	workspace, err := os.MkdirTemp(l.root, ".staging-"+key+"-")
	if err != nil {
		return Publication{}, fmt.Errorf("prepared: create workspace: %w", err)
	}
	removeWorkspace := true
	defer func() {
		if removeWorkspace {
			_ = os.RemoveAll(workspace)
		}
	}()

	output, err := build(ctx, workspace)
	if err != nil {
		return Publication{}, err
	}
	files, err := validateOutput(workspace, output)
	if err != nil {
		return Publication{}, err
	}
	if err := ctx.Err(); err != nil {
		return Publication{}, err
	}
	for _, name := range files {
		if err := syncFile(filepath.Join(workspace, name)); err != nil {
			return Publication{}, err
		}
	}

	record := metadata{Version: 1, Specification: spec, Files: files}
	body, err := json.Marshal(record)
	if err != nil {
		return Publication{}, fmt.Errorf("prepared: encode metadata: %w", err)
	}
	metaPath := filepath.Join(workspace, publicationMetadata)
	if err := os.WriteFile(metaPath, append(body, '\n'), 0o600); err != nil {
		return Publication{}, fmt.Errorf("prepared: write metadata: %w", err)
	}
	if err := syncFile(metaPath); err != nil {
		return Publication{}, err
	}
	if err := syncDir(workspace); err != nil {
		return Publication{}, err
	}

	final := filepath.Join(l.root, key)
	if err := os.Rename(workspace, final); err != nil {
		// Another process may have won the same immutable publication race.
		if pub, ok, lookupErr := l.Lookup(spec); lookupErr == nil && ok {
			return pub, nil
		}
		return Publication{}, fmt.Errorf("prepared: publish: %w", err)
	}
	removeWorkspace = false
	if err := syncDir(l.root); err != nil {
		return Publication{}, err
	}
	pub := Publication{Key: key, Directory: final, Files: append([]string(nil), files...)}
	l.remember(spec, pub)
	return pub, nil
}

// Lookup returns only a complete publication whose metadata matches spec. Staging directories and
// directories without a valid publication marker are never visible as hits.
func (l *Library) Lookup(spec Specification) (Publication, bool, error) {
	key, err := keyFor(spec)
	if err != nil {
		return Publication{}, false, err
	}
	if entry, ok := l.cached(key); ok {
		if entry.specification != spec {
			return Publication{}, false, ErrIncomplete
		}
		return clonePublication(entry.publication), true, nil
	}
	entry, ok, err := l.load(key)
	if err != nil || !ok {
		return Publication{}, ok, err
	}
	if entry.specification != spec {
		return Publication{}, false, ErrIncomplete
	}
	return clonePublication(entry.publication), true, nil
}

func (l *Library) load(key string) (catalogEntry, bool, error) {
	dir := filepath.Join(l.root, key)
	body, err := os.ReadFile(filepath.Join(dir, publicationMetadata))
	if errors.Is(err, os.ErrNotExist) {
		return catalogEntry{}, false, nil
	}
	if err != nil {
		return catalogEntry{}, false, fmt.Errorf("prepared: read metadata: %w", err)
	}
	var record metadata
	if err := json.Unmarshal(body, &record); err != nil || record.Version != 1 {
		return catalogEntry{}, false, ErrIncomplete
	}
	recordedKey, err := keyFor(record.Specification)
	if err != nil || recordedKey != key {
		return catalogEntry{}, false, ErrIncomplete
	}
	files, err := validateOutput(dir, Output{Files: record.Files})
	if err != nil {
		return catalogEntry{}, false, err
	}
	pub := Publication{Key: key, Directory: dir, Files: files}
	l.remember(record.Specification, pub)
	entry, _ := l.cached(key)
	return entry, true, nil
}

// Open opens one file declared by a complete immutable publication. The publication key and asset
// name are both untrusted URL material: malformed keys, traversal, metadata, and undeclared files
// are reported as misses rather than exposing the library layout.
func (l *Library) Open(key, name string) (Asset, bool, error) {
	if len(key) != sha256.Size*2 {
		return Asset{}, false, nil
	}
	if _, err := hex.DecodeString(key); err != nil {
		return Asset{}, false, nil
	}
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == publicationMetadata {
		return Asset{}, false, nil
	}

	entry, ok := l.cached(key)
	if !ok {
		var err error
		entry, ok, err = l.load(key)
		if err != nil || !ok {
			return Asset{}, ok, err
		}
	}
	if _, declared := entry.files[clean]; !declared {
		return Asset{}, false, nil
	}
	info, err := regularFileInside(entry.publication.Directory, clean)
	if err != nil || info.Size() == 0 {
		return Asset{}, false, ErrIncomplete
	}
	f, err := os.Open(filepath.Join(entry.publication.Directory, clean))
	if err != nil {
		return Asset{}, false, fmt.Errorf("prepared: open asset: %w", err)
	}
	return Asset{Content: f, Modified: info.ModTime()}, true, nil
}

func (l *Library) cached(key string) (catalogEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.catalog[key]
	return entry, ok
}

func (l *Library) remember(spec Specification, pub Publication) {
	files := make(map[string]struct{}, len(pub.Files))
	for _, name := range pub.Files {
		files[name] = struct{}{}
	}
	l.mu.Lock()
	l.catalog[pub.Key] = catalogEntry{
		specification: spec, publication: clonePublication(pub), files: files,
	}
	l.mu.Unlock()
}

func clonePublication(pub Publication) Publication {
	pub.Files = append([]string(nil), pub.Files...)
	return pub
}

func keyFor(spec Specification) (string, error) {
	r := spec.Rendition
	if strings.TrimSpace(spec.SourceFingerprint) == "" || strings.TrimSpace(r.VideoCodec) == "" ||
		strings.TrimSpace(r.AudioCodec) == "" || r.Width <= 0 || r.Height <= 0 ||
		r.FrameRate <= 0 || r.VideoBitrateKbps <= 0 || r.AudioBitrateKbps <= 0 ||
		r.SegmentDurationMS <= 0 || r.PackagingVersion <= 0 {
		return "", ErrInvalidSpecification
	}
	body, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("prepared: encode specification: %w", err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func validateOutput(root string, output Output) ([]string, error) {
	if len(output.Files) == 0 {
		return nil, ErrIncomplete
	}
	files := make([]string, 0, len(output.Files))
	seen := make(map[string]struct{}, len(output.Files))
	for _, name := range output.Files {
		clean := filepath.Clean(name)
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("%w: unsafe file %q", ErrIncomplete, name)
		}
		if _, ok := seen[clean]; ok {
			return nil, fmt.Errorf("%w: duplicate file %q", ErrIncomplete, clean)
		}
		info, err := regularFileInside(root, clean)
		if err != nil || info.Size() == 0 {
			return nil, fmt.Errorf("%w: file %q", ErrIncomplete, clean)
		}
		seen[clean] = struct{}{}
		files = append(files, clean)
	}
	return files, nil
}

// regularFileInside rejects symlinks in every path component. Merely cleaning a relative path is
// insufficient: a builder could otherwise point "segments" outside its workspace with a symlink,
// causing validation and the later durability sync to operate on unrelated operator files.
func regularFileInside(root, name string) (os.FileInfo, error) {
	current := root
	parts := strings.Split(name, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrIncomplete
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, ErrIncomplete
		}
		if i == len(parts)-1 {
			if !info.Mode().IsRegular() {
				return nil, ErrIncomplete
			}
			return info, nil
		}
	}
	return nil, ErrIncomplete
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("prepared: open for sync: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("prepared: sync file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("prepared: close synced file: %w", err)
	}
	return nil
}

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("prepared: open directory for sync: %w", err)
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return fmt.Errorf("prepared: sync directory: %w", err)
	}
	if err := d.Close(); err != nil {
		return fmt.Errorf("prepared: close synced directory: %w", err)
	}
	return nil
}

func (l *Library) lock(key string) func() {
	l.mu.Lock()
	kl := l.locks[key]
	if kl == nil {
		kl = &keyLock{}
		l.locks[key] = kl
	}
	kl.refs++
	l.mu.Unlock()

	kl.mu.Lock()
	return func() {
		kl.mu.Unlock()
		l.mu.Lock()
		kl.refs--
		if kl.refs == 0 {
			delete(l.locks, key)
		}
		l.mu.Unlock()
	}
}
