package filler_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

const missingMediaReason = "manifested media is missing, symlinked, or not a regular file"

// repairedClipFixture lays down a downloaded clip under its content-hash path (as the transcode
// re-key leaves it) plus a manifest row parked in repair with the given reason.
func repairedClipFixture(t *testing.T, reason string) (dir string, authority *manifestAuthority, source *fakeSource, now time.Time) {
	t.Helper()
	dir = t.TempDir()
	temporary := filepath.Join(t.TempDir(), "download.mp4")
	if err := os.WriteFile(temporary, []byte("downloaded video bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	clipHash, err := filler.ClipID(temporary)
	if err != nil {
		t.Fatal(err)
	}
	media, err := filler.ClipPath(dir, clipHash, ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(media), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, media); err != nil {
		t.Fatal(err)
	}
	digest, size, err := filler.FileSHA256(media)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(dir, media)
	if err != nil {
		t.Fatal(err)
	}
	now = time.Unix(1_800_000_000, 0).UTC()
	authority = &manifestAuthority{found: true, artifact: filler.AcquisitionArtifact{
		ID: "artifact-1", AcquisitionID: "acq-1", SourceID: "youtube:classic",
		Provider: "youtube", SourceURL: "https://youtube.com/watch?v=one",
		StagingPath: ".loomarr-acquisitions/acq-1/download.mp4", MediaPath: "download.mp4",
		SidecarPath: "download.info.json", MediaSHA256: digest, MediaBytes: size,
		ClipHash: clipHash, State: filler.ArtifactRepair, RepairReason: reason,
		CompletedAt: now, UpdatedAt: now,
	}}
	source = &fakeSource{clips: []filler.RawClip{raw(clipHash, "Downloaded ad", filler.Commercial, 30_000, 0)}}
	source.clips[0].Path = filepath.ToSlash(relative)
	return dir, authority, source, now
}

// ⚠ #1395: a transient "media is missing" (seen mid-transcode re-key) was stored as a permanent
// repair and returned without ever looking at the file again, so two clips whose files sit on
// disk, intact, never entered the catalog.
func TestSync_TransientMissingMediaRepairIsReverifiedAndConsumed(t *testing.T) {
	dir, authority, source, now := repairedClipFixture(t, missingMediaReason)
	st := newMemStore()
	syncer := filler.NewSyncer(source, st, testLayout(dir), func() time.Time { return now }, discardLog()).
		WithAcquisitionManifests(authority)

	if _, err := syncer.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.clips[authority.artifact.ClipHash]; !ok {
		t.Fatal("clip whose file is present and intact never entered the catalog")
	}
	if authority.artifact.State != filler.ArtifactConsumed || authority.artifact.RepairReason != "" {
		t.Fatalf("artifact = %+v, want consumed with the stale repair reason cleared", authority.artifact)
	}
}

// A repair that is NOT retryable stays quarantined, but must not shout about it every sync.
func TestSync_QuarantinedArtifactWarnsOncePerReason(t *testing.T) {
	dir, authority, source, now := repairedClipFixture(t, "staged sidecar validation: sidecar belongs to another item")
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	st := newMemStore()
	syncer := filler.NewSyncer(source, st, testLayout(dir), func() time.Time { return now }, log).
		WithAcquisitionManifests(authority)

	for range 3 {
		if _, err := syncer.Sync(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(buf.String(), "remains quarantined"); n != 1 {
		t.Fatalf("quarantine warned %d times over 3 syncs, want once:\n%s", n, buf.String())
	}
	if _, ok := st.clips[authority.artifact.ClipHash]; ok {
		t.Fatal("non-retryable repair must keep the clip out of the catalog")
	}
}
