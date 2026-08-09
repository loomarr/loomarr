package images

import (
	"bytes"
	"context"
	"errors"
	"image"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeAVIF returns an encoder that writes plausible bytes and records what it was asked for.
//
// ⚠ The real encoder forks ffmpeg, so a test that used it would be testing libaom rather than this
// job. What the job actually owns is which rungs it asks for, how it records what came back, and
// what it does when the encoder lies — all of which this can drive and none of which needs AV1.
func fakeAVIF(calls *[]encodeCall, body []byte) AVIFEncoder {
	return func(_ context.Context, img image.Image, dst string) error {
		*calls = append(*calls, encodeCall{width: img.Bounds().Dx(), dst: dst})
		if err := os.MkdirAll(dirOf(dst), 0o750); err != nil {
			return err
		}
		return os.WriteFile(dst, body, 0o600)
	}
}

// encodeCall pairs the image handed to the encoder with the path it was told to write.
//
// ⚠ Recording BOTH is the point. The job resizes the whole ladder in one pass and then walks the
// resulting map to write each rung, so the failure available here is a mismatched pairing — a
// 780px image written to `…_w154.avif`. Every rung would exist, every row would look right, and
// the srcset would hand a browser five copies of the same size under five different names.
type encodeCall struct {
	width int
	dst   string
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}

// seedWithWebP ingests an image and produces one WebP rendition, which is what puts it on the
// AVIF job's work list ("has a rendition, but not this format").
func seedWithWebP(t *testing.T, svc *Service, role Role) Image {
	t.Helper()
	ctx := context.Background()
	rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(900, 1350))), IngestRequest{
		Role: role, Origin: OriginUpload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rendition(ctx, rec.Hash, FormatWebP, role.Widths()[0]); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestAVIFJobEncodesTheWholeLadderAndRecordsWhatIsOnDisk(t *testing.T) {
	svc, fs := newTestService(t)
	var calls []encodeCall
	job := NewAVIFJob(svc, fs, fakeAVIF(&calls, []byte("avif-bytes")), nil)
	ctx := context.Background()

	rec := seedWithWebP(t, svc, RolePoster)

	res, err := job.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	widths := RolePoster.Widths()
	if res.Images != 1 || res.Renditions != len(widths) {
		t.Fatalf("Run = %+v, want one image and %d renditions (the whole poster ladder)", res, len(widths))
	}

	// ⚠ Each rung's IMAGE must match the width its FILENAME claims. Nothing else in the pipeline
	// checks this: a mispaired ladder produces every expected file and every expected row, and the
	// only symptom is that a browser picking `w780` from the srcset gets 154 pixels stretched over
	// a tile. The paths are content-addressed, so the mistake would also be cached as immutable.
	for _, c := range calls {
		if !strings.Contains(c.dst, "_w"+strconv.Itoa(c.width)+".avif") {
			t.Errorf("a %dpx image was written to %s — the rung and its filename disagree", c.width, c.dst)
		}
	}
	if len(calls) != len(widths) {
		t.Errorf("the encoder ran %d times for a %d-rung ladder", len(calls), len(widths))
	}

	got, err := fs.ListDerivatives(ctx, rec.Hash)
	if err != nil {
		t.Fatal(err)
	}
	var avifRows int
	for _, d := range got {
		if d.Format != FormatAVIF {
			continue
		}
		avifRows++
		if d.Bytes != int64(len("avif-bytes")) {
			t.Errorf("derivative w%d records %d bytes, want the size measured from disk", d.Width, d.Bytes)
		}
	}
	if avifRows != len(widths) {
		t.Errorf("%d AVIF rows recorded, want %d", avifRows, len(widths))
	}

	// Idempotent: a second pass finds nothing to do, because the work list is now empty.
	calls = nil
	if res, err = job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if res.Considered != 0 || len(calls) != 0 {
		t.Errorf("a second pass re-encoded %d rungs — the job would fork ffmpeg over the whole "+
			"catalog on every tick", len(calls))
	}
}

// ffmpeg can exit 0 having written nothing. A zero-byte derivative recorded as present renders as
// a BROKEN image, which is strictly worse than an absent one — absent has a designed fallback.
func TestAVIFJobDoesNotRecordAnEncoderThatWroteNothing(t *testing.T) {
	svc, fs := newTestService(t)
	empty := func(_ context.Context, _ image.Image, dst string) error {
		if err := os.MkdirAll(dirOf(dst), 0o750); err != nil {
			return err
		}
		return os.WriteFile(dst, nil, 0o600) // exit 0, zero bytes
	}
	job := NewAVIFJob(svc, fs, empty, nil)
	ctx := context.Background()

	rec := seedWithWebP(t, svc, RoleIcon)
	if _, err := job.Run(ctx); err != nil {
		t.Fatal(err)
	}

	got, _ := fs.ListDerivatives(ctx, rec.Hash)
	for _, d := range got {
		if d.Format == FormatAVIF {
			t.Fatalf("recorded a zero-byte AVIF rendition at w%d — it would be served as a broken image", d.Width)
		}
	}
}

// `images.formats` is the operator's CPU switch, and it must be read per run.
func TestAVIFJobHonoursTheFormatsSetting(t *testing.T) {
	fs := newFakeStore()
	formats := []Format{FormatWebP, FormatJPEG} // avif dropped
	svc := New(Config{
		Dir:            t.TempDir(),
		MaxUploadBytes: func() int64 { return 2 << 20 },
		Formats:        func() []Format { return formats },
	}, fs, func() time.Time { return fixedNow })

	var calls []encodeCall
	job := NewAVIFJob(svc, fs, fakeAVIF(&calls, []byte("x")), nil)
	ctx := context.Background()
	seedWithWebP(t, svc, RolePoster)

	if _, err := job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatalf("images.formats excludes avif and the job still encoded %d rungs", len(calls))
	}

	// Turned back on, the SAME job instance starts working — hot-apply, not restart.
	formats = append(formats, FormatAVIF)
	if _, err := job.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(calls) == 0 {
		t.Error("re-enabling avif needed a restart — the setting is captured at construction")
	}
}

