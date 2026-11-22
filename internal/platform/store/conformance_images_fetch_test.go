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

// The expiry purge pages through everything due: a purged image is requeued (its fetch sentinel
// reset), which takes it out of the expired set, so each read returns the next batch until none.
func testImageExpiryPagesThroughEverythingDue(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0)
	cutoff := now.Add(-180 * 24 * time.Hour)
	const due, batch = 11, 4
	for i := range due {
		img := numberedImage(i, now)
		img.OriginFetchedAt = cutoff.Add(-time.Duration(i+1) * time.Hour)
		if err := s.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
	}
	fresh := numberedImage(due, now)
	if err := s.PutImage(ctx, fresh); err != nil {
		t.Fatal(err)
	}

	var pages []int
	seen := map[string]bool{}
	for range due {
		page, err := s.ListImagesExpiredBefore(ctx, cutoff, batch)
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, len(page))
		if len(page) == 0 {
			break
		}
		for _, img := range page {
			if seen[img.Hash] {
				t.Fatalf("image %s came back after it was requeued", img.Hash)
			}
			seen[img.Hash] = true
			img.OriginFetchedAt = time.Time{}
			if err := s.PutImage(ctx, img); err != nil {
				t.Fatal(err)
			}
		}
	}
	if want := []int{4, 4, 3, 0}; fmt.Sprint(pages) != fmt.Sprint(want) {
		t.Fatalf("expired pages = %v, want %v", pages, want)
	}
	if seen[fresh.Hash] {
		t.Fatal("the purge took an image inside the ceiling")
	}
	queue, err := s.ListImagesAwaitingFetch(ctx, now, 2*due)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != due {
		t.Fatalf("%d purged images awaiting fetch, want %d", len(queue), due)
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
