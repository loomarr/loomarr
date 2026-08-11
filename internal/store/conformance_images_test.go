package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Image service conformance (§22, V52). Part of the ONE suite both backends run — never forked per
// dialect (CLAUDE.md). The dialect differences here are real (INTEGER vs BOOLEAN for `animated`,
// TEXT vs JSONB for `meta`, INTEGER vs BIGINT for the epochs), which is exactly why these
// assertions must be shared: a Postgres-only surprise should fail a test, not a deployment.

// imageAt builds a distinct image for a given seed.
//
// ⚠ **Hash and every other string are DERIVED from the seed and never equal each other.** §10
// shipped two live bugs — a catalog-wide delete and a tagger that never tagged — because fixtures
// set a clip's hash and path to the same string, making a hash-keyed call and a path-keyed one
// indistinguishable to every test that passed. The same trap is available here (hash, source URL,
// owner id) and is closed by construction.
func imageAt(seed string, at time.Time) Image {
	// A 64-hex hash built from the seed, so it is unique per seed and can never coincide with a
	// URL, an owner id, or a path.
	hash := ""
	for len(hash) < 64 {
		for _, c := range seed {
			hash += string("0123456789abcdef"[int(c)%16])
			if len(hash) >= 64 {
				break
			}
		}
	}
	return Image{
		Hash:            hash,
		Origin:          "remote",
		SourceURL:       "https://image.example.test/" + seed + ".jpg",
		Visibility:      "member",
		Role:            "poster",
		MIME:            "image/jpeg",
		Width:           1000,
		Height:          1500,
		Bytes:           123456,
		Animated:        false,
		Placeholder:     "ph-" + seed,
		DominantHex:     "#336699",
		Meta:            `{"attribution":"` + seed + `"}`,
		OriginFetchedAt: at,
		CreatedAt:       at,
		UpdatedAt:       at,
		LastUsedAt:      at,
	}
}

func testImageRoundTrip(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	want := imageAt("alpha", at)
	if err := s.PutImage(ctx, want); err != nil {
		t.Fatalf("PutImage: %v", err)
	}

	got, err := s.GetImage(ctx, want.Hash)
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}

	// Field-by-field rather than a struct compare, so a failure names the column. The dialects
	// store several of these differently and a bare "structs differ" would not say which.
	if got.Origin != want.Origin || got.SourceURL != want.SourceURL {
		t.Errorf("origin/source = %q/%q, want %q/%q", got.Origin, got.SourceURL, want.Origin, want.SourceURL)
	}
	if got.Visibility != want.Visibility || got.Role != want.Role {
		t.Errorf("visibility/role = %q/%q", got.Visibility, got.Role)
	}
	if got.Width != 1000 || got.Height != 1500 || got.Bytes != 123456 {
		t.Errorf("dimensions/bytes = %dx%d %d", got.Width, got.Height, got.Bytes)
	}
	if got.Placeholder != want.Placeholder || got.DominantHex != want.DominantHex {
		t.Errorf("placeholder/dominant = %q/%q", got.Placeholder, got.DominantHex)
	}
	// ⚠ meta is TEXT on sqlite and JSONB on Postgres. JSONB does not preserve byte-for-byte input
	// (it normalises whitespace and key order), so assert it parses back to the same VALUE rather
	// than the same string — a string compare here would pass on sqlite and fail on Postgres,
	// which is the exact class this shared suite exists to catch.
	if !jsonHasAttribution(got.Meta, "alpha") {
		t.Errorf("meta = %q, want an attribution of alpha", got.Meta)
	}
	if !got.OriginFetchedAt.Equal(at) {
		t.Errorf("originFetchedAt = %v, want %v", got.OriginFetchedAt, at)
	}
	if got.Animated {
		t.Error("animated came back true for a still")
	}
}

func testImageMissingIsNotFound(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	_, err := s.GetImage(context.Background(), imageAt("nope", time.Now()).Hash)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetImage for an absent hash = %v, want ErrNotFound", err)
	}
}

