package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// numberedImage is a remote image whose hash, source URL and placeholder all derive from n and
// never equal each other, for tests that need more images than imageAt's seeds can name.
func numberedImage(n int, at time.Time) clipstore.Image {
	img := imageAt("numbered", at)
	img.Hash = fmt.Sprintf("%064x", n+1)
	img.SourceURL = fmt.Sprintf("https://image.example.test/n/%d.jpg", n)
	img.Placeholder = fmt.Sprintf("ph-%d", n)
	return img
}

// The expiry purge pages through everything due by a (fetch time, hash) cursor: each image comes
// back once, a purged one is requeued (its fetch sentinel reset), and one whose purge failed is
// passed over rather than read again or allowed to hold back the rows behind it.
func testImageExpiryPagesThroughEverythingDue(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0)
	cutoff := now.Add(-180 * 24 * time.Hour)
	const due, batch = 11, 4
	// Two pairs share a fetch time, so the hash half of the cursor is exercised.
	for i := range due {
		img := numberedImage(i, now)
		img.OriginFetchedAt = cutoff.Add(-time.Duration(12-i/2) * time.Hour)
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	fresh := numberedImage(due, now)
	if err := s.PutImage(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	failing := numberedImage(0, now).Hash // the oldest: at the front of every read that started over

	var pages []int
	seen := map[string]bool{}
	var after clipstore.ExpiryCursor
	for range due {
		page, err := s.ListImagesExpiredBefore(ctx, cutoff, after, batch)
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, len(page))
		for _, img := range page {
			if seen[img.Hash] {
				t.Fatalf("image %s came back twice in one sweep", img.Hash)
			}
			seen[img.Hash] = true
			after = clipstore.ExpiryCursor{FetchedAt: img.OriginFetchedAt, Hash: img.Hash}
			if img.Hash == failing {
				continue // its purge failed: the row stays expired
			}
			img.OriginFetchedAt = time.Time{}
			if err := s.PutImage(ctx, img); err != nil {
				t.Fatal(err)
			}
		}
		if len(page) < batch {
			break
		}
	}
	if want := []int{4, 4, 3}; fmt.Sprint(pages) != fmt.Sprint(want) {
		t.Fatalf("expired pages = %v, want %v", pages, want)
	}
	if len(seen) != due || seen[fresh.Hash] {
		t.Fatalf("the sweep saw %d images (fresh one %v), want the %d due and not the fresh one", len(seen), seen[fresh.Hash], due)
	}
	if left, err := s.ListImagesExpiredBefore(ctx, cutoff, clipstore.ExpiryCursor{}, batch); err != nil || len(left) != 1 || left[0].Hash != failing {
		t.Fatalf("still expired = %d rows, %v; want just the one whose purge failed", len(left), err)
	}
	queue, err := s.ListImagesAwaitingFetch(ctx, now, 2*due)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != due-1 {
		t.Fatalf("%d purged images awaiting fetch, want %d", len(queue), due-1)
	}
}

// The fetch queue is served by priority, then in the order images joined it, never the order they
// were created: artwork adopted after a mass requeue goes ahead of all of it, a refresh goes ahead
// of bulk artwork, and within a class the earlier arrival goes first.
func testImageFetchQueueOrderIsWhenQueued(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0)
	pending := func(n int, created, queued time.Time, priority int) clipstore.Image {
		img := numberedImage(n, created)
		img.OriginFetchedAt = time.Time{}
		img.FetchQueuedAt = queued
		img.FetchPriority = priority
		return img
	}
	bulk := pending(0, now.Add(-3*time.Hour), now.Add(-3*time.Hour), 2)                // a still, waiting longest
	refreshEarly := pending(1, now.Add(-200*24*time.Hour), now.Add(-time.Hour), 1)     // requeued an hour ago
	refreshLate := pending(2, now.Add(-300*24*time.Hour), now.Add(-30*time.Minute), 1) // requeued later
	adoptedAfter := pending(3, now.Add(-time.Minute), now.Add(-time.Minute), 0)        // adopted after the purge
	unstamped := numberedImage(4, now.Add(-2*time.Hour))                               // no queue time: joins at creation
	unstamped.OriginFetchedAt = time.Time{}
	for _, img := range []clipstore.Image{bulk, refreshEarly, refreshLate, adoptedAfter, unstamped} {
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	queue, err := s.ListImagesAwaitingFetch(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, img := range queue {
		got = append(got, img.Hash)
	}
	want := []string{unstamped.Hash, adoptedAfter.Hash, refreshEarly.Hash, refreshLate.Hash, bulk.Hash}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("queue order = %v, want %v: adopted first, then refreshes, then bulk, each by arrival", got, want)
	}
	if row, _ := s.GetImage(ctx, bulk.Hash); row.FetchPriority != 2 {
		t.Fatalf("priority read back as %d, want 2", row.FetchPriority)
	}
	if row, _ := s.GetImage(ctx, unstamped.Hash); !row.FetchQueuedAt.Equal(unstamped.CreatedAt) {
		t.Fatalf("an unstamped pending row joined at %v, want its creation %v", row.FetchQueuedAt, unstamped.CreatedAt)
	}
}

