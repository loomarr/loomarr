package images

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/mantonx/loomarr/internal/images/rustgen"
)

func seedWithWebP(t *testing.T, svc *Service, role Role, width, height int) Image {
	t.Helper()
	ctx := context.Background()
	rec, err := svc.Ingest(ctx, bytes.NewReader(pngBytes(t, testImage(width, height))), IngestRequest{
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

func TestAVIFJobUsesRustWorkerForWholeLadder(t *testing.T) {
	svc, fs := newTestService(t)
	rec := seedWithWebP(t, svc, RoleIcon, 64, 64)
	job := NewAVIFJob(svc, fs, nil)

	result, err := job.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Images != 1 || result.Renditions != len(RoleIcon.Widths()) {
		t.Fatalf("Run = %+v, want one complete icon ladder", result)
	}
	rows, err := fs.ListDerivatives(context.Background(), rec.Hash)
	if err != nil {
		t.Fatal(err)
	}
	var avif int
	for _, row := range rows {
		if row.Format != FormatAVIF {
			continue
		}
		avif++
		if row.Recipe != renditionRecipe || row.OutputHash == "" || row.Animated {
			t.Errorf("AVIF provenance = %+v", row)
		}
		data, readErr := os.ReadFile(row.Path)
		if readErr != nil || len(data) < 12 || string(data[4:8]) != "ftyp" {
			t.Errorf("AVIF w%d is not a real ISOBMFF image: %v", row.Width, readErr)
		}
	}
	if avif != len(RoleIcon.Widths()) {
		t.Errorf("AVIF rows = %d, want %d", avif, len(RoleIcon.Widths()))
	}

	second, err := job.Run(context.Background())
	if err != nil || second.Considered != 0 {
		t.Errorf("idempotent second pass = %+v, %v", second, err)
	}
}

type failOnceRenderer struct {
	mu       sync.Mutex
	failures int
	next     Renderer
}

func (r *failOnceRenderer) Generate(ctx context.Context, req rustgen.Request) (rustgen.Manifest, error) {
	r.mu.Lock()
	if r.failures > 0 {
		r.failures--
		r.mu.Unlock()
		return rustgen.Manifest{}, errors.New("worker failed")
	}
	r.mu.Unlock()
	return r.next.Generate(ctx, req)
}

func TestAVIFJobDoesNotRecordWorkerFailureAndContinues(t *testing.T) {
	svc, fs := newTestService(t)
	first := seedWithWebP(t, svc, RoleIcon, 32, 32)
	second := seedWithWebP(t, svc, RoleIcon, 48, 48)
	failed, succeeded := first, second
	if second.Hash < first.Hash {
		failed, succeeded = second, first
	}
	real := svc.renderer
	svc.renderer = &failOnceRenderer{failures: 1, next: real}

	result, err := NewAVIFJob(svc, fs, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Images != 1 {
		t.Errorf("Run = %+v, want one failed image and one completed image", result)
	}
	for _, row := range fs.derivatives[failed.Hash] {
		if row.Format == FormatAVIF {
			t.Fatalf("failed worker recorded phantom AVIF: %+v", row)
		}
	}
	found := false
	for _, row := range fs.derivatives[succeeded.Hash] {
		found = found || row.Format == FormatAVIF
	}
	if !found {
		t.Error("one worker failure stalled the following image")
	}
}
