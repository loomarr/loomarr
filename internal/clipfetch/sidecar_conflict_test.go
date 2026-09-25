package clipfetch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// foreignSidecar leaves at root/download.info.json a valid sidecar that belongs to ANOTHER
// acquisition, and returns its bytes.
func foreignSidecar(t *testing.T, root string) []byte {
	t.Helper()
	other := filepath.Join(t.TempDir(), "download.mp4")
	if err := os.WriteFile(other, []byte("someone else's bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filler.WriteSidecarTags(other, filler.SidecarTags{SourceID: "youtube:other", AcquisitionID: "acq-other"}, true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(strings.TrimSuffix(other, ".mp4") + ".info.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "download.info.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return raw
}

// copyStagedSidecar reproduces a previous attempt that published the sidecar and died before the media.
func copyStagedSidecar(t *testing.T, root string, artifact filler.AcquisitionArtifact) []byte {
	t.Helper()
	stage := strings.TrimSuffix(filepath.Join(root, filepath.FromSlash(artifact.StagingPath)), ".mp4") + ".info.json"
	raw, err := os.ReadFile(stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "download.info.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func publishFixture(t *testing.T, root string, artifact filler.AcquisitionArtifact) (*Ingestor, []Output) {
	t.Helper()
	db := openRecoveryStore(t)
	seedRecoveryArtifact(t, db, artifact)
	stage := filepath.Join(root, filepath.FromSlash(artifact.StagingPath))
	ing := &Ingestor{dropDir: root, writer: db, now: time.Now}
	return ing, []Output{{MediaPath: stage, SidecarPath: strings.TrimSuffix(stage, ".mp4") + ".info.json"}}
}

// ⚠ #1395: 35 artifacts sat in repair with "publish sidecar: target … already exists" because a
// previous attempt had left its own sidecar behind and publication refused to notice.
func TestPublish_ExistingIdenticalSidecarIsIdempotentSuccess(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	copyStagedSidecar(t, root, artifact)
	ing, outputs := publishFixture(t, root, artifact)

	got, err := ing.publish(t.Context(), []filler.AcquisitionArtifact{artifact}, outputs)
	if err != nil || got[0].State != filler.ArtifactPublished {
		t.Fatalf("publish = %+v, %v; want published", got[0], err)
	}
	if _, err := os.Stat(filepath.Join(root, "download.mp4")); err != nil {
		t.Fatalf("media not published beside the pre-existing sidecar: %v", err)
	}
}

func TestPublish_ConflictingSidecarIsReportedWithBothIdentitiesAndNeverOverwritten(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	before := foreignSidecar(t, root)
	ing, outputs := publishFixture(t, root, artifact)

	got, err := ing.publish(t.Context(), []filler.AcquisitionArtifact{artifact}, outputs)
	if err == nil {
		t.Fatal("a sidecar belonging to another acquisition must be a conflict")
	}
	for _, id := range []string{"acq-other", "acq-1"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("conflict %q does not name %s", err, id)
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, "download.info.json"))
	if string(after) != string(before) {
		t.Fatal("another item's sidecar was overwritten")
	}
	if got[0].State != filler.ArtifactRepair {
		t.Fatalf("state = %s, want repair", got[0].State)
	}
	if _, err := os.Stat(filepath.Join(root, "download.mp4")); !os.IsNotExist(err) {
		t.Fatalf("media published next to a conflicting sidecar: %v", err)
	}
}

const sidecarExistsReason = "publish sidecar: target /data/filler/_watch/download.info.json already exists"

func repairRow(artifact filler.AcquisitionArtifact, reason string) filler.AcquisitionArtifact {
	artifact.State = filler.ArtifactRepair
	artifact.RepairReason = reason
	return artifact
}

func TestRecover_SidecarAlreadyExistsRepairRowIsRepublished(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	copyStagedSidecar(t, root, artifact)
	db := openRecoveryStore(t)
	seedRecoveryArtifact(t, db, repairRow(artifact, sidecarExistsReason))

	result, err := RecoverAcquisitionArtifacts(t.Context(), root, root, db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if err != nil || !found || result.Published != 1 || got.State != filler.ArtifactPublished || got.RepairReason != "" {
		t.Fatalf("recovery = %+v, artifact = %+v, found=%v, err=%v; want the repair row republished", result, got, found, err)
	}
}

func TestRecover_SidecarAlreadyExistsConflictSettlesAndIsNotRetriedForever(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	before := foreignSidecar(t, root)
	db := openRecoveryStore(t)
	seedRecoveryArtifact(t, db, repairRow(artifact, sidecarExistsReason))

	if _, err := RecoverAcquisitionArtifacts(t.Context(), root, root, db, time.Now); err != nil {
		t.Fatal(err)
	}
	got, _, _ := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if got.State != filler.ArtifactRepair || strings.Contains(got.RepairReason, "already exists") || !strings.Contains(got.RepairReason, "acq-other") {
		t.Fatalf("artifact = %+v, want a settled repair naming the other identity", got)
	}
	settled := got.RepairReason
	if _, err := RecoverAcquisitionArtifacts(t.Context(), root, root, db, time.Now); err != nil {
		t.Fatal(err)
	}
	again, _, _ := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if again.RepairReason != settled {
		t.Fatalf("settled reason changed on rerun: %q -> %q", settled, again.RepairReason)
	}
	after, _ := os.ReadFile(filepath.Join(root, "download.info.json"))
	if string(after) != string(before) {
		t.Fatal("another item's sidecar was overwritten")
	}
}

func TestRecover_MissingMediaWithNoFileAnywhereBecomesTerminal(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	if err := os.RemoveAll(filepath.Join(root, ".loomarr-acquisitions")); err != nil {
		t.Fatal(err)
	}
	db := openRecoveryStore(t)
	seedRecoveryArtifact(t, db, repairRow(artifact, filler.ArtifactMediaMissing))

	if _, err := RecoverAcquisitionArtifacts(t.Context(), root, root, db, time.Now); err != nil {
		t.Fatal(err)
	}
	got, _, _ := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if got.State != filler.ArtifactRepair || got.RepairReason != filler.ArtifactMediaGone {
		t.Fatalf("artifact = %+v, want terminal %q", got, filler.ArtifactMediaGone)
	}
}

func TestRecover_MissingMediaWhoseFileExistsIsLeftForSync(t *testing.T) {
	root := t.TempDir()
	artifact := stagedRecoveryArtifact(t, root)
	media, err := filler.ClipPath(root, artifact.ClipHash, ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(media), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, filepath.FromSlash(artifact.StagingPath)), media); err != nil {
		t.Fatal(err)
	}
	db := openRecoveryStore(t)
	seedRecoveryArtifact(t, db, repairRow(artifact, filler.ArtifactMediaMissing))

	if _, err := RecoverAcquisitionArtifacts(t.Context(), root, root, db, time.Now); err != nil {
		t.Fatal(err)
	}
	got, _, _ := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if got.RepairReason != filler.ArtifactMediaMissing {
		t.Fatalf("reason = %q; a present file must stay retryable, not be declared gone", got.RepairReason)
	}
}