// A never-fetched row reads back with the zero fetch time, the sentinel every caller checks with
// IsZero, not as fetched at the Unix epoch.
func testImagePendingReadsBackWithNoFetchTime(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0)
	pending := numberedImage(0, now)
	pending.OriginFetchedAt = time.Time{}
	if err := s.PutImage(ctx, pending); err != nil {
		t.Fatal(err)
	}
	reads := map[string]func() (clipstore.Image, error){
		"GetImage": func() (clipstore.Image, error) { return s.GetImage(ctx, pending.Hash) },
		"GetImageBySourceURL": func() (clipstore.Image, error) {
			return s.GetImageBySourceURL(ctx, pending.SourceURL)
		},
		"ImagesByHash": func() (clipstore.Image, error) {
			imgs, _, err := s.ImagesByHash(ctx, []string{pending.Hash})
			if err != nil || len(imgs) != 1 {
				return clipstore.Image{}, fmt.Errorf("ImagesByHash = %d rows, %w", len(imgs), err)
			}
			return imgs[0], nil
		},
	}
	for name, read := range reads {
		got, err := read()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !got.OriginFetchedAt.IsZero() {
			t.Errorf("%s read a pending row back as fetched at %v", name, got.OriginFetchedAt)
		}
	}
}

// A failed download holds its image off the fetch queue until its retry time, so failing images
// stop holding the front; each failure counts, and the download that lands clears the record.
func testImageFetchFailureBacksOff(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0)
	failing := numberedImage(0, now.Add(-time.Hour))
	failing.OriginFetchedAt = time.Time{}
	behind := numberedImage(1, now)
	behind.OriginFetchedAt = time.Time{}
	fetched := numberedImage(2, now)
	for _, img := range []clipstore.Image{failing, behind, fetched} {
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	queueAt := func(at time.Time) []string {
		t.Helper()
		rows, err := s.ListImagesAwaitingFetch(ctx, at, 1)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.Hash
		}
		return out
	}
	if got := queueAt(now); len(got) != 1 || got[0] != failing.Hash {
		t.Fatalf("front of the queue = %v, want the older image", got)
	}

	retry := now.Add(5 * time.Minute)
	if err := s.RecordImageFetchFailure(ctx, failing.Hash, now, retry); err != nil {
		t.Fatal(err)
	}
	if got := queueAt(now); len(got) != 1 || got[0] != behind.Hash {
		t.Fatalf("front of the queue after a failure = %v, want the image behind it", got)
	}
	if got := queueAt(retry); len(got) != 1 || got[0] != failing.Hash {
		t.Fatalf("front of the queue once the retry is due = %v, want the failed image again", got)
	}
	got, err := s.GetImage(ctx, failing.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.FetchFailures != 1 || !got.FetchFailedAt.Equal(now) || !got.FetchRetryAt.Equal(retry) {
		t.Fatalf("after one failure = %d, failed %v, retry %v", got.FetchFailures, got.FetchFailedAt, got.FetchRetryAt)
	}
	if err := s.RecordImageFetchFailure(ctx, failing.Hash, retry, retry.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetImage(ctx, failing.Hash); got.FetchFailures != 2 || !got.FetchRetryAt.Equal(retry.Add(10*time.Minute)) {
		t.Fatalf("after two failures = %d, retry %v", got.FetchFailures, got.FetchRetryAt)
	}

	// An ordinary write of the pending row keeps its failure record.
	got.UpdatedAt = retry.Add(time.Minute)
	if err := s.PutImage(ctx, got); err != nil {
		t.Fatal(err)
	}
	if kept, _ := s.GetImage(ctx, failing.Hash); kept.FetchFailures != 2 || !kept.FetchFailedAt.Equal(retry) ||
		!kept.FetchRetryAt.Equal(retry.Add(10*time.Minute)) {
		t.Fatalf("a pending write lost the failure record: %d, failed %v, retry %v", kept.FetchFailures, kept.FetchFailedAt, kept.FetchRetryAt)
	}

	got.OriginFetchedAt = retry.Add(10 * time.Minute)
	if err := s.PutImage(ctx, got); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetImage(ctx, failing.Hash); got.FetchFailures != 0 || !got.FetchFailedAt.IsZero() || !got.FetchRetryAt.IsZero() {
		t.Fatalf("a landed download keeps %d failures, failed %v, retry %v", got.FetchFailures, got.FetchFailedAt, got.FetchRetryAt)
	}

	if err := s.RecordImageFetchFailure(ctx, fetched.Hash, now, retry); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetImage(ctx, fetched.Hash); got.FetchFailures != 0 || !got.FetchRetryAt.IsZero() {
		t.Fatalf("a failure was recorded against an image with bytes: %d, retry %v", got.FetchFailures, got.FetchRetryAt)
	}

	// A deferral (the pass found the network down) holds the image without counting a failure,
	// and leaves an image with bytes alone.
	if err := s.DeferImageFetch(ctx, behind.Hash, retry); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetImage(ctx, behind.Hash); got.FetchFailures != 0 || !got.FetchRetryAt.Equal(retry) {
		t.Fatalf("deferred = %d failures, retry %v; want none counted and held until %v", got.FetchFailures, got.FetchRetryAt, retry)
	}
	if q := queueAt(now); len(q) != 0 {
		t.Fatalf("queue while deferred = %v, want empty", q)
	}
	if err := s.DeferImageFetch(ctx, fetched.Hash, retry); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetImage(ctx, fetched.Hash); !got.FetchRetryAt.IsZero() {
		t.Fatalf("an image with bytes was deferred to %v", got.FetchRetryAt)
	}
}

// One read returns many images and their renditions, across more hashes than one statement
// takes; a hash with no row is absent, and an image with no renditions still comes back.
func testImagesByHash(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	at := time.Unix(1_700_000_000, 0)
	const stored = 520
	var hashes []string
	for i := range stored {
		img := numberedImage(i, at)
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, img.Hash)
	}
	withRenditions := map[string]bool{hashes[3]: true, hashes[stored-1]: true}
	for h := range withRenditions {
		for _, d := range []clipstore.ImageDerivative{
			{ImageHash: h, Recipe: "loomarr-rendition-v1", Format: "avif", Width: 342, Path: "drv/" + h[:8] + "/a342.avif", CreatedAt: at},
			{ImageHash: h, Recipe: "loomarr-rendition-v1", Format: "webp", Width: 342, Path: "drv/" + h[:8] + "/w342.webp", CreatedAt: at},
		} {
			if err := s.PutImageDerivative(ctx, d); err != nil {
				t.Fatal(err)
			}
		}
	}
	missing := numberedImage(stored+5, at).Hash
	asked := append(append([]string{missing}, hashes...), hashes[0])

	imgs, derivs, err := s.ImagesByHash(ctx, asked)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]clipstore.Image{}
	for _, img := range imgs {
		if _, dup := got[img.Hash]; dup {
			t.Fatalf("image %s came back twice", img.Hash)
		}
		got[img.Hash] = img
	}
	if len(got) != stored {
		t.Fatalf("ImagesByHash returned %d images, want %d", len(got), stored)
	}
	if _, ok := got[missing]; ok {
		t.Fatal("a hash with no row came back")
	}
	if img := got[hashes[10]]; img.SourceURL != numberedImage(10, at).SourceURL {
		t.Fatalf("image did not round trip: %+v", img)
	}
	perImage := map[string][]string{}
	for _, d := range derivs {
		perImage[d.ImageHash] = append(perImage[d.ImageHash], d.Format)
	}
	if len(perImage) != len(withRenditions) {
		t.Fatalf("renditions came back for %d images, want %d", len(perImage), len(withRenditions))
	}
	for h := range withRenditions {
		if fmt.Sprint(perImage[h]) != "[avif webp]" {
			t.Errorf("renditions of %s = %v, want [avif webp]", h, perImage[h])
		}
	}

	if imgs, derivs, err := s.ImagesByHash(ctx, nil); err != nil || len(imgs) != 0 || len(derivs) != 0 {
		t.Fatalf("ImagesByHash(nil) = %d, %d, %v; want nothing", len(imgs), len(derivs), err)
	}
}
