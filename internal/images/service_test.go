package images

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeStore is an in-memory Store. Small enough to be obviously correct; the real persistence is
// covered by the store conformance suite against BOTH dialects, so duplicating that here would
// only add a second thing to keep in step.
type fakeStore struct {
	images      map[string]Image
	refs        []Ref
	derivatives map[string][]Derivative
	touched     map[string]time.Time
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		images:      map[string]Image{},
		derivatives: map[string][]Derivative{},
		touched:     map[string]time.Time{},
	}
}

func (f *fakeStore) PutImage(_ context.Context, img Image) error {
	// Mirrors the real upsert's single-writer rule: a re-put must not reset created_at.
	if existing, ok := f.images[img.Hash]; ok {
		img.CreatedAt = existing.CreatedAt
	}
	f.images[img.Hash] = img
	return nil
}

func (f *fakeStore) GetImage(_ context.Context, hash string) (Image, error) {
	img, ok := f.images[hash]
	if !ok {
		return Image{}, ErrNotFound
	}
	return img, nil
}

// ⚠ Mirrors the real store's `origin_fetched_at > 0` predicate rather than matching on source_url
// alone. A fake that returned the placeholder row would make Adopt look correct here while the
// SQL one behaved differently — and the placeholder is precisely the row whose hash stops
// resolving, so accepting it is the bug this method exists to prevent.
func (f *fakeStore) GetFetchedBySourceURL(_ context.Context, src string) (Image, error) {
	if src == "" {
		return Image{}, ErrNotFound
	}
	for _, img := range f.images {
		if img.SourceURL == src && !img.OriginFetchedAt.IsZero() {
			return img, nil
		}
	}
	return Image{}, ErrNotFound
}

func (f *fakeStore) TouchImage(_ context.Context, hash string, at time.Time) error {
	f.touched[hash] = at
	return nil
}

func (f *fakeStore) PutRef(_ context.Context, ref Ref) error {
	f.refs = append(f.refs, ref)
	return nil
}

func (f *fakeStore) PutDerivative(_ context.Context, d Derivative) error {
	f.derivatives[d.ImageHash] = append(f.derivatives[d.ImageHash], d)
	return nil
}

func (f *fakeStore) ListDerivatives(_ context.Context, hash string) ([]Derivative, error) {
	return f.derivatives[hash], nil
}

var fixedNow = time.Unix(1_700_000_000, 0)

func newTestService(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	fs := newFakeStore()
	svc := New(Config{
		Dir:            t.TempDir(),
		MaxUploadBytes: func() int64 { return 2 << 20 },
		PublicBaseURL:  func() string { return "https://loomarr.example.test" },
	}, fs, func() time.Time { return fixedNow })
	return svc, fs
}

