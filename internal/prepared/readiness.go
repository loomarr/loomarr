package prepared

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const (
	readinessMetadata = ".readiness.json"
	readinessVersion  = 1
)

// BindingKey identifies one Channel's selection of one library item. Publications remain shared
// and channel-free; only this control-plane source selection is channel-aware because audio policy
// may differ by Channel.
type BindingKey struct {
	ChannelID     string `json:"channelId"`
	LibraryItemID string `json:"libraryItemId"`
}

// Binding is the resolved local source for one active policy. Policy includes global tier,
// language, and path-map inputs; ChannelPolicy independently invalidates per-Channel audio changes.
type Binding struct {
	Policy        string  `json:"policy"`
	ChannelPolicy string  `json:"channelPolicy,omitempty"`
	Request       Request `json:"request"`
}

type readinessDocument struct {
	Version      int                 `json:"version"`
	Bindings     []bindingRecord     `json:"bindings"`
	Fingerprints []fingerprintRecord `json:"fingerprints"`
}

type bindingRecord struct {
	Key     BindingKey `json:"key"`
	Binding Binding    `json:"binding"`
}

type fingerprintRecord struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	ModifiedNS  int64  `json:"modifiedNs"`
	AudioTrack  int    `json:"audioTrack"`
	Fingerprint string `json:"fingerprint"`
}

// Readiness is the durable, regenerable source index. Reads never touch disk after OpenReadiness;
// control-plane writes snapshot memory and atomically replace the versioned file.
type Readiness struct {
	root string

	mu           sync.RWMutex
	bindings     map[BindingKey]Binding
	fingerprints map[fileVersion]string
	persistMu    sync.Mutex
}

// OpenReadiness loads the prepared root's source index. A malformed index returns a usable empty
// value plus an error so composition can warn and retain immediate live fallback; the next
// successful Remember call atomically replaces the bad bytes.
func OpenReadiness(library *Library) (*Readiness, error) {
	index := &Readiness{
		bindings: make(map[BindingKey]Binding), fingerprints: make(map[fileVersion]string),
	}
	if library == nil || library.root == "" {
		return index, fmt.Errorf("prepared: open readiness index: %w", ErrInvalidSpecification)
	}
	index.root = library.root
	body, err := os.ReadFile(filepath.Join(index.root, readinessMetadata))
	if errors.Is(err, os.ErrNotExist) {
		return index, nil
	}
	if err != nil {
		return index, fmt.Errorf("prepared: read readiness index: %w", err)
	}
	var document readinessDocument
	if err := json.Unmarshal(body, &document); err != nil || document.Version != readinessVersion {
		if err == nil {
			err = fmt.Errorf("unsupported version %d", document.Version)
		}
		return index, fmt.Errorf("prepared: decode readiness index: %w", err)
	}
	if err := index.load(document); err != nil {
		index.bindings = make(map[BindingKey]Binding)
		index.fingerprints = make(map[fileVersion]string)
		return index, err
	}
	return index, nil
}

// Binding returns a source only when every current policy input matches the persisted selection.
func (r *Readiness) Binding(key BindingKey, policy, channelPolicy string) (Request, bool) {
	if r == nil {
		return Request{}, false
	}
	r.mu.RLock()
	binding, ok := r.bindings[key]
	r.mu.RUnlock()
	if !ok || binding.Policy != policy || binding.ChannelPolicy != channelPolicy {
		return Request{}, false
	}
	return binding.Request, true
}

// RememberBinding replaces the prior policy for this Channel/item pair and durably snapshots the
// whole small control file. It is called only by the bounded readiness scheduler, never tune.
func (r *Readiness) RememberBinding(key BindingKey, binding Binding) error {
	return r.RememberBindings(map[BindingKey]Binding{key: binding})
}

// RememberBindings commits one planner pass with one atomic control-file replacement. Memory does
// not expose the new bindings until the durable rename succeeds.
func (r *Readiness) RememberBindings(updates map[BindingKey]Binding) error {
	if r == nil {
		return ErrInvalidSpecification
	}
	normalized := make(map[BindingKey]Binding, len(updates))
	for key, binding := range updates {
		if err := validateBinding(key, &binding); err != nil {
			return err
		}
		normalized[key] = binding
	}
	if len(normalized) == 0 {
		return nil
	}
	r.persistMu.Lock()
	defer r.persistMu.Unlock()
	bindings, fingerprints := r.clone()
	for key, binding := range normalized {
		bindings[key] = binding
	}
	if err := r.persist(documentFrom(bindings, fingerprints)); err != nil {
		return err
	}
	r.mu.Lock()
	r.bindings = bindings
	r.fingerprints = fingerprints
	r.mu.Unlock()
	return nil
}

func (r *Readiness) fingerprint(version fileVersion) (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.RLock()
	fingerprint, ok := r.fingerprints[version]
	r.mu.RUnlock()
	return fingerprint, ok
}

