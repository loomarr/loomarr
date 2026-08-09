package images

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// recordingNotifier captures what the GC would tell an operator.
type recordingNotifier struct{ msgs []string }

func (n *recordingNotifier) Warn(_ context.Context, _, _, text string) { n.msgs = append(n.msgs, text) }

func newTestGC(t *testing.T, ttl time.Duration, budgetMB int) (*GC, *Service, *fakeStore, *recordingNotifier) {
	t.Helper()
	svc, fs := newTestService(t)
	n := &recordingNotifier{}
	return NewGC(svc, fs, func() time.Duration { return ttl }, func() int { return budgetMB }, n, nil), svc, fs, n
}

// seedFetchedRemote puts a fully-fetched remote image in the store, with its bytes on disk.
func seedFetchedRemote(t *testing.T, svc *Service, fs *fakeStore, seedW, seedH int, fetchedAt time.Time) Image {
	t.Helper()
	ctx := context.Background()
	rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(seedW, seedH))), IngestRequest{
		Role: RolePoster, Origin: OriginUpload,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Re-shape it into the remote row the TTL sweep looks at. Done through the store rather than
	// through Adopt because Adopt deliberately produces a row with no bytes.
	rec.Origin = OriginRemote
	rec.SourceURL = "https://image.tmdb.org/t/p/original/seed.jpg"
	rec.OriginFetchedAt = fetchedAt
	if err := fs.PutImage(ctx, rec); err != nil {
		t.Fatal(err)
	}
	// ⚠ Referenced, because an UNreferenced expired image is the orphan sweep's business, not the
	// TTL sweep's — and the first draft of this fixture omitted the ref, so the orphan sweep
	// deleted the row the TTL sweep had just requeued and the assertion below failed for the wrong
	// reason. That collision is why Run collects orphans first; the fixture now isolates the sweep
	// it is actually about.
	if err := fs.PutRef(ctx, Ref{
		ImageHash: rec.Hash, OwnerKind: "channel", OwnerID: "ch-ttl", Role: RolePoster,
	}); err != nil {
		t.Fatal(err)
	}
	return rec
}

// The TMDB six-month ceiling. ⚠ A compliance term, not a cache heuristic: the bytes must be GONE
// at the ceiling, and the row goes back on the fetch queue rather than the delete being made
// conditional on a successful re-download.
func TestGCPurgesExpiredBytesAndRequeuesTheRow(t *testing.T) {
	const ttl = 30 * 24 * time.Hour
	gc, svc, fs, _ := newTestGC(t, ttl, 0)
	ctx := context.Background()

	stale := seedFetchedRemote(t, svc, fs, 400, 600, fixedNow.Add(-ttl-time.Hour))
	if _, err := svc.Rendition(ctx, stale.Hash, FormatWebP, 342); err != nil {
		t.Fatal(err)
	}
	origPath, err := svc.blob.OriginalPath(stale.Hash, extForMIME(stale.MIME))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.blob.Stat(origPath); !ok {
		t.Fatal("the fixture has no original on disk, so this cannot observe a purge")
	}

	res, err := gc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Expired != 1 {
		t.Fatalf("Run = %+v, want the expired image swept", res)
	}

	if _, ok := svc.blob.Stat(origPath); ok {
		t.Error("expired TMDB bytes are still on disk — the ceiling is a licence term, and keeping " +
			"them until a re-download succeeds puts compliance inside an error branch")
	}
	if ds, _ := fs.ListDerivatives(ctx, stale.Hash); len(ds) != 0 {
		t.Errorf("%d derivative rows survived the purge", len(ds))
	}

	got, err := fs.GetImage(ctx, stale.Hash)
	if err != nil {
		t.Fatalf("the row was deleted rather than requeued: %v", err)
	}
	if !got.OriginFetchedAt.IsZero() {
		t.Error("originFetchedAt was not cleared — the row is off the fetch queue with no bytes, " +
			"so the image would never come back")
	}
	// And it is genuinely back on the queue, not merely flagged.
	queued, err := fs.ListAwaitingFetch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].Hash != stale.Hash {
		t.Errorf("the requeued row is not on the fetch job's work list: %+v", queued)
	}
}

