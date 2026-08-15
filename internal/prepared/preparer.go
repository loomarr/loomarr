package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	// ErrInvalidSource means preparation was asked to read something other than a regular file or
	// was given an invalid selected-track identity.
	ErrInvalidSource = errors.New("prepared: invalid source")
	// ErrSourceChanged means the source mutated while its content identity was being computed.
	ErrSourceChanged = errors.New("prepared: source changed while fingerprinting")
	// ErrPackagerUnavailable means preparation has no media packager wired.
	ErrPackagerUnavailable = errors.New("prepared: packager unavailable")
)

// Source is the exact local input and selected tracks preparation reads. Track selection is source
// identity: two airings selecting different audio tracks must never reuse the same publication.
type Source struct {
	Path       string `json:"path"`
	AudioTrack int    `json:"audioTrack"`
}

// Request describes one reusable prepared rendition without naming a Channel or client platform.
type Request struct {
	Source    Source            `json:"source"`
	Rendition RenditionContract `json:"rendition"`
}

// Packager writes every immutable media file for a request into workspace and declares the
// complete output. Library owns validation and the atomic commit after this returns.
type Packager interface {
	Package(context.Context, string, Source, RenditionContract) (Output, error)
}

// Preparer is the control-plane entry point for prepared media. Prepare may hash and package;
// Lookup performs only a stat, warmed fingerprint lookup, and immutable Library lookup, making it
// safe for the tune path without turning a cold request into work.
type Preparer struct {
	library   *Library
	packager  Packager
	readiness *Readiness

	mu    sync.Mutex
	locks map[fileVersion]*fingerprintLock
}

type fileVersion struct {
	path       string
	size       int64
	modifiedNS int64
	audioTrack int
}

type fingerprintLock struct {
	mu   sync.Mutex
	refs int
}

type PreparerDependencies struct {
	Library   *Library
	Packager  Packager
	Readiness *Readiness
}

func NewPreparer(deps PreparerDependencies) *Preparer {
	return &Preparer{
		library: deps.Library, packager: deps.Packager, readiness: deps.Readiness,
		locks: make(map[fileVersion]*fingerprintLock),
	}
}

// Lookup reports a complete publication specification only when the source fingerprint has already
// been warmed by Prepare. It never reads source bytes and never calls Packager.
func (p *Preparer) Lookup(request Request) (Specification, bool, error) {
	if p == nil || p.library == nil || p.readiness == nil {
		return Specification{}, false, nil
	}
	_, version, err := sourceVersion(request.Source)
	if err != nil {
		return Specification{}, false, err
	}
	fingerprint, warmed := p.readiness.fingerprint(version)
	if !warmed {
		return Specification{}, false, nil
	}
	spec := Specification{SourceFingerprint: fingerprint, Rendition: request.Rendition}
	_, ready, err := p.library.Peek(spec)
	if err != nil || !ready {
		return Specification{}, false, err
	}
	return spec, true, nil
}

// Prepare computes stable content identity and publishes the requested rendition. Concurrent and
// cross-Channel requests for the same source/rendition share the fingerprint and Library build.
func (p *Preparer) Prepare(ctx context.Context, request Request) (Publication, error) {
	if p == nil || p.library == nil || p.packager == nil || p.readiness == nil {
		return Publication{}, ErrPackagerUnavailable
	}
	source, version, err := sourceVersion(request.Source)
	if err != nil {
		return Publication{}, err
	}
	fingerprint, err := p.fingerprint(ctx, source, version)
	if err != nil {
		return Publication{}, err
	}
	spec := Specification{SourceFingerprint: fingerprint, Rendition: request.Rendition}
	return p.library.Publish(ctx, spec, func(ctx context.Context, workspace string) (Output, error) {
		output, err := p.packager.Package(ctx, workspace, source, request.Rendition)
		if err != nil {
			return Output{}, err
		}
		_, after, err := sourceVersion(source)
		if err != nil {
			return Output{}, err
		}
		if after != version {
			return Output{}, ErrSourceChanged
		}
		return output, nil
	})
}

func sourceVersion(source Source) (Source, fileVersion, error) {
	if source.AudioTrack < 0 || source.Path == "" {
		return Source{}, fileVersion{}, ErrInvalidSource
	}
	path, err := filepath.Abs(filepath.Clean(source.Path))
	if err != nil {
		return Source{}, fileVersion{}, fmt.Errorf("%w: resolve path: %v", ErrInvalidSource, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Source{}, fileVersion{}, fmt.Errorf("%w: stat %q", ErrInvalidSource, path)
	}
	source.Path = path
	return source, fileVersion{
		path: path, size: info.Size(), modifiedNS: info.ModTime().UnixNano(), audioTrack: source.AudioTrack,
	}, nil
}

func (p *Preparer) fingerprint(ctx context.Context, source Source, version fileVersion) (string, error) {
	p.mu.Lock()
	if fingerprint, ok := p.readiness.fingerprint(version); ok {
		p.mu.Unlock()
		return fingerprint, nil
	}
	lock := p.locks[version]
	if lock == nil {
		lock = &fingerprintLock{}
		p.locks[version] = lock
	}
	lock.refs++
	p.mu.Unlock()

	lock.mu.Lock()
	defer func() {
		lock.mu.Unlock()
		p.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(p.locks, version)
		}
		p.mu.Unlock()
	}()

	fingerprint, ok := p.readiness.fingerprint(version)
	if ok {
		return fingerprint, nil
	}

	f, err := os.Open(source.Path)
	if err != nil {
		return "", fmt.Errorf("prepared: open source: %w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, contextReader{ctx: ctx, reader: f})
	closeErr := f.Close()
	if copyErr != nil {
		return "", fmt.Errorf("prepared: fingerprint source: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("prepared: close source: %w", closeErr)
	}
	_, after, err := sourceVersion(source)
	if err != nil {
		return "", err
	}
	if after != version {
		return "", ErrSourceChanged
	}
	fingerprint = "sha256:" + hex.EncodeToString(hash.Sum(nil)) + fmt.Sprintf(":audio:%d", source.AudioTrack)
	if err := p.readiness.rememberFingerprint(version, fingerprint); err != nil {
		return "", err
	}
	return fingerprint, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
