package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/testkit"
)

// An in-memory images.FetchStore for the adapter tests in this package.
//
// ⚠ It lives here rather than in internal/testkit on purpose. The testkit's rule (AGENTS.md) is one
// shared implementation per EXTERNAL service — a media server, Tunarr, Seerr, TMDB — so that no
// phase invents a private mock of something the product talks to over a wire. The image store is
// neither external nor a service: it is this codebase's own persistence interface, and the real
// conformance proof for it is the store suite running against SQLite and Postgres. What these
// adapter tests need is something that holds rows so the resolver's own decisions can be observed.
type memImageStore struct {
	mu     sync.Mutex
	images map[string]images.Image
	refs   map[string][]images.Ref
	derivs map[string][]images.Derivative
}

func newMemImageStore() *memImageStore {
	return &memImageStore{
		images: map[string]images.Image{},
		refs:   map[string][]images.Ref{},
		derivs: map[string][]images.Derivative{},
	}
}

func (m *memImageStore) PutImage(_ context.Context, img images.Image) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.images[img.Hash] = img
	return nil
}

func (m *memImageStore) GetImage(_ context.Context, hash string) (images.Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	img, ok := m.images[hash]
	if !ok {
		return images.Image{}, images.ErrNotFound
	}
	return img, nil
}

// ⚠ Mirrors the SQL predicate — fetched rows only. A fake that matched on source_url alone would
// hand back the URL-keyed placeholder, whose hash is about to stop resolving, and every caller
// would look correct here while breaking against a real store.
func (m *memImageStore) GetFetchedBySourceURL(_ context.Context, src string) (images.Image, error) {
	if src == "" {
		return images.Image{}, images.ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, img := range m.images {
		if img.SourceURL == src && !img.OriginFetchedAt.IsZero() {
			return img, nil
		}
	}
	return images.Image{}, images.ErrNotFound
}

func (m *memImageStore) TouchImage(_ context.Context, hash string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if img, ok := m.images[hash]; ok {
		img.LastUsedAt = at
		m.images[hash] = img
	}
	return nil
}

func (m *memImageStore) PutRef(_ context.Context, ref images.Ref) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refs[ref.ImageHash] = append(m.refs[ref.ImageHash], ref)
	return nil
}

func (m *memImageStore) PutDerivative(_ context.Context, d images.Derivative) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.derivs[d.ImageHash] = append(m.derivs[d.ImageHash], d)
	return nil
}

func (m *memImageStore) ListDerivatives(_ context.Context, hash string) ([]images.Derivative, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.derivs[hash], nil
}

func (m *memImageStore) ListAwaitingFetch(_ context.Context, limit int) ([]images.Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]images.Image, 0, limit)
	for _, img := range m.images {
		if img.Origin == images.OriginRemote && img.OriginFetchedAt.IsZero() {
			out = append(out, img)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memImageStore) ListByOrigin(_ context.Context, origin images.Origin, limit int) ([]images.Image, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]images.Image, 0, limit)
	for _, img := range m.images {
		if img.Origin == origin {
			out = append(out, img)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memImageStore) RepointRefs(_ context.Context, from, to string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if refs, ok := m.refs[from]; ok {
		for i := range refs {
			refs[i].ImageHash = to
		}
		m.refs[to] = append(m.refs[to], refs...)
		delete(m.refs, from)
	}
	return nil
}

func (m *memImageStore) DeleteImage(_ context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.images, hash)
	delete(m.refs, hash)
	delete(m.derivs, hash)
	return nil
}

// newTestImageService builds a real *images.Service over the in-memory store, writing blobs into a
// temp dir. `base` is the public URL the service builds rendition URLs from, so a test can assert
// the exact string a client receives.
func newTestImageService(t testing.TB, dir, base string, st images.Store) *images.Service {
	t.Helper()
	return images.New(images.Config{
		Dir:           dir,
		PublicBaseURL: func() string { return base },
	}, st, testkit.RustImageRenderer(t), nil)
}
