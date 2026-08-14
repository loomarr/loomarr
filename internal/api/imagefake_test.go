package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"github.com/mantonx/loomarr/internal/images"
)

// fakeImageService is the api package's stand-in for api.ImageService (§22, V52).
//
// ⚠ **This is a fake of an INTERNAL collaborator, not a private mock of an external service.**
// AGENTS.md's "phases do not invent private mocks; extend the testkit" rule governs the services
// Loomarr talks to over a network — media server, Tunarr, Seerr, TMDB, the LLM — which is why
// `internal/testkit` holds exactly those and nothing else. The image service is ours, is already
// covered by its own tests in internal/images, and writes to DISK; wiring the real one here would
// make a handler test depend on a temp dir, an encoder and the blob layout, to assert something
// none of those decide. `testAuthorizer` in this package is the same pattern.
//
// What a handler test SHOULD pin is the request the handler constructs, which is where the
// security-relevant decisions live: role, visibility, origin and the owner Ref. Those are recorded
// verbatim below so a test can assert them, because every one of them is a silent failure if it
// drifts — a channel icon ingested as `member` visibility would 404 for Tunarr, which fetches it
// with no credentials, and nothing else in the suite would notice.
type fakeImageService struct {
	mu sync.Mutex

	// lastIngest is the request the handler built, and lastBytes what it handed over.
	lastIngest images.IngestRequest
	lastBytes  []byte
	ingests    int

	// ingestErr, when set, makes Ingest fail — for the "storage broke" branch.
	ingestErr error

	// records is the hash → image map Get serves from, seeded by Ingest.
	records map[string]images.Image

	// formats is which rendition formats "already exist" — see HasFormat.
	formats map[images.Format]bool
}

func newFakeImageService() *fakeImageService {
	return &fakeImageService{records: map[string]images.Image{}, formats: map[images.Format]bool{}}
}

// Ingest records the request and derives a hash from the BYTES.
//
// ⚠ The hash is a real sha256 of the content, never the owner id or a fixed string. §10 shipped
// two live bugs because fixtures set a clip's hash and path to the same value, making a
// hash-keyed call indistinguishable from a path-keyed one; images.Image's own doc comment carries
// that warning. Deriving it here means a test that asserts a logo URL is genuinely asserting
// content addressing rather than a coincidence.
func (f *fakeImageService) Ingest(_ context.Context, r io.Reader, req images.IngestRequest) (images.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return images.Image{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ingests++
	f.lastIngest = req
	f.lastBytes = data
	if f.ingestErr != nil {
		return images.Image{}, f.ingestErr
	}
	sum := sha256.Sum256(data)
	img := images.Image{
		Hash:       hex.EncodeToString(sum[:]),
		Origin:     req.Origin,
		Visibility: req.Visibility,
		Role:       req.Role,
		Width:      512,
		Height:     512,
	}
	f.records[img.Hash] = img
	return img, nil
}

func (f *fakeImageService) Get(_ context.Context, hash string) (images.Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	img, ok := f.records[hash]
	if !ok {
		return images.Image{}, images.ErrNotFound
	}
	return img, nil
}

func (f *fakeImageService) Rendition(_ context.Context, hash string, _ images.Format, _ int) (images.Rendition, error) {
	return images.Rendition{Hash: hash}, nil
}

// HasFormat defaults to FALSE for every format, which is the state a freshly-ingested image is
// actually in: AVIF is job-produced, so it does not exist at upload time.
//
// ⚠ Defaulting to true would make the fake disagree with reality in the exact direction that hid
// the bug this method exists to prevent — advertising an AVIF that 404s, which `<picture>` turns
// into a broken image rather than a fallback. `formats` lets a test opt one in explicitly.
func (f *fakeImageService) HasFormat(_ context.Context, _ string, format images.Format) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.formats[format], nil
}

// URLFor mirrors the real service's shape (`/v1/images/{hash}/w{width}.{ext}`) with no base URL,
// so a test can assert the exact string a channel's logo is set to.
func (f *fakeImageService) URLFor(hash string, width int, format images.Format) string {
	return fmt.Sprintf("/v1/images/%s/w%d.%s", hash, width, format.Ext())
}

func (f *fakeImageService) SrcSet(hash string, role images.Role, format images.Format) string {
	out := ""
	for i, w := range role.Widths() {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%s %dw", f.URLFor(hash, w, format), w)
	}
	return out
}

// snapshot returns the recorded ingest under the lock, for assertions.
func (f *fakeImageService) snapshot() (images.IngestRequest, []byte, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastIngest, f.lastBytes, f.ingests
}