// A remote image inside its TTL must be left completely alone.
func TestGCLeavesFreshRemoteImagesAlone(t *testing.T) {
	const ttl = 30 * 24 * time.Hour
	gc, svc, fs, _ := newTestGC(t, ttl, 0)
	ctx := context.Background()

	fresh := seedFetchedRemote(t, svc, fs, 300, 450, fixedNow.Add(-time.Hour))
	if err := fs.PutRef(ctx, Ref{ImageHash: fresh.Hash, OwnerKind: "channel", OwnerID: "c1", Role: RolePoster}); err != nil {
		t.Fatal(err)
	}

	res, err := gc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Expired != 0 || res.OrphansDeleted != 0 {
		t.Fatalf("Run = %+v, want a fresh referenced image untouched", res)
	}
	if _, err := fs.GetImage(ctx, fresh.Hash); err != nil {
		t.Errorf("a fresh image was collected: %v", err)
	}
}

// An image nothing references is deleted, files first.
func TestGCDeletesOrphansAndTheirBytes(t *testing.T) {
	gc, svc, fs, _ := newTestGC(t, 0, 0)
	ctx := context.Background()

	orphan, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(200, 300))), IngestRequest{
		Role: RolePoster, Origin: OriginUpload, // no owner ⇒ no ref
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rendition(ctx, orphan.Hash, FormatWebP, 154); err != nil {
		t.Fatal(err)
	}
	origPath, _ := svc.blob.OriginalPath(orphan.Hash, extForMIME(orphan.MIME))
	drvPath, _ := svc.blob.DerivativePath(orphan.Hash, 154, FormatWebP)

	res, err := gc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.OrphansDeleted != 1 {
		t.Fatalf("Run = %+v, want the unreferenced image collected", res)
	}
	if _, ok := svc.blob.Stat(origPath); ok {
		t.Error("the orphan's original is still on disk — the row is gone, so nothing can ever find it again")
	}
	if _, ok := svc.blob.Stat(drvPath); ok {
		t.Error("the orphan's derivative is still on disk")
	}
	if _, err := fs.GetImage(ctx, orphan.Hash); err == nil {
		t.Error("the orphan row survived")
	}
}

// Eviction: coldest first, and it stops as soon as it is under budget.
func TestGCEvictsTheColdestDerivativesUntilUnderBudget(t *testing.T) {
	gc, svc, fs, _ := newTestGC(t, 0, 1) // 1 MB budget
	ctx := context.Background()

	// Three images, referenced (so the orphan sweep leaves them), each with one 600KB derivative.
	// 1.8MB total against a 1MB budget: two must go, and they must be the two coldest.
	type seeded struct {
		hash string
		path string
	}
	var rows []seeded
	for i, lastUsed := range []time.Time{
		fixedNow.Add(-72 * time.Hour), // coldest
		fixedNow.Add(-48 * time.Hour),
		fixedNow, // hottest
	} {
		rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(120+i*40, 180+i*40))), IngestRequest{
			Role: RolePoster, Origin: OriginUpload, OwnerKind: "channel", OwnerID: "c" + string(rune('1'+i)),
		})
		if err != nil {
			t.Fatal(err)
		}
		rec.LastUsedAt = lastUsed
		if err := fs.PutImage(ctx, rec); err != nil {
			t.Fatal(err)
		}
		path, _ := svc.blob.DerivativePath(rec.Hash, 154, FormatWebP)
		if err := svc.blob.Write(path, make([]byte, 600<<10)); err != nil {
			t.Fatal(err)
		}
		if err := fs.PutDerivative(ctx, Derivative{
			ImageHash: rec.Hash, Format: FormatWebP, Width: 154,
			Bytes: 600 << 10, Path: path, CreatedAt: fixedNow,
		}); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, seeded{hash: rec.Hash, path: path})
	}

	res, err := gc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Evicted != 2 {
		t.Fatalf("Run = %+v, want exactly two evictions (1.8MB down to under 1MB)", res)
	}
	if _, ok := svc.blob.Stat(rows[2].path); !ok {
		t.Error("the HOTTEST image's derivative was evicted — the order is inverted")
	}
	for _, cold := range rows[:2] {
		if _, ok := svc.blob.Stat(cold.path); ok {
			t.Errorf("a cold derivative survived eviction: %s", cold.path)
		}
	}

	// ⚠ Eviction must never take the image row with it. A derivative is regenerable; the row is
	// the only record of where the bytes came from.
	for _, r := range rows {
		if _, err := fs.GetImage(ctx, r.hash); err != nil {
			t.Errorf("evicting a derivative deleted its image row: %v", err)
		}
	}

	total, _ := fs.TotalDerivativeBytes(ctx)
	if total > 1<<20 {
		t.Errorf("still %d bytes against a 1MB budget", total)
	}
}

