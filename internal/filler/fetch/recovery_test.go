package fetch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/testkit"
)

func openRecoveryStore(t *testing.T) records.Store {
	t.Helper()
	db, err := records.Open(t.Context(), testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedRecoveryArtifact(t *testing.T, db records.Store, artifact acquisition.AcquisitionArtifact) {
	t.Helper()
	if err := db.UpsertAcquisitionRun(t.Context(), acquisition.AcquisitionRun{
		ID: artifact.AcquisitionID, Trigger: acquisition.AcquisitionPull, SourceID: artifact.SourceID,
		Status: acquisition.AcquisitionSuccess, Requested: 1, UpdatedAt: artifact.UpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAcquisitionArtifacts(t.Context(), []acquisition.AcquisitionArtifact{artifact}); err != nil {
		t.Fatal(err)
	}
}

func stagedRecoveryArtifact(t *testing.T, root string) acquisition.AcquisitionArtifact {
	t.Helper()
	stage := filepath.Join(root, ".loomarr-acquisitions", "acq-1", "000", "download.mp4")
	if err := os.MkdirAll(filepath.Dir(stage), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage, []byte("downloaded video bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := asset.WriteSidecarTags(stage, asset.SidecarTags{
		SourceID: "youtube:classic", AcquisitionID: "acq-1",
	}, true); err != nil {
		t.Fatal(err)
	}
	digest, size, err := asset.FileSHA256(stage)
	if err != nil {
		t.Fatal(err)
	}
	clipHash, err := asset.ClipID(stage)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	return acquisition.AcquisitionArtifact{
		ID: "artifact-1", AcquisitionID: "acq-1", SourceID: "youtube:classic",
		Provider: "youtube", SourceURL: "https://youtube.com/watch?v=one",
		StagingPath: filepath.ToSlash(filepath.Join(".loomarr-acquisitions", "acq-1", "000", "download.mp4")),
		MediaPath:   "download.mp4", SidecarPath: "download.info.json",
		MediaSHA256: digest, MediaBytes: size, ClipHash: clipHash,
		ProviderArchiveEntry: "youtube one", ProviderArchiveCommitted: true,
		State: acquisition.ArtifactStaged, CompletedAt: now, UpdatedAt: now,
	}
}

func TestRecoverAcquisitionArtifacts_PublishesValidatedStagedPair(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	db := openRecoveryStore(t)
	seedRecoveryArtifact(t, db, artifact)

	result, err := RecoverAcquisitionArtifacts(t.Context(), root, root, db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if err != nil || !found || result.Published != 1 || got.State != acquisition.ArtifactPublished {
		t.Fatalf("recovery = %+v, artifact = %+v, found=%v, err=%v", result, got, found, err)
	}
	for _, path := range []string{"download.mp4", "download.info.json"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("published %s: %v", path, err)
		}
	}
}

func TestRecoverAcquisitionArtifacts_DigestSubstitutionBecomesRepair(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	stage := filepath.Join(root, filepath.FromSlash(artifact.StagingPath))
	if err := os.WriteFile(stage, []byte("substituted bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := openRecoveryStore(t)
	seedRecoveryArtifact(t, db, artifact)

	result, err := RecoverAcquisitionArtifacts(t.Context(), root, root, db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if err != nil || !found || result.Repair != 1 || got.State != acquisition.ArtifactRepair || got.RepairReason == "" {
		t.Fatalf("recovery = %+v, artifact = %+v, found=%v, err=%v", result, got, found, err)
	}
	if _, err := os.Stat(filepath.Join(root, artifact.MediaPath)); !os.IsNotExist(err) {
		t.Fatalf("substituted media was published: %v", err)
	}
}

func TestRecoverAcquisitionArtifacts_CountsTransitionedArtifactOnce(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	stage := filepath.Join(root, filepath.FromSlash(artifact.StagingPath))
	if err := os.WriteFile(stage, []byte("substituted bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := openRecoveryStore(t)
	seedRecoveryArtifact(t, db, artifact)
	recoveredAt := artifact.UpdatedAt.Add(time.Hour)

	result, err := RecoverAcquisitionArtifacts(t.Context(), root, root, db, func() time.Time { return recoveredAt })
	if err != nil {
		t.Fatal(err)
	}
	if result.Repair != 1 {
		t.Fatalf("recovery = %+v, want one repair count for one manifest", result)
	}
	got, found, err := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if err != nil || !found || !got.UpdatedAt.Equal(recoveredAt) {
		t.Fatalf("artifact = %+v, found=%v, err=%v; want later recovery timestamp", got, found, err)
	}
}

func TestRecoverAcquisitionArtifacts_HoldsHistoricalYtDlpStageWithoutExactArchiveEntry(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	artifact.ProviderArchiveEntry = ""
	artifact.ProviderArchiveCommitted = false
	db := openRecoveryStore(t)
	seedRecoveryArtifact(t, db, artifact)

	result, err := RecoverAcquisitionArtifacts(t.Context(), root, root, db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if err != nil || !found || result.Repair != 1 || got.State != acquisition.ArtifactRepair {
		t.Fatalf("recovery = %+v, artifact = %+v, found=%v, err=%v", result, got, found, err)
	}
	if _, err := os.Stat(filepath.Join(root, artifact.MediaPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("historical staged media became visible: %v", err)
	}
}
