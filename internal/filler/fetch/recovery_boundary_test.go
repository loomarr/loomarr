package fetch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/records"
)

func recoveryBoundaryArtifact(t *testing.T, root, id string, state acquisition.ArtifactState, at time.Time) acquisition.Artifact {
	t.Helper()
	media := filepath.Join(root, "download-"+id+".mp4")
	if err := os.WriteFile(media, []byte("recovery boundary bytes "+id), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := asset.WriteSidecarTags(media, asset.SidecarTags{SourceID: "youtube:classic", AcquisitionID: "acq-boundary"}, true); err != nil {
		t.Fatal(err)
	}
	digest, size, err := asset.FileSHA256(media)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := asset.ClipID(media)
	if err != nil {
		t.Fatal(err)
	}
	return acquisition.Artifact{
		ID: id, AcquisitionID: "acq-boundary", SourceID: "youtube:classic", Provider: "youtube",
		SourceURL: "https://youtube.com/watch?v=boundary", StagingPath: filepath.ToSlash(filepath.Join(".loomarr-acquisitions", id, "download.mp4")),
		MediaPath: filepath.Base(media), SidecarPath: strings.TrimSuffix(filepath.Base(media), ".mp4") + ".info.json",
		MediaSHA256: digest, MediaBytes: size, ClipHash: hash, State: state, RepairReason: repairReason(state),
		ProviderArchiveEntry: "youtube " + id, ProviderArchiveCommitted: true,
		CompletedAt: at, UpdatedAt: at,
	}
}

func repairReason(state acquisition.ArtifactState) string {
	if state == acquisition.ArtifactRepair {
		return "boundary repair"
	}
	return ""
}

func seedBoundaryRun(t *testing.T, db records.Store, at time.Time) {
	t.Helper()
	if err := db.UpsertAcquisitionRun(t.Context(), acquisition.AcquisitionRun{ID: "acq-boundary", Trigger: acquisition.TriggerPull, SourceID: "youtube:classic", Status: acquisition.RunSuccess, Requested: 1, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
}

func moveBoundaryMedia(t *testing.T, watch, catalog string, artifact acquisition.Artifact, sidecar bool) string {
	t.Helper()
	source := filepath.Join(watch, artifact.MediaPath)
	destination, err := cliplibrary.ClipPath(catalog, artifact.ClipHash, filepath.Ext(artifact.MediaPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, destination); err != nil {
		t.Fatal(err)
	}
	if sidecar {
		if err := asset.WriteSidecarTags(destination, asset.SidecarTags{SourceID: artifact.SourceID, AcquisitionID: artifact.AcquisitionID}, true); err != nil {
			t.Fatal(err)
		}
	}
	return destination
}

func TestRecoverAcquisitionArtifacts_RestoresMissingCatalogProvenance(t *testing.T) {
	watch, catalog := t.TempDir(), t.TempDir()
	db := openRecoveryStore(t)
	at := time.Unix(1_900_000_000, 0).UTC()
	seedBoundaryRun(t, db, at)
	artifact := recoveryBoundaryArtifact(t, watch, "missing-sidecar", acquisition.ArtifactPublished, at)
	if err := db.UpsertAcquisitionArtifacts(t.Context(), []acquisition.Artifact{artifact}); err != nil {
		t.Fatal(err)
	}
	destination := moveBoundaryMedia(t, watch, catalog, artifact, false)
	if _, err := RecoverAcquisitionArtifacts(t.Context(), watch, catalog, db, func() time.Time { return at.Add(time.Minute) }); err != nil {
		t.Fatal(err)
	}
	got, found, err := db.AcquisitionArtifactForClip(t.Context(), "unrelated.mp4", artifact.ClipHash)
	if err != nil || !found || got.State != acquisition.ArtifactConsumed {
		t.Fatalf("artifact = %+v, found=%v, err=%v", got, found, err)
	}
	if _, err := os.Stat(strings.TrimSuffix(destination, ".mp4") + ".info.json"); err != nil {
		t.Fatalf("restored catalog provenance: %v", err)
	}
}

func TestRecoverAcquisitionArtifacts_DoesNotReplaceMalformedCatalogProvenance(t *testing.T) {
	watch, catalog := t.TempDir(), t.TempDir()
	db := openRecoveryStore(t)
	at := time.Unix(1_900_000_000, 0).UTC()
	seedBoundaryRun(t, db, at)
	artifact := recoveryBoundaryArtifact(t, watch, "malformed-sidecar", acquisition.ArtifactPublished, at)
	if err := db.UpsertAcquisitionArtifacts(t.Context(), []acquisition.Artifact{artifact}); err != nil {
		t.Fatal(err)
	}
	destination := moveBoundaryMedia(t, watch, catalog, artifact, false)
	sidecar := strings.TrimSuffix(destination, ".mp4") + ".info.json"
	if err := os.WriteFile(sidecar, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverAcquisitionArtifacts(t.Context(), watch, catalog, db, func() time.Time { return at.Add(time.Minute) }); err != nil {
		t.Fatal(err)
	}
	got, found, err := db.AcquisitionArtifactForClip(t.Context(), artifact.MediaPath, artifact.ClipHash)
	if err != nil || !found || got.State != acquisition.ArtifactRepair {
		t.Fatalf("artifact = %+v, found=%v, err=%v", got, found, err)
	}
	contents, err := os.ReadFile(sidecar)
	if err != nil || string(contents) != "not json" {
		t.Fatalf("sidecar was changed: %q, %v", contents, err)
	}
}

func TestRecoverAcquisitionArtifacts_PagesPastOldNonterminalRows(t *testing.T) {
	for _, state := range []acquisition.ArtifactState{acquisition.ArtifactRepair, acquisition.ArtifactPublished} {
		t.Run(string(state), func(t *testing.T) {
			watch, catalog := t.TempDir(), t.TempDir()
			db := openRecoveryStore(t)
			at := time.Unix(1_900_000_000, 0).UTC()
			seedBoundaryRun(t, db, at)
			for i := 0; i < 500; i++ {
				artifact := recoveryBoundaryArtifact(t, watch, "old-"+time.Unix(int64(i), 0).UTC().Format("150405.000000000"), state, at.Add(time.Duration(i)*time.Second))
				if err := db.UpsertAcquisitionArtifacts(t.Context(), []acquisition.Artifact{artifact}); err != nil {
					t.Fatal(err)
				}
			}
			target := recoveryBoundaryArtifact(t, watch, "target", acquisition.ArtifactStaged, at.Add(501*time.Second))
			if err := db.UpsertAcquisitionArtifacts(t.Context(), []acquisition.Artifact{target}); err != nil {
				t.Fatal(err)
			}
			stage := filepath.Join(watch, target.StagingPath)
			if err := os.MkdirAll(filepath.Dir(stage), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(watch, target.MediaPath), stage); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(watch, target.SidecarPath), strings.TrimSuffix(stage, ".mp4")+".info.json"); err != nil {
				t.Fatal(err)
			}
			if _, err := RecoverAcquisitionArtifacts(t.Context(), watch, catalog, db, func() time.Time { return at.Add(10 * time.Minute) }); err != nil {
				t.Fatal(err)
			}
			got, found, err := db.AcquisitionArtifactForClip(t.Context(), target.MediaPath, target.ClipHash)
			if err != nil || !found || got.State != acquisition.ArtifactPublished {
				t.Fatalf("target = %+v, found=%v, err=%v", got, found, err)
			}
		})
	}
}