func TestIngestStoresAndDescribes(t *testing.T) {
	ctx := context.Background()
	svc, fs := newTestService(t)

	data := pngBytes(t, testImage(600, 900))
	got, err := svc.Ingest(ctx, bytes.NewReader(data), IngestRequest{
		Role: RolePoster, Visibility: VisibilityPublic, Origin: OriginUpload,
		OwnerKind: "channel", OwnerID: "ch_1",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	if got.Hash != HashBytes(data) {
		t.Error("hash is not the content hash of the bytes")
	}
	if got.Width != 600 || got.Height != 900 {
		t.Errorf("dimensions = %dx%d, want 600x900 — the frontend needs these for zero CLS", got.Width, got.Height)
	}
	if got.Placeholder == "" {
		t.Error("no placeholder recorded")
	}
	if got.DominantHex == "" {
		t.Error("no dominant colour recorded")
	}
	// ⚠ A locally-produced image must NOT look like it is awaiting a fetch, or the fetch job
	// queues it forever.
	if got.OriginFetchedAt.IsZero() {
		t.Error("originFetchedAt is zero for an upload; the fetch job would pick it up forever")
	}
	if len(fs.refs) != 1 || fs.refs[0].OwnerID != "ch_1" {
		t.Errorf("expected one ref for ch_1, got %+v", fs.refs)
	}
}

// Identical bytes are the same image. Re-ingesting must be a no-op rather than a second record, so
// the upload path is safe to retry.
func TestIngestIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, fs := newTestService(t)
	data := pngBytes(t, testImage(64, 96))

	a, err := svc.Ingest(ctx, bytes.NewReader(data), IngestRequest{Role: RoleIcon})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Ingest(ctx, bytes.NewReader(data), IngestRequest{Role: RoleIcon})
	if err != nil {
		t.Fatal(err)
	}
	if a.Hash != b.Hash {
		t.Error("identical bytes produced different hashes")
	}
	if len(fs.images) != 1 {
		t.Errorf("re-ingest created %d records, want 1", len(fs.images))
	}
}

// ⚠ Decode runs BEFORE anything is written, so a rejected upload never touches the disk.
func TestIngestRefusesNonImagesWithoutWriting(t *testing.T) {
	ctx := context.Background()
	svc, fs := newTestService(t)

	svgish := strings.NewReader(`<?xml version="1.0"?><svg><script>alert(1)</script></svg>`)
	if _, err := svc.Ingest(ctx, svgish, IngestRequest{Role: RoleIcon}); err == nil {
		t.Fatal("Ingest accepted an SVG")
	}
	if len(fs.images) != 0 {
		t.Error("a rejected upload still created a record")
	}
}

func TestIngestEnforcesTheSizeCapOnTheRead(t *testing.T) {
	ctx := context.Background()
	fs := newFakeStore()
	svc := New(Config{Dir: t.TempDir(), MaxUploadBytes: func() int64 { return 100 }}, fs, func() time.Time { return fixedNow })

	data := pngBytes(t, testImage(200, 300)) // comfortably over 100 bytes
	if _, err := svc.Ingest(ctx, bytes.NewReader(data), IngestRequest{Role: RoleIcon}); err == nil {
		t.Fatal("Ingest accepted a body past the cap")
	}
}

func TestRenditionGeneratesWebPAndCachesIt(t *testing.T) {
	ctx := context.Background()
	svc, fs := newTestService(t)

	rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(1000, 1500))),
		IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}

	r, err := svc.Rendition(ctx, rec.Hash, FormatWebP, 342)
	if err != nil {
		t.Fatalf("Rendition: %v", err)
	}
	if r.ContentType != "image/webp" {
		t.Errorf("contentType = %q", r.ContentType)
	}
	if r.Hash != rec.Hash {
		t.Errorf("rendition hash = %q, want the parent %q — this is the ETag", r.Hash, rec.Hash)
	}
	if r.Bytes == 0 {
		t.Error("rendition reports zero bytes")
	}
	if len(fs.derivatives[rec.Hash]) != 1 {
		t.Errorf("expected one derivative row, got %d", len(fs.derivatives[rec.Hash]))
	}

	// A second call must reuse the file rather than re-encode.
	if _, err := svc.Rendition(ctx, rec.Hash, FormatWebP, 342); err != nil {
		t.Fatal(err)
	}
	if n := len(fs.derivatives[rec.Hash]); n != 1 {
		t.Errorf("a cached rendition was re-encoded: %d derivative rows", n)
	}
}

// ⚠ **The amplification guard at the service level.** The width comes from a URL; honouring it
// literally would let an unauthenticated caller make the box encode an unbounded number of
// renditions. Every request must land on a ladder rung.
func TestRenditionSnapsWidthToTheLadder(t *testing.T) {
	ctx := context.Background()
	svc, fs := newTestService(t)

	rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(1000, 1500))),
		IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}

	// Ten arbitrary widths must collapse onto ladder rungs, not create ten files.
	for _, w := range []int{1, 7, 100, 155, 200, 343, 501, 600, 900, 99999} {
		if _, err := svc.Rendition(ctx, rec.Hash, FormatWebP, w); err != nil {
			t.Fatalf("Rendition(%d): %v", w, err)
		}
	}
	got := len(fs.derivatives[rec.Hash])
	if got > len(RolePoster.Widths()) {
		t.Errorf("ten arbitrary widths produced %d renditions; the ladder has only %d rungs — "+
			"an unauthenticated caller could amplify this without bound", got, len(RolePoster.Widths()))
	}
}

// AVIF is job-produced. Asking for one that does not exist is a normal absence, so the caller
// omits that <source> and the browser takes WebP — never an error, never an inline encode.
func TestRenditionDoesNotEncodeAVIFInline(t *testing.T) {
	ctx := context.Background()
	svc, fs := newTestService(t)

	rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(600, 900))),
		IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Rendition(ctx, rec.Hash, FormatAVIF, 342); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AVIF rendition = %v, want ErrNotFound (job-only)", err)
	}
	if len(fs.derivatives[rec.Hash]) != 0 {
		t.Error("an AVIF derivative was produced inline; that is a forked encoder on a request")
	}
}