// A build whose ffmpeg carries no AV1 encoder must degrade to "no AVIF", not to a failing job.
func TestAVIFJobWithNoEncoderIsANoOp(t *testing.T) {
	svc, fs := newTestService(t)
	job := NewAVIFJob(svc, fs, nil, nil)
	seedWithWebP(t, svc, RolePoster)

	res, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("a build with no AV1 encoder failed the job instead of skipping it: %v", err)
	}
	if res.Renditions != 0 {
		t.Errorf("Run = %+v, want nothing produced", res)
	}
}

// One unreadable original must not stall the batch behind it — the work list is ordered, so the
// same image would be first again on every subsequent pass.
func TestAVIFJobKeepsGoingPastAFailure(t *testing.T) {
	svc, fs := newTestService(t)
	var seen int
	flaky := func(_ context.Context, _ image.Image, dst string) error {
		seen++
		if seen == 1 {
			return errors.New("encoder blew up")
		}
		if err := os.MkdirAll(dirOf(dst), 0o750); err != nil {
			return err
		}
		return os.WriteFile(dst, []byte("ok"), 0o600)
	}
	job := NewAVIFJob(svc, fs, flaky, nil)
	ctx := context.Background()

	// Two images on the work list: distinct bytes, so distinct hashes.
	for _, dim := range [][2]int{{800, 1200}, {640, 960}} {
		rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(dim[0], dim[1]))),
			IngestRequest{Role: RolePoster, Origin: OriginUpload})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Rendition(ctx, rec.Hash, FormatWebP, 154); err != nil {
			t.Fatal(err)
		}
	}

	res, err := job.Run(ctx)
	if err != nil {
		t.Fatalf("one failing image failed the whole pass: %v", err)
	}
	if res.Failed != 1 || res.Images != 1 {
		t.Errorf("Run = %+v, want one failure and one image still encoded", res)
	}
}