// Under budget, eviction must not run at all — the budget is a backstop, not a routine trim.
func TestGCDoesNotEvictWhenUnderBudget(t *testing.T) {
	gc, svc, _, _ := newTestGC(t, 0, 64)
	ctx := context.Background()

	rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(200, 300))), IngestRequest{
		Role: RolePoster, Origin: OriginUpload, OwnerKind: "channel", OwnerID: "c1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rendition(ctx, rec.Hash, FormatWebP, 154); err != nil {
		t.Fatal(err)
	}

	res, err := gc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Evicted != 0 {
		t.Errorf("Run = %+v, want nothing evicted under a 64MB budget", res)
	}
}

// The unrecoverable warning: counted, not repaired, and said ONCE.
func TestGCWarnsOnceAboutUploadsWhoseFilesAreGone(t *testing.T) {
	gc, svc, fs, notifier := newTestGC(t, 0, 0)
	ctx := context.Background()

	for i := range 3 {
		rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(100+i*10, 150+i*10))), IngestRequest{
			Role: RoleIcon, Origin: OriginUpload, OwnerKind: "channel", OwnerID: "c" + string(rune('1'+i)),
		})
		if err != nil {
			t.Fatal(err)
		}
		// Lose the bytes the way a restore onto an empty volume does.
		path, _ := svc.blob.OriginalPath(rec.Hash, extForMIME(rec.MIME))
		if err := svc.blob.Remove(path); err != nil {
			t.Fatal(err)
		}
	}

	res, err := gc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.MissingUnrecoverable != 3 {
		t.Fatalf("Run = %+v, want three unrecoverable rows counted", res)
	}
	// ⚠ One message for the sweep, not one per image. An operator who restored onto an empty image
	// directory has hundreds of these, and hundreds of feed rows would bury the one fact they need.
	if len(notifier.msgs) != 1 {
		t.Fatalf("%d notifications for one sweep, want exactly 1", len(notifier.msgs))
	}
	// ⚠ The message must name the REMEDY. "3 images are missing" is not actionable; "back up the
	// /data volume" is the sentence §22 requires to reach the operator.
	if !strings.Contains(notifier.msgs[0], "/data") {
		t.Errorf("the warning does not tell the operator what to do about it: %q", notifier.msgs[0])
	}

	// ⚠ The rows are NOT deleted. They are still referenced by their channels, and deleting them
	// would turn a visible, explained loss into a silent one.
	if _, err := fs.ListUnrecoverable(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if rows, _ := fs.ListUnrecoverable(ctx, 10); len(rows) != 3 {
		t.Errorf("%d unrecoverable rows remain, want all 3 kept so the loss stays visible", len(rows))
	}
}

// A healthy install says nothing at all.
func TestGCIsSilentWhenNothingIsMissing(t *testing.T) {
	gc, svc, _, notifier := newTestGC(t, 0, 0)
	ctx := context.Background()

	if _, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(120, 180))), IngestRequest{
		Role: RoleIcon, Origin: OriginUpload, OwnerKind: "channel", OwnerID: "c1",
	}); err != nil {
		t.Fatal(err)
	}

	res, err := gc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.MissingUnrecoverable != 0 || len(notifier.msgs) != 0 {
		t.Errorf("a healthy install produced %d warnings (%+v)", len(notifier.msgs), res)
	}
}