// ⚠ Re-ingesting identical bytes yields the identical hash — that is what content addressing MEANS
// — so an upsert is a re-import of something already held. It must not reset `created_at`, or the
// GC's expiry sweep reads an age the row invented. Single-writer columns omitted from DO UPDATE is
// the rule V44's clip metadata already follows.
func testImageUpsertPreservesCreatedAt(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)

	first := time.Unix(1_700_000_000, 0)
	img := imageAt("beta", first)
	if err := s.PutImage(ctx, img); err != nil {
		t.Fatal(err)
	}

	later := first.Add(72 * time.Hour)
	img.CreatedAt = later
	img.UpdatedAt = later
	img.Role = "backdrop" // a genuine change, which MUST land
	if err := s.PutImage(ctx, img); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetImage(ctx, img.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(first) {
		t.Errorf("createdAt = %v after re-put, want the original %v — a re-import must not "+
			"make the row lie about its age", got.CreatedAt, first)
	}
	if !got.UpdatedAt.Equal(later) {
		t.Errorf("updatedAt = %v, want %v — the mutable half must still move", got.UpdatedAt, later)
	}
	if got.Role != "backdrop" {
		t.Errorf("role = %q, want the updated backdrop", got.Role)
	}
}

func testImageRefsAndOwnerLookup(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	a, b := imageAt("gamma", at), imageAt("delta", at)
	for _, img := range []Image{a, b} {
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}

	// ⚠ owner id is deliberately unlike the hash: they are different namespaces and a test that
	// let them coincide would not notice a query keyed on the wrong one.
	if err := s.PutImageRef(ctx, ImageRef{ImageHash: a.Hash, OwnerKind: "channel", OwnerID: "ch_1", Role: "icon"}); err != nil {
		t.Fatal(err)
	}
	// Re-adding the same ref must be a no-op, not a duplicate-key error: reconcile paths are
	// re-entrant by design and will re-assert a ref they already made.
	if err := s.PutImageRef(ctx, ImageRef{ImageHash: a.Hash, OwnerKind: "channel", OwnerID: "ch_1", Role: "icon"}); err != nil {
		t.Fatalf("re-adding an identical ref errored: %v", err)
	}
	if err := s.PutImageRef(ctx, ImageRef{ImageHash: b.Hash, OwnerKind: "channel", OwnerID: "ch_2", Role: "icon"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ImagesForOwner(ctx, "channel", "ch_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Hash != a.Hash {
		t.Fatalf("ImagesForOwner(ch_1) = %d rows, want just %s", len(got), a.Hash)
	}

	if err := s.DeleteImageRefs(ctx, "channel", "ch_1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ImagesForOwner(ctx, "channel", "ch_1"); len(got) != 0 {
		t.Errorf("refs survived deletion: %d", len(got))
	}
	// Deleting one owner's refs must not touch another's.
	if got, _ := s.ImagesForOwner(ctx, "channel", "ch_2"); len(got) != 1 {
		t.Errorf("deleting ch_1's refs also removed ch_2's")
	}
}

// The GC's work list. An image nothing points at is collectable; one with a ref is not.
func testImageOrphanDetection(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	kept, orphan := imageAt("epsilon", at), imageAt("zeta", at)
	for _, img := range []Image{kept, orphan} {
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PutImageRef(ctx, ImageRef{ImageHash: kept.Hash, OwnerKind: "channel", OwnerID: "ch_1", Role: "icon"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListOrphanImages(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Hash != orphan.Hash {
		t.Fatalf("ListOrphanImages = %d rows, want just the unreferenced %s", len(got), orphan.Hash)
	}
}

func testImageDerivatives(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	img := imageAt("eta", at)
	if err := s.PutImage(ctx, img); err != nil {
		t.Fatal(err)
	}

	for _, d := range []ImageDerivative{
		{ImageHash: img.Hash, Format: "webp", Width: 342, Bytes: 9000, Path: "drv/x/y/a_w342.webp", CreatedAt: at},
		{ImageHash: img.Hash, Format: "webp", Width: 500, Bytes: 14000, Path: "drv/x/y/a_w500.webp", CreatedAt: at},
	} {
		if err := s.PutImageDerivative(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	list, err := s.ListImageDerivatives(ctx, img.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("ListImageDerivatives = %d, want 2", len(list))
	}

	// Re-encoding one rung replaces it rather than duplicating — the key is (hash, format, width).
	if err := s.PutImageDerivative(ctx, ImageDerivative{
		ImageHash: img.Hash, Format: "webp", Width: 500, Bytes: 15500,
		Path: "drv/x/y/a_w500.webp", CreatedAt: at.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListImageDerivatives(ctx, img.Hash)
	if len(list) != 2 {
		t.Fatalf("re-putting a rung created a duplicate: %d rows", len(list))
	}

	if err := s.DeleteImageDerivatives(ctx, img.Hash); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.ListImageDerivatives(ctx, img.Hash); len(list) != 0 {
		t.Errorf("derivatives survived deletion: %d", len(list))
	}
}

// The AVIF job's work list: "has some rendition but not this format".
//
// ⚠ The has-some-derivative half is load-bearing. Without it the job would pick up images whose
// bytes have not been processed at all, encoding AVIF for something with no WebP — inverting the
// very priority the lazy/job split exists to create.
func testImagesMissingFormat(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	webpOnly := imageAt("theta", at)
	bothFormats := imageAt("iota", at)
	untouched := imageAt("kappa", at) // no derivatives at all
	for _, img := range []Image{webpOnly, bothFormats, untouched} {
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.PutImageDerivative(ctx, ImageDerivative{ImageHash: webpOnly.Hash, Format: "webp", Width: 500, Path: "a", CreatedAt: at}))
	must(s.PutImageDerivative(ctx, ImageDerivative{ImageHash: bothFormats.Hash, Format: "webp", Width: 500, Path: "b", CreatedAt: at}))
	must(s.PutImageDerivative(ctx, ImageDerivative{ImageHash: bothFormats.Hash, Format: "avif", Width: 500, Path: "c", CreatedAt: at}))

	got, err := s.ListImagesMissingFormat(ctx, "avif", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Hash != webpOnly.Hash {
		hashes := make([]string, len(got))
		for i, g := range got {
			hashes[i] = g.Hash
		}
		t.Fatalf("ListImagesMissingFormat(avif) = %v, want just the webp-only image %s "+
			"(an image with NO derivatives must not be picked up)", hashes, webpOnly.Hash)
	}
}

// The fetch queue and the TMDB expiry sweep read the same column from opposite ends, and must not
// see each other's rows: a never-fetched image is infinitely stale by date arithmetic, so the
// expiry query has to exclude it explicitly or it would purge the fetch job's entire backlog.
func testImageFetchQueueAndExpirySweepAreDisjoint(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0)

	pending := imageAt("lambda", now)
	pending.OriginFetchedAt = time.Unix(0, 0) // never fetched

	fresh := imageAt("mu", now)
	fresh.OriginFetchedAt = now

	stale := imageAt("nu", now)
	stale.OriginFetchedAt = now.Add(-200 * 24 * time.Hour) // well past six months

	for _, img := range []Image{pending, fresh, stale} {
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}

	queue, err := s.ListImagesAwaitingFetch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Hash != pending.Hash {
		t.Fatalf("ListImagesAwaitingFetch = %d rows, want just the never-fetched one", len(queue))
	}

	cutoff := now.Add(-180 * 24 * time.Hour)
	expired, err := s.ListImagesExpiredBefore(ctx, cutoff, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].Hash != stale.Hash {
		got := make([]string, len(expired))
		for i, e := range expired {
			got[i] = e.Hash
		}
		t.Fatalf("ListImagesExpiredBefore = %v, want just the stale one — a never-fetched image "+
			"must NOT look expired, or the sweep purges the fetch backlog", got)
	}
}

// Only upload/generated images are unrecoverable; remote and extracted can be got back. The GC
// warns about the first group and silently repairs the second, so the split has to be right.
func testImageUnrecoverableSelection(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	origins := map[string]string{"xi": "upload", "omicron": "generated", "pi": "remote", "rho": "extracted"}
	wantHashes := map[string]bool{}
	for seed, origin := range origins {
		img := imageAt(seed, at)
		img.Origin = origin
		if origin == "upload" || origin == "generated" {
			wantHashes[img.Hash] = true
		}
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListUnrecoverableImages(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(wantHashes) {
		t.Fatalf("ListUnrecoverableImages = %d rows, want %d (upload + generated only)", len(got), len(wantHashes))
	}
	for _, g := range got {
		if !wantHashes[g.Hash] {
			t.Errorf("a recoverable image (%s) was listed as unrecoverable", g.Origin)
		}
	}
}

// Deleting an image must take its refs and derivatives with it. Expressed as a schema cascade
// rather than an application transaction the caller could get half-right.
func testImageDeleteCascades(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	img := imageAt("sigma", at)
	if err := s.PutImage(ctx, img); err != nil {
		t.Fatal(err)
	}
	if err := s.PutImageRef(ctx, ImageRef{ImageHash: img.Hash, OwnerKind: "channel", OwnerID: "ch_9", Role: "icon"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutImageDerivative(ctx, ImageDerivative{
		ImageHash: img.Hash, Format: "webp", Width: 500, Path: "p", CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteImage(ctx, img.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetImage(ctx, img.Hash); !errors.Is(err, ErrNotFound) {
		t.Errorf("image survived deletion")
	}
	if d, _ := s.ListImageDerivatives(ctx, img.Hash); len(d) != 0 {
		t.Errorf("%d derivative rows survived the image — they now reference nothing", len(d))
	}
	if o, _ := s.ImagesForOwner(ctx, "channel", "ch_9"); len(o) != 0 {
		t.Errorf("refs survived the image they pointed at")
	}
}

func testImageTouch(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	img := imageAt("tau", at)
	if err := s.PutImage(ctx, img); err != nil {
		t.Fatal(err)
	}
	later := at.Add(48 * time.Hour)
	if err := s.TouchImage(ctx, img.Hash, later); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetImage(ctx, img.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastUsedAt.Equal(later) {
		t.Errorf("lastUsedAt = %v, want %v — the cache-budget eviction reads this", got.LastUsedAt, later)
	}
}

// The rehydrate job's work list — every image of one origin, whatever its fetch state.
//
// ⚠ This set is deliberately NOT expressible by the two lists that already exist, and the test
// says so by construction: it seeds a remote image that has been fetched and one that has not,
// and requires both back. `ListImagesAwaitingFetch` is scoped to the never-fetched sentinel and
// the expiry sweep to a cutoff, but a file can go missing under a row in either state — which is
// the only thing rehydrate is looking for.
func testImagesByOrigin(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	fetched := imageAt("rho", at)
	fetched.Origin, fetched.OriginFetchedAt = "remote", at
	pending := imageAt("sigma", at)
	pending.Origin, pending.OriginFetchedAt = "remote", time.Time{}
	uploaded := imageAt("tau", at)
	uploaded.Origin, uploaded.OriginFetchedAt = "upload", at

	for _, img := range []Image{fetched, pending, uploaded} {
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListImagesByOrigin(ctx, "remote", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("ListImagesByOrigin(remote) = %d rows, want 2 (the fetched one and the pending one)", len(got))
	}
	for _, img := range got {
		if img.Hash == uploaded.Hash {
			t.Error("ListImagesByOrigin(remote) returned an upload — the origin filter is not applied")
		}
	}

	if got, err = s.ListImagesByOrigin(ctx, "upload", 50); err != nil {
		t.Fatal(err)
	} else if len(got) != 1 || got[0].Hash != uploaded.Hash {
		t.Errorf("ListImagesByOrigin(upload) = %d rows, want just the upload", len(got))
	}
}

// The fetch job's re-key: an adopted row keyed on its source URL becomes a row keyed on the
// content hash, and its refs must travel with it.
//
// ⚠ The collision half is the part worth testing, and it is why this is not an UPDATE. Two
// distinct source URLs can hold identical bytes — the same poster reached by two paths — so both
// placeholder rows re-key onto ONE content hash. A second repoint carrying the same
// (owner_kind, owner_id, role) must be a no-op rather than a primary-key violation.
// Finding a remote image by the URL it came from — the lookup that stops a re-adopt from
// re-downloading bytes already on disk (V52 phase 7).
//
// ⚠ **The `origin_fetched_at > 0` half is the point, not a tidy-up.** Adopt keys a pending row on a
// hash of the source URL; the fetch re-keys it onto the content hash and deletes the placeholder,
// and for the width of that operation BOTH rows carry the same source URL — deliberately, so no
// ref ever points at a hash with no row. A lookup that returned the placeholder would hand back a
// hash that is about to stop resolving, which is worse than returning nothing.
func testImageFetchedBySourceURL(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	fetched := imageAt("fetched", at)
	if err := s.PutImage(ctx, fetched); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetFetchedImageBySourceURL(ctx, fetched.SourceURL)
	if err != nil {
		t.Fatalf("GetFetchedImageBySourceURL: %v", err)
	}
	if got.Hash != fetched.Hash {
		t.Errorf("hash = %q, want %q", got.Hash, fetched.Hash)
	}

	// A row adopted but never fetched must NOT be returned: it has no bytes to reuse, and its hash
	// is the placeholder the re-key deletes.
	pending := imageAt("pending", at)
	pending.OriginFetchedAt = time.Time{}
	if err := s.PutImage(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFetchedImageBySourceURL(ctx, pending.SourceURL); !errors.Is(err, ErrNotFound) {
		t.Errorf("a never-fetched row was returned (err = %v), want ErrNotFound", err)
	}

	// Both rows sharing one source URL — the state that exists during a re-key. The fetched one
	// wins; without the predicate this is a coin toss decided by row order.
	shadow := imageAt("shadow", at)
	shadow.SourceURL = fetched.SourceURL
	shadow.OriginFetchedAt = time.Time{}
	if err := s.PutImage(ctx, shadow); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetFetchedImageBySourceURL(ctx, fetched.SourceURL)
	if err != nil {
		t.Fatalf("GetFetchedImageBySourceURL with a placeholder alongside: %v", err)
	}
	if got.Hash != fetched.Hash {
		t.Errorf("hash = %q, want the FETCHED row %q — the placeholder shadowed it", got.Hash, fetched.Hash)
	}

	// An empty URL is every non-remote row's value, so it must never match one.
	if _, err := s.GetFetchedImageBySourceURL(ctx, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty source URL returned %v, want ErrNotFound", err)
	}
	if _, err := s.GetFetchedImageBySourceURL(ctx, "https://nothing.example.test/x.jpg"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown source URL returned %v, want ErrNotFound", err)
	}
}

func testImageRefsRepoint(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	placeholderA := imageAt("upsilon", at)
	placeholderB := imageAt("phi", at)
	content := imageAt("chi", at)
	for _, img := range []Image{placeholderA, placeholderB, content} {
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}

	// Both placeholders decorate the SAME owner in the same role — the collision case.
	for _, h := range []string{placeholderA.Hash, placeholderB.Hash} {
		if err := s.PutImageRef(ctx, ImageRef{
			ImageHash: h, OwnerKind: "channel", OwnerID: "ch-repoint", Role: "poster",
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.RepointImageRefs(ctx, placeholderA.Hash, content.Hash); err != nil {
		t.Fatal(err)
	}
	if err := s.RepointImageRefs(ctx, placeholderB.Hash, content.Hash); err != nil {
		t.Fatalf("the second repoint onto the same content hash failed — an UPDATE would collide here: %v", err)
	}

	owned, err := s.ImagesForOwner(ctx, "channel", "ch-repoint")
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 1 || owned[0].Hash != content.Hash {
		t.Fatalf("after repointing, the owner has %d images, want just the content-hashed one", len(owned))
	}

	// The placeholders are now orphans, which is what lets the GC delete them.
	orphans, err := s.ListOrphanImages(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	var sawA, sawB bool
	for _, o := range orphans {
		sawA = sawA || o.Hash == placeholderA.Hash
		sawB = sawB || o.Hash == placeholderB.Hash
	}
	if !sawA || !sawB {
		t.Error("a repointed-away placeholder is still referenced — the source refs were not cleared")
	}
}

// The GC's two eviction inputs: how much disk the derivatives occupy, and which to drop first.
//
// ⚠ The order comes from the parent IMAGE's last_used_at, not the derivative's created_at, and
// this test pins that by making the two disagree: the coldest image's derivative is the NEWEST
// file. A created_at ordering would return them in exactly the wrong order and still look
// plausible, which is why the fixture is built to distinguish them rather than to pass.
func testImageDerivativeBudgetAndEvictionOrder(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)

	hot := imageAt("psi", at)
	hot.LastUsedAt = at.Add(72 * time.Hour)
	cold := imageAt("omega", at)
	cold.LastUsedAt = at
	for _, img := range []Image{hot, cold} {
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}

	if total, err := s.TotalImageDerivativeBytes(ctx); err != nil {
		t.Fatalf("total over an empty table must be 0, not an error: %v", err)
	} else if total != 0 {
		t.Errorf("TotalImageDerivativeBytes on a fresh store = %d, want 0", total)
	}

	// The cold image's derivative is the NEWER file — so created_at and last_used_at disagree.
	if err := s.PutImageDerivative(ctx, ImageDerivative{
		ImageHash: hot.Hash, Format: "webp", Width: 500, Bytes: 12000,
		Path: "drv/h/o/hot_w500.webp", CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutImageDerivative(ctx, ImageDerivative{
		ImageHash: cold.Hash, Format: "webp", Width: 500, Bytes: 8000,
		Path: "drv/c/o/cold_w500.webp", CreatedAt: at.Add(96 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	total, err := s.TotalImageDerivativeBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if total != 20000 {
		t.Errorf("TotalImageDerivativeBytes = %d, want 20000", total)
	}

	coldest, err := s.ListColdestDerivatives(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(coldest) != 2 {
		t.Fatalf("ListColdestDerivatives = %d rows, want 2", len(coldest))
	}
	if coldest[0].ImageHash != cold.Hash {
		t.Errorf("eviction order starts with the hot image — the order must come from the parent "+
			"image's last_used_at, not the derivative's created_at (got %s, want %s)",
			coldest[0].ImageHash, cold.Hash)
	}

	// Evicting one rung leaves the other rung and both parent rows alone.
	if err := s.DeleteImageDerivative(ctx, cold.Hash, "webp", 500); err != nil {
		t.Fatal(err)
	}
	if total, err = s.TotalImageDerivativeBytes(ctx); err != nil {
		t.Fatal(err)
	} else if total != 12000 {
		t.Errorf("after evicting one rung the total is %d, want 12000", total)
	}
	if _, err := s.GetImage(ctx, cold.Hash); err != nil {
		t.Errorf("evicting a derivative deleted its image: %v", err)
	}
}

// jsonHasAttribution avoids depending on byte-identical JSON across dialects: Postgres JSONB
// normalises whitespace and key order, sqlite TEXT preserves whatever was written. Asserting on
// the VALUE rather than the encoding is what keeps this one assertion honest on both.
func jsonHasAttribution(meta, want string) bool {
	return meta != "" && strings.Contains(meta, `"`+want+`"`)
}