// A malformed hash must be indistinguishable from a missing one to a caller: a distinct error
// confirms which hashes exist.
func TestGetHidesMalformedHashesBehindNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	for _, bad := range []string{"../../etc/passwd", "", "zzzz"} {
		if _, err := svc.Get(context.Background(), bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q) = %v, want ErrNotFound", bad, err)
		}
	}
}

func TestAdoptRecordsWithoutFetching(t *testing.T) {
	ctx := context.Background()
	svc, fs := newTestService(t)

	const src = "https://image.tmdb.org/t/p/original/abc.jpg"
	rec, err := svc.Adopt(ctx, src, IngestRequest{Role: RolePoster, OwnerKind: "channel", OwnerID: "ch_2"})
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if rec.Origin != OriginRemote || rec.SourceURL != src {
		t.Errorf("origin/source = %q/%q", rec.Origin, rec.SourceURL)
	}
	// ⚠ Zero is the "never fetched" sentinel the fetch job selects on. If Adopt stamped it, the
	// image would never be fetched at all.
	if !rec.OriginFetchedAt.IsZero() {
		t.Error("Adopt stamped originFetchedAt; the fetch job would never pick the image up")
	}
	// Nothing was downloaded, so there must be no bytes to serve yet.
	if _, err := svc.Rendition(ctx, rec.Hash, FormatWebP, 342); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unfetched image served a rendition: %v", err)
	}

	// Re-adopting the same URL is a no-op rather than a duplicate.
	again, err := svc.Adopt(ctx, src, IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}
	if again.Hash != rec.Hash || len(fs.images) != 1 {
		t.Errorf("re-adopt created a second record: %d images", len(fs.images))
	}
}

func TestAdoptRequiresAbsoluteHTTPS(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	for _, bad := range []string{"http://insecure.test/a.jpg", "/relative.jpg", "ftp://x/a.jpg", "not a url"} {
		if _, err := svc.Adopt(ctx, bad, IngestRequest{Role: RolePoster}); err == nil {
			t.Errorf("Adopt(%q) was accepted", bad)
		}
	}
}

// ⚠ The URL-derived placeholder identity must live in a different preimage space from real content
// hashes. Both are sha256 hex of identical length, and a collision would make a fetch silently
// overwrite an unrelated image — because the content address IS the identity.
func TestAdoptIdentityCannotCollideWithAContentHash(t *testing.T) {
	const src = "https://image.tmdb.org/t/p/original/abc.jpg"
	if hashOfURL(src) == HashBytes([]byte(src)) {
		t.Fatal("the adopted-image id is the plain hash of the URL; a real image whose BYTES are " +
			"that URL string would collide with it")
	}
}

func TestURLForAndSrcSet(t *testing.T) {
	svc, _ := newTestService(t)
	hash := strings.Repeat("a", 64)

	got := svc.URLFor(hash, 342, FormatWebP)
	want := "https://loomarr.example.test/v1/images/" + hash + "/w342.webp"
	if got != want {
		t.Errorf("URLFor = %q, want %q", got, want)
	}

	set := svc.SrcSet(hash, RolePoster, FormatAVIF)
	for _, w := range RolePoster.Widths() {
		if !strings.Contains(set, ".avif "+strconv.Itoa(w)+"w") {
			t.Errorf("srcset missing the %dw descriptor: %s", w, set)
		}
	}
}

// An empty public base must produce a RELATIVE URL rather than one starting with a stray host —
// it is the safe fallback when the operator has not set server.public_url.
func TestURLForFallsBackToRelative(t *testing.T) {
	svc := New(Config{Dir: t.TempDir()}, newFakeStore(), func() time.Time { return fixedNow })
	got := svc.URLFor(strings.Repeat("b", 64), 92, FormatWebP)
	if !strings.HasPrefix(got, "/v1/images/") {
		t.Errorf("URLFor with no public base = %q, want a relative /v1/images/… URL", got)
	}
}

func TestProducesUsesTheCompatibilityLadder(t *testing.T) {
	svc := New(Config{Dir: t.TempDir()}, newFakeStore(), func() time.Time { return fixedNow })
	for _, f := range []Format{FormatAVIF, FormatWebP, FormatJPEG} {
		if !svc.Produces(f) {
			t.Errorf("Produces(%s) = false, want the fixed compatibility ladder", f)
		}
	}
}