func (r *Readiness) rememberFingerprint(version fileVersion, fingerprint string) error {
	if r == nil || !validFileVersion(version) || strings.TrimSpace(fingerprint) == "" {
		return ErrInvalidSource
	}
	r.persistMu.Lock()
	defer r.persistMu.Unlock()
	bindings, fingerprints := r.clone()
	for prior := range fingerprints {
		if prior.path == version.path && prior.audioTrack == version.audioTrack && prior != version {
			delete(fingerprints, prior)
		}
	}
	fingerprints[version] = fingerprint
	if err := r.persist(documentFrom(bindings, fingerprints)); err != nil {
		return err
	}
	r.mu.Lock()
	r.bindings = bindings
	r.fingerprints = fingerprints
	r.mu.Unlock()
	return nil
}

func (r *Readiness) load(document readinessDocument) error {
	for _, record := range document.Bindings {
		binding := record.Binding
		if err := validateBinding(record.Key, &binding); err != nil {
			return fmt.Errorf("prepared: invalid readiness binding: %w", err)
		}
		if _, duplicate := r.bindings[record.Key]; duplicate {
			return fmt.Errorf("prepared: duplicate readiness binding %+v", record.Key)
		}
		r.bindings[record.Key] = binding
	}
	for _, record := range document.Fingerprints {
		version := fileVersion{
			path: record.Path, size: record.Size, modifiedNS: record.ModifiedNS, audioTrack: record.AudioTrack,
		}
		if !validFileVersion(version) || strings.TrimSpace(record.Fingerprint) == "" {
			return ErrInvalidSource
		}
		if _, duplicate := r.fingerprints[version]; duplicate {
			return fmt.Errorf("prepared: duplicate readiness fingerprint for %q", version.path)
		}
		r.fingerprints[version] = record.Fingerprint
	}
	return nil
}

func (r *Readiness) persist(document readinessDocument) error {
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("prepared: encode readiness index: %w", err)
	}
	temporary, err := os.CreateTemp(r.root, ".readiness-")
	if err != nil {
		return fmt.Errorf("prepared: create readiness workspace: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(append(body, '\n')); err != nil {
		return fmt.Errorf("prepared: write readiness index: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("prepared: sync readiness index: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("prepared: close readiness index: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(r.root, readinessMetadata)); err != nil {
		return fmt.Errorf("prepared: publish readiness index: %w", err)
	}
	removeTemporary = false
	return syncDir(r.root)
}

func (r *Readiness) clone() (map[BindingKey]Binding, map[fileVersion]string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bindings := make(map[BindingKey]Binding, len(r.bindings))
	for key, binding := range r.bindings {
		bindings[key] = binding
	}
	fingerprints := make(map[fileVersion]string, len(r.fingerprints))
	for version, fingerprint := range r.fingerprints {
		fingerprints[version] = fingerprint
	}
	return bindings, fingerprints
}

func documentFrom(
	bindings map[BindingKey]Binding, fingerprints map[fileVersion]string,
) readinessDocument {
	document := readinessDocument{Version: readinessVersion}
	for key, binding := range bindings {
		document.Bindings = append(document.Bindings, bindingRecord{Key: key, Binding: binding})
	}
	for version, fingerprint := range fingerprints {
		document.Fingerprints = append(document.Fingerprints, fingerprintRecord{
			Path: version.path, Size: version.size, ModifiedNS: version.modifiedNS,
			AudioTrack: version.audioTrack, Fingerprint: fingerprint,
		})
	}
	slices.SortFunc(document.Bindings, func(a, b bindingRecord) int {
		if byChannel := strings.Compare(a.Key.ChannelID, b.Key.ChannelID); byChannel != 0 {
			return byChannel
		}
		return strings.Compare(a.Key.LibraryItemID, b.Key.LibraryItemID)
	})
	slices.SortFunc(document.Fingerprints, func(a, b fingerprintRecord) int {
		if byPath := strings.Compare(a.Path, b.Path); byPath != 0 {
			return byPath
		}
		if a.AudioTrack < b.AudioTrack {
			return -1
		}
		if a.AudioTrack > b.AudioTrack {
			return 1
		}
		if a.ModifiedNS < b.ModifiedNS {
			return -1
		}
		if a.ModifiedNS > b.ModifiedNS {
			return 1
		}
		return 0
	})
	return document
}

func validateBinding(key BindingKey, binding *Binding) error {
	if strings.TrimSpace(key.ChannelID) == "" || strings.TrimSpace(key.LibraryItemID) == "" ||
		strings.TrimSpace(binding.Policy) == "" || binding.Request.Source.AudioTrack < 0 ||
		strings.TrimSpace(binding.Request.Source.Path) == "" {
		return ErrInvalidSource
	}
	path, err := filepath.Abs(filepath.Clean(binding.Request.Source.Path))
	if err != nil || !filepath.IsAbs(path) {
		return ErrInvalidSource
	}
	binding.Request.Source.Path = path
	_, err = keyFor(Specification{SourceFingerprint: "readiness", Rendition: binding.Request.Rendition})
	return err
}

func validFileVersion(version fileVersion) bool {
	return filepath.IsAbs(version.path) && version.size >= 0 && version.modifiedNS != 0 && version.audioTrack >= 0
}
